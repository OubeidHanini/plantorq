package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// --- fakes -----------------------------------------------------------------

type fakePublisher struct {
	mu        sync.Mutex
	connected atomic.Bool
	readings  []telemetry.Reading
	statuses  []telemetry.GatewayStatus
}

func (p *fakePublisher) PublishReading(_ context.Context, r telemetry.Reading) error {
	if !p.connected.Load() {
		return errors.New("offline")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readings = append(p.readings, r)
	return nil
}

func (p *fakePublisher) PublishStatus(_ context.Context, s telemetry.GatewayStatus) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statuses = append(p.statuses, s)
	return nil
}

func (p *fakePublisher) Connected() bool { return p.connected.Load() }

func (p *fakePublisher) delivered() []telemetry.Reading {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]telemetry.Reading(nil), p.readings...)
}

type memBuffer struct {
	mu    sync.Mutex
	items []Buffered
	next  uint64
}

func (b *memBuffer) Push(r telemetry.Reading) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	b.items = append(b.items, Buffered{ID: b.next, Reading: r})
	return nil
}

func (b *memBuffer) Peek(n int) ([]Buffered, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Buffered(nil), b.items[:min(n, len(b.items))]...), nil
}

func (b *memBuffer) Ack(items []Buffered) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = b.items[len(items):]
	return nil
}

func (b *memBuffer) Len() int { b.mu.Lock(); defer b.mu.Unlock(); return len(b.items) }

type fakeReader struct {
	fail   atomic.Bool
	resets atomic.Int32
}

func (r *fakeReader) Read(context.Context) (map[string]float64, error) {
	if r.fail.Load() {
		return nil, errors.New("device unreachable")
	}
	return map[string]float64{"temperature": 50}, nil
}

func (r *fakeReader) Reset() { r.resets.Add(1) }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newForwarder(pub Publisher, buf Buffer) *Forwarder {
	return &Forwarder{pub: pub, buf: buf, log: quiet(), metrics: NopMetrics{}, drainEvery: time.Hour, drainBatch: 2}
}

func reading(ts int64) telemetry.Reading {
	return telemetry.Reading{Tenant: "acme", Machine: "cnc-01", Timestamp: ts}
}

// --- tests -----------------------------------------------------------------

func TestForwarderStoresWhileOfflineAndReplaysInOrder(t *testing.T) {
	pub, buf := &fakePublisher{}, &memBuffer{}
	f := newForwarder(pub, buf)
	ctx := context.Background()

	f.forward(ctx, reading(1))
	f.forward(ctx, reading(2))
	if buf.Len() != 2 || len(pub.delivered()) != 0 {
		t.Fatalf("offline: buffered=%d delivered=%d, want 2 and 0", buf.Len(), len(pub.delivered()))
	}

	pub.connected.Store(true)
	// A new reading arriving while a backlog exists must queue behind it.
	f.forward(ctx, reading(3))
	if buf.Len() != 3 {
		t.Fatalf("reading bypassed backlog: buffered=%d", buf.Len())
	}

	f.drain(ctx) // drainBatch=2 forces two loop iterations
	got := pub.delivered()
	if len(got) != 3 || buf.Len() != 0 {
		t.Fatalf("after drain delivered=%d buffered=%d, want 3 and 0", len(got), buf.Len())
	}
	for i, r := range got {
		if r.Timestamp != int64(i+1) {
			t.Fatalf("order broken: position %d has ts %d", i, r.Timestamp)
		}
	}
}

func TestForwarderPublishesDirectlyWhenOnline(t *testing.T) {
	pub, buf := &fakePublisher{}, &memBuffer{}
	pub.connected.Store(true)
	f := newForwarder(pub, buf)

	f.forward(context.Background(), reading(1))
	if len(pub.delivered()) != 1 || buf.Len() != 0 || f.Published() != 1 {
		t.Fatal("online reading should be published without buffering")
	}
}

func TestPollerBacksOffAndRecovers(t *testing.T) {
	rd := &fakeReader{}
	rd.fail.Store(true)
	p := &Poller{
		machine: Machine{ID: "cnc-01", Reader: rd}, tenant: "acme",
		interval: 5 * time.Millisecond, maxDelay: 20 * time.Millisecond,
		log: quiet(), metrics: NopMetrics{}, latency: func(time.Duration) {},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan telemetry.Reading, 10)
	go p.run(ctx, out)

	time.Sleep(40 * time.Millisecond)
	if rd.resets.Load() == 0 {
		t.Fatal("reader was not reset after failures")
	}
	if p.Online(time.Now(), time.Second) {
		t.Fatal("machine reported online while failing")
	}

	rd.fail.Store(false)
	select {
	case r := <-out:
		if r.Machine != "cnc-01" || r.Tenant != "acme" || r.Source != telemetry.SourceEdge {
			t.Fatalf("unexpected reading %+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("poller did not recover")
	}
	if !p.Online(time.Now(), time.Second) {
		t.Fatal("machine should be online after a successful poll")
	}
}

func TestBackoffIsCapped(t *testing.T) {
	p := &Poller{interval: time.Second, maxDelay: 10 * time.Second}
	for failures := 1; failures < 70; failures++ {
		if d := p.backoff(failures); d > 12*time.Second || d <= 0 {
			t.Fatalf("backoff(%d) = %s, outside (0, 12s]", failures, d)
		}
	}
}

func TestAgentStatus(t *testing.T) {
	pub, buf := &fakePublisher{}, &memBuffer{}
	pub.connected.Store(true)
	_ = buf.Push(reading(1))
	a := NewAgent(Config{Tenant: "acme", Gateway: "gw-1", PollInterval: time.Second},
		[]Machine{{ID: "a", Reader: &fakeReader{}}, {ID: "b", Reader: &fakeReader{}}}, pub, buf, nil, quiet())
	a.pollers[0].lastOK.Store(time.Now().UnixMilli())

	s := a.Status(time.Now(), 12.5)
	if s.MachinesOnline != 1 || s.MachinesTotal != 2 || s.Buffered != 1 || !s.Connected || s.MsgPerSec != 12.5 {
		t.Fatalf("unexpected status %+v", s)
	}
}
