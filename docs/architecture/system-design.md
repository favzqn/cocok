# COCOK System Design

Modular monolith. One Go binary serves the API; Next.js serves the UI; PostgreSQL is the system of record; Redis handles cache + job coordination (from Phase 3+).

## Module Boundaries

```
cmd/server            wiring only
internal/
  config              env config
  platform            db pool, errors, money, ids, request-id
  httpx               router, middleware (auth, tenant, rbac, ratelimit), responses
  auth                register/login/sessions/password reset
  organizations       orgs, members, invitations
  audit               append-only audit writer + query
  accounts            financial accounts        (Phase 2)
  imports             ingestion pipeline        (Phase 2)
  transactions        canonical model           (Phase 2)
  reconciliation      sessions, match groups    (Phase 3)
  matching            scoring engine            (Phase 3)
  exceptions          (Phase 4)
  settlements         (Phase 4)
  reports             (Phase 5)
  connectors          xendit/midtrans adapters  (Phase 6)
```

Rules:
- Handlers thin: parse → call service → render. No business logic in handlers.
- Services own transactions; repositories (sqlc) are dumb data access.
- No circular imports; cross-module calls go through small interfaces defined by the consumer.
- Every query that touches org-owned data takes `organization_id` as first arg.

## Request Flow

```
Client → Chi router
  → request-id middleware
  → recovery + structured log
  → session auth (cookie → user)
  → tenant middleware (org from route/param, membership checked)
  → rbac middleware (permission required by route)
  → handler → service → sqlc → PostgreSQL
  → audit writer (fire-and-forget in tx where financial)
```

## Auth Model (Phase 1)

- Opaque session tokens: 32 random bytes, SHA-256 stored in `sessions.token_hash`, httpOnly+Secure+SameSite=Lax cookie.
- Access = the session cookie itself; `POST /auth/refresh` rotates the token (new cookie, old revoked) — satisfies refresh-token-rotation requirement.
- Passwords: Argon2id (t=1, m=64MB, p=1, keyLen=32), per-password salt, stored `argon2id$v=1$m=...$salt$hash`.
- Rate limit: per-IP + per-email counters in Redis (Phase 1: in-memory fallback if Redis down).

## Tenant Isolation

1. Route context carries `OrgID` + `Role` after membership check.
2. Every repo call requires `organization_id`; sqlc queries always filter by it.
3. Cross-org lookup returns `404` (not 403) to avoid ID probing.
4. Phase 2+: PostgreSQL RLS policies as defense-in-depth (`SET app.current_org`).

## Money

`int64` minor units everywhere. Currency table decides precision (IDR=0, USD=2). Parsing from CSV lives in one place (`platform/money`). No `float64` in money paths — lint rule + tests.

## Jobs (Phase 3+)

Asynq on Redis. Imports, reconciliation runs, report exports. Transactional outbox table when events must survive crashes; no Kafka.

## Error Contract

```json
{ "error": { "code": "validation_failed", "message": "...", "details": {...}, "request_id": "..." } }
```

Codes: `unauthorized`, `forbidden`, `not_found`, `validation_failed`, `conflict`, `rate_limited`, `internal`.
