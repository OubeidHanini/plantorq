package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

type fakeStore struct {
	mu      sync.Mutex
	batches [][]telemetry.Reading
	fail    int
}

func (f *fakeStore) WriteReadings(_ context.Context, rs []telemetry.Reading) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail > 0 {
		f.fail--
		return errors.New("db down")
	}
	f.batches = append(f.batches, append([]telemetry.Reading(nil), rs...))
	return nil
}

func (f *fakeStore) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.batches {
		n += len(b)
	}
	return n
}

type countingMetrics struct {
	NopMetrics
	mu      sync.Mutex
	dropped map[string]int
}

func (m *countingMetrics) Dropped(reason string, n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropped[reason] += n
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func cfg(batch, queue int) Config {
	return Config{BatchSize: batch, FlushInterval: time.Hour, QueueSize: queue, MaxRetries: 3, RetryBackoff: time.Millisecond}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestFlushesOnBatchSizeAndOnShutdown(t *testing.T) {
	store := &fakeStore{}
	s := NewService(cfg(3, 100), store, nil, quiet())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	for i := 0; i < 4; i++ {
		s.Accept(telemetry.Reading{Machine: "m", Timestamp: int64(i)})
	}
	waitFor(t, func() bool { return store.total() == 3 })

	cancel()
	<-done
	if store.total() != 4 {
		t.Fatalf("after shutdown %d readings written, want 4", store.total())
	}
}

func TestRetriesTransientFailures(t *testing.T) {
	store := &fakeStore{fail: 2}
	s := NewService(cfg(1, 10), store, nil, quiet())
	s.write(context.Background(), []telemetry.Reading{{Machine: "m"}})
	if store.total() != 1 {
		t.Fatal("expected success on third attempt")
	}
}

func TestDropsWhenQueueFullInsteadOfBlocking(t *testing.T) {
	m := &countingMetrics{dropped: map[string]int{}}
	s := NewService(cfg(10, 1), &fakeStore{}, m, quiet())
	if !s.Accept(telemetry.Reading{}) {
		t.Fatal("first reading should be accepted")
	}
	if s.Accept(telemetry.Reading{}) {
		t.Fatal("second reading should be dropped: queue is full and Run is not consuming")
	}
	if m.dropped["queue_full"] != 1 {
		t.Fatalf("drop not counted: %v", m.dropped)
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	m := &countingMetrics{dropped: map[string]int{}}
	s := NewService(cfg(1, 10), &fakeStore{fail: 100}, m, quiet())
	s.write(context.Background(), []telemetry.Reading{{}, {}})
	if m.dropped["write_failed"] != 2 {
		t.Fatalf("expected 2 readings dropped after retries, got %v", m.dropped)
	}
}
