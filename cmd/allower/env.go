package main

import (
	"fmt"
	"os"
	"time"
)

type env struct {
	ipinfoToken string
	ipinfoDir   string
	ipinfoSync  time.Duration
	configPath  string
}

func getDuration(ek string) (time.Duration, error) {
	durationStr, found := os.LookupEnv(ek)
	if !found {
		return 0, fmt.Errorf("%s environment variable not set", ek)
	}
	return time.ParseDuration(durationStr)
}

func getEnv() (*env, error) {
	ipinfoToken, found := os.LookupEnv("IPINFO_TOKEN")
	if !found {
		return nil, fmt.Errorf("IPINFO_TOKEN environment variable not set")
	}

	ipinfoDir, found := os.LookupEnv("IPINFO_DIR")
	if !found {
		return nil, fmt.Errorf("IPINFO_DIR environment variable not set")
	}

	ipinfoSync, err := getDuration("IPINFO_SYNC")
	if err != nil {
		return nil, err
	}

	configPath, found := os.LookupEnv("CONFIG_PATH")
	if !found {
		return nil, fmt.Errorf("CONFIG_PATH environment variable not set")
	}

	return &env{
		ipinfoToken: ipinfoToken,
		ipinfoDir:   ipinfoDir,
		ipinfoSync:  ipinfoSync,
		configPath:  configPath,
	}, nil
}
