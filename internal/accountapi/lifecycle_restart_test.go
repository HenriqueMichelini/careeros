package accountapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/testsupport/aiadmission"
	"strings"
	"testing"
	"time"
)

type lifecycleRestartInput struct {
	DSN, Account, Session, Witness, Evidence string
	Expected                                 []byte
}

func TestLifecyclePhysicalPostgresRestartRecoversSameStatusWithoutCallbacks(t *testing.T) {
	dsn, pgctl, data, log := privateRestartPostgres(t)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
	f, h, provider, purpose, router, input := terminalDraftFixture(t)
	path := filepath.Join(t.TempDir(), "lifecycle-callback-witness")
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		witness(t, path, "provider-stage")
		response, err := provider.Respond(ctx, p, r)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	created := qualificationRequest(t, f, router, "/api/drafts", "lifecycle-restart-delivered", "", body)
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	receipt := openingRequest(t, f, router, "POST", "/api/drafts/lifecycle-restart-delivered/versions/1/opening", struct{}{})
	var issued draftrecovery.OpeningReceipt
	if receipt.Code != 200 || json.Unmarshal(receipt.Body.Bytes(), &issued) != nil {
		t.Fatal(receipt.Code, receipt.Body.String())
	}
	confirm := openingRequest(t, f, router, "POST", "/api/drafts/lifecycle-restart-delivered/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: issued.Receipt, BundleDigest: issued.BundleDigest, Manifest: issued.Manifest})
	if confirm.Code != 200 {
		t.Fatal(confirm.Code, confirm.Body.String())
	}
	if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		witness(t, path, "provider-stage")
		return nil, false, errors.New("synthetic unknown provider")
	}); err != nil {
		t.Fatal(err)
	}
	if w := qualificationRequest(t, f, router, "/api/drafts", "lifecycle-restart-unknown", "", body); w.Code < 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := f.DB.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '45 days',expires_at=clock_timestamp()-interval '31 days' WHERE account_id=$1`, f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	worker, _ := accountstore.NewLifecycleWorker(f.Store, func(context.Context, accountstore.LifecycleEmailNotice) (accountstore.LifecycleEmailResult, error) {
		witness(t, path, "email-attempt")
		return accountstore.LifecycleEmailResult{}, errors.New("synthetic unknown email")
	})
	if _, err := worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	status := openingRequest(t, f, router, "GET", "/api/lifecycle/status", nil)
	if status.Code != 200 {
		t.Fatal(status.Code, status.Body.String())
	}
	file := filepath.Join(t.TempDir(), "handoff.json")
	raw, _ := json.Marshal(lifecycleRestartInput{f.DSN, f.Principal.AccountID, f.Login.SessionToken, path, os.Getenv("CAREEROS_LOCAL_CHECK_EVIDENCE"), status.Body.Bytes()})
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
	command := exec.CommandContext(ctx, "rtk", "proxy", os.Args[0], "-test.run=^TestLifecycleRestartChild$", "-test.timeout=45s", "-test.v")
	command.Env = append(os.Environ(), "CAREEROS_LIFECYCLE_RESTART_HANDOFF="+file)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	observed, err := os.ReadFile(path)
	if err != nil || string(observed) != strings.Repeat("provider-stage\n", 5)+strings.Repeat("email-attempt\n", 2) {
		t.Fatal("restart repeated callback", string(observed), err)
	}
	if dir := os.Getenv("CAREEROS_LOCAL_CHECK_EVIDENCE"); dir != "" {
		if err = os.WriteFile(filepath.Join(dir, "lifecycle-restart-witness.txt"), observed, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "lifecycle-restart-before.json"), status.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("real fsync-enabled PostgreSQL immediate restart plus fresh process: exact canonical/alias lifecycle status, unknown notice intents unchanged, no callback, primary content absent, provider36688/occupancy1 and delivered effect1 retained")
}
func TestLifecycleRestartChild(t *testing.T) {
	file := os.Getenv("CAREEROS_LIFECYCLE_RESTART_HANDOFF")
	if file == "" {
		t.Skip("private lifecycle restart child")
	}
	raw, err := os.ReadFile(file)
	var in lifecycleRestartInput
	if err != nil || json.Unmarshal(raw, &in) != nil {
		t.Fatal("invalid synthetic lifecycle handoff")
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
	router := accountapi.NewPilot(service, store, nil)
	call := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-CareerOS-Account", in.Account)
		r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: in.Session})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/lifecycle/status", "/.netlify/functions/account/api/lifecycle/status"} {
		w := call(path)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), in.Expected) {
			t.Fatal("restart changed exact lifecycle metadata", path, w.Code, w.Body.String())
		}
	}
	worker, _ := accountstore.NewLifecycleWorker(store, func(context.Context, accountstore.LifecycleEmailNotice) (accountstore.LifecycleEmailResult, error) {
		witness(t, in.Witness, "unexpected-email")
		return accountstore.LifecycleEmailResult{}, errors.New("no restart resend")
	})
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	final := call("/api/lifecycle/status")
	if final.Code != 200 || !bytes.Equal(final.Body.Bytes(), in.Expected) {
		t.Fatal("job replay changed original clocks/outcomes", final.Code, final.Body.String())
	}
	if in.Evidence != "" {
		if err = os.WriteFile(filepath.Join(in.Evidence, "lifecycle-restart-after.json"), final.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("pgx", in.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var profiles, content, effects, occupied int
	var counted int64
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM careeros_profiles),(SELECT count(*) FROM careeros_draft_content),(SELECT count(*) FROM ai_fixture_action WHERE customer_effect),(SELECT counted FROM ai_fixture_scope WHERE namespace='managed-draft' AND scope='global'),(SELECT occupied FROM ai_fixture_scope WHERE namespace='managed-draft' AND scope='global')`).Scan(&profiles, &content, &effects, &counted, &occupied); err != nil || profiles != 0 || content != 0 || effects != 1 || counted != 36688 || occupied != 1 {
		t.Fatal("restart lost content/financial truth", profiles, content, effects, counted, occupied, err)
	}
}
