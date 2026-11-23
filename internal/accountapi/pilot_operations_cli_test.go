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
	"slices"
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
	for _, mode := range []string{"capture", "capture_unavailable", "reviewer_expires", "finance_only",
		"finance_priced_live", "finance_priced_corrected", "finance_priced_atomicity", "finance_priced_event_finality", "finance_priced_stop_finality",
		"finance_denied_account", "finance_denied_profile", "finance_denied_compromise",
		"priced_live", "priced_replay", "priced_replay_after_correction", "reviewed_maximum_breakdown"} {
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
	if mode == "reviewed_maximum_breakdown" {
		verifyOperationsReviewedMaximumBreakdown(t, f, ops, env, dispatch)
		return
	}
	gross := int64(900)
	evidence := aicontrol.ReconciliationCommand{ID: "fixture-operations-unpriced", Kind: "unpriced", Dispatch: dispatch,
		Evidence: aicontrol.ReconciliationEvidence{Source: "fixture-gross-bill", Revision: 1, Reason: "fixture-unpriced",
			Provider: "typesafe", Model: "jev-1.13.0", Tier: "standard", Rate: "unpriced-fixture-rate", Currency: "EUR",
			BillingNamespace: "synthetic", RequestID: "fixture-reconciliation", Complete: true, Gross: &gross}}
	if mode == "finance_only" {
		evidence.ID = strings.Repeat("u", 128)
	}
	priced := strings.HasPrefix(mode, "priced_") || strings.HasPrefix(mode, "finance_priced_")
	criticalKind := "invariant_breach"
	if priced {
		gross, criticalKind = 2000, "maximum_breach"
		difference := int64(0)
		evidence.ID, evidence.Kind, evidence.Evidence.Reason = "fixture-first-priced-breach", "bill", "fixture-priced"
		evidence.Evidence.Rate, evidence.Evidence.Currency, evidence.Evidence.Difference = aicontrol.QualificationRateID, "USD", &difference
	}
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
	if strings.HasPrefix(mode, "finance_") {
		financeEnv := append(slices.Clone(env), "CAREEROS_OPERATIONS_JOURNAL_DIRECTORY=", "CAREEROS_OPERATIONS_JOURNAL_KEY=")
		if mode == "finance_priced_atomicity" {
			// Simulate a write that returns successfully while the source amount
			// stays unchanged. The private receipt and financial write must roll back together.
			if _, err := f.DB.Exec(`CREATE FUNCTION fixture_skip_finance_accounting() RETURNS trigger
			 LANGUAGE plpgsql AS $fixture$ BEGIN RETURN OLD; END $fixture$;
			 CREATE TRIGGER fixture_skip_finance_accounting BEFORE UPDATE OF counted ON ai_fixture_dispatch
			 FOR EACH ROW EXECUTE FUNCTION fixture_skip_finance_accounting()`); err != nil {
				t.Fatal(err)
			}
			verifyFinanceTransitionRolledBack(t, f, financeEnv, raw)
			return
		}
		if strings.HasSuffix(mode, "_finality") {
			verifyFinanceReceiptFinality(t, f, financeEnv, raw, mode)
			return
		}
		if strings.HasPrefix(mode, "finance_denied_") {
			verifyFinanceReviewerFence(t, f, financeEnv, raw, strings.TrimPrefix(mode, "finance_denied_"))
			return
		}
		_, diagnostic, err = runPilotOperationsFixtureCLI(t, "reconciliation-local", financeEnv, raw, "--namespace", "qualification", "--mode", "apply")
		if err != nil || strings.Contains(string(diagnostic), `"Kind":"`+criticalKind+`"`) {
			t.Fatalf("finance-only command crossed its independent boundary: %v %s", err, diagnostic)
		}
		var inheritedStop, operationsSideEffect, deferred bool
		if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM ai_recon_stop WHERE stopped AND event_id=$1),
		 EXISTS(SELECT 1 FROM careeros_operations_pending)
		 OR EXISTS(SELECT 1 FROM careeros_control_state WHERE operations_fault OR inference_stopped),
		 EXISTS(SELECT 1 FROM careeros_operations_deferred WHERE event_id=$1 AND awaiting_observer)`, evidence.ID).
			Scan(&inheritedStop, &operationsSideEffect, &deferred); err != nil || !inheritedStop || operationsSideEffect || !deferred {
			t.Fatalf("finance-only path must retain #283 and defer independently verified observation: %v %v %v %v", inheritedStop, operationsSideEffect, deferred, err)
		}
		requestedAmounts := map[string]int64{evidence.ID: 1344}
		if priced {
			requestedAmounts[evidence.ID] = 2000
			applyOperationsBillCorrection(t, financeEnv, evidence, 2400)
			requestedAmounts["fixture-priced-correction"] = 2400
		}
		if mode == "finance_priced_corrected" {
			correction := evidence
			correction.ID, correction.Kind, correction.Preview = "fixture-final-price-correction", "corrected_bill", ""
			correction.ExpectedVersion, correction.Evidence.Revision = 2, 3
			reviewedGross := int64(900)
			correction.Evidence.Supersedes, correction.Evidence.Gross = "fixture-priced-correction", &reviewedGross
			applyOperationsReconciliationCLI(t, financeEnv, correction)
			var current int64
			if err := f.DB.QueryRow(`SELECT counted FROM ai_fixture_dispatch WHERE namespace='qualification' AND id=$1`, dispatch).Scan(&current); err != nil || current != reviewedGross {
				t.Fatalf("fixture must correct exposure before owner inspection: %d %v", current, err)
			}
		}
		origins := make(map[int64]time.Time, len(requestedAmounts))
		for command, wantAmount := range requestedAmounts {
			var original time.Time
			var amount, maximum int64
			var typedReceipt bool
			firstDispatch := ""
			if priced && command == evidence.ID {
				firstDispatch = dispatch
			}
			if err = f.DB.QueryRow(`SELECT e.created_at,e.obligation,e.reserved,d.awaiting_observer AND d.source_transaction IS NOT NULL
			 AND octet_length(d.source_binding)=32 AND d.evidence=jsonb_build_object(
			 'ID',e.id,'Rate',$2::text,'Amount',e.obligation,'Maximum',e.reserved,'Kind',$3::text,'At',e.created_at,'IncidentDispatch',$4::text)
			 FROM ai_recon_event e JOIN careeros_operations_deferred d ON d.namespace=e.namespace AND d.event_id=e.id
			 WHERE e.namespace='qualification' AND e.id=$1`, command, aicontrol.QualificationRateID, criticalKind, firstDispatch).
				Scan(&original, &amount, &maximum, &typedReceipt); err != nil || amount != wantAmount || maximum != 1344 || !typedReceipt {
				t.Fatalf("expected the exact committed financial event: %d %d %v", amount, maximum, err)
			}
			origins[amount] = original
		}
		var rawReceiptAccess bool
		if err = f.DB.QueryRow(`WITH reader AS (SELECT rolname FROM pg_roles
		 WHERE rolname='ai_fixture_review_' || substr(encode(sha256(convert_to(current_database() || '/' || current_schema(),'UTF8')),'hex'),1,16))
		 SELECT has_table_privilege(rolname,'careeros_operations_deferred','SELECT,INSERT,UPDATE,DELETE')
		 OR has_table_privilege(rolname,'careeros_operations_pending','INSERT,UPDATE,DELETE')
		 OR has_function_privilege(rolname,'careeros_operations_snapshot(text,text)','EXECUTE')
		 OR has_function_privilege(rolname,'careeros_replay_operations_captures(text)','EXECUTE') FROM reader`).Scan(&rawReceiptAccess); err != nil || rawReceiptAccess {
			t.Fatalf("finance role has raw receipt, delivery or replay access: %v %v", rawReceiptAccess, err)
		}
		previous := make(map[int64]string, len(origins))
		for attempt := range 2 {
			out, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "inspect")
			var status accountstore.OperationsStatus
			if err != nil || json.Unmarshal(out, &status) != nil {
				t.Fatalf("owner inspection failed: %v %s", err, diagnostic)
			}
			var found []accountstore.OperationsIncident
			for _, incident := range status.Incidents {
				if incident.Kind == criticalKind {
					found = append(found, incident)
				}
			}
			if len(found) != len(origins) {
				t.Fatalf("owner inspection lost or duplicated a source event: %+v", found)
			}
			ids := map[string]bool{}
			for _, incident := range found {
				if incident.ID == "" || ids[incident.ID] || !incident.EventAt.Equal(origins[incident.Exposure]) || incident.Severity != "critical" || incident.Cap != 1344 {
					t.Fatalf("owner inspection changed the original identity, amount, or timing: %+v", incident)
				}
				if attempt > 0 && incident.ID != previous[incident.Exposure] {
					t.Fatalf("repeat inspection changed event identity: %+v", incident)
				}
				previous[incident.Exposure], ids[incident.ID] = incident.ID, true
			}
			wantNotices := 0
			if attempt == 0 {
				wantNotices = len(origins)
			}
			if count := strings.Count(string(diagnostic), `"Kind":"`+criticalKind+`"`); count != wantNotices {
				t.Fatalf("want %d new critical notice(s), got %d: %s", wantNotices, count, diagnostic)
			}
		}
		if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM careeros_operations_deferred WHERE namespace='qualification')
		 OR EXISTS(SELECT 1 FROM careeros_operations_pending WHERE critical_event)`).Scan(&deferred); err != nil || deferred {
			t.Fatalf("successful owner delivery left its durable source queue pending: %v %v", deferred, err)
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
	captureUnavailable := mode == "capture_unavailable" || strings.HasPrefix(mode, "priced_replay")
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
	if err != nil || !captureUnavailable && !strings.Contains(string(diagnostic), `"Kind":"`+criticalKind+`"`) {
		t.Fatalf("real command did not deliver its own known critical signal: %v %s", err, diagnostic)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, incident := range status.Incidents {
		if incident.Kind == criticalKind {
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
			evidence.ID).Scan(&eventAt, &obligation, &maximum); err != nil || priced && obligation <= maximum || !priced && obligation != maximum {
			t.Fatalf("fixture lost the original reconciliation event: %d %d %v", obligation, maximum, err)
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
		if mode == "priced_replay_after_correction" {
			applyOperationsBillCorrection(t, env, evidence, 900)
			var counted int64
			if err := f.DB.QueryRow(`SELECT counted FROM ai_fixture_dispatch WHERE namespace='qualification' AND id=$1`, dispatch).Scan(&counted); err != nil || counted >= maximum {
				t.Fatalf("fixture did not correct exposure below the original maximum: %d %v", counted, err)
			}
		}
		if priced {
			if err := ops.Inspect(ctx); err != nil {
				t.Fatal(err)
			}
			before, err := ops.OwnerStatus(ctx, f.Principal)
			if err != nil {
				t.Fatal(err)
			}
			for _, incident := range before.Incidents {
				if incident.Kind == criticalKind {
					t.Fatalf("snapshot replaced a deferred priced event: %+v", incident)
				}
			}
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
			if incident.Kind == criticalKind {
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
			if n.Kind == criticalKind {
				delivered++
			}
		}
		var pending int
		if err = f.DB.QueryRow(`SELECT count(*) FROM careeros_operations_pending WHERE critical_event`).Scan(&pending); err != nil || pending != 0 || delivered != 1 {
			t.Fatalf("successful consumption must acknowledge once: pending=%d delivered=%d %v", pending, delivered, err)
		}
	} else if count != 1 {
		t.Fatalf("expected one retained critical incident: %+v", status.Incidents)
	}
	if mode == "priced_live" {
		var originalAt time.Time
		if err := f.DB.QueryRow(`SELECT created_at FROM ai_recon_event WHERE namespace='qualification' AND id=$1`, evidence.ID).Scan(&originalAt); err != nil {
			t.Fatal(err)
		}
		applyOperationsBillCorrection(t, env, evidence, 2400)
		if err := ops.Inspect(ctx); err != nil {
			t.Fatal(err)
		}
		status, err := ops.OwnerStatus(ctx, f.Principal)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[int64]accountstore.OperationsIncident{}
		for _, incident := range status.Incidents {
			if incident.Kind == criticalKind {
				seen[incident.Exposure] = incident
			}
		}
		if len(seen) != 2 || seen[2000].ID == seen[2400].ID || !seen[2000].EventAt.Equal(originalAt) || !seen[2400].EventAt.After(originalAt) {
			t.Fatalf("live snapshots did not preserve the distinct priced event identities: %+v", seen)
		}
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

func applyOperationsBillCorrection(t *testing.T, env []string, prior aicontrol.ReconciliationCommand, amount int64) {
	t.Helper()
	correction := prior
	correction.ID, correction.Kind, correction.Preview = "fixture-priced-correction", "corrected_bill", ""
	correction.ExpectedVersion, correction.Evidence.Revision = 1, 2
	correction.Evidence.Supersedes, correction.Evidence.Gross = prior.ID, &amount
	applyOperationsReconciliationCLI(t, env, correction)
}

func verifyFinanceReviewerFence(t *testing.T, f *aiadmission.Fixture, env []string, command []byte, fence string) {
	t.Helper()
	var schema string
	if err := f.DB.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	schema = fmt.Sprintf("%q", schema)
	marker := `INSERT INTO %s.careeros_deletion_operations
	 (account_id,operation_id,request_hash,kind,profile_id,profile_revision,state,fenced_at,account_generation)
	 SELECT account.id,'fixture-independent-reviewer-denial',sha256('fixture'::bytea),%s,
	        COALESCE(profile.profile_id,''::bytea),COALESCE(profile.revision,0),'fenced_pending',clock_timestamp(),account.generation
	 FROM %s.careeros_accounts account LEFT JOIN %s.careeros_profiles profile ON profile.account_id=account.id WHERE account.id=actor;`
	var applyMarker string
	switch fence {
	case "account":
		applyMarker = fmt.Sprintf(marker, schema, "'account'", schema, schema)
	case "profile":
		applyMarker = fmt.Sprintf(marker, schema, "'profile'", schema, schema) + fmt.Sprintf(`
		 INSERT INTO %[1]s.careeros_deleted_profiles(account_id,profile_id,operation_id,fenced_at)
		 SELECT account_id,profile_id,'fixture-independent-reviewer-denial',clock_timestamp()
		 FROM %[1]s.careeros_profiles WHERE account_id=actor;`, schema)
	case "compromise":
		applyMarker = fmt.Sprintf(`INSERT INTO %[1]s.careeros_control_accounts(account_id,minimum_generation)
		 SELECT id,generation FROM %[1]s.careeros_accounts WHERE id=actor;`, schema)
	default:
		t.Fatalf("unexpected reviewer fence fixture: %s", fence)
	}
	// Install an independent denial after the command's first authorization.
	// The source attestation must reject it and leave the whole transition undone.
	if _, err := f.DB.Exec(fmt.Sprintf(`CREATE FUNCTION fixture_finance_reviewer_denial() RETURNS trigger
	 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,pg_temp AS $fixture$
	 DECLARE actor text; BEGIN
	 SELECT c.actor INTO STRICT actor FROM %s.ai_recon_command c WHERE c.namespace=NEW.namespace AND c.id=NEW.id;
	 %s
	 IF NOT FOUND THEN RAISE EXCEPTION 'test fixture missing reviewer marker'; END IF;
	 RETURN NEW; END $fixture$;
	 CREATE TRIGGER fixture_finance_reviewer_denial AFTER INSERT ON ai_recon_event
	 FOR EACH ROW EXECUTE FUNCTION fixture_finance_reviewer_denial()`, schema, applyMarker)); err != nil {
		t.Fatal(err)
	}
	verifyFinanceTransitionRolledBack(t, f, env, command)
}

func verifyFinanceTransitionRolledBack(t *testing.T, f *aiadmission.Fixture, env []string, command []byte) {
	t.Helper()
	_, diagnostic, err := runPilotOperationsFixtureCLI(t, "reconciliation-local", env, command, "--namespace", "qualification", "--mode", "apply")
	if err == nil || !strings.Contains(string(diagnostic), aicontrol.ErrUnavailable.Error()) {
		t.Fatalf("attestation must reject the changed source conditions: %v %s", err, diagnostic)
	}
	var changed bool
	if err = f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM ai_recon_command) OR EXISTS(SELECT 1 FROM ai_recon_event)
	 OR EXISTS(SELECT 1 FROM ai_recon_stop WHERE stopped) OR EXISTS(SELECT 1 FROM careeros_operations_deferred)`).
		Scan(&changed); err != nil || changed {
		t.Fatalf("denied attestation changed the financial event, stop, or private observations: %v %v", changed, err)
	}
}

func verifyFinanceReceiptFinality(t *testing.T, f *aiadmission.Fixture, env []string, command []byte, mode string) {
	t.Helper()
	var installed int
	if err := f.DB.QueryRow(`SELECT count(*) FROM pg_trigger
	 WHERE tgrelid IN ('ai_fixture_dispatch'::regclass,'ai_recon_command'::regclass,'ai_recon_event'::regclass,
	  'ai_recon_stop'::regclass,'ai_recon_state'::regclass,'ai_recon_contract'::regclass)
	 AND tgname='careeros_guard_finance_receipt' AND tgenabled='O' AND tgconstraint=0 AND NOT tgdeferrable`).Scan(&installed); err != nil || installed != 6 {
		t.Fatalf("expected immediate guards on all bound finance records: %d %v", installed, err)
	}
	// Simulate two independent bookkeeping faults after the counted amount
	// has been written, so the amount alone still agrees with the receipt.
	var fault string
	switch mode {
	case "finance_priced_event_finality":
		fault = `UPDATE ai_recon_event SET owner_review_required=false WHERE namespace=NEW.namespace AND dispatch=NEW.id;`
	case "finance_priced_stop_finality":
		fault = `UPDATE ai_recon_stop SET stopped=false WHERE namespace=NEW.namespace AND stopped;`
	default:
		t.Fatalf("unexpected finance finality fixture: %s", mode)
	}
	if _, err := f.DB.Exec(`CREATE FUNCTION fixture_finance_bookkeeping_fault() RETURNS trigger
	 LANGUAGE plpgsql AS $fixture$ BEGIN ` + fault + ` RETURN NEW; END $fixture$;
	 CREATE TRIGGER fixture_finance_bookkeeping_fault AFTER UPDATE OF counted ON ai_fixture_dispatch
	 FOR EACH ROW WHEN (NEW.counted>OLD.counted) EXECUTE FUNCTION fixture_finance_bookkeeping_fault()`); err != nil {
		t.Fatal(err)
	}
	verifyFinanceTransitionRolledBack(t, f, env, command)
}

func applyOperationsReconciliationCLI(t *testing.T, env []string, command aicontrol.ReconciliationCommand) {
	t.Helper()
	raw, _ := json.Marshal(command)
	out, diagnostic, err := runPilotOperationsFixtureCLI(t, "reconciliation-local", env, raw, "--namespace", "qualification", "--mode", "preview")
	var preview aicontrol.ReconciliationPreview
	if err != nil || json.Unmarshal(out, &preview) != nil || preview.Token == "" {
		t.Fatalf("preview %s: %v %s", command.Kind, err, diagnostic)
	}
	command.Preview = preview.Token
	raw, _ = json.Marshal(command)
	if _, diagnostic, err = runPilotOperationsFixtureCLI(t, "reconciliation-local", env, raw, "--namespace", "qualification", "--mode", "apply"); err != nil {
		t.Fatalf("apply %s: %v %s", command.Kind, err, diagnostic)
	}
}

func verifyOperationsReviewedMaximumBreakdown(t *testing.T, f *aiadmission.Fixture, ops *accountstore.Operations, env []string, dispatch string) {
	t.Helper()
	ctx := context.Background()
	// Age the fixture's original intent so the real command can review it.
	var maximum int64
	if err := f.DB.QueryRow(`UPDATE ai_fixture_dispatch SET intent_at=clock_timestamp()-interval '169 hours'
	 WHERE namespace='qualification' AND id=$1 RETURNING maximum`, dispatch).Scan(&maximum); err != nil || maximum != 1344 {
		t.Fatalf("fixture original frozen maximum: %d %v", maximum, err)
	}
	check := func(category string, counted, reviewed, estimate, billed, unknown int64) {
		t.Helper()
		status, err := ops.OwnerStatus(ctx, f.Principal)
		figures := status.Figures
		if err != nil || figures == nil || figures.Counted != counted || figures.ConservativeMaximum != reviewed ||
			figures.SupportedEstimate != estimate || figures.AttributableBills != billed || figures.UnknownDifference != unknown || figures.PossibleRemote != 1 {
			t.Fatalf("%s reconciled figures: %+v %v", category, figures, err)
		}
	}
	command := aicontrol.ReconciliationCommand{ID: "fixture-reviewed-maximum", Kind: "maximum", Dispatch: dispatch,
		Evidence: aicontrol.ReconciliationEvidence{Source: "fixture-reviewed-maximum", Revision: 1, Reason: "fixture-reviewed-maximum",
			Provider: "typesafe", Model: "jev-1.13.0", Tier: "standard", Rate: aicontrol.QualificationRateID, Currency: "USD",
			BillingNamespace: "synthetic", RequestID: "fixture-reconciliation", Complete: true}}
	applyOperationsReconciliationCLI(t, env, command)
	var noExpense bool
	if err := f.DB.QueryRow(`SELECT expense IS NULL FROM ai_fixture_dispatch WHERE namespace='qualification' AND id=$1`, dispatch).
		Scan(&noExpense); err != nil || !noExpense {
		t.Fatalf("reviewed maximum unexpectedly has actual expense: %v %v", noExpense, err)
	}
	check(command.Kind, maximum, maximum, 0, 0, maximum)

	input, zero := int64(1000), int64(0)
	command.ID, command.Kind, command.ExpectedVersion = "fixture-supported-estimate", "usage", 1
	command.Evidence.Source, command.Evidence.Supersedes = "fixture-metered-usage", "fixture-reviewed-maximum"
	command.Evidence.Usage, command.Evidence.Difference = &aicontrol.Usage{Input: &input, Output: &zero}, &zero
	applyOperationsReconciliationCLI(t, env, command)
	check(command.Kind, 42, 0, 42, 0, 0)

	bill := int64(80)
	command.ID, command.Kind, command.ExpectedVersion = "fixture-confirmed-bill", "bill", 2
	command.Evidence.Source, command.Evidence.Supersedes = "fixture-confirmed-bill", "fixture-supported-estimate"
	command.Evidence.Usage, command.Evidence.Gross = nil, &bill
	applyOperationsReconciliationCLI(t, env, command)
	check(command.Kind, bill, 0, 0, bill, 0)
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
			at := time.Now().UTC().Add(-2 * time.Minute)
			latest := at.Add(time.Minute)
			observed := at
			limited, err := accountstore.OpenLocalOperationsJournal(accountstore.OperationsJournalConfig{Directory: directory,
				BindingKey: sha256.Sum256([]byte("synthetic-operations-integration")), MaxBytes: 512, Clock: func() time.Time { return observed }})
			if err != nil {
				t.Fatal(err)
			}
			if err = limited.SignalOfflineFault(ctx, cause.kind); !errors.Is(err, auth.ErrUnavailable) {
				t.Fatalf("fixture did not fail its main journal write: %v", err)
			}
			observed = latest
			if err = limited.SignalOfflineFault(ctx, cause.kind); !errors.Is(err, auth.ErrUnavailable) {
				t.Fatalf("fixture did not fail the later main journal write: %v", err)
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
				Incident: status.Emergency.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled",
				ReceiptSet: status.Emergency.ReceiptSet, ClassifiedCauses: []accountstore.OperationsFault{"no_reportable_fault"}}
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
			if original.State != "unacknowledged" || !original.EventAt.Equal(at) || original.LatestObservedAt == nil || !original.LatestObservedAt.Equal(latest) {
				t.Fatalf("original type, onset, or latest fault was lost: %+v", original)
			}
			if err = ops.ResolveIncident(ctx, f.Principal, original.ID, original.Revision, ""); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("interval proof bypassed separate %s verifications: %v", cause.purpose, err)
			}
			proof := operationslocal.RecoveryStatement{Purpose: cause.purpose, Reference: "fixture-typed-recovery", Incident: original.ID,
				Revision: original.Revision, ObservedAt: at.Add(time.Second), Result: cause.result}
			writeOperationsProof(t, path, priv, proof)
			resolve := []string{"--action", "resolve", "--id", original.ID, "--revision", fmt.Sprint(original.Revision), "--reference", proof.Reference}
			if _, _, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, resolve...); err == nil {
				t.Fatal("proof preceding the latest retained fault resolved the incident")
			}
			proof.ObservedAt = time.Now().UTC()
			writeOperationsProof(t, path, priv, proof)
			_, diagnostic, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, resolve...)
			if err != nil {
				t.Fatalf("separate typed proof failed: %v %s", err, diagnostic)
			}
		})
	}
}

func TestPilotOperationsRepeatedFaultKeepsFirstNoticeAndRequiresCurrentProof(t *testing.T) {
	for _, mode := range []string{"journal", "sql_fallback"} {
		t.Run(mode, func(t *testing.T) {
			f, h, _, _ := qualificationFixture(t)
			ctx := context.Background()
			first := time.Now().UTC().Add(-2 * time.Minute)
			latest := first.Add(time.Minute)
			observed := first
			j, ops, notices, directory := pilotOperationsFixture(t, f, h, func() time.Time { return observed })
			env := pilotOperationsCLIEnv(t, f, directory, "qualification")
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "repeat-recovery.json")
			env = append(env, "CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path, "CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub))
			if mode == "journal" {
				for _, observedAt := range []time.Time{first, latest} {
					observed = observedAt
					if err = j.SignalOfflineFault(ctx, accountstore.OperationsBackupFault); err != nil {
						t.Fatal(err)
					}
				}
				if len(*notices) != 1 {
					t.Fatalf("repeated observation duplicated the original notice: %+v", *notices)
				}
			} else {
				cfg := accountstore.OperationsJournalConfig{Directory: directory,
					BindingKey: sha256.Sum256([]byte("synthetic-operations-integration"))}
				for i := 0; i < 2; i++ {
					if err = f.Store.SignalLocalOperationsFallback(ctx, cfg, accountstore.OperationsBackupFault); err != nil {
						t.Fatal(err)
					}
				}
				status, err := ops.OwnerStatus(ctx, f.Principal)
				if err != nil || status.Emergency == nil || !status.Emergency.RecordedAt.After(status.Emergency.StartedAt) {
					t.Fatalf("SQL did not preserve separate first and latest observations: %+v %v", status, err)
				}
				first, latest = status.Emergency.StartedAt, status.Emergency.RecordedAt
				review := operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-repeat-sql-reviewed",
					Incident: status.Emergency.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled"}
				writeOperationsProof(t, path, priv, review)
				if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment", "--reference", review.Reference); err != nil {
					t.Fatalf("replay repeated SQL backup observations: %v %s", err, diagnostic)
				}
			}
			status, err := ops.OwnerStatus(ctx, f.Principal)
			if err != nil || status.Emergency != nil {
				t.Fatalf("private operations status: %+v %v", status, err)
			}
			var backup accountstore.OperationsIncident
			for _, incident := range status.Incidents {
				if incident.Kind == "backup_fault" {
					backup = incident
				}
			}
			if backup.Revision != 1 || !backup.EventAt.Equal(first) || backup.LatestObservedAt == nil || !backup.LatestObservedAt.Equal(latest) {
				t.Fatalf("fault observations changed first identity or lost latest time: %+v", backup)
			}
			proof := operationslocal.RecoveryStatement{Purpose: "backup", Reference: "fixture-backup-repeat-proof",
				Incident: backup.ID, Revision: backup.Revision, ObservedAt: first, Result: "synthetic_restore_check_passed"}
			args := []string{"--action", "resolve", "--id", backup.ID, "--revision", "1", "--reference", proof.Reference}
			writeOperationsProof(t, path, priv, proof)
			if _, _, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err == nil {
				t.Fatal("signed proof for the first occurrence resolved a later observation")
			}
			proof.ObservedAt = time.Now().UTC()
			writeOperationsProof(t, path, priv, proof)
			if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err != nil {
				t.Fatalf("current signed backup proof did not resolve: %v %s", err, diagnostic)
			}
		})
	}
}

func TestPilotOperationsPartialFaultReceiptRequiresVerifiedClassification(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "existing_partial"
		if replace {
			name = "atomic_replacement"
		}
		t.Run(name, func(t *testing.T) {
			f, h, _, _ := qualificationFixture(t)
			j, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
			ctx := context.Background()
			legacy := filepath.Join(directory, "journal.fault.backup_fault")
			if err := os.WriteFile(legacy, []byte(`{"payload":`), 0600); err != nil {
				t.Fatal(err)
			}
			if replace {
				if err := j.SignalOfflineFault(ctx, accountstore.OperationsBackupFault); !errors.Is(err, auth.ErrUnavailable) {
					t.Fatalf("fixture should still report the failed main write: %v", err)
				}
				raw, err := os.ReadFile(legacy)
				if err != nil || !json.Valid(raw) {
					t.Fatalf("complete fault could not replace the legacy partial file: %v", err)
				}
			}
			env := pilotOperationsCLIEnv(t, f, directory, "qualification")
			if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment"); err != nil {
				t.Fatalf("partial receipt could not enter verified review: %v %s", err, diagnostic)
			}
			status, err := ops.OwnerStatus(ctx, f.Principal)
			if err != nil || status.Emergency == nil || !status.ContainmentUnavailable ||
				!slices.Contains(status.Emergency.Causes, accountstore.OperationsFault("journal_cause_unconfirmed")) {
				t.Fatalf("partial receipt was silently treated as known: %+v %v", status, err)
			}
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "partial-receipt-review.json")
			env = append(env, "CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path, "CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub))
			proof := operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-classified-interval",
				Incident: status.Emergency.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled",
				ReceiptSet: status.Emergency.ReceiptSet}
			args := []string{"--action", "replay-containment", "--reference", proof.Reference}
			writeOperationsProof(t, path, priv, proof)
			if _, _, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err == nil {
				t.Fatal("unclassified signed interval cleared a legacy partial receipt")
			}
			if _, err = os.Stat(legacy); err != nil {
				t.Fatalf("failed review discarded the fail-closed sidecar: %v", err)
			}
			proof.ClassifiedCauses = []accountstore.OperationsFault{accountstore.OperationsBackupFault}
			writeOperationsProof(t, path, priv, proof)
			if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err != nil {
				t.Fatalf("signed classification could not clear reviewed sidecar: %v %s", err, diagnostic)
			}
			if _, err = os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("reviewed sidecar cleanup failed: %v", err)
			}
			controls, err := f.Store.LocalControls(nil).Status(ctx, f.ControlOperator.Principal, "")
			if err != nil || !controls.InferenceStopped || !controls.InvitationStopped {
				t.Fatalf("verified classification auto-reopened controls: %+v %v", controls, err)
			}
		})
	}
}

func TestPilotOperationsClassifiedRecurrenceRequiresProofAfterItsReview(t *testing.T) {
	for _, cause := range []struct {
		kind            accountstore.OperationsFault
		purpose, result string
	}{
		{accountstore.OperationsBackupFault, "backup", "synthetic_restore_check_passed"},
		{accountstore.OperationsProviderFault, "provider", "synthetic_provider_health_check_passed"},
	} {
		t.Run(cause.purpose, func(t *testing.T) {
			f, h, _, _ := qualificationFixture(t)
			j, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
			ctx := context.Background()
			if err := j.SignalOfflineFault(ctx, cause.kind); err != nil {
				t.Fatal(err)
			}
			cfg := accountstore.OperationsJournalConfig{Directory: directory,
				BindingKey: sha256.Sum256([]byte("synthetic-operations-integration"))}
			if err := f.Store.SignalLocalOperationsFallback(ctx, cfg, accountstore.OperationsControlFault); err != nil {
				t.Fatal(err)
			}
			status, err := ops.OwnerStatus(ctx, f.Principal)
			if err != nil || status.Emergency == nil || len(status.Incidents) != 1 {
				t.Fatalf("fixture must have the original fault and an existing emergency: %+v %v", status, err)
			}
			original, emergency := status.Incidents[0], status.Emergency
			earlierProof := time.Now().UTC()
			partial := filepath.Join(directory, "journal.fault."+string(cause.kind))
			if err := os.WriteFile(partial, []byte(`{"payload":`), 0600); err != nil {
				t.Fatal(err)
			}
			env := pilotOperationsCLIEnv(t, f, directory, "qualification")
			if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment"); err != nil {
				t.Fatalf("record the separately unclassified recurrence: %v %s", err, diagnostic)
			}
			status, err = ops.OwnerStatus(ctx, f.Principal)
			if err != nil || status.Emergency == nil || status.Emergency.ID == emergency.ID || !status.Emergency.StartedAt.Equal(emergency.StartedAt) {
				t.Fatalf("new receipt must advance the review without replacing the start: %+v %v", status, err)
			}

			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "classification-and-recovery.json")
			env = append(env, "CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path, "CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub))
			classification := operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-recurrence-review",
				Incident: status.Emergency.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled",
				ReceiptSet: status.Emergency.ReceiptSet, ClassifiedCauses: []accountstore.OperationsFault{cause.kind}}
			writeOperationsProof(t, path, priv, classification)
			if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment",
				"--reference", classification.Reference); err != nil {
				t.Fatalf("verify the classification: %v %s", err, diagnostic)
			}
			status, err = ops.OwnerStatus(ctx, f.Principal)
			if err != nil {
				t.Fatal(err)
			}
			var classified accountstore.OperationsIncident
			for _, incident := range status.Incidents {
				if incident.Kind == string(cause.kind) {
					classified = incident
				}
			}
			if classified.ID != original.ID || classified.Revision != original.Revision || !classified.EventAt.Equal(original.EventAt) ||
				classified.LatestObservedAt == nil || !classified.LatestObservedAt.Equal(classification.ObservedAt) || classified.State != "unacknowledged" {
				t.Fatalf("classification replaced the original incident or lost its latest bound: %+v", classified)
			}

			recovery := operationslocal.RecoveryStatement{Purpose: cause.purpose, Reference: "fixture-cause-recovery",
				Incident: original.ID, Revision: original.Revision, Result: cause.result}
			args := []string{"--action", "resolve", "--id", original.ID, "--revision", fmt.Sprint(original.Revision), "--reference", recovery.Reference}
			for _, tooEarly := range []time.Time{earlierProof, classification.ObservedAt} {
				recovery.ObservedAt = tooEarly
				writeOperationsProof(t, path, priv, recovery)
				if _, _, err = runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err == nil {
					t.Fatal("cause-specific proof without a later observation resolved the classified recurrence")
				}
			}
			recovery.ObservedAt = time.Now().UTC()
			writeOperationsProof(t, path, priv, recovery)
			if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, args...); err != nil {
				t.Fatalf("current cause-specific proof did not resolve the incident: %v %s", err, diagnostic)
			}
		})
	}
}

func TestPilotOperationsSignedClearanceFollowsExactReceiptSetAndRetainsBothCauses(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	j, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	if err := j.SignalOfflineFault(ctx, accountstore.OperationsBackupFault); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || len(status.Incidents) != 1 {
		t.Fatalf("fixture needs the existing backup fault: %+v %v", status, err)
	}
	original := status.Incidents[0]
	backupFile := filepath.Join(directory, "journal.fault.backup_fault")
	providerFile := filepath.Join(directory, "journal.fault.provider_fault")
	partial := []byte(`{"payload":`)
	if err = os.WriteFile(backupFile, partial, 0600); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "receipt-set-proof.json")
	env := append(pilotOperationsCLIEnv(t, f, directory, "qualification"),
		"CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path, "CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub))
	prepare := func() accountstore.OperationsEmergency {
		t.Helper()
		if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment"); err != nil {
			t.Fatalf("prepare current receipt set: %v %s", err, diagnostic)
		}
		status, err := ops.OwnerStatus(ctx, f.Principal)
		if err != nil || status.Emergency == nil || len(status.Emergency.ReceiptSet) != 64 || !status.ContainmentUnavailable {
			t.Fatalf("prepared status must expose the private receipt set: %+v %v", status, err)
		}
		return *status.Emergency
	}
	classify := func(e accountstore.OperationsEmergency, kinds ...accountstore.OperationsFault) operationslocal.RecoveryStatement {
		return operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-exact-receipt-set",
			Incident: e.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled",
			ReceiptSet: e.ReceiptSet, ClassifiedCauses: kinds}
	}
	clear := func(proof operationslocal.RecoveryStatement, allowed bool) {
		t.Helper()
		writeOperationsProof(t, path, priv, proof)
		_, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment",
			"--reference", proof.Reference)
		if allowed != (err == nil) {
			t.Fatalf("clearance allowed=%v: %v %s", allowed, err, diagnostic)
		}
		for _, receipt := range []string{backupFile, providerFile} {
			_, err = os.Stat(receipt)
			if allowed && !errors.Is(err, os.ErrNotExist) || !allowed && err != nil {
				t.Fatalf("clearance allowed=%v left unexpected receipt state: %v", allowed, err)
			}
		}
	}
	first := prepare()
	firstProof := classify(first, accountstore.OperationsBackupFault)
	if err = os.WriteFile(providerFile, partial, 0600); err != nil {
		t.Fatal(err)
	}
	clear(firstProof, false)
	added := prepare()
	if added.ID == first.ID || added.ReceiptSet == first.ReceiptSet || !added.StartedAt.Equal(first.StartedAt) {
		t.Fatalf("added unreadable receipt must advance the review generation: %+v", added)
	}
	addedProof := classify(added, accountstore.OperationsBackupFault, accountstore.OperationsProviderFault)
	replacement := filepath.Join(directory, "fixture-interrupted-receipt")
	if err = os.WriteFile(replacement, partial, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(replacement, providerFile); err != nil {
		t.Fatal(err)
	}
	clear(addedProof, false)
	current := prepare()
	if current.ID == added.ID || current.ReceiptSet == added.ReceiptSet || !current.StartedAt.Equal(first.StartedAt) {
		t.Fatalf("replacement with identical bytes must advance the receipt generation: %+v", current)
	}
	currentProof := classify(current, accountstore.OperationsBackupFault, accountstore.OperationsProviderFault)
	wrongSet := currentProof
	wrongSet.ReceiptSet = added.ReceiptSet
	clear(wrongSet, false)
	clear(currentProof, true)

	status, err = ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency != nil || status.ContainmentUnavailable {
		t.Fatalf("verified clearance did not retire the reviewed set: %+v %v", status, err)
	}
	causes := map[string]accountstore.OperationsIncident{}
	for _, incident := range status.Incidents {
		if incident.Kind == "backup_fault" || incident.Kind == "provider_fault" {
			if incident.State != "unacknowledged" || incident.LatestObservedAt == nil || !incident.LatestObservedAt.Equal(currentProof.ObservedAt) {
				t.Fatalf("classification did not preserve a typed open cause and its time bound: %+v", incident)
			}
			causes[incident.Kind] = incident
		}
	}
	backup, provider := causes["backup_fault"], causes["provider_fault"]
	if len(causes) != 2 || backup.ID != original.ID || !backup.EventAt.Equal(original.EventAt) || !provider.EventAt.Equal(first.StartedAt) {
		t.Fatalf("verified classification lost a cause or its first incident time: %+v", causes)
	}
	resolve := func(incident accountstore.OperationsIncident, purpose, result string) {
		t.Helper()
		proof := operationslocal.RecoveryStatement{Purpose: purpose, Reference: "fixture-separate-cause-proof", Incident: incident.ID,
			Revision: incident.Revision, ObservedAt: time.Now().UTC(), Result: result}
		writeOperationsProof(t, path, priv, proof)
		if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "resolve",
			"--id", incident.ID, "--revision", fmt.Sprint(incident.Revision), "--reference", proof.Reference); err != nil {
			t.Fatalf("resolve the specific cause: %v %s", err, diagnostic)
		}
	}
	resolve(backup, "backup", "synthetic_restore_check_passed")
	status, err = ops.OwnerStatus(ctx, f.Principal)
	if err != nil || !slices.ContainsFunc(status.Incidents, func(incident accountstore.OperationsIncident) bool {
		return incident.ID == provider.ID && incident.State == "unacknowledged"
	}) {
		t.Fatalf("backup proof also resolved the provider cause: %+v %v", status, err)
	}
	resolve(provider, "provider", "synthetic_provider_health_check_passed")
}

func TestPilotOperationsClearanceKeepsReceiptChangedDuringVerification(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	j, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	receipt := filepath.Join(directory, "journal.fault.backup_fault")
	if err := os.WriteFile(receipt, []byte(`{"payload":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ops.ReplayEmergency(ctx, f.Principal, ""); err != nil {
		t.Fatal(err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency == nil || status.Emergency.ReceiptSet == "" {
		t.Fatalf("prepare the initial interrupted receipt: %+v %v", status, err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "signed-receipt-review.json")
	t.Setenv("CAREEROS_OPERATIONS_RECOVERY_ARTIFACT", path)
	t.Setenv("CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY", hex.EncodeToString(pub))
	proof := operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-verified-then-changed",
		Incident: status.Emergency.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled",
		ReceiptSet: status.Emergency.ReceiptSet, ClassifiedCauses: []accountstore.OperationsFault{accountstore.OperationsBackupFault}}
	writeOperationsProof(t, path, priv, proof)
	verifier, err := operationslocal.Recovery()
	if err != nil {
		t.Fatal(err)
	}
	replacement := []byte(`{"payload":{"kind":`)
	ops, err = f.Store.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "fixture-receipt-changes-during-review",
		VerifyEmergencyReview: func(ctx context.Context, reference string, e accountstore.OperationsEmergency) (accountstore.OperationsEmergencyReview, error) {
			review, err := verifier.VerifyEmergency(ctx, reference, e)
			if err != nil {
				return review, err
			}
			return review, os.WriteFile(receipt, replacement, 0600)
		}}, h.LocalOperationsSource(), j)
	if err != nil {
		t.Fatal(err)
	}
	if err = ops.ReplayEmergency(ctx, f.Principal, proof.Reference); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("a receipt changed after verification must preserve the stop: %v", err)
	}
	raw, err := os.ReadFile(receipt)
	if err != nil || !bytes.Equal(raw, replacement) {
		t.Fatalf("clearance discarded an unreviewed receipt: %v", err)
	}
	status, err = ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency == nil || status.Emergency.ID != proof.Incident || !status.ContainmentUnavailable {
		t.Fatalf("changed receipt must retain the current stop: %+v %v", status, err)
	}
}

func TestPilotOperationsMarkerOnlyClearanceUsesCurrentFallbackGeneration(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	_, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "marker-interval.json")
	env := append(pilotOperationsCLIEnv(t, f, directory, "qualification"),
		"CAREEROS_OPERATIONS_RECOVERY_ARTIFACT="+path, "CAREEROS_OPERATIONS_RECOVERY_PUBLIC_KEY="+hex.EncodeToString(pub))
	prepare := func() accountstore.OperationsEmergency {
		t.Helper()
		if _, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment"); err != nil {
			t.Fatalf("prepare marker interval: %v %s", err, diagnostic)
		}
		status, err := ops.OwnerStatus(ctx, f.Principal)
		if err != nil || status.Emergency == nil || len(status.Emergency.ReceiptSet) != 64 || !status.ContainmentUnavailable {
			t.Fatalf("expected a closed, reviewable marker interval: %+v %v", status, err)
		}
		return *status.Emergency
	}
	proofFor := func(e accountstore.OperationsEmergency) operationslocal.RecoveryStatement {
		return operationslocal.RecoveryStatement{Purpose: "containment", Reference: "fixture-markers-review",
			Incident: e.ID, Revision: 1, ObservedAt: time.Now().UTC(), Result: "synthetic_containment_interval_reconciled",
			ReceiptSet: e.ReceiptSet, ClassifiedCauses: []accountstore.OperationsFault{accountstore.OperationsBackupFault}}
	}
	clear := func(reference string, allowed bool) {
		t.Helper()
		_, diagnostic, err := runPilotOperationsFixtureCLI(t, "pilot-operations-local", env, nil, "--action", "replay-containment",
			"--reference", reference)
		if allowed != (err == nil) {
			t.Fatalf("marker review allowed=%v: %v %s", allowed, err, diagnostic)
		}
	}
	var previous accountstore.OperationsEmergency
	markers := []string{"journal.pending", "journal.uncertain", "journal.queued", "journal.fence"}
	for _, name := range markers {
		var oldProof operationslocal.RecoveryStatement
		if previous.ID != "" {
			oldProof = proofFor(previous)
			writeOperationsProof(t, path, priv, oldProof)
		}
		marker := []byte{1}
		if name == "journal.fence" {
			marker = []byte{1, 1}
		}
		// Represent an interrupted local write that left only the durable marker.
		file := filepath.Join(directory, name)
		if err = os.WriteFile(file, marker, 0600); err != nil {
			t.Fatal(err)
		}
		if previous.ID != "" {
			clear(oldProof.Reference, false)
			if raw, err := os.ReadFile(file); err != nil || !bytes.Equal(raw, marker) {
				t.Fatalf("older clearance changed the new %s generation: %v", name, err)
			}
		}
		current := prepare()
		if previous.ID == current.ID || previous.ReceiptSet == current.ReceiptSet {
			t.Fatalf("%s did not advance the signed recovery generation: %+v", name, current)
		}
		previous = current
	}
	if unchanged := prepare(); unchanged.ID != previous.ID || unchanged.ReceiptSet != previous.ReceiptSet {
		t.Fatalf("repeated reads of unchanged markers advanced generation: %+v", unchanged)
	}
	proof := proofFor(previous)
	unclassified := proof
	unclassified.ClassifiedCauses = nil
	writeOperationsProof(t, path, priv, unclassified)
	clear(unclassified.Reference, false)
	writeOperationsProof(t, path, priv, proof)
	clear(proof.Reference, true)
	for _, name := range markers[:3] {
		if _, err = os.Stat(filepath.Join(directory, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("reviewed marker %s was retained: %v", name, err)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(directory, "journal.fence")); err != nil || !bytes.Equal(raw, []byte{0, 0}) {
		t.Fatalf("reviewed latch did not clear: %v", err)
	}
	status, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || status.Emergency != nil || !slices.ContainsFunc(status.Incidents, func(incident accountstore.OperationsIncident) bool {
		return incident.Kind == string(accountstore.OperationsBackupFault) && incident.State == "unacknowledged" &&
			incident.LatestObservedAt != nil && incident.LatestObservedAt.Equal(proof.ObservedAt)
	}) {
		t.Fatalf("verified marker interval did not preserve the independently classified open cause: %+v %v", status, err)
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
