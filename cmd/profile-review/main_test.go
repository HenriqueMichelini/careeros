package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func clearAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CAREEROS_PUBLIC_ORIGIN", "")
	t.Setenv("CAREEROS_GOOGLE_CLIENT_ID", "")
	t.Setenv("CAREEROS_GOOGLE_CLIENT_SECRET", "")
	t.Setenv("CAREEROS_GOOGLE_REDIRECT_URI", "")
	t.Setenv("CAREEROS_REQUIRE_AUTH", "")
}

func TestNewHandlerPreservesGuestBYOKWhenAuthIsNotRequired(t *testing.T) {
	clearAuthEnv(t)
	handler := newHandler()
	request := httptest.NewRequest(http.MethodPost, "/api/cv/generate", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want the existing 401 key response", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body["error"] != "key" {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestNewHandlerFailsClosedWhenAuthIsRequiredButUnconfigured(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("CAREEROS_REQUIRE_AUTH", "1")
	handler := newHandler()
	request := httptest.NewRequest(http.MethodPost, "/api/profile/review", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body["error"] != "auth_unavailable" {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestNewHandlerMountsAuthRoutes(t *testing.T) {
	clearAuthEnv(t)
	handler := newHandler()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/google/start", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 without provider configuration", recorder.Code)
	}
}
