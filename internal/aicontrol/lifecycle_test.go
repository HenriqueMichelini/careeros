package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func saveLifecycleProfile(t *testing.T, f *fixture) []byte {
	t.Helper()
	raw, err := os.ReadFile("../profiledocument/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Document json.RawMessage }
	if err = json.Unmarshal(raw, &cases); err != nil || len(cases) == 0 {
		t.Fatal(err)
	}
	if _, err = f.store.SaveProfile(context.Background(), f.p, "lifecycle_profile", 0, cases[0].Document); err != nil {
		t.Fatal(err)
	}
	return cases[0].Document
}

func TestA31_ProfileAndAccountDeletionRecreationRetainNonContentObligations(t *testing.T) {
	f := setup(t, nil)
	career := saveLifecycleProfile(t, f)
	s := f.open(t, "delete")
	d := f.sent(t, s)
	for i := 0; i < 2; i++ {
		if err := f.h.SyntheticReviewer().DeleteAction(context.Background(), f.p, s.ID()); err != nil {
			t.Fatal(err)
		}
		// There is no live account/Profile erasure workflow in this slice. These
		// explicit operator-owned synthetic SQL events exercise real row deletion
		// and the existing account/session authorization predicates.
		if _, err := f.db.Exec(`DELETE FROM careeros_profiles WHERE account_id=$1`, f.p.AccountID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`DELETE FROM careeros_profile_operations WHERE account_id=$1`, f.p.AccountID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Exec(`UPDATE careeros_accounts SET status='deleted',generation=generation+1 WHERE id=$1`, f.p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`DELETE FROM careeros_identities WHERE account_id=$1`, f.p.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(context.Background(), f.token); err == nil {
		t.Fatal("deleted account session survived")
	}
	if _, err := f.h.Open(context.Background(), f.p, "delete", "cv_generation", []byte(`{"synthetic":"root"}`)); err == nil {
		t.Fatal("deleted owner replay revived")
	}
	response, err := send(s)
	closeResp(response)
	if err == nil || f.calls.Load() != 1 {
		t.Fatal("deleted owner sent", err, f.calls.Load())
	}
	recreated, _ := login(t, f.svc, "fixture-owner")
	if recreated.AccountID == f.p.AccountID {
		t.Fatal("new identity reused deleted account")
	}
	if _, err = f.h.Status(context.Background(), recreated, s.ID()); err == nil {
		t.Fatal("recreated account inherited old action")
	}
	if err = f.h.SyntheticReviewer().SeedGrant(context.Background(), recreated, time.Now().Add(time.Hour), 3); err != nil {
		t.Fatal(err)
	}
	newAction, err := f.h.Open(context.Background(), recreated, "delete", "cv_generation", []byte(`{"synthetic":"root"}`))
	if err != nil || newAction.ID() == s.ID() {
		t.Fatal("new owner did not receive distinct identity", err)
	}
	var profiles, operations int
	f.db.QueryRow(`SELECT count(*) FROM careeros_profiles`).Scan(&profiles)
	f.db.QueryRow(`SELECT count(*) FROM careeros_profile_operations`).Scan(&operations)
	var count int
	var amount int64
	var state string
	if err = f.db.QueryRow(`SELECT count(*),sum(counted) FROM ai_fixture_dispatch`).Scan(&count, &amount); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT state FROM ai_fixture_action WHERE id=$1`, s.ID()).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if profiles != 0 || operations != 0 || count != 1 || amount != 60 || state != "deleted" {
		t.Fatal("deletion lost obligation or retained Profile", profiles, operations, count, amount, state)
	}
	var anchor time.Time
	if err = f.db.QueryRow(`SELECT durable_bound FROM ai_fixture_dispatch WHERE id=$1`, d.ID).Scan(&anchor); err != nil || !anchor.Equal(*d.DurableBound) {
		t.Fatal("deletion changed clock anchor", err)
	}
	for _, table := range []string{"ai_fixture_action", "ai_fixture_stage", "ai_fixture_dispatch", "ai_fixture_allocation", "ai_fixture_evidence"} {
		rows, err := f.db.Query(`SELECT row_to_json(v)::text FROM ` + table + ` v`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value string
			if err = rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(value, string(career)) || strings.Contains(value, "I hope to learn Javascript") || strings.Contains(value, "Javascript") || strings.Contains(value, "synthetic-must-not-reach-fixture") {
				t.Fatal("career/credential payload persisted in ledger", table)
			}
		}
		rows.Close()
	}
}

// SQL snapshots are deliberately older rows in the actual PostgreSQL schema,
// not fake status enums. A pinned connection owns temporary before/after images.
func captureFixtureImage(t *testing.T, c *sql.Conn, prefix string, tables []string) {
	t.Helper()
	for _, table := range tables {
		if _, err := c.ExecContext(context.Background(), `CREATE TEMP TABLE `+prefix+table+` AS TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
}
func restoreFixtureImage(t *testing.T, c *sql.Conn, prefix string, tables []string) {
	t.Helper()
	tx, err := c.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`TRUNCATE ` + strings.Join(tables, ",")); err != nil {
		t.Fatal(err)
	}
	// Deferred insertion order follows the schema's foreign-key graph.
	for _, table := range tables {
		if _, err = tx.Exec(`INSERT INTO ` + table + ` SELECT * FROM ` + prefix + table); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestA32_ActualStaleSQLRestoreNeedsIndependentObligationReconciliation(t *testing.T) {
	f := setup(t, nil)
	saveLifecycleProfile(t, f)
	intent := f.open(t, "restore_intent")
	deleted := f.open(t, "restore_delete")
	customer := f.open(t, "restore_customer")
	if err := customer.ReserveCustomer(context.Background()); err != nil {
		t.Fatal(err)
	}
	conn, err := f.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Include all fixture tables, including additions such as append-only
	// observations, while preserving their FK insertion order.
	tables := []string{"ai_fixture_namespace", "ai_fixture_policy", "ai_fixture_grant", "ai_fixture_scope", "ai_fixture_action", "ai_fixture_stage", "ai_fixture_child", "ai_fixture_dispatch", "ai_fixture_allocation", "ai_fixture_evidence"}
	rows, err := conn.QueryContext(context.Background(), `SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename LIKE 'ai_fixture_%' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, table := range tables {
		known[table] = true
	}
	for rows.Next() {
		var table string
		if err = rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		if !known[table] {
			tables = append(tables, table)
		}
	}
	rows.Close()
	tables = append(tables, "careeros_profiles", "careeros_profile_operations")
	captureFixtureImage(t, conn, "before_", tables)
	d := f.sent(t, intent)
	if err = f.h.SyntheticReviewer().DeleteAction(context.Background(), f.p, deleted.ID()); err != nil {
		t.Fatal(err)
	}
	if err = f.h.SyntheticReviewer().Customer(context.Background(), f.p, customer.ID(), CustomerEvent{Kind: "cancel"}); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(context.Background(), `DELETE FROM careeros_profiles WHERE account_id=$1`, f.p.AccountID); err != nil {
		t.Fatal(err)
	}
	captureFixtureImage(t, conn, "external_reconciled_", tables)
	f.h.cfg.Gate.Close("fixture")
	if err = f.h.cfg.Gate.Drain(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	restoreFixtureImage(t, conn, "before_", tables)
	var dispatches, profiles int
	var customerState, deleteState string
	f.db.QueryRow(`SELECT count(*) FROM ai_fixture_dispatch`).Scan(&dispatches)
	f.db.QueryRow(`SELECT count(*) FROM careeros_profiles`).Scan(&profiles)
	f.db.QueryRow(`SELECT state FROM ai_fixture_action WHERE id=$1`, customer.ID()).Scan(&customerState)
	f.db.QueryRow(`SELECT state FROM ai_fixture_action WHERE id=$1`, deleted.ID()).Scan(&deleteState)
	if dispatches != 0 || profiles != 1 || customerState != "recoverable" || deleteState != "recoverable" {
		t.Fatal("fixture did not actually rewind lost later facts", dispatches, profiles, customerState, deleteState)
	}
	for _, epoch := range []int{1, 2, 999} {
		if _, err = f.db.Exec(`UPDATE ai_fixture_namespace SET epoch=$1`, epoch); err != nil {
			t.Fatal(err)
		}
		if _, err = f.h.Open(context.Background(), f.p, "different_endpoint_key", "cv_generation", []byte(`{"synthetic":"root"}`)); err == nil {
			t.Fatal("database mutation reopened admission")
		}
		if _, err = f.h.Status(context.Background(), f.p, intent.ID()); err == nil {
			t.Fatal("stale protected status escaped")
		}
		resp, sendErr := send(intent)
		closeResp(resp)
		if sendErr == nil || f.calls.Load() != 1 || intent.ResultAllowed(context.Background()) == nil {
			t.Fatal("stale callback/result survived gate")
		}
	}
	// Explicit test-owned reconciliation restores every lost obligation, deletion
	// and terminal customer event BEFORE the independent gateway is opened.
	restoreFixtureImage(t, conn, "external_reconciled_", tables)
	if _, err = f.db.Exec(`UPDATE ai_fixture_namespace SET epoch=2`); err != nil {
		t.Fatal(err)
	}
	f.h.cfg.Gate.Reconcile("fixture", 2)
	status := f.status(t, intent)
	if len(status.Dispatches) != 1 || status.Dispatches[0].ID != d.ID || status.Dispatches[0].Counted != 60 || f.scope(t).Counted != 60 {
		t.Fatal("reconciliation lost unknown obligation", status)
	}
	if f.status(t, deleted).State != "deleted" || f.status(t, customer).State != "canceled" || f.status(t, customer).CustomerReserved {
		t.Fatal("terminal state resurrected")
	}
	f.db.QueryRow(`SELECT count(*) FROM careeros_profiles`).Scan(&profiles)
	if profiles != 0 {
		t.Fatal("reconciliation restored deleted career content")
	}
	resp, err := send(intent)
	closeResp(resp)
	if err == nil || f.calls.Load() != 1 || intent.ResultAllowed(context.Background()) == nil {
		t.Fatal("reopening revived old process capability", err)
	}
	fresh, err := f.h.Open(context.Background(), f.p, "restore_intent", "cv_generation", []byte(`{"synthetic":"root"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err = send(fresh)
	closeResp(resp)
	if !errors.Is(err, ErrOutputUnavailable) || f.calls.Load() != 1 {
		t.Fatal("reconciled intent replay sent again", err)
	}
}

func TestA33_RetirementRequiresDrainedSendersAndAllExplicitHorizons(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "retire")
	d := f.sent(t, s)
	proof := SyntheticRetirementProof{Namespace: "fixture", Epoch: 1, CapabilitiesExpired: true, RestoreHorizonExpired: true, SendersExcluded: true, AllEntrypointsClosed: true}
	reviewer := f.h.SyntheticReviewer()
	if err := reviewer.PruneNamespaceMarkers(context.Background(), f.p, proof); err == nil {
		t.Fatal("active namespace pruned")
	}
	started, finish := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		resp, err := s.gate.invoke(context.Background(), func(context.Context) (*http.Response, error) {
			close(started)
			<-finish
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("delayed fixture result"))}, nil
		})
		closeResp(resp)
		done <- err
	}()
	<-started
	f.h.cfg.Gate.Close("fixture")
	if err := reviewer.RetireNamespace(context.Background(), f.p); err != nil {
		t.Fatal(err)
	}
	if err := reviewer.RetireNamespace(context.Background(), f.p); err != nil {
		t.Fatal("duplicate closed-first retirement failed", err)
	}
	if err := reviewer.PruneNamespaceMarkers(context.Background(), f.p, proof); err == nil {
		t.Fatal("delayed callback falsely excluded")
	}
	close(finish)
	if err := <-done; !errors.Is(err, ErrDenied) {
		t.Fatal("retired callback returned result", err)
	}
	if err := f.h.cfg.Gate.Drain(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{"capability", "restore", "sender", "entrypoint"} {
		incomplete := proof
		switch missing {
		case "capability":
			incomplete.CapabilitiesExpired = false
		case "restore":
			incomplete.RestoreHorizonExpired = false
		case "sender":
			incomplete.SendersExcluded = false
		case "entrypoint":
			incomplete.AllEntrypointsClosed = false
		}
		if err := reviewer.PruneNamespaceMarkers(context.Background(), f.p, incomplete); err == nil {
			t.Fatal("missing explicit retirement qualification accepted", missing)
		}
	}
	if err := reviewer.PruneNamespaceMarkers(context.Background(), f.p, proof); err != nil {
		t.Fatal(err)
	}
	f.h.cfg.Gate.Reconcile("fixture", 1)
	f.h.cfg.Gate.Reconcile("fixture", 99)
	if f.h.cfg.Gate.permits("fixture", 1) || f.h.cfg.Gate.permits("fixture", 99) {
		t.Fatal("retired independent authority reopened")
	}
	// Even a stale database tombstone cannot undo independent retirement.
	if _, err := f.db.Exec(`UPDATE ai_fixture_namespace SET retired=false,epoch=99`); err != nil {
		t.Fatal(err)
	}
	resp, err := send(s)
	closeResp(resp)
	if err == nil || f.calls.Load() != 1 || s.ResultAllowed(context.Background()) == nil {
		t.Fatal("retired process capability revived")
	}
	var amount int64
	var occupied int
	var anchor time.Time
	var compacted bool
	if err = f.db.QueryRow(`SELECT counted,durable_bound,compacted FROM ai_fixture_dispatch WHERE id=$1`, d.ID).Scan(&amount, &anchor, &compacted); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT occupied FROM ai_fixture_scope WHERE scope='global'`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if amount != 60 || occupied != 1 || !anchor.Equal(*d.DurableBound) || !compacted {
		t.Fatal("retirement discarded financial/physical obligation", amount, occupied, anchor, compacted)
	}
}
