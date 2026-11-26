package pilotruntime

import (
	"context"
	"database/sql"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/hosteddraft"
)

// MigrateHostedDraft installs the three additive pieces under the same closed
// fence and transaction. It never inserts the owner-approved test scope.
func MigrateHostedDraft(ctx context.Context, dsn string, actionKeyDigest, artifactKeyDigest [32]byte, expires time.Time) error {
	return accountstore.RunHostedDraftMigration(ctx, dsn, func(ctx context.Context, tx *sql.Tx) error {
		if err := hosteddraft.MigrateScope(ctx, tx); err != nil {
			return err
		}
		cfg := aicontrol.HostedDraftMigrationConfig{BindingKeyDigest: actionKeyDigest, ExpiresAt: expires}
		if err := aicontrol.MigrateHostedDraft(ctx, tx, cfg); err != nil {
			return err
		}
		if err := draftrecovery.MigrateHostedDraft(ctx, tx, artifactKeyDigest); err != nil {
			return err
		}
		return hosteddraft.MigrateAttestation(ctx, tx)
	})
}
