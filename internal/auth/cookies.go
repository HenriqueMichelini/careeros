package auth

import (
	"net/http"
	"time"
)

const (
	sessionCookieName = "__Host-careeros_session"
	csrfCookieName    = "__Host-careeros_csrf"
	oidcCookieName    = "__Host-careeros_oidc"
)

func sessionTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" || len(cookie.Value) > 128 {
		return "", false
	}
	return cookie.Value, true
}

func csrfTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || len(cookie.Value) > 128 {
		return ""
	}
	return cookie.Value
}

func oidcBindingFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(oidcCookieName)
	if err != nil || cookie.Value == "" || len(cookie.Value) > 128 {
		return ""
	}
	return cookie.Value
}

func setSessionCookies(w http.ResponseWriter, result LoginResult) {
	setSessionCookie(w, result.SessionToken, result.ExpiresAt)
	setCSRFCookie(w, result.CSRFToken, result.ExpiresAt)
}

func clearSessionCookies(w http.ResponseWriter) {
	clearSessionCookie(w)
	clearCSRFCookie(w)
}

func setOIDCBindingCookie(w http.ResponseWriter, binding string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcCookieName,
		Value:    binding,
		Path:     "/",
		MaxAge:   cookieMaxAge(expiresAt),
		Expires:  expiresAt.UTC(),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOIDCBindingCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func setSessionCookie(w http.ResponseWriter, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   cookieMaxAge(expiresAt),
		Expires:  expiresAt.UTC(),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func setCSRFCookie(w http.ResponseWriter, value string, expiresAt time.Time) {
	// The double-submit CSRF token must be readable by same-origin JavaScript.
	// It is not an authentication secret and is bound to the session server-side.
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   cookieMaxAge(expiresAt),
		Expires:  expiresAt.UTC(),
		Secure:   true,
		HttpOnly: false, // NOSONAR
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCSRFCookie(w http.ResponseWriter) {
	// Matches the intentionally script-readable double-submit CSRF cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		Secure:   true,
		HttpOnly: false, // NOSONAR
		SameSite: http.SameSiteLaxMode,
	})
}

func cookieMaxAge(expiresAt time.Time) int {
	maxAge := int(time.Until(expiresAt) / time.Second)
	if maxAge < 1 {
		return 1
	}
	return maxAge
}
