package accountstore

import (
	"context"
	"time"

	"professional-information-repo/internal/auth"
)

type OperationsDiagnostic struct {
	ID      string          `json:"id"`
	Kind    OperationsFault `json:"kind"`
	Build   string          `json:"build"`
	EventAt time.Time       `json:"eventAt"`
}

func (o *Operations) RecordDiagnostic(ctx context.Context, kind OperationsFault) error {
	switch kind {
	case OperationsDatabaseFault, OperationsAuthorityFault, OperationsControlFault, OperationsAuthFault, OperationsProviderFault, OperationsBackupFault:
	default:
		return ErrValidation
	}
	id, err := newID()
	if err != nil {
		return unavailable(err)
	}
	_, err = o.store.db.ExecContext(ctx, `INSERT INTO careeros_operations_diagnostics(id,kind,build,event_at) VALUES($1,$2,$3,$4)`,
		id, kind, o.cfg.Build, o.journal.cfg.Clock().UTC())
	return unavailable(err)
}

func (o *Operations) OwnerDiagnostics(ctx context.Context, p auth.Principal) ([]OperationsDiagnostic, error) {
	tx, err := o.supportTx(ctx, p, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,kind,build,event_at FROM careeros_operations_diagnostics
	  WHERE event_at>$1 ORDER BY event_at,id`, o.journal.cfg.Clock().UTC().Add(-diagnosticTTL))
	if err != nil {
		return nil, unavailable(err)
	}
	results := []OperationsDiagnostic{}
	for rows.Next() {
		var d OperationsDiagnostic
		if err = rows.Scan(&d.ID, &d.Kind, &d.Build, &d.EventAt); err != nil {
			break
		}
		results = append(results, d)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, unavailable(err)
	}
	if err = supportActor(ctx, tx, p, true); err != nil {
		return nil, err
	}
	return results, nil
}
