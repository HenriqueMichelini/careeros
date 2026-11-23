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
	f, h, _, router := qualificationFixture(t)
	_, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
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
	_, diagnostic, err = runPilotOperationsFixtureCLI(t, "reconciliation-local", env, raw, "--namespace", "qualification", "--mode", "apply")
	if err != nil || !strings.Contains(string(diagnostic), `"Kind":"invariant_breach"`) {
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
	if count != 1 {
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
