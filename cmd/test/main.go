package main

import (
	"context"
	"fmt"
	"log"
	"math/bits"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/CaptainFallaway/Reverser/pkg/ipinfo"

	"github.com/dustin/go-humanize"
)

type Network struct {
	addr   netip.Addr
	subnet netip.Addr
}

func (n Network) String() string {
	cidr := 0
	for _, x := range n.subnet.AsSlice() {
		cidr += bits.OnesCount8(x)
	}
	return fmt.Sprintf("%s/%d", n.addr.String(), cidr)
}

func parseNetwork(s string) (Network, error) {
	_, net, err := net.ParseCIDR(s)
	if err != nil {
		log.Fatal(err)
	}

	addr, ok := netip.AddrFromSlice(net.IP)
	if !ok {
		return Network{}, fmt.Errorf("invalid IP address: %s", net.IP)
	}

	subnet, ok := netip.AddrFromSlice(net.Mask)
	if !ok {
		return Network{}, fmt.Errorf("invalid subnet mask: %s", net.Mask)
	}

	return Network{
		addr:   addr,
		subnet: subnet,
	}, nil
}

func main() {
	// network, err := parseNetwork("2a02:aa6:446:66::1000:0/100")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// fmt.Println(network)

	db := ipinfo.New(os.Getenv("IPINFO_TOKEN"), "./test-data")

	start := time.Now()
	changed, err := db.Sync(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Database sync completed. Changed: %v\n", changed)
	syncDone := time.Since(start)

	loadStart := time.Now()
	err = db.Load()
	if err != nil {
		log.Fatal(err)
	}
	loadDone := time.Since(loadStart)

	db.Lookup(netip.MustParseAddr("155.4.194.14"))

	lookupStart := time.Now()
	ip := netip.MustParseAddr("85.24.194.40")
	info, err := db.Lookup(ip)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Lookup for IP %s: Info: %+v\n", ip, info)
	lookupDone := time.Since(lookupStart)

	total := time.Since(start)

	fmt.Printf("Timing:\n")
	fmt.Printf("  Sync: %v\n", syncDone)
	fmt.Printf("  Load: %v\n", loadDone)
	fmt.Printf("  Lookup: %v\n", lookupDone)
	fmt.Printf("  Total: %v\n", total)

	runtime.GC()
	stats := new(runtime.MemStats)
	runtime.ReadMemStats(stats)
	printMemoryStats(stats)
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	<-c
}

func printMemoryStats(stats *runtime.MemStats) {
	fmt.Printf("Memory Usage:\n")
	fmt.Printf("  Alloc: %s bytes\n", humanize.Bytes(stats.Alloc))
	fmt.Printf("  Total Alloc: %s bytes\n", humanize.Bytes(stats.TotalAlloc))
	fmt.Printf("  Sys: %s bytes\n", humanize.Bytes(stats.Sys))
	fmt.Printf("  Num GC: %d\n", stats.NumGC)
}
