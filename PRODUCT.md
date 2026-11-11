# COCOK — Product Truth

## Product

COCOK: B2B financial reconciliation SaaS for Indonesian businesses. Connects bank statements, payment gateways (Xendit/Midtrans), e-wallets, marketplaces, and invoicing ledgers; normalizes into one canonical transaction model; auto-matches; surfaces exceptions; tracks settlements; full audit trail.

**Position:** "Import financial data from anywhere. Automatically reconcile everything. Investigate only what doesn't match."

**Not:** a bank, payment processor, fund holder, or credential store. Data processing + reconciliation only.

## Platform

`web` — Next.js responsive web app, desktop-first for finance operators.

## Users & Jobs

- **Primary:** Indonesian SMEs with multi-channel payments (bank + QRIS + gateways + marketplaces). Job: close the books without Excel hell.
- **Secondary:** finance/accounting teams, large volumes, auditability needs.
- **Tertiary:** accounting firms, multi-client workspaces.

## Durable Facts (must preserve)

- Currency default **IDR**, timezone **Asia/Jakarta**, locale **id-ID**.
- Money = **integer minor units** (`amount_minor BIGINT`). Never float.
- Multi-tenant: every owned entity carries `organization_id`; cross-org access is a security bug.
- Roles: owner, admin, finance_manager, finance_staff, viewer — permission-based, not hardcoded checks.
- Reconciliation = relationship between records, never mutates source transactions.
- Deterministic matching first; AI/ML deferred until P0 solid.
- Audit log append-only via application interfaces.
- Monolith: Go + Chi + pgx + sqlc + PostgreSQL + Redis; Next.js frontend. No microservices.

## Stack

- **Frontend:** Next.js (App Router), TypeScript strict, Tailwind, shadcn/ui, TanStack Query/Table, React Hook Form, Zod, Recharts.
- **Backend:** Go, Chi, pgx, sqlc, PostgreSQL, Redis, Asynq (jobs, later phase).
- **Infra:** Docker Compose (local), AWS + Terraform (later phase), GitHub Actions CI.

## Design Direction

Enterprise fintech ops: deep navy/charcoal nav, light neutral canvas, white surfaces, restrained green (matched) / amber (review) / red (discrepancy), tabular numerals for money. Data-dense, minimal, trustworthy. No gradients/glassmorphism/oversized empty cards. (Inferred from PRD §11 — confirmed by PRD, not separately interviewed.)

## Assumptions (labeled)

- Design direction, pricing tiers, and phase ordering taken verbatim from PRD v1.0; not re-interviewed.
- Session auth = opaque server-side sessions (DB-stored, hashed tokens, httpOnly cookie) with rotation endpoint; satisfies PRD refresh requirement with fewer moving parts than JWT pair. Documented in `docs/architecture/security-model.md`.
- Provider adapters (Xendit/Midtrans) ship behind real adapter interfaces with clearly labeled sandbox implementations until credentials exist (PRD §25).
