package accountapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type deletionRestartInput struct {
	DSN, Account, Session, CSRF, Operation, Kind, Witness string
	Expected                                              []byte
	Counted                                               int64
}

func TestDeletionPhysicalPostgresRestartAndFreshStatusProcess(t *testing.T) {
	for _, kind := range []string{"profile", "account"} {
		t.Run(kind, func(t *testing.T) {
			dsn, pgctl, data, log := privateRestartPostgres(t)
			t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
			f, h, provider, purpose, router, input := terminalDraftFixture(t)
			path := filepath.Join(t.TempDir(), "no-inference-witness")
			if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
				witness(t, path, "managed-stage")
				response, err := provider.Respond(ctx, p, r)
				return response, true, err
			}); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(input)
			if w := qualificationRequest(t, f, router, "/api/drafts", "deleted-restart", "", body); w.Code != 201 {
				t.Fatal(w.Code, w.Body.String())
			}
			request := accountstore.DeletionRequest{OperationID: "deletion-restart", Kind: kind, Confirm: true}
			if kind == "profile" {
				request.ProfileID = input.Profile.ID
				request.Revision = input.Profile.Revision
			}
			deletion := accountapi.NewPilot(f.Service, f.Store, nil)
			reply := openingRequest(t, f, deletion, "POST", "/api/deletion/operations", request)
			if reply.Code != 202 {
				t.Fatal(reply.Code, reply.Body.String())
			}
			// Discard the POST acknowledgement. After closure, only a NEW verified
			// provider sign-in can obtain metadata-only status authority.
			if kind == "account" {
				start, err := f.Service.BeginLogin(context.Background(), "google", "/", nil)
				if err != nil {
					t.Fatal(err)
				}
				u, _ := url.Parse(start.URL)
				fresh, err := f.Service.CompleteLogin(context.Background(), "google", u.Query().Get("state"), "fixture-account", start.BindingToken)
				if err != nil {
					t.Fatal(err)
				}
				f.Login = fresh
				f.Principal = fresh.Principal
			}
			status := openingRequest(t, f, deletion, "GET", "/api/deletion/operations/deletion-restart", nil)
			if status.Code != 200 {
				t.Fatal(status.Code, status.Body.String())
			}
			var counted int64
			if err := f.DB.QueryRow(`SELECT sum(counted) FROM ai_fixture_dispatch`).Scan(&counted); err != nil {
				t.Fatal(err)
			}
			handoff := deletionRestartInput{DSN: f.DSN, Account: f.Principal.AccountID, Session: f.Login.SessionToken, CSRF: f.Login.CSRFToken, Operation: "deletion-restart", Kind: kind, Witness: path, Expected: status.Body.Bytes(), Counted: counted}
			raw, _ := json.Marshal(handoff)
			file := filepath.Join(t.TempDir(), "handoff.json")
			if err := os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			purpose.Close()
			h.Close()
			f.Store.Close()
			f.DB.Close()
			restartCommand(t, pgctl, "-D", data, "-l", log, "-m", "immediate", "-w", "restart")
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDeletionRestartChild$", "-test.v", "-test.timeout=45s")
			cmd.Env = append(os.Environ(), "CAREEROS_DELETION_RESTART_HANDOFF="+file)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fresh deletion status process: %v\n%s", err, out)
			}
			observed, err := os.ReadFile(path)
			if err != nil || string(observed) != strings.Repeat("managed-stage\n", 4) {
				t.Fatal("restart inferred again", string(observed), err)
			}
			t.Log("real fsync PostgreSQL immediate crash/restart + fresh process: exact status bytes, immutable original fence, zero inference, cleanup once, provider obligation preserved")
		})
	}
}

func TestDeletionRestartChild(t *testing.T) {
	file := os.Getenv("CAREEROS_DELETION_RESTART_HANDOFF")
	if file == "" {
		t.Skip("private deletion restart child")
	}
	raw, err := os.ReadFile(file)
	var in deletionRestartInput
	if err != nil || json.Unmarshal(raw, &in) != nil {
		t.Fatal("invalid synthetic handoff")
	}
	store, err := accountstore.Open(context.Background(), in.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := auth.Config{PublicOrigin: aiadmission.Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(aiadmission.Origin, "synthetic-client", "synthetic-secret")}}
	registry, _ := auth.NewRegistry(restartOIDC{})
	service, err := auth.NewService(cfg, registry, store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	h, _ := openManagedDraftFixture(t, in.DSN, "managed-draft", managedDraftFullBound)
	if err = h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		witness(t, in.Witness, "unexpected-send")
		return nil, false, errors.New("deletion recovery must not infer")
	}); err != nil {
		t.Fatal(err)
	}
	purpose, err := draftrecovery.OpenManagedTerminal(context.Background(), in.DSN, draftrecovery.Config{Actions: h, BindingKey: []byte("synthetic-managed-draft-binding-key")})
	if err != nil {
		t.Fatal(err)
	}
	defer purpose.Close()
	router := accountapi.NewSyntheticTerminalDraft(service, store, h, purpose)
	call := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-CareerOS-Account", in.Account)
		r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: in.Session})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/deletion/operations/" + in.Operation, "/.netlify/functions/account/api/deletion/operations/" + in.Operation} {
		w := call(path)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), in.Expected) {
			t.Fatal("restart changed exact deletion status", path, w.Code, w.Body.String())
		}
	}
	w := call("/api/drafts/deleted-restart/versions/1")
	if w.Code != 410 && w.Code != 401 {
		t.Fatal("restart revived career history", w.Code, w.Body.String())
	}
	if _, err = store.PurgeDeletion(context.Background(), in.Account, in.Operation); err != nil {
		t.Fatal(err)
	}
	first := call("/api/deletion/operations/" + in.Operation)
	if first.Code != 200 {
		t.Fatal(first.Code)
	}
	if _, err = store.PurgeDeletion(context.Background(), in.Account, in.Operation); err != nil {
		t.Fatal(err)
	}
	again := call("/api/deletion/operations/" + in.Operation)
	if !bytes.Equal(first.Body.Bytes(), again.Body.Bytes()) {
		t.Fatal("cleanup replay reset anchors")
	}
	db, err := sql.Open("pgx", in.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var profiles, content int
	var counted int64
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM careeros_profiles),(SELECT count(*) FROM careeros_draft_content),(SELECT sum(counted) FROM ai_fixture_dispatch)`).Scan(&profiles, &content, &counted); err != nil || profiles != 0 || content != 0 || counted != in.Counted {
		t.Fatal("cleanup lost truth/provider survivor", profiles, content, counted, err)
	}
}
