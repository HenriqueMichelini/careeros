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
	"professional-information-repo/internal/testsupport/aiadmission"
	"testing"
	"time"
)

type controlRestartInput struct {
	DSN, Operator, Restricted, Owner, Witness, Evidence string
	Expected                                            []byte
	Receipt                                             accountstore.ControlStatus
	Command                                             accountstore.ControlCommand
}

func TestOperatorControlPhysicalRestartPreservesStopsAndUnknownHold(t *testing.T) {
	dsn, pgctl, data, log := privateRestartPostgres(t)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
	f, h, _, purpose, router, input := terminalDraftFixture(t)
	witnessPath := filepath.Join(t.TempDir(), "callback-witness")
	if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		witness(t, witnessPath, "provider-stage")
		return nil, false, errors.New("synthetic unknown")
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	response := qualificationRequest(t, f, router, "/api/drafts", "unknown-before-control-restart", "", body)
	if response.Code < 400 {
		t.Fatal(response.Code, response.Body.String())
	}
	command := accountstore.ControlCommand{Control: "compromise", Stop: true, ExpectedVersion: 1, AccountID: f.Principal.AccountID}
	receipt, err := f.Store.LocalControls(nil).Apply(context.Background(), f.ControlOperator.Principal, "restart-compromise", command)
	if err != nil {
		t.Fatal(err)
	}
	restricted := controlFreshIdentity(t, f.Service, "fixture-account")
	copy := *f
	copy.Login = restricted
	copy.Principal = restricted.Principal
	rights := openingRequest(t, &copy, router, "GET", "/api/rights/status", nil)
	if rights.Code != 200 {
		t.Fatal(rights.Code, rights.Body.String())
	}
	file := filepath.Join(t.TempDir(), "handoff.json")
	raw, _ := json.Marshal(controlRestartInput{f.DSN, f.ControlOperator.SessionToken, restricted.SessionToken, f.Principal.AccountID, witnessPath, os.Getenv("CAREEROS_LOCAL_CHECK_EVIDENCE"), rights.Body.Bytes(), receipt, command})
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	purpose.Close()
	h.Close()
	f.Store.Close()
	f.DB.Close()
	restartCommand(t, pgctl, "-D", data, "-l", log, "-m", "immediate", "-w", "restart")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, "rtk", "proxy", os.Args[0], "-test.run=^TestOperatorControlRestartChild$", "-test.timeout=45s", "-test.v")
	child.Env = append(os.Environ(), "CAREEROS_CONTROL_RESTART_HANDOFF="+file)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	observed, err := os.ReadFile(witnessPath)
	if err != nil || string(observed) != "provider-stage\n" {
		t.Fatal("restart repeated callback", string(observed), err)
	}
	if dir := os.Getenv("CAREEROS_LOCAL_CHECK_EVIDENCE"); dir != "" {
		if err = os.WriteFile(filepath.Join(dir, "control-restart-witness.txt"), observed, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("real fsync PostgreSQL immediate restart and fresh child process preserved exact restricted metadata and command receipt, denied unproved reopening, retained1344/unknown1 with one durable fake-call witness")
}
func TestOperatorControlRestartChild(t *testing.T) {
	file := os.Getenv("CAREEROS_CONTROL_RESTART_HANDOFF")
	if file == "" {
		t.Skip("private control restart child")
	}
	raw, err := os.ReadFile(file)
	var in controlRestartInput
	if err != nil || json.Unmarshal(raw, &in) != nil {
		t.Fatal("invalid private control handoff")
	}
	store, err := accountstore.Open(context.Background(), in.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	registry, _ := auth.NewRegistry(restartOIDC{})
	service, err := auth.NewService(auth.Config{PublicOrigin: aiadmission.Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(aiadmission.Origin, "synthetic-client", "synthetic-secret")}}, registry, store.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	p, err := service.Authenticate(context.Background(), in.Operator)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.LocalControls(nil).Apply(context.Background(), p, "restart-compromise", in.Command)
	actual, _ := json.Marshal(receipt)
	expected, _ := json.Marshal(in.Receipt)
	if err != nil || !bytes.Equal(actual, expected) {
		t.Fatal("restart changed immutable receipt", err)
	}
	reopen := in.Command
	reopen.Stop = false
	reopen.ExpectedVersion = receipt.Version
	reopen.RecoveryReference = "restored-db-is-not-exclusion"
	if _, err = store.LocalControls(nil).Apply(context.Background(), p, "unsupported-reopen", reopen); err == nil {
		t.Fatal("restored stop bit reopened without source-owned proof")
	}
	router := accountapi.NewPilot(service, store, nil)
	for _, path := range []string{"/api/rights/status", "/.netlify/functions/account/api/rights/status"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-CareerOS-Account", in.Owner)
		r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: in.Restricted})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), in.Expected) {
			t.Fatal("restart changed restricted metadata", path, w.Code, w.Body.String())
		}
		if in.Evidence != "" {
			if err = os.WriteFile(filepath.Join(in.Evidence, "control-restart-after.json"), w.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	db, err := sql.Open("pgx", in.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var total int64
	var unknown, held int
	if err = db.QueryRow(`SELECT (SELECT counted FROM ai_fixture_scope WHERE namespace='managed-draft' AND scope='global'),(SELECT occupied FROM ai_fixture_scope WHERE namespace='managed-draft' AND scope='global'),(SELECT count(*) FROM careeros_draft_allowance WHERE state='reserved')`).Scan(&total, &unknown, &held); err != nil || total != 1344 || unknown != 1 || held != 0 {
		t.Fatal("restart released financial/provider hold or retained customer reservation", total, unknown, held, err)
	}
}
