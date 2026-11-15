-- name: CreateOrganization :one
INSERT INTO organizations (name, slug, business_type, default_currency, timezone)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations WHERE id = $1;

-- name: ListOrganizationsForUser :many
SELECT o.*, m.role AS member_role, m.status AS member_status
FROM organizations o
JOIN organization_members m ON m.organization_id = o.id
WHERE m.user_id = $1 AND m.status = 'active'
ORDER BY o.created_at;

-- name: GetMembership :one
SELECT * FROM organization_members
WHERE organization_id = $1 AND user_id = $2 AND status = 'active';

-- name: CreateMembership :one
INSERT INTO organization_members (organization_id, user_id, role, status)
VALUES ($1, $2, $3, 'active')
RETURNING *;

-- name: ListMembers :many
SELECT m.id, m.organization_id, m.user_id, m.role, m.status, m.created_at,
       u.email, u.full_name
FROM organization_members m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = $1 AND m.status <> 'removed'
ORDER BY m.created_at;

-- name: UpdateMemberRole :exec
UPDATE organization_members SET role = $3
WHERE organization_id = $1 AND user_id = $2 AND status = 'active';

-- name: CreateInvitation :one
INSERT INTO invitations (organization_id, email, role, token_hash, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetInvitationByTokenHash :one
SELECT * FROM invitations WHERE token_hash = $1;

-- name: MarkInvitationAccepted :exec
UPDATE invitations SET accepted_at = now()
WHERE id = $1 AND accepted_at IS NULL;

-- name: ListInvitations :many
SELECT * FROM invitations
WHERE organization_id = $1 AND accepted_at IS NULL AND expires_at > now()
ORDER BY created_at DESC;

-- name: UpdateOrganization :exec
UPDATE organizations
SET name = $2, timezone = $3, updated_at = now()
WHERE id = $1;
