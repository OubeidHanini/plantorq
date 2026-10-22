// Package alerting is the alerting domain: rules, alerts and the state
// machine that turns a stream of readings into alert lifecycle events.
package alerting

import (
	"errors"
	"fmt"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Operator is the comparison a rule applies (a Strategy selected by value).
type Operator string

const (
	GT  Operator = ">"
	GTE Operator = ">="
	LT  Operator = "<"
	LTE Operator = "<="
)

var comparators = map[Operator]func(v, t float64) bool{
	GT:  func(v, t float64) bool { return v > t },
	GTE: func(v, t float64) bool { return v >= t },
	LT:  func(v, t float64) bool { return v < t },
	LTE: func(v, t float64) bool { return v <= t },
}

func (o Operator) Valid() bool { _, ok := comparators[o]; return ok }

// Breached reports whether value violates threshold under o.
func (o Operator) Breached(value, threshold float64) bool {
	if cmp, ok := comparators[o]; ok {
		return cmp(value, threshold)
	}
	return false
}

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

func (s Severity) Valid() bool {
	return s == SeverityInfo || s == SeverityWarning || s == SeverityCritical
}

// Rule raises an alert when Metric stays beyond Threshold for at least For.
type Rule struct {
	ID        int64         `json:"id"`
	TenantID  int64         `json:"-"`
	Tenant    string        `json:"tenant"`
	Name      string        `json:"name"`
	Machine   string        `json:"machine"` // empty = every machine of the tenant
	Metric    string        `json:"metric"`
	Operator  Operator      `json:"operator"`
	Threshold float64       `json:"threshold"`
	For       time.Duration `json:"-"`
	Severity  Severity      `json:"severity"`
	Enabled   bool          `json:"enabled"`
}

// ErrInvalidRule wraps every validation failure so callers can map it to a
// client error without string matching.
var ErrInvalidRule = errors.New("invalid rule")

// Validate checks the invariants of a rule.
func (r Rule) Validate() error {
	var reason string
	switch {
	case r.Name == "" || len(r.Name) > 120:
		reason = "name must be 1-120 characters"
	case r.Metric == "" || len(r.Metric) > 64:
		reason = "metric must be 1-64 characters"
	case r.Machine != "" && !telemetry.ValidSlug(r.Machine):
		reason = fmt.Sprintf("machine %q is not a valid id", r.Machine)
	case !r.Operator.Valid():
		reason = "operator must be one of > >= < <="
	case !r.Severity.Valid():
		reason = "severity must be info, warning or critical"
	case r.For < 0 || r.For > 24*time.Hour:
		reason = "for must be between 0s and 24h"
	default:
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidRule, reason)
}

// AppliesTo reports whether the rule watches the given machine.
func (r Rule) AppliesTo(machine string) bool { return r.Machine == "" || r.Machine == machine }
