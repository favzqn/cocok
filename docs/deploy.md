# Deploy

## Stack

| Piece | How |
|---|---|
| API | `services/api/Dockerfile` — static Go binary, migrations run automatically on boot |
| Web | `apps/web/Dockerfile` — Next.js production build (`npm start`) |
| DB | Postgres 16 (managed RDS in AWS later; compose locally) |

## Local production-shaped run

```bash
export POSTGRES_PASSWORD=change-me
docker compose -f docker-compose.prod.yml up -d --build
# API  http://localhost:8081  (health: /healthz, probes: /metrics)
# Web  http://localhost:3000
```

## Environment (API)

| Var | Default | Notes |
|---|---|---|
| `DATABASE_URL` | — | required; migrations applied on every boot (goose) |
| `SERVER_ADDR` | `:8080` | |
| `WEB_ORIGIN` | `http://localhost:3000` | CORS allow-origin + links in emails |
| `SESSION_COOKIE_SECURE` | `false` | **`true` behind TLS** |
| `SESSION_TTL_HOURS` | `720` | sliding session lifetime |
| `AUTH_RATE_RPS` | `0.5` | auth endpoints per-IP; raise only for load tests |
| `LOG_LEVEL` | `info` | JSON logs to stdout |

## Checklist before real traffic

1. TLS terminator (ALB/NGINX) → set `SESSION_COOKIE_SECURE=true`, `WEB_ORIGIN=https://…`.
2. Managed Postgres with daily backups; app connects with a **non-superuser** role.
3. Ship `/metrics` to Prometheus; alert on 5xx rate + `/healthz` 503.
4. Load test: `k6 run -e BASE_URL=https://… loadtest/k6.js` (local reference: 5 VU / 15s, p95 ~10ms, 0% errors).
5. Webhook URLs: paste `webhook_url` from the Integrations panel into Xendit/Midtrans **sandbox** dashboards first; flip `mode` to live only with real credentials (not built yet — see IMPLEMENTATION_PLAN Phase 6 deferrals).
6. Email: swap `notifications.OutboxMailer` for an SMTP/SES implementation of `auth.Mailer` (interface already in place).

## Operations

- **Logs**: JSON on stdout — `request_id` correlates client → API.
- **Audit**: `audit_logs` is append-only enforced by DB trigger (UPDATE/DELETE raise).
- **DB down**: `/healthz` returns 503 (orchestrator restarts/skips the instance).
