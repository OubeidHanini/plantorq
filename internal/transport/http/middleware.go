package http

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/OubeidHanini/plantorq/internal/domain/identity"
)

type ctxKey struct{}

func principalFrom(ctx context.Context) identity.Principal {
	p, _ := ctx.Value(ctxKey{}).(identity.Principal)
	return p
}

// authenticate verifies the bearer token and stores the principal in the
// request context. Browsers cannot set headers on WebSocket upgrades, so the
// token may also come from the access_token query parameter.
func (s *Server) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || token == r.Header.Get("Authorization") {
			token = r.URL.Query().Get("access_token")
		}
		p, err := s.Tokens.Verify(token)
		if token == "" || err != nil {
			writeJSON(w, http.StatusUnauthorized, errorBody{"authentication required"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		s.Log.Info("http request",
			"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(), "request_id", middleware.GetReqID(r.Context()))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
				s.Log.Error("panic in handler", "panic", rec, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, errorBody{"internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && slices.Contains(s.Origins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "plantorq_api_request_duration_seconds",
	Help: "API request latency by route and status.",
}, []string{"method", "route", "status"})

// instrument records latency per route pattern (not raw path, which would
// explode label cardinality with ids).
func instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		httpDuration.WithLabelValues(r.Method, route, strconv.Itoa(ww.Status())).Observe(time.Since(start).Seconds())
	})
}
