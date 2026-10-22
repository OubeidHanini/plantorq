package platform

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// --- fakes -----------------------------------------------------------------

type fakeUsers map[string]identity.User

func (f fakeUsers) UserByEmail(_ context.Context, email string) (identity.User, error) {
	if u, ok := f[email]; ok {
		return u, nil
	}
	return identity.User{}, ErrNotFound
}

type plainPasswords struct{}

func (plainPasswords) Verify(hash, plain string) bool { return hash == "hash:"+plain }

type fakeTokens struct{}

func (fakeTokens) Issue(p identity.Principal) (string, time.Time, error) {
	return "token-for-" + p.Email, time.Now().Add(time.Hour), nil
}

type fakeMachines []telemetry.Machine

func (f fakeMachines) ListMachines(_ context.Context, tenantID int64) ([]telemetry.Machine, error) {
	var out []telemetry.Machine
	for _, m := range f {
		if m.TenantID == tenantID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f fakeMachines) MachineExists(ctx context.Context, tenantID int64, slug string) (bool, error) {
	ms, _ := f.ListMachines(ctx, tenantID)
	for _, m := range ms {
		if m.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}

type fakeRules struct{ created []alerting.Rule }

func (f *fakeRules) ListRules(context.Context, int64) ([]alerting.Rule, error) { return f.created, nil }
func (f *fakeRules) CreateRule(_ context.Context, r alerting.Rule) (alerting.Rule, error) {
	r.ID = int64(len(f.created) + 1)
	f.created = append(f.created, r)
	return r, nil
}
func (f *fakeRules) UpdateRule(_ context.Context, r alerting.Rule) (alerting.Rule, error) {
	return r, nil
}
func (f *fakeRules) DeleteRule(context.Context, int64, int64) error { return nil }

type fakeAlerts []alerting.Alert

func (f fakeAlerts) ListAlerts(_ context.Context, tenantID int64, _ AlertFilter) ([]alerting.Alert, error) {
	var out []alerting.Alert
	for _, a := range f {
		if a.TenantID == tenantID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f fakeAlerts) Acknowledge(context.Context, int64, int64, int64, time.Time) (alerting.Alert, error) {
	return alerting.Alert{}, nil
}

var (
	admin  = identity.Principal{UserID: 1, TenantID: 10, TenantSlug: "acme", Role: identity.RoleAdmin}
	viewer = identity.Principal{UserID: 2, TenantID: 10, TenantSlug: "acme", Role: identity.RoleViewer}
)

// --- auth ------------------------------------------------------------------

func TestLogin(t *testing.T) {
	users := fakeUsers{"ops@acme.io": {ID: 1, TenantID: 10, Email: "ops@acme.io", Role: identity.RoleOperator, PasswordHash: "hash:pw"}}
	s := NewAuthService(users, plainPasswords{}, fakeTokens{})
	ctx := context.Background()

	sess, err := s.Login(ctx, "  OPS@acme.io ", "pw")
	if err != nil || sess.Token != "token-for-ops@acme.io" || sess.User.TenantID != 10 {
		t.Fatalf("login failed: %+v %v", sess, err)
	}
	for _, tc := range []struct{ email, pw string }{{"ops@acme.io", "bad"}, {"ghost@acme.io", "pw"}} {
		if _, err := s.Login(ctx, tc.email, tc.pw); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("Login(%q) err = %v, want ErrUnauthorized", tc.email, err)
		}
	}
}

// --- rules -----------------------------------------------------------------

func validRule() alerting.Rule {
	return alerting.Rule{Name: "Overheat", Metric: "temperature", Operator: alerting.GT,
		Threshold: 70, Severity: alerting.SeverityCritical, Enabled: true, Machine: "cnc-01"}
}

func TestCreateRule(t *testing.T) {
	machines := fakeMachines{{TenantID: 10, Slug: "cnc-01"}, {TenantID: 99, Slug: "foreign"}}
	repo := &fakeRules{}
	s := NewRuleService(repo, machines)
	ctx := context.Background()

	r := validRule()
	r.TenantID, r.Tenant = 99, "evil" // must be ignored
	got, err := s.Create(ctx, admin, r)
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantID != 10 || got.Tenant != "acme" {
		t.Fatalf("tenant not taken from principal: %+v", got)
	}

	if _, err := s.Create(ctx, viewer, validRule()); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer create: err = %v, want ErrForbidden", err)
	}

	bad := validRule()
	bad.Operator = "=="
	if _, err := s.Create(ctx, admin, bad); !errors.Is(err, ErrInvalid) || !errors.Is(err, alerting.ErrInvalidRule) {
		t.Errorf("invalid rule: err = %v, want ErrInvalid wrapping ErrInvalidRule", err)
	}

	foreign := validRule()
	foreign.Machine = "foreign" // exists, but in another tenant
	if _, err := s.Create(ctx, admin, foreign); !errors.Is(err, ErrInvalid) {
		t.Errorf("cross-tenant machine: err = %v, want ErrInvalid", err)
	}
}

// --- fleet -----------------------------------------------------------------

func TestMachinesStatus(t *testing.T) {
	now := time.Now()
	machines := fakeMachines{
		{TenantID: 10, Slug: "cnc-01"}, {TenantID: 10, Slug: "cnc-02"},
		{TenantID: 10, Slug: "comp-c2"}, {TenantID: 10, Slug: "press-p04"},
	}
	alerts := fakeAlerts{
		{TenantID: 10, Machine: "cnc-02", Severity: alerting.SeverityWarning},
		{TenantID: 10, Machine: "cnc-02", Severity: alerting.SeverityCritical},
		{TenantID: 10, Machine: "comp-c2", Severity: alerting.SeverityWarning},
		{TenantID: 99, Machine: "cnc-01", Severity: alerting.SeverityCritical}, // other tenant
	}
	live := NewLiveView()
	for _, m := range []string{"cnc-01", "cnc-02", "comp-c2"} {
		live.ApplyReading(telemetry.Reading{Tenant: "acme", Machine: m, Source: telemetry.SourceEdge,
			Timestamp: now.UnixMilli(), Values: map[string]float64{"temperature": 50}})
	}
	s := NewFleetService(machines, alerts, nil, live, 10*time.Second)
	s.now = func() time.Time { return now }

	views, err := s.Machines(context.Background(), admin)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]MachineStatus{"cnc-01": StatusRunning, "cnc-02": StatusCritical, "comp-c2": StatusWarning, "press-p04": StatusOffline}
	for _, v := range views {
		if v.Status != want[v.Slug] {
			t.Errorf("%s status = %s, want %s", v.Slug, v.Status, want[v.Slug])
		}
	}
}

func TestLiveViewMergesSources(t *testing.T) {
	live := NewLiveView()
	live.ApplyReading(telemetry.Reading{Tenant: "acme", Machine: "m", Source: telemetry.SourceEdge, Timestamp: 2, Values: map[string]float64{"temperature": 60}})
	live.ApplyReading(telemetry.Reading{Tenant: "acme", Machine: "m", Source: telemetry.SourceAnalytics, Timestamp: 3, Values: map[string]float64{"anomaly_score": 0.9}})
	live.ApplyReading(telemetry.Reading{Tenant: "acme", Machine: "m", Source: telemetry.SourceEdge, Timestamp: 1, Values: map[string]float64{}})

	r, ok := live.Latest("acme", "m")
	if !ok || r.Values["temperature"] != 60 || r.Values["anomaly_score"] != 0.9 || r.Timestamp != 2 {
		t.Fatalf("unexpected merged reading %+v", r)
	}
	if _, ok := live.Latest("other", "m"); ok {
		t.Fatal("reading leaked across tenants")
	}
}

func TestHistoryRejectsUnknownMachine(t *testing.T) {
	s := NewFleetService(fakeMachines{}, fakeAlerts{}, nil, NewLiveView(), time.Second)
	if _, err := s.History(context.Background(), admin, "ghost", "temperature", time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
