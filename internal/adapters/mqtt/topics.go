package mqtt

import (
	"fmt"
	"strings"

	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Topic layout, shared by every service and by the Python analytics service:
//
//	plantorq/{tenant}/{machine}/telemetry   raw readings from an edge agent
//	plantorq/{tenant}/{machine}/analytics   derived values (anomaly scores)
//	plantorq/{tenant}/alerts                alert lifecycle events
//	plantorq/{tenant}/gateways/{gateway}    edge agent health reports
const (
	root = "plantorq"

	kindTelemetry = "telemetry"
	kindAnalytics = "analytics"

	SubAllTelemetry = root + "/+/+/" + kindTelemetry
	SubAllAnalytics = root + "/+/+/" + kindAnalytics
	SubAllAlerts    = root + "/+/alerts"
	SubAllGateways  = root + "/+/gateways/+"
)

func TelemetryTopic(tenant, machine string) string {
	return fmt.Sprintf("%s/%s/%s/%s", root, tenant, machine, kindTelemetry)
}

func AlertsTopic(tenant string) string { return fmt.Sprintf("%s/%s/alerts", root, tenant) }

func GatewayTopic(tenant, gateway string) string {
	return fmt.Sprintf("%s/%s/gateways/%s", root, tenant, gateway)
}

// ParseReadingTopic extracts tenant and machine from a telemetry or analytics
// topic. Payload fields are never trusted for routing: the topic, which the
// broker ACL can restrict per tenant, is the source of truth.
func ParseReadingTopic(topic string) (tenant, machine string, ok bool) {
	parts := strings.Split(topic, "/")
	if len(parts) != 4 || parts[0] != root {
		return "", "", false
	}
	if parts[3] != kindTelemetry && parts[3] != kindAnalytics {
		return "", "", false
	}
	if !telemetry.ValidSlug(parts[1]) || !telemetry.ValidSlug(parts[2]) {
		return "", "", false
	}
	return parts[1], parts[2], true
}
