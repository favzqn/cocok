-- +goose Up
-- Phase 6: provider integrations, webhook events (dedup), auth tokens, sandbox mailer.

CREATE TABLE integrations (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider          TEXT NOT NULL CHECK (provider IN ('xendit','midtrans')),
    mode              TEXT NOT NULL DEFAULT 'sandbox' CHECK (mode IN ('sandbox','live')),
    webhook_secret    TEXT NOT NULL,          -- xendit callback token / midtrans server key
    default_account_id UUID REFERENCES financial_accounts(id) ON DELETE SET NULL,
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_integration_provider_org
    ON integrations(organization_id, provider) WHERE status = 'active';

CREATE TABLE webhook_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id   UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    organization_id  UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,
    event_key        TEXT NOT NULL,           -- stable provider id (invoice/payment id)
    event_type       TEXT NOT NULL DEFAULT '',
    payload          JSONB NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('processed','ignored','failed')),
    reason           TEXT,
    transaction_id   UUID REFERENCES transactions(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, event_key)
);
CREATE INDEX idx_webhook_events_org ON webhook_events(organization_id, created_at DESC);

CREATE TABLE auth_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('verify_email','reset_password')),
    token_hash  TEXT NOT NULL UNIQUE,          -- sha256(raw)
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_tokens_user ON auth_tokens(user_id, kind) WHERE used_at IS NULL;

-- Sandbox mailer: "sent" emails land here (console log + row) until a real
-- provider (SMTP/SES) is configured in Phase 7.
CREATE TABLE email_outbox (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    to_email   CITEXT NOT NULL,
    subject    TEXT NOT NULL,
    body       TEXT NOT NULL,
    kind       TEXT NOT NULL DEFAULT 'generic',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS email_outbox;
DROP TABLE IF EXISTS auth_tokens;
DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS integrations;
