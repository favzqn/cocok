package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/cocok/cocok/services/api/internal/auth"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/go-chi/chi/v5"
)

type AuthHandlers struct {
	svc    *auth.Service
	log    *slog.Logger
	secure bool
}

func NewAuthHandlers(svc *auth.Service, log *slog.Logger, secure bool) *AuthHandlers {
	return &AuthHandlers{svc: svc, log: log, secure: secure}
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name,omitempty"`
}

// Register creates the user then issues a session (register+login in one step).
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	u, err := h.svc.Register(r.Context(), auth.RegisterInput{
		Email: in.Email, Password: in.Password, FullName: in.FullName,
	})
	if err != nil {
		h.writeSvcErr(w, r, err)
		return
	}
	res, err := h.svc.Login(r.Context(), in.Email, in.Password, r.UserAgent(), auth.ClientIP(r))
	if err != nil {
		// user created but session failed — still return created, client can login
		JSON(w, http.StatusCreated, userJSON(u))
		return
	}
	h.setSessionCookie(w, res.RawToken, res.ExpiresAt)
	JSON(w, http.StatusCreated, userJSON(res.User))
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if err := Decode(r, &in); err != nil {
		BadRequest(w, r, "invalid JSON body", nil)
		return
	}
	res, err := h.svc.Login(r.Context(), in.Email, in.Password, r.UserAgent(), auth.ClientIP(r))
	if err != nil {
		h.writeSvcErr(w, r, err)
		return
	}
	h.setSessionCookie(w, res.RawToken, res.ExpiresAt)
	JSON(w, http.StatusOK, userJSON(res.User))
}

func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			h.log.Error("logout revoke failed", "error", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode,
	})
	JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Refresh rotates the session token (old revoked, new cookie issued).
func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(auth.CookieName)
	if err != nil || c.Value == "" {
		Err(w, r, http.StatusUnauthorized, "unauthorized", "no session", nil)
		return
	}
	res, err := h.svc.Refresh(r.Context(), c.Value, r.UserAgent(), auth.ClientIP(r))
	if err != nil {
		Err(w, r, http.StatusUnauthorized, "unauthorized", "invalid or expired session", nil)
		return
	}
	h.setSessionCookie(w, res.RawToken, res.ExpiresAt)
	JSON(w, http.StatusOK, userJSON(res.User))
}

func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		Err(w, r, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
		return
	}
	JSON(w, http.StatusOK, userJSON(u))
}

func (h *AuthHandlers) setSessionCookie(w http.ResponseWriter, raw string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    raw,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandlers) writeSvcErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrDisabled), errors.Is(err, auth.ErrInvalidSession):
		Err(w, r, http.StatusUnauthorized, "unauthorized", "invalid email or password", nil)
	case errors.Is(err, auth.ErrEmailTaken):
		Err(w, r, http.StatusConflict, "conflict", "email already registered", nil)
	case errors.Is(err, platform.ErrValidation):
		BadRequest(w, r, stripValidationPrefix(err), nil)
	default:
		Internal(w, r, h.log, err)
	}
}

func stripValidationPrefix(err error) string {
	msg := err.Error()
	const p = "validation failed: "
	if len(msg) > len(p) && msg[:len(p)] == p {
		return msg[len(p):]
	}
	return msg
}

func userJSON(u *db.User) map[string]any {
	return map[string]any{
		"id":         u.ID.String(),
		"email":      string(u.Email),
		"full_name":  u.FullName,
		"status":     u.Status,
		"created_at": u.CreatedAt,
	}
}

// Routes mounts auth endpoints under a chi router.
func (h *AuthHandlers) Routes(r chi.Router) {
	r.Use(RateLimit(0.5, 10)) // 0.5 rps, burst 10 per IP
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/logout", h.Logout)
	r.Post("/refresh", h.Refresh)
	r.Get("/me", h.Me)
}
