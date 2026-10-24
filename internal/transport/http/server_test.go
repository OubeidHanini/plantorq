package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OubeidHanini/plantorq/internal/app/platform"
	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

var operator = identity.Principal{UserID: 1, TenantID: 10, TenantSlug: "acme", Role: identity.RoleOperator}

type fakeAuth struct{}

func (fakeAuth) Login(_ context.Context, email, pw string) (platform.Session, error) {
	if pw != "good" {
		return platform.Session{}, platform.ErrUnauthorized
	}
	return platform.Session{Token: "valid", User: operator}, nil
}

type fakeTokens struct{}

func (fakeTokens) Verify(tok string) (identity.Principal, error) {
	if tok == "valid" {
		return operator, nil
	}
	return identity.Principal{}, errors.New("bad token")
}

type fakeFleet struct{}

func (fakeFleet) Machines(context.Context, identity.Principal) ([]platform.MachineView, error) {
	return []platform.MachineView{{Machine: telemetry.Machine{Slug: "cnc-01"}, Status: platform.StatusRunning}}, nil
}
func (fakeFleet) History(context.Context, identity.Principal, string, string, time.Duration) ([]telemetry.Sample, error) {
	return nil, platform.ErrNotFound
}
func (fakeFleet) Gateways(identity.Principal) []telemetry.GatewayStatus { return nil }

type fakeRules struct{ got alerting.Rule }

func (f *fakeRules) List(context.Context, identity.Principal) ([]alerting.Rule, error) {
	return nil, nil
}
func (f *fakeRules) Create(_ context.Context, p identity.Principal, r alerting.Rule) (alerting.Rule, error) {
	f.got = r
	if !p.Role.Can(identity.RoleAdmin) {
		return alerting.Rule{}, platform.ErrForbidden
	}
	return r, nil
}
func (f *fakeRules) Update(context.Context, identity.Principal, alerting.Rule) (alerting.Rule, error) {
	return alerting.Rule{}, nil
}
func (f *fakeRules) Delete(context.Context, identity.Principal, int64) error { return nil }

type fakeAlerts struct{}

func (fakeAlerts) List(context.Context, identity.Principal, platform.AlertFilter) ([]alerting.Alert, error) {
	return nil, errors.New("db exploded: secret connection string")
}
func (fakeAlerts) Acknowledge(context.Context, identity.Principal, int64) (alerting.Alert, error) {
	return alerting.Alert{}, nil
}

func newTestServer() (*httptest.Server, *fakeRules) {
	rules := &fakeRules{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewServer(Deps{Auth: fakeAuth{}, Tokens: fakeTokens{}, Fleet: fakeFleet{}, Rules: rules,
		Alerts: fakeAlerts{}, Hub: NewHub(log), Log: log})
	return httptest.NewServer(s.Routes()), rules
}

// do performs a request and returns the status code and the response body.
func do(t *testing.T, srv *httptest.Server, method, path, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func TestStatusMapping(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()

	tests := []struct {
		name, method, path, token, body string
		want                            int
	}{
		{"login ok", "POST", "/api/v1/auth/login", "", `{"email":"a","password":"good"}`, 200},
		{"login bad password", "POST", "/api/v1/auth/login", "", `{"email":"a","password":"bad"}`, 401},
		{"login unknown field", "POST", "/api/v1/auth/login", "", `{"email":"a","pass":"x"}`, 400},
		{"no token", "GET", "/api/v1/machines", "", "", 401},
		{"bad token", "GET", "/api/v1/machines", "forged", "", 401},
		{"machines", "GET", "/api/v1/machines", "valid", "", 200},
		{"history not found", "GET", "/api/v1/machines/ghost/history", "valid", "", 404},
		{"operator cannot create rule", "POST", "/api/v1/rules", "valid", `{"name":"x"}`, 403},
		{"invalid id", "DELETE", "/api/v1/rules/abc", "valid", "", 400},
		{"internal error", "GET", "/api/v1/alerts", "valid", "", 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := do(t, srv, tt.method, tt.path, tt.token, tt.body)
			if status != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", status, tt.want, body)
			}
		})
	}
}

func TestInternalErrorsDoNotLeak(t *testing.T) {
	srv, _ := newTestServer()
	defer srv.Close()
	_, body := do(t, srv, "GET", "/api/v1/alerts", "valid", "")
	if strings.Contains(body, "secret") {
		t.Fatalf("internal error leaked to client: %s", body)
	}
}

func TestRuleDTOMapping(t *testing.T) {
	srv, rules := newTestServer()
	defer srv.Close()
	do(t, srv, "POST", "/api/v1/rules", "valid",
		`{"name":"Hot","metric":"temperature","operator":">","threshold":70,"for_seconds":30,"severity":"critical","enabled":true}`)
	if rules.got.For != 30*time.Second || rules.got.Operator != alerting.GT {
		t.Fatalf("DTO not mapped to domain: %+v", rules.got)
	}
}

func TestHubIsolatesTenants(t *testing.T) {
	h := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
	acme := &wsClient{tenant: "acme", send: make(chan []byte, 1)}
	other := &wsClient{tenant: "nordwerk", send: make(chan []byte, 1)}
	h.add(acme)
	h.add(other)

	h.Broadcast("acme", Event{Type: "alert", Data: map[string]int{"id": 1}})
	h.Broadcast("acme", Event{Type: "alert"}) // buffer full: dropped, must not block

	var ev Event
	if err := json.Unmarshal(<-acme.send, &ev); err != nil || ev.Type != "alert" {
		t.Fatalf("acme client got %+v, %v", ev, err)
	}
	select {
	case msg := <-other.send:
		t.Fatalf("event leaked to another tenant: %s", msg)
	default:
	}
}
