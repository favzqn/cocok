package httpx

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

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

// parseRange reads ?from=&to= as YYYY-MM-DD (Asia/Jakarta). to is exclusive
// next-day midnight; either side may be empty.
func parseRange(r *http.Request) (from, to *time.Time, err error) {
	loc, lerr := time.LoadLocation("Asia/Jakarta")
	if lerr != nil {
		return nil, nil, lerr
	}
	if v := r.URL.Query().Get("from"); v != "" {
		t, perr := time.ParseInLocation("2006-01-02", v, loc)
		if perr != nil {
			return nil, nil, fmt.Errorf("bad from")
		}
		from = &t
	}
	if v := r.URL.Query().Get("to"); v != "" {
		t, perr := time.ParseInLocation("2006-01-02", v, loc)
		if perr != nil {
			return nil, nil, fmt.Errorf("bad to")
		}
		end := t.AddDate(0, 0, 1) // exclusive
		to = &end
	}
	return from, to, nil
}

func (h *ReportHandlers) Summary(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	from, to, perr := parseRange(r)
	if perr != nil {
		BadRequest(w, r, "from/to must be YYYY-MM-DD", nil)
		return
	}
	out, err := h.svc.Summary(r.Context(), orgID, from, to)
	if err != nil {
		Internal(w, r, h.log, err)
		return
	}
	JSON(w, http.StatusOK, out)
}

// Export streams a CSV report as an attachment (?kind=transactions|matches|exceptions|settlements).
func (h *ReportHandlers) Export(w http.ResponseWriter, r *http.Request) {
	orgID, _ := OrgFrom(r.Context())
	from, to, perr := parseRange(r)
	if perr != nil {
		BadRequest(w, r, "from/to must be YYYY-MM-DD", nil)
		return
	}
	kind := r.URL.Query().Get("kind")
	name, data, err := h.svc.Export(r.Context(), orgID, kind, from, to)
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
