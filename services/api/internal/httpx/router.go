package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/cocok/cocok/services/api/internal/organizations"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Auth         *AuthHandlers
	Orgs         *OrgHandlers
	Accounts     *AccountHandlers
	Imports      *ImportHandlers
	Recon        *ReconHandlers
	Exceptions   *ExceptionHandlers
	Settlements  *SettlementHandlers
	Reports      *ReportHandlers
	Integrations *IntegrationHandlers
	Webhooks     *WebhookHandlers
	AuditLogs    *AuditLogHandlers
	OrgsSvc      *organizations.Service
	DB           *pgxpool.Pool
	Log          *slog.Logger
	WebOrigin    string
}

// SecurityHeaders sets conservative browser hardening headers. CSP is
// intentionally omitted until Next.js nonce wiring exists (documented in
// docs/security-review.md); HSTS is ignored by browsers on plain HTTP.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func CORS(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin != "" && r.Header.Get("Origin") == origin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	metrics := newMetrics()
	r.Use(middleware.Recoverer)
	r.Use(WithRequestID)
	r.Use(SecurityHeaders)
	r.Use(middleware.RealIP)
	r.Use(metrics.Middleware)
	r.Use(middleware.Logger)
	r.Use(middleware.Compress(5))
	r.Use(CORS(d.WebOrigin))

	// Liveness + DB readiness in one probe.
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if d.DB != nil {
			ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
			defer cancel()
			if err := d.DB.Ping(ctx); err != nil {
				Err(w, req, http.StatusServiceUnavailable, "internal", "database unavailable", nil)
				return
			}
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/metrics", metrics.Handler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", d.Auth.Routes)

		// Everything below requires a session.
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(d.Auth.svc))

			r.Get("/auth/me", d.Auth.Me)

			r.Get("/organizations", d.Orgs.List)
			r.Post("/organizations", d.Orgs.Create)
			r.Post("/invitations/accept", d.Orgs.AcceptInvite)

			// Tenant-scoped routes: membership proven before handler.
			r.Route("/organizations/{orgID}", func(r chi.Router) {
				r.Use(WithOrg(d.OrgsSvc))

				r.Get("/", d.Orgs.Get)
				r.With(RequirePermission("org:manage")).Patch("/", d.Orgs.Update)
				r.Get("/members", d.Orgs.Members)
				r.With(RequirePermission("member:manage")).Post("/invitations", d.Orgs.Invite)
				r.With(RequirePermission("member:manage")).Patch("/members/role", d.Orgs.ChangeRole)

				// Phase 2: ingestion.
				r.With(RequirePermission("data:view")).Get("/accounts", d.Accounts.List)
				r.With(RequirePermission("account:manage")).Post("/accounts", d.Accounts.Create)
				r.With(RequirePermission("account:manage")).Patch("/accounts/{accountID}", d.Accounts.Update)

				r.With(RequirePermission("data:view")).Get("/imports", d.Imports.List)
				r.With(RequirePermission("import:run")).Post("/imports", d.Imports.Upload)
				r.With(RequirePermission("import:run")).Post("/imports/{importID}/commit", d.Imports.Commit)
				r.With(RequirePermission("data:view")).Get("/transactions", d.Imports.ListTransactions)

				// Phase 3: reconciliation.
				r.Route("/reconciliation", func(r chi.Router) {
					r.With(RequirePermission("reconcile:run")).Post("/runs", d.Recon.Run)
					r.With(RequirePermission("data:view")).Get("/groups", d.Recon.ListGroups)
					r.With(RequirePermission("data:view")).Get("/candidates", d.Recon.ListCandidates)
					r.With(RequirePermission("match:approve")).Post("/groups", d.Recon.ManualMatch)
					r.With(RequirePermission("match:approve")).Post("/groups/{groupID}/approve", d.Recon.Approve)
					r.With(RequirePermission("match:approve")).Post("/groups/{groupID}/reject", d.Recon.Reject)
					r.With(RequirePermission("match:approve")).Post("/groups/{groupID}/unmatch", d.Recon.Unmatch)
				})

				// Phase 4: exceptions + settlements.
				r.Route("/exceptions", func(r chi.Router) {
					r.With(RequirePermission("data:view")).Get("/", d.Exceptions.List)
					r.With(RequirePermission("exception:resolve")).Post("/", d.Exceptions.Flag)
					r.With(RequirePermission("exception:resolve")).Post("/{exceptionID}/resolve", d.Exceptions.Resolve)
					r.With(RequirePermission("exception:resolve")).Post("/{exceptionID}/dismiss", d.Exceptions.Dismiss)
				})
				r.Route("/settlements", func(r chi.Router) {
					r.With(RequirePermission("data:view")).Get("/", d.Settlements.List)
					r.With(RequirePermission("reconcile:run")).Post("/", d.Settlements.Create)
					r.With(RequirePermission("reconcile:run")).Patch("/{settlementID}", d.Settlements.Update)
					r.With(RequirePermission("reconcile:run")).Delete("/{settlementID}", d.Settlements.Delete)
				})

				// Phase 5: analytics & reporting.
				r.Route("/reports", func(r chi.Router) {
					r.With(RequirePermission("data:view")).Get("/summary", d.Reports.Summary)
					r.With(RequirePermission("report:export")).Get("/export", d.Reports.Export)
				})

				r.With(RequirePermission("audit:view")).Get("/audit-logs", d.AuditLogs.List)
				r.With(RequirePermission("data:view")).Get("/webhook-events", d.Webhooks.ListEvents)

				// Phase 6: provider integrations.
				r.Route("/integrations", func(r chi.Router) {
					r.With(RequirePermission("data:view")).Get("/", d.Integrations.List)
					r.With(RequirePermission("integration:manage")).Post("/", d.Integrations.Create)
					r.With(RequirePermission("integration:manage")).Delete("/{integrationID}", d.Integrations.Disable)
				})
			})
		})

		// Provider webhooks: public — signature verification is the auth.
		r.With(RateLimit(2, 20)).Post("/webhooks/{provider}/{integrationID}", d.Webhooks.Receive)
	})

	return r
}
