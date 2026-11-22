package accountstore

import (
	"context"
	"database/sql"

	"professional-information-repo/internal/auth"
)

func stopCompromisedAccount(ctx context.Context, tx *sql.Tx, actor auth.Principal, owner string) error {
	var already bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_control_accounts WHERE account_id=$1 AND compromised)`, owner).Scan(&already); err != nil {
		return unavailable(err)
	}
	if already {
		return nil
	}
	var generation uint64
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT generation,status FROM careeros_accounts WHERE id=$1 FOR UPDATE`, owner).Scan(&generation, &state); err != nil {
		return unavailable(err)
	}
	if state != "active" || generation == ^uint64(0) {
		return auth.ErrForbidden
	}
	var priorState sql.NullString
	var priorHeld sql.NullBool
	var grantVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT state,retrieval_held,authority_version FROM careeros_pilot_access WHERE account_id=$1 FOR UPDATE`, owner).Scan(&priorState, &priorHeld, &grantVersion)
	if err != nil && err != sql.ErrNoRows {
		return unavailable(err)
	}
	if err = closeUnconfirmedForControl(ctx, tx, owner); err != nil {
		return err
	}
	// All purpose/shared/allowance waits precede the final exact actor check.
	if err = controlActor(ctx, tx, actor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_control_accounts(account_id,compromised,minimum_generation,prior_grant_state,prior_retrieval_held,held_grant_version) VALUES($1,true,$2,$3,$4,$5) ON CONFLICT(account_id) DO UPDATE SET compromised=true,minimum_generation=GREATEST(careeros_control_accounts.minimum_generation,EXCLUDED.minimum_generation),stopped_at=clock_timestamp(),prior_grant_state=EXCLUDED.prior_grant_state,prior_retrieval_held=EXCLUDED.prior_retrieval_held,held_grant_version=EXCLUDED.held_grant_version`, owner, generation+1, priorState, priorHeld, func() any {
		if grantVersion.Valid {
			return grantVersion.Int64 + 1
		}
		return nil
	}()); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_pilot_access SET state=CASE WHEN activated_at IS NOT NULL AND state<>'revoked' THEN 'suspended' ELSE state END,control_compromise_held=true,authority_version=authority_version+1 WHERE account_id=$1 AND authority_version<9223372036854775807`, owner); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_accounts SET generation=generation+1 WHERE id=$1`, owner); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_sessions WHERE account_id=$1`, owner); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_deletion_status_sessions WHERE account_id=$1`, owner); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_oidc_transactions WHERE account_id=$1`, owner); err != nil {
		return unavailable(err)
	}
	return nil
}

func closeUnconfirmedForControl(ctx context.Context, tx *sql.Tx, owner string) error {
	present, err := installed(ctx, tx, "ai_fixture_action")
	if err != nil || !present {
		return err
	}
	purpose, err := installed(ctx, tx, "careeros_draft_actions")
	if err != nil {
		return err
	}
	if purpose {
		rows, e := tx.QueryContext(ctx, `SELECT d.namespace,d.action_id FROM careeros_draft_actions d JOIN ai_fixture_action a ON a.namespace=d.namespace AND a.id=d.action_id WHERE d.owner=$1 AND NOT a.customer_effect ORDER BY d.namespace,d.action_id FOR UPDATE OF d`, owner)
		if e != nil {
			return unavailable(e)
		}
		type ref struct{ namespace, id string }
		var refs []ref
		for rows.Next() {
			var r ref
			if e = rows.Scan(&r.namespace, &r.id); e != nil {
				break
			}
			refs = append(refs, r)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return unavailable(e)
		}
		for _, r := range refs {
			if _, e = tx.ExecContext(ctx, `SELECT id FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, r.namespace, r.id); e != nil {
				return unavailable(e)
			}
			allowance, e := installed(ctx, tx, "careeros_draft_allowance")
			if e != nil {
				return e
			}
			if allowance {
				if _, e = tx.ExecContext(ctx, `UPDATE careeros_draft_allowance SET state='released',released_at=clock_timestamp() WHERE namespace=$1 AND action_id=$2 AND state='reserved'`, r.namespace, r.id); e != nil {
					return unavailable(e)
				}
			}
			if _, e = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET state=CASE WHEN state='deleted' THEN state ELSE 'closed' END,cleanup_queued=true WHERE namespace=$1 AND action_id=$2`, r.namespace, r.id); e != nil {
				return unavailable(e)
			}
			if _, e = tx.ExecContext(ctx, `UPDATE careeros_draft_admissions SET state=CASE WHEN state='deleted' THEN state ELSE 'closed' END WHERE namespace=$1 AND owner=$2 AND creation_key=(SELECT creation_key FROM ai_fixture_action WHERE namespace=$1 AND id=$3)`, r.namespace, owner, r.id); e != nil {
				return unavailable(e)
			}
		}
	}
	// Local callback leases and every provider allocation/observation survive.
	_, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state=CASE WHEN state IN ('deleted','closed','canceled') THEN state ELSE 'canceled' END,customer_reserved=false WHERE owner=$1 AND NOT customer_effect`, owner)
	return unavailable(err)
}

func reopenCompromisedAccount(ctx context.Context, tx *sql.Tx, owner string) error {
	var minimum uint64
	if err := tx.QueryRowContext(ctx, `SELECT minimum_generation FROM careeros_control_accounts WHERE account_id=$1 AND compromised FOR UPDATE`, owner).Scan(&minimum); err != nil {
		return ErrConflict
	}
	// Resume only this control's unchanged suspension. Independent suspension,
	// explicit revocation/closure and the original clocks are never overwritten.
	if _, err := tx.ExecContext(ctx, `UPDATE careeros_pilot_access p SET state=CASE WHEN p.state='suspended' AND p.control_compromise_held AND p.authority_version=c.held_grant_version AND a.status='active' THEN c.prior_grant_state ELSE p.state END,control_compromise_held=false,authority_version=authority_version+1 FROM careeros_control_accounts c,careeros_accounts a WHERE p.account_id=$1 AND c.account_id=p.account_id AND a.id=p.account_id AND p.authority_version<9223372036854775807`, owner); err != nil {
		return unavailable(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE careeros_accounts SET generation=GREATEST(generation,$2) WHERE id=$1`, owner, minimum); err != nil {
		return unavailable(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM careeros_sessions WHERE account_id=$1`, owner); err != nil {
		return unavailable(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM careeros_deletion_status_sessions WHERE account_id=$1`, owner); err != nil {
		return unavailable(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE careeros_control_accounts SET compromised=false WHERE account_id=$1`, owner); err != nil {
		return unavailable(err)
	}
	return nil
}
