package proxy

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

var Metrics = &metrics{}
var MetricsEnabled = false // Fine to be volatile

type metrics struct {
	requests atomic.Uint64
	denied   atomic.Uint64
	sum      atomic.Uint64
	buckets  [8]atomic.Uint64
}

func (m *metrics) Record(d time.Duration, denied bool) {
	if !MetricsEnabled {
		return
	}

	m.requests.Add(1)
	if denied {
		m.denied.Add(1)
	}
	m.sum.Add(uint64(d))

	switch {
	case d < 100*time.Microsecond:
		m.buckets[0].Add(1)
	case d < 250*time.Microsecond:
		m.buckets[1].Add(1)
	case d < 500*time.Microsecond:
		m.buckets[2].Add(1)
	case d < time.Millisecond:
		m.buckets[3].Add(1)
	case d < 2500*time.Microsecond:
		m.buckets[4].Add(1)
	case d < 5*time.Millisecond:
		m.buckets[5].Add(1)
	case d < 10*time.Millisecond:
		m.buckets[6].Add(1)
	default:
		m.buckets[7].Add(1)
	}
}

func (m *metrics) Log() {
	requests := m.requests.Load()
	if requests == 0 {
		return
	}

	denied := m.denied.Load()

	percent := func(n uint64) string {
		return fmt.Sprintf("%.1f%%", float64(n)*100/float64(requests))
	}

	log.Info().
		Str("avg", time.Duration(m.sum.Load()/requests).String()).
		Str("<100µs", percent(m.buckets[0].Load())).
		Str("<250µs", percent(m.buckets[1].Load())).
		Str("<500µs", percent(m.buckets[2].Load())).
		Str("<1ms", percent(m.buckets[3].Load())).
		Str("<2.5ms", percent(m.buckets[4].Load())).
		Str("<5ms", percent(m.buckets[5].Load())).
		Str("<10ms", percent(m.buckets[6].Load())).
		Str(">=10ms", percent(m.buckets[7].Load())).
		Uint64("requests", requests).
		Uint64("denied", denied).
		Msg("connection metrics")
}

func (m *metrics) PeriodicallyLog(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.Log()
		case <-ctx.Done():
			return
		}
	}
}
