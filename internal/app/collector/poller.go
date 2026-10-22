package collector

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Machine is one device the agent polls.
type Machine struct {
	ID     string
	Reader DeviceReader
}

// Poller polls a single machine on a fixed interval. On consecutive failures
// it backs off exponentially (with jitter, so many agents recovering from the
// same outage do not hammer devices in lockstep).
type Poller struct {
	machine  Machine
	tenant   string
	gateway  string
	interval time.Duration
	maxDelay time.Duration
	log      *slog.Logger
	metrics  Metrics
	latency  func(time.Duration)

	lastOK atomic.Int64 // Unix ms of the last successful poll
}

func (p *Poller) run(ctx context.Context, out chan<- telemetry.Reading) {
	failures := 0
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		start := time.Now()
		values, err := p.machine.Reader.Read(ctx)
		elapsed := time.Since(start)
		p.metrics.PollCompleted(p.machine.ID, elapsed.Seconds(), err)

		if err != nil {
			failures++
			p.machine.Reader.Reset()
			delay := p.backoff(failures)
			p.log.Warn("poll failed", "machine", p.machine.ID, "failures", failures, "retry_in", delay, "err", err)
			timer.Reset(delay)
			continue
		}
		if failures > 0 {
			p.log.Info("machine recovered", "machine", p.machine.ID, "after_failures", failures)
		}
		failures = 0
		p.latency(elapsed)
		p.lastOK.Store(start.UnixMilli())

		r := telemetry.Reading{
			Tenant: p.tenant, Machine: p.machine.ID, Gateway: p.gateway,
			Source: telemetry.SourceEdge, Timestamp: start.UnixMilli(), Values: values,
		}
		select {
		case out <- r:
		case <-ctx.Done():
			return
		}
		timer.Reset(max(0, p.interval-elapsed))
	}
}

// backoff returns interval * 2^(failures-1), capped, with ±20% jitter.
func (p *Poller) backoff(failures int) time.Duration {
	d := p.interval << min(failures-1, 10)
	if d > p.maxDelay || d <= 0 {
		d = p.maxDelay
	}
	jitter := 0.8 + 0.4*rand.Float64()
	return time.Duration(float64(d) * jitter)
}

// Online reports whether the machine answered within the last staleAfter.
func (p *Poller) Online(now time.Time, staleAfter time.Duration) bool {
	last := p.lastOK.Load()
	return last > 0 && now.Sub(time.UnixMilli(last)) <= staleAfter
}
