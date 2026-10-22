package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	appalerting "github.com/OubeidHanini/plantorq/internal/app/alerting"
	"github.com/OubeidHanini/plantorq/internal/app/platform"
	"github.com/OubeidHanini/plantorq/internal/domain/alerting"
	"github.com/OubeidHanini/plantorq/internal/domain/identity"
	"github.com/OubeidHanini/plantorq/internal/domain/telemetry"
)

// Compile-time proof that DB satisfies every port it is wired into.
var (
	_ platform.UserRepository     = (*DB)(nil)
	_ platform.MachineRepository  = (*DB)(nil)
	_ platform.RuleRepository     = (*DB)(nil)
	_ platform.AlertRepository    = (*DB)(nil)
	_ appalerting.RuleSource      = (*DB)(nil)
	_ appalerting.AlertRepository = (*DB)(nil)
)

// --- users -----------------------------------------------------------------

func (db *DB) UserByEmail(ctx context.Context, email string) (identity.User, error) {
	var u identity.User
	err := db.pool.QueryRow(ctx, `
		SELECT u.id, u.tenant_id, t.slug, u.email, u.name, u.role, u.password_hash
		FROM users u JOIN tenants t ON t.id = u.tenant_id
		WHERE u.email = $1`, email).
		Scan(&u.ID, &u.TenantID, &u.TenantSlug, &u.Email, &u.Name, &u.Role, &u.PasswordHash)
	return u, notFound(err)
}

// --- machines --------------------------------------------------------------

func (db *DB) ListMachines(ctx context.Context, tenantID int64) ([]telemetry.Machine, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, tenant_id, slug, name, kind, protocol FROM machines
		WHERE tenant_id = $1 ORDER BY slug`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (telemetry.Machine, error) {
		var m telemetry.Machine
		err := row.Scan(&m.ID, &m.TenantID, &m.Slug, &m.Name, &m.Kind, &m.Protocol)
		return m, err
	})
}

func (db *DB) MachineExists(ctx context.Context, tenantID int64, slug string) (bool, error) {
	var ok bool
	err := db.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM machines WHERE tenant_id = $1 AND slug = $2)`, tenantID, slug).Scan(&ok)
	return ok, err
}

// --- rules -----------------------------------------------------------------

const ruleColumns = `r.id, r.tenant_id, t.slug, r.name, r.machine_slug, r.metric, r.operator,
	r.threshold, r.for_seconds, r.severity, r.enabled`

func scanRule(row pgx.Row) (alerting.Rule, error) {
	var r alerting.Rule
	var forSeconds int
	err := row.Scan(&r.ID, &r.TenantID, &r.Tenant, &r.Name, &r.Machine, &r.Metric, &r.Operator,
		&r.Threshold, &forSeconds, &r.Severity, &r.Enabled)
	r.For = time.Duration(forSeconds) * time.Second
	return r, err
}

func (db *DB) queryRules(ctx context.Context, where string, args ...any) ([]alerting.Rule, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+ruleColumns+`
		FROM alert_rules r JOIN tenants t ON t.id = r.tenant_id `+where+` ORDER BY r.id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (alerting.Rule, error) { return scanRule(row) })
}

func (db *DB) ListRules(ctx context.Context, tenantID int64) ([]alerting.Rule, error) {
	return db.queryRules(ctx, `WHERE r.tenant_id = $1`, tenantID)
}

// EnabledRules returns the rules of all tenants, for the rules engine.
func (db *DB) EnabledRules(ctx context.Context) ([]alerting.Rule, error) {
	return db.queryRules(ctx, `WHERE r.enabled`)
}

func (db *DB) CreateRule(ctx context.Context, r alerting.Rule) (alerting.Rule, error) {
	return scanRule(db.pool.QueryRow(ctx, `
		WITH r AS (
			INSERT INTO alert_rules (tenant_id, name, machine_slug, metric, operator, threshold, for_seconds, severity, enabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *
		)
		SELECT `+ruleColumns+` FROM r JOIN tenants t ON t.id = r.tenant_id`,
		r.TenantID, r.Name, r.Machine, r.Metric, r.Operator, r.Threshold, int(r.For.Seconds()), r.Severity, r.Enabled))
}

// UpdateRule only matches rules of the caller's tenant: a guessed id from
// another tenant behaves exactly like a missing one.
func (db *DB) UpdateRule(ctx context.Context, r alerting.Rule) (alerting.Rule, error) {
	rule, err := scanRule(db.pool.QueryRow(ctx, `
		WITH r AS (
			UPDATE alert_rules SET name=$3, machine_slug=$4, metric=$5, operator=$6, threshold=$7,
				for_seconds=$8, severity=$9, enabled=$10, updated_at=now()
			WHERE id = $1 AND tenant_id = $2 RETURNING *
		)
		SELECT `+ruleColumns+` FROM r JOIN tenants t ON t.id = r.tenant_id`,
		r.ID, r.TenantID, r.Name, r.Machine, r.Metric, r.Operator, r.Threshold, int(r.For.Seconds()), r.Severity, r.Enabled))
	return rule, notFound(err)
}

func (db *DB) DeleteRule(ctx context.Context, tenantID, id int64) error {
	tag, err := db.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return platform.ErrNotFound
	}
	return nil
}

// --- alerts ----------------------------------------------------------------

const alertColumns = `a.id, a.tenant_id, t.slug, COALESCE(a.rule_id, 0), a.rule_name, a.machine_slug, a.metric,
	a.operator, a.threshold, a.value, a.severity, a.started_at, a.resolved_at, a.acked_by, a.acked_at`

func scanAlert(row pgx.Row) (alerting.Alert, error) {
	var a alerting.Alert
	err := row.Scan(&a.ID, &a.TenantID, &a.Tenant, &a.RuleID, &a.RuleName, &a.Machine, &a.Metric,
		&a.Operator, &a.Threshold, &a.Value, &a.Severity, &a.StartedAt, &a.ResolvedAt, &a.AckedBy, &a.AckedAt)
	return a, err
}

func (db *DB) ListAlerts(ctx context.Context, tenantID int64, f platform.AlertFilter) ([]alerting.Alert, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+alertColumns+`
		FROM alerts a JOIN tenants t ON t.id = a.tenant_id
		WHERE a.tenant_id = $1 AND ($2 = false OR a.resolved_at IS NULL)
		ORDER BY a.started_at DESC LIMIT $3`, tenantID, f.OpenOnly, f.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (alerting.Alert, error) { return scanAlert(row) })
}

func (db *DB) Acknowledge(ctx context.Context, tenantID, alertID, userID int64, at time.Time) (alerting.Alert, error) {
	a, err := scanAlert(db.pool.QueryRow(ctx, `
		WITH a AS (
			UPDATE alerts SET acked_by = $3, acked_at = $4
			WHERE id = $1 AND tenant_id = $2 RETURNING *
		)
		SELECT `+alertColumns+` FROM a JOIN tenants t ON t.id = a.tenant_id`, alertID, tenantID, userID, at))
	return a, notFound(err)
}

// Open inserts a firing alert. The partial unique index turns a duplicate
// into a no-op, in which case the already-open alert is returned.
func (db *DB) Open(ctx context.Context, in alerting.Alert) (alerting.Alert, bool, error) {
	a, err := scanAlert(db.pool.QueryRow(ctx, `
		WITH a AS (
			INSERT INTO alerts (tenant_id, rule_id, rule_name, machine_slug, metric, operator, threshold, value, severity, started_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (rule_id, machine_slug) WHERE resolved_at IS NULL DO NOTHING
			RETURNING *
		)
		SELECT `+alertColumns+` FROM a JOIN tenants t ON t.id = a.tenant_id`,
		in.TenantID, in.RuleID, in.RuleName, in.Machine, in.Metric, in.Operator, in.Threshold, in.Value, in.Severity, in.StartedAt))
	if err == nil {
		return a, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return alerting.Alert{}, false, err
	}
	existing, err := scanAlert(db.pool.QueryRow(ctx, `SELECT `+alertColumns+`
		FROM alerts a JOIN tenants t ON t.id = a.tenant_id
		WHERE a.rule_id = $1 AND a.machine_slug = $2 AND a.resolved_at IS NULL`, in.RuleID, in.Machine))
	return existing, false, err
}

func (db *DB) Resolve(ctx context.Context, ruleID int64, machine string, at time.Time) (alerting.Alert, bool, error) {
	a, err := scanAlert(db.pool.QueryRow(ctx, `
		WITH a AS (
			UPDATE alerts SET resolved_at = $3
			WHERE rule_id = $1 AND machine_slug = $2 AND resolved_at IS NULL RETURNING *
		)
		SELECT `+alertColumns+` FROM a JOIN tenants t ON t.id = a.tenant_id`, ruleID, machine, at))
	if errors.Is(err, pgx.ErrNoRows) {
		return alerting.Alert{}, false, nil
	}
	return a, err == nil, err
}
