# COCOK Database Design (Phase 0–1 scope)

PostgreSQL 15+. UUID v7-ish (uuid generated app-side, time-ordered via `gen_random_uuid` fallback). All timestamps `timestamptz` UTC. Money: `BIGINT` minor units.

## Phase 1 ERD

```
users ──< organization_members >── organizations ──< invitations
                │
                └──< sessions (user, not org-scoped)
organizations ──< audit_logs
```

## Tables

### users
| col | type | notes |
|---|---|---|
| id | uuid PK | |
| email | citext UNIQUE | lowercased on write |
| password_hash | text | argon2id |
| full_name | text | |
| email_verified_at | timestamptz NULL | |
| status | text CHECK (active, disabled) | default active |
| created_at / updated_at | timestamptz | |

### organizations
| col | type | notes |
|---|---|---|
| id | uuid PK | |
| name | text | |
| slug | text UNIQUE | |
| business_type | text NULL | |
| default_currency | char(3) | default 'IDR' |
| timezone | text | default 'Asia/Jakarta' |
| subscription_plan | text | default 'free' |
| status | CHECK (active, suspended) | |
| created_at / updated_at | | |

### organization_members
| col | type | notes |
|---|---|---|
| id | uuid PK | |
| organization_id | uuid FK | ON DELETE CASCADE |
| user_id | uuid FK | |
| role | text CHECK (owner, admin, finance_manager, finance_staff, viewer) | |
| status | CHECK (active, invited, removed) | |
| created_at | | |
| UNIQUE (organization_id, user_id) | | |

Constraint: exactly one active owner enforced in service layer (documented; partial unique index `WHERE role='owner' AND status='active'` per org added when Phase 2 lands — multi-owner orgs allowed initially, PRD doesn't forbid).

### invitations
| col | type | notes |
|---|---|---|
| id | uuid PK | |
| organization_id | uuid FK | |
| email | citext | |
| role | text | same CHECK as members |
| token_hash | text | sha256 of raw token; raw shown once |
| expires_at | timestamptz | 7 days |
| accepted_at | timestamptz NULL | |
| created_by | uuid FK users | |

### sessions
| col | type | notes |
|---|---|---|
| id | uuid PK | |
| user_id | uuid FK | ON DELETE CASCADE |
| token_hash | text UNIQUE | sha256(raw) |
| user_agent / ip | text | |
| expires_at | timestamptz | 30d sliding on refresh |
| revoked_at | timestamptz NULL | |
| created_at | | |

### audit_logs (append-only)
| col | type | notes |
|---|---|---|
| id | uuid PK | |
| organization_id | uuid NULL | |
| actor_id | uuid NULL | |
| action | text | e.g. `user.login`, `org.member.role_changed` |
| entity_type / entity_id | text / uuid | |
| previous_state / new_state | jsonb NULL | |
| reason | text NULL | |
| metadata | jsonb | request_id, ip |
| created_at | | |

No UPDATE/DELETE grants for app role on this table (Phase 2 hardening).

## Indexes (Phase 1)

- `users(email)` unique
- `organization_members(user_id)`, `(organization_id)`
- `sessions(token_hash)` unique, `sessions(expires_at)` partial for cleanup
- `invitations(token_hash)` unique, `(organization_id)`
- `audit_logs(organization_id, created_at DESC)`, `(entity_type, entity_id)`

## Migrations

`services/api/migrations/NNNN_name.sql`, applied by goose embedded in the binary (`migrate` subcommand) and in CI. Reversible where practical (down files).
