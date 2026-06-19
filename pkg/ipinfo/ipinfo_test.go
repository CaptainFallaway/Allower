package ipinfo_test

import (
	"Reverser/pkg/ipinfo"
	"context"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"os"
	"testing"
	"time"
)

var db *ipinfo.DB

type Fataler interface {
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func LoadAndGetDB(t Fataler) *ipinfo.DB {
	if db != nil {
		return db
	}

	db := ipinfo.New(os.Getenv("IPINFO_TOKEN"), "../../test-data")

	updated, err := db.Sync(context.Background())
	if err != nil {
		t.Fatalf("failed to sync dataset: %v", err)
	}
	if updated {
		fmt.Println("dataset was update")
	}

	err = db.Load()
	if err != nil {
		t.Fatalf("failed to load dataset: %v", err)
	}

	return db
}

func BenchmarkLookup(b *testing.B) {
	db := LoadAndGetDB(b)
	ips := generateRandomPublicIPs(100_000, 0.5, uint64(time.Now().Unix()))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ip := ips[rand.IntN(len(ips))]
		_, err := db.Lookup(ip)
		if err != nil {
			b.Fatalf("failed to lookup IP address: %v", err)
		}
	}
}

// Generates a random public IPv4 address (avoids private/reserved ranges)
func randomPublicIPv4(rng *rand.Rand) netip.Addr {
	for {
		b := [4]byte{
			byte(rng.IntN(256)),
			byte(rng.IntN(256)),
			byte(rng.IntN(256)),
			byte(rng.IntN(256)),
		}
		addr := netip.AddrFrom4(b)
		if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
			addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() ||
			b[0] == 0 || // 0.0.0.0/8
			b[0] == 100 && b[1] >= 64 && b[1] <= 127 || // 100.64.0.0/10
			b[0] == 169 && b[1] == 254 || // 169.254.0.0/16
			b[0] == 192 && b[1] == 0 && b[2] == 0 || // 192.0.0.0/24
			b[0] == 192 && b[1] == 0 && b[2] == 2 || // 192.0.2.0/24
			b[0] == 198 && b[1] == 51 && b[2] == 100 || // 198.51.100.0/24
			b[0] == 203 && b[1] == 0 && b[2] == 113 || // 203.0.113.0/24
			b[0] >= 224 || // 224.0.0.0/4
			b[0] == 198 && b[1] >= 18 && b[1] <= 19 { // 198.18.0.0/15
			continue
		}
		return addr
	}
}

// Generates a random public IPv6 address (global unicast 2000::/3)
func randomPublicIPv6(rng *rand.Rand) netip.Addr {
	for {
		var b [16]byte
		b[0] = byte(0x20 + rng.IntN(0x20)) // 2000::/3
		for i := 1; i < 16; i++ {
			b[i] = byte(rng.IntN(256))
		}
		addr := netip.AddrFrom16(b)
		if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
			addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
			continue
		}
		return addr
	}
}

// Generates a list of random public IPs (mix of v4/v6, ratio 0.0=all v6, 1.0=all v4)
func generateRandomPublicIPs(count int, ipv4Ratio float64, seed uint64) []netip.Addr {
	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
	ips := make([]netip.Addr, count)
	for i := range ips {
		if rng.Float64() < ipv4Ratio {
			ips[i] = randomPublicIPv4(rng)
		} else {
			ips[i] = randomPublicIPv6(rng)
		}
	}
	return ips
}
