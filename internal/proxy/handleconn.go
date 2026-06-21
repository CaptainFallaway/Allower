package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/rs/zerolog"
)

func (e *Entrypoint) handleConn(client *net.TCPConn) {
	defer client.Close()

	// net.TCPConn.RemoteAddr() returns a net.Addr, but we know it's a *net.TCPAddr, so we can assert it and extract the IP address.
	ip := client.RemoteAddr().(*net.TCPAddr).AddrPort().Addr().Unmap() // Unmap IPv4-mapped IPv6 addresses to pure IPv4 for consistent allow matching & logging

	log := e.log.With().Str("remote_ip", ip.String()).Str("target", e.target).Logger()

	for _, a := range e.allowers {
		if !a.IsAllowed(ip) {
			log.Info().Msg("connection denied")
			return
		}
	}

	log.Debug().Msg("connection accepted")

	// Dial the target with a timeout context to avoid hanging if the target is unreachable.
	ctx, cancel := context.WithTimeout(e.ctx, e.dialTimeout)

	target, err := new(net.Dialer).DialContext(ctx, "tcp", e.target)
	if err != nil {
		log.Error().Err(err).Msg("failed to dial target")
		cancel()
		return
	}
	defer target.Close()
	cancel()

	targetTCP := target.(*net.TCPConn)

	// Set keep-alive on both connections to help detect dead peers.
	client.SetKeepAlive(true)
	targetTCP.SetKeepAlive(true)
	client.SetKeepAlivePeriod(e.keepalive)
	targetTCP.SetKeepAlivePeriod(e.keepalive)

	log.Debug().Msg("proxying connection")

	e.bidirectionalCopy(log, target.(*net.TCPConn), client)
}

func (e *Entrypoint) bidirectionalCopy(log zerolog.Logger, target, client *net.TCPConn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Copy from client to target
	go func() {
		defer wg.Done()

		_, err := io.Copy(target, client)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Error().Err(err).Msg("error copying from client to target")
		}

		target.CloseWrite() // signal EOF downstream
	}()

	// Copy from target to client
	go func() {
		defer wg.Done()

		_, err := io.Copy(client, target)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Error().Err(err).Msg("error copying from target to client")
		}

		client.CloseWrite() // signal EOF back to client
	}()

	wg.Wait()

	log.Debug().Msg("connection closed")
}
