package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/cocok/cocok/services/api/internal/imports"
	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ImportHandlers struct {
	svc *imports.Service
	log *slog.Logger
}

func NewImportHandlers(svc *imports.Service, log *slog.Logger) *ImportHandlers {
	return &ImportHandlers{svc: svc, log: log}
}

// Upload accepts multipart/form-data: file + account_id + mapping (JSON).
func (h *ImportHandlers) Upload(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 12<<20) // file + form overhead
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		BadRequest(w, r, "invalid multipart form (max ~10MB file)", nil)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		BadRequest(w, r, "file is required", nil)
		return
	}
	defer file.Close()
	raw := make([]byte, 0, 1<<20)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := file.Read(buf)
		raw = append(raw, buf[:n]...)
		if len(raw) > 10<<20 {
			BadRequest(w, r, "file exceeds 10MB", nil)
			return
		}
		if rerr != nil {
			break
		}
	}

	accID, err := uuid.Parse(r.FormValue("account_id"))
	if err != nil {
		BadRequest(w, r, "account_id is required", nil)
		return
	}
	var m imports.Mapping
	if ms := r.FormValue("mapping"); ms != "" {
		if err := json.Unmarshal([]byte(ms), &m); err != nil {
			BadRequest(w, r, "mapping must be a JSON object", nil)
			return
		}
	}
	if strings.TrimSpace(m.Date) == "" || strings.TrimSpace(m.Amount) == "" {
		BadRequest(w, r, "mapping.date and mapping.amount are required", map[string]any{
			"required": []string{"date", "amount"},
		})
		return
	}

	prev, err := h.svc.Upload(r.Context(), orgID, u.ID, imports.UploadInput{
		AccountID: accID,
		Filename:  hdr.Filename,
		Raw:       raw,
		Mapping:   m,
	})
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
	JSON(w, http.StatusCreated, prev)
}

func (h *ImportHandlers) Commit(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	u := UserFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "importID"))
	if err != nil {
		NotFound(w, r)
		return
	}
	out, err := h.svc.Commit(r.Context(), orgID, u.ID, id)
	if err != nil {
		switch {
		case errors.Is(err, platform.ErrNotFound):
			NotFound(w, r)
		case errors.Is(err, platform.ErrValidation):
			Err(w, r, http.StatusConflict, "conflict", stripValidationPrefix(err), nil)
		default:
			Internal(w, r, h.log, err)
		}
		return
	}
	JSON(w, http.StatusOK, out)
}

func (h *ImportHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	rows, err := h.svc.ListBatches(r.Context(), orgID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, b := range rows {
		out = append(out, map[string]any{
			"id":              b.ID.String(),
			"account_id":      b.AccountID.String(),
			"filename":        b.Filename,
			"status":          b.Status,
			"row_count":       b.RowCount,
			"valid_count":     b.ValidCount,
			"invalid_count":   b.InvalidCount,
			"inserted_count":  b.InsertedCount,
			"duplicate_count": b.DuplicateCount,
			"created_at":      b.CreatedAt,
			"committed_at":    b.CommittedAt,
		})
	}
	JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *ImportHandlers) ListTransactions(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	var accFilter uuid.UUID
	if v := r.URL.Query().Get("account_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			BadRequest(w, r, "invalid account_id", nil)
			return
		}
		accFilter = id
	}
	limit := int32(50)
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			BadRequest(w, r, "invalid limit", nil)
			return
		}
		limit = int32(n)
	}
	rows, err := h.svc.ListTransactions(r.Context(), orgID, accFilter, limit)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, t := range rows {
		out = append(out, map[string]any{
			"id":              t.ID.String(),
			"account_id":      t.AccountID.String(),
			"import_batch_id": t.ImportBatchID,
			"booked_at":       t.BookedAt,
			"direction":       t.Direction,
			"amount_minor":    t.AmountMinor,
			"currency":        t.Currency,
			"description":     t.Description,
			"counterparty":    t.Counterparty,
			"external_ref":    t.ExternalRef,
			"created_at":      t.CreatedAt,
		})
	}
	JSON(w, http.StatusOK, map[string]any{"data": out})
}
