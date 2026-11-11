# COCOK Implementation Plan

Status legend: `[ ]` todo · `[~]` in progress · `[x]` done & verified (tests run)

## Phase 0 — Foundation
- [x] Monorepo layout, git init
- [x] Architecture docs (system-design, database-design)
- [x] docker-compose: postgres 15 + redis 7 (also supports local Postgres; local dev ran on Postgres 14)
- [x] Go module skeleton: config, pgx pool, chi router, health checks, graceful shutdown, structured logging
- [x] Migrations runner (goose embedded) + initial migration (+1 follow-up for audit FKs)
- [x] Next.js skeleton: app shell, Tailwind, strict TS
- [x] Makefile + .env.example + README local dev
- [x] GitHub Actions CI: go vet/test/build + web typecheck/build (Postgres service for integration tests)

**Accept:** ✅ fresh clone path documented; `/healthz` green; verified live (API :8081, web :3000).

## Phase 1 — Identity & Multi-Tenancy
- [x] Argon2id password hashing + unit tests
- [x] Register / login / logout / refresh (session rotation) / me
- [x] Organizations CRUD, membership, invitations (create/accept), last-owner guard
- [x] RBAC: 5 roles → permission matrix; `RequirePermission` middleware
- [x] Tenant middleware: org context + membership check; cross-org → 404
- [x] Audit writer: org created, invite created/accepted, role changed (fire-and-log; in-tx audit lands Phase 3)
- [x] API tests: happy path, invalid input, unauthorized, cross-tenant, token rotation revocation (`services/api/tests`)
- [x] Frontend: login, register, dashboard shell with org list/create, logout, loading/error/empty states
- [x] Frontend: org switcher (sidebar select, persisted active org), members table with role change, invite form with one-time link, `/invite` accept page
- [ ] Email verification + password reset (needs mailer — deferred with Phase 6 notifications)

**Accept:** ✅ two orgs in DB; cross-org 404 tested; permission matrix enforced (403 test); audit rows verified live.

## Phase 2 — Ingestion (next session)
Financial accounts → CSV/XLSX import (template mapping, preview, validate, commit) → normalization → canonical transactions → duplicate detection → raw file storage.

## Phase 3 — Reconciliation engine
Candidate generation (indexed, no O(n²)) → normalization → scoring (amount/ref/date/counterparty/description, versioned weights) → conflict resolution → match groups (1:1, 1:N, N:1) → approve/reject/unmatch/manual → audit.

## Phase 4 — Exceptions & Settlements
## Phase 5 — Analytics & Reporting
## Phase 6 — Provider integrations (Xendit/Midtrans sandbox, webhooks, sig verify, dedup)
## Phase 7 — Production hardening (security review, k6, observability, deploy)

---

## Testing Strategy (standing rules)
- Money paths: no float — enforced by tests + review.
- Tenant isolation test per new org-owned endpoint.
- Concurrency tests where allocations happen (Phase 3+).
- `go test ./...` must pass before marking any task `[x]`.

## Known deviations from PRD (documented decisions)
1. Session auth as opaque DB sessions instead of JWT pair — see system-design.md §Auth Model.
2. Multi-owner orgs allowed in Phase 1; single-owner constraint deferred (PRD silent).
3. RLS deferred to Phase 2 when org-owned tables appear.
