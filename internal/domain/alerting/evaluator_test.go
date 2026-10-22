package alerting

import (
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

var t0 = time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)

func reading(machine string, offset time.Duration, temp float64) telemetry.Reading {
	return telemetry.Reading{
		Tenant:    "acme",
		Machine:   machine,
		Timestamp: t0.Add(offset).UnixMilli(),
		Values:    map[string]float64{"temperature": temp},
	}
}

func overheat(forDur time.Duration) Rule {
	return Rule{ID: 1, Tenant: "acme", Name: "Overheat", Metric: "temperature",
		Operator: GT, Threshold: 70, For: forDur, Severity: SeverityCritical, Enabled: true}
}

func TestStateMachine(t *testing.T) {
	type step struct {
		at        time.Duration
		temp      float64
		wantState State
		wantEvent EventKind // "" = no event
	}
	tests := []struct {
		name  string
		rule  Rule
		steps []step
	}{
		{
			name: "spike shorter than For never fires",
			rule: overheat(30 * time.Second),
			steps: []step{
				{0, 75, Pending, ""},
				{10 * time.Second, 76, Pending, ""},
				{20 * time.Second, 60, Inactive, ""},
			},
		},
		{
			name: "sustained breach fires once then resolves",
			rule: overheat(30 * time.Second),
			steps: []step{
				{0, 75, Pending, ""},
				{30 * time.Second, 76, Firing, Fired},
				{40 * time.Second, 77, Firing, ""},
				{50 * time.Second, 65, Inactive, Resolved},
			},
		},
		{
			name: "For=0 fires immediately",
			rule: overheat(0),
			steps: []step{
				{0, 71, Firing, Fired},
				{time.Second, 69, Inactive, Resolved},
			},
		},
		{
			name:  "threshold equality does not breach strict operator",
			rule:  overheat(0),
			steps: []step{{0, 70, Inactive, ""}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEvaluator()
			e.SetRules([]Rule{tt.rule}, t0)
			for i, s := range tt.steps {
				events := e.Observe(reading("cnc-02", s.at, s.temp))
				if got := e.StateOf(tt.rule.ID, "cnc-02"); got != s.wantState {
					t.Fatalf("step %d: state = %s, want %s", i, got, s.wantState)
				}
				switch {
				case s.wantEvent == "" && len(events) != 0:
					t.Fatalf("step %d: unexpected events %+v", i, events)
				case s.wantEvent != "" && (len(events) != 1 || events[0].Kind != s.wantEvent):
					t.Fatalf("step %d: events = %+v, want one %s", i, events, s.wantEvent)
				}
			}
		})
	}
}

func TestMachineScopingAndIsolation(t *testing.T) {
	scoped := overheat(0)
	scoped.Machine = "cnc-01"
	e := NewEvaluator()
	e.SetRules([]Rule{scoped}, t0)

	if ev := e.Observe(reading("cnc-02", 0, 99)); len(ev) != 0 {
		t.Fatalf("rule scoped to cnc-01 fired for cnc-02: %+v", ev)
	}
	other := reading("cnc-01", 0, 99)
	other.Tenant = "nordwerk"
	if ev := e.Observe(other); len(ev) != 0 {
		t.Fatalf("rule of tenant acme fired for another tenant: %+v", ev)
	}
	if ev := e.Observe(reading("cnc-01", 0, 99)); len(ev) != 1 {
		t.Fatalf("expected rule to fire for cnc-01, got %+v", ev)
	}
}

func TestReloadKeepsStateAndResolvesRemovedRules(t *testing.T) {
	e := NewEvaluator()
	r := overheat(0)
	e.SetRules([]Rule{r}, t0)
	e.Observe(reading("cnc-02", 0, 80))

	if ev := e.SetRules([]Rule{r}, t0); len(ev) != 0 {
		t.Fatalf("reload with same rules emitted %+v", ev)
	}
	if e.StateOf(1, "cnc-02") != Firing {
		t.Fatal("reload lost firing state")
	}
	ev := e.SetRules(nil, t0.Add(time.Minute))
	if len(ev) != 1 || ev[0].Kind != Resolved || ev[0].Machine != "cnc-02" {
		t.Fatalf("removing a firing rule should resolve it, got %+v", ev)
	}
}

func TestValidate(t *testing.T) {
	good := overheat(time.Minute)
	if err := good.Validate(); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	bad := []func(*Rule){
		func(r *Rule) { r.Name = "" },
		func(r *Rule) { r.Operator = "==" },
		func(r *Rule) { r.Severity = Severity("panic") },
		func(r *Rule) { r.Machine = "Bad Machine" },
		func(r *Rule) { r.For = -time.Second },
	}
	for i, mutate := range bad {
		r := good
		mutate(&r)
		if r.Validate() == nil {
			t.Errorf("case %d: invalid rule accepted: %+v", i, r)
		}
	}
}

func BenchmarkObserve(b *testing.B) {
	e := NewEvaluator()
	var rs []Rule
	for i := 0; i < 50; i++ {
		r := overheat(time.Minute)
		r.ID = int64(i)
		rs = append(rs, r)
	}
	e.SetRules(rs, t0)
	rd := reading("cnc-01", 0, 72)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rd.Timestamp = t0.Add(time.Duration(i) * time.Second).UnixMilli()
		e.Observe(rd)
	}
}
