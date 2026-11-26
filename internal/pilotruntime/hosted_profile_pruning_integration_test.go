package pilotruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
)

func TestClosedHostedProfilePruningWithLocalTLSPostgres(t *testing.T) {
	runHostedTLSFixture(t, "", "TestClosedHostedProfilePruningTLSFixtureChild")
}

func TestClosedHostedProfilePruningTLSFixtureChild(t *testing.T) {
	dsn := hostedTLSFixtureDSN(t)
	ctx := t.Context()
	if err := accountstore.MigrateHostedPilot(ctx, dsn(fixtureMigrator)); err != nil {
		t.Fatal(err)
	}
	fixtureConnect := func(role string) *pgx.Conn {
		t.Helper()
		conn, err := pgx.Connect(ctx, dsn(role)+"&sslrootcert="+url.QueryEscape(os.Getenv("SSL_CERT_FILE")))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close(ctx) })
		return conn
	}
	migrator := fixtureConnect(fixtureMigrator)
	provider := newHostedFixtureGoogle(t)
	config := HostedConfig{
		Assets:      fstest.MapFS{"index.html": {Data: []byte("closed fixture")}},
		DatabaseURL: dsn(fixtureRuntime),
		GoogleSubjects: `[{"issuer":"https://accounts.google.com","subject":"fixture-A"},` +
			`{"issuer":"https://accounts.google.com","subject":"fixture-B"}]`,
		Auth: auth.Config{
			PublicOrigin: hostedTestOrigin, SessionTTL: time.Hour, TransactionTTL: 5 * time.Minute,
			HTTPClient: &http.Client{Transport: provider},
			Providers:  []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(hostedTestOrigin, hostedTestClient, "inert-fixture-secret")},
		},
	}
	open := func() (http.Handler, func()) {
		t.Helper()
		server, closeStore, err := NewHostedServer(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		return server.Handler, closeStore
	}
	handler, closeStore := open()
	defer closeStore()
	cookies := hostedFixtureLogin(t, provider, handler, "fixture-A")
	owner := hostedFixtureSession(t, handler, cookies)
	if _, err := migrator.Exec(ctx, `INSERT INTO careeros_294.careeros_pilot_access
(account_id,provider,issuer,subject,activated_at,expires_at,participant_slot,state)
SELECT account_id,provider,issuer,subject,clock_timestamp(),clock_timestamp()+interval '1 hour',1,'active'
FROM careeros_294.careeros_identities WHERE account_id=$1`, owner); err != nil {
		t.Fatal(err)
	}
	document := hostedProfilePruningDocument(t)
	before := hostedPruningProfileBytes(t, document)
	save := func(operation string, revision int64, doc []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, hostedTestOrigin+"/api/profile/current", bytes.NewBufferString(
			fmt.Sprintf(`{"operationId":%q,"expectedRevision":%d,"document":%s}`, operation, revision, doc)))
		req.Header.Set("Origin", hostedTestOrigin)
		req.Header.Set("X-CareerOS-Account", owner)
		for _, cookie := range cookies {
			req.AddCookie(cookie)
			if cookie.Name == "__Host-careeros_csrf" {
				req.Header.Set("X-CSRF-Token", cookie.Value)
			}
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	check := func(step string, got *httptest.ResponseRecorder, want int) {
		t.Helper()
		if got.Code != want {
			t.Fatalf("%s = %d, expected %d: %s", step, got.Code, want, got.Body.String())
		}
	}
	check("approved excerpt save", save("profile-with-excerpt", 0, before), http.StatusOK)
	if _, err := migrator.Exec(ctx, `ALTER TABLE careeros_294.careeros_profile_operations DROP COLUMN hosted_save_transaction`); err != nil {
		t.Fatal(err)
	}
	if err := accountstore.MigrateHostedPilot(ctx, dsn(fixtureMigrator)); err != nil {
		t.Fatal(err)
	}
	var legacyReceipt bool
	if err := migrator.QueryRow(ctx, `SELECT hosted_save_transaction IS NULL FROM careeros_294.careeros_profile_operations
WHERE account_id=$1 AND operation_id='profile-with-excerpt'`, owner).Scan(&legacyReceipt); err != nil || !legacyReceipt {
		t.Fatalf("legacy receipt gained a transaction binding or was lost: %v", err)
	}
	check("legacy receipt replay after migration", save("profile-with-excerpt", 0, before), http.StatusOK)
	removed := document.Evidence[0].ID
	document.Revision++
	for i := range document.Facts {
		document.Facts[i].Owner.Revision = document.Revision
	}
	document.Facts[0].Support = "unsupported"
	document.Evidence = []profiledocument.Evidence{}
	document.Links = document.Links[1:]
	after := hostedPruningProfileBytes(t, document)
	if _, err := migrator.Exec(ctx, `CREATE FUNCTION careeros_294.fixture_refuse_pruning() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,pg_temp AS $$ BEGIN RAISE EXCEPTION 'synthetic pruning write failure'; END $$;
CREATE TRIGGER fixture_refuse_pruning BEFORE INSERT ON careeros_294.careeros_profile_pruning
FOR EACH ROW EXECUTE FUNCTION careeros_294.fixture_refuse_pruning()`); err != nil {
		t.Fatal(err)
	}
	check("failed pruning Save", save("remove-excerpt", 1, after), http.StatusServiceUnavailable)
	check("receipt after failed Save", hostedFixtureRequest(handler, http.MethodGet, "/api/profile/operations/remove-excerpt", cookies,
		map[string]string{"X-CareerOS-Account": owner}), http.StatusNotFound)
	var stored []byte
	if err := migrator.QueryRow(ctx, `SELECT document FROM careeros_294.careeros_profiles WHERE account_id=$1`, owner).Scan(&stored); err != nil || !bytes.Equal(stored, before) {
		t.Fatalf("failed pruning changed Profile: %v", err)
	}
	if _, err := migrator.Exec(ctx, `DROP TRIGGER fixture_refuse_pruning ON careeros_294.careeros_profile_pruning;
DROP FUNCTION careeros_294.fixture_refuse_pruning()`); err != nil {
		t.Fatal(err)
	}
	check("approved excerpt removal", save("remove-excerpt", 1, after), http.StatusOK)
	closeStore()
	handler, closeStore = open()
	defer closeStore()
	check("replay after runtime restart", save("remove-excerpt", 1, after), http.StatusOK)

	var count int
	var validFence bool
	if err := migrator.QueryRow(ctx, `SELECT count(*),COALESCE(bool_and(previous_revision=1 AND new_revision=2
AND recovery_copy_deadline-removed_at=interval '720 hours' AND evidence_id=$2),false)
FROM careeros_294.careeros_profile_pruning WHERE account_id=$1`, owner, removed).Scan(&count, &validFence); err != nil || count != 1 || !validFence {
		t.Fatalf("pruning fence count = %d, valid = %t, error = %v", count, validFence, err)
	}
	runtime := fixtureConnect(fixtureRuntime)
	var denied *pgconn.PgError
	if _, err := runtime.Exec(ctx, `DELETE FROM careeros_294.careeros_profile_pruning WHERE account_id=$1`, owner); !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatalf("hosted runtime pruning fence deletion = %v", err)
	}
	resurrected, err := profiledocument.Decode(before)
	if err != nil {
		t.Fatal(err)
	}
	resurrected.Revision = 3
	for i := range resurrected.Facts {
		resurrected.Facts[i].Owner.Revision = resurrected.Revision
	}
	resubmitted := hostedPruningProfileBytes(t, resurrected)
	check("attempt to restore pruned identifier", save("resurrect-excerpt", 2, resubmitted), http.StatusConflict)
	if _, err := runtime.Exec(ctx, `UPDATE careeros_294.careeros_profiles SET revision=3,document=$2 WHERE account_id=$1`, owner, resubmitted); !errors.As(err, &denied) || denied.Message != "hosted Profile contains a pruned evidence identifier" {
		t.Fatalf("hosted runtime raw attempt to restore a pruned identifier = %v", err)
	}
	// Reconciliation may retain a pruning fence even when the Profile is absent.
	if _, err := migrator.Exec(ctx, `DELETE FROM careeros_294.careeros_profiles WHERE account_id=$1`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Exec(ctx, `INSERT INTO careeros_294.careeros_profiles (account_id,profile_id,revision,document)
VALUES ($1,$2,3,$3)`, owner, []byte(document.ID), resubmitted); !errors.As(err, &denied) || denied.Message != "hosted Profile cannot be reinserted behind a pruning fence" {
		t.Fatalf("hosted runtime raw attempt to reinsert a pruned Profile = %v", err)
	}
}

func hostedProfilePruningDocument(t *testing.T) profiledocument.Document {
	t.Helper()
	fixtures, err := os.ReadFile("../profiledocument/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct{ Document json.RawMessage }
	if err = json.Unmarshal(fixtures, &entries); err != nil || len(entries) == 0 {
		t.Fatalf("Profile fixture unavailable: %v", err)
	}
	doc, err := profiledocument.Decode(entries[0].Document)
	if err != nil {
		t.Fatal(err)
	}
	opaque := doc.Facts[0]
	opaque.ID, opaque.Field = "opaque-notes", "opaque_notes"
	opaque.Support = "unsupported"
	opaque.Value = json.RawMessage(`{"note":"synthetic \u0000 value"}`)
	doc.Facts = append(doc.Facts, opaque)
	return doc
}

func hostedPruningProfileBytes(t *testing.T, doc profiledocument.Document) []byte {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profiledocument.Decode(raw); err != nil {
		t.Fatal(err)
	}
	return raw
}
