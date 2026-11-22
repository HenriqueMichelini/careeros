package accountstore

import (
	"context"
	"database/sql"

	"professional-information-repo/internal/auth"
)

// CheckControlDatabase binds a code-installed local verifier to this Store's
// actual database/schema/server. It returns no routing or identity metadata.
func (s *Store) CheckControlDatabase(ctx context.Context, other *sql.DB) error {
	if s == nil || s.db == nil || other == nil {
		return auth.ErrUnavailable
	}
	const query = `SELECT current_database(),current_schema(),COALESCE(inet_server_addr()::text,'unix'),current_setting('port')`
	var own, peer [4]string
	if err := s.db.QueryRowContext(ctx, query).Scan(&own[0], &own[1], &own[2], &own[3]); err != nil {
		return unavailable(err)
	}
	if err := other.QueryRowContext(ctx, query).Scan(&peer[0], &peer[1], &peer[2], &peer[3]); err != nil {
		return unavailable(err)
	}
	if own != peer {
		return auth.ErrForbidden
	}
	return nil
}
