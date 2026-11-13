# Security Review (Phase 7)

Scope: application + database. Infrastructure (cloud IAM, network) belongs to the
deploy target and is covered in `deploy.md` checklist.

## Applied in this phase

| Item | Status | Where |
|---|---|---|
| Audit log immutability | ✅ DB trigger blocks UPDATE/DELETE | `migrations/00007_hardening.sql`, tested |
| Security headers | ✅ nosniff, DENY framing, referrer-policy, permissions-policy, HSTS | `httpx.SecurityHeaders` middleware |
| Auth brute force | ✅ per-IP rate limit (0.5 rps default, `AUTH_RATE_RPS` override for tests) | `httpx.RateLimit` on `/auth/*` |
| Webhook abuse | ✅ per-IP rate limit 2 rps + **signature is the auth** | webhook route |
| Health/readiness | ✅ `/healthz` pings DB → 503 | router |
| Metrics exposure | ✅ `/metrics` counters, no dependency data leaked | `httpx/metrics.go` |

## Carried from earlier phases (verified)

- **Tenant isolation**: middleware membership check; every sqlc query filters
  `organization_id`; cross-org → 404; tested per endpoint group (integration suite).
- **Password storage**: Argon2id (t=1, m=64MB, p=1), per-password salt.
- **Sessions**: opaque 32-byte tokens, SHA-256 at rest, httpOnly + SameSite=Lax,
  rotation on refresh, revoke-all on password reset (tested).
- **Tokens** (verify/reset): SHA-256 at rest, single-use, expiring (24h / 1h).
- **User enumeration**: forgot-password always 200; login timing blunted.
- **Money**: integer minor units everywhere; parser rejects over-precision; no float.
- **SQLi**: 100% sqlc parameterized queries.
- **CORS**: exact origin + credentials; OPTIONS short-circuit.
- **Body limits**: JSON 1MB, webhook 1MB, CSV upload 10MB.
- **Webhook signatures**: Xendit token constant-time compare; Midtrans SHA-512
  HMAC-style construction with `hmac.Equal`; duplicate events idempotent (200).
- **Audit trail**: append-only (DB-enforced), covers auth, org, import, match,
  exception, settlement, integration, webhook events.

## Accepted risks / deferred (with upgrade paths)

1. **Postgres RLS** — not enabled. Defense today = org-scoped queries (mechanical,
   tested) + tenant middleware. Upgrade path: per-request `SET LOCAL app.current_org`
   requires wrapping handlers in transactions (invasive); schedule with the
   non-superuser DB role rollout.
2. **No CSP** — Next.js needs nonce wiring before a strict CSP won't break dev
   iteration; add when a frontend security pass happens.
3. **`webhook_secret` stored plaintext** — needed to verify signatures; encrypt at
   rest with KMS envelope encryption when running on AWS (Phase 7 deploy).
4. **Superuser dev role** — local `aaa` is superuser; production must use a
   restricted role (deploy checklist item 2).
5. **Raw CSV stored BYTEA** — ≤10MB cap enforced; move to object storage + virus
   scanning when files grow.
6. **No account lockout** (only rate limiting) — acceptable at 0.5 rps per IP;
   revisit with IP-reputation if exposed publicly.
7. **k6 covers happy-path reads/writes only** — no fuzzing; OWASP ZAP baseline
   run recommended before public launch.
