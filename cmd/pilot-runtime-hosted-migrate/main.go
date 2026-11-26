package main

import (
	"context"
	"encoding/hex"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/pilotruntime"
)

func main() {
	schemaID := flag.Bool("schema-id", false, "print the embedded Account schema identity")
	scopedFakeDraft := flag.Bool("scoped-fake-draft", false, "install the separately prepared fake Draft schema")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected hosted migration arguments")
	}
	if *schemaID {
		_, _ = os.Stdout.WriteString(accountstore.HostedPilotSchemaID() + "\n")
		return
	}
	if err := migrate(*scopedFakeDraft); err != nil {
		log.Fatal("hosted pilot migration refused or failed")
	}
}

func migrate(scopedFakeDraft bool) error {
	process, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(process, 30*time.Second)
	defer cancel()
	dsn := os.Getenv("CAREEROS_HOSTED_MIGRATOR_DATABASE_URL")
	if !scopedFakeDraft {
		return accountstore.MigrateHostedPilot(ctx, dsn)
	}
	action, err := migrationDigest(os.Getenv("CAREEROS_HOSTED_DRAFT_ACTION_KEY_SHA256"))
	if err != nil {
		return err
	}
	artifact, err := migrationDigest(os.Getenv("CAREEROS_HOSTED_DRAFT_ARTIFACT_KEY_SHA256"))
	if err != nil || action == artifact {
		return auth.ErrUnavailable
	}
	rawExpiry := os.Getenv("CAREEROS_HOSTED_DRAFT_EXPIRES_AT")
	expiry, err := time.Parse(time.RFC3339, rawExpiry)
	if err != nil || expiry.Format(time.RFC3339) != rawExpiry || expiry.Location() != time.UTC {
		return auth.ErrUnavailable
	}
	return pilotruntime.MigrateHostedDraft(ctx, dsn, action, artifact, expiry)
}

func migrationDigest(value string) ([32]byte, error) {
	digest, err := hex.DecodeString(value)
	if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != value {
		return [32]byte{}, auth.ErrUnavailable
	}
	return [32]byte(digest), nil
}
