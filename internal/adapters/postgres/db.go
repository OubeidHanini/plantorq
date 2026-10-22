// Package postgres implements the repositories with pgx. All SQL lives here;
// the rest of the code base only sees domain types and ports.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/OubeidHanini/plantorq/internal/app/platform"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migrationLock is an arbitrary key for pg_advisory_lock, so that several
// replicas starting at once apply migrations exactly once.
const migrationLock = 7_262_026

type DB struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// Open connects with retries: in Compose and Kubernetes the database may
// still be starting when the service boots.
func Open(ctx context.Context, dsn string, log *slog.Logger) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	var pool *pgxpool.Pool
	for attempt := 1; ; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
			pool.Close()
		}
		if attempt == 30 || ctx.Err() != nil {
			return nil, fmt.Errorf("postgres: connect: %w", err)
		}
		log.Warn("postgres not ready, retrying", "attempt", attempt, "err", err)
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &DB{pool: pool, log: log}, nil
}

func (db *DB) Close()                         { db.pool.Close() }
func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

// Migrate applies pending migrations in order, each in its own transaction.
func (db *DB) Migrate(ctx context.Context) error {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLock); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLock) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(name, "migrations/"), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: bad name: %w", name, err)
		}
		var applied bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)", version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		db.log.Info("migration applied", "version", version, "file", name)
	}
	return nil
}

// notFound maps pgx's no-rows error to the application error.
func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return platform.ErrNotFound
	}
	return err
}
