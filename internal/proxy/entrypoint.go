package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
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

	keepalive time.Duration // Tunable OS socket keepalive duration

	listener *net.TCPListener
	log      zerolog.Logger
}

func NewEntrypoint(ctx context.Context, ec config.Entrypoint, allowers []Allower) (*Entrypoint, error) {
	e := &Entrypoint{
		ctx:         ctx,
		target:      ec.Target,
		dialTimeout: ec.DialTimeout.Duration,
		keepalive:   ec.Keepalive.Duration,
		allowers:    allowers,
	}

	lc := new(net.ListenConfig) // Mostly for the context usage, might want to look into SO_REUSEPORT later tho

	e.log = log.With().Str("entrypoint", ec.Name).Logger()

	ln, err := lc.Listen(ctx, "tcp", ec.Addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create entrypoint listener: %w", err)
	}

	e.listener = ln.(*net.TCPListener)

	return e, nil
}

func (e *Entrypoint) Listen() {
	for {
		conn, err := e.listener.AcceptTCP()
		if errors.Is(err, net.ErrClosed) {
			return
		} else if err != nil {
			log.Error().Err(err).Msg("failed to accept connection")
			continue
		}
		go e.handleConn(conn)
	}
}

func (e *Entrypoint) Close() error {
	err := e.listener.Close()
	if err != nil {
		return fmt.Errorf("failed to close entrypoint: %w", err)
	}
	return nil
}
