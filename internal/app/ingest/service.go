// Package ingest is the ingestor use case: accept readings from the message
// bus and persist them in batches.
package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Store persists readings. Implemented by adapters/influx.
type Store interface {
	WriteReadings(ctx context.Context, readings []telemetry.Reading) error
}

// Metrics is the observability port of the use case.
type Metrics interface {
	Written(n int)
	Dropped(reason string, n int)
	FlushObserved(seconds float64)
}

type NopMetrics struct{}

func (NopMetrics) Written(int)           {}
func (NopMetrics) Dropped(string, int)   {}
func (NopMetrics) FlushObserved(float64) {}

type Config struct {
	BatchSize     int
	FlushInterval time.Duration
	QueueSize     int
	MaxRetries    int
	RetryBackoff  time.Duration
}

// Service batches readings and writes them when a batch is full or the flush
// interval elapses, whichever comes first.
//
// Accept never blocks: the MQTT client delivers messages on its own goroutine,
// and stalling it because the database is slow would back up the broker.
// Instead, overload shows up as a "queue_full" drop metric that can be
// alerted on and fixed by scaling out (the ingestor is stateless).
type Service struct {
	cfg     Config
	store   Store
	metrics Metrics
	log     *slog.Logger
	queue   chan telemetry.Reading
}

func NewService(cfg Config, store Store, metrics Metrics, log *slog.Logger) *Service {
	if metrics == nil {
		metrics = NopMetrics{}
	}
	return &Service{cfg: cfg, store: store, metrics: metrics, log: log,
		queue: make(chan telemetry.Reading, cfg.QueueSize)}
}

// Accept enqueues r, returning false if it had to be dropped.
func (s *Service) Accept(r telemetry.Reading) bool {
	select {
	case s.queue <- r:
		return true
	default:
		s.metrics.Dropped("queue_full", 1)
		return false
	}
}

// Run processes the queue until ctx is cancelled, then flushes what is left.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()
	batch := make([]telemetry.Reading, 0, s.cfg.BatchSize)

	flush := func(ctx context.Context) {
		if len(batch) > 0 {
			s.write(ctx, batch)
			batch = batch[:0]
		}
	}

	for {
		select {
		case r := <-s.queue:
			batch = append(batch, r)
			if len(batch) >= s.cfg.BatchSize {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		case <-ctx.Done():
			s.shutdown(batch)
			return
		}
	}
}

// shutdown drains the queue and performs a final write under a fresh
// deadline, so a rolling deploy does not lose in-flight readings.
func (s *Service) shutdown(batch []telemetry.Reading) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		select {
		case r := <-s.queue:
			batch = append(batch, r)
			continue
		default:
		}
		break
	}
	if len(batch) > 0 {
		s.write(ctx, batch)
	}
}

func (s *Service) write(ctx context.Context, batch []telemetry.Reading) {
	delay := s.cfg.RetryBackoff
	for attempt := 0; ; attempt++ {
		start := time.Now()
		err := s.store.WriteReadings(ctx, batch)
		s.metrics.FlushObserved(time.Since(start).Seconds())
		if err == nil {
			s.metrics.Written(len(batch))
			return
		}
		if attempt >= s.cfg.MaxRetries || ctx.Err() != nil {
			s.log.Error("dropping batch after retries", "readings", len(batch), "err", err)
			s.metrics.Dropped("write_failed", len(batch))
			return
		}
		s.log.Warn("write failed, retrying", "attempt", attempt+1, "retry_in", delay, "err", err)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
		}
		delay *= 2
	}
}
