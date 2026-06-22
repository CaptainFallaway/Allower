package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func getIp(ta *net.TCPAddr) netip.Addr {
	ip, _ := netip.AddrFromSlice(ta.IP)
	return ip.Unmap() // Unmap IPv4-mapped IPv6 addresses to pure IPv4 for consistent allow matching & logging
}

func (e *Entrypoint) handleConn(client *net.TCPConn, traceSeq uint64, start time.Time) {
	ip := getIp(client.RemoteAddr().(*net.TCPAddr))

	log := e.log.With().Str("remote_ip", ip.String()).Uint64("trace", traceSeq).Logger()

	for _, a := range e.allowers {
		if !a.IsAllowed(ip) {
			Metrics.Record(time.Since(start), true)
			log.Debug().Msg("connection denied")
			client.SetLinger(0) // Ensure the connection is closed immediately without waiting for pending data to be sent
			client.Close()
			return
		}
	}

	Metrics.Record(time.Since(start), false)

	log.Debug().Msg("connection accepted")

	ctx, cancel := context.WithTimeout(e.ctx, e.dialTimeout)

	target, err := new(net.Dialer).DialContext(ctx, "tcp", e.target) // target gets closed in `bidiretionalCopy`
	if err != nil {
		log.Error().Err(err).Msg("failed to dial target")
		client.Close()
		cancel()
		return
	}
	cancel()

	targetTCP := target.(*net.TCPConn)

	// Set keep-alive on both connections to help detect dead peers.
	e.setKeepalive(client, targetTCP)

	e.bidirectionalCopy(log, targetTCP, client)
}

func (e *Entrypoint) setKeepalive(client, target *net.TCPConn) {
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
	go copyOneWay(target, client, "target -> client")

	wg.Wait()
	closeBoth()

	log.Debug().Msg("connection closed")
}
