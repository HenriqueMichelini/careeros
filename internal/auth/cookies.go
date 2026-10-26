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
	setCookie(w, sessionCookieName, result.SessionToken, true, result.ExpiresAt)
	setCookie(w, csrfCookieName, result.CSRFToken, false, result.ExpiresAt)
}

func clearSessionCookies(w http.ResponseWriter) {
	clearCookie(w, sessionCookieName, true)
	clearCookie(w, csrfCookieName, false)
}

func setOIDCBindingCookie(w http.ResponseWriter, binding string, expiresAt time.Time) {
	setCookie(w, oidcCookieName, binding, true, expiresAt)
}

func clearOIDCBindingCookie(w http.ResponseWriter) {
	clearCookie(w, oidcCookieName, true)
}

func setCookie(w http.ResponseWriter, name, value string, httpOnly bool, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt) / time.Second)
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  expiresAt.UTC(),
		Secure:   true,
		HttpOnly: httpOnly,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, name string, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		Secure:   true,
		HttpOnly: httpOnly,
		SameSite: http.SameSiteLaxMode,
	})
}
