package platform

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// MachineStatus is derived, never stored: it combines liveness and alerts.
type MachineStatus string

const (
	StatusRunning  MachineStatus = "running"
	StatusWarning  MachineStatus = "warning"
	StatusCritical MachineStatus = "critical"
	StatusOffline  MachineStatus = "offline"
)

type MachineView struct {
	telemetry.Machine
	Status   MachineStatus      `json:"status"`
	LastSeen int64              `json:"last_seen,omitempty"`
	Values   map[string]float64 `json:"values"`
}

type FleetService struct {
	machines   MachineRepository
	alerts     AlertRepository
	history    HistoryReader
	live       *LiveView
	staleAfter time.Duration
	now        func() time.Time
}

func NewFleetService(machines MachineRepository, alerts AlertRepository, history HistoryReader, live *LiveView, staleAfter time.Duration) *FleetService {
	return &FleetService{machines: machines, alerts: alerts, history: history, live: live, staleAfter: staleAfter, now: time.Now}
}

// Machines returns every machine of the tenant with live values and status.
func (s *FleetService) Machines(ctx context.Context, p identity.Principal) ([]MachineView, error) {
	machines, err := s.machines.ListMachines(ctx, p.TenantID)
	if err != nil {
		return nil, err
	}
	open, err := s.alerts.ListAlerts(ctx, p.TenantID, AlertFilter{OpenOnly: true, Limit: 500})
	if err != nil {
		return nil, err
	}
	worst := map[string]alerting.Severity{}
	for _, a := range open {
		if a.Severity == alerting.SeverityCritical || worst[a.Machine] == "" {
			worst[a.Machine] = a.Severity
		}
	}

	now := s.now()
	views := make([]MachineView, 0, len(machines))
	for _, m := range machines {
		v := MachineView{Machine: m, Status: StatusOffline, Values: map[string]float64{}}
		if r, ok := s.live.Latest(p.TenantSlug, m.Slug); ok {
			v.Values, v.LastSeen = r.Values, r.Timestamp
			if now.Sub(r.Time()) <= s.staleAfter {
				v.Status = statusFor(worst[m.Slug])
			}
		}
		views = append(views, v)
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Slug < views[j].Slug })
	return views, nil
}

func statusFor(sev alerting.Severity) MachineStatus {
	switch sev {
	case alerting.SeverityCritical:
		return StatusCritical
	case alerting.SeverityWarning:
		return StatusWarning
	}
	return StatusRunning
}

// History returns a downsampled series, choosing the bucket size from the
// range so charts always get ~120 points regardless of the window.
func (s *FleetService) History(ctx context.Context, p identity.Principal, machine, metric string, rng time.Duration) ([]telemetry.Sample, error) {
	if rng <= 0 || rng > 7*24*time.Hour {
		return nil, fmt.Errorf("%w: range must be between 1s and 7d", ErrInvalid)
	}
	ok, err := s.machines.MachineExists(ctx, p.TenantID, machine)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	step := max(time.Second, (rng / 120).Truncate(time.Second))
	return s.history.History(ctx, p.TenantSlug, machine, metric, rng, step)
}

func (s *FleetService) Gateways(p identity.Principal) []telemetry.GatewayStatus {
	return s.live.Gateways(p.TenantSlug)
}
