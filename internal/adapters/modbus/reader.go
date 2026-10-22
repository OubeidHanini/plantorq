package modbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	gomodbus "github.com/goburrow/modbus"

	"github.com/OubeidHanini/plantorq/internal/app/collector"
)

// TCPReader reads one Modbus TCP slave (unit id) in a single transaction
// covering all configured registers. It implements collector.DeviceReader.
type TCPReader struct {
	mu      sync.Mutex
	handler *gomodbus.TCPClientHandler
	client  gomodbus.Client
	regs    []Register
	start   uint16
	qty     uint16
}

var _ collector.DeviceReader = (*TCPReader)(nil)

func NewTCPReader(address string, unitID byte, timeout time.Duration, regs []Register) (*TCPReader, error) {
	start, qty, err := Span(regs)
	if err != nil {
		return nil, err
	}
	h := gomodbus.NewTCPClientHandler(address)
	h.SlaveId = unitID
	h.Timeout = timeout
	h.IdleTimeout = time.Minute
	return &TCPReader{handler: h, client: gomodbus.NewClient(h), regs: regs, start: start, qty: qty}, nil
}

// Read performs one FC03 request. The context bounds the call even though
// the underlying library only supports a fixed timeout.
func (r *TCPReader) Read(ctx context.Context) (map[string]float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := r.client.ReadHoldingRegisters(r.start, r.qty)
	if err != nil {
		return nil, fmt.Errorf("modbus %s unit %d: %w", r.handler.Address, r.handler.SlaveId, err)
	}
	return Decode(raw, r.start, r.regs)
}

// Reset closes the TCP connection; the library reconnects on the next call.
func (r *TCPReader) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.handler.Close()
}
