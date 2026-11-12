-- +goose Up
-- Phase 3: reconciliation (candidates → groups → decisions).

CREATE TABLE scoring_weights (
    version              INTEGER PRIMARY KEY,
    weights              JSONB NOT NULL,
    min_score            DOUBLE PRECISION NOT NULL DEFAULT 0.6,
    amount_tolerance_bp  INTEGER NOT NULL DEFAULT 100,
    date_window_days     INTEGER NOT NULL DEFAULT 7,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO scoring_weights (version, weights) VALUES (1, '{
  "amount": 0.35, "date": 0.20, "ref": 0.15, "counterparty": 0.15, "description": 0.15
}'::jsonb);

CREATE TABLE match_groups (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'proposed'
        CHECK (status IN ('proposed','approved','rejected')),
    kind             TEXT NOT NULL CHECK (kind IN ('1:1','1:N','N:1','manual')),
    score            DOUBLE PRECISION,
    weights_version  INTEGER REFERENCES scoring_weights(version),
    components       JSONB,
    created_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at       TIMESTAMPTZ
);
CREATE INDEX idx_match_groups_org_status ON match_groups(organization_id, status, created_at DESC);

CREATE TABLE match_group_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_group_id   UUID NOT NULL REFERENCES match_groups(id) ON DELETE CASCADE,
    transaction_id   UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    side             TEXT NOT NULL CHECK (side IN ('credit','debit')),
    UNIQUE (match_group_id, transaction_id)
);
CREATE INDEX idx_match_items_tx ON match_group_items(transaction_id);

CREATE TABLE match_candidates (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    credit_ids       UUID[] NOT NULL,
    debit_ids        UUID[] NOT NULL,
    score            DOUBLE PRECISION NOT NULL,
    components       JSONB NOT NULL DEFAULT '{}'::jsonb,
    weights_version  INTEGER NOT NULL REFERENCES scoring_weights(version),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_match_candidates_org ON match_candidates(organization_id, score DESC);

CREATE INDEX idx_transactions_org_dir_amount
    ON transactions(organization_id, direction, amount_minor);

-- +goose Down
DROP TABLE IF EXISTS match_candidates;
DROP TABLE IF EXISTS match_group_items;
DROP TABLE IF EXISTS match_groups;
DROP TABLE IF EXISTS scoring_weights;
DROP INDEX IF EXISTS idx_transactions_org_dir_amount;
