package accountstore

import (
	"context"
	"database/sql"

	"professional-information-repo/internal/auth"
)

// AuthorizeTransaction composes a resource write with the existing account and
// exact-session authority in the caller's transaction. Call it again after any
// later blocking locks, before the final write. It does not commit the transaction.
// No caller-supplied owner or public Principal fields substitute for MatchesSession.
func AuthorizeTransaction(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	return authorized(ctx, tx, p)
}
