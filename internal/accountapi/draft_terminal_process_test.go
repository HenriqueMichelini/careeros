package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type terminalProcessInput struct {
	DSN, Account, Session, CSRF, Witness, Output, Operation, Ready, Start string
	Action                                                                draftrecovery.Action
	Receipt                                                               draftrecovery.OpeningReceipt
	Expected                                                              []byte
}
type terminalProcessOutput struct {
	Code int
	Body []byte
}

func terminalProcess(t *testing.T, in terminalProcessInput) *exec.Cmd {
	t.Helper()
	raw, _ := json.Marshal(in)
	path := filepath.Join(t.TempDir(), "handoff.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestManagedTerminalProcessChild$", "-test.v", "-test.timeout=40s")
	cmd.Env = append(os.Environ(), "CAREEROS_TERMINAL_PROCESS_HANDOFF="+path)
	return cmd
}

func TestManagedTerminalTwoServerProcessesRaceThenPhysicalRestartPreservesWinner(t *testing.T) {
	dsn, pgctl, data, log := privateRestartPostgres(t)
	t.Setenv("CAREEROS_TEST_DATABASE_URL", dsn)
	f, h, provider, store, router, input := terminalDraftFixture(t)
	path := filepath.Join(t.TempDir(), "managed-stage-witness")
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		witness(t, path, "managed-stage")
		response, err := provider.Respond(ctx, p, r)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	const key = "terminal-process-race"
	created := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	var ready draftrecovery.Result
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &ready) != nil {
		t.Fatalf("creation %d %s", created.Code, created.Body.String())
	}
	issued := openingRequest(t, f, router, "POST", "/api/drafts/"+key+"/versions/1/opening", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
		t.Fatal(issued.Body.String())
	}
	base := terminalProcessInput{DSN: f.DSN, Account: f.Principal.AccountID, Session: f.Login.SessionToken, CSRF: f.Login.CSRFToken, Witness: path, Action: ready.Action, Receipt: receipt}
	var cmds []*exec.Cmd
	var logs []*bytes.Buffer
	var outputs []string
	var readyPaths []string
	startPath := filepath.Join(t.TempDir(), "start")
	for _, op := range []string{"cancel", "confirm"} {
		in := base
		in.Operation = op
		in.Start = startPath
		in.Ready = filepath.Join(t.TempDir(), "ready")
		readyPaths = append(readyPaths, in.Ready)
		in.Output = filepath.Join(t.TempDir(), op+".json")
		outputs = append(outputs, in.Output)
		cmd := terminalProcess(t, in)
		out := new(bytes.Buffer)
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
		logs = append(logs, out)
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		})
	}
	for _, path := range readyPaths {
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(path); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("child initialization failed: %s / %s", logs[0].String(), logs[1].String())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	block, err := f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback()
	var owner string
	if err = block.QueryRow(`SELECT account_id FROM careeros_pilot_access WHERE account_id=$1 FOR UPDATE`, f.Principal.AccountID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(startPath, []byte("start"), 0600); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(10 * time.Second)
	for {
		var n int
		if err = f.DB.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND state='active' AND wait_event_type='Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n >= 2 {
			break
		}
		if time.Now().After(until) {
			var active string
			_ = f.DB.QueryRow(`SELECT coalesce(string_agg(state||':'||coalesce(wait_event_type,'')||':'||left(query,150),'; '),'') FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid()`).Scan(&active)
			t.Fatalf("independent server processes did not reach both waits: %s logs %s / %s", active, logs[0].String(), logs[1].String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err = block.Commit(); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range cmds {
		if err = cmd.Wait(); err != nil {
			t.Fatalf("process %d %v %s", i, err, logs[i].String())
		}
	}
	for _, out := range outputs {
		raw, err := os.ReadFile(out)
		var r terminalProcessOutput
		if err != nil || json.Unmarshal(raw, &r) != nil || r.Code < 200 || r.Code >= 500 {
			t.Fatalf("race output %s %s %v", out, raw, err)
		}
	}
	status := openingRequest(t, f, router, "GET", "/api/drafts/"+key+"/versions/1", nil)
	var outcome draftrecovery.Result
	if (status.Code != 200 && status.Code != 410) || json.Unmarshal(status.Body.Bytes(), &outcome) != nil {
		t.Fatalf("winner %d %s", status.Code, status.Body.String())
	}
	var reserved, effects int
	if err = f.DB.QueryRow(`SELECT count(*) FILTER(WHERE customer_reserved),count(*) FILTER(WHERE customer_effect) FROM ai_fixture_action WHERE owner=$1`, f.Principal.AccountID).Scan(&reserved, &effects); err != nil || reserved != 0 || effects < 0 || effects > 1 || outcome.Delivered != (effects == 1) {
		t.Fatalf("one winner reserved%d effects%d state%+v %v", reserved, effects, outcome, err)
	}
	base.Operation, base.Expected = "recover", status.Body.Bytes()
	base.Output = filepath.Join(t.TempDir(), "restart.json")
	store.Close()
	h.Close()
	f.Store.Close()
	f.DB.Close()
	restartCommand(t, pgctl, "-D", data, "-l", log, "-m", "immediate", "-w", "restart")
	cmd := terminalProcess(t, base)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh process after real PostgreSQL immediate restart %v %s", err, out)
	}
	observed, err := os.ReadFile(path)
	if err != nil || string(observed) != strings.Repeat("managed-stage\n", 4) {
		t.Fatalf("race/restart resent inference %q %v", observed, err)
	}
	t.Logf("two independent server processes raced one opening/cancel; physical fsync PostgreSQL restart + fresh process preserved exact winner and four-stage durable no-resend witness; delivered=%v", outcome.Delivered)
}

func TestManagedTerminalProcessChild(t *testing.T) {
	path := os.Getenv("CAREEROS_TERMINAL_PROCESS_HANDOFF")
	if path == "" {
		t.Skip("helper launched only by managed terminal process parent")
	}
	raw, err := os.ReadFile(path)
	var in terminalProcessInput
	if err != nil || json.Unmarshal(raw, &in) != nil {
		t.Fatal("invalid synthetic process handoff")
	}
	profiles, err := accountstore.Open(context.Background(), in.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer profiles.Close()
	cfg := auth.Config{PublicOrigin: aiadmission.Origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(aiadmission.Origin, "synthetic-client", "synthetic-secret")}}
	registry, err := auth.NewRegistry(restartOIDC{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(cfg, registry, profiles.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	h, _ := openManagedDraftFixture(t, in.DSN, "managed-draft", managedDraftFullBound)
	if err = h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		witness(t, in.Witness, "unexpected-send")
		return nil, false, errors.New("terminal recovery must not infer")
	}); err != nil {
		t.Fatal(err)
	}
	store, err := draftrecovery.OpenManagedTerminal(context.Background(), in.DSN, draftrecovery.Config{Actions: h, BindingKey: []byte("synthetic-managed-draft-binding-key")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := accountapi.NewSyntheticTerminalDraft(service, profiles, h, store)
	call := func(method, path string, v any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(v)
		request := httptest.NewRequest(method, path, bytes.NewReader(raw))
		request.Header.Set("Origin", aiadmission.Origin)
		request.Header.Set("X-CareerOS-Account", in.Account)
		request.Header.Set("X-CSRF-Token", in.CSRF)
		request.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: in.Session})
		request.AddCookie(&http.Cookie{Name: "__Host-careeros_csrf", Value: in.CSRF})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, request)
		return w
	}
	if in.Ready != "" {
		if err = os.WriteFile(in.Ready, []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			if _, err := os.Stat(in.Start); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("parent did not release race barrier")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	key := "/api/drafts/terminal-process-race"
	var w *httptest.ResponseRecorder
	switch in.Operation {
	case "cancel":
		w = call("POST", key+"/cancel", map[string]any{"action": in.Action})
	case "confirm":
		w = call("POST", key+"/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: in.Receipt.Receipt, BundleDigest: in.Receipt.BundleDigest, Manifest: in.Receipt.Manifest})
	case "recover":
		for _, route := range []string{key, key + "/versions/1", "/.netlify/functions/account" + key + "/versions/1"} {
			w = call("GET", route, nil)
			if (w.Code != 200 && w.Code != 410) || !bytes.Equal(w.Body.Bytes(), in.Expected) {
				t.Fatalf("restart exact bytes %d %s", w.Code, w.Body.String())
			}
		}
		w = call("POST", key+"/cancel", map[string]any{"action": in.Action})
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), in.Expected) {
			t.Fatalf("idempotent cancellation after restart %d %s", w.Code, w.Body.String())
		}
	default:
		t.Fatal("unknown bounded operation")
	}
	out, _ := json.Marshal(terminalProcessOutput{Code: w.Code, Body: w.Body.Bytes()})
	if err = os.WriteFile(in.Output, out, 0600); err != nil {
		t.Fatal(err)
	}
}
