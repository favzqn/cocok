package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/cocok/cocok/services/api/internal/organizations"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
)

type OrgHandlers struct {
	svc *organizations.Service
	log *slog.Logger
}

func NewOrgHandlers(svc *organizations.Service, log *slog.Logger) *OrgHandlers {
	return &OrgHandlers{svc: svc, log: log}
}

// List returns orgs the caller belongs to.
func (h *OrgHandlers) List(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	rows, err := h.svc.ListForUser(r.Context(), u.ID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"id":               row.ID.String(),
			"name":             row.Name,
			"slug":             row.Slug,
			"business_type":    row.BusinessType.String,
			"default_currency": row.DefaultCurrency,
			"timezone":         row.Timezone,
			"status":           row.Status,
			"role":             row.MemberRole,
			"created_at":       row.CreatedAt,
		})
	}
	JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *OrgHandlers) Create(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	var in organizations.CreateOrgInput
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	org, err := h.svc.CreateWithOwner(r.Context(), u.ID, in)
	if err != nil {
		switch {
		case errors.Is(err, organizations.ErrSlugTaken):
			Err(w, r, http.StatusConflict, "conflict", "slug already taken", nil)
		case errors.Is(err, platform.ErrValidation):
			BadRequest(w, r, err.Error(), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, http.StatusCreated, orgJSON(org))
}

// Get requires WithOrg (membership proven) — cross-org probes get 404 upstream.
func (h *OrgHandlers) Get(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	org, err := h.svc.Get(r.Context(), orgID)
	if err != nil {
		NotFound(w, r)
		return
	}
	v := orgJSON(org)
	v["role"] = RoleFrom(r.Context())
	JSON(w, http.StatusOK, v)
}

func (h *OrgHandlers) Members(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	rows, err := h.svc.ListMembers(r.Context(), orgID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		out = append(out, map[string]any{
			"id":         m.ID.String(),
			"user_id":    m.UserID.String(),
			"email":      string(m.Email),
			"full_name":  m.FullName,
			"role":       m.Role,
			"status":     m.Status,
			"created_at": m.CreatedAt,
		})
	}
	JSON(w, http.StatusOK, map[string]any{"data": out})
}

type inviteBody struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *OrgHandlers) Invite(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in inviteBody
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	raw, inv, err := h.svc.Invite(r.Context(), orgID, u.ID, organizations.InviteInput{Email: in.Email, Role: in.Role})
	if err != nil {
		if errors.Is(err, platform.ErrValidation) {
			BadRequest(w, r, err.Error(), nil)
			return
		}
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{
		"id":           inv.ID.String(),
		"email":        string(inv.Email),
		"role":         inv.Role,
		"expires_at":   inv.ExpiresAt,
		"invite_token": raw, // shown once; only hash is stored
	})
}

type changeRoleBody struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *OrgHandlers) ChangeRole(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in changeRoleBody
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	target, err := uuid.Parse(in.UserID)
	if err != nil {
		BadRequest(w, r, "invalid user_id", nil)
		return
	}
	if err := h.svc.ChangeRole(r.Context(), orgID, u.ID, target, in.Role); err != nil {
		switch {
		case errors.Is(err, organizations.ErrLastOwner):
			Err(w, r, http.StatusConflict, "conflict", err.Error(), nil)
		case errors.Is(err, platform.ErrValidation), errors.Is(err, platform.ErrNotFound):
			BadRequest(w, r, err.Error(), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type acceptInviteBody struct {
	Token string `json:"token"`
}

// AcceptInvite requires auth only (not org scoping — joining is pre-membership).
func (h *OrgHandlers) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	var in acceptInviteBody
	if err := Decode(r, &in); err != nil || in.Token == "" {
		BadRequest(w, r, "token is required", nil)
		return
	}
	org, role, err := h.svc.AcceptInvite(r.Context(), u, in.Token)
	if err != nil {
		if errors.Is(err, platform.ErrValidation) {
			BadRequest(w, r, err.Error(), nil)
			return
		}
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"organization": orgJSON(org), "role": role})
}

// Update patches org settings (name/timezone) — org:manage.
func (h *OrgHandlers) Update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in organizations.UpdateSettingsInput
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	org, err := h.svc.UpdateSettings(r.Context(), orgID, u.ID, in)
	if err != nil {
		switch {
		case errors.Is(err, platform.ErrNotFound):
			NotFound(w, r)
		case errors.Is(err, platform.ErrValidation):
			BadRequest(w, r, stripValidationPrefix(err), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, http.StatusOK, orgJSON(org))
}

func orgJSON(o *db.Organization) map[string]any {
	return map[string]any{
		"id":                o.ID.String(),
		"name":              o.Name,
		"slug":              o.Slug,
		"business_type":     o.BusinessType.String,
		"default_currency":  o.DefaultCurrency,
		"timezone":          o.Timezone,
		"subscription_plan": o.SubscriptionPlan,
		"status":            o.Status,
		"created_at":        o.CreatedAt,
	}
}
