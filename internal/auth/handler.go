package auth

import (
	"encoding/json"
	"errors"
	"net/http"
)

// NewHandler serves the allowlisted OIDC start/callback, session probe and
// logout routes. A nil service keeps every route fail-closed with 503.
func NewHandler(service *Service) http.Handler {
	handler := &Handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/{provider}/start", handler.start)
	mux.HandleFunc("GET /api/auth/{provider}/callback", handler.callback)
	mux.HandleFunc("GET /api/auth/session", handler.session)
	mux.HandleFunc("POST /api/auth/logout", handler.logout)
	mux.HandleFunc("POST /api/auth/logout-all", handler.logoutAll)
	return withAuthHeaders(mux)
}

type Handler struct {
	service *Service
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeAuthError(w, ErrUnavailable)
		return
	}
	providerID := r.PathValue("provider")
	var current *Principal
	if token, ok := sessionTokenFromRequest(r); ok {
		if principal, err := h.service.Authenticate(r.Context(), token); err == nil {
			current = &principal
		}
	}
	start, err := h.service.BeginLogin(r.Context(), providerID, r.URL.Query().Get("return_to"), current)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	setOIDCBindingCookie(w, start.BindingToken, start.ExpiresAt)
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, start.URL, http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeAuthError(w, ErrUnavailable)
		return
	}
	providerID := r.PathValue("provider")
	query := r.URL.Query()
	binding := oidcBindingFromRequest(r)
	if query.Get("error") != "" {
		h.service.CancelLogin(r.Context(), query.Get("state"))
		clearOIDCBindingCookie(w)
		writeAuthError(w, ErrInvalidCallback)
		return
	}
	result, err := h.service.CompleteLogin(r.Context(), providerID, query.Get("state"), query.Get("code"), binding)
	if err != nil {
		clearOIDCBindingCookie(w)
		writeAuthError(w, err)
		return
	}
	setSessionCookies(w, result)
	clearOIDCBindingCookie(w)
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, result.ReturnTo, http.StatusSeeOther)
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeAuthError(w, ErrUnavailable)
		return
	}
	w.Header().Set("Vary", "Cookie")
	token, ok := sessionTokenFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusOK, SessionView{Authenticated: false})
		return
	}
	principal, err := h.service.Authenticate(r.Context(), token)
	if err != nil {
		clearSessionCookies(w)
		writeJSON(w, http.StatusOK, SessionView{Authenticated: false})
		return
	}
	writeJSON(w, http.StatusOK, h.service.View(&principal))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		clearSessionCookies(w)
		writeAuthError(w, ErrUnavailable)
		return
	}
	if !h.service.SameOrigin(r) {
		writeAuthError(w, ErrForbidden)
		return
	}
	if token, ok := sessionTokenFromRequest(r); ok {
		if principal, err := h.service.Authenticate(r.Context(), token); err == nil {
			if !h.validCSRF(r, principal) {
				writeAuthError(w, ErrForbidden)
				return
			}
			if err := h.service.Logout(r.Context(), token); err != nil {
				writeAuthError(w, err)
				return
			}
		}
	}
	clearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeAuthError(w, ErrUnavailable)
		return
	}
	if !h.service.SameOrigin(r) {
		writeAuthError(w, ErrForbidden)
		return
	}
	token, ok := sessionTokenFromRequest(r)
	if !ok {
		writeAuthError(w, ErrAuthenticationRequired)
		return
	}
	principal, err := h.service.Authenticate(r.Context(), token)
	if err != nil {
		clearSessionCookies(w)
		writeAuthError(w, ErrAuthenticationRequired)
		return
	}
	if !h.validCSRF(r, principal) {
		writeAuthError(w, ErrForbidden)
		return
	}
	if _, err := h.service.LogoutAll(r.Context(), principal); err != nil {
		if errors.Is(err, ErrAuthenticationRequired) {
			clearSessionCookies(w)
		}
		writeAuthError(w, err)
		return
	}
	clearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) validCSRF(r *http.Request, principal Principal) bool {
	header := r.Header.Get("X-CSRF-Token")
	cookie := csrfTokenFromRequest(r)
	if header == "" || cookie == "" || header != cookie {
		return false
	}
	return h.service.ValidateCSRF(principal, header)
}

func withAuthHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "auth_error"
	switch {
	case errors.Is(err, ErrProviderNotAllowed):
		status, code = http.StatusNotFound, "provider_not_allowed"
	case errors.Is(err, ErrInvalidTransaction), errors.Is(err, ErrInvalidCallback):
		status, code = http.StatusBadRequest, "invalid_callback"
	case errors.Is(err, ErrAuthenticationRequired):
		status, code = http.StatusUnauthorized, "authentication_required"
	case errors.Is(err, ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrCapacity):
		status, code = http.StatusServiceUnavailable, "auth_unavailable"
	}
	writeJSON(w, status, map[string]string{"error": code})
}
