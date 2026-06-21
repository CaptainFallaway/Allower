package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CaptainFallaway/Allower/internal/config"
	"github.com/CaptainFallaway/Allower/internal/proxy"
	"github.com/CaptainFallaway/Allower/internal/rule"
	"github.com/CaptainFallaway/Allower/pkg/ipinfo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	if pretty, _ := strconv.ParseBool(os.Getenv("LOG_PRETTY")); pretty {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.DateTime})
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	}

	level := os.Getenv("LOG_LEVEL")
	switch strings.TrimSpace(strings.ToLower(level)) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func run(appCtx context.Context) error {
	env, err := getEnv()
	if err != nil {
		return err
	}

	ds := ipinfo.New(env.ipinfoToken, env.ipinfoDir, ipinfo.WithLookupRecordPool())
	go periodicallySyncDataset(appCtx, env.ipinfoSync, ds)

	conf, err := config.Load(env.configPath)
	if err != nil {
		return err
	}

	entrypoints, err := makeEntrypoints(appCtx, *conf, ds)
	if err != nil {
		return err
	}

	for _, ep := range entrypoints {
		go ep.Accept()
		defer ep.Close()
	}

	log.Info().Msgf("%d entrypoints listening...", len(entrypoints))

	<-appCtx.Done()

	return nil
}

func makeEntrypoints(appCtx context.Context, conf config.Config, ds *ipinfo.Dataset) ([]*proxy.Entrypoint, error) {
	rulesMap := make(map[string]*rule.Rule, len(conf.Rules))
	for _, r := range conf.Rules {
		rulesMap[r.Name] = rule.New(r, ds)
	}

	eps := make([]*proxy.Entrypoint, len(conf.Entrypoints))
	var err error

	for i, ep := range conf.Entrypoints {
		allowers := make([]proxy.Allower, len(ep.Rules))
		for j, r := range ep.Rules {
			allowers[j] = rulesMap[r] // we can do this because rules are guaranteed to be defined before entrypoints in the config package
		}

		eps[i], err = proxy.NewEntrypoint(appCtx, ep, allowers)
		if err != nil {
			return nil, err
		}
	}

	return eps, nil
}

func periodicallySyncDataset(appCtx context.Context, duration time.Duration, ds *ipinfo.Dataset) {
	for {
		// Might want to implement some sort of backoff system, this means that if the dataset fails at startup, we'll reject all ipinfo based traffic
		updated, err := ds.Sync(appCtx)
		if err != nil {
			log.Error().Err(err).Msg("error syncing ipinfo dataset")
		} else if updated {
			log.Info().Msg("ipinfo dataset updated")
		}

		err = ds.Load()
		if err != nil {
			log.Error().Err(err).Msg("error loading ipinfo dataset")
		}

		select {
		case <-appCtx.Done():
			return
		case <-time.After(duration): // Should probably make this configurable
		}
	}
}

func newAppContext() context.Context {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt, syscall.SIGKILL)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		log.Info().Msgf("received signal %q shutting down...", <-c)
		cancel()
	}()

	return ctx
}

func main() {
	ctx := newAppContext()
	if err := run(ctx); err != nil {
		fmt.Printf("runtime error: %v\n", err)
		os.Exit(1)
	}
}
