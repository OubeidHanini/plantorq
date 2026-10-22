package platform

import (
	"sync"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// LiveView is an in-memory projection of the latest reading of every machine
// and the latest status of every gateway, fed from the message bus. It lets
// the dashboard load instantly without querying the time-series database.
//
// Each API replica builds its own copy from the bus, so the API stays
// horizontally scalable without shared state.
type LiveView struct {
	mu       sync.RWMutex
	readings map[string]map[string]telemetry.Reading
	gateways map[string]map[string]telemetry.GatewayStatus
}

func NewLiveView() *LiveView {
	return &LiveView{
		readings: map[string]map[string]telemetry.Reading{},
		gateways: map[string]map[string]telemetry.GatewayStatus{},
	}
}

// ApplyReading merges r into the latest values of its machine. Readings from
// different sources (edge values, analytics scores) are combined, and stale
// out-of-order messages are ignored.
func (v *LiveView) ApplyReading(r telemetry.Reading) {
	v.mu.Lock()
	defer v.mu.Unlock()
	byMachine := v.readings[r.Tenant]
	if byMachine == nil {
		byMachine = map[string]telemetry.Reading{}
		v.readings[r.Tenant] = byMachine
	}
	cur, ok := byMachine[r.Machine]
	if !ok {
		cur = telemetry.Reading{Tenant: r.Tenant, Machine: r.Machine, Values: map[string]float64{}}
	}
	values := make(map[string]float64, len(cur.Values)+len(r.Values))
	for k, val := range cur.Values {
		values[k] = val
	}
	for k, val := range r.Values {
		values[k] = val
	}
	cur.Values = values
	if r.Source == telemetry.SourceEdge && r.Timestamp >= cur.Timestamp {
		cur.Timestamp, cur.Gateway, cur.Source = r.Timestamp, r.Gateway, r.Source
	}
	byMachine[r.Machine] = cur
}

func (v *LiveView) ApplyGateway(s telemetry.GatewayStatus) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.gateways[s.Tenant] == nil {
		v.gateways[s.Tenant] = map[string]telemetry.GatewayStatus{}
	}
	v.gateways[s.Tenant][s.Gateway] = s
}

// Latest returns the latest reading of a machine.
func (v *LiveView) Latest(tenant, machine string) (telemetry.Reading, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	r, ok := v.readings[tenant][machine]
	return r, ok
}

// Gateways returns the latest status of every gateway of a tenant.
func (v *LiveView) Gateways(tenant string) []telemetry.GatewayStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]telemetry.GatewayStatus, 0, len(v.gateways[tenant]))
	for _, s := range v.gateways[tenant] {
		out = append(out, s)
	}
	return out
}
