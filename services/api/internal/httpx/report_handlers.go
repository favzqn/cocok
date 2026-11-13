package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/cocok/cocok/services/api/internal/reports"
	"github.com/google/uuid"
)

type ReportHandlers struct {
	svc *reports.Service
	log *slog.Logger
}

func NewReportHandlers(svc *reports.Service, log *slog.Logger) *ReportHandlers {
	return &ReportHandlers{svc: svc, log: log}
}

func (h *ReportHandlers) Summary(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	out, err := h.svc.Summary(r.Context(), orgID)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// Export streams a CSV report as an attachment (?kind=transactions|matches|exceptions|settlements).
func (h *ReportHandlers) Export(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	kind := r.URL.Query().Get("kind")
	name, data, err := h.svc.Export(r.Context(), orgID, kind)
	if err != nil {
		if errors.Is(err, reports.ErrUnknownKind) {
			BadRequest(w, r, "kind must be transactions, matches, exceptions or settlements", nil)
			return
		}
		Internal(w, r, h.log, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// keep uuid import used (future filters)
var _ = uuid.Nil
var _ = platform.ErrValidation
