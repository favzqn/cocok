package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/cocok/cocok/services/api/internal/settlements"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SettlementHandlers struct {
	svc *settlements.Service
	log *slog.Logger
}

func NewSettlementHandlers(svc *settlements.Service, log *slog.Logger) *SettlementHandlers {
	return &SettlementHandlers{svc: svc, log: log}
}

func (h *SettlementHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	status := r.URL.Query().Get("status")
	rows, err := h.svc.List(r.Context(), orgID, status)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (h *SettlementHandlers) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in settlements.CreateInput
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	s, err := h.svc.Create(r.Context(), orgID, u.ID, in)
	if err != nil {
		if errors.Is(err, platform.ErrValidation) {
			BadRequest(w, r, stripValidationPrefix(err), nil)
			return
		}
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusCreated, s)
}

func (h *SettlementHandlers) Update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	var in settlements.UpdateInput
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	s, err := h.svc.Update(r.Context(), orgID, u.ID, id, in)
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
	JSON(w, http.StatusOK, s)
}

func (h *SettlementHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	if err := h.svc.Delete(r.Context(), orgID, u.ID, id); err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			NotFound(w, r)
			return
		}
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
