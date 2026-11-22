package accountapi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"professional-information-repo/internal/accountstore"
	"testing"
	"time"
)

func TestDeletionLocalCleanupCommandRunsOnlyAlreadyFencedOperation(t *testing.T) {
	dsn, _ := database(t)
	store := open(t, dsn, "deletion_cli")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	who := login(t, svc, "cli-deletion")
	raw := fixture(t, 1)
	if _, err := store.SaveProfile(context.Background(), who.Principal, "save", 0, raw); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	json.Unmarshal(raw, &doc)
	if _, err := store.StartDeletion(context.Background(), who.Principal, accountstore.DeletionRequest{OperationID: "cli-purge", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "rtk", "proxy", "go", "run", "./cmd/deletion-local", "--synthetic-only", "--account-id", who.Account.ID, "--operation-id", "cli-purge")
	command.Dir = "../.."
	command.Env = append(os.Environ(), "CAREEROS_DELETION_LOCAL_DATABASE_URL="+dsn)
	out, err := command.CombinedOutput()
	var result accountstore.DeletionStatus
	if err != nil || json.Unmarshal(out, &result) != nil || result.State != "active_content_removed" {
		t.Fatalf("local code-owned cleanup failed: %v %s", err, out)
	}
	if result.RecoveryCopies != "not_verified" {
		t.Fatal("CLI invented residual disposal", result)
	}
}
