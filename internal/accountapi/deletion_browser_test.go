package accountapi

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Private test-binary bootstrap/control only. No product constructor mounts it.
func TestDeletionBrowserServer(t *testing.T) {
	path := os.Getenv("CAREEROS_DELETION_BROWSER_CONFIG")
	if path == "" {
		t.Skip("private browser helper")
	}
	var cfg struct{ Origin, Address, Token string }
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &cfg) != nil || len(cfg.Token) < 32 || !strings.HasPrefix(cfg.Address, "127.0.0.1:") {
		t.Fatal("invalid private fixture")
	}
	dsn, store, _, operator := pilotFixture(t)
	registry, _ := auth.NewRegistry(fakeOIDC{})
	svc, err := auth.NewService(auth.Config{PublicOrigin: cfg.Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(cfg.Origin, "synthetic", "synthetic-secret")}}, registry, store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	who, invite := pilotInvite(t, store, svc, operator, "deletion-browser")
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
	stop := make(chan struct{})
	var once sync.Once
	var posts atomic.Int32
	current := who
	cookies := func(w http.ResponseWriter) {
		http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_session", Value: current.SessionToken, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_csrf", Value: current.CSRFToken, Path: "/", Secure: true, SameSite: http.SameSiteLaxMode})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pilot/deletion-test/", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(cfg.Token)) != 1 {
			http.Error(w, "private fixture", 403)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/pilot/deletion-test/") {
		case "health":
			w.WriteHeader(204)
		case "bootstrap":
			if _, e := r.Cookie("__Host-careeros_session"); e != nil {
				cookies(w)
			}
			w.WriteHeader(204)
		case "reauth":
			current = login(t, svc, "deletion-browser")
			cookies(w)
			w.WriteHeader(204)
		case "switch":
			var invitation accountstore.PilotStatus
			current, invitation = pilotInvite(t, store, svc, operator, "foreign-deletion-browser")
			_, err = store.ActivatePilot(r.Context(), current.Principal, "activate-foreign", invitation.InvitationID, true)
			cookies(w)
			w.WriteHeader(204)
		case "purge":
			var key string
			if err = db.QueryRow(`SELECT operation_id FROM careeros_deletion_operations WHERE account_id=$1 ORDER BY fenced_at DESC LIMIT 1`, who.Account.ID).Scan(&key); err == nil {
				_, err = store.PurgeDeletion(r.Context(), who.Account.ID, key)
			}
		case "snapshot":
			var primary, ops int
			var accountState string
			err = db.QueryRow(`SELECT count(*) FROM careeros_profiles WHERE account_id=$1`, who.Account.ID).Scan(&primary)
			if err == nil {
				err = db.QueryRow(`SELECT count(*) FROM careeros_deletion_operations WHERE account_id=$1`, who.Account.ID).Scan(&ops)
			}
			if err == nil {
				err = db.QueryRow(`SELECT status FROM careeros_accounts WHERE id=$1`, who.Account.ID).Scan(&accountState)
			}
			json.NewEncoder(w).Encode(map[string]any{"primary": primary, "operations": ops, "posts": posts.Load(), "accountState": accountState})
		case "stop":
			w.WriteHeader(204)
			once.Do(func() { close(stop) })
		default:
			http.NotFound(w, r)
		}
		if err != nil {
			t.Error("private control failed", err)
			w.WriteHeader(500)
		}
	})
	product := NewPilot(svc, store, nil)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/deletion/operations" {
			posts.Add(1)
		}
		product.ServeHTTP(w, r)
	}))
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	select {
	case <-stop:
	case <-time.After(150 * time.Second):
		t.Error("browser helper timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
