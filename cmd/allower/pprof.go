//go:build pprof

package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/rs/zerolog/log"
)

func init() {
	go func() {
		log.Info().Msg("starting pprof server on :6060")
		log.Error().Err(http.ListenAndServe(":6060", nil)).Msg("stuff")
	}()
}
