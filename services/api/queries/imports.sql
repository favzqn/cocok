-- name: CreateImportBatch :one
INSERT INTO import_batches (organization_id, account_id, filename, mapping, raw_file, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetImportBatch :one
SELECT * FROM import_batches
WHERE organization_id = $1 AND id = $2;

-- name: ListImportBatches :many
SELECT * FROM import_batches
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT 50;

-- name: UpdateImportBatchPreview :exec
UPDATE import_batches
SET mapping = $3, row_count = $4, valid_count = $5, invalid_count = $6, updated_at = now()
WHERE organization_id = $1 AND id = $2;

-- name: MarkImportBatchCommitted :exec
UPDATE import_batches
SET status = 'committed', inserted_count = $3, duplicate_count = $4,
    committed_at = now(), updated_at = now()
WHERE organization_id = $1 AND id = $2;

-- name: InsertTransaction :execrows
INSERT INTO transactions (
    organization_id, account_id, import_batch_id, booked_at, direction,
    amount_minor, currency, description, counterparty, external_ref,
    fingerprint, source_row, raw_row
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (account_id, fingerprint) DO NOTHING;

-- name: ListTransactions :many
SELECT * FROM transactions
WHERE organization_id = $1
  AND (sqlc.narg('account_id')::uuid IS NULL OR account_id = sqlc.narg('account_id'))
ORDER BY booked_at DESC, id
LIMIT sqlc.narg('limit');
