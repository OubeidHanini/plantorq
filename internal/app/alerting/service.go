// Package alerting is the rules-engine use case: evaluate incoming readings
// against tenant rules, persist alert lifecycle changes and notify listeners.
package alerting

import (
	"context"
	"log/slog"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// RuleSource provides the enabled rules of every tenant.
type RuleSource interface {
	EnabledRules(ctx context.Context) ([]alerting.Rule, error)
}

// AlertRepository persists alerts. Open must be idempotent: opening an alert
// that is already open returns the existing one with created=false.
type AlertRepository interface {
	Open(ctx context.Context, a alerting.Alert) (alerting.Alert, bool, error)
	Resolve(ctx context.Context, ruleID int64, machine string, at time.Time) (alerting.Alert, bool, error)
}

// Notifier broadcasts alert changes (MQTT today; e-mail or webhooks would be
// additional implementations composed behind the same port).
type Notifier interface {
	Notify(ctx context.Context, a alerting.Alert) error
}

// Metrics is the observability port of the rules engine.
type Metrics interface {
	ReadingEvaluated()
	AlertChanged(kind alerting.EventKind, severity alerting.Severity)
}

type NopMetrics struct{}

func (NopMetrics) ReadingEvaluated()                                  {}
func (NopMetrics) AlertChanged(alerting.EventKind, alerting.Severity) {}

type Service struct {
	eval     *alerting.Evaluator
	rules    RuleSource
	alerts   AlertRepository
	notifier Notifier
	metrics  Metrics
	log      *slog.Logger
	readings chan telemetry.Reading
}

func NewService(rules RuleSource, alerts AlertRepository, notifier Notifier, metrics Metrics, log *slog.Logger) *Service {
	if metrics == nil {
		metrics = NopMetrics{}
	}
	return &Service{
		eval: alerting.NewEvaluator(), rules: rules, alerts: alerts, notifier: notifier, metrics: metrics, log: log,
		readings: make(chan telemetry.Reading, 4096),
	}
}

// Accept queues a reading for evaluation without blocking the caller.
func (s *Service) Accept(r telemetry.Reading) bool {
	select {
	case s.readings <- r:
		return true
	default:
		s.log.Warn("evaluation queue full, reading skipped", "machine", r.Machine)
		return false
	}
}

// Run reloads rules every reloadEvery and evaluates readings on a single
// goroutine, which keeps event ordering per machine trivially correct.
func (s *Service) Run(ctx context.Context, reloadEvery time.Duration) error {
	if err := s.ReloadRules(ctx); err != nil {
		return err
	}
	t := time.NewTicker(reloadEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := s.ReloadRules(ctx); err != nil {
				s.log.Error("rule reload failed, keeping previous set", "err", err)
			}
		case r := <-s.readings:
			s.metrics.ReadingEvaluated()
			s.handle(ctx, s.eval.Observe(r))
		}
	}
}

// ReloadRules fetches the current rule set and applies it.
func (s *Service) ReloadRules(ctx context.Context) error {
	rules, err := s.rules.EnabledRules(ctx)
	if err != nil {
		return err
	}
	s.handle(ctx, s.eval.SetRules(rules, time.Now()))
	return nil
}

// Evaluate processes one reading synchronously (used by tests).
func (s *Service) Evaluate(ctx context.Context, r telemetry.Reading) {
	s.handle(ctx, s.eval.Observe(r))
}

func (s *Service) handle(ctx context.Context, events []alerting.Event) {
	for _, ev := range events {
		var (
			a       alerting.Alert
			changed bool
			err     error
		)
		switch ev.Kind {
		case alerting.Fired:
			a, changed, err = s.alerts.Open(ctx, alerting.NewAlert(ev))
		case alerting.Resolved:
			a, changed, err = s.alerts.Resolve(ctx, ev.Rule.ID, ev.Machine, ev.At)
		}
		if err != nil {
			s.log.Error("persisting alert failed", "kind", ev.Kind, "rule", ev.Rule.ID, "machine", ev.Machine, "err", err)
			continue
		}
		if !changed {
			continue
		}
		s.metrics.AlertChanged(ev.Kind, a.Severity)
		s.log.Info("alert "+string(ev.Kind), "alert", a.ID, "rule", a.RuleName, "machine", a.Machine, "value", a.Value)
		if err := s.notifier.Notify(ctx, a); err != nil {
			s.log.Warn("alert notification failed", "alert", a.ID, "err", err)
		}
	}
}
