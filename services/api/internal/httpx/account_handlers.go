package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/cocok/cocok/services/api/internal/accounts"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AccountHandlers struct {
	svc *accounts.Service
	log *slog.Logger
}

func NewAccountHandlers(svc *accounts.Service, log *slog.Logger) *AccountHandlers {
	return &AccountHandlers{svc: svc, log: log}
}

func (h *AccountHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	rows, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, a := range rows {
		out = append(out, accountJSON(a))
	}
	JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *AccountHandlers) Create(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in accounts.CreateInput
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	acc, err := h.svc.Create(r.Context(), orgID, u.ID, in)
	if err != nil {
		if errors.Is(err, platform.ErrValidation) {
			BadRequest(w, r, stripValidationPrefix(err), nil)
			return
		}
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusCreated, accountJSON(*acc))
}

func (h *AccountHandlers) Update(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	var in accounts.UpdateInput
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	acc, err := h.svc.Update(r.Context(), orgID, u.ID, id, in)
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
	JSON(w, http.StatusOK, accountJSON(*acc))
}

func accountJSON(a db.FinancialAccount) map[string]any {
	return map[string]any{
		"id":             a.ID.String(),
		"name":           a.Name,
		"kind":           a.Kind,
		"account_number": a.AccountNumber,
		"currency":       a.Currency,
		"status":         a.Status,
		"created_at":     a.CreatedAt,
		"updated_at":     a.UpdatedAt,
	}
}
