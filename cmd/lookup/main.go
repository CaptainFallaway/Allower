package main

import (
	"context"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"time"

	"github.com/CaptainFallaway/Reverser/pkg/ipinfo"
)

func main() {
	doSync := flag.Bool("s", false, "sync dataset before lookup")
	flag.Parse()

	if err := run(*doSync); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func run(doSync bool) error {
	start := time.Now()

	token, dir, err := loadEnv()
	if err != nil {
		return err
	}

	ips, err := getValidIPsFromArgs(doSync)
	if err != nil {
		return err
	}

	db := ipinfo.New(token, dir)

	if doSync {
		updated, err := db.Sync(context.Background())
		if err != nil {
			return fmt.Errorf("failed to sync dataset: %v", err)
		}
		if updated {
			fmt.Println("Dataset was updated during sync.")
		} else {
			fmt.Println("Dataset is already up to date.")
		}
	}

	err = db.Load()
	if err != nil {
		return fmt.Errorf("failed to load dataset: %v", err)
	}

	lookupStart := time.Now()

	count := 0
	for _, ip := range ips {
		record, err := db.Lookup(ip)
		if err != nil {
			fmt.Printf("\nFailed to lookup IP %s: %v\n", ip, err)
			continue
		}
		printRecord(ip, record)
		count++
	}

	fmt.Printf("\nLookups completed: %d\n", count)
	fmt.Printf("Total execution time: %s\n", time.Since(start))
	fmt.Printf("Lookup time: %s\n", time.Since(lookupStart))

	return nil
}

func loadEnv() (string, string, error) {
	token, found := os.LookupEnv("IPINFO_TOKEN")
	if !found {
		return "", "", fmt.Errorf("IPINFO_TOKEN environment variable not set")
	}

	dir, found := os.LookupEnv("IPINFO_DIR")
	if !found {
		return "", "", fmt.Errorf("IPINFO_URL environment variable not set")
	}

	return token, dir, nil
}

func getValidIPsFromArgs(offsetArgs bool) ([]netip.Addr, error) {
	off := 0
	if offsetArgs {
		off = 1
	}

	if len(os.Args) < 2+off {
		return nil, fmt.Errorf("no IP addresses provided")
	}

	ips := make([]netip.Addr, 0, len(os.Args)-1+off)

	for _, arg := range os.Args[1+off:] {
		ip, err := netip.ParseAddr(arg)
		if err != nil {
			fmt.Printf("could not parse arg '%s': %v\n", arg, err)
			continue
		}
		ips = append(ips, ip)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no valid IP addresses provided")
	}

	return ips, nil
}

func printRecord(addr netip.Addr, record *ipinfo.Record) {
	fmt.Printf("\nIP: %s\n", addr)
	fmt.Printf("  Country: %s\n", record.Country)
	fmt.Printf("  Country Code: %s\n", record.CountryCode)
	fmt.Printf("  Continent: %s\n", record.Continent)
	fmt.Printf("  Continent Code: %s\n", record.ContinentCode)
	fmt.Printf("  AS Number: %s\n", record.AsNumber)
	fmt.Printf("  AS Name: %s\n", record.ASName)
	fmt.Printf("  AS Domain: %s\n", record.ASDomain)
}
