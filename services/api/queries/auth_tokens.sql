-- name: CreateAuthToken :one
INSERT INTO auth_tokens (user_id, kind, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAuthTokenByHash :one
SELECT * FROM auth_tokens WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now();

-- name: MarkAuthTokenUsed :exec
UPDATE auth_tokens SET used_at = now() WHERE id = $1 AND used_at IS NULL;

-- name: SetUserPasswordHash :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: MarkEmailVerified :exec
UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()), updated_at = now()
WHERE id = $1;

-- name: InsertEmailOutbox :exec
INSERT INTO email_outbox (to_email, subject, body, kind) VALUES ($1, $2, $3, $4);

-- name: ListEmailOutbox :many
SELECT * FROM email_outbox WHERE to_email = $1 ORDER BY created_at DESC LIMIT 10;

