package pilotruntime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/hosteddraft"
)

const (
	draftFixtureWriter           = "careeros_294_draft_writer"
	draftFixtureWriterPassword   = "local-draft-writer-fixture"
	draftFixtureAttestor         = "careeros_294_draft_attestor"
	draftFixtureAttestorPassword = "local-draft-attestor-fixture"
)

func TestHostedDraftAuthorityWithLocalTLSPostgres(t *testing.T) {
	const role = "CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 2 PASSWORD '%s';\n"
	roles := fmt.Sprintf(role, draftFixtureWriter, draftFixtureWriterPassword) + fmt.Sprintf(role, draftFixtureAttestor, draftFixtureAttestorPassword)
	runHostedTLSFixture(t, roles, "TestHostedDraftAuthorityTLSFixtureChild")
}

func TestHostedDraftAuthorityTLSFixtureChild(t *testing.T) {
	dsn := hostedTLSFixtureDSN(t)
	certificate := os.Getenv("SSL_CERT_FILE")
	adminDSN := url.URL{
		Scheme: "postgresql",
		User:   url.User("fixture_admin"),
		Path:   "/careeros_294",
		RawQuery: url.Values{
			"host":    {filepath.Dir(filepath.Dir(certificate))},
			"port":    {os.Getenv("CAREEROS_HOSTED_TLS_FIXTURE_PORT")},
			"sslmode": {"disable"},
		}.Encode(),
	}
	connect := func(dsn string) *pgx.Conn {
		t.Helper()
		connection, err := pgx.Connect(t.Context(), dsn)
		if err != nil {
			t.Fatalf("connect isolated fixture role: %v", err)
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := connection.Close(ctx); err != nil {
				t.Errorf("close isolated fixture role: %v", err)
			}
		})
		return connection
	}
	exec := func(connection *pgx.Conn, statement string) {
		t.Helper()
		if _, err := connection.Exec(t.Context(), statement); err != nil {
			t.Fatalf("execute isolated fixture SQL: %v", err)
		}
	}
	tlsRole := func(role, password string) *pgx.Conn {
		t.Helper()
		address, err := url.Parse(dsn(role))
		if err != nil {
			t.Fatal(err)
		}
		address.User = url.UserPassword(role, password)
		address.RawQuery += "&sslrootcert=" + url.QueryEscape(certificate)
		return connect(address.String())
	}
	admin := connect(adminDSN.String())
	var owned bool
	if err := admin.QueryRow(t.Context(), `SELECT current_setting('data_directory')=$1 AND inet_server_addr() IS NULL`, filepath.Dir(certificate)).Scan(&owned); err != nil || !owned {
		t.Fatalf("verify private PostgreSQL fixture: %v", err)
	}
	if err := accountstore.MigrateHostedPilot(t.Context(), dsn(fixtureMigrator)); err != nil {
		t.Fatalf("migrate fixture Account schema: %v", err)
	}
	exec(admin, hosteddraft.BootstrapSQL())
	migrator := tlsRole(fixtureMigrator, fixturePassword)
	exec(migrator, hosteddraft.MigrationSQL())
	if err := accountstore.MigrateHostedPilot(t.Context(), dsn(fixtureMigrator)); err != nil {
		t.Fatalf("repeat fixture Account migration with the Draft schema installed: %v", err)
	}
	closed, err := accountstore.OpenHostedPilot(t.Context(), dsn(fixtureRuntime))
	if err != nil {
		t.Fatalf("open closed Account runtime with the Draft schema installed: %v", err)
	}
	defer closed.Close()
	if err := closed.HostedPilotClosed(t.Context()); err != nil {
		t.Fatalf("closed Account readiness with the Draft schema installed: %v", err)
	}
	runtime := tlsRole(fixtureRuntime, fixturePassword)
	writer := tlsRole(draftFixtureWriter, draftFixtureWriterPassword)
	attestor := tlsRole(draftFixtureAttestor, draftFixtureAttestorPassword)

	const intent = `SELECT careeros_294_draft.record_fake_intent($1,$2,$3,$4::bigint::numeric,$5,$6,$7)`
	const attest = `SELECT careeros_294_draft.record_fake_attestation($1,$2::smallint,$3,$4)`
	ownerToken := sha256.Sum256([]byte("draft-fixture-token-a"))
	otherToken := sha256.Sum256([]byte("draft-fixture-token-b"))
	request := sha256.Sum256([]byte("draft-fixture-request"))
	changedRequest := sha256.Sum256([]byte("draft-fixture-changed-request"))
	result := sha256.Sum256([]byte("draft-fixture-result"))
	intentArgs := func(stage string, binding []byte) []any {
		return []any{"draft-action", "draft-owner", "draft-session-a", int64(1), ownerToken[:], stage, binding}
	}
	attestArgs := func(stage int16, binding []byte) []any {
		return []any{"draft-action", stage, binding, result[:]}
	}
	denied := func(connection *pgx.Conn, statement string, args ...any) {
		t.Helper()
		_, err := connection.Exec(t.Context(), statement, args...)
		var pgError *pgconn.PgError
		if !errors.As(err, &pgError) || pgError.Code != "42501" {
			t.Fatalf("fixture role permission result = %v, want insufficient privilege", err)
		}
	}
	for _, connection := range []*pgx.Conn{runtime, writer, attestor} {
		denied(connection, "SELECT * FROM careeros_294_draft.fake_scope")
		denied(connection, "SELECT * FROM careeros_294_draft.fake_intents")
		denied(connection, "SELECT * FROM careeros_294_draft.fake_attestations")
		denied(connection, "DELETE FROM careeros_294_draft.fake_scope")
	}
	for _, connection := range []*pgx.Conn{writer, attestor} {
		denied(connection, "SELECT * FROM careeros_294.careeros_sessions")
	}
	denied(runtime, intent, intentArgs("field_validation", request[:])...)
	denied(runtime, attest, attestArgs(1, request[:])...)
	denied(attestor, intent, intentArgs("field_validation", request[:])...)
	denied(writer, attest, attestArgs(1, request[:])...)

	check := func(label string, connection *pgx.Conn, expected bool, statement string, args ...any) {
		t.Helper()
		var actual bool
		if err := connection.QueryRow(t.Context(), statement, args...).Scan(&actual); err != nil || actual != expected {
			t.Fatalf("%s = %t, want %t, error: %v", label, actual, expected, err)
		}
	}
	exec(migrator, hostedDraftFixtureAccounts)
	// This transition belongs only to the private test cluster. It creates
	// neither an invitation path nor a runtime configuration for real egress.
	const openInference = `UPDATE careeros_294.careeros_control_state SET initialized=true, inference_stopped=false WHERE singleton`
	const closeInference = `UPDATE careeros_294.careeros_control_state SET inference_stopped=true WHERE singleton`
	exec(migrator, openInference)
	check("no test scope on install", writer, false, intent, intentArgs("field_validation", request[:])...)
	exec(migrator, closeInference)
	exec(migrator, hostedDraftFixtureScope)
	check("inference stopped", writer, false, intent, intentArgs("field_validation", request[:])...)
	check("missing intent", attestor, false, attest, attestArgs(1, request[:])...)
	exec(migrator, openInference)
	foreign := intentArgs("field_validation", request[:])
	foreign[1], foreign[2], foreign[4] = "draft-other", "draft-session-b", otherToken[:]
	check("foreign Account", writer, false, intent, foreign...)
	badToken := intentArgs("field_validation", request[:])
	badToken[4] = otherToken[:]
	check("wrong Session binding", writer, false, intent, badToken...)
	exec(migrator, "UPDATE careeros_294_draft.fake_scope SET expires_at='infinity'")
	check("unbounded scope expiry", writer, false, intent, intentArgs("field_validation", request[:])...)
	exec(migrator, "UPDATE careeros_294_draft.fake_scope SET expires_at=clock_timestamp()+interval '5 minutes'")
	check("skipped stage", writer, false, intent, intentArgs("generation", request[:])...)
	check("first intent", writer, true, intent, intentArgs("field_validation", request[:])...)
	check("duplicate intent", writer, false, intent, intentArgs("field_validation", changedRequest[:])...)
	check("mismatched attestation", attestor, false, attest, attestArgs(1, changedRequest[:])...)
	check("unattested predecessor", writer, false, intent, intentArgs("generation", request[:])...)
	check("first attestation", attestor, true, attest, attestArgs(1, request[:])...)
	check("duplicate attestation", attestor, false, attest, attestArgs(1, request[:])...)
	check("next ordered intent", writer, true, intent, intentArgs("generation", changedRequest[:])...)
	exec(migrator, closeInference)
	check("already-issued intent can be attested during stop", attestor, true, attest, attestArgs(2, changedRequest[:])...)
	check("stop denies subsequent intent", writer, false, intent, intentArgs("resume_support", request[:])...)

	var intents, attestations int
	if err := migrator.QueryRow(t.Context(), `SELECT
	 (SELECT count(*) FROM careeros_294_draft.fake_intents),
	 (SELECT count(*) FROM careeros_294_draft.fake_attestations)`).Scan(&intents, &attestations); err != nil || intents != 2 || attestations != 2 {
		t.Fatalf("durable evidence = %d intents, %d attestations, error: %v", intents, attestations, err)
	}
}

const hostedDraftFixtureAccounts = `
INSERT INTO careeros_294.careeros_accounts (id,status,generation,created_at,email,display_name) VALUES
 ('draft-owner','active',1,clock_timestamp(),'',''), ('draft-other','active',1,clock_timestamp(),'','');
INSERT INTO careeros_294.careeros_identities (provider,issuer,subject,account_id,created_at) VALUES
 ('google','https://accounts.google.com','draft-a','draft-owner',clock_timestamp()),
 ('google','https://accounts.google.com','draft-b','draft-other',clock_timestamp());
INSERT INTO careeros_294.careeros_sessions (id,account_id,token_hash,csrf_hash,generation,created_at,expires_at) VALUES
 ('draft-session-a','draft-owner',sha256('draft-fixture-token-a'::bytea),sha256('draft-csrf-a'::bytea),1,clock_timestamp(),clock_timestamp()+interval '5 minutes'),
 ('draft-session-b','draft-other',sha256('draft-fixture-token-b'::bytea),sha256('draft-csrf-b'::bytea),1,clock_timestamp(),clock_timestamp()+interval '5 minutes');
INSERT INTO careeros_294.careeros_pilot_access
 (account_id,provider,issuer,subject,age_eligible,eligibility_checked_at,eligibility_checked_by,activated_at,expires_at,participant_slot,state) VALUES
 ('draft-owner','google','https://accounts.google.com','draft-a',true,clock_timestamp(),'draft-other',clock_timestamp(),clock_timestamp()+interval '5 minutes',1,'active');
INSERT INTO careeros_294.careeros_profiles (account_id,profile_id,revision,document) VALUES ('draft-owner','draft-profile',1,'{}');
`

const hostedDraftFixtureScope = `
INSERT INTO careeros_294_draft.fake_scope
 (singleton,action_id,account_id,grant_activated_at,grant_version,profile_id,profile_revision,inference_epoch,expires_at)
 SELECT true,'draft-action',account_id,activated_at,authority_version,'draft-profile',1,1,clock_timestamp()+interval '5 minutes'
 FROM careeros_294.careeros_pilot_access WHERE account_id='draft-owner';
`
