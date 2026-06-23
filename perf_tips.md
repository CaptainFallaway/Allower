# Proxy performance tips

(Researched with gpt-5.5 and summarized here from benchmark conclusions i've had)

This document collects ideas and tradeoffs for improving Allower's TCP proxy performance, especially under high connection churn and denied-traffic workloads.

## 1. Separate the workloads

Different benchmarks stress very different parts of the system.

| Workload | Example | Likely bottleneck |
| --- | --- | --- |
| Long-lived allowed TCP streams | `iperf3` through the proxy | byte copying, NIC, kernel TCP stack |
| High-rate denied TCP connections | `wrk -c5000` from a blocked peer | accepts/sec, closes/sec, kernel socket churn |
| Many short-lived allowed connections | HTTP without keep-alive, small RPCs | frontend accepts, backend dials, goroutine churn |

A proxy can perform very well for long-lived streams while struggling under denied connection storms. Benchmark and optimize these cases separately.

## 2. Add better internal measurements

Before changing architecture, add cheap counters around the hot paths.

Useful metrics:

- accepted TCP connections/sec
- denied TCP connections/sec
- allowed TCP connections/sec
- active proxied TCP connections
- active copy goroutines
- failed backend dials/sec
- backend dial latency
- bytes copied client to target
- bytes copied target to client
- connection lifetime histogram
- denied IP cardinality
- top denied IPs
- goroutine count
- open file descriptor count

Good places to instrument:

- `AcceptTCP()` in `internal/proxy/entrypoint.go`
- the deny path in `internal/proxy/handleconn.go`
- successful backend dials
- failed backend dials
- copy completion in both directions

Current rule-decision metrics are useful, but they do not show the biggest pressure during denial storms: connection churn.

## 3. Track denies and escalate abusive IPs

A useful design is to track repeated denied connections in userspace, then temporarily block abusive IPs at a lower layer.

Conceptual flow:

```text
connection accepted
    -> extract remote IP
    -> rules deny
    -> increment deny tracker for IP
    -> if threshold exceeded:
           add temporary kernel/blocklist entry
    -> close connection
```

Example policy:

```text
100 denied connections in 10s -> block for 5m
repeat after unblock          -> block for 30m
repeat again                  -> block for 2h
```

Implementation notes:

- use decay/sliding windows so the map does not grow forever
- shard the tracker to avoid a single global lock
- cap the maximum number of tracked IPs
- expire old entries periodically
- avoid logging every denied connection
- expose current temporary block count
- treat IPv4 and IPv6 correctly
- be careful with NATed users, where one IP may represent many clients

A rough in-memory structure:

```go
type DenyTracker struct {
    shards [256]denyShard
}

type denyShard struct {
    mu  sync.Mutex
    ips map[netip.Addr]*denyState
}

type denyState struct {
    count      int
    windowFrom time.Time
    blockedTil time.Time
    strikes    int
}
```

## 4. Kernel-level blocking from a scratch image

Running in a `scratch` image does not prevent kernel-level blocking. The container does not need the `nft` binary if the Go process talks to the kernel directly.

### Option A: nftables via Go netlink

Use a Go library such as `github.com/google/nftables` to update nftables tables/sets directly through netlink.

Requirements:

- the process must run in the relevant network namespace
- the container needs sufficient privileges, usually `CAP_NET_ADMIN`
- the host kernel must support nftables
- the app may need to create the table/chain/set or assume they already exist

Architecture:

```text
Allower
    -> deny tracker
    -> nftables netlink API
    -> temporary blocked IP set
```

This works from `scratch` because it is just a Go binary making syscalls/netlink messages.

### Option B: privileged firewall helper

Keep Allower unprivileged and run a small privileged sidecar/helper that owns firewall mutation.

```text
Allower, unprivileged
    -> request: block 1.2.3.4 for 5m

Firewall helper, privileged
    -> update nftables/eBPF/ipset
```

Communication options:

- Unix socket
- localhost HTTP/gRPC
- host-level API
- Kubernetes API object, if applicable

This is often cleaner operationally because the proxy itself does not need `CAP_NET_ADMIN`.

### Option C: host-level service

On VMs, a systemd service on the host can manage firewall state. Allower can call it through a small restricted API.

This avoids giving the container broad network-admin privileges.

### Option D: eBPF/XDP

Advanced but very fast:

```text
Allower
    -> updates BPF map of blocked IPs

XDP/tc/cgroup eBPF program
    -> drops/rejects packets from blocked IPs
```

Pros:

- very fast
- avoids userspace accept/close entirely
- good for high packet rates

Cons:

- more complex deployment
- requires privileges
- harder to debug

### Option E: provider or orchestrator controls

In cloud/Kubernetes environments, temporary blocks may be pushed into:

- security groups
- firewall rules
- Cilium policies
- Calico policies
- load balancer ACLs
- CDN/WAF rules

These are usually slower to update but safer operationally.

## 5. Choose between drop and reject

For temporary blocks, `drop` and `reject` have different behavior.

### Reject / TCP reset

Pros:

- client quickly learns the connection is refused
- friendlier for legitimate clients
- less client-side waiting

Cons:

- abusive clients can reconnect rapidly
- still sends response packets

### Drop

Pros:

- uses less outbound bandwidth
- can slow reconnect loops
- often better for abusive traffic

Cons:

- clients hang until timeout
- harder to debug
- may be less friendly to legitimate users

A reasonable tiered policy:

```text
normal deny: userspace close or kernel reject
abusive deny: temporary kernel drop
```

## 6. Understand TCP keepalive vs HTTP keep-alive

These are different features.

| Feature | Layer | Purpose |
| --- | --- | --- |
| TCP keepalive | TCP/socket | probe idle connections to detect dead peers |
| HTTP keep-alive | HTTP | send multiple HTTP requests over one TCP connection |
| backend connection pooling | HTTP/client | reuse backend TCP connections for many HTTP requests |
| multiplexing | HTTP/2, HTTP/3, custom protocols | many logical streams over one connection |

The current proxy uses TCP keepalive:

```go
client.SetKeepAlive(true)
target.SetKeepAlive(true)
client.SetKeepAlivePeriod(e.keepalive)
target.SetKeepAlivePeriod(e.keepalive)
```

That helps detect dead idle peers. It does not make the proxy reuse connections, and it does not reduce `wrk` reconnect churn when denied connections are closed/reset.

For a protocol-agnostic TCP proxy, backend connection reuse is generally not possible unless the proxy understands the protocol and can safely multiplex or pool it.

## 7. Improve keepalive and idle handling for raw TCP

For generic TCP proxying, keepalive improvements are mostly about connection health and resource cleanup.

### Add idle timeouts

A client can connect and then do nothing. TCP keepalive defaults are often too slow for proxy resource management.

A useful policy:

```text
if no bytes move in either direction for N minutes:
    close both sides
```

Implementation options:

- update a `lastActivity` timestamp whenever bytes are copied
- use a timer/sweeper to close idle connections
- set deadlines on both sockets and extend them when bytes move

### Tune TCP keepalive parameters

On newer Go/Linux versions, `net.TCPConn.SetKeepAliveConfig` may expose more controls than `SetKeepAlivePeriod`.

Useful knobs:

- idle time before probes
- probe interval
- probe count

Example desired behavior:

```text
idle for 60s
probe every 10s
fail after 3 probes
```

### Consider `TCP_USER_TIMEOUT`

On Linux, `TCP_USER_TIMEOUT` limits how long transmitted data may remain unacknowledged before TCP gives up.

This can help a proxy clean up connections where a peer has become unreachable but the socket has not closed cleanly. It requires `setsockopt`, likely via `golang.org/x/sys/unix`.

### Preserve half-close behavior

Calling `CloseWrite()` after `io.Copy()` finishes is good for many TCP protocols because it supports half-closed connections.

Avoid `SetLinger(0)` for normal allowed connections unless intentionally aborting. RST-style close is more appropriate for denied or failed connections.

## 8. Add an optional HTTP-aware mode

Allower is not only for HTTP, so this should be optional. However, HTTP denied traffic can be much cheaper if the proxy returns an HTTP response instead of resetting TCP.

For HTTP entrypoints, denied clients could receive:

```http
HTTP/1.1 403 Forbidden
Content-Length: 0
Connection: keep-alive
```

This lets clients such as `wrk` reuse their existing connections instead of reconnecting continuously.

Possible config direction:

```yaml
entrypoints:
  - name: web
    protocol: http
    deny_response: 403

  - name: raw-tcp
    protocol: tcp
    deny_action: reset
```

Tradeoffs:

- the proxy becomes protocol-aware for that entrypoint
- HTTPS passthrough cannot return HTTP `403` unless TLS is terminated
- non-HTTP workloads still need raw TCP behavior

## 9. Avoid goroutine creation for immediately denied connections

Current behavior accepts a TCP connection and starts a goroutine before the deny decision:

```go
conn, err := e.listener.AcceptTCP()
...
go e.handleConn(conn, traceSeq, start)
```

Under denied connection storms, this creates scheduler pressure.

Potential flow:

```text
AcceptTCP
    -> get remote IP
    -> check allowers
    -> denied? close or enqueue to deny workers
    -> allowed? start proxy handler
```

If closing inline stalls the accept loop, use a bounded deny worker pool.

```text
accept loop:
    denied connection -> bounded deny queue

deny workers:
    SetLinger(0)
    Close()
```

This caps CPU/goroutine work spent on denied traffic.

## 10. Add backpressure and load shedding

Under overload, denied traffic should not consume all resources.

Possible controls:

- global max active connections
- per-IP active connection limit
- per-IP connection rate limit
- bounded denied-close workers
- backend dial concurrency limit
- max pending accepted connections

Examples:

```text
if active connections > max:
    close new connection immediately

if IP has more than N active conns:
    deny or temporarily block

if IP connects more than N times/sec:
    temporarily block
```

A backend dial limit is also useful so a burst of allowed clients cannot create an uncontrolled dial storm against the target service.

## 11. Improve allowed TCP proxying

The long-lived `iperf3` path is already a good fit for a raw TCP proxy. Still, several small improvements may help.

### Use one fewer goroutine per proxied connection

Current shape:

```text
handler goroutine
    -> copy goroutine A
    -> copy goroutine B
    -> wait
```

Alternative:

```text
handler goroutine
    -> copy direction A directly
    -> copy goroutine B
```

This saves one goroutine per allowed connection.

### Reuse `net.Dialer`

Instead of allocating a new dialer per connection:

```go
target, err := new(net.Dialer).DialContext(ctx, "tcp", e.target)
```

Store one on `Entrypoint`:

```go
type Entrypoint struct {
    dialer net.Dialer
}
```

Then use:

```go
target, err := e.dialer.DialContext(ctx, "tcp", e.target)
```

### Be careful with manual buffers

`io.Copy` can use optimized paths for TCP connections on some platforms. Do not replace it with `io.CopyBuffer` and a pool unless benchmarks show an improvement.

If manual buffers are tested:

```go
buf := bufferPool.Get().([]byte)
defer bufferPool.Put(buf)
_, err := io.CopyBuffer(dst, src, buf)
```

Validate that it does not reduce throughput by disabling optimized paths.

### Tune socket buffers only with evidence

For high-throughput or high-latency links, socket buffer sizes may matter:

```go
conn.SetReadBuffer(...)
conn.SetWriteBuffer(...)
```

Larger buffers can improve throughput in some cases, but they increase memory per connection.

## 12. Listener and accept scaling

A single accept loop can become a bottleneck under high connection churn.

Options:

### Multiple accept goroutines

Multiple goroutines can call `AcceptTCP()` on the same listener. This may help, but benefits vary.

### `SO_REUSEPORT`

Use multiple listeners bound to the same address and let the kernel distribute connections.

```text
N listeners with SO_REUSEPORT
N accept loops
kernel distributes incoming connections
```

In Go this requires `net.ListenConfig.Control`.

Benefits:

- better accept distribution
- lower accept lock contention in some cases
- natural sharding of per-listener state

Downsides:

- more complexity
- can increase CPU burn during denial storms
- not a substitute for pre-userspace blocking

## 13. Keep logging off the hot path

Avoid per-connection log work when the event will not be emitted.

Current pattern to avoid:

```go
log := e.log.With().
    Str("remote_ip", ip.String()).
    Uint64("trace", traceSeq).
    Logger()
```

This does work for every connection even when debug logs are disabled.

Prefer event-local fields:

```go
e.log.Debug().
    Stringer("remote_ip", ip).
    Uint64("trace", traceSeq).
    Msg("connection denied")
```

Or guard expensive fields:

```go
if zerolog.GlobalLevel() <= zerolog.DebugLevel {
    e.log.Debug().
        Str("remote_ip", ip.String()).
        Uint64("trace", traceSeq).
        Msg("connection denied")
}
```

For denied traffic, consider sampled logs:

```text
log first denial for an IP
then every 1000th denial
```

Never log every denied connection during a flood.

## 14. Reduce metrics contention

Current metrics use global atomics. That is fine at moderate rates, but shared atomics can show up at very high connection rates.

Consider:

- per-entrypoint metrics
- sharded counters
- per-worker counters
- periodic aggregation

Also, `MetricsEnabled` is currently a plain bool. If it can change while traffic is running, use `atomic.Bool` to avoid a data race.

## 15. Plan UDP separately

UDP support will need a different design.

There is no accept/close. The proxy will handle packets and pseudo-sessions.

Things to consider:

### Per-client flow table

```text
(client IP, client port) -> backend UDP state
```

Expire flows after inactivity:

```text
if no packet for 30s:
    delete flow
```

### Denied UDP is cheaper but still not free

Denied UDP packets can simply be dropped in userspace, but high-rate floods still burn kernel and userspace packet-processing CPU unless dropped earlier.

### Be careful with automatic UDP IP blocking

UDP source addresses can be spoofed. Do not blindly escalate UDP denies into long kernel-level IP blocks unless the deployment environment makes spoofing unlikely.

### XDP/eBPF can be very valuable for UDP

For high-rate UDP drops, XDP/eBPF can avoid delivering packets to the socket at all.

## 16. Inspect host and kernel behavior

For VM benchmarking, kernel/network behavior is as important as Go code.

Useful commands:

```bash
ss -s
nstat -az
sar -n TCP,ETCP 1
mpstat -P ALL 1
pidstat -w -t -p <pid> 1
```

Look for:

- listen drops
- listen overflows
- TCP resets
- retransmits
- syncookies
- softirq CPU
- `TIME_WAIT` counts
- orphan sockets

Useful kernel settings to understand, not blindly change:

```text
net.core.somaxconn
net.ipv4.tcp_max_syn_backlog
net.ipv4.tcp_syncookies
net.ipv4.ip_local_port_range
net.ipv4.tcp_tw_reuse
net.core.netdev_max_backlog
net.core.rmem_max
net.core.wmem_max
```

Also inspect NIC queueing/RSS:

```bash
ethtool -l <iface>
ethtool -x <iface>
```

If all RX processing lands on one CPU, Go-level optimizations may not help much.

## 17. Design benchmarks around the question

Use different tests for different bottlenecks.

### Long-lived throughput

```bash
iperf3
```

Measures streaming proxy throughput.

### Connection churn

Use tools that report connections/sec or requests/sec under short connection lifetimes.

### Mixed workload

Run allowed and denied traffic at the same time:

```text
allowed iperf3
+
denied connection storm
```

This answers the most important operational question:

> Can denied traffic avoid hurting allowed traffic?

Temporary kernel blocks, bounded deny workers, and backpressure should be judged by how well they preserve allowed throughput under denied load.

## 18. Suggested roadmap

### Phase 1: visibility

Add metrics for:

- accepted/sec
- denied/sec
- allowed/sec
- active connections
- backend dial latency
- bytes copied
- top denied IPs or sampled denied IPs

### Phase 2: hot-path cleanup

- avoid per-connection logger construction
- reuse `net.Dialer`
- avoid goroutine creation for obviously denied connections
- add bounded deny workers
- add a sharded per-IP deny tracker

### Phase 3: overload protection

- max active connections
- per-IP connection rate limit
- per-IP active connection cap
- backend dial concurrency cap
- idle timeout

### Phase 4: temporary kernel blocking

Pick one approach:

- direct nftables netlink from Go with `CAP_NET_ADMIN`
- unprivileged Allower plus privileged firewall sidecar
- eBPF/XDP for a deeper, high-performance path

Given a `scratch` image, the most practical options are:

```text
Go nftables netlink library + CAP_NET_ADMIN
```

or:

```text
unprivileged Allower + privileged firewall sidecar
```

The sidecar is usually cleaner from a security and operations perspective.

### Phase 5: protocol-specific modes

Consider separate behavior by entrypoint type:

- raw TCP mode
- HTTP mode with `403` keep-alive denial
- future UDP mode with flow table and expiry

## Big takeaway

For generic TCP/UDP proxying, TCP keepalive mostly helps detect dead idle connections. It does not provide Traefik-style request reuse unless the proxy becomes protocol-aware.

For denied IP traffic, the largest performance improvement is not making the Go rule check faster. It is preventing abusive IPs from reaching the userspace accept path at all.

A deny tracker plus temporary kernel-level blocks is a strong direction, especially if combined with bounded deny workers and overload protection so denied traffic cannot starve allowed traffic.
