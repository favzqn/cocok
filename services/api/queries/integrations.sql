-- name: CreateIntegration :one
INSERT INTO integrations (organization_id, provider, mode, webhook_secret, default_account_id, created_by)
VALUES ($1, $2, 'sandbox', $3, $4, $5)
RETURNING *;

-- name: ListIntegrations :many
SELECT * FROM integrations
WHERE organization_id = $1
ORDER BY created_at;

-- name: GetIntegration :one
SELECT * FROM integrations WHERE organization_id = $1 AND id = $2;

-- name: GetIntegrationByID :one
SELECT * FROM integrations WHERE id = $1 AND status = 'active';

-- name: DisableIntegration :exec
UPDATE integrations SET status = 'disabled', updated_at = now()
WHERE organization_id = $1 AND id = $2;
