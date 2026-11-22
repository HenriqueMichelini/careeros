package accountapi_test

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
)

// This helper exists only in the test binary. Its local control routes are not
// part of the product constructor, a Function or an operator/bootstrap interface.
func TestControlledDraftBrowserServer(t *testing.T) {
	path := os.Getenv("CAREEROS_DRAFT_BROWSER_CONFIG")
	if path == "" {
		t.Skip("browser helper executed by the real-router browser fixture")
	}
	var cfg struct {
		Origin, Address, Token, Language string
		Managed                          bool
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &cfg) != nil || len(cfg.Token) < 32 {
		t.Fatal("invalid private browser fixture configuration")
	}
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || !strings.HasPrefix(cfg.Address, "127.0.0.1:") {
		t.Fatal("browser fixture requires literal loopback")
	}
	var r *recoveryFixture
	var omitNextCatalog atomic.Bool
	var failNextAnalysis atomic.Bool
	var failNextControlled atomic.Bool
	if cfg.Managed {
		f, actions, provider, store, _, _ := managedDraftFixture(t)
		r = &recoveryFixture{f: f, actions: actions, store: store}
		provider.DraftLanguage = cfg.Language
		if err = actions.SetQualificationResponder(func(ctx context.Context, name string, request *http.Request) (*http.Response, bool, error) {
			workflow, _ := aidispatch.WorkflowRoot(ctx)
			switch workflow {
			case "application_draft":
				r.sends.Add(1)
				if aidiagnostics.StageName(ctx) == "artifact_support" && omitNextCatalog.Swap(false) {
					provider.OmitDraftCatalog = true
					defer func() { provider.OmitDraftCatalog = false }()
				}
			case "qualification_gaps":
				r.checks.Add(1)
				if failNextAnalysis.Swap(false) {
					return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"controlled rate limit"}`))}, true, nil
				}
			default:
				t.Error("unexpected browser workflow")
			}
			response, err := provider.Respond(ctx, name, request)
			return response, true, err
		}); err != nil {
			t.Fatal(err)
		}
	} else {
		r = newRecoveryFixture(t, nil)
	}
	var doc profiledocument.Document
	if json.Unmarshal(r.f.Document, &doc) != nil {
		t.Fatal("invalid fixture Profile")
	}
	doc.Revision++
	for i := range doc.Facts {
		if doc.Facts[i].Owner.ID == doc.ID {
			doc.Facts[i].Owner.Revision = doc.Revision
		}
	}
	name := doc.Facts[0]
	name.ID = "identity-name"
	name.Field = "fullName"
	name.Value = json.RawMessage(`"Original Person"`)
	name.Order = 2
	name.Support = "unsupported"
	name.Origin = profiledocument.Origin{Kind: "manual_edit", Original: "user"}
	name.Normalization.Observed = "Original Person"
	doc.Facts = append(doc.Facts, name)
	raw, _ = json.Marshal(doc)
	if _, err = r.f.Store.SaveProfile(context.Background(), r.f.Principal, "browser-identity", 1, raw); err != nil {
		t.Fatal(err)
	}
	r.f.Document = raw
	authCfg := auth.Config{PublicOrigin: cfg.Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(cfg.Origin, "synthetic-client", "synthetic-secret")}}
	registry, err := auth.NewRegistry(restartOIDC{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(authCfg, registry, r.f.Store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Managed {
		r.router = accountapi.NewSyntheticDraft(service, r.f.Store, r.actions, r.store)
	} else {
		// Reuse the actual controlled callbacks, with independent counters.
		r.router = accountapi.NewControlledDraftRecovery(service, r.store, func(ctx context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
			r.sends.Add(1)
			if failNextControlled.Swap(false) {
				return draftrecovery.Bundle{}, errors.New("controlled incomplete replacement")
			}
			return completeRecoveryBundle(g), nil
		})
	}
	mux := http.NewServeMux()
	var mu sync.Mutex
	current := r.f.Login
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	guard := func(fn http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, request *http.Request) {
			if subtle.ConstantTimeCompare([]byte(request.URL.Query().Get("token")), []byte(cfg.Token)) != 1 {
				http.Error(w, "private fixture", 403)
				return
			}
			fn(w, request)
		}
	}
	cookies := func(w http.ResponseWriter, login auth.LoginResult) {
		http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_session", Value: login.SessionToken, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: "__Host-careeros_csrf", Value: login.CSRFToken, Path: "/", Secure: true, SameSite: http.SameSiteLaxMode})
	}
	mux.HandleFunc("/api/drafts/test/health", guard(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, map[string]bool{"ready": true}) }))
	mux.HandleFunc("/api/drafts/test/bootstrap", guard(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		login := current
		mu.Unlock()
		cookies(w, login)
		writeJSON(w, map[string]string{"account": login.Account.ID})
	}))
	if cfg.Managed {
		mux.HandleFunc("/api/drafts/test/incomplete-next", guard(func(w http.ResponseWriter, _ *http.Request) {
			omitNextCatalog.Store(true)
			w.WriteHeader(204)
		}))
		mux.HandleFunc("/api/drafts/test/fail-next-analysis", guard(func(w http.ResponseWriter, _ *http.Request) {
			failNextAnalysis.Store(true)
			w.WriteHeader(204)
		}))
		for _, suspended := range []bool{true, false} {
			route := "restore-test-grant"
			if suspended {
				route = "suspend-test-grant"
			}
			mux.HandleFunc("/api/drafts/test/"+route, guard(func(w http.ResponseWriter, request *http.Request) {
				mu.Lock()
				principal := current.Principal
				mu.Unlock()
				if suspended {
					_, err := r.f.Store.PilotCommand(request.Context(), principal, "browser-suspend", accountstore.PilotCommand{
						Kind: "suspend", Identity: auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "fixture-account"},
					})
					if err != nil {
						http.Error(w, "fixture suspension", 500)
						return
					}
				} else {
					// Restoring this synthetic fixture exists only in the test binary.
					tx, err := r.f.DB.BeginTx(request.Context(), nil)
					if err == nil {
						defer tx.Rollback()
						err = accountstore.AuthorizeTransaction(request.Context(), tx, principal)
					}
					if err == nil {
						_, err = tx.ExecContext(request.Context(), `UPDATE careeros_pilot_access SET state='active' WHERE account_id=$1 AND state='suspended'`, principal.AccountID)
					}
					if err == nil {
						err = tx.Commit()
					}
					if err != nil {
						http.Error(w, "fixture restoration", 500)
						return
					}
				}
				w.WriteHeader(204)
			}))
		}
	}
	mux.HandleFunc("/api/drafts/test/stats", guard(func(w http.ResponseWriter, _ *http.Request) {
		var effects int
		if err := r.f.DB.QueryRow(`SELECT count(*) FROM ai_fixture_action WHERE customer_effect OR customer_reserved`).Scan(&effects); err != nil {
			http.Error(w, "fixture query", 500)
			return
		}
		if cfg.Managed {
			writeJSON(w, map[string]any{"draft": r.sends.Load(), "qualification": r.checks.Load(), "allowanceEffects": effects})
		} else {
			writeJSON(w, map[string]any{"generation": r.sends.Load(), "completion": r.checks.Load(), "allowanceEffects": effects})
		}
	}))
	mux.HandleFunc("/api/drafts/test/review-failed-replacement", guard(func(w http.ResponseWriter, _ *http.Request) {
		if cfg.Managed {
			http.Error(w, "controlled only", 400)
			return
		}
		failNextControlled.Store(true)
		w.WriteHeader(204)
	}))
	// Local-only fault injection selects an exact purpose Action, so the actual
	// router can return terminal authority for a cached prior bundle while the
	// newer incomplete Action remains readable. No hosted deletion is executed.
	mux.HandleFunc("/api/drafts/test/review-close-key", guard(func(w http.ResponseWriter, request *http.Request) {
		key := request.URL.Query().Get("key")
		if cfg.Managed || key == "" {
			http.Error(w, "controlled exact key required", 400)
			return
		}
		tx, err := r.f.DB.BeginTx(request.Context(), nil)
		if err == nil {
			defer tx.Rollback()
			_, err = tx.ExecContext(request.Context(), `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, r.f.Principal.AccountID)
			if err == nil {
				_, err = tx.ExecContext(request.Context(), `UPDATE careeros_draft_actions SET state='closed' WHERE owner=$1 AND creation_key=$2`, r.f.Principal.AccountID, key)
			}
			if err == nil {
				err = tx.Commit()
			}
		}
		if err != nil {
			http.Error(w, "controlled closure", 500)
			return
		}
		w.WriteHeader(204)
	}))
	// Inject the already-selected opening decision for review tests only. This
	// neither implements the #285 protocol nor changes customer allowance.
	mux.HandleFunc("/api/drafts/test/review-delivered", guard(func(w http.ResponseWriter, request *http.Request) {
		if cfg.Managed {
			http.Error(w, "controlled decision only", 400)
			return
		}
		tx, err := r.f.DB.BeginTx(request.Context(), nil)
		if err == nil {
			defer tx.Rollback()
			_, err = tx.ExecContext(request.Context(), `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, r.f.Principal.AccountID)
			if err == nil {
				_, err = tx.ExecContext(request.Context(), `UPDATE controlled_draft_test_authority SET delivered_action=(SELECT action_id FROM careeros_draft_actions WHERE owner=$1 AND state='ready' AND ($2='' OR creation_key=$2) ORDER BY admitted_at DESC LIMIT 1) WHERE owner=$1`, r.f.Principal.AccountID, request.URL.Query().Get("key"))
			}
			if err == nil {
				err = tx.Commit()
			}
		}
		if err != nil {
			http.Error(w, "controlled delivery decision", 500)
			return
		}
		w.WriteHeader(204)
	}))
	mux.HandleFunc("/api/drafts/test/expire", guard(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		login := current
		mu.Unlock()
		_, err := r.f.DB.ExecContext(request.Context(), `UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, login.Principal.SessionID)
		if err != nil {
			http.Error(w, "fixture expiry", 500)
			return
		}
		w.WriteHeader(204)
	}))
	for _, allow := range []bool{false, true} {
		route := "deny-read"
		if allow {
			route = "allow-read"
		}
		mux.HandleFunc("/api/drafts/test/"+route, guard(func(w http.ResponseWriter, request *http.Request) {
			mu.Lock()
			login := current
			mu.Unlock()
			tx, err := r.f.DB.BeginTx(request.Context(), nil)
			if err == nil {
				defer tx.Rollback()
				if _, err = tx.ExecContext(request.Context(), `SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, login.Account.ID); err == nil {
					_, err = tx.ExecContext(request.Context(), `UPDATE controlled_draft_test_authority SET allow_read=$2 WHERE owner=$1`, login.Account.ID, allow)
				}
				if err == nil {
					err = tx.Commit()
				}
			}
			if err != nil {
				http.Error(w, "fixture read decision", 500)
				return
			}
			w.WriteHeader(204)
		}))
	}
	for _, kind := range []string{"reauth", "switch"} {
		kind := kind
		mux.HandleFunc("/api/drafts/test/"+kind, guard(func(w http.ResponseWriter, request *http.Request) {
			start, e := r.f.Service.BeginLogin(request.Context(), "google", "/", nil)
			if e != nil {
				http.Error(w, "fixture sign-in", 500)
				return
			}
			parsed, _ := url.Parse(start.URL)
			subject := "fixture-account"
			if kind == "switch" {
				subject = "foreign-browser-account"
			}
			login, e := r.f.Service.CompleteLogin(request.Context(), "google", parsed.Query().Get("state"), subject, start.BindingToken)
			if e != nil {
				http.Error(w, "fixture session", 500)
				return
			}
			mu.Lock()
			current = login
			mu.Unlock()
			cookies(w, login)
			writeJSON(w, map[string]string{"account": login.Account.ID})
		}))
	}
	mux.HandleFunc("/api/drafts/test/profile-change", guard(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var changed profiledocument.Document
		if json.Unmarshal(r.f.Document, &changed) != nil {
			http.Error(w, "fixture source", 500)
			return
		}
		expected := changed.Revision
		changed.Revision++
		for i := range changed.Facts {
			if changed.Facts[i].Owner.ID == changed.ID {
				changed.Facts[i].Owner.Revision = changed.Revision
			}
		}
		encoded, _ := json.Marshal(changed)
		if _, err := r.f.Store.SaveProfile(request.Context(), current.Principal, fmt.Sprintf("browser-profile-%d", changed.Revision), expected, encoded); err != nil {
			http.Error(w, "fixture source change", 500)
			return
		}
		r.f.Document = encoded
		w.WriteHeader(204)
	}))
	stop := make(chan struct{})
	var once sync.Once
	mux.HandleFunc("/api/drafts/test/stop", guard(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204); once.Do(func() { close(stop) }) }))
	mux.Handle("/api/drafts", r.router)
	mux.Handle("/api/drafts/", r.router)
	if cfg.Managed {
		mux.Handle("/", r.router)
	} else {
		mux.Handle("/", accountapi.New(service, r.f.Store))
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Log("private controlled Draft browser server ready")
	select {
	case <-stop:
	case <-time.After(140 * time.Second):
		t.Error("browser fixture timed out")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
