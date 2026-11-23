package accountapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/profiledocument"
)

// This bounded local fixture retains non-content decisions. It does not
// export credentials, sessions, Profiles, Draft bundles, or human review text.
// The two content owners in the scenario already exist in the older backup.
var restoreAuthorityTables = []string{
	"ai_fixture_grant",
	"ai_fixture_scope",
	"ai_fixture_action",
	"ai_fixture_stage",
	"ai_fixture_child",
	"ai_fixture_dispatch",
	"ai_fixture_allocation",
	"ai_fixture_evidence",
	"ai_fixture_observation",
	"ai_recon_contract",
	"ai_recon_command",
	"ai_recon_state",
	"ai_recon_event",
	"ai_recon_stop",
	"ai_recon_action_key",
	"ai_recon_legacy_marker",
	"careeros_draft_admissions",
	"careeros_draft_actions",
	"careeros_draft_allowance",
	"careeros_draft_opening_receipts",
	"careeros_control_state",
	"careeros_control_accounts",
	"careeros_control_commands",
}

type restoreDecisions struct {
	Backup, Catalog string
	Deletion        accountstore.DeletionSnapshot
	Lifecycle       accountstore.LifecycleSnapshot
	Pruning         accountstore.ProfilePruningSnapshot
	Tables          map[string]json.RawMessage
}

func restoreCatalog(t *testing.T, db *sql.DB) string {
	t.Helper()
	var catalog string
	err := db.QueryRow(`SELECT coalesce(jsonb_agg(to_jsonb(columns) ORDER BY table_name,ordinal_position),'[]'::jsonb)::text
	FROM (SELECT table_name,column_name,ordinal_position,data_type,udt_name,is_nullable,column_default
	FROM information_schema.columns WHERE table_schema=current_schema()) columns`).Scan(&catalog)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func captureRestoreDecisions(t *testing.T, db *sql.DB, recovery *accountstore.DeletionRecovery, backup string) restoreDecisions {
	t.Helper()
	ctx := context.Background()
	deletion, err := recovery.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := recovery.ExportLifecycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pruning, err := recovery.ExportProfilePruning(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result := restoreDecisions{Backup: backup, Catalog: restoreCatalog(t, db), Deletion: deletion, Lifecycle: lifecycle,
		Pruning: pruning, Tables: make(map[string]json.RawMessage, len(restoreAuthorityTables))}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, table := range restoreAuthorityTables {
		var raw []byte
		query := fmt.Sprintf(`SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)
		FROM (SELECT * FROM %s LIMIT 129) r HAVING count(*)<=128`, pgx.Identifier{table}.Sanitize())
		if err := tx.QueryRowContext(ctx, query).Scan(&raw); err != nil || len(raw) > 96<<10 {
			t.Fatalf("bounded authority capture for %s: %v", table, err)
		}
		result.Tables[table] = raw
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return result
}

func readRestoreDecisions(path, expected, expectedBackup, expectedCatalog string) (restoreDecisions, error) {
	var result restoreDecisions
	if digest, err := restoreFileDigest(path); err != nil || digest != expected {
		return result, errors.New("missing or mismatched independent decision receipt")
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &result) != nil || result.Backup != expectedBackup ||
		result.Catalog != expectedCatalog || len(result.Tables) != len(restoreAuthorityTables) {
		return result, errors.New("incompatible backup or incomplete decision receipt")
	}
	for _, table := range restoreAuthorityTables {
		if len(result.Tables[table]) == 0 {
			return result, fmt.Errorf("missing required authority class %s", table)
		}
	}
	return result, nil
}

func applyRestoreLedger(ctx context.Context, db *sql.DB, decisions restoreDecisions) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range restoreAuthorityTables {
		if err := restoreDecisionRows(ctx, tx, table, decisions.Tables[table]); err != nil {
			return fmt.Errorf("reconcile %s: %w", table, err)
		}
	}
	return tx.Commit()
}

// Before restoring a readable revision, recompute the fixture's deterministic
// excerpt cleanup using the same canonical document operation as the lifecycle
// worker. The already-applied sealed fence owns the removal date, so writing the
// repaired original revision does not start a new recovery-copy interval.
func applyRestorePrunedProfile(ctx context.Context, db *sql.DB, f accountstore.ProfilePruningFence) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var prior []byte
	err = tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1 AND profile_id=$2 AND revision=$3
		AND EXISTS(SELECT 1 FROM careeros_profile_pruning p WHERE p.account_id=$1 AND p.profile_id=$2 AND p.evidence_id=$4
		AND p.previous_revision=$3 AND p.new_revision=$5 AND p.removed_at=$6 AND p.recovery_copy_deadline=$7) FOR UPDATE`,
		f.AccountID, []byte(f.ProfileID), f.PreviousRevision, f.EvidenceID, f.NewRevision, f.RemovedAt, f.RecoveryCopyDeadline).Scan(&prior)
	if err != nil {
		return err
	}
	repaired, changed, err := profiledocument.CleanupRetainedEvidence(prior)
	if err != nil || !changed {
		return errors.New("older detail could not be repaired under its original fence")
	}
	doc, err := profiledocument.Decode(repaired)
	if err != nil || doc.ID != f.ProfileID || doc.Revision != f.NewRevision {
		return errors.New("repaired Profile does not match the confirmed revision")
	}
	for _, evidence := range doc.Evidence {
		if evidence.ID == f.EvidenceID {
			return errors.New("reconciled Profile would retain the removed excerpt")
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE careeros_profiles SET document=$2,revision=$3 WHERE account_id=$1`, f.AccountID, repaired, f.NewRevision); err != nil {
		return err
	}
	return tx.Commit()
}

// The allowlisted relation must have a primary key. Only originating fixtures
// supply row values. The older image owns the content tables and schema.
func restoreDecisionRows(ctx context.Context, tx *sql.Tx, table string, values json.RawMessage) error {
	rows, err := tx.QueryContext(ctx, `SELECT a.attname, EXISTS(
		SELECT 1 FROM pg_index i WHERE i.indrelid=a.attrelid AND i.indisprimary AND a.attnum=ANY(i.indkey))
		FROM pg_attribute a WHERE attrelid=to_regclass($1) AND attnum>0 AND NOT attisdropped ORDER BY attnum`, table)
	if err != nil {
		return err
	}
	var keys, columns, updates []string
	for rows.Next() {
		var name string
		var key bool
		if err := rows.Scan(&name, &key); err != nil {
			rows.Close()
			return err
		}
		id := pgx.Identifier{name}.Sanitize()
		columns = append(columns, id)
		if key {
			keys = append(keys, id)
		} else {
			updates = append(updates, id+"=EXCLUDED."+id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return errors.New("restore authority table has no stable key")
	}
	relation := pgx.Identifier{table}.Sanitize()
	conflict := "DO NOTHING"
	if len(updates) != 0 {
		conflict = "DO UPDATE SET " + strings.Join(updates, ",") + " WHERE to_jsonb(current_row) <> to_jsonb(EXCLUDED)"
	}
	query := fmt.Sprintf(`INSERT INTO %s AS current_row (%s)
	SELECT %s FROM jsonb_populate_recordset(NULL::%s,$1::jsonb) WHERE true ON CONFLICT (%s) %s`,
		relation, strings.Join(columns, ","), strings.Join(columns, ","), relation, strings.Join(keys, ","), conflict)
	_, err = tx.ExecContext(ctx, query, values)
	return err
}
