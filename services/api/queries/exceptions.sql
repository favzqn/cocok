-- name: InsertException :execrows
INSERT INTO exceptions (organization_id, type, status, subject_tx_ids, fingerprint, detail, note, created_by)
VALUES ($1, $2, 'open', $3, $4, $5, $6, $7)
ON CONFLICT (organization_id, type, fingerprint) WHERE status = 'open'
DO NOTHING;

-- name: ListExceptions :many
SELECT * FROM exceptions
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(type)::text IS NULL OR type = sqlc.narg(type))
ORDER BY detected_at DESC
LIMIT 200;

-- name: GetException :one
SELECT * FROM exceptions WHERE organization_id = $1 AND id = $2;

-- name: DecideException :execrows
UPDATE exceptions
SET status = $3, note = COALESCE($5, note), resolved_at = now(), resolved_by = $4
WHERE organization_id = $1 AND id = $2 AND status = 'open';

-- name: DeleteOpenGenerated :exec
DELETE FROM exceptions
WHERE organization_id = $1 AND type = ANY($2::text[]) AND status = 'open';

-- name: GetDismissedExceptionFps :many
SELECT fingerprint FROM exceptions
WHERE organization_id = $1 AND type = $2 AND status = 'dismissed';
