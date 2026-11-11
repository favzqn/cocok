package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/cocok/cocok/services/api/internal/platform"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Err maps domain errors to the API error contract.
func Err(w http.ResponseWriter, r *http.Request, status int, code, msg string, details map[string]any) {
	JSON(w, status, errorBody{Error: errorDetail{
		Code: code, Message: msg, Details: details, RequestID: RequestID(r.Context()),
	}})
}

func Internal(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	log.Error("internal error", "request_id", RequestID(r.Context()), "error", err)
	Err(w, r, http.StatusInternalServerError, "internal", "internal server error", nil)
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	Err(w, r, http.StatusNotFound, "not_found", "resource not found", nil)
}

func BadRequest(w http.ResponseWriter, r *http.Request, msg string, details map[string]any) {
	Err(w, r, http.StatusBadRequest, "validation_failed", msg, details)
}

func Decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func IsNotFound(err error) bool { return errors.Is(err, platform.ErrNotFound) }
