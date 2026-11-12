-- name: CreateFinancialAccount :one
INSERT INTO financial_accounts (organization_id, name, kind, account_number, currency, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetFinancialAccount :one
SELECT * FROM financial_accounts
WHERE organization_id = $1 AND id = $2;

-- name: ListFinancialAccounts :many
SELECT * FROM financial_accounts
WHERE organization_id = $1
ORDER BY created_at;

-- name: UpdateFinancialAccount :exec
UPDATE financial_accounts
SET name = $3, kind = $4, status = $5, updated_at = now()
WHERE organization_id = $1 AND id = $2;
