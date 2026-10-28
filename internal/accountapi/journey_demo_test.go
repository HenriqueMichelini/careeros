package accountapi

// Test-only loopback fixture: actual auth/session and PostgreSQL Profile APIs,
// with synthetic OIDC and deterministic workflow responses supplied by the
// browser fixture. It cannot be included in a production Go binary.
import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/auth"
)

type journeyOIDC struct {
	origin  string
	subject *atomic.Value
}

func (p journeyOIDC) ID() string { return "google" }
func (p journeyOIDC) AuthorizationURL(state, nonce, challenge, redirect string) string {
	return p.origin + "/api/auth/google/callback?state=" + url.QueryEscape(state) + "&code=" + p.subject.Load().(string)
}
func (p journeyOIDC) Exchange(_ context.Context, code, verifier, redirect, nonce string) (auth.Identity, error) {
	return auth.Identity{Provider: "google", Issuer: "https://accounts.google.com", Subject: code, Email: code + "@synthetic.invalid", Name: code, EmailVerified: true}, nil
}

func TestJourneyDemoServer(t *testing.T) {
	if os.Getenv("CAREEROS_JOURNEY_DEMO") != "1" {
		t.Skip("explicit synthetic browser fixture only")
	}
	dsn, _ := database(t)
	store := open(t, dsn, "journey-browser")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	public := os.Getenv("CAREEROS_JOURNEY_ORIGIN")
	parsed, err := url.Parse(public)
	if err != nil || parsed.Hostname() != "127.0.0.1" {
		t.Fatal("loopback origin required")
	}
	var subject atomic.Value
	subject.Store("alpha")
	cfg := auth.Config{PublicOrigin: public, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(public, "synthetic-client", "synthetic-secret")}}
	registry, _ := auth.NewRegistry(journeyOIDC{public, &subject})
	svc, err := auth.NewService(cfg, registry, store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/demo-account", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name != "alpha" && name != "beta" {
			http.Error(w, "synthetic account only", 400)
			return
		}
		subject.Store(name)
		w.WriteHeader(204)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("synthetic only")) })
	account := New(svc, store)
	mux.Handle("/api/auth/", account)
	mux.Handle("/api/profile/current", account)
	mux.Handle("/api/profile/operations/", account)
	for _, path := range []string{"/api/application-draft", "/api/qualification-gaps", "/api/profile/ingest", "/api/profile/review"} {
		synthetic := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"syntheticWorkflow":true}`))
		})
		mux.Handle(path, ProtectWorkflow(svc, store, path, synthetic))
	}
	fmt.Println("JOURNEY_DEMO_READY")
	server := &http.Server{Addr: os.Getenv("CAREEROS_JOURNEY_ADDR"), Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		t.Fatal(err)
	}
}
