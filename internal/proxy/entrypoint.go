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
	keepalive   time.Duration // Tunable OS socket keepalive duration
	listener    *net.TCPListener

	log    zerolog.Logger
	dialer *net.Dialer

	closeChan chan<- *net.TCPConn
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

	// This for some reason made the denying much, much faster :)
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

	e.log.Info().Str("addr", ec.Addr).Int("rules", len(e.allowers)).Msg("entrypoint listening")

	return e, nil
}

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

func (e *Entrypoint) Close() error {
	err := e.listener.Close()
	if err != nil {
		return fmt.Errorf("failed to close entrypoint: %w", err)
	}
	return nil
}
