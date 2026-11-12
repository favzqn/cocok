package httpx

import (
	"log/slog"
	"net/http"

	"github.com/cocok/cocok/services/api/internal/organizations"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	Auth      *AuthHandlers
	Orgs      *OrgHandlers
	Accounts  *AccountHandlers
	Imports   *ImportHandlers
	Recon     *ReconHandlers
	OrgsSvc   *organizations.Service
	Log       *slog.Logger
	WebOrigin string
}

// CORS allows the web app origin to send credentialed requests.
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
	r.Use(middleware.Recoverer)
	r.Use(WithRequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Compress(5))
	r.Use(CORS(d.WebOrigin))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

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
			})
		})
	})

	return r
}
