package accountapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
)

type pauseInspection struct {
	accountstore.OperationsRiskSource
	accountstore.OperationsRiskBinding
	drain   func(context.Context, func(accountstore.OperationsRiskSample) error) error
	sampled chan struct{}
}

func (s pauseInspection) BindOperations(*accountstore.Operations) {}

func (s pauseInspection) DrainOperations(ctx context.Context, publish func(accountstore.OperationsRiskSample) error) error {
	err := s.drain(ctx, publish)
	close(s.sampled)
	return err
}

func awaitOperationsPhase(t *testing.T, arrived <-chan struct{}) {
	t.Helper()
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("controlled operations phase did not arrive")
	}
}

func TestPilotOperationsDurableCrossingSurvivesNewerBelowCapDisplay(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	j, ops, notices, _ := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	source := h.LocalOperationsSource()
	paused := pauseInspection{OperationsRiskSource: source, OperationsRiskBinding: source.(accountstore.OperationsRiskBinding),
		drain: source.(interface {
			DrainOperations(context.Context, func(accountstore.OperationsRiskSample) error) error
		}).DrainOperations,
		sampled: make(chan struct{}),
	}
	separate, err := accountstore.Open(ctx, f.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer separate.Close()
	inspector, err := separate.LocalOperations(accountstore.OperationsConfig{Owner: "Henrique", Build: "synthetic-inspector"}, paused, j)
	if err != nil {
		t.Fatal(err)
	}
	staged, allowCommit, committed, allowDelivery := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer func() {
		for _, ch := range []chan struct{}{allowCommit, allowDelivery} {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
	}()
	reservations := 0
	h.SetFault("after_operations_reserve_snapshot", func() error {
		reservations++
		if reservations == 3 {
			close(staged)
			<-allowCommit
		}
		return nil
	})
	h.SetFault("before_operations_reserve_delivery", func() error {
		if reservations == 3 {
			close(committed)
			<-allowDelivery
		}
		return nil
	})
	h.SetFault("before_claim", func() error {
		if reservations == 3 {
			return aicontrol.ErrUnavailable
		}
		return nil
	})
	body := f.Body(t, "/api/qualification-gaps")
	httpDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		httpDone <- qualificationRequest(t, f, router, "/api/qualification-gaps", "durable-old-crossing", "", body)
	}()
	awaitOperationsPhase(t, staged)
	inspected := make(chan error, 1)
	go func() { inspected <- inspector.Inspect(ctx) }()
	awaitOperationsPhase(t, paused.sampled)
	close(allowCommit)
	awaitOperationsPhase(t, committed)
	if err = <-inspected; err != nil {
		t.Fatal(err)
	}
	display, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || display.Figures == nil || display.Figures.Counted != 13344 {
		t.Fatalf("simultaneous inspector should see the prior below-bound ledger: %+v %v", display, err)
	}
	close(allowDelivery)
	<-httpDone
	end, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || end.Figures == nil || !end.Figures.At.Equal(display.Figures.At) || len(provider.Calls) != 2 {
		t.Fatalf("older observation regressed display or entered the third fake provider: %+v %v", end, err)
	}
	warnings := 0
	for _, n := range *notices {
		if n.Kind == "spend_warning" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("committed older threshold delivered %d warnings, want 1", warnings)
	}
	for _, incident := range end.Incidents {
		if incident.Kind == "spend_warning" && incident.Threshold == 24000 && incident.Exposure == 25344 &&
			incident.EventAt.Before(display.Figures.At) {
			return
		}
	}
	t.Fatalf("missing original derived warning in the durable list: %+v", end.Incidents)
}

type syntheticBodyTimeout struct{}

func (syntheticBodyTimeout) Read([]byte) (int, error) { return 0, context.DeadlineExceeded }
func (syntheticBodyTimeout) Close() error             { return nil }

func TestPilotOperationsTimeoutAfterHeadersSignalsProviderAndRetainsUnknown(t *testing.T) {
	f, h, _, router := qualificationFixture(t)
	_, ops, notices, _ := pilotOperationsFixture(t, f, h, time.Now)
	calls := 0
	if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: syntheticBodyTimeout{}}, false, nil
	}); err != nil {
		t.Fatal(err)
	}
	response := qualificationRequest(t, f, router, "/api/qualification-gaps", "timeout-after-headers", "", f.Body(t, "/api/qualification-gaps"))
	if response.Code == 200 || calls != 1 || len(*notices) != 2 {
		t.Fatalf("body timeout response/calls/notices: %d %d %+v", response.Code, calls, *notices)
	}
	if (*notices)[0].Kind != "unknown_dispatch" || (*notices)[1].Kind != "provider_fault" {
		t.Fatalf("body timeout lost purpose-separated signal: %+v", *notices)
	}
	status, err := ops.OwnerStatus(context.Background(), f.Principal)
	if err != nil || status.Figures == nil || status.Figures.PossibleRemote != 1 || status.Figures.Counted != 1344 {
		t.Fatalf("timeout changed independent hold: %+v %v", status, err)
	}
}

func TestPilotOperationsExpiredSessionAfterFinalJournalWaitNeverEntersFakeProvider(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	_, _, notices, directory := pilotOperationsFixture(t, f, h, time.Now)
	arrived, proceed := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-proceed:
		default:
			close(proceed)
		}
	}()
	h.SetFault("after_dispatch_lease", func() error { close(arrived); <-proceed; return nil })
	body := f.Body(t, "/api/qualification-gaps")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- qualificationRequest(t, f, router, "/api/qualification-gaps", "expires-after-outage-journal", "", body)
	}()
	awaitOperationsPhase(t, arrived)
	lock, err := os.OpenFile(filepath.Join(directory, "journal.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	var expiry time.Time
	if err = f.DB.QueryRow(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '1 second'
	WHERE id=$1 RETURNING expires_at`, f.Principal.SessionID).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	close(proceed)
	time.Sleep(time.Until(expiry) + 50*time.Millisecond)
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	response := <-done
	if response.Code == 200 || len(provider.Calls) != 0 || len(*notices) != 1 || (*notices)[0].Kind != "unknown_dispatch" {
		t.Fatalf("stale exact session reached the fake provider or erased its durable intent: %d %+v %+v", response.Code, provider.Calls, *notices)
	}
}

func TestPilotOperationsRestrictedRightsSessionHasNoOnlineOwnerAuthority(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	j, ops, _, _ := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	key := sha256.Sum256([]byte("fixture-offline-owner-separate"))
	if err := ops.ProvisionOfflineOwner(ctx, f.Principal, key); err != nil {
		t.Fatal(err)
	}
	controls := f.Store.LocalControls(nil)
	before, err := controls.Status(ctx, f.ControlOperator.Principal, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = controls.Apply(ctx, f.ControlOperator.Principal, "synthetic-compromise-operations-owner", accountstore.ControlCommand{
		Control: "compromise", AccountID: f.Principal.AccountID, Stop: true, ExpectedVersion: before.Version})
	if err != nil {
		t.Fatal(err)
	}
	restricted := controlFreshIdentity(t, f.Service, "fixture-account").Principal
	if _, err = ops.OwnerStatus(ctx, restricted); !(errors.Is(err, auth.ErrForbidden) || errors.Is(err, auth.ErrAuthenticationRequired)) {
		t.Fatalf("restricted rights session inherited online owner authority: %v", err)
	}
	if err = j.WithOfflineOwnerStatus(ctx, key, func(accountstore.OperationsStatus) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestPilotOperationsUnwritableLatchPersistsIndependentUncertainty(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	j, ops, _, directory := pilotOperationsFixture(t, f, h, time.Now)
	ctx := context.Background()
	offline := sha256.Sum256([]byte("fixture-offline-owner-latch"))
	if err := ops.ProvisionOfflineOwner(ctx, f.Principal, offline); err != nil {
		t.Fatal(err)
	}
	latch := filepath.Join(directory, "journal.fence")
	if err := os.Chmod(latch, 0400); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(latch, 0600)
	if err := j.SignalOfflineFault(ctx, accountstore.OperationsControlFault); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatalf("readable but unwritable latch accepted a required update: %v", err)
	}
	raw, err := os.ReadFile(latch)
	if err != nil || !bytes.Equal(raw, []byte{0, 0}) {
		t.Fatalf("fixture did not preserve old healthy marker bytes: %v %v", raw, err)
	}
	if err = os.Chmod(latch, 0600); err != nil {
		t.Fatal(err)
	}
	separate, err := accountstore.OpenLocalOperationsJournal(accountstore.OperationsJournalConfig{
		Directory: directory, BindingKey: sha256.Sum256([]byte("synthetic-operations-integration"))})
	if err != nil {
		t.Fatal(err)
	}
	if err = separate.WithOfflineOwnerStatus(ctx, offline, func(s accountstore.OperationsStatus) error {
		if !s.ContainmentUnavailable || len(s.Incidents) != 0 {
			t.Fatalf("independent uncertainty disappeared with recovery of latch file writes: %+v", s)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	controls, err := f.Store.LocalControls(nil).Status(ctx, f.ControlOperator.Principal, "")
	if err != nil || controls.InferenceStopped {
		t.Fatalf("offline fault unexpectedly used SQL fallback: %+v %v", controls, err)
	}
	blocked := qualificationRequest(t, f, router, "/api/qualification-gaps", "old-healthy-marker-is-insufficient", "", f.Body(t, "/api/qualification-gaps"))
	if blocked.Code == 200 || len(provider.Calls) != 0 {
		t.Fatalf("protected path trusted old readable marker: %d %v", blocked.Code, provider.Calls)
	}
}

func TestPilotOperationsAutomaticAndInspectedDraftExhaustionUseTheSameAuthority(t *testing.T) {
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	f, h, _, router := qualificationConfigFixture(t, aicontrol.ManagedDraftSyntheticConfig("qualification", gate, managedDraftFullBound))
	ctx := context.Background()
	drafts, err := draftrecovery.OpenManagedTerminal(ctx, f.DSN, draftrecovery.Config{
		Actions: h, BindingKey: []byte("synthetic-managed-draft-binding-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer drafts.Close()
	if err = drafts.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Seed five already delivered synthetic Drafts in the authoritative
	// allowance. The fixture ledger's legacy grant counter has no matching rows.
	if _, err := f.DB.Exec(`WITH seeded AS (
	  INSERT INTO ai_fixture_action(namespace,id,owner,creation_key,workflow,binding,execution_binding,execution_version,state,customer_effect,grant_version)
	  SELECT 'qualification','prior-draft-'||n,$1,'prior-draft-'||n,'application_draft',decode(repeat('00',32),'hex'),
	  decode(repeat('00',32),'hex'),$2,'delivered',true,1 FROM generate_series(1,5) n RETURNING namespace,id,owner
	) INSERT INTO careeros_draft_allowance(namespace,action_id,owner,grant_activated_at,grant_expires_at,admitted_at,deadline,state,confirmed_at)
	  SELECT s.namespace,s.id,s.owner,p.activated_at,p.expires_at,clock_timestamp()-interval '5 minutes',p.expires_at,'delivered',clock_timestamp()
	  FROM seeded s JOIN careeros_pilot_access p ON p.account_id=s.owner`, f.Principal.AccountID, aicontrol.ManagedDraftPolicyVersion); err != nil {
		t.Fatal(err)
	}
	_, ops, notices, _ := pilotOperationsFixture(t, f, h, time.Now)
	if err := ops.Inspect(ctx); err != nil {
		t.Fatal(err)
	}
	initial, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || initial.Figures == nil || initial.Figures.ExhaustedDraftGrants != 1 {
		t.Fatalf("inspected allowance: %+v %v", initial, err)
	}
	if err = h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
		w := httptest.NewRecorder()
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.WriteString(`{"error":"synthetic_rate_limit"}`)
		return w.Result(), false, nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = qualificationRequest(t, f, router, "/api/qualification-gaps", "snapshot-after-draft-exhaustion", "", f.Body(t, "/api/qualification-gaps"))
	current, err := ops.OwnerStatus(ctx, f.Principal)
	if err != nil || current.Figures == nil || !current.Figures.At.After(initial.Figures.At) || current.Figures.ExhaustedDraftGrants != 1 ||
		len(*notices) == 0 || (*notices)[0].Kind != "unknown_dispatch" {
		t.Fatalf("automatic observer replaced the authoritative exhausted grant: %+v %v %+v", current, err, *notices)
	}
}
