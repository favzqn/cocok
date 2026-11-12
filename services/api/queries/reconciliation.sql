-- name: GetActiveScoringWeights :one
SELECT * FROM scoring_weights ORDER BY version DESC LIMIT 1;

-- name: ListCandidatePairs :many
SELECT d.id AS debit_id, c.id AS credit_id
FROM transactions d
JOIN transactions c
  ON c.organization_id = d.organization_id
 AND c.direction = 'credit'
 AND c.amount_minor BETWEEN d.amount_minor - GREATEST(1, d.amount_minor * sqlc.arg(tolerance_bp) / 10000)
                     AND d.amount_minor + GREATEST(1, d.amount_minor * sqlc.arg(tolerance_bp) / 10000)
 AND c.booked_at >= d.booked_at - make_interval(days => sqlc.arg(window_days))
 AND c.booked_at <= d.booked_at + make_interval(days => sqlc.arg(window_days))
 AND c.id <> d.id
WHERE d.organization_id = sqlc.arg(organization_id)
  AND d.direction = 'debit'
  AND NOT EXISTS (
    SELECT 1 FROM match_group_items i
    JOIN match_groups g ON g.id = i.match_group_id
    WHERE i.transaction_id = d.id AND g.status = 'approved'
  )
  AND NOT EXISTS (
    SELECT 1 FROM match_group_items i
    JOIN match_groups g ON g.id = i.match_group_id
    WHERE i.transaction_id = c.id AND g.status = 'approved'
  )
ORDER BY abs(c.amount_minor - d.amount_minor), abs(extract(epoch from c.booked_at - d.booked_at))
LIMIT sqlc.arg(pair_limit);

-- name: ListUnmatchedTransactions :many
SELECT t.* FROM transactions t
WHERE t.organization_id = sqlc.arg(organization_id)
  AND NOT EXISTS (
    SELECT 1 FROM match_group_items i
    JOIN match_groups g ON g.id = i.match_group_id
    WHERE i.transaction_id = t.id AND g.status = 'approved'
  )
ORDER BY t.booked_at
LIMIT sqlc.arg(tx_limit);

-- name: GetTransactionsByIDs :many
SELECT * FROM transactions
WHERE organization_id = sqlc.arg(organization_id) AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: InsertMatchCandidate :exec
INSERT INTO match_candidates (organization_id, credit_ids, debit_ids, score, components, weights_version)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ClearMatchStateForOrg :exec
DELETE FROM match_candidates WHERE organization_id = $1;

-- name: DeleteProposedGroups :exec
DELETE FROM match_groups WHERE organization_id = $1 AND status = 'proposed';

-- name: CreateMatchGroup :one
INSERT INTO match_groups (organization_id, status, kind, score, weights_version, components, created_by, decided_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: AddMatchGroupItem :exec
INSERT INTO match_group_items (match_group_id, transaction_id, side)
VALUES ($1, $2, $3);

-- name: GetActiveItemCountForTxs :one
SELECT count(*)::int AS n
FROM match_group_items i
JOIN match_groups g ON g.id = i.match_group_id
WHERE i.transaction_id = ANY(sqlc.arg(ids)::uuid[])
  AND g.status IN ('proposed','approved');

-- name: GetMatchGroup :one
SELECT * FROM match_groups WHERE organization_id = $1 AND id = $2;

-- name: DecideMatchGroup :exec
UPDATE match_groups SET status = $3, decided_at = now()
WHERE organization_id = $1 AND id = $2 AND status = 'proposed';

-- name: DeleteMatchGroup :exec
DELETE FROM match_groups WHERE organization_id = $1 AND id = $2;

-- name: ListMatchGroupItems :many
SELECT * FROM match_group_items WHERE match_group_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListMatchGroups :many
SELECT * FROM match_groups
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY created_at DESC
LIMIT 100;

-- name: ListMatchCandidates :many
SELECT * FROM match_candidates
WHERE organization_id = $1
ORDER BY score DESC
LIMIT 100;
