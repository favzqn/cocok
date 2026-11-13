package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/cocok/cocok/services/api/internal/exceptions"
	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ExceptionHandlers struct {
	svc *exceptions.Service
	log *slog.Logger
}

func NewExceptionHandlers(svc *exceptions.Service, log *slog.Logger) *ExceptionHandlers {
	return &ExceptionHandlers{svc: svc, log: log}
}

func (h *ExceptionHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	status, typ := r.URL.Query().Get("status"), r.URL.Query().Get("type")
	if status != "" && status != "open" && status != "resolved" && status != "dismissed" {
		BadRequest(w, r, "status must be open, resolved or dismissed", nil)
		return
	}
	if typ != "" && typ != "unmatched" && typ != "near_miss" && typ != "manual" {
		BadRequest(w, r, "type must be unmatched, near_miss or manual", nil)
		return
	}
	rows, err := h.svc.List(r.Context(), orgID, status, typ)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": rows})
}

type flagExceptionBody struct {
	TransactionIDs []string `json:"transaction_ids"`
	Note           string   `json:"note,omitempty"`
}

func (h *ExceptionHandlers) Flag(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in flagExceptionBody
	if err := Decode(r, &in); err != nil || len(in.TransactionIDs) == 0 {
		BadRequest(w, r, "transaction_ids is required", nil)
		return
	}
	ids, err := parseIDs(in.TransactionIDs)
	if err != nil {
		BadRequest(w, r, "invalid transaction id", nil)
		return
	}
	exc, err := h.svc.Flag(r.Context(), orgID, u.ID, ids, in.Note)
	if err != nil {
		switch {
		case errors.Is(err, platform.ErrNotFound), errors.Is(err, exceptions.ErrAlreadyOpen):
			if errors.Is(err, exceptions.ErrAlreadyOpen) {
				Err(w, r, http.StatusConflict, "conflict", err.Error(), nil)
			} else {
				NotFound(w, r)
			}
		case errors.Is(err, platform.ErrValidation):
			BadRequest(w, r, stripValidationPrefix(err), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, http.StatusCreated, exc)
}

type decideExceptionBody struct {
	Note string `json:"note,omitempty"`
}

func (h *ExceptionHandlers) Resolve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, "resolved")
}

func (h *ExceptionHandlers) Dismiss(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, "dismissed")
}

func (h *ExceptionHandlers) decide(w http.ResponseWriter, r *http.Request, status string) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "exceptionID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	var in decideExceptionBody
	_ = Decode(r, &in) // body optional
	if err := h.svc.Decide(r.Context(), orgID, u.ID, id, status, in.Note); err != nil {
		switch {
		case errors.Is(err, platform.ErrNotFound):
			NotFound(w, r)
		case errors.Is(err, exceptions.ErrNotOpen):
			Err(w, r, http.StatusConflict, "conflict", err.Error(), nil)
		case errors.Is(err, platform.ErrValidation):
			BadRequest(w, r, stripValidationPrefix(err), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func parseIDs(ss []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(ss))
	for _, s := range ss {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
