package accountapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

const (
	archivePruningID  = "restore-archive-excerpt"
	laterPruningID    = "restore-later-excerpt"
	archiveDeletionID = "restore-already-deleted-profile"
)

func revokeRestoreGrant(t *testing.T, fixture, who *aiadmission.Fixture, subject string) accountstore.PilotStatus {
	t.Helper()
	ctx := context.Background()
	identity := auth.IdentityRef{Provider: "google", Issuer: "https://accounts.google.com", Subject: subject}
	if _, err := fixture.Store.PilotCommand(ctx, fixture.Principal, subject+"-revoke", accountstore.PilotCommand{Kind: "revoke", Identity: identity}); err != nil {
		t.Fatal(err)
	}
	status, err := fixture.Store.PilotStatus(ctx, who.Principal)
	if err != nil || status.State != "revoked" || status.CanActivate || status.CanDraft || status.CanStandalone ||
		status.ExpiresAt == nil || status.RetrieveUntil == nil {
		t.Fatal("source revocation must deny new rights and retain original access clocks", err)
	}
	return status
}

func deleteRestoreProfile(t *testing.T, owner *aiadmission.Fixture) accountstore.DeletionStatus {
	t.Helper()
	ctx := context.Background()
	var document profiledocument.Document
	if err := json.Unmarshal(owner.Document, &document); err != nil {
		t.Fatal(err)
	}
	in := accountstore.DeletionRequest{
		OperationID: archiveDeletionID, Kind: "profile", ProfileID: document.ID, Revision: document.Revision, Confirm: true,
	}
	if _, err := owner.Store.StartDeletion(ctx, owner.Principal, in); err != nil {
		t.Fatal(err)
	}
	removed, err := owner.Store.PurgeDeletion(ctx, owner.Principal.AccountID, archiveDeletionID)
	if err != nil || removed.State != "active_content_removed" || removed.RemovedAt == nil || removed.RecoveryCopyDeadline == nil {
		t.Fatal("archive fixture must contain a completed Profile deletion", err)
	}
	return removed
}

type restoreArchiveFacts struct {
	prunedOwner, deletedOwner, draftOwner                 string
	prunedDocument, profileID, grant, deletion, delivery  string
	grantExpiry, accessEnded, prunedAt, pruningDeadline   time.Time
	deletionFence, deletionRemoved, releaseAt, admittedAt time.Time
	previousRevision, newRevision                         int64
	deletedContent, closedContent, effect                 bool
}

func observeRestoreArchiveFacts(t *testing.T, db *sql.DB, pruned, deleted, draft string) restoreArchiveFacts {
	t.Helper()
	f := restoreArchiveFacts{prunedOwner: pruned, deletedOwner: deleted, draftOwner: draft}
	err := db.QueryRow(`SELECT convert_from(p.document,'UTF8'),f.previous_revision,f.new_revision,f.removed_at,
		f.recovery_copy_deadline,a.state,a.expires_at,a.access_ended_at
		FROM careeros_profile_pruning f JOIN careeros_profiles p USING(account_id,profile_id)
		JOIN careeros_pilot_access a USING(account_id)
		WHERE f.account_id=$1 AND f.evidence_id=$2 AND p.revision=f.new_revision`, pruned, archivePruningID).
		Scan(&f.prunedDocument, &f.previousRevision, &f.newRevision, &f.prunedAt, &f.pruningDeadline, &f.grant, &f.grantExpiry, &f.accessEnded)
	if err != nil {
		t.Fatal("archive grant/pruning witness", err)
	}
	err = db.QueryRow(`SELECT convert_from(m.profile_id,'UTF8'),d.state,d.fenced_at,d.removed_at,
		EXISTS(SELECT 1 FROM careeros_profiles WHERE account_id=d.account_id)
		FROM careeros_deletion_operations d JOIN careeros_deleted_profiles m USING(account_id,operation_id)
		WHERE d.account_id=$1 AND d.operation_id=$2 AND d.kind='profile'
		AND d.profile_id=m.profile_id AND d.fenced_at=m.fenced_at`, deleted, archiveDeletionID).
		Scan(&f.profileID, &f.deletion, &f.deletionFence, &f.deletionRemoved, &f.deletedContent)
	if err != nil {
		t.Fatal("archive deletion witness", err)
	}
	err = db.QueryRow(`SELECT o.state,o.admitted_at,o.released_at,a.customer_effect,
		EXISTS(SELECT 1 FROM careeros_draft_content c WHERE c.namespace=d.namespace AND c.action_id=d.action_id)
		FROM careeros_draft_actions d JOIN careeros_draft_allowance o USING(namespace,action_id)
		JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action_id
		WHERE d.owner=$1 AND d.creation_key=$2 AND d.state='closed' AND o.confirmed_at IS NULL`, draft, archiveClosedDraft).
		Scan(&f.delivery, &f.admittedAt, &f.releaseAt, &f.effect, &f.closedContent)
	if err != nil {
		t.Fatal("archive closed delivery witness", err)
	}
	return f
}

func (f restoreArchiveFacts) checkOrigins(t *testing.T, grant accountstore.PilotStatus, deletion accountstore.DeletionStatus) {
	t.Helper()
	doc, err := profiledocument.Decode([]byte(f.prunedDocument))
	if err != nil || doc.Revision != 2 || f.previousRevision != 1 || f.newRevision != 2 || len(doc.Evidence) != 1 ||
		doc.Evidence[0].ID == archivePruningID || f.prunedAt.IsZero() || !f.pruningDeadline.Equal(f.prunedAt.Add(720*time.Hour)) {
		t.Fatal("archive must already contain the cleaned Profile and original detail deadline", err)
	}
	if f.grant != "revoked" || !f.grantExpiry.Equal(*grant.ExpiresAt) ||
		!f.accessEnded.Add(720*time.Hour).Equal(*grant.RetrieveUntil) {
		t.Fatal("archive revocation must match the source authorization and its original clocks")
	}
	if f.deletion != "active_content_removed" || f.deletedContent || f.profileID != doc.ID ||
		!f.deletionFence.Equal(deletion.FencedAt) || !f.deletionRemoved.Equal(*deletion.RemovedAt) ||
		!f.deletionFence.Add(720*time.Hour).Equal(*deletion.RecoveryCopyDeadline) {
		t.Fatal("archive must already contain the deletion marker and original copy deadline")
	}
	if f.delivery != "released" || f.effect || f.closedContent || !f.releaseAt.After(f.admittedAt) {
		t.Fatal("archive delivery must already be closed, cleaned and released at the original time")
	}
}

func (f restoreArchiveFacts) checkRetained(t *testing.T, db *sql.DB, stage string) {
	t.Helper()
	if got := observeRestoreArchiveFacts(t, db, f.prunedOwner, f.deletedOwner, f.draftOwner); got != f {
		t.Fatalf("%s rewrote or dropped an original archive witness", stage)
	}
	t.Logf("restore observation %s: revoked grant, cleaned Profile detail, deleted Profile and closed delivery retained with original clocks", stage)
}

func restorePruningFence(t *testing.T, snapshot accountstore.ProfilePruningSnapshot, owner, evidenceID string) accountstore.ProfilePruningFence {
	t.Helper()
	for _, fence := range snapshot.Fences {
		if fence.AccountID == owner && fence.EvidenceID == evidenceID {
			return fence
		}
	}
	t.Fatalf("missing independent pruning fence %s", evidenceID)
	return accountstore.ProfilePruningFence{}
}
