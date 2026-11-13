# COCOK

**Financial Reconciliation Infrastructure for Indonesian Businesses.**

Import financial data from anywhere. Automatically reconcile everything. Investigate only what doesn't match.

> Status: **Phase 0–5 complete** — identity, RBAC, multi-tenancy, CSV ingestion with dedup, reconciliation engine, exceptions, settlements with auto-settle, analytics + CSV reports. See `IMPLEMENTATION_PLAN.md`.

## Stack

- **Backend:** Go · Chi · pgx · sqlc · PostgreSQL · Redis (later phases)
- **Frontend:** Next.js · TypeScript (strict) · Tailwind CSS · TanStack Query · React Hook Form · Zod
- **Infra:** Docker Compose · GitHub Actions

## Quick start

Prereqs: Go 1.26+, Node 22+, PostgreSQL 15+ (or `docker compose up -d`).

```bash
# 1. database (compose or your own Postgres)
docker compose up -d
createdb cocok   # if running local Postgres instead

# 2. backend
cp .env-template .env
make migrate
make api                 # serves http://localhost:8081

# 3. frontend (second terminal)
cd apps/web && npm install
make web                 # serves http://localhost:3000
```

Open http://localhost:3000 → create account → create organization.

## Tests

```bash
make test                # unit tests + vet
make test-integration    # real-Postgres API tests (tenant isolation, auth, RBAC)
```

`test-integration` needs `TEST_DATABASE_URL` (default `postgres://aaa@localhost:5432/cocok_test`). The test schema is wiped on every run.

## Repository layout

```
apps/web/          Next.js app
services/api/      Go API (modular monolith)
  cmd/server/      entrypoint (also runs migrations)
  internal/        domain modules (auth, organizations, audit, httpx, platform)
  migrations/      goose SQL migrations
  queries/         sqlc queries
  tests/           integration tests
docs/architecture/ design documents
```

## Architecture docs

- [System design](docs/architecture/system-design.md)
- [Database design](docs/architecture/database-design.md)
- [Implementation plan](IMPLEMENTATION_PLAN.md)

## Security notes (Phase 1)

- Argon2id password hashing, per-password salt.
- Opaque session tokens stored as SHA-256 hashes, httpOnly cookie, rotation on refresh, revocation on logout.
- Tenant isolation enforced by membership check middleware; cross-org reads return `404`.
- Permission-based authorization (5 roles → permission matrix in `internal/auth/rbac.go`).
- Audit log is append-only through application interfaces.
