package alerting

import "time"

// Alert is a persisted occurrence of a rule firing for one machine. Rule
// fields are copied in so history stays readable after the rule changes.
type Alert struct {
	ID         int64      `json:"id"`
	TenantID   int64      `json:"-"`
	Tenant     string     `json:"tenant"`
	RuleID     int64      `json:"rule_id"`
	RuleName   string     `json:"rule_name"`
	Machine    string     `json:"machine"`
	Metric     string     `json:"metric"`
	Operator   Operator   `json:"operator"`
	Threshold  float64    `json:"threshold"`
	Value      float64    `json:"value"`
	Severity   Severity   `json:"severity"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	AckedBy    *int64     `json:"acked_by,omitempty"`
	AckedAt    *time.Time `json:"acked_at,omitempty"`
}

// Open reports whether the alert is still active.
func (a Alert) Open() bool { return a.ResolvedAt == nil }

// State is the lifecycle label shown to users and sent to subscribers.
func (a Alert) State() string {
	if a.Open() {
		return string(Fired)
	}
	return string(Resolved)
}

// NewAlert builds the alert to persist when a rule starts firing.
func NewAlert(ev Event) Alert {
	return Alert{
		TenantID:  ev.Rule.TenantID,
		Tenant:    ev.Rule.Tenant,
		RuleID:    ev.Rule.ID,
		RuleName:  ev.Rule.Name,
		Machine:   ev.Machine,
		Metric:    ev.Rule.Metric,
		Operator:  ev.Rule.Operator,
		Threshold: ev.Rule.Threshold,
		Value:     ev.Value,
		Severity:  ev.Rule.Severity,
		StartedAt: ev.Since,
	}
}
