package accountapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDeletionOwnershipConfirmationAndRequestBinding(t *testing.T) {
	dsn, db := database(t)
	store := open(t, dsn, "deletion_ownership")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	a, b := login(t, svc, "deletion-a"), login(t, svc, "deletion-b")
	h := New(svc, store)
	raw := fixture(t, 1)
	for _, who := range []string{"a", "b"} {
		owner := a
		if who == "b" {
			owner = b
		}
		if w := api(t, h, owner, "PUT", "/api/profile/current", command("save", 0, raw)); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	var doc struct {
		ID string `json:"id"`
	}
	json.Unmarshal(raw, &doc)
	valid := map[string]any{"operationId": "owner-delete", "kind": "profile", "profileId": doc.ID, "revision": 1, "confirm": true}
	for _, change := range []map[string]any{{"confirm": false}, {"revision": 2}, {"accountId": b.Account.ID}, {"profileId": "foreign-profile"}} {
		input := map[string]any{}
		for k, v := range valid {
			input[k] = v
		}
		for k, v := range change {
			input[k] = v
		}
		body, _ := json.Marshal(input)
		if w := api(t, h, a, "POST", "/api/deletion/operations", body); w.Code < 400 {
			t.Fatal("invalid confirmation gained effect", change, w.Code)
		}
	}
	var operations int
	if err := db.QueryRow(`SELECT count(*) FROM careeros_deletion_operations`).Scan(&operations); err != nil || operations != 0 {
		t.Fatal("rejected request persisted", operations, err)
	}
	body, _ := json.Marshal(valid)
	if w := api(t, h, a, "POST", "/api/deletion/operations", body); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := api(t, h, b, "GET", "/api/profile/export", nil); w.Code != 200 {
		t.Fatal("another owner lost content", w.Code)
	}
	if w := api(t, h, b, "GET", "/api/deletion/operations/owner-delete", nil); w.Code != 404 {
		t.Fatal("foreign operation leaked", w.Code)
	}
	valid["revision"] = 2
	body, _ = json.Marshal(valid)
	if w := api(t, h, a, "POST", "/api/deletion/operations", body); w.Code != 409 {
		t.Fatal("changed-content replay accepted", w.Code)
	}
}

func TestDeletionTablesRejectInheritedLedgerReaderPrivileges(t *testing.T) {
	dsn, db := database(t)
	var schema string
	if err := db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	parent := fmt.Sprintf("deletion_parent_%d", time.Now().UnixNano())
	child := parent + "_child"
	if _, err := db.Exec(`CREATE ROLE ` + parent + ` NOLOGIN;CREATE ROLE ` + child + ` NOLOGIN IN ROLE ` + parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DROP OWNED BY ` + child + `,` + parent); db.Exec(`DROP ROLE ` + child + `,` + parent) })
	if _, err := db.Exec(`ALTER DEFAULT PRIVILEGES IN SCHEMA ` + schema + ` GRANT SELECT ON TABLES TO ` + parent); err != nil {
		t.Fatal(err)
	}
	store := open(t, dsn, "deletion_acl")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"careeros_deletion_operations", "careeros_deleted_profiles", "careeros_deletion_status_sessions", "careeros_profile_pruning", "careeros_invitation_cleanup", "careeros_lifecycle_notices", "careeros_lifecycle_cleanup", "careeros_lifecycle_scan", "careeros_lifecycle_purpose_cleanup", "careeros_lifecycle_residuals", "careeros_lifecycle_residual_evidence"} {
		var canRead bool
		if err := db.QueryRow(`SELECT has_table_privilege($1,$2,'SELECT')`, child, table).Scan(&canRead); err != nil || canRead {
			t.Fatal("inherited content/status capability", table, canRead, err)
		}
	}
	for _, name := range []string{"careeros_account_closed", "careeros_deleted_profile_restored", "careeros_action_predates_deletion", "careeros_invitation_denied"} {
		var def, config string
		var privileged, mutable bool
		if err := db.QueryRow(`SELECT pg_get_functiondef(oid),array_to_string(proconfig,';'),prosecdef,provolatile<>'s' FROM pg_proc WHERE pronamespace=current_schema()::regnamespace AND proname=$1`, name).Scan(&def, &config, &privileged, &mutable); err != nil {
			t.Fatal(err)
		}
		if !privileged || mutable || config != "search_path=pg_catalog, pg_temp" || !strings.Contains(def, schema+".careeros_") {
			t.Fatal("unbounded deny predicate", name, config, privileged, mutable)
		}
		var public bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_proc p CROSS JOIN LATERAL aclexplode(COALESCE(p.proacl,acldefault('f',p.proowner))) a WHERE p.pronamespace=current_schema()::regnamespace AND p.proname=$1 AND a.grantee=0 AND a.privilege_type='EXECUTE')`, name).Scan(&public); err != nil || public {
			t.Fatal("PUBLIC execute", name, public, err)
		}
	}
}
