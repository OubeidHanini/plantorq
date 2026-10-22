package collector

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
	"github.com/OubeidHanini/plantorq/internal/pkg/stats"
)

type Config struct {
	Tenant         string
	Gateway        string
	PollInterval   time.Duration
	MaxBackoff     time.Duration
	StatusInterval time.Duration
	DrainInterval  time.Duration
	DrainBatch     int
}

// Agent wires pollers, the forwarder and the health reporter together.
//
//	poller(machine 1) ─┐
//	poller(machine 2) ─┼─> readings chan ─> Forwarder ─> Publisher | Buffer
//	poller(machine N) ─┘
//
// One goroutine per machine isolates slow or dead devices from healthy ones.
type Agent struct {
	cfg       Config
	log       *slog.Logger
	pollers   []*Poller
	forwarder *Forwarder
	pub       Publisher
	buf       Buffer
	metrics   Metrics
	latency   *stats.Window
}

func NewAgent(cfg Config, machines []Machine, pub Publisher, buf Buffer, metrics Metrics, log *slog.Logger) *Agent {
	if metrics == nil {
		metrics = NopMetrics{}
	}
	a := &Agent{cfg: cfg, log: log, pub: pub, buf: buf, metrics: metrics, latency: stats.NewWindow(512)}
	for _, m := range machines {
		a.pollers = append(a.pollers, &Poller{
			machine: m, tenant: cfg.Tenant, gateway: cfg.Gateway,
			interval: cfg.PollInterval, maxDelay: cfg.MaxBackoff,
			log: log, metrics: metrics, latency: a.latency.Add,
		})
	}
	a.forwarder = &Forwarder{
		pub: pub, buf: buf, log: log, metrics: metrics,
		drainEvery: cfg.DrainInterval, drainBatch: cfg.DrainBatch,
	}
	return a
}

// Run blocks until ctx is cancelled and all goroutines have stopped.
func (a *Agent) Run(ctx context.Context) {
	readings := make(chan telemetry.Reading, 256)
	var wg sync.WaitGroup
	for _, p := range a.pollers {
		wg.Add(1)
		go func(p *Poller) { defer wg.Done(); p.run(ctx, readings) }(p)
	}
	wg.Add(2)
	go func() { defer wg.Done(); a.forwarder.run(ctx, readings) }()
	go func() { defer wg.Done(); a.reportStatus(ctx) }()
	wg.Wait()
}

func (a *Agent) reportStatus(ctx context.Context) {
	t := time.NewTicker(a.cfg.StatusInterval)
	defer t.Stop()
	prev, prevAt := a.forwarder.Published(), time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			cur := a.forwarder.Published()
			status := a.Status(now, float64(cur-prev)/now.Sub(prevAt).Seconds())
			prev, prevAt = cur, now
			a.metrics.StatusReported(status)
			if !a.pub.Connected() {
				continue
			}
			if err := a.pub.PublishStatus(ctx, status); err != nil {
				a.log.Warn("status publish failed", "err", err)
			}
		}
	}
}

// Status snapshots the agent's health.
func (a *Agent) Status(now time.Time, msgPerSec float64) telemetry.GatewayStatus {
	online := 0
	for _, p := range a.pollers {
		if p.Online(now, 3*a.cfg.PollInterval+time.Second) {
			online++
		}
	}
	return telemetry.GatewayStatus{
		Tenant: a.cfg.Tenant, Gateway: a.cfg.Gateway, Timestamp: now.UnixMilli(),
		MsgPerSec: msgPerSec, Buffered: a.buf.Len(),
		PollP95Ms:      float64(a.latency.Quantile(0.95).Microseconds()) / 1000,
		MachinesOnline: online, MachinesTotal: len(a.pollers),
		Connected: a.pub.Connected(),
	}
}
