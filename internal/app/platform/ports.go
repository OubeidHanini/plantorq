// Package platform holds the use cases behind the public API. Every method
// takes the caller's identity.Principal and scopes data access by its
// tenant, so tenant isolation and authorisation live in one place instead of
// being re-implemented (or forgotten) in each HTTP handler.
package platform

import (
	"context"
	"errors"
	"time"

	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Errors returned by use cases; the transport layer maps them to status codes.
var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrUnauthorized = errors.New("invalid credentials")
	ErrInvalid      = errors.New("invalid input")
)

type UserRepository interface {
	UserByEmail(ctx context.Context, email string) (identity.User, error)
}

type MachineRepository interface {
	ListMachines(ctx context.Context, tenantID int64) ([]telemetry.Machine, error)
	MachineExists(ctx context.Context, tenantID int64, slug string) (bool, error)
}

type RuleRepository interface {
	ListRules(ctx context.Context, tenantID int64) ([]alerting.Rule, error)
	CreateRule(ctx context.Context, r alerting.Rule) (alerting.Rule, error)
	UpdateRule(ctx context.Context, r alerting.Rule) (alerting.Rule, error)
	DeleteRule(ctx context.Context, tenantID, id int64) error
}

type AlertFilter struct {
	OpenOnly bool
	Limit    int
}

type AlertRepository interface {
	ListAlerts(ctx context.Context, tenantID int64, f AlertFilter) ([]alerting.Alert, error)
	Acknowledge(ctx context.Context, tenantID, alertID, userID int64, at time.Time) (alerting.Alert, error)
}

type HistoryReader interface {
	History(ctx context.Context, tenant, machine, metric string, rng, step time.Duration) ([]telemetry.Sample, error)
}

type PasswordVerifier interface {
	Verify(hash, plain string) bool
}

type TokenIssuer interface {
	Issue(p identity.Principal) (token string, expires time.Time, err error)
}

// require returns ErrForbidden unless p holds at least role.
func require(p identity.Principal, role identity.Role) error {
	if !p.Role.Can(role) {
		return ErrForbidden
	}
	return nil
}
