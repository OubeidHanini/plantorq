package mqtt

import (
	"context"

	"github.com/OubeidHanini/plantorq/internal/app/collector"
	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// TelemetryPublisher implements collector.Publisher on top of a Client.
type TelemetryPublisher struct{ *Client }

var _ collector.Publisher = TelemetryPublisher{}

func (p TelemetryPublisher) PublishReading(ctx context.Context, r telemetry.Reading) error {
	return p.PublishJSON(ctx, TelemetryTopic(r.Tenant, r.Machine), r)
}

func (p TelemetryPublisher) PublishStatus(ctx context.Context, s telemetry.GatewayStatus) error {
	return p.PublishJSON(ctx, GatewayTopic(s.Tenant, s.Gateway), s)
}

// AlertMessage is the wire format of alert events on the bus.
type AlertMessage struct {
	alerting.Alert
	State string `json:"state"`
}

// AlertNotifier implements the rules engine's Notifier port by broadcasting
// alert lifecycle changes; the API relays them to browsers.
type AlertNotifier struct{ *Client }

func (n AlertNotifier) Notify(ctx context.Context, a alerting.Alert) error {
	return n.PublishJSON(ctx, AlertsTopic(a.Tenant), AlertMessage{Alert: a, State: a.State()})
}
