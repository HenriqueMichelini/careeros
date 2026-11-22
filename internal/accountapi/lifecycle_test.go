package accountapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
)

func TestLifecycleExpiredPilotRemovesContentWithoutClosingAccount(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "lifecycle-expired")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	h := NewPilot(svc, store, nil)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	other := login(t, svc, "lifecycle-independent")
	if w := api(t, New(svc, store), other, "PUT", "/api/profile/current", command("other-save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '45 days',expires_at=clock_timestamp()-interval '31 days' WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	var emailKeys []string
	worker, err := accountstore.NewLifecycleWorker(store, func(_ context.Context, n accountstore.LifecycleEmailNotice) (accountstore.LifecycleEmailResult, error) {
		emailKeys = append(emailKeys, n.Key)
		return accountstore.LifecycleEmailResult{State: "sent"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	w := api(t, h, who, "GET", "/api/lifecycle/status", nil)
	if w.Code != 200 {
		t.Fatal("metadata status", w.Code, w.Body.String())
	}
	var status accountstore.LifecycleStatus
	if err = json.Unmarshal(w.Body.Bytes(), &status); err != nil || len(status.Notices) != 2 || len(status.Cleanup) != 1 || status.Cleanup[0].State != "active_content_removed" {
		t.Fatal("lifecycle metadata", w.Body.String(), err)
	}
	if len(emailKeys) != 2 || emailKeys[0] == emailKeys[1] {
		t.Fatal("two independently bound notices", emailKeys)
	}
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(emailKeys) != 2 {
		t.Fatal("notice resend", emailKeys)
	}
	w = api(t, New(svc, store), who, "GET", "/api/profile/current", nil)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"document":null`)) {
		t.Fatal("expired primary content survived", w.Code, w.Body.String())
	}
	if w = api(t, New(svc, store), other, "GET", "/api/profile/current", nil); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("ação")) {
		t.Fatal("independent account changed", w.Code, w.Body.String())
	}
	var accountState string
	if err = db.QueryRow(`SELECT status FROM careeros_accounts WHERE id=$1`, who.Account.ID).Scan(&accountState); err != nil || accountState != "active" {
		t.Fatal("expiry closed sign-in account", accountState, err)
	}
}

func TestLifecycleNoticesAreReadableWithoutRunningJobsAndFailureNeverExtends(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "notice-failure")
	store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '14 days 1 second',expires_at=clock_timestamp()-interval '1 second' WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	h := NewPilot(svc, store, nil)
	read := func() accountstore.LifecycleStatus {
		t.Helper()
		w := api(t, h, who, "GET", "/api/lifecycle/status", nil)
		var v accountstore.LifecycleStatus
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return v
	}
	before := read()
	if len(before.Notices) != 1 || before.Notices[0].EmailState != "pending" {
		t.Fatal(before)
	}
	var queued int
	db.QueryRow(`SELECT count(*) FROM careeros_lifecycle_notices`).Scan(&queued)
	if queued != 0 {
		t.Fatal("status mutated email queue", queued)
	}
	calls := 0
	worker, _ := accountstore.NewLifecycleWorker(store, func(context.Context, accountstore.LifecycleEmailNotice) (accountstore.LifecycleEmailResult, error) {
		calls++
		return accountstore.LifecycleEmailResult{State: "failed"}, nil
	})
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	after := read()
	if calls != 1 || after.Notices[0].EmailState != "failed" || !after.Notices[0].OperatorSignal || !after.Notices[0].RemovalAt.Equal(before.Notices[0].RemovalAt) {
		t.Fatal("failed notice changed clock or lost signal", calls, after)
	}
	if _, err = worker.Run(context.Background(), 10); err != nil || calls != 1 {
		t.Fatal("retry before bounded eligibility", calls, err)
	}
	for i := 0; i < 4; i++ {
		if _, err = db.Exec(`UPDATE careeros_lifecycle_notices SET next_attempt_at=clock_timestamp()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		if _, err = worker.Run(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
	}
	after = read()
	if calls != 3 || after.Notices[0].Attempts != 3 || !after.Notices[0].RemovalAt.Equal(before.Notices[0].RemovalAt) {
		t.Fatal("unbounded retry or retention extension", calls, after)
	}
	other := login(t, svc, "foreign-notice-owner")
	w := api(t, h, other, "GET", "/api/lifecycle/status", nil)
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(before.Notices[0].ID)) {
		t.Fatal("foreign notice leaked", w.Code, w.Body.String())
	}
}

func TestLifecycleUnknownEmailDoesNotRetryOrDelayRemoval(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "notice-unknown")
	store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true)
	if w := api(t, NewPilot(svc, store, nil), who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '44 days'+interval '1 second',expires_at=clock_timestamp()-interval '30 days'+interval '1 second' WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	worker, _ := accountstore.NewLifecycleWorker(store, func(context.Context, accountstore.LifecycleEmailNotice) (accountstore.LifecycleEmailResult, error) {
		calls++
		return accountstore.LifecycleEmailResult{}, errors.New("synthetic unknown handoff")
	})
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("both notices due", calls)
	}
	if _, err = db.Exec(`SELECT pg_sleep(1.1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("unknown outcome resent", calls)
	}
	w := api(t, NewPilot(svc, store, nil), who, "GET", "/api/lifecycle/status", nil)
	var v accountstore.LifecycleStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || len(v.Cleanup) != 1 || v.Cleanup[0].State != "active_content_removed" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, n := range v.Notices {
		if n.EmailState != "unknown" || !n.OperatorSignal {
			t.Fatal("unknown not visible/escalated", n)
		}
	}
	if bytes.Contains(w.Body.Bytes(), []byte("synthetic unknown handoff")) || bytes.Contains(w.Body.Bytes(), []byte("@")) {
		t.Fatal("recipient or driver payload in metadata", w.Body.String())
	}
}

func TestLifecycleNeverActivatedInvitationCleanupRequiresFreshClearance(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, old := pilotInvite(t, store, svc, operator, "unused-invitation")
	h := New(svc, store)
	if w := api(t, h, who, "PUT", "/api/profile/current", command("independent-save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE careeros_pilot_access SET invited_at=clock_timestamp()-interval '8 days',invitation_expires_at=clock_timestamp()-interval '1 day' WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	worker, _ := accountstore.NewLifecycleWorker(store, nil)
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	var cleared, payloadGone, noGrant bool
	if err = db.QueryRow(`SELECT age_eligible IS NULL AND eligibility_checked_at IS NULL AND eligibility_checked_by IS NULL,provider IS NULL AND issuer IS NULL AND subject IS NULL,activated_at IS NULL AND expires_at IS NULL AND participant_slot IS NULL FROM careeros_pilot_access WHERE account_id=$1`, who.Account.ID).Scan(&cleared, &payloadGone, &noGrant); err != nil || !cleared || !payloadGone || !noGrant {
		t.Fatal("unused invitation payload/clearance survived or grant started", cleared, payloadGone, noGrant, err)
	}
	if w := api(t, h, who, "GET", "/api/profile/current", nil); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("ação")) {
		t.Fatal("independent account content removed", w.Code, w.Body.String())
	}
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: "unused-invitation"}
	next, err := store.PilotCommand(context.Background(), operator.Principal, "fresh-reissue", accountstore.PilotCommand{Kind: "invite", Identity: identity})
	if err != nil || next.InvitationID == old.InvitationID || next.CanActivate {
		t.Fatal("reissue inherited clearance", next, err)
	}
	recovery, _ := accountstore.NewDeletionRecovery(store, []byte("synthetic-independent-invitation-deny-key"))
	retained, err := recovery.ExportLifecycle(context.Background())
	if err != nil || len(retained.Invitations) != 1 || retained.Invitations[0].InvitationID != old.InvitationID {
		t.Fatal("reissue lost original invitation denial", retained, err)
	}
	if _, err = store.ActivatePilot(context.Background(), who.Principal, "unproved-activation", next.InvitationID, true); err != auth.ErrForbidden {
		t.Fatal("activation without fresh clearance", err)
	}
	if _, err = store.PilotCommand(context.Background(), operator.Principal, "fresh-clear", accountstore.PilotCommand{Kind: "clear", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ActivatePilot(context.Background(), who.Principal, "fresh-activation", next.InvitationID, true); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleRemovesOrphanExcerptWithoutLosingCurrentFacts(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "orphan-cleanup")
	if _, err := store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true); err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(fixture(t, 1), &doc); err != nil {
		t.Fatal(err)
	}
	var id string
	json.Unmarshal(doc["id"], &id)
	var evidence, links []json.RawMessage
	json.Unmarshal(doc["evidence"], &evidence)
	json.Unmarshal(doc["links"], &links)
	retainedEvidence := len(evidence)
	evidence = append(evidence, json.RawMessage(`{"id":"orphan-excerpt","revision":1,"excerpt":"DELETED DETAIL","origin":"submitted","approval":"approved"}`))
	link, _ := json.Marshal(map[string]any{"id": "orphan-link", "kind": "supports", "from": map[string]any{"profileId": id, "id": "deleted-fact", "revision": 1}, "to": map[string]any{"profileId": id, "id": "orphan-excerpt", "revision": 1}, "state": "invalidated"})
	links = append(links, link)
	doc["evidence"], _ = json.Marshal(evidence)
	doc["links"], _ = json.Marshal(links)
	raw, _ := json.Marshal(doc)
	if _, err := profiledocument.Decode(raw); err != nil {
		t.Fatal("older synthetic fixture invalid", err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO careeros_profiles(account_id,profile_id,revision,document) VALUES($1,$2,1,$3)`, who.Account.ID, []byte(id), raw); err != nil {
		t.Fatal(err)
	}
	worker, _ := accountstore.NewLifecycleWorker(store, nil)
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	w := api(t, NewPilot(svc, store, nil), who, "GET", "/api/profile/current", nil)
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("DELETED DETAIL")) || !bytes.Contains(w.Body.Bytes(), []byte("ação")) {
		t.Fatal("orphan payload remains or current fact lost", w.Code, w.Body.String())
	}
	var got struct {
		Document struct {
			Revision int64 `json:"revision"`
			Evidence []any `json:"evidence"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Document.Revision != 2 || len(got.Document.Evidence) != retainedEvidence {
		t.Fatal("canonical cleanup revision", w.Body.String())
	}
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	w = api(t, NewPilot(svc, store, nil), who, "GET", "/api/profile/current", nil)
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Document.Revision != 2 {
		t.Fatal("cleanup re-aged unchanged Profile", w.Body.String())
	}
}

func TestLifecyclePruningHandoffDeniesOlderProfileWithoutBlockingReplacement(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "pruning-handoff")
	store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true)
	var doc map[string]json.RawMessage
	json.Unmarshal(fixture(t, 1), &doc)
	var id string
	json.Unmarshal(doc["id"], &id)
	var evidence []json.RawMessage
	json.Unmarshal(doc["evidence"], &evidence)
	evidence = append(evidence, json.RawMessage(`{"id":"orphan","revision":1,"excerpt":"DELETED PAYLOAD","origin":"synthetic","approval":"approved"}`))
	doc["evidence"], _ = json.Marshal(evidence)
	old, _ := json.Marshal(doc)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO careeros_profiles VALUES($1,$2,1,$3)`, who.Account.ID, []byte(id), old); err != nil {
		t.Fatal(err)
	}
	worker, _ := accountstore.NewLifecycleWorker(store, nil)
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	recovery, _ := accountstore.NewDeletionRecovery(store, []byte("synthetic-independent-pruning-handoff-key"))
	snapshot, err := recovery.ExportProfilePruning(context.Background())
	if err != nil || len(snapshot.Fences) != 1 {
		t.Fatal(snapshot, err)
	}
	encoded, _ := json.Marshal(snapshot)
	if bytes.Contains(encoded, []byte("DELETED PAYLOAD")) || bytes.Contains(encoded, []byte("ação")) {
		t.Fatal("pruning handoff contains career bytes")
	}
	if _, err = db.Exec(`UPDATE careeros_profiles SET revision=1,document=$2 WHERE account_id=$1`, who.Account.ID, old); err != nil {
		t.Fatal(err)
	}
	h := NewPilot(svc, store, nil)
	if w := api(t, h, who, "GET", "/api/profile/current", nil); w.Code != 403 {
		t.Fatal("older pruned Profile readable", w.Code, w.Body.String())
	}
	w := api(t, h, who, "GET", "/api/lifecycle/status", nil)
	var status accountstore.LifecycleStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.CareerReadAllowed || status.ContentExpired {
		t.Fatal("metadata confused career denial with session/expiry", w.Code, w.Body.String())
	}
	if _, err = db.Exec(`DELETE FROM careeros_profiles WHERE account_id=$1`, who.Account.ID); err != nil {
		t.Fatal(err)
	}
	next := bytes.ReplaceAll(fixture(t, 1), []byte(`"`+id+`"`), []byte(`"replacement-profile-b"`))
	if w = api(t, h, who, "PUT", "/api/profile/current", command("replacement", 0, next)); w.Code != 200 {
		t.Fatal("independent replacement denied", w.Code, w.Body.String())
	}
	if err = recovery.ApplyProfilePruning(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if w = api(t, h, who, "GET", "/api/profile/current", nil); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("replacement-profile-b")) {
		t.Fatal("old pruning fence affected replacement", w.Code, w.Body.String())
	}
	again, err := recovery.ExportProfilePruning(context.Background())
	if err != nil || !again.Fences[0].RemovedAt.Equal(snapshot.Fences[0].RemovedAt) || !again.Fences[0].RecoveryCopyDeadline.Equal(snapshot.Fences[0].RecoveryCopyDeadline) {
		t.Fatal("pruning restore reset clocks", again, err)
	}
}

func TestLifecycleResidualEvidenceAndNoticeRecoveryKeepOriginalTruth(t *testing.T) {
	dsn, store, svc, operator := pilotFixture(t)
	who, invite := pilotInvite(t, store, svc, operator, "residual-handoff")
	store.ActivatePilot(context.Background(), who.Principal, "activate", invite.InvitationID, true)
	if w := api(t, NewPilot(svc, store, nil), who, "PUT", "/api/profile/current", command("save", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`UPDATE careeros_pilot_access SET activated_at=clock_timestamp()-interval '45 days',expires_at=clock_timestamp()-interval '31 days' WHERE account_id=$1`, who.Account.ID)
	emails := 0
	stage := 1
	worker, err := accountstore.NewLifecycleWorker(store, func(context.Context, accountstore.LifecycleEmailNotice) (accountstore.LifecycleEmailResult, error) {
		emails++
		return accountstore.LifecycleEmailResult{}, errors.New("synthetic unknown")
	}, func(_ context.Context, o accountstore.LifecycleResidualObligation) (accountstore.LifecycleResidualEvidence, error) {
		state := "pending"
		id := "pending-"
		if stage == 2 {
			state = "verified_unrecoverable"
			id = "verified-"
		}
		return accountstore.LifecycleResidualEvidence{ID: id + o.Class, Revision: int64(stage), State: state}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	recovery, _ := accountstore.NewDeletionRecovery(store, []byte("synthetic-independent-lifecycle-recovery-key"))
	snapshot, err := recovery.ExportLifecycle(context.Background())
	if err != nil || len(snapshot.Notices) != 2 || len(snapshot.Residuals) != 2 {
		t.Fatal(snapshot, err)
	}
	encoded, _ := json.Marshal(snapshot)
	if bytes.Contains(encoded, []byte("ação")) || bytes.Contains(encoded, []byte("@")) {
		t.Fatal("lifecycle handoff leaked career/recipient")
	}
	if _, err = db.Exec(`UPDATE careeros_lifecycle_notices SET email_state='pending',attempts=0,operator_signal=false;DELETE FROM careeros_lifecycle_residual_evidence;DELETE FROM careeros_lifecycle_residuals`); err != nil {
		t.Fatal(err)
	}
	if err = recovery.ApplyLifecycle(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	stage = 2
	if _, err = worker.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if emails != 2 {
		t.Fatal("restore reset unknown notice intent", emails)
	}
	w := api(t, NewPilot(svc, store, nil), who, "GET", "/api/lifecycle/status", nil)
	var status accountstore.LifecycleStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, r := range status.Residuals {
		if r.State != "verified_unrecoverable" {
			t.Fatal("controlled evidence not recorded", r)
		}
	}
	again, err := recovery.ExportLifecycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range again.Residuals {
		if !r.OriginalFencedAt.Equal(snapshot.Residuals[i].OriginalFencedAt) || (r.Deadline != nil && !r.Deadline.Equal(*snapshot.Residuals[i].Deadline)) {
			t.Fatal("residual clock reset", again)
		}
	}
}
