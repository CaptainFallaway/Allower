# Allower proxy performance discussion context

This document summarizes a performance investigation around Allower's TCP proxy. It is intended to be self-contained context for further discussion with other LLMs or engineers.

## Project context

Allower is a Go project containing a protocol-agnostic TCP proxy with rule-based IP filtering.

Relevant package:

```text
internal/proxy/
  entrypoint.go
  handleconn.go
  metrics.go
```

The proxy accepts TCP connections, checks the remote IP against configured allowers/rules, then either denies the connection or dials a target and bidirectionally copies bytes between the client and target.

Rules are not believed to be the bottleneck. The IP-based allow/block checks benchmark extremely fast, reportedly with `0 allocs/op` and only a few nanoseconds per call.

## Original observed behavior

A VM benchmark with long-lived `iperf3` traffic through the proxy achieved approximately:

```text
~10 Gbit/s throughput, near QSFP link speed
~45% CPU usage
```

After adding another peer, denying that peer via rules, and running a workload similar to:

```bash
wrk -t20 -c5000 -d30s http://<ip>
```

while also proxying `iperf3`, performance dropped dramatically:

```text
~500-600 Mbit/s iperf throughput
~400% CPU usage, later experiments saw 500-600% CPU in allowed-wrk cases
```

A 30-second CPU profile on a local machine with mostly blocked `wrk` requests showed most CPU time spent in TCP accept/close related paths. Logging, rules, and other logic were relatively small contributors.

## Important distinction discovered

The benchmark changed from a byte-throughput workload to a connection-churn workload.

### Long-lived allowed stream workload

Example:

```text
iperf3 through proxy
```

Characteristics:

```text
few long-lived TCP streams
large continuous byte streams
low connection setup rate
high bytes per syscall/wakeup
```

### Denied wrk workload

Example:

```text
wrk with many concurrent connections from a blocked peer
```

Characteristics:

```text
high connection churn
many accepts
many closes/resets
kernel TCP/socket bookkeeping
many goroutine wakeups
many short-lived sockets
```

The rule decision happens after the kernel and Go runtime have already accepted the TCP connection, so even a nanosecond-level rule check does not remove the cost of accepting and closing TCP sockets.

## Relevant code shape before/around investigation

The accept loop was roughly:

```go
func (e *Entrypoint) Accept() {
    for {
        conn, err := e.listener.AcceptTCP()
        start := time.Now()
        if errors.Is(err, net.ErrClosed) {
            return
        } else if err != nil {
            log.Error().Err(err).Msg("failed to accept connection")
            continue
        }
        go e.handleConn(conn, start)
    }
}
```

The connection handler checks the remote IP, denies if any allower rejects, or dials the target:

```go
func (e *Entrypoint) handleConn(client *net.TCPConn, start time.Time) {
    ip := getIp(client.RemoteAddr().(*net.TCPAddr))

    log := e.log.With().Str("remote_ip", ip.String()).Logger()

    for _, a := range e.allowers {
        if !a.IsAllowed(ip) {
            Metrics.Record(time.Since(start), true)
            log.Debug().Msg("connection denied")
            e.closeChan <- client
            return
        }
    }

    Metrics.Record(time.Since(start), false)

    ctx, cancel := context.WithTimeout(e.ctx, e.dialTimeout)
    target, err := e.dialer.DialContext(ctx, "tcp", e.target)
    if err != nil {
        log.Error().Err(err).Msg("failed to dial target")
        client.Close()
        cancel()
        return
    }
    cancel()

    targetTCP := target.(*net.TCPConn)
    e.setKeepalive(log, client, targetTCP)
    e.bidirectionalCopy(log, targetTCP, client)
}
```

Bidirectional copy is roughly:

```go
func (e *Entrypoint) bidirectionalCopy(log zerolog.Logger, target, client *net.TCPConn) {
    var wg sync.WaitGroup
    var once sync.Once

    closeBoth := func() {
        once.Do(func() {
            _ = client.Close()
            _ = target.Close()
        })
    }

    copyOneWay := func(src, dst *net.TCPConn, direction string) {
        defer wg.Done()

        if _, err := io.Copy(dst, src); err != nil {
            if !errors.Is(err, net.ErrClosed) {
                log.Warn().Err(err).Str("direction", direction).Msg("proxy copy failed")
            }
            closeBoth()
            return
        }

        if err := dst.CloseWrite(); err != nil && !errors.Is(err, net.ErrClosed) {
            log.Warn().Err(err).Str("direction", direction).Msg("failed to half-close proxy connection")
            closeBoth()
        }
    }

    wg.Add(2)
    go copyOneWay(client, target, "client -> target")
    go copyOneWay(target, client, "target -> client")
    wg.Wait()
    closeBoth()

    log.Debug().Msg("connection closed")
}
```

## Major experiment results

### 1. Moving rule checks into the accept loop was bad

An attempted optimization moved the remote IP extraction and deny check into the accept loop to avoid spawning a goroutine for denied connections.

Result:

```text
performed horribly
sometimes appeared to allow some connections through, possibly due to a logic bug or unexpected control flow
```

Interpretation:

The accept loop is very sensitive. Putting more work into it can serialize too much connection handling and reduce accept throughput. Keeping the accept loop simple and dispatching to handler goroutines performed better in this workload.

Current preferred shape:

```text
accept loop:
    accept and dispatch quickly

handler goroutine:
    classify allowed/denied

denied close worker:
    bounded expensive close/reset work

allowed copy goroutines:
    proxy bytes
```

### 2. Logging behavior was surprising

A suggestion was to avoid string conversion and use zerolog `.Stringer(...)` or event-local fields.

Actual result:

```text
using .Stringer performed much worse
.Stringer took a much larger chunk of CPU than Str("remote_ip", ip.String())
```

Interpretation:

`netip.Addr.String()` is cheap enough, and the `.Stringer` path may introduce interface dispatch, escaping, or less compiler-friendly behavior. In this specific profile, immediate string conversion with `.Str(...)` was faster.

### 3. Trace + pretty logging seemed to improve iperf during blocked wrk

Before other fixes, enabling trace and pretty logging unexpectedly increased `iperf3` bandwidth while denied `wrk` traffic was running:

```text
structured info logging + denied wrk: ~1-2 Gbit/s-ish in one case
trace + pretty logging + denied wrk: ~6-7 Gbit/s in one case
```

This was surprising because logging usually makes things slower.

Working interpretation:

Trace/pretty logging likely throttled denied connection handling by spending/blocking more time in logging/stdout. That accidentally reduced the rate at which many goroutines hammered `Close()`/RST paths, preserving more CPU for established `io.Copy` streams.

After adding the explicit close worker/channel, this accidental logging-related improvement disappeared, which supports the idea that logging was acting as an accidental backpressure mechanism.

### 4. Removing one copy goroutine did not matter

A proposed optimization was to spawn only one copy goroutine and run the opposite direction in the existing handler goroutine, reducing allowed connections from three goroutines to two.

Result:

```text
no meaningful improvement
in some profiles the original two-copy-goroutine version looked as good or better
```

Interpretation:

For long-lived high-throughput streams, one extra goroutine per allowed connection is not the main cost. The dominant costs are kernel networking, socket forwarding, and `io.Copy`/`splice` paths.

### 5. Huge improvement: bounded denied-close worker/channel

The most important improvement was adding a worker goroutine and buffered channel for denied connection closing:

```go
c := make(chan *net.TCPConn, 1024)
e.closeChan = c

go func() {
    for {
        select {
        case conn := <-c:
            conn.SetLinger(0)
            conn.Close()
        case <-ctx.Done():
            return
        }
    }
}()
```

Deny path changed to:

```go
e.closeChan <- client
return
```

Result:

```text
CPU time in closing dropped from dominating ~60-70% of flamegraph
to a much more even accept/close distribution
iperf3 under denied wrk improved from ~600 Mbit/s to almost 10 Gbit/s
CPU around ~250% in this high-bandwidth denied case
```

Interpretation:

The improvement likely came from bounding and isolating close/reset work, not from making `Close()` itself cheap.

The close channel/worker:

```text
serializes close/reset work
smooths bursts
creates backpressure when the close queue fills
prevents unbounded denied goroutines from all entering Close() concurrently
preserves CPU for established allowed copy loops
```

This optimizes the important operational goal:

```text
preserve allowed throughput under denied traffic
```

not necessarily:

```text
maximize denied connections/second
```

### 6. Close queue high-watermark was monitored

A test goroutine sampled `len(closeChan)` every 50 microseconds, storing a max in an atomic. Another goroutine logged the max every 30 seconds.

Result:

```text
with channel capacity 1024, observed max did not exceed ~1000
```

Interpretation:

The close queue was near saturation but not obviously growing without bound. However, sampling `len(c)` can miss short full periods and does not show how many goroutines are blocked trying to send.

Better future metrics:

```text
close queue current length
close queue high watermark on enqueue
close queue full events via non-blocking send/select default
close enqueue block latency
close worker completed closes/sec
open FD count
goroutine count
```

A better saturation check:

```go
select {
case e.closeChan <- client:
    observeMax(len(e.closeChan))
default:
    closeQueueFull.Add(1)
    start := time.Now()
    e.closeChan <- client
    closeQueueBlocked.Add(1)
    closeQueueBlockNanos.Add(uint64(time.Since(start)))
}
```

## Allowed wrk workload result

After stopping denial and allowing the `wrk` peer through to an nginx demo site, the mixed workload again reduced `iperf3` throughput substantially:

```text
wrk allowed to nginx demo site
wrk used ~1500 connections, not 5000, due to FD limits on the wrk peer
wrk transferred about 1.89 GB over 30 seconds
iperf3 through proxy dropped to ~600 Mbit/s
CPU around ~500-600%
profile was ~90% io.Copy / TCP copy path
```

The `wrk` transfer amount corresponds roughly to:

```text
1.89 GB / 30s ~= 63 MB/s ~= 504 Mbit/s
```

If GiB, roughly:

```text
~541 Mbit/s
```

So total proxied payload was approximately:

```text
~0.5 Gbit/s wrk HTTP traffic
+ ~0.6 Gbit/s iperf traffic
= ~1.1 Gbit/s total payload
```

Yet CPU was very high. This suggests the bottleneck was not raw bandwidth but connection/event/syscall pressure.

### Why 1500 allowed HTTP connections hurt

With the current raw TCP proxy, 1500 frontend HTTP connections imply roughly:

```text
1500 client TCP sockets
1500 backend target TCP sockets
3000 TCP sockets
3000 copy directions/goroutines
many small HTTP requests/responses
many netpoll wakeups
many small socket events
```

Even if payload bandwidth is only ~500 Mbit/s, CPU can be high because the workload has low bytes per event/syscall.

This is very different from `iperf3`:

```text
few sockets
large continuous streams
high bytes per syscall/wakeup
```

## io.Copy fast path confirmation

The CPU profile for allowed `wrk` + `iperf3` showed paths like:

```text
io.Copy
net.(*TCPConn).ReadFrom
internal/poll.splice...
syscall.Syscall
```

This confirms Go was using the optimized TCP-to-TCP `io.Copy` fast path on Linux, likely via `splice`.

Conceptual behavior:

```text
socket -> kernel pipe buffer -> socket
```

rather than:

```text
socket -> userspace Go []byte -> socket
```

Implications:

```text
manual read/write buffers are unlikely to improve this path
sync.Pool buffers are unlikely to help
wrapping *net.TCPConn can accidentally hide ReadFrom/WriteTo and disable fast paths
io.Copy is probably already the right primitive for raw TCP proxying
```

CPU can still be high because `splice` avoids userspace payload copying but does not eliminate:

```text
socket polling
syscall entry/exit
TCP receive/send processing
pipe buffer management
skb accounting
scheduler wakeups
per-connection kernel state
```

## Keepalive / backend pooling discussion

TCP keepalive and HTTP keep-alive are different.

Current code uses TCP keepalive:

```go
client.SetKeepAlive(true)
target.SetKeepAlive(true)
client.SetKeepAlivePeriod(e.keepalive)
target.SetKeepAlivePeriod(e.keepalive)
```

This helps detect dead idle peers. It does not reuse backend connections.

For generic raw TCP, backend connection pooling is generally unsafe. TCP is an ordered byte stream with no built-in request boundaries or multiplexing. The safe mapping is usually:

```text
client connection A <-> target connection A
client connection B <-> target connection B
client connection C <-> target connection C
```

Not:

```text
client A \
client B  -> one shared target TCP connection
client C /
```

Sharing one target connection across arbitrary clients would interleave bytes and corrupt protocols unless the proxy understands the protocol and the protocol supports multiplexing/pooling.

For HTTP, connection pooling is possible because HTTP has request/response framing. An HTTP-aware proxy could use `http.Transport` to pool backend connections:

```text
many client HTTP connections
    -> controlled pool of backend HTTP connections
```

This could help the nginx/wrk workload by reducing backend socket count, backend dials, and target-side event pressure. However, it requires an optional HTTP-aware mode and would no longer be pure raw TCP proxying for that entrypoint.

## Traefik comparison discussed

Traefik is fast, but it generally operates at HTTP/ingress level and relies on:

```text
HTTP keep-alive
backend connection pooling
HTTP/2 multiplexing where applicable
timeouts and limits
external infrastructure for hostile traffic filtering
```

Traefik's HTTP deny behavior often returns an HTTP response such as `403`, potentially over a keep-alive connection. That is different from a raw TCP proxy accepting then immediately resetting/closing a denied connection.

For serious unwanted IP traffic, production setups often block before the application proxy:

```text
nftables/iptables/ipset
cloud firewall/security groups
Kubernetes NetworkPolicy
CDN/WAF
Cilium/Calico/eBPF/XDP
```

## Scratch image and kernel-level blocking

Running in a `scratch` image does not prevent kernel-level blocking if the Go binary talks directly to the kernel.

Potential approaches:

### Direct nftables netlink from Go

Use a library such as:

```text
github.com/google/nftables
```

Requirements:

```text
process in correct network namespace
CAP_NET_ADMIN or equivalent privileges
host kernel nftables support
rules/table/set created by app or pre-created
```

### Privileged firewall helper/sidecar

Keep Allower unprivileged and run a privileged helper to mutate nftables/eBPF/ipset state.

```text
Allower -> request temporary block
helper  -> update kernel firewall state
```

This is cleaner operationally because the proxy itself does not need broad network-admin privileges.

### eBPF/XDP

Advanced option for very high-rate drops:

```text
Allower updates BPF map
XDP/tc/cgroup eBPF program drops or rejects traffic before userspace accept
```

Very fast, but more complex to deploy/debug and requires privileges.

## Drop vs reject discussion

For temporary kernel-level blocks:

### Reject / TCP reset

Pros:

```text
client quickly knows connection is refused
friendlier for legitimate clients
less client-side waiting
```

Cons:

```text
abusive clients can reconnect rapidly
still sends response packets
```

### Drop

Pros:

```text
less outbound bandwidth
can slow reconnect loops
better for abusive traffic
```

Cons:

```text
clients hang until timeout
harder to debug
less friendly to legitimate clients
```

Possible tiered policy:

```text
normal deny: userspace close or kernel reject
abusive repeated deny: temporary kernel drop
```

## Suggested future improvements

### 1. Keep the denied close worker/channel design

The benchmark strongly supports this. Make it production-safe and observable.

Possible cleanup:

```go
select {
case e.closeChan <- client:
case <-e.ctx.Done():
    _ = client.SetLinger(0)
    _ = client.Close()
}
```

Reason: avoid blocking forever on send if the close worker exits on context cancellation.

### 2. Improve close worker metrics

Track:

```text
close queue length
close queue max
close queue full count
close enqueue blocked count
close enqueue block duration
close completed count/sec
```

### 3. Make close worker count and queue size configurable

Test combinations:

```text
queue size: 256, 1024, 4096
workers:    1, 2, 4, GOMAXPROCS/2
```

Measure:

```text
allowed iperf throughput under denied wrk
CPU usage
close queue length
FD count
goroutine count
denied connections/sec
```

The best setting may be a small worker count plus enough buffer to smooth bursts.

### 4. Add fairness / limits for allowed workloads

The allowed `wrk` case suggests one peer with many allowed connections can consume large amounts of CPU despite modest bandwidth.

Useful controls:

```text
max active connections per entrypoint
max active connections per source IP
per-IP connection rate limit
backend dial concurrency limit
backend active connection limit
idle timeout
```

Example config idea:

```yaml
entrypoints:
  - name: web
    max_active_connections: 2000
    max_connections_per_ip: 500
    max_backend_dials: 100
```

### 5. Consider optional HTTP-aware mode

For HTTP workloads, raw TCP proxying forces one backend TCP connection per frontend TCP connection. An HTTP-aware mode could unlock:

```text
backend connection pooling
MaxConnsPerHost
MaxIdleConns
IdleConnTimeout
HTTP 403 denial over keep-alive
request-level limits
possibly HTTP/2 support
```

This would be separate from raw TCP mode.

Potential config idea:

```yaml
entrypoints:
  - name: raw
    protocol: tcp

  - name: web
    protocol: http
    deny_response: 403
    backend_pooling: true
```

### 6. Add idle/dead connection handling

For generic TCP:

```text
TCP keepalive detects dead idle peers eventually
idle deadlines clean up application-level idle connections sooner
TCP_USER_TIMEOUT can limit unacknowledged data lifetime on Linux
```

Potential policies:

```text
close if no bytes move in either direction for N minutes
configure TCP keepalive idle/interval/count where supported
consider TCP_USER_TIMEOUT via x/sys/unix
```

### 7. Avoid unnecessary hot-path logging changes unless profiled

Empirical result:

```text
Str("remote_ip", ip.String()) was faster than Stringer in this workload
```

Trust profiles. Do not assume deferred/stringer logging is faster.

### 8. Keep `io.Copy` for raw TCP

The profile confirmed the fast path:

```text
net.(*TCPConn).ReadFrom -> internal/poll.splice -> syscall
```

Avoid manual buffer loops unless a benchmark proves a win. Be careful with connection wrappers that hide `ReadFrom`/`WriteTo`.

### 9. Benchmark lower wrk connection counts and larger responses

To isolate whether the issue is connection/event fanout or bytes/sec, test:

```bash
wrk -t20 -c100  -d30s http://...
wrk -t20 -c250  -d30s http://...
wrk -t20 -c500  -d30s http://...
wrk -t20 -c1000 -d30s http://...
wrk -t20 -c1500 -d30s http://...
```

Record:

```text
wrk requests/sec
wrk transfer/sec
iperf throughput
Allower CPU
goroutine count
active client/target conns
```

Also test larger nginx responses to increase bytes per event:

```bash
# example inside nginx container/image
# create a larger static file and wrk that URL
wrk -t20 -c1500 -d30s http://<ip>/10mb.bin
```

If larger responses greatly improve throughput per CPU, the bottleneck is small-message event overhead.

### 10. Inspect host/kernel behavior

Since profiles show kernel-assisted socket forwarding, OS/NIC tuning matters.

Useful commands:

```bash
ss -s
nstat -az
sar -n TCP,ETCP 1
mpstat -P ALL 1
pidstat -w -t -p <pid> 1
sudo perf top -p <pid>
ethtool -k <iface>
ethtool -l <iface>
ethtool -x <iface>
```

Look for:

```text
listen drops/overflows
TCP retransmits/resets
syncookies
softirq CPU saturation
uneven IRQ/RSS distribution
TIME_WAIT/orphan socket pressure
```

## Current high-level conclusions

1. Rules are not the bottleneck.
2. Denied connection storms were dominated by accept/close/reset and kernel socket cleanup.
3. A bounded denied-close worker/channel produced the biggest improvement by isolating and throttling close/reset work.
4. Logging changes can have counterintuitive effects because they may accidentally throttle hot paths.
5. The raw TCP copy path is already hitting Go/Linux's optimized `io.Copy` fast path via `ReadFrom`/`splice`.
6. Allowed high-concurrency HTTP traffic is expensive despite modest bandwidth because it creates many socket events, syscalls, and copy directions.
7. For generic TCP, backend connection pooling is not safe unless the proxy becomes protocol-aware.
8. Big future wins are likely from fairness/limits, optional HTTP mode, kernel-level temporary blocking for repeat offenders, and OS/NIC tuning rather than micro-optimizing `io.Copy`.

## Useful concise prompt for another LLM

```text
I am building Allower, a Go raw TCP proxy with IP allow/block rules. Rules are extremely fast and not the bottleneck. The proxy accepts TCP connections, checks remote IP, denies or dials a target, then uses io.Copy between *net.TCPConn pairs. Profiles confirm io.Copy uses net.(*TCPConn).ReadFrom and Linux splice fast paths.

Initial long-lived iperf3 through the proxy hit near 10 Gbit/s at low CPU. When denied wrk traffic ran concurrently, iperf dropped to ~600 Mbit/s and profiles showed accept/close dominating. Moving checks into the accept loop was worse. The biggest improvement was adding a bounded denied-close channel/worker: denied handlers enqueue *net.TCPConn, one worker does SetLinger(0)+Close. That restored iperf under denied wrk to near 10 Gbit/s by isolating/throttling close/reset work. Queue cap was 1024 and sampled max was ~1000.

When the wrk peer was allowed to reach nginx, with ~1500 wrk connections and ~1.89 GB transferred over 30s (~0.5 Gbit/s), iperf still dropped to ~600 Mbit/s and CPU rose to ~500-600%. Profile was ~90% io.Copy / net.(*TCPConn).ReadFrom / internal/poll.splice / syscall. This suggests not rule/logging overhead but high connection/event/syscall pressure from many small HTTP streams.

I want advice on architecture and next optimizations: close worker safety/metrics, fairness limits, per-IP connection caps, backend dial caps, optional HTTP-aware mode with backend pooling, kernel-level temporary blocks from a scratch container via nftables netlink or helper sidecar, TCP keepalive/idle timeout/TCP_USER_TIMEOUT, and host/NIC tuning. Avoid suggesting manual buffer loops unless justified, because io.Copy fast path is active.
```
