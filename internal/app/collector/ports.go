// Package collector is the edge agent use case: poll industrial devices,
// forward readings to the platform, and never lose data while offline.
//
// It depends only on the ports below; protocol, broker and storage details
// are plugged in from the composition root (cmd/edge-agent).
package collector

import (
	"context"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// DeviceReader reads one machine's current values. Each industrial protocol
// (Modbus TCP today, OPC UA next) is a separate implementation: a Strategy,
// so adding a protocol never touches the collector.
type DeviceReader interface {
	Read(ctx context.Context) (map[string]float64, error)
	// Reset drops the underlying connection so the next Read reconnects.
	Reset()
}

// Publisher delivers readings and health reports to the platform.
type Publisher interface {
	PublishReading(ctx context.Context, r telemetry.Reading) error
	PublishStatus(ctx context.Context, s telemetry.GatewayStatus) error
	Connected() bool
}

// Buffered is a reading waiting in the offline buffer.
type Buffered struct {
	ID      uint64
	Reading telemetry.Reading
}

// Buffer persists readings while the platform is unreachable.
type Buffer interface {
	Push(r telemetry.Reading) error
	Peek(n int) ([]Buffered, error)
	Ack(items []Buffered) error
	Len() int
}

// Metrics receives operational signals. Kept as a port so the use case does
// not import Prometheus; see adapters/prom for the implementation.
type Metrics interface {
	PollCompleted(machine string, seconds float64, err error)
	Published(fromBuffer bool)
	Buffered()
	// StatusReported receives each periodic health snapshot, so gauges such
	// as buffer depth stay current even while the broker is unreachable.
	StatusReported(s telemetry.GatewayStatus)
}

// NopMetrics discards all signals (handy in tests).
type NopMetrics struct{}

func (NopMetrics) PollCompleted(string, float64, error)   {}
func (NopMetrics) Published(bool)                         {}
func (NopMetrics) Buffered()                              {}
func (NopMetrics) StatusReported(telemetry.GatewayStatus) {}
