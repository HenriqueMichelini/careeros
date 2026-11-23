package accountapi_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/operationslocal"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func pilotOperationsCLIEnv(t *testing.T, f *aiadmission.Fixture, directory, namespace string) []string {
	t.Helper()
	reader, err := url.Parse(f.DSN)
	if err != nil {
		t.Fatal(err)
	}
	var database, schema string
	if err = f.DB.QueryRow("SELECT current_database(),current_schema()").Scan(&database, &schema); err != nil {
		t.Fatal(err)
	}
	role := sha256.Sum256([]byte(database + "/" + schema))
	reader.User = url.User(fmt.Sprintf("ai_fixture_review_%x", role[:8]))
	key := sha256.Sum256([]byte("synthetic-operations-integration"))
	return []string{
		"CAREEROS_OPERATIONS_DATABASE_URL=" + f.DSN,
		"CAREEROS_OPERATIONS_READER_URL=" + reader.String(),
		"CAREEROS_OPERATIONS_NAMESPACE=" + namespace,
		"CAREEROS_OPERATIONS_SESSION_TOKEN=" + f.Login.SessionToken,
		"CAREEROS_OPERATIONS_JOURNAL_DIRECTORY=" + directory,
		"CAREEROS_OPERATIONS_JOURNAL_KEY=" + hex.EncodeToString(key[:]),
		"CAREEROS_RECONCILIATION_LOCAL_DATABASE_URL=" + reader.String(),
		"CAREEROS_RECONCILIATION_LOCAL_SESSION_TOKEN=" + f.Login.SessionToken,
	}
}

func runPilotOperationsFixtureCLI(t *testing.T, program string, env []string, input []byte, args ...string) ([]byte, []byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rtk", append([]string{"proxy", "go", "run", "../../cmd/" + program, "--synthetic-only"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = bytes.NewReader(input)
	var output, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostic
	err := cmd.Run()
	return output.Bytes(), diagnostic.Bytes(), err
}

func TestPilotOperationsReconciliationCLIObservesItsOwnCommittedInvariant(t *testing.T) {
	for _, mode := range []string{"capture", "capture_unavailable", "reviewer_expires", "finance_only"} {
		t.Run(mode, func(t *testing.T) { pilotOperationsReconciliationCLI(t, mode) })
	}
}

func pilotOperationsReconciliationCLI(t *testing.T, mode string) {
	t.Helper()
	f, h, _, router := qualificationFixture(t)
	j, ops, notices, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		w := httptest.NewRecorder()
		w.Header().Set("X-Request-ID", "fixture-reconciliation")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.WriteString(`{"error":"synthetic_rate_limit"}`)
		return w.Result(), false, nil
	}); err != nil {
		t.Fatal(err)
	}
	response := qualificationRequest(t, f, router, "/api/qualification-gaps", "operations-actual-reconciliation", "", f.Body(t, "/api/qualification-gaps"))
	if response.Code == 200 {
		t.Fatal("fixture should retain the original possible dispatch")
	}
	var dispatch string
	if err := f.DB.QueryRow(`SELECT id FROM ai_fixture_dispatch WHERE namespace='qualification' AND remote_state='possibly_active'`).Scan(&dispatch); err != nil {
		t.Fatal(err)
	}
	if err := aicontrol.InstallLocalReconciliation(ctx, f.DB, "qualification", f.Principal.AccountID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	env := pilotOperationsCLIEnv(t, f, directory, "qualification")
	gross := int64(900)
	evidence := aicontrol.ReconciliationCommand{ID: "fixture-operations-unpriced", Kind: "unpriced", Dispatch: dispatch,
		Evidence: aicontrol.ReconciliationEvidence{Source: "fixture-gross-bill", Revision: 1, Reason: "fixture-unpriced",
			Provider: "typesafe", Model: "jev-1.13.0", Tier: "standard", Rate: "unpriced-fixture-rate", Currency: "EUR",
			BillingNamespace: "synthetic", RequestID: "fixture-reconciliation", Complete: true, Gross: &gross}}
	raw, _ := json.Marshal(evidence)
	out, diagnostic, err := runPilotOperationsFixtureCLI(t, "reconciliation-local", env, raw, "--namespace", "qualification", "--mode", "preview")
	if err != nil {
		t.Fatalf("preview fixture evidence: %v %s", err, diagnostic)
	}
	var preview aicontrol.ReconciliationPreview
	if json.Unmarshal(out, &preview) != nil || preview.Token == "" {
		t.Fatal("preview did not produce a synthetic command binding")
	}
	evidence.Preview = preview.Token
	raw, _ = json.Marshal(evidence)
	if mode == "finance_only" {
		env = append(env, "CAREEROS_OPERATIONS_JOURNAL_DIRECTORY=", "CAREEROS_OPERATIONS_JOURNAL_KEY=")
		_, diagnostic, err = runPilotOperationsFixtureCLI(t, "reconciliation-local", env, raw, "--namespace", "qualification", "--mode", "apply")
		if err != nil || strings.Contains(string(diagnostic), `"Kind":"invariant_breach"`) {
			t.Fatalf("finance-only command crossed its independent boundary: %v %s", err, diagnostic)
		}
		var inheritedStop, operationsSideEffect bool
		if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM ai_recon_stop WHERE stopped AND event_id=$1),
		 EXISTS(SELECT 1 FROM careeros_operations_pending) OR EXISTS(SELECT 1 FROM careeros_operations_deferred)
		 OR EXISTS(SELECT 1 FROM careeros_control_state WHERE operations_fault OR inference_stopped)`, evidence.ID).
			Scan(&inheritedStop, &operationsSideEffect); err != nil || !inheritedStop || operationsSideEffect {
			t.Fatalf("finance-only authority changed #291 state or lost #283: %v %v %v", inheritedStop, operationsSideEffect, err)
		}
		return
	}
	if mode == "reviewer_expires" {
		blocker, err := f.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback()
		if _, err = blocker.Exec(`SELECT pg_advisory_xact_lock(72481324)`); err != nil {
			t.Fatal(err)
		}
		var expiry time.Time
		if err = f.DB.QueryRow(`UPDATE ai_recon_operator SET expires_at=clock_timestamp()+interval '2 seconds'
		 WHERE namespace='qualification' AND actor=$1 RETURNING expires_at`, f.Principal.AccountID).Scan(&expiry); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			_, _, err := runPilotOperationsFixtureCLI(t, "reconciliation-local", env, raw, "--namespace", "qualification", "--mode", "apply")
			done <- err
		}()
		waitForOperationsLease(t, f.DB, 72481324, "ExclusiveLock", false)
		time.Sleep(time.Until(expiry) + 50*time.Millisecond)
		if err = blocker.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err = <-done; err == nil {
			t.Fatal("expired reconciliation reviewer committed the command")
		}
		var committed bool
		if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM ai_recon_command WHERE namespace='qualification' AND id=$1)
		 OR EXISTS(SELECT 1 FROM careeros_operations_deferred WHERE namespace='qualification' AND event_id=$1)`, evidence.ID).Scan(&committed); err != nil || committed {
			t.Fatalf("expired reviewer left a command or deferred receipt: %v %v", committed, err)
		}
		return
	}
	captureUnavailable := mode == "capture_unavailable"
	if captureUnavailable {
		_, err = f.DB.Exec(`CREATE FUNCTION fixture_operations_capture_unavailable() RETURNS trigger LANGUAGE plpgsql AS $$
		 BEGIN RAISE EXCEPTION 'fixture capture unavailable'; END $$;
		 CREATE TRIGGER fixture_operations_capture_unavailable BEFORE INSERT ON careeros_operations_pending
		 FOR EACH ROW EXECUTE FUNCTION fixture_operations_capture_unavailable()`)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, diagnostic, err = runPilotOperationsFixtureCLI(t, "reconciliation-local", env, raw, "--namespace", "qualification", "--mode", "apply")
	if err != nil || !captureUnavailable && !strings.Contains(string(diagnostic), `"Kind":"invariant_breach"`) {
		t.Fatalf("real command did not deliver its own known invariant signal: %v %s", err, diagnostic)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, incident := range status.Incidents {
		if incident.Kind == "invariant_breach" {
			count++
		}
	}
	if captureUnavailable {
		if status.Emergency == nil || !status.ContainmentUnavailable {
			t.Fatalf("owner did not receive the durable capture failure: %+v", status)
		}
		var eventAt time.Time
		var obligation, maximum int64
		if err = f.DB.QueryRow(`SELECT created_at,obligation,reserved FROM ai_recon_event WHERE namespace='qualification' AND id=$1`,
			evidence.ID).Scan(&eventAt, &obligation, &maximum); err != nil || obligation != maximum {
			t.Fatalf("fixture lost the unpriced event at the unchanged maximum: %d %d %v", obligation, maximum, err)
		}
		emergency := status.Emergency.ID
		ops, err = f.Store.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "fixture-recovery-writer",
			VerifyEmergencyReview: func(_ context.Context, reference string, got accountstore.OperationsEmergency) (accountstore.OperationsEmergencyReview, error) {
				if got.ID != emergency || reference != "fixture-reviewed-writer" {
					return accountstore.OperationsEmergencyReview{}, auth.ErrForbidden
				}
				return accountstore.OperationsEmergencyReview{ObservedAt: time.Now().UTC()}, nil
			}}, h.LocalOperationsSource(), j)
		if err != nil {
			t.Fatal(err)
		}
		if err = ops.ReplayEmergency(ctx, f.Principal, "fixture-reviewed-writer"); !errors.Is(err, auth.ErrUnavailable) {
			t.Fatalf("a readable ledger bypassed the unavailable capture writer: %v", err)
		}
		if _, err = f.DB.Exec(`DROP TRIGGER fixture_operations_capture_unavailable ON careeros_operations_pending`); err != nil {
			t.Fatal(err)
		}
		if err = ops.ReplayEmergency(ctx, f.Principal, "fixture-reviewed-writer"); err != nil {
			t.Fatalf("verified recovery could not commit through the restored writer: %v", err)
		}
		status, err = ops.OwnerStatus(ctx, f.Principal)
		if err != nil || status.Emergency != nil {
			t.Fatalf("recovered writer did not clear the reviewed emergency: %+v %v", status, err)
		}
		criticals := 0
		for _, incident := range status.Incidents {
			if incident.Kind == "invariant_breach" {
				criticals++
				if !incident.EventAt.Equal(eventAt) || incident.Exposure != obligation || incident.Cap != maximum {
					t.Fatalf("replay changed the original typed event: %+v", incident)
				}
			}
		}
		if criticals != 1 || len(*notices) == 0 {
			t.Fatalf("expected one replayed critical event: %+v %+v", status.Incidents, *notices)
		}
		var delivered int
		for i := 0; i < 2; i++ {
			if err = ops.Inspect(ctx); err != nil {
				t.Fatal(err)
			}
		}
		for _, n := range *notices {
			if n.Kind == "invariant_breach" {
				delivered++
			}
		}
		var pending int
		if err = f.DB.QueryRow(`SELECT count(*) FROM careeros_operations_pending WHERE critical_event`).Scan(&pending); err != nil || pending != 0 || delivered != 1 {
			t.Fatalf("successful consumption must acknowledge once: pending=%d delivered=%d %v", pending, delivered, err)
		}
	} else if count != 1 {
		t.Fatalf("expected one retained invariant incident: %+v", status.Incidents)
	}
	var stopped bool
	if err = f.DB.QueryRow(`SELECT stopped FROM ai_recon_stop WHERE namespace='qualification'`).Scan(&stopped); err != nil || !stopped {
		t.Fatalf("inherited #283 stop was lost: %v", err)
	}
	var db, schema string
	if err = f.DB.QueryRow("SELECT current_database(),current_schema()").Scan(&db, &schema); err != nil {
		t.Fatal(err)
	}
	role := sha256.Sum256([]byte(db + "/" + schema))
	var direct bool
	if err = f.DB.QueryRow(`SELECT has_column_privilege($1,'careeros_operations_pending','sample','INSERT')`,
		fmt.Sprintf("ai_fixture_rt_%x", role[:8])).Scan(&direct); err != nil || direct {
		t.Fatalf("runtime retains arbitrary serialized outbox write authority: %v", err)
	}
	if err = f.DB.QueryRow(`SELECT has_function_privilege($1,'careeros_operations_record_emergency(text)','EXECUTE')`,
		fmt.Sprintf("ai_fixture_review_%x", role[:8])).Scan(&direct); err != nil || direct {
		t.Fatalf("financial reader can execute the private emergency mutation: %v", err)
	}
	for _, databaseRole := range []string{fmt.Sprintf("ai_fixture_review_%x", role[:8]), fmt.Sprintf("ai_fixture_rt_%x", role[:8])} {
		if err = f.DB.QueryRow(`SELECT has_table_privilege($1,'careeros_operations_pending','DELETE')
		 OR has_table_privilege($1,'careeros_operations_deferred','INSERT,UPDATE,DELETE')
		 OR has_function_privilege($1,'careeros_capture_operations(text,text)','EXECUTE')
		 OR has_function_privilege($1,'careeros_replay_operations_captures(text)','EXECUTE')`, databaseRole).Scan(&direct); err != nil || direct {
			t.Fatalf("local database role retained unmediated capture authority: %v", err)
		}
	}
}

func TestPilotOperationsOfflineCLIFallsBackToSQLAndRequiresSignedIntervalReview(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	_, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	env := pilotOperationsCLIEnv(t, f, directory, "qualification")
	wrong := accountstore.OperationsJournalConfig{Directory: directory, BindingKey: sha256.Sum256([]byte("unregistered-synthetic-observer"))}
	if err := f.Store.SignalLocalOperationsFallback(ctx, wrong, accountstore.OperationsControlFault); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("SQL accepted an unrelated observer: %v", err)
	}
	latch := filepath.Join(directory, "journal.fence")
	if err := os.Chmod(latch, 0400); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(latch, 0600)
	if err := os.Chmod(directory, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(directory, 0700)
	_, _, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "fault", "--kind", "control_fault")
	if err == nil {
		t.Fatal("the offline command reported a successful independent journal write")
	}
	old, err := os.ReadFile(latch)
	if err != nil || !bytes.Equal(old, []byte{0, 0}) {
		t.Fatalf("fixture did not preserve the old latch: %v %v", old, err)
	}
	if err = os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(latch, 0600); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency == nil || !status.ContainmentUnavailable || len(status.Incidents) != 0 {
		t.Fatalf("SQL fallback was missing or the original journal changed: %+v %v", status, err)
	}
	e := status.Emergency
	if len(e.Causes) != 1 || e.Causes[0] != accountstore.OperationsControlFault || !e.StartedAt.Equal(e.RecordedAt) {
		t.Fatalf("lost the original typed cause: %+v", e)
	}
	detached := directory + "-detached"
	if err = os.Rename(directory, detached); err != nil {
		t.Fatal(err)
	}
	out, diagnostic, statusErr := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "status")
	if err = os.Rename(detached, directory); err != nil {
		t.Fatal(err)
	}
	var fallback accountstore.OperationsStatus
	if statusErr != nil || json.Unmarshal(out, &fallback) != nil || fallback.Emergency == nil || fallback.Emergency.ID != e.ID || len(fallback.Incidents) != 0 {
		t.Fatalf("owner status lost its limited SQL receipt while the journal was absent: %+v %v %s", fallback, statusErr, diagnostic)
	}
	blocked := qualificationRequest(t, f, router, "/api/qualification-gaps", "emergency-survives-journal-restoration", "", f.Body(t, "/api/qualification-gaps"))
	if blocked.Code == http.StatusOK || len(provider.Calls) != 0 {
		t.Fatalf("restored readable files reopened inference: %d", blocked.Code)
	}
	if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment"); err != nil {
		t.Fatalf("replay the emergency into the restored journal: %v %s", err, diagnostic)
	}
	status, err = ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency == nil || status.Emergency.ID != e.ID || len(status.Incidents) != 2 {
		t.Fatalf("replay did not retain the emergency and original cause: %+v %v", status, err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "signed-emergency.json")
	env = append(env, "CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path,
		"CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub))
	proof := operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-interval-review", Incident: e.ID,
		Revision: 1, Result: "synthetic_containment_interval_reconciled", ObservedAt: e.RecordedAt}
	writeOperationsProof(t, path, priv, proof)
	args := []string{"--action", "replay-containment", "--reference", proof.Reference}
	if _, _, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err == nil {
		t.Fatal("review as old as the emergency was accepted")
	}
	proof.ObservedAt = time.Now().UTC()
	writeOperationsProof(t, path, priv, proof)
	if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err != nil {
		t.Fatalf("verified replay command failed: %v %s", err, diagnostic)
	}
	status, err = ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency != nil || status.ContainmentUnavailable || len(status.Incidents) != 2 {
		t.Fatalf("verified interval was not retained and cleared independently: %+v %v", status, err)
	}
	var controlIncident accountstore.OperationsIncident
	for _, incident := range status.Incidents {
		if incident.Kind == "control_fault" {
			controlIncident = incident
		}
	}
	if controlIncident.State != "unacknowledged" {
		t.Fatalf("interval verification resolved the original fault by itself: %+v", controlIncident)
	}
	if err = ops.ResolveIncident(ctx, f.Principal, controlIncident.ID, controlIncident.Revision, ""); err != nil {
		t.Fatal(err)
	}
	operator := f.ControlOperator.Principal
	controls := f.Store.LocalControls(nil)
	current, err := controls.Status(ctx, operator, "")
	if err != nil || !current.InvitationStopped || !current.InferenceStopped {
		t.Fatalf("interval recovery reopened either control: %+v %v", current, err)
	}
	verifier, err := aicontrol.NewSyntheticControlRecovery(ctx, f.Store, h)
	if err != nil {
		t.Fatal(err)
	}
	for _, control := range []string{"inference", "invitations"} {
		command := accountstore.ControlCommand{Control: control, ExpectedVersion: current.Version}
		command.RecoveryReference, err = verifier.Prepare(ctx, operator, command)
		if err != nil {
			t.Fatal(err)
		}
		current, err = f.Store.LocalControls(verifier).Apply(ctx, operator, "reopen-after-interval-"+control, command)
		if err != nil {
			t.Fatal(err)
		}
	}
	if current.InferenceStopped || current.InvitationStopped {
		t.Fatalf("verified #290 actions did not reopen both controls: %+v", current)
	}
}

func writeOperationsProof(t *testing.T, path string, key ed25519.PrivateKey, statement operationslocal.RecoveryStatement) {
	t.Helper()
	raw, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := json.Marshal(struct {
		Payload   json.RawMessage
		Signature []byte
	}{raw, ed25519.Sign(key, raw)})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, signed, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPilotOperationsSupportCLIReviewAndCloseAcceptEmptyInput(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	_, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	created, err := ops.CreateCase(ctx, f.Principal, accountstore.OperationsCaseInput{Purpose: "product", Locale: "en",
		Message: "Synthetic product question.", ReceivedAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	env := pilotOperationsCLIEnv(t, f, directory, "qualification")
	update := func(kind string, body []byte) error {
		_, _, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, body,
			"--action", "case-update", "--id", created.ID, "--kind", kind)
		return err
	}
	if err = update("review", nil); err != nil {
		t.Fatalf("empty review failed: %v", err)
	}
	if err = update("respond", nil); err == nil {
		t.Fatal("empty response was accepted")
	}
	const response = "Synthetic substantive response."
	if err = update("respond", []byte(`{"response":"`+response+`"}`)); err != nil {
		t.Fatalf("minimized response failed: %v", err)
	}
	if err = update("close", nil); err != nil {
		t.Fatalf("empty close failed: %v", err)
	}
	if err = ops.WithCase(ctx, f.Principal, created.ID, false, func(c accountstore.OperationsCase) error {
		if !c.NecessityReviewAt.After(created.NecessityReviewAt) || c.RespondedAt == nil || c.ClosedAt == nil || c.Response != response {
			t.Fatalf("case review, response, and closure did not persist: %+v", c)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPilotOperationsJournalOnlyCauseSurvivesAndRequiresSeparateProof(t *testing.T) {
	for _, cause := range []struct {
		kind            accountstore.OperationsFault
		purpose, result string
	}{
		{accountstore.OperationsBackupFault, "backup", "synthetic_restore_check_passed"},
		{accountstore.OperationsProviderFault, "provider", "synthetic_provider_health_check_passed"},
	} {
		t.Run(cause.purpose, func(t *testing.T) {
			f, h, _, _ := qualificationFixture(t)
			_, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
			ctx := context.Background()
			at := time.Now().UTC().Add(-time.Minute)
			limited, err := accountstore.OpenLocalOperationsJournal(accountstore.OperationsJournalConfig{Directory: directory,
				BindingKey: sha256.Sum256([]byte("synthetic-operations-integration")), MaxBytes: 512, Clock: func() time.Time { return at }})
			if err != nil {
				t.Fatal(err)
			}
			if err = limited.SignalOfflineFault(ctx, cause.kind); !errors.Is(err, auth.ErrUnavailable) {
				t.Fatalf("fixture did not fail its main journal write: %v", err)
			}
			status, err := ops.OwnerStatus(ctx, f.Principal)
			if err != nil || status.Emergency != nil || !status.ContainmentUnavailable {
				t.Fatalf("expected the journal to hold the sole emergency receipt: %+v %v", status, err)
			}
			env := pilotOperationsCLIEnv(t, f, directory, "qualification")
			_, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment")
			if err != nil {
				t.Fatalf("replay the independent cause: %v %s", err, diagnostic)
			}
			status, err = ops.OwnerStatus(ctx, f.Principal)
			if err != nil || status.Emergency == nil {
				t.Fatalf("missing owner emergency: %+v %v", status, err)
			}
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "journal-only-recovery.json")
			env = append(env, "CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path, "CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub))
			interval := operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-journal-only-reviewed",
				Incident: status.Emergency.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled"}
			writeOperationsProof(t, path, priv, interval)
			_, diagnostic, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment", "--reference", interval.Reference)
			if err != nil {
				t.Fatalf("reviewed interval failed: %v %s", err, diagnostic)
			}
			status, err = ops.OwnerStatus(ctx, f.Principal)
			if err != nil || status.Emergency != nil {
				t.Fatalf("journal emergency did not clear: %+v %v", status, err)
			}
			var original accountstore.OperationsIncident
			for _, incident := range status.Incidents {
				if incident.Kind == string(cause.kind) {
					original = incident
				}
			}
			if original.State != "unacknowledged" || !original.EventAt.Equal(at) {
				t.Fatalf("original type or onset was lost: %+v", original)
			}
			if err = ops.ResolveIncident(ctx, f.Principal, original.ID, original.Revision, ""); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("interval proof bypassed separate %s verifications: %v", cause.purpose, err)
			}
			proof := operationslocal.RecoveryStatement{Purpose: cause.purpose, Reference: "fixture-typed-recovery", Incident: original.ID,
				Revision: original.Revision, ObservedAt: time.Now().UTC(), Result: cause.result}
			writeOperationsProof(t, path, priv, proof)
			_, diagnostic, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "resolve",
				"--id", original.ID, "--revision", fmt.Sprint(original.Revision), "--reference", proof.Reference)
			if err != nil {
				t.Fatalf("separate typed proof failed: %v %s", err, diagnostic)
			}
		})
	}
}

func TestPilotOperationsActualCLIResolvesOnlySignedBackupAndServiceEpisode(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	j, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	env := pilotOperationsCLIEnv(t, f, directory, "qualification")
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "verified-exercise.json")
	env = append(env, "CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub), "CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path)
	if err = ops.ObserveFault(ctx, accountstore.OperationsBackupFault); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || len(status.Incidents) != 1 {
		t.Fatalf("fixture incident: %+v %v", status, err)
	}
	incident := status.Incidents[0]
	backup := operationslocal.RecoveryStatement{Purpose: "backup", Reference: "fixture-restoration-reviewed",
		Incident: incident.ID, Revision: incident.Revision, Result: "synthetic_restore_check_passed", ObservedAt: time.Now().UTC()}
	wrong := backup
	wrong.Revision++
	writeOperationsProof(t, path, key, wrong)
	args := []string{"--action", "resolve", "--id", incident.ID, "--revision", fmt.Sprint(incident.Revision), "--reference", backup.Reference}
	if _, _, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err == nil {
		t.Fatal("a signed artifact for a different incident revision resolved backup")
	}
	writeOperationsProof(t, path, key, backup)
	if _, diagnostic, e := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); e != nil {
		t.Fatalf("actual command rejected matched independently signed restore observation: %v %s", e, diagnostic)
	}
	operator := f.ControlOperator.Principal
	controls := f.Store.LocalControls(nil)
	current, err := controls.Status(ctx, operator, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controls.Apply(ctx, operator, "fixture-service-invite-stop", accountstore.ControlCommand{
		Control: "invitations", Stop: true, ExpectedVersion: current.Version}); err != nil {
		t.Fatal(err)
	}
	if err = j.PublishOutage(ctx, "service", "investigating", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	initial, err := f.Store.ActiveOperationsOutage(ctx)
	if err != nil || initial == "" {
		t.Fatalf("initial episode: %q %v", initial, err)
	}
	if err = j.PublishOutage(ctx, "service", "recovering", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	same, err := f.Store.ActiveOperationsOutage(ctx)
	if err != nil || same != initial {
		t.Fatalf("same-impact progress replaced the episode: %q %v", same, err)
	}
	verifier, err := aicontrol.NewSyntheticControlRecovery(ctx, f.Store, h)
	if err != nil {
		t.Fatal(err)
	}
	current, err = controls.Status(ctx, operator, "")
	if err != nil {
		t.Fatal(err)
	}
	stale := accountstore.ControlCommand{Control: "inference", ExpectedVersion: current.Version}
	stale.RecoveryReference, err = verifier.Prepare(ctx, operator, stale)
	if err != nil {
		t.Fatal(err)
	}
	next := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if _, diagnostic, e := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "publish",
		"--new-episode", "--impact", "service", "--state", "investigating", "--next-update", next); e != nil {
		t.Fatalf("start another same-impact episode: %v %s", e, diagnostic)
	}
	newID, err := f.Store.ActiveOperationsOutage(ctx)
	if err != nil || newID == "" || newID == initial {
		t.Fatalf("explicit new same-impact episode: %q %v", newID, err)
	}
	if _, err = f.Store.LocalControls(verifier).Apply(ctx, operator, "stale-episode-reopen", stale); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("old-episode reopening proof remained applicable: %v", err)
	}
	var reference string
	for _, control := range []string{"inference", "invitations"} {
		current, err = controls.Status(ctx, operator, "")
		if err != nil {
			t.Fatal(err)
		}
		reopen := accountstore.ControlCommand{Control: control, ExpectedVersion: current.Version}
		reopen.RecoveryReference, err = verifier.Prepare(ctx, operator, reopen)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Store.LocalControls(verifier).Apply(ctx, operator, "fixture-service-recover-"+control, reopen); err != nil {
			t.Fatal(err)
		}
		reference = reopen.RecoveryReference
	}
	if err = ops.PublishVerifiedRecovery(ctx, f.Principal, reference, ""); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("invitation and inference recovery alone asserted service health: %v", err)
	}
	service := operationslocal.RecoveryStatement{Purpose: "service", Reference: "fixture-service-observation",
		Episode: newID, Result: "synthetic_service_probe_passed", ObservedAt: time.Now().UTC()}
	writeOperationsProof(t, path, key, service)
	if _, diagnostic, e := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "publish-recovery", "--id", reference, "--reference", service.Reference); e != nil {
		t.Fatalf("matching signed service proof failed: %v %s", e, diagnostic)
	}
	public, err := j.PublicStatus(ctx)
	if err != nil || public.Status != "recovered" || public.NextUpdateAt != nil {
		t.Fatalf("verified service status: %+v %v", public, err)
	}
}
