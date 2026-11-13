-- name: InsertWebhookEvent :execrows
INSERT INTO webhook_events (integration_id, organization_id, provider, event_key, event_type, payload, status, reason, transaction_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, sqlc.narg(transaction_id))
ON CONFLICT (provider, event_key) DO NOTHING;

-- name: ListWebhookEvents :many
SELECT * FROM webhook_events
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT 100;
