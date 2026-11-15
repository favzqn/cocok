package httpx

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/connectors"
	"github.com/cocok/cocok/services/api/internal/integrations"
	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/cocok/cocok/services/api/internal/webhooks"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type IntegrationHandlers struct {
	svc *integrations.Service
	log *slog.Logger
}

func NewIntegrationHandlers(svc *integrations.Service, log *slog.Logger) *IntegrationHandlers {
	return &IntegrationHandlers{svc: svc, log: log}
}

func (h *IntegrationHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	rows, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (h *IntegrationHandlers) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in integrations.CreateInput
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	out, err := h.svc.Create(r.Context(), orgID, u.ID, in)
	if err != nil {
		switch {
		case errors.Is(err, integrations.ErrDuplicate):
			Err(w, r, http.StatusConflict, "conflict", err.Error(), nil)
		case errors.Is(err, platform.ErrValidation):
			BadRequest(w, r, stripValidationPrefix(err), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, http.StatusCreated, out)
}

func (h *IntegrationHandlers) Disable(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "integrationID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	if err := h.svc.Disable(r.Context(), orgID, u.ID, id); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			NotFound(w, r)
			return
		}
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AuditLogHandlers exposes the org's append-only audit trail.
type AuditLogHandlers struct {
	svc *audit.Writer
	log *slog.Logger
}

func NewAuditLogHandlers(svc *audit.Writer, log *slog.Logger) *AuditLogHandlers {
	return &AuditLogHandlers{svc: svc, log: log}
}

func (h *AuditLogHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	rows, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// WebhookHandlers receives provider callbacks (no session — signature IS auth).
type WebhookHandlers struct {
	svc *webhooks.Service
	log *slog.Logger
}

func NewWebhookHandlers(svc *webhooks.Service, log *slog.Logger) *WebhookHandlers {
	return &WebhookHandlers{svc: svc, log: log}
}

func (h *WebhookHandlers) Receive(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	integrationID := chi.URLParam(r, "integrationID")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		BadRequest(w, r, "cannot read body", nil)
		return
	}
	res, err := h.svc.Handle(r.Context(), provider, integrationID, r.Header, body)
	if err != nil {
		switch {
		case errors.Is(err, webhooks.ErrNotFound):
			NotFound(w, r)
		case errors.Is(err, connectors.ErrBadSignature):
			Err(w, r, http.StatusUnauthorized, "unauthorized", "invalid signature", nil)
		case errors.Is(err, connectors.ErrUnknownEvent):
			BadRequest(w, r, err.Error(), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, res.Status, res)
}
