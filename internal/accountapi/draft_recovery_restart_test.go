package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type restartHandoff struct {
	DSN, Account, Session, CSRF, Witness string
	Expected                             []byte
	Managed                              bool
	Unknown                              []byte
}

func restartCommand(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "rtk", append([]string{"proxy"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("private PostgreSQL/process command failed: %s %v\n%s", filepath.Base(args[0]), err, out)
	}
}
func privateRestartPostgres(t *testing.T) (string, string, string, string) {
	t.Helper()
	if os.Getenv("CAREEROS_TEST_DATABASE_URL") == "" {
		t.Skip("private PostgreSQL fixture required")
	}
	pgctl := os.Getenv("CAREEROS_DRAFT_TEST_PGCTL")
	if filepath.Base(pgctl) != "pg_ctl" {
		t.Fatal("private runner must identify PostgreSQL binaries")
	}
	root, err := os.MkdirTemp("", "careeros-draft-restart-")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	data, sock, log := filepath.Join(root, "data"), filepath.Join(root, "socket"), filepath.Join(root, "postgres.log")
	if err = os.Mkdir(sock, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	restartCommand(t, filepath.Join(filepath.Dir(pgctl), "initdb"), "-D", data, "--no-locale", "--encoding=UTF8", "--auth-local=trust", "--auth-host=trust")
	restartCommand(t, pgctl, "-D", data, "-l", log, "-o", fmt.Sprintf("-h 127.0.0.1 -k %s -p %d", sock, port), "-w", "start")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "rtk", "proxy", pgctl, "-D", data, "-m", "immediate", "-w", "stop").Run()
		os.RemoveAll(root)
	})
	restartCommand(t, filepath.Join(filepath.Dir(pgctl), "psql"), "-h", "127.0.0.1", "-p", fmt.Sprint(port), "-d", "postgres", "-c", "CREATE DATABASE careeros_test")
	return fmt.Sprintf("postgresql://127.0.0.1:%d/careeros_test?sslmode=disable", port), pgctl, data, log
}
func witness(t *testing.T, path, kind string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.WriteString(kind + "\n"); err != nil {
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
}

func TestControlledDraftPostgresAndServerProcessRestartReadsExactBundle(t *testing.T) {
	dsn, pgctl, data, log := privateRestartPostgres(t)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
	r := newRecoveryFixture(t, nil)
	path := filepath.Join(t.TempDir(), "controlled-inference-witness")
	r.provider = func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		witness(t, path, "generation")
		return completeRecoveryBundle(g), nil
	}
	r.review = func(ctx context.Context, q draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
		witness(t, path, "completion-check")
		return controlledRecoveryReview(ctx, q)
	}
	created := r.request(t, "POST", "/api/drafts", "physical-restart", r.input(t))
	if created.Code != 201 {
		t.Fatalf("creation: %d %s", created.Code, created.Body.String())
	}
	handoff := restartHandoff{DSN: r.f.DSN, Account: r.f.Principal.AccountID, Session: r.f.Login.SessionToken, CSRF: r.f.Login.CSRFToken, Witness: path, Expected: created.Body.Bytes()}
	raw, _ := json.Marshal(handoff)
	handoffPath := filepath.Join(t.TempDir(), "handoff.json")
	if err := os.WriteFile(handoffPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// This cluster belongs only to this test, not the shared suite fixture.
	// Default fsync/WAL are enabled; an immediate restart exercises crash recovery.
	r.store.Close()
	r.actions.Close()
	r.f.Store.Close()
	r.f.DB.Close()
	restartCommand(t, pgctl, "-D", data, "-l", log, "-m", "immediate", "-w", "restart")
	cmd := exec.Command(os.Args[0], "-test.run=^TestControlledDraftRestartChild$", "-test.v", "-test.timeout=45s")
	cmd.Env = append(os.Environ(), "CAREEROS_DRAFT_RESTART_HANDOFF="+handoffPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh server process: %v\n%s", err, out)
	}
	observed, err := os.ReadFile(path)
	if err != nil || string(observed) != "generation\ncompletion-check\n" || r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatalf("restart reran controlled work: %q %v", observed, err)
	}
	t.Log("real private PostgreSQL immediate restart and fresh server process preserved exact complete Result bytes; durable generation/check witness unchanged")
}

func TestManagedDraftPhysicalRestartKeepsExactReadyAndUnknownIntent(t *testing.T) {
	dsn, pgctl, data, log := privateRestartPostgres(t)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
	f, h, p, recovery, router, input := managedDraftFixture(t)
	path := filepath.Join(t.TempDir(), "managed-draft-inference-witness")
	if err := h.SetQualificationResponder(func(ctx context.Context, provider string, req *http.Request) (*http.Response, bool, error) {
		witness(t, path, aidiagnostics.StageName(ctx))
		response, err := p.Respond(ctx, provider, req)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	created := qualificationRequest(t, f, router, "/api/drafts", "physical-restart", "", body)
	if created.Code != 201 {
		t.Fatalf("managed creation before restart: %d %s", created.Code, created.Body.String())
	}
	h.SetFault("after_intent_commit", func() error { return errors.New("synthetic process loss before send") })
	interrupted := qualificationRequest(t, f, router, "/api/drafts", "unknown-restart", "", body)
	unknown := managedDraftStatus(t, f, router, "unknown-restart")
	if interrupted.Code < 400 || unknown.Code != 202 || strings.Contains(unknown.Body.String(), `"retryAllowed":true`) {
		t.Fatalf("unknown intent setup: %d %d %s", interrupted.Code, unknown.Code, unknown.Body.String())
	}
	handoff := restartHandoff{
		DSN: f.DSN, Account: f.Principal.AccountID, Session: f.Login.SessionToken, CSRF: f.Login.CSRFToken,
		Witness: path, Expected: created.Body.Bytes(), Managed: true, Unknown: unknown.Body.Bytes(),
	}
	raw, _ := json.Marshal(handoff)
	handoffPath := filepath.Join(t.TempDir(), "handoff.json")
	if err := os.WriteFile(handoffPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	recovery.Close()
	h.Close()
	f.Store.Close()
	f.DB.Close()
	restartCommand(t, pgctl, "-D", data, "-l", log, "-m", "immediate", "-w", "restart")
	cmd := exec.Command(os.Args[0], "-test.run=^TestControlledDraftRestartChild$", "-test.v", "-test.timeout=45s")
	cmd.Env = append(os.Environ(), "CAREEROS_DRAFT_RESTART_HANDOFF="+handoffPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh managed server process: %v\n%s", err, out)
	}
	observed, err := os.ReadFile(path)
	if err != nil || string(observed) != "field_validation\ngeneration\nresume_support\nartifact_support\n" {
		t.Fatalf("physical restart reran managed inference: %q %v", observed, err)
	}
}

type restartOIDC struct{}

func (restartOIDC) ID() string { return "google" }
func (restartOIDC) AuthorizationURL(string, string, string, string) string {
	return "https://synthetic.invalid"
}
func (restartOIDC) Exchange(context.Context, string, string, string, string) (auth.Identity, error) {
	return auth.Identity{}, errors.New("restart reader cannot exchange provider identities")
}

func TestControlledDraftRestartChild(t *testing.T) {
	path := os.Getenv("CAREEROS_DRAFT_RESTART_HANDOFF")
	if path == "" {
		t.Skip("helper process executed by the dedicated real-restart parent")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var in restartHandoff
	if json.Unmarshal(raw, &in) != nil {
		t.Fatal("invalid synthetic handoff")
	}
	store, err := accountstore.Open(context.Background(), in.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := auth.Config{PublicOrigin: aiadmission.Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(aiadmission.Origin, "synthetic-client", "synthetic-secret")}}
	registry, err := auth.NewRegistry(restartOIDC{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(cfg, registry, store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	if in.Managed {
		h, recovery := openManagedDraftFixture(t, in.DSN, "managed-draft", managedDraftFullBound)
		if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
			witness(t, in.Witness, "unexpected-managed-send")
			return nil, false, errors.New("unexpected handoff")
		}); err != nil {
			t.Fatal(err)
		}
		router := accountapi.NewSyntheticDraft(service, store, h, recovery)
		checkRestartReads(t, router, in)
		request := httptest.NewRequest(http.MethodGet, "/api/drafts/unknown-restart", nil)
		request.Header.Set("X-CareerOS-Account", in.Account)
		request.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: in.Session})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		if w.Code != 202 || !bytes.Equal(w.Body.Bytes(), in.Unknown) {
			t.Fatalf("fresh process changed unknown status: %d %s", w.Code, w.Body.String())
		}
		return
	}
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("controlled-draft", 1)
	actions, err := aicontrol.NewSynthetic(context.Background(), in.DSN, aicontrol.DefaultSyntheticConfig("controlled-draft", gate))
	if err != nil {
		t.Fatal(err)
	}
	defer actions.Close()
	decisions := draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
		var d draftrecovery.Decision
		var delivered string
		err := a.Transaction.QueryRowContext(ctx, `SELECT allow_generate,allow_publish,allow_read,grant_end,epoch,deleted,revoked,cancelled,delivered_action FROM controlled_draft_test_authority WHERE owner=$1 FOR UPDATE`, a.Principal.AccountID).Scan(&d.Generate, &d.Publish, &d.Read, &d.GrantExpiresAt, &d.Epoch, &d.Deleted, &d.Revoked, &d.Cancelled, &delivered)
		d.Delivered = a.Action.ID != "" && delivered == a.Action.ID
		return d, err
	})
	recovery, err := draftrecovery.Open(context.Background(), in.DSN, draftrecovery.Config{Actions: actions, BindingKey: []byte("draft-test-binding-key-32-bytes-long"), Decisions: decisions, Review: func(ctx context.Context, q draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
		witness(t, in.Witness, "unexpected-completion-check")
		return controlledRecoveryReview(ctx, q)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	router := accountapi.NewControlledDraftRecovery(service, recovery, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		witness(t, in.Witness, "unexpected-generation")
		return completeRecoveryBundle(g), nil
	})
	checkRestartReads(t, router, in)
}

func checkRestartReads(t *testing.T, router http.Handler, in restartHandoff) {
	t.Helper()
	for _, route := range []string{"/api/drafts/physical-restart", "/api/drafts/physical-restart/versions/1", "/.netlify/functions/account/api/drafts/physical-restart/versions/1"} {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		request.Header.Set("X-CareerOS-Account", in.Account)
		request.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: in.Session})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), in.Expected) {
			t.Fatalf("fresh process exact recovery %s: %d %s", route, w.Code, w.Body.String())
		}
	}
	if len(strings.TrimSpace(string(in.Expected))) == 0 {
		t.Fatal("empty expected bundle")
	}
}
