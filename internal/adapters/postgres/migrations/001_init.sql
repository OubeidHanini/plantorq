-- Tenants are the SaaS customers; every other row belongs to exactly one.
CREATE TABLE tenants (
    id         BIGSERIAL PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    tenant_id     BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE machines (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug       TEXT NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    protocol   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE alert_rules (
    id           BIGSERIAL PRIMARY KEY,
    tenant_id    BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    machine_slug TEXT NOT NULL DEFAULT '',
    metric       TEXT NOT NULL,
    operator     TEXT NOT NULL CHECK (operator IN ('>', '>=', '<', '<=')),
    threshold    DOUBLE PRECISION NOT NULL,
    for_seconds  INTEGER NOT NULL DEFAULT 0 CHECK (for_seconds >= 0),
    severity     TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX alert_rules_tenant_idx ON alert_rules (tenant_id);

-- Alerts copy the rule fields so history stays readable after a rule changes.
CREATE TABLE alerts (
    id           BIGSERIAL PRIMARY KEY,
    tenant_id    BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_id      BIGINT REFERENCES alert_rules(id) ON DELETE SET NULL,
    rule_name    TEXT NOT NULL,
    machine_slug TEXT NOT NULL,
    metric       TEXT NOT NULL,
    operator     TEXT NOT NULL,
    threshold    DOUBLE PRECISION NOT NULL,
    value        DOUBLE PRECISION NOT NULL,
    severity     TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    resolved_at  TIMESTAMPTZ,
    acked_by     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    acked_at     TIMESTAMPTZ
);
-- At most one open alert per rule and machine. This makes "fire" idempotent
-- at the database level, even across rules-engine restarts or replicas.
CREATE UNIQUE INDEX alerts_one_open_idx ON alerts (rule_id, machine_slug) WHERE resolved_at IS NULL;
CREATE INDEX alerts_tenant_started_idx ON alerts (tenant_id, started_at DESC);
