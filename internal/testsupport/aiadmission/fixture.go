// Package aiadmission contains synthetic test fixtures only. No production
// constructor imports it; every entrypoint requires *testing.T and a local DSN.
package aiadmission

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/qualificationmatching"
)

const Origin = "http://127.0.0.1:8787"

type Fixture struct {
	DSN               string
	DB                *sql.DB
	Store             *accountstore.Store
	Service           *auth.Service
	Login             auth.LoginResult
	Principal         auth.Principal
	Document          json.RawMessage
	ProfileProjection map[string]any
	Qualifications    map[string]any
}

type fakeOIDC struct{}

func (fakeOIDC) ID() string { return "google" }
func (fakeOIDC) AuthorizationURL(state, nonce, challenge, redirect string) string {
	return "https://synthetic.invalid/authorize?state=" + url.QueryEscape(state)
}
func (fakeOIDC) Exchange(_ context.Context, code, verifier, redirect, nonce string) (auth.Identity, error) {
	return auth.Identity{Provider: "google", Issuer: "https://accounts.google.com", Subject: code, Email: "synthetic@example.invalid", Name: "Synthetic", EmailVerified: true}, nil
}

func NewFixture(t *testing.T) *Fixture {
	t.Helper()
	dsn := os.Getenv("CAREEROS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CAREEROS_TEST_DATABASE_URL required for actual PostgreSQL acceptance")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	host := u.Query().Get("host")
	if host == "" {
		host = u.Hostname()
	}
	if !strings.HasPrefix(host, "/") && host != "127.0.0.1" && host != "::1" {
		t.Fatal("synthetic fixture requires Unix-socket or literal loopback PostgreSQL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("careeros_ai_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	f := &Fixture{DSN: u.String()}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	f.DB, err = sql.Open("pgx", f.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.DB.Close() })
	f.Store, err = accountstore.Open(context.Background(), f.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Store.Close() })
	if err = f.Store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg := auth.Config{PublicOrigin: Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(Origin, "synthetic-client", "synthetic-secret")}}
	registry, err := auth.NewRegistry(fakeOIDC{})
	if err != nil {
		t.Fatal(err)
	}
	f.Service, err = auth.NewService(cfg, registry, f.Store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	start, err := f.Service.BeginLogin(context.Background(), "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	begin, _ := url.Parse(start.URL)
	f.Login, err = f.Service.CompleteLogin(context.Background(), "google", begin.Query().Get("state"), "fixture-account", start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	f.Principal = f.Login.Principal
	f.Document = profile(t)
	if _, err = f.Store.SaveProfile(context.Background(), f.Principal, "synthetic-initial", 0, f.Document); err != nil {
		t.Fatal(err)
	}
	f.ProfileProjection = map[string]any{"careerGoals": "", "skills": "Java", "competencies": "", "tools": "", "employmentStatus": "", "currentSalary": "", "desiredSalary": "", "additionalInfo": "", "experience": []any{}, "projects": []any{}}
	f.Qualifications = map[string]any{"education": []any{}, "certifications": []any{}, "languages": []any{}}
	return f
}

func profile(t *testing.T) json.RawMessage {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "profiledocument", "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Document map[string]any }
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	doc := fixtures[0].Document
	doc["entities"] = []any{}
	fact := doc["facts"].([]any)[0].(map[string]any)
	fact["context"] = []any{}
	fact["value"] = "Java"
	fact["assertion"] = "affirmed"
	fact["intent"] = "actual"
	fact["certainty"] = "certain"
	fact["normalization"] = map[string]any{"observed": "Java", "canonical": nil, "policy": "exact-alias-v1"}
	fact["temporal"] = map[string]any{"wording": "", "precision": "unknown"}
	doc["evidence"].([]any)[0].(map[string]any)["excerpt"] = "I use Java."
	doc["links"] = doc["links"].([]any)[:1]
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = profiledocument.Decode(raw); err != nil {
		t.Fatal("invalid canonical Profile fixture", err)
	}
	return raw
}

// ConfigureEnv selects only the disposable local database and synthetic OIDC
// client labels. Opaque-session validation performs no Google exchange or JWKS call.
func (f *Fixture) ConfigureEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CAREEROS_DATABASE_URL", f.DSN)
	t.Setenv("CAREEROS_PUBLIC_ORIGIN", Origin)
	t.Setenv("CAREEROS_GOOGLE_CLIENT_ID", "synthetic-client")
	t.Setenv("CAREEROS_GOOGLE_CLIENT_SECRET", "synthetic-secret")
	t.Setenv("CAREEROS_GOOGLE_REDIRECT_URI", "")
}

func (f *Fixture) Body(t *testing.T, path string) []byte {
	t.Helper()
	var doc profiledocument.Document
	if err := json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	var body any
	switch path {
	case "/api/profile/review":
		body = map[string]any{"document": doc, "section": "skills", "locale": "en"}
	case "/api/profile/ingest":
		body = map[string]any{"input": "I use Java.", "profile": f.ProfileProjection}
	case "/api/cv/generate":
		body = map[string]any{"locale": "en", "facts": []any{map[string]any{"id": "f0", "section": "skills", "entryId": "", "field": "skills", "text": "Java"}}}
	default:
		input := map[string]any{"repository": f.ProfileProjection, "qualifications": f.Qualifications, "profileEvidence": qualificationmatching.ApprovedProjection(doc), "jobPosting": "Java required.", "confirmedQualifications": []any{}}
		if path == "/api/qualification-gaps" {
			input["understandJob"] = true
			delete(input, "confirmedQualifications")
		}
		body = input
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (f *Fixture) Request(t *testing.T, ctx context.Context, path string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
	r.Header.Set("Origin", Origin)
	r.Header.Set("X-CareerOS-Account", f.Login.Account.ID)
	r.Header.Set("X-CSRF-Token", f.Login.CSRFToken)
	r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: f.Login.SessionToken})
	r.AddCookie(&http.Cookie{Name: "__Host-careeros_csrf", Value: f.Login.CSRFToken})
	r.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	return r
}

// EffectiveRoot mirrors the documented account guard's enforced Draft review
// flags when binding a fixture Action. It does not mutate the HTTP request: a
// caller can still supply false flags so tests prove that guard enforces them.
func EffectiveRoot(t *testing.T, path string, body []byte) []byte {
	t.Helper()
	if path != "/api/application-draft" {
		return body
	}
	var input map[string]json.RawMessage
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatal(err)
	}
	input["reviewResume"] = json.RawMessage("true")
	input["reviewArtifacts"] = json.RawMessage("true")
	root, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func (f *Fixture) Event(t *testing.T, path, route string, body []byte) events.APIGatewayProxyRequest {
	t.Helper()
	r := f.Request(t, context.Background(), route, body)
	headers := map[string]string{}
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}
	return events.APIGatewayProxyRequest{Path: path, HTTPMethod: "POST", Body: string(body), Headers: headers}
}
