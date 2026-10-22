// Package http is the driving adapter that exposes the platform use cases as
// a REST + WebSocket API. Handlers stay thin: decode, call a use case, encode.
// Business rules, authorisation and tenant scoping live in app/platform.
package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/OubeidHanini/plantorq/internal/app/platform"
	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// The interfaces below are declared by the consumer (Go idiom): the HTTP
// layer states exactly what it needs, which keeps handlers testable with tiny
// fakes and decoupled from the concrete services.

type Authenticator interface {
	Login(ctx context.Context, email, password string) (platform.Session, error)
}

type TokenVerifier interface {
	Verify(token string) (identity.Principal, error)
}

type Fleet interface {
	Machines(ctx context.Context, p identity.Principal) ([]platform.MachineView, error)
	History(ctx context.Context, p identity.Principal, machine, metric string, rng time.Duration) ([]telemetry.Sample, error)
	Gateways(p identity.Principal) []telemetry.GatewayStatus
}

type Rules interface {
	List(ctx context.Context, p identity.Principal) ([]alerting.Rule, error)
	Create(ctx context.Context, p identity.Principal, r alerting.Rule) (alerting.Rule, error)
	Update(ctx context.Context, p identity.Principal, r alerting.Rule) (alerting.Rule, error)
	Delete(ctx context.Context, p identity.Principal, id int64) error
}

type Alerts interface {
	List(ctx context.Context, p identity.Principal, f platform.AlertFilter) ([]alerting.Alert, error)
	Acknowledge(ctx context.Context, p identity.Principal, id int64) (alerting.Alert, error)
}

type Deps struct {
	Auth    Authenticator
	Tokens  TokenVerifier
	Fleet   Fleet
	Rules   Rules
	Alerts  Alerts
	Hub     *Hub
	Log     *slog.Logger
	Origins []string // allowed CORS / WebSocket origins; empty = same-origin only
}

type Server struct{ Deps }

func NewServer(d Deps) *Server { return &Server{Deps: d} }

// Routes builds the HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, s.recoverer, s.logRequests, instrument, s.cors)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.login)
		r.Get("/ws", s.authenticate(s.websocket))

		r.Group(func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler { return s.authenticate(next.ServeHTTP) })
			r.Get("/me", s.me)
			r.Get("/machines", s.listMachines)
			r.Get("/machines/{machine}/history", s.machineHistory)
			r.Get("/gateways", s.listGateways)
			r.Get("/alerts", s.listAlerts)
			r.Post("/alerts/{id}/ack", s.ackAlert)
			r.Get("/rules", s.listRules)
			r.Post("/rules", s.createRule)
			r.Put("/rules/{id}", s.updateRule)
			r.Delete("/rules/{id}", s.deleteRule)
		})
	})
	return r
}
