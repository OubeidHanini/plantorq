// Package telemetry holds the core measurement types of Plantorq. It has no
// knowledge of transports or storage: MQTT topics, InfluxDB points and JSON
// wire details live in the adapters.
package telemetry

import (
	"regexp"
	"time"
)

// Source identifies who produced a reading.
const (
	SourceEdge      = "edge"
	SourceAnalytics = "analytics"
)

// Reading is one snapshot of a machine's measurements.
type Reading struct {
	Tenant    string             `json:"tenant"`
	Machine   string             `json:"machine"`
	Gateway   string             `json:"gateway,omitempty"`
	Source    string             `json:"source"`
	Timestamp int64              `json:"ts"` // Unix milliseconds
	Values    map[string]float64 `json:"values"`
}

// Time returns the reading timestamp as a time.Time.
func (r Reading) Time() time.Time { return time.UnixMilli(r.Timestamp) }

// GatewayStatus is the periodic health report of an edge agent.
type GatewayStatus struct {
	Tenant         string  `json:"tenant"`
	Gateway        string  `json:"gateway"`
	Timestamp      int64   `json:"ts"`
	MsgPerSec      float64 `json:"msg_per_sec"`
	Buffered       int     `json:"buffered"`
	PollP95Ms      float64 `json:"poll_p95_ms"`
	MachinesOnline int     `json:"machines_online"`
	MachinesTotal  int     `json:"machines_total"`
	Connected      bool    `json:"connected"`
}

// Sample is one point of a metric's time series.
type Sample struct {
	Time  int64   `json:"t"` // Unix milliseconds
	Value float64 `json:"v"`
}

// Machine is a monitored piece of equipment registered for a tenant.
type Machine struct {
	ID       int64  `json:"-"`
	TenantID int64  `json:"-"`
	Slug     string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Protocol string `json:"protocol"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidSlug reports whether s is usable as a tenant, machine or gateway id.
// Ids end up in topics, tags and URLs, so the alphabet is deliberately strict.
func ValidSlug(s string) bool { return slugRe.MatchString(s) }
