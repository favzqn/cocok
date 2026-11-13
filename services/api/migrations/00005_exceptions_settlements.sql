-- +goose Up
-- Phase 4: exceptions (investigation queue) + settlements (expected cash movements).

CREATE TABLE exceptions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    type             TEXT NOT NULL CHECK (type IN ('unmatched','near_miss','manual')),
    status           TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open','resolved','dismissed')),
    subject_tx_ids   UUID[] NOT NULL,
    fingerprint      TEXT NOT NULL,
    detail           JSONB NOT NULL DEFAULT '{}'::jsonb,
    note             TEXT,
    detected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at      TIMESTAMPTZ,
    created_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    resolved_by      UUID REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX idx_exceptions_org_status ON exceptions(organization_id, status, detected_at DESC);
CREATE UNIQUE INDEX uq_exception_open
    ON exceptions(organization_id, type, fingerprint) WHERE status = 'open';

CREATE TABLE settlements (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id             UUID REFERENCES financial_accounts(id) ON DELETE SET NULL,
    counterparty           TEXT,
    reference              TEXT,
    expected_amount_minor  BIGINT NOT NULL CHECK (expected_amount_minor >= 0),
    currency               CHAR(3) NOT NULL DEFAULT 'IDR',
    expected_date          TIMESTAMPTZ NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','partial','settled','failed')),
    settled_amount_minor   BIGINT NOT NULL DEFAULT 0 CHECK (settled_amount_minor >= 0),
    settled_at             TIMESTAMPTZ,
    notes                  TEXT,
    linked_group_id        UUID REFERENCES match_groups(id) ON DELETE SET NULL,
    created_by             UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_settlements_org_status ON settlements(organization_id, status, expected_date);

-- +goose Down
DROP TABLE IF EXISTS settlements;
DROP TABLE IF EXISTS exceptions;
