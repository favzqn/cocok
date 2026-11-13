-- name: CreateSettlement :one
INSERT INTO settlements (organization_id, account_id, counterparty, reference,
    expected_amount_minor, currency, expected_date, notes, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListSettlements :many
SELECT * FROM settlements
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY expected_date
LIMIT 200;

-- name: ListPendingSettlements :many
SELECT * FROM settlements
WHERE organization_id = $1 AND status = 'pending'
ORDER BY expected_date
LIMIT 500;

-- name: GetSettlement :one
SELECT * FROM settlements WHERE organization_id = $1 AND id = $2;

-- name: UpdateSettlement :exec
UPDATE settlements
SET account_id = $3, counterparty = $4, reference = $5,
    expected_amount_minor = $6, currency = $7, expected_date = $8,
    status = $9, settled_amount_minor = $10, settled_at = $11,
    notes = $12, linked_group_id = $13, updated_at = now()
WHERE organization_id = $1 AND id = $2;

-- name: DeleteSettlement :exec
DELETE FROM settlements WHERE organization_id = $1 AND id = $2;
