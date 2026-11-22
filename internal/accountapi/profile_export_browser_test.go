package accountapi

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"professional-information-repo/internal/auth"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test-binary-only controlled bootstrap; no route is installed by a product constructor.
func TestProfileExportBrowserServer(t *testing.T) {
	path := os.Getenv("CAREEROS_EXPORT_BROWSER_CONFIG")
	if path == "" {
		t.Skip("private browser helper")
	}
	var cfg struct{ Origin, Address, Token string }
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &cfg) != nil || len(cfg.Token) < 32 || !strings.HasPrefix(cfg.Address, "127.0.0.1:") {
		t.Fatal("invalid private fixture")
	}
	dsn, store, _, operator := pilotFixture(t)
	registry, err := auth.NewRegistry(fakeOIDC{})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(auth.Config{PublicOrigin: cfg.Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(cfg.Origin, "synthetic", "synthetic-secret")}}, registry, store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	who, invite := pilotInvite(t, store, svc, operator, "export-browser")
	if _, err = store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("testdata/profile-export-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveProfile(context.Background(), who.Principal, "save", 0, raw); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mux := http.NewServeMux()
	stop := make(chan struct{})
	var once sync.Once
	current := who
	mux.HandleFunc("/api/pilot/export-test/", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(cfg.Token)) != 1 {
			http.Error(w, "private fixture", 403)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/pilot/export-test/") {
		case "health":
			w.WriteHeader(204)
		case "bootstrap", "reauth":
			if strings.HasSuffix(r.URL.Path, "reauth") {
				current = login(t, svc, "export-browser")
			}
			http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_session", Value: current.SessionToken, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_csrf", Value: current.CSRFToken, Path: "/", Secure: true, SameSite: http.SameSiteLaxMode})
			w.WriteHeader(204)
		case "switch":
			current = login(t, svc, "foreign-export-browser")
			http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_session", Value: current.SessionToken, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_csrf", Value: current.CSRFToken, Path: "/", Secure: true, SameSite: http.SameSiteLaxMode})
		case "restore":
			_, err = db.Exec("UPDATE careeros_pilot_access SET state='active' WHERE account_id=$1", who.Account.ID)
		case "expire":
			_, err = db.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", current.Principal.SessionID)
		case "revoke":
			_, err = db.Exec("UPDATE careeros_pilot_access SET state='revoked' WHERE account_id=$1", who.Account.ID)
		case "window":
			_, err = db.Exec("UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 day' WHERE account_id=$1", who.Account.ID)
		case "stop":
			w.WriteHeader(204)
			once.Do(func() { close(stop) })
		default:
			http.NotFound(w, r)
		}
		if err != nil {
			t.Error("private control failed", err)
		}
	})
	mux.Handle("/", NewPilot(svc, store, nil))
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	select {
	case <-stop:
	case <-time.After(120 * time.Second):
		t.Error("browser helper timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
