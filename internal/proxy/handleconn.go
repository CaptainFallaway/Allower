package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"

	"github.com/rs/zerolog"
)

func getIp(ta *net.TCPAddr) netip.Addr {
	ip, ok := netip.AddrFromSlice(ta.IP)
	if !ok {
		panic("did not expect invalid IP address: " + ta.IP.String())
	}
	return ip.Unmap() // Unmap IPv4-mapped IPv6 addresses to pure IPv4 for consistent allow matching & logging
}

func (e *Entrypoint) handleConn(log zerolog.Logger, client *net.TCPConn) {
	ctx, cancel := context.WithTimeout(e.ctx, e.dialTimeout)

	target, err := e.dialer.DialContext(ctx, "tcp", e.target) // target gets closed in `bidiretionalCopy`
	if err != nil {
		log.Error().Err(err).Msg("failed to dial target")
		client.Close()
		cancel()
		return
	}
	cancel()

	targetTCP := target.(*net.TCPConn)

	// Set keep-alive on both connections to help detect dead peers.
	e.setKeepalive(log, client, targetTCP)

	e.bidirectionalCopy(log, targetTCP, client)
}

func (e *Entrypoint) setKeepalive(log zerolog.Logger, client, target *net.TCPConn) {
	if err := client.SetKeepAlive(true); err != nil {
		log.Warn().Err(err).Msg("failed to configure client keepalive")
	}
	if err := target.SetKeepAlive(true); err != nil {
		log.Warn().Err(err).Msg("failed to configure target keepalive")
	}
	if err := client.SetKeepAlivePeriod(e.keepalive); err != nil {
		log.Warn().Err(err).Msg("failed to configure client keepalive period")
	}
	if err := target.SetKeepAlivePeriod(e.keepalive); err != nil {
		log.Warn().Err(err).Msg("failed to configure target keepalive period")
	}
}

func (e *Entrypoint) bidirectionalCopy(log zerolog.Logger, target, client *net.TCPConn) {
	var wg sync.WaitGroup
	var once sync.Once

	closeBoth := func() {
		once.Do(func() {
			_ = client.Close()
			_ = target.Close()
		})
	}

	copyOneWay := func(src, dst *net.TCPConn, direction string) {
		defer wg.Done()

		if _, err := io.Copy(dst, src); err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Warn().
					Err(err).
					Str("direction", direction).
					Msg("proxy copy failed")
			}
			closeBoth()
			return
		}

		if err := dst.CloseWrite(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Warn().
				Err(err).
				Str("direction", direction).
				Msg("failed to half-close proxy connection")
			closeBoth()
		}
	}

	wg.Add(2)

	go copyOneWay(client, target, "client -> target")
	copyOneWay(target, client, "target -> client")

	wg.Wait()
	closeBoth()

	log.Debug().Msg("connection closed")
}
