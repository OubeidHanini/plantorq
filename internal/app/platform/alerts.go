package platform

import (
	"context"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
)

type AlertService struct {
	alerts AlertRepository
	now    func() time.Time
}

func NewAlertService(alerts AlertRepository) *AlertService {
	return &AlertService{alerts: alerts, now: time.Now}
}

func (s *AlertService) List(ctx context.Context, p identity.Principal, f AlertFilter) ([]alerting.Alert, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	return s.alerts.ListAlerts(ctx, p.TenantID, f)
}

// Acknowledge records that an operator has seen the alert.
func (s *AlertService) Acknowledge(ctx context.Context, p identity.Principal, id int64) (alerting.Alert, error) {
	if err := require(p, identity.RoleOperator); err != nil {
		return alerting.Alert{}, err
	}
	return s.alerts.Acknowledge(ctx, p.TenantID, id, p.UserID, s.now())
}
