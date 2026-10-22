package alerting

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

type staticRules []alerting.Rule

func (s staticRules) EnabledRules(context.Context) ([]alerting.Rule, error) { return s, nil }

type memAlerts struct {
	nextID int64
	open   map[string]alerting.Alert
}

func key(ruleID int64, machine string) string { return fmt.Sprintf("%s#%d", machine, ruleID) }

func (m *memAlerts) Open(_ context.Context, a alerting.Alert) (alerting.Alert, bool, error) {
	k := key(a.RuleID, a.Machine)
	if existing, ok := m.open[k]; ok {
		return existing, false, nil
	}
	m.nextID++
	a.ID = m.nextID
	m.open[k] = a
	return a, true, nil
}

func (m *memAlerts) Resolve(_ context.Context, ruleID int64, machine string, at time.Time) (alerting.Alert, bool, error) {
	k := key(ruleID, machine)
	a, ok := m.open[k]
	if !ok {
		return alerting.Alert{}, false, nil
	}
	delete(m.open, k)
	a.ResolvedAt = &at
	return a, true, nil
}

type recorder struct{ got []alerting.Alert }

func (r *recorder) Notify(_ context.Context, a alerting.Alert) error {
	r.got = append(r.got, a)
	return nil
}

func TestFiresPersistsNotifiesAndResolves(t *testing.T) {
	rule := alerting.Rule{ID: 1, TenantID: 9, Tenant: "acme", Name: "Overheat", Metric: "temperature",
		Operator: alerting.GT, Threshold: 70, Severity: alerting.SeverityCritical, Enabled: true}
	repo := &memAlerts{open: map[string]alerting.Alert{}}
	notes := &recorder{}
	s := NewService(staticRules{rule}, repo, notes, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	if err := s.ReloadRules(ctx); err != nil {
		t.Fatal(err)
	}

	at := time.Now()
	read := func(offset time.Duration, v float64) telemetry.Reading {
		return telemetry.Reading{Tenant: "acme", Machine: "cnc-02", Timestamp: at.Add(offset).UnixMilli(),
			Values: map[string]float64{"temperature": v}}
	}

	s.Evaluate(ctx, read(0, 75))
	s.Evaluate(ctx, read(time.Second, 76)) // still firing: no duplicate
	if len(repo.open) != 1 || len(notes.got) != 1 {
		t.Fatalf("open=%d notifications=%d, want 1 and 1", len(repo.open), len(notes.got))
	}
	fired := notes.got[0]
	if fired.TenantID != 9 || fired.RuleName != "Overheat" || fired.Value != 75 || !fired.Open() {
		t.Fatalf("unexpected fired alert %+v", fired)
	}

	s.Evaluate(ctx, read(2*time.Second, 60))
	if len(repo.open) != 0 || len(notes.got) != 2 || notes.got[1].Open() {
		t.Fatalf("alert not resolved: open=%d notes=%+v", len(repo.open), notes.got)
	}
}
