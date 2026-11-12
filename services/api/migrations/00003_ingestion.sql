-- +goose Up
-- Phase 2: ingestion (accounts → import batches → canonical transactions).
-- RLS deferred (documented deviation): every query filters organization_id;
-- add FORCE RLS + app.current_org in Phase 7 hardening.

CREATE TABLE financial_accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL DEFAULT 'bank'
        CHECK (kind IN ('bank','payment_gateway','e_wallet','marketplace','other')),
    account_number  TEXT,
    currency        CHAR(3) NOT NULL DEFAULT 'IDR',
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_fin_accounts_org ON financial_accounts(organization_id);

CREATE TABLE import_batches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id      UUID NOT NULL REFERENCES financial_accounts(id) ON DELETE CASCADE,
    filename        TEXT NOT NULL,
    mapping         JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_file        BYTEA NOT NULL,
    status          TEXT NOT NULL DEFAULT 'previewed'
        CHECK (status IN ('previewed','committed','failed')),
    row_count       INTEGER NOT NULL DEFAULT 0,
    valid_count     INTEGER NOT NULL DEFAULT 0,
    invalid_count   INTEGER NOT NULL DEFAULT 0,
    inserted_count  INTEGER NOT NULL DEFAULT 0,
    duplicate_count INTEGER NOT NULL DEFAULT 0,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    committed_at    TIMESTAMPTZ
);
CREATE INDEX idx_import_batches_org ON import_batches(organization_id, created_at DESC);

CREATE TABLE transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id      UUID NOT NULL REFERENCES financial_accounts(id) ON DELETE CASCADE,
    import_batch_id UUID REFERENCES import_batches(id) ON DELETE SET NULL,
    booked_at       TIMESTAMPTZ NOT NULL,
    direction       TEXT NOT NULL CHECK (direction IN ('credit','debit')),
    amount_minor    BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency        CHAR(3) NOT NULL DEFAULT 'IDR',
    description     TEXT NOT NULL DEFAULT '',
    counterparty    TEXT,
    external_ref    TEXT,
    fingerprint     TEXT NOT NULL,
    source_row      INTEGER NOT NULL,
    raw_row         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, fingerprint)
);
CREATE INDEX idx_transactions_org_time ON transactions(organization_id, booked_at DESC);
CREATE INDEX idx_transactions_account_time ON transactions(account_id, booked_at DESC);
CREATE INDEX idx_transactions_batch ON transactions(import_batch_id);

-- +goose Down
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS import_batches;
DROP TABLE IF EXISTS financial_accounts;
