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

## Phase 2 — Ingestion ✅
- [x] Migration: `financial_accounts`, `import_batches` (raw CSV stored BYTEA, ≤10MB), `transactions` (canonical, `UNIQUE(account_id, fingerprint)`)
- [x] sqlc queries + generated code
- [x] `platform/money.go` — amount string → int64 minor units (IDR=0 dec, USD=2 dec; grouping/decimal auto-detect, no float) + tests
- [x] Financial accounts CRUD (`account:manage` / `data:view`)
- [x] CSV import: multipart upload → column mapping (date/amount required, description/counterparty/direction/reference optional) → per-row validate → preview (sample + row errors) → commit in one tx
- [x] Direction: from mapped column (credit/debit/masuk/keluar/…) else sign of amount; amount stored abs, `amount_minor >= 0`
- [x] Dates: 8 layouts, parsed in Asia/Jakarta
- [x] Duplicate detection: sha256 fingerprint (date|direction|amount|norm(desc)|norm(counterparty)|ref), `ON CONFLICT DO NOTHING`; re-import counts as duplicates
- [x] Audit: `account.created/updated`, `import.uploaded/committed`
- [x] API tests: happy path, preview-before-commit, double-commit 409, re-import dedup, bad-row preview, cross-tenant 404, viewer 403, finance_staff import OK / account-create 403
- [x] Frontend: accounts panel, CSV upload + mapping selects + preview + commit, canonical transactions table

**Accept:** ✅ `go vet` + `go test ./...` green (unit + integration vs live Postgres); web typecheck green.

Deferred within Phase 2 (documented):
- XLSX import — needs `excelize` dependency; CSV covers current scope. Add when a real marketplace/gateway export requires it.
- RLS (`SET app.current_org`) — all queries already filter `organization_id`; add FORCE RLS in Phase 7 hardening.
- Object storage for raw files — BYTEA inline fine ≤10MB; move to S3 when files grow.
- Org timezone override — parse uses Asia/Jakarta (product default) until org settings land.


## Phase 3 — Reconciliation ✅
- [x] Migration: `scoring_weights` (versioned, seed v1), `match_groups` (proposed/approved/rejected; 1:1, 1:N, N:1, manual), `match_group_items`, `match_candidates` + candidate index
- [x] Candidate generation: indexed SQL pairs (amount band ±100bp, date window ±7d, excludes approved) + bounded N-ary subsets (bucket by counterparty/date, ≤8 members, exact integer sum); pair limit 500, tx scan cap 5000
- [x] Scoring (`internal/matching`, pure): amount/date/ref/counterparty/description components in [0,1], weighted sum from `scoring_weights`, version stamped on candidates/groups; unit tests
- [x] Conflict resolution: sort by score (deterministic tie-break) → greedy, each tx joins ≤1 proposed group; re-check under org advisory lock before insert (race-safe)
- [x] Run replaces previous proposed groups atomically; approved groups untouched
- [x] Approve / reject (frees txs) / unmatch (deletes group) — all in tx + advisory xact lock per org, audit events `reconciliation.run`, `match.approved/rejected/unmatched/manual`
- [x] Manual match: explicit tx ids, requires ≥1 credit + ≥1 debit, sum(credits)==sum(debits), conflict if already matched
- [x] RBAC: `reconcile:run` (run), `match:approve` (manual/approve/reject/unmatch), `data:view` (lists); cross-tenant → 404
- [x] API tests: run→approve flow, re-run replace, double-approve 409, unmatch frees, manual balanced/one-sided/cross-org, viewer 403, staff run-yet-approve-403, cross-tenant 404, **4-goroutine concurrency → exactly 1 created**
- [x] Frontend: Reconciliation panel (Run matching, status tabs, approve/reject/unmatch per RBAC), sidebar nav enabled

**Accept:** ✅ `go vet` + `go test ./...` green (unit + integration vs live Postgres); web typecheck green.

Deferred within Phase 3 (documented ceilings):
- N-ary buckets truncate at 8 members / 5000-tx scan — raise or move to SQL enumeration when volume demands.
- Weights are DB-versioned but no admin API yet — bump via SQL insert (v2+) until Settings lands (Phase 7).
- Manual match UI (API + tests exist; table multi-select deferred).
- Rejected-group history is kept; purging/retention policy deferred to Phase 7.


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
