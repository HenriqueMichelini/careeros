package auth

import (
	"context"
	"net/http"
)

type principalContextKey struct{}

// Protect is the deny-by-default middleware for account-owned entrypoints.
// Safe methods require a live session. Unsafe methods additionally require an
// exact same-origin request and a per-session CSRF token in both the header
// and the CSRF cookie.
func (s *Service) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if unsafeMethod(r.Method) && !s.SameOrigin(r) {
			writeAuthError(w, ErrForbidden)
			return
		}
		token, ok := sessionTokenFromRequest(r)
		if !ok {
			writeAuthError(w, ErrAuthenticationRequired)
			return
		}
		principal, err := s.Authenticate(r.Context(), token)
		if err != nil {
			writeAuthError(w, ErrAuthenticationRequired)
			return
		}
		if unsafeMethod(r.Method) && !validRequestCSRF(s, r, principal) {
			writeAuthError(w, ErrForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
	})
}

// ProtectFunc adapts a handler that needs the authenticated principal.
func (s *Service) ProtectFunc(next func(http.ResponseWriter, *http.Request, Principal)) http.Handler {
	return s.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromRequest(r)
		if !ok {
			writeAuthError(w, ErrAuthenticationRequired)
			return
		}
		next(w, r, principal)
	}))
}

// PrincipalFromRequest returns the principal injected by Protect.
func PrincipalFromRequest(r *http.Request) (Principal, bool) {
	principal, ok := r.Context().Value(principalContextKey{}).(Principal)
	return principal, ok
}

// UnavailableHandler is the fail-closed fallback when a protected route is
// enabled before the auth service is configured.
func UnavailableHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeAuthError(w, ErrUnavailable)
	})
}

func validRequestCSRF(service *Service, r *http.Request, principal Principal) bool {
	header := r.Header.Get("X-CSRF-Token")
	cookie := csrfTokenFromRequest(r)
	if header == "" || cookie == "" || header != cookie {
		return false
	}
	return service.ValidateCSRF(principal, header)
}

func unsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
