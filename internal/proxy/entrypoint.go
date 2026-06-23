package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/CaptainFallaway/Allower/internal/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Allower interface {
	IsAllowed(ip netip.Addr) bool
}

type Entrypoint struct {
	ctx context.Context

	allowers []Allower

	target      string
	dialTimeout time.Duration
	keepalive   time.Duration // Tunable OS socket keepalive duration
	listener    *net.TCPListener

	log    zerolog.Logger
	dialer *net.Dialer
}

func NewEntrypoint(ctx context.Context, ec config.Entrypoint, allowers []Allower) (*Entrypoint, error) {
	e := &Entrypoint{
		ctx:         ctx,
		allowers:    allowers,
		target:      ec.Target,
		dialTimeout: ec.DialTimeout.Duration,
		keepalive:   ec.Keepalive.Duration,
		dialer:      new(net.Dialer),
	}

	lc := new(net.ListenConfig) // Mostly for the context usage, might want to look into SO_REUSEPORT later tho

	e.log = log.With().Str("entrypoint", ec.Name).Str("target", e.target).Logger()

	ln, err := lc.Listen(ctx, "tcp", ec.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create entrypoint listener: %w", err)
	}

	e.listener = ln.(*net.TCPListener)

	e.log.Info().Str("addr", ec.Addr).Int("rules", len(e.allowers)).Msg("entrypoint listening")

	return e, nil
}

var seq atomic.Uint64

func (e *Entrypoint) Accept() {
	for {
		conn, err := e.listener.AcceptTCP()
		traceSeq := seq.Add(1)
		if errors.Is(err, net.ErrClosed) {
			return
		} else if err != nil {
			log.Error().Err(err).Uint64("trace", traceSeq).Msg("failed to accept connection")
			continue
		}

		start := time.Now()

		ip := getIp(conn.RemoteAddr().(*net.TCPAddr))

		log := e.log.With().Stringer("remote_ip", ip).Uint64("trace", traceSeq).Logger()

		for _, a := range e.allowers {
			if !a.IsAllowed(ip) {
				Metrics.Record(time.Since(start), true)
				log.Debug().Msg("connection denied")
				conn.Close() // Profile to see if I need to make this a goroutine and have a worker pool for closing connections...
				continue
			}
		}

		Metrics.Record(time.Since(start), false)

		log.Debug().Msg("connection accepted")

		go e.handleConn(log, conn)
	}
}

func (e *Entrypoint) Close() error {
	err := e.listener.Close()
	if err != nil {
		return fmt.Errorf("failed to close entrypoint: %w", err)
	}
	return nil
}
