package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/cocok/cocok/services/api/internal/reconciliation"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ReconHandlers struct {
	svc *reconciliation.Service
	log *slog.Logger
}

func NewReconHandlers(svc *reconciliation.Service, log *slog.Logger) *ReconHandlers {
	return &ReconHandlers{svc: svc, log: log}
}

// Run triggers candidate generation + proposed groups.
func (h *ReconHandlers) Run(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	out, err := h.svc.Run(r.Context(), orgID, u.ID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

func (h *ReconHandlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	status := r.URL.Query().Get("status")
	if status != "" && status != "proposed" && status != "approved" && status != "rejected" {
		BadRequest(w, r, "status must be proposed, approved or rejected", nil)
		return
	}
	groups, err := h.svc.ListGroups(r.Context(), orgID, status)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": groups})
}

func (h *ReconHandlers) ListCandidates(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	cands, err := h.svc.ListCandidates(r.Context(), orgID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"data": cands})
}

type manualMatchBody struct {
	TransactionIDs []string `json:"transaction_ids"`
}

func (h *ReconHandlers) ManualMatch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	var in manualMatchBody
	if err := Decode(r, &in); err != nil || len(in.TransactionIDs) == 0 {
		BadRequest(w, r, "transaction_ids is required", nil)
		return
	}
	ids := make([]uuid.UUID, 0, len(in.TransactionIDs))
	for _, s := range in.TransactionIDs {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			BadRequest(w, r, "invalid transaction id", nil)
			return
		}
		ids = append(ids, id)
	}
	group, err := h.svc.ManualMatch(r.Context(), orgID, u.ID, ids)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	JSON(w, http.StatusCreated, group)
}

func (h *ReconHandlers) Approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, true)
}

func (h *ReconHandlers) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, false)
}

func (h *ReconHandlers) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "groupID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	if err := h.svc.Decide(r.Context(), orgID, u.ID, id, approve); err != nil {
		h.writeErr(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ReconHandlers) Unmatch(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "groupID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	if err := h.svc.Unmatch(r.Context(), orgID, u.ID, id); err != nil {
		h.writeErr(w, r, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ReconHandlers) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, platform.ErrNotFound):
		NotFound(w, r)
	case errors.Is(err, reconciliation.ErrConflict), errors.Is(err, reconciliation.ErrNoProposedGroup):
		Err(w, r, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, reconciliation.ErrUnbalanced):
		Err(w, r, http.StatusUnprocessableEntity, "validation_failed", err.Error(), nil)
	case errors.Is(err, platform.ErrValidation):
		BadRequest(w, r, stripValidationPrefix(err), nil)
	default:
		Internal(w, r, h.log, err)
	}
}
