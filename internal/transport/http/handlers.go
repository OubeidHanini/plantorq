package http

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/OubeidHanini/plantorq/internal/app/platform"
	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		s.writeError(w, r, err)
		return
	}
	sess, err := s.Auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, principalFrom(r.Context()))
}

func (s *Server) listMachines(w http.ResponseWriter, r *http.Request) {
	machines, err := s.Fleet.Machines(r.Context(), principalFrom(r.Context()))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, machines)
}

func (s *Server) machineHistory(w http.ResponseWriter, r *http.Request) {
	rng, err := time.ParseDuration(r.URL.Query().Get("range"))
	if err != nil {
		rng = time.Hour
	}
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "temperature"
	}
	samples, err := s.Fleet.History(r.Context(), principalFrom(r.Context()), chi.URLParam(r, "machine"), metric, rng)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

func (s *Server) listGateways(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Fleet.Gateways(principalFrom(r.Context())))
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	f := platform.AlertFilter{OpenOnly: r.URL.Query().Get("state") == "open", Limit: limit}
	alerts, err := s.Alerts.List(r.Context(), principalFrom(r.Context()), f)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (s *Server) ackAlert(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	a, err := s.Alerts.Acknowledge(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// ruleDTO is the API representation of a rule. Keeping it separate from the
// domain type lets the wire format (e.g. for_seconds) evolve independently.
type ruleDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Machine    string  `json:"machine"`
	Metric     string  `json:"metric"`
	Operator   string  `json:"operator"`
	Threshold  float64 `json:"threshold"`
	ForSeconds int     `json:"for_seconds"`
	Severity   string  `json:"severity"`
	Enabled    bool    `json:"enabled"`
}

func toRuleDTO(r alerting.Rule) ruleDTO {
	return ruleDTO{ID: r.ID, Name: r.Name, Machine: r.Machine, Metric: r.Metric, Operator: string(r.Operator),
		Threshold: r.Threshold, ForSeconds: int(r.For.Seconds()), Severity: string(r.Severity), Enabled: r.Enabled}
}

func (d ruleDTO) toDomain() alerting.Rule {
	return alerting.Rule{ID: d.ID, Name: d.Name, Machine: d.Machine, Metric: d.Metric,
		Operator: alerting.Operator(d.Operator), Threshold: d.Threshold,
		For: time.Duration(d.ForSeconds) * time.Second, Severity: alerting.Severity(d.Severity), Enabled: d.Enabled}
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.Rules.List(r.Context(), principalFrom(r.Context()))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	out := make([]ruleDTO, len(rules))
	for i, rule := range rules {
		out[i] = toRuleDTO(rule)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var dto ruleDTO
	if err := decode(r, &dto); err != nil {
		s.writeError(w, r, err)
		return
	}
	rule, err := s.Rules.Create(r.Context(), principalFrom(r.Context()), dto.toDomain())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRuleDTO(rule))
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var dto ruleDTO
	if err := decode(r, &dto); err != nil {
		s.writeError(w, r, err)
		return
	}
	dto.ID = id
	rule, err := s.Rules.Update(r.Context(), principalFrom(r.Context()), dto.toDomain())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRuleDTO(rule))
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.Rules.Delete(r.Context(), principalFrom(r.Context()), id); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: id must be a positive integer", platform.ErrInvalid)
	}
	return id, nil
}
