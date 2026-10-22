package collector

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Forwarder implements store-and-forward delivery:
//
//   - online and nothing buffered: publish directly;
//   - offline, or a backlog exists: append to the persistent buffer, so
//     ordering is preserved and nothing is lost;
//   - a drain loop replays the backlog, oldest first, once the link is back.
type Forwarder struct {
	pub        Publisher
	buf        Buffer
	log        *slog.Logger
	metrics    Metrics
	drainEvery time.Duration
	drainBatch int

	published atomic.Uint64
}

func (f *Forwarder) run(ctx context.Context, in <-chan telemetry.Reading) {
	go f.drainLoop(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case r := <-in:
			f.forward(ctx, r)
		}
	}
}

func (f *Forwarder) forward(ctx context.Context, r telemetry.Reading) {
	if f.pub.Connected() && f.buf.Len() == 0 {
		if err := f.pub.PublishReading(ctx, r); err == nil {
			f.published.Add(1)
			f.metrics.Published(false)
			return
		}
	}
	if err := f.buf.Push(r); err != nil {
		f.log.Error("buffer write failed, reading lost", "machine", r.Machine, "err", err)
		return
	}
	f.metrics.Buffered()
}

func (f *Forwarder) drainLoop(ctx context.Context) {
	t := time.NewTicker(f.drainEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.drain(ctx)
		}
	}
}

// drain replays buffered readings until the buffer is empty or a publish
// fails. Items are acknowledged only after successful delivery
// (at-least-once semantics; consumers are idempotent on timestamp).
func (f *Forwarder) drain(ctx context.Context) {
	for f.pub.Connected() && ctx.Err() == nil {
		items, err := f.buf.Peek(f.drainBatch)
		if err != nil || len(items) == 0 {
			return
		}
		sent := items[:0:0]
		for _, it := range items {
			if err := f.pub.PublishReading(ctx, it.Reading); err != nil {
				break
			}
			sent = append(sent, it)
			f.published.Add(1)
			f.metrics.Published(true)
		}
		if err := f.buf.Ack(sent); err != nil {
			f.log.Error("buffer ack failed", "err", err)
			return
		}
		if len(sent) < len(items) {
			return // link dropped mid-batch; retry on next tick
		}
		f.log.Info("replayed buffered readings", "count", len(sent), "remaining", f.buf.Len())
	}
}

// Published returns the total number of readings delivered.
func (f *Forwarder) Published() uint64 { return f.published.Load() }
