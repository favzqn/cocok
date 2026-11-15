-- name: TxSummaryByDirection :many
SELECT direction,
       count(*)::int AS n,
       coalesce(sum(amount_minor), 0)::bigint AS total
FROM transactions
WHERE organization_id = $1
  AND (sqlc.narg(from_t)::timestamptz IS NULL OR booked_at >= sqlc.narg(from_t))
  AND (sqlc.narg(to_t)::timestamptz IS NULL OR booked_at < sqlc.narg(to_t))
GROUP BY direction;

-- name: MatchedSummaryByDirection :many
SELECT t.direction,
       count(*)::int AS n,
       coalesce(sum(t.amount_minor), 0)::bigint AS total
FROM match_group_items i
JOIN match_groups g ON g.id = i.match_group_id
JOIN transactions t ON t.id = i.transaction_id
WHERE g.organization_id = $1 AND g.status = 'approved'
  AND (sqlc.narg(from_t)::timestamptz IS NULL OR t.booked_at >= sqlc.narg(from_t))
  AND (sqlc.narg(to_t)::timestamptz IS NULL OR t.booked_at < sqlc.narg(to_t))
GROUP BY t.direction;

-- name: GroupSummaryByStatus :many
SELECT status,
       count(*)::int AS n,
       coalesce(avg(score), 0)::float AS avg_score
FROM match_groups
WHERE organization_id = $1
  AND (sqlc.narg(from_t)::timestamptz IS NULL OR created_at >= sqlc.narg(from_t))
  AND (sqlc.narg(to_t)::timestamptz IS NULL OR created_at < sqlc.narg(to_t))
GROUP BY status;

-- name: ExceptionSummaryByStatus :many
SELECT status, count(*)::int AS n
FROM exceptions
WHERE organization_id = $1
  AND (sqlc.narg(from_t)::timestamptz IS NULL OR detected_at >= sqlc.narg(from_t))
  AND (sqlc.narg(to_t)::timestamptz IS NULL OR detected_at < sqlc.narg(to_t))
GROUP BY status;

-- name: SettlementSummaryByStatus :many
SELECT status,
       count(*)::int AS n,
       coalesce(sum(expected_amount_minor), 0)::bigint AS expected_total,
       coalesce(sum(settled_amount_minor), 0)::bigint AS settled_total
FROM settlements
WHERE organization_id = $1
  AND (sqlc.narg(from_t)::timestamptz IS NULL OR expected_date >= sqlc.narg(from_t))
  AND (sqlc.narg(to_t)::timestamptz IS NULL OR expected_date < sqlc.narg(to_t))
GROUP BY status;

-- name: AccountRollup :many
SELECT a.id, a.name, a.currency,
       count(t.id)::int AS tx_count,
       coalesce(sum(t.amount_minor) FILTER (WHERE t.direction = 'credit'), 0)::bigint AS credit_total,
       coalesce(sum(t.amount_minor) FILTER (WHERE t.direction = 'debit'), 0)::bigint AS debit_total,
       count(t.id) FILTER (
           WHERE EXISTS (
               SELECT 1 FROM match_group_items i
               JOIN match_groups g ON g.id = i.match_group_id
               WHERE i.transaction_id = t.id AND g.status = 'approved'
           )
       )::int AS matched_count
FROM financial_accounts a
LEFT JOIN transactions t ON t.account_id = a.id
    AND (sqlc.narg(from_t)::timestamptz IS NULL OR t.booked_at >= sqlc.narg(from_t))
    AND (sqlc.narg(to_t)::timestamptz IS NULL OR t.booked_at < sqlc.narg(to_t))
WHERE a.organization_id = $1
GROUP BY a.id, a.name, a.currency
ORDER BY a.name;

-- name: ListTransactionsExport :many
SELECT t.id, t.booked_at, t.direction, t.amount_minor, t.currency,
       t.description, t.counterparty, t.external_ref,
       EXISTS (
           SELECT 1 FROM match_group_items i
           JOIN match_groups g ON g.id = i.match_group_id
           WHERE i.transaction_id = t.id AND g.status = 'approved'
       ) AS matched
FROM transactions t
WHERE t.organization_id = $1
  AND (sqlc.narg(from_t)::timestamptz IS NULL OR t.booked_at >= sqlc.narg(from_t))
  AND (sqlc.narg(to_t)::timestamptz IS NULL OR t.booked_at < sqlc.narg(to_t))
ORDER BY t.booked_at, t.id;
