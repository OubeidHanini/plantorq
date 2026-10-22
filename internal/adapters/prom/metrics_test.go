package prom

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Each test uses its own registry, so metrics never leak between tests.

func TestCollectorMetrics(t *testing.T) {
	m := NewCollector(prometheus.NewRegistry())

	m.PollCompleted("cnc-01", 0.01, nil)
	m.PollCompleted("cnc-01", 0.02, errors.New("timeout"))
	m.Published(true)
	m.StatusReported(telemetry.GatewayStatus{Buffered: 42, MachinesOnline: 4, MachinesTotal: 5, Connected: true})

	checks := map[string]struct{ got, want float64 }{
		"poll ok":     {testutil.ToFloat64(m.polls.WithLabelValues("cnc-01", "ok")), 1},
		"poll error":  {testutil.ToFloat64(m.polls.WithLabelValues("cnc-01", "error")), 1},
		"replayed":    {testutil.ToFloat64(m.published.WithLabelValues("replay")), 1},
		"depth":       {testutil.ToFloat64(m.depth), 42},
		"online":      {testutil.ToFloat64(m.online), 4},
		"connected":   {testutil.ToFloat64(m.connected), 1},
		"poll timing": {float64(testutil.CollectAndCount(m.pollTime)), 1}, // errors are not timed
	}
	for name, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", name, c.got, c.want)
		}
	}
}

func TestRulesMetrics(t *testing.T) {
	m := NewRules(prometheus.NewRegistry())
	m.ReadingEvaluated()
	m.AlertChanged(alerting.Fired, alerting.SeverityCritical)
	m.AlertChanged(alerting.Fired, alerting.SeverityCritical)

	if got := testutil.ToFloat64(m.transitions.WithLabelValues("firing", "critical")); got != 2 {
		t.Fatalf("critical firings = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.evaluated); got != 1 {
		t.Fatalf("evaluated = %v, want 1", got)
	}
}
