package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type env struct {
	ipinfoToken     string
	ipinfoDir       string
	ipinfoSync      time.Duration
	configPath      string
	metricsInterval time.Duration
	logPretty       bool
	logLevel        string
}

func getDuration(ek string) (time.Duration, error) {
	durationStr, found := os.LookupEnv(ek)
	if !found {
		return 0, fmt.Errorf("%s environment variable not set", ek)
	}
	return time.ParseDuration(durationStr)
}

func getBool(ek string) (bool, error) {
	boolStr, found := os.LookupEnv(ek)
	if !found {
		return false, fmt.Errorf("%s environment variable not set", ek)
	}
	return strconv.ParseBool(boolStr)
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

	metricsInterval, err := getDuration("METRICS")
	if err != nil {
		return nil, err
	}

	logPretty, err := getBool("LOG_PRETTY")
	if err != nil {
		return nil, err
	}

	logLevel, found := os.LookupEnv("LOG_LEVEL")
	if !found {
		return nil, fmt.Errorf("LOG_LEVEL environment variable not set")
	}

	return &env{
		ipinfoToken:     ipinfoToken,
		ipinfoDir:       ipinfoDir,
		ipinfoSync:      ipinfoSync,
		configPath:      configPath,
		metricsInterval: metricsInterval,
		logPretty:       logPretty,
		logLevel:        logLevel,
	}, nil
}
