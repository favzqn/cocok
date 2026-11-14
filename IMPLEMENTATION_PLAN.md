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
- [x] XLSX upload (added later): `excelize` first-sheet → shared CSV pipeline, original bytes stored for reprocessing; server-side `GuessMapping` when client sends no mapping (xlsx has no readable header client-side)

**Accept:** ✅ `go vet` + `go test ./...` green (unit + integration vs live Postgres); web typecheck green.

Deferred within Phase 2 (documented):
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


## Phase 4 — Exceptions & Settlements ✅
- [x] Migration: `exceptions` (type unmatched/near_miss/manual; open/resolved/dismissed; subject tx uuid[]; partial unique on open fp) + `settlements` (expected amount/date, pending/partial/settled/failed, linked_group_id)
- [x] Exception generation inside Run's tx: every leftover unmatched tx + near-miss candidates (score ∈ [0.3, min_score)); dismissed fingerprints never re-opened; audit `exception.resolved/dismissed`
- [x] Exceptions API: list (status/type filter), manual flag (409 on dup open), resolve/dismiss with note (`exception:resolve`)
- [x] Settlements API: CRUD (`reconcile:run`), status transitions manual, settled defaults amount + timestamp; audit created/updated/deleted
- [x] **Auto-settle**: approve → inside same tx, pending settlement with ±1% amount + ±7d date vs group → status settled + linked_group_id + audit `settlement.settled`
- [x] API tests: generation (0 when fully matched, orphan → 1, dismissed stays gone), manual flag 409, viewer 403, cross-tenant 404; settlement CRUD, auto-settle on approve (amount + link verified), staff create OK, viewer create 403, cross-tenant 404
- [x] Frontend: Exceptions panel (status tabs, resolve/dismiss), Settlements panel (create form, status actions, delete), sidebar nav enabled

**Accept:** ✅ `go vet` + `go test ./...` green (unit + integration vs live Postgres); web typecheck green.

Deferred within Phase 4 (documented):
- Exception notes on resolve/dismiss via UI (API accepts `note`; UI sends empty — prompt() not worth it until bulk actions).
- Settlement amount units in UI = major units (IDR no decimals); minor-units input when FX accounts land.
- Auto-settle picks single closest pending settlement; partial/multi-group settlement splitting deferred.
- Rejected-run exception purging/retention → Phase 7.

## Phase 5 — Analytics & Reporting ✅
- [x] Summary endpoint (`GET /reports/summary`, `data:view`): tx by direction, matched via approved groups + match rate, groups/exceptions/settlements by status, per-account rollup (count, credit/debit totals, matched count)
- [x] CSV export (`GET /reports/export?kind=`, `report:export`): transactions (with matched flag), matches (group members), exceptions, settlements — stdlib `encoding/csv`, timestamped filename attachment
- [x] API test: summary numbers vs seeded data (3 tx / 2 matched / rate ~0.667 / 1 open exception), CSV header + matched column verified, bad kind 400, cross-tenant 404
- [x] Frontend: Reports panel — stat cards (tx, match rate, matched, open exceptions), value flows, per-account table, 4 CSV download buttons; sidebar nav enabled

**Accept:** ✅ `go vet` + `go test ./...` green; web typecheck green.

Deferred within Phase 5 (documented):
- Date-range filters on summary/export (whole-org only until period reporting lands).
- PDF/Excel export — CSV covers operator needs; add when clients demand.
- Charts (Recharts in stack) — tables first; sparklines when history accumulates.

## Phase 6 — Provider integrations & notifications ✅
- [x] Migration: `integrations` (xendit/midtrans, mode `sandbox`, webhook_secret, default_account; unique active per provider+org), `webhook_events` (UNIQUE provider+event_key = dedup), `auth_tokens` (verify_email/reset_password, single-use, hashed), `email_outbox` (sandbox mailer)
- [x] `internal/connectors` — adapter interface; **Xendit**: `x-callback-token` constant-time compare; **Midtrans**: hex(sha512(order_id+status_code+gross_amount+server_key)) in `x-signature`; event parse → normalized PaymentEvent (json.Number — no float)
- [x] Webhooks: public `POST /api/v1/webhooks/{provider}/{integrationID}` (signature IS auth) → verify → dedup → mint credit transaction (amount via platform.ParseAmount, fingerprint double-guard) → event row processed/ignored/failed → audit; 401 bad sig, 404 unknown integration, 200 duplicate (stops provider retries)
- [x] Integrations CRUD (`data:view` / `integration:manage`), webhook URL returned for provider dashboard paste
- [x] Notifications: `auth.Mailer` interface + `notifications.OutboxMailer` (row + log, SMTP lands Phase 7); register sends verification email; `POST /auth/verify-email`, `/auth/forgot-password` (always 200, no enumeration), `/auth/reset-password` (revokes ALL sessions) — auth route rate-limit applies
- [x] API tests: Xendit full flow (401 → processed → tx minted → duplicate no-op → 404), Midtrans sig valid/invalid, integration dup 409, viewer 403, cross-tenant 404; verify single-use, reset (weak 400, old pass dead, sessions revoked, token single-use), forgot unknown-email 200
- [x] Frontend: Integrations panel (copy webhook URL, connect form, disable per RBAC), `/forgot-password`, `/reset-password`, `/verify-email` pages, login link; nav enabled

**Accept:** ✅ `go vet` + `go test ./...` green; web typecheck green.

Deferred within Phase 6 (documented):
- Outbound APIs (create invoice/charge) — inbound-only adapters until a flow needs them.
- Live mode: `mode` column exists but always written `sandbox` until credentials (PRODUCT.md §Assumptions).
- Email provider (SMTP/SES) behind the same Mailer interface — Phase 7.
- Webhook retry/backoff UI — event table stores status for ops inspection.

## Phase 7 — Production hardening ✅
- [x] Migration `00007`: `audit_logs` append-only **enforced by DB trigger** (UPDATE/DELETE raise) + test proving it
- [x] Security headers middleware (nosniff, X-Frame-Options DENY, referrer-policy, permissions-policy, HSTS); CSP documented as deferred (Next nonce work)
- [x] `/healthz` pings DB → 503 when down; `/metrics` Prometheus-text counters (hand-rolled, zero deps)
- [x] Auth rate limit configurable (`AUTH_RATE_RPS`, default 0.5 rps); webhook route rate-limited 2 rps
- [x] **k6**: `loadtest/k6.js` smoke (5 VU × 15s, session cookie exported from setup — k6 VU-jar gotcha) — **0% errors, p95 10.5ms, 2042 reqs** vs local API
- [x] Docker: `services/api/Dockerfile` (static build), `apps/web/Dockerfile`, `docker-compose.prod.yml` (db+api+web, healthcheck-gated migrations)
- [x] Docs: `docs/deploy.md` (env table, pre-traffic checklist, ops), `docs/security-review.md` (applied items, verified carry-overs, accepted risks + upgrade paths)
- [x] Live-server incident fixed: stale `go run` child kept port 8081 across restarts (pkill patterns missed go-build cache path) — verified fresh binary via summary/integrations/metrics probes

**Accept:** ✅ `go vet` + `go test ./...` green; k6 thresholds green; security headers + metrics + append-only trigger tested.

Known deferral (full rationale in `docs/security-review.md`): Postgres RLS —
application-layer tenancy enforced + tested; upgrade path = per-request
`SET LOCAL` tx wrapper alongside non-superuser DB role rollout.


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
