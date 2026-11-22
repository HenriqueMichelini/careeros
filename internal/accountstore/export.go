package accountstore

import (
	"bytes"
	"context"

	"professional-information-repo/internal/auth"
)

// WithProfileExport releases only the retained Profile/provenance bytes. The
// callback writes the actual outer response while account, session and pilot
// mutation locks remain held. It must not retain bytes or call back into Store.
// No export content or new retention class is persisted.
func (s *Store) WithProfileExport(ctx context.Context, p auth.Principal, write func([]byte) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if s.pilot {
		if err = lockPilot(ctx, tx); err != nil {
			return err
		}
	}
	if err = authorized(ctx, tx, p); err != nil {
		return err
	}
	if s.pilot {
		if err = requirePilot(ctx, tx, p.AccountID, PilotRetrieve); err != nil {
			return err
		}
	}
	result, err := currentProfile(ctx, tx, p.AccountID)
	if err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(result.Document), []byte("null")) {
		return ErrNotFound
	}
	raw := append([]byte(`{"format":"careeros-profile-export","version":1,"profile":`), result.Document...)
	raw = append(raw, '}')
	// Reading/encoding may wait or take time. Check trusted time after that work.
	if err = authorized(ctx, tx, p); err != nil {
		return err
	}
	if s.pilot {
		if err = requirePilot(ctx, tx, p.AccountID, PilotRetrieve); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return unavailable(err)
	}
	if err = write(raw); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}
