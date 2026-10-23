package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SeedDemo creates a demo tenant, users, machines and rules when the database
// is empty, so `docker compose up` gives a working platform immediately.
// hash is the bcrypt hash of the demo password shared by all demo users.
func (db *DB) SeedDemo(ctx context.Context, hash string) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenants)`).Scan(&exists); err != nil || exists {
			return err
		}
		var tenantID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO tenants (slug, name) VALUES ('acme', 'Acme Metalworks GmbH') RETURNING id`).Scan(&tenantID); err != nil {
			return err
		}

		batch := &pgx.Batch{}
		for _, u := range [][3]string{
			{"admin@acme.io", "Jonas Weber", "admin"},
			{"operator@acme.io", "Lea Schmidt", "operator"},
			{"viewer@acme.io", "Tom Becker", "viewer"},
		} {
			batch.Queue(`INSERT INTO users (tenant_id, email, name, password_hash, role) VALUES ($1, $2, $3, $4, $5)`,
				tenantID, u[0], u[1], hash, u[2])
		}
		for _, m := range [][4]string{
			{"cnc-01", "CNC-01", "CNC mill", "Modbus TCP"},
			{"cnc-02", "CNC-02", "CNC mill", "Modbus TCP"},
			{"robot-07", "Robot R-07", "Welding robot", "Modbus TCP"},
			{"comp-c2", "Compressor C-2", "Air compressor", "Modbus TCP"},
			{"press-p04", "Press P-04", "Hydraulic press", "Modbus TCP"},
		} {
			batch.Queue(`INSERT INTO machines (tenant_id, slug, name, kind, protocol) VALUES ($1, $2, $3, $4, $5)`,
				tenantID, m[0], m[1], m[2], m[3])
		}
		rules := []struct {
			name, machine, metric, op string
			threshold                 float64
			forSeconds                int
			severity                  string
		}{
			{"Spindle overheat", "", "temperature", ">", 70, 10, "critical"},
			{"High vibration", "", "vibration", ">", 7, 5, "warning"},
			// Tuned on simulated data: normal load noise never stays >= 0.8 for
			// more than 1s, a bearing fault holds it for 5-6s. Requiring 2s
			// (3 consecutive samples) separates them with margin on both sides.
			{"Anomaly detected", "", "anomaly_score", ">=", 0.8, 2, "warning"},
		}
		for _, r := range rules {
			batch.Queue(`INSERT INTO alert_rules (tenant_id, name, machine_slug, metric, operator, threshold, for_seconds, severity)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				tenantID, r.name, r.machine, r.metric, r.op, r.threshold, r.forSeconds, r.severity)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return err
		}
		db.log.Info("demo data seeded", "tenant", "acme")
		return nil
	})
}
