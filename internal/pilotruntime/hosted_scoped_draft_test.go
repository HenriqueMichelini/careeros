package pilotruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/hosteddraft"
	"professional-information-repo/internal/testsupport/aiadmission"
)

const hostedScopedDraftKey = "approved-hosted-draft"
const hostedScopedDraftID = "synthetic_hosted_draft"

func TestHostedScopedDraftWithLocalTLSPostgres(t *testing.T) {
	runHostedTLSFixture(t, hostedDraftFixtureRoles(), "TestHostedScopedDraftTLSFixtureChild")
}

func TestHostedScopedDraftTLSFixtureChild(t *testing.T) {
	runHostedScopedDraftFixture(t, false)
}

func TestHostedScopedDraftStopWithLocalTLSPostgres(t *testing.T) {
	runHostedTLSFixture(t, hostedDraftFixtureRoles(), "TestHostedScopedDraftStopTLSFixtureChild")
}

func TestHostedScopedDraftStopTLSFixtureChild(t *testing.T) {
	runHostedScopedDraftFixture(t, true)
}

func runHostedScopedDraftFixture(t *testing.T, stopDuringHandoff bool) {
	t.Helper()
	dsn := hostedTLSFixtureDSN(t)
	ctx := t.Context()
	certificate := os.Getenv("SSL_CERT_FILE")
	adminURL := url.URL{
		Scheme: "postgresql", User: url.User("fixture_admin"), Path: "/careeros_294",
		RawQuery: url.Values{
			"host": {filepath.Dir(filepath.Dir(certificate))},
			"port": {os.Getenv("CAREEROS_HOSTED_TLS_FIXTURE_PORT")}, "sslmode": {"disable"},
		}.Encode(),
	}
	admin, err := pgx.Connect(ctx, adminURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	var owned bool
	if err := admin.QueryRow(ctx, `SELECT current_setting('data_directory')=$1 AND inet_server_addr() IS NULL`, filepath.Dir(certificate)).Scan(&owned); err != nil || !owned {
		t.Fatalf("private database fixture verification: %t %v", owned, err)
	}
	if err := accountstore.MigrateHostedPilot(ctx, dsn(fixtureMigrator)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, hosteddraft.BootstrapSQL()); err != nil {
		t.Fatal(err)
	}
	actionKey := sha256.Sum256([]byte("only the fixture knows this action binding key"))
	artifactKey := sha256.Sum256([]byte("only the fixture knows this artifact binding key"))
	attestationToken := sha256.Sum256([]byte("only the fixture knows this attestation token"))
	expires := time.Now().Add(15 * time.Minute).UTC().Truncate(time.Second)
	if err := MigrateHostedDraft(ctx, dsn(fixtureMigrator), sha256.Sum256(actionKey[:]), sha256.Sum256(artifactKey[:]), expires); err != nil {
		t.Fatalf("scoped migration: %v", err)
	}
	if err := MigrateHostedDraft(ctx, dsn(fixtureMigrator), sha256.Sum256(actionKey[:]), sha256.Sum256(artifactKey[:]), expires); err != nil {
		t.Fatalf("repeat identical scoped migration: %v", err)
	}
	if err := MigrateHostedDraft(ctx, dsn(fixtureMigrator), sha256.Sum256(actionKey[:]), sha256.Sum256([]byte("different key")), expires); err == nil {
		t.Fatal("scoped migration accepted an unreviewed artifact key")
	}

	provider := newHostedFixtureGoogle(t)
	config := HostedConfig{
		Assets:         fstest.MapFS{"index.html": {Data: []byte("closed fixture")}},
		DatabaseURL:    dsn(fixtureRuntime),
		GoogleSubjects: `[{"issuer":"https://accounts.google.com","subject":"fixture-A"},{"issuer":"https://accounts.google.com","subject":"fixture-B"}]`,
		Auth: auth.Config{
			PublicOrigin: hostedTestOrigin, SessionTTL: time.Hour, TransactionTTL: 5 * time.Minute,
			HTTPClient: &http.Client{Transport: provider},
			Providers:  []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(hostedTestOrigin, hostedTestClient, "inert-fixture-secret")},
		},
	}
	base, closeBase, err := NewHostedServer(ctx, config)
	if err != nil {
		t.Fatalf("closed base with staged Draft schema: %v", err)
	}
	defer closeBase()
	ownerCookies := hostedFixtureLogin(t, provider, base.Handler, "fixture-A")
	otherCookies := hostedFixtureLogin(t, provider, base.Handler, "fixture-B")
	owner := hostedFixtureSession(t, base.Handler, ownerCookies)
	other := hostedFixtureSession(t, base.Handler, otherCookies)
	exec := func(statement string, arguments ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, statement, arguments...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO careeros_294.careeros_pilot_access
 (account_id,provider,issuer,subject,age_eligible,eligibility_checked_at,eligibility_checked_by,activated_at,expires_at,participant_slot,state)
 SELECT account_id,provider,issuer,subject,true,clock_timestamp(),$2,clock_timestamp(),$3,1,'active'
 FROM careeros_294.careeros_identities WHERE account_id=$1`, owner, other, expires)

	request := func(handler http.Handler, method, path, id string, cookies []*http.Cookie, body []byte, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, hostedTestOrigin+path, bytes.NewReader(body))
		r.Header.Set("Origin", hostedTestOrigin)
		r.Header.Set("X-CareerOS-Account", id)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
			if cookie.Name == "__Host-careeros_csrf" {
				r.Header.Set("X-CSRF-Token", cookie.Value)
			}
		}
		for header, value := range headers {
			r.Header.Set(header, value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	profile := aiadmission.Profile(t)
	save := fmt.Appendf(nil, `{"operationId":"draft-fixture-profile","expectedRevision":0,"document":%s}`, profile)
	if got := request(base.Handler, http.MethodPut, "/api/profile/current", owner, ownerCookies, save, nil); got.Code != 200 {
		t.Fatalf("save approved Profile: %d %s", got.Code, got.Body.String())
	}
	closeBase()
	exec(`UPDATE careeros_294.careeros_control_state SET initialized=true WHERE singleton`)
	exec(`INSERT INTO careeros_294_draft.fake_scope
 (singleton,action_id,account_id,grant_activated_at,grant_version,profile_id,profile_revision,inference_epoch,expires_at)
 SELECT true,$2,a.account_id,a.activated_at,a.authority_version,p.profile_id,p.revision,c.inference_epoch,$3
 FROM careeros_294.careeros_pilot_access a JOIN careeros_294.careeros_profiles p USING (account_id)
 CROSS JOIN careeros_294.careeros_control_state c WHERE a.account_id=$1 AND c.singleton`, owner, hostedScopedDraftID, expires)
	exec(`INSERT INTO careeros_294_draft.hosted_draft_action_scope(singleton,action_id,creation_key) VALUES(true,$1,$2)`, hostedScopedDraftID, hostedScopedDraftKey)

	roleDSN := func(role, password string) string {
		u, err := url.Parse(dsn(role))
		if err != nil {
			t.Fatal(err)
		}
		u.User = url.UserPassword(role, password)
		return u.String()
	}
	attestorDB, err := accountstore.OpenHostedDraftAttestor(ctx, roleDSN(draftFixtureAttestor, draftFixtureAttestorPassword))
	if err != nil {
		t.Fatal(err)
	}
	defer attestorDB.Close()
	attestorHandler, err := hosteddraft.NewAttestorHandler(attestorDB, attestationToken)
	if err != nil {
		t.Fatal(err)
	}
	attestorServer := httptest.NewTLSServer(attestorHandler)
	defer attestorServer.Close()
	attestorURL, err := url.Parse(attestorServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	attestorClient, err := hosteddraft.NewAttestorClientWithHTTPClient(
		"https://careeros-294-attestor-123456789012.southamerica-east1.run.app", attestationToken, nil,
		&http.Client{Transport: fixtureAttestorTransport{attestorURL.Host, attestorServer.Client().Transport}},
	)
	if err != nil {
		t.Fatalf("HTTPS fixture attestor client: %v", err)
	}
	var sends atomic.Int32
	var stopDone <-chan error
	respond := hosteddraft.NewFakeResponder(attestorClient)
	config.Draft = &HostedDraftConfig{
		LedgerURL: roleDSN(draftFixtureLedger, draftFixtureLedgerPassword),
		ActionKey: actionKey, ArtifactKey: artifactKey,
		Respond: func(ctx context.Context, call aicontrol.HostedDraftCall) (*http.Response, error) {
			if sends.Add(1) == 1 && stopDuringHandoff {
				stopDone = queueHostedFixtureStop(t, adminURL.String(), admin)
			}
			return respond(ctx, call)
		},
	}
	badConfig := config
	badDraft := *config.Draft
	badDraft.ArtifactKey = sha256.Sum256([]byte("different from the installed artifact key"))
	badConfig.Draft = &badDraft
	degraded, closeDegraded, err := NewHostedServer(ctx, badConfig)
	if err != nil {
		t.Fatalf("bad artifact key should preserve Account startup: %v", err)
	}
	if hostedFixtureSession(t, degraded.Handler, ownerCookies) != owner {
		t.Fatal("unavailable artifact authority disabled Account session")
	}
	if got := hostedFixtureRequest(degraded.Handler, http.MethodGet, "/readyz", nil, nil); got.Code != 503 {
		t.Fatalf("mismatched artifact key looked ready: %d", got.Code)
	}
	closeDegraded()
	current, closeCurrent, err := NewHostedServer(ctx, config)
	if err != nil {
		t.Fatalf("scoped runtime startup: %v", err)
	}
	defer closeCurrent()
	ownerRequest := func(method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
		return request(current.Handler, method, path, owner, ownerCookies, body, headers)
	}
	want := func(label string, got *httptest.ResponseRecorder, status int) {
		t.Helper()
		if got.Code != status {
			t.Fatalf("%s: %d want %d: %s", label, got.Code, status, got.Body.String())
		}
	}
	ready := ownerRequest(http.MethodGet, "/readyz", nil, nil)
	want("closed readiness", ready, 200)
	if !strings.Contains(ready.Body.String(), `"inference":"closed"`) {
		t.Fatal("closed readiness did not report canonical stop")
	}
	input := []byte(`{"profileSource":{"id":"p","revision":1},"job":{"posting":"Java required."},"language":"en"}`)
	keyHeader := map[string]string{"X-CareerOS-Action-Key": hostedScopedDraftKey}
	if got := ownerRequest(http.MethodPost, "/api/drafts", input, keyHeader); got.Code < 400 || sends.Load() != 0 {
		t.Fatalf("inference stop must deny before send: %d %s", got.Code, got.Body.String())
	}
	exec(`UPDATE careeros_294.careeros_control_state SET inference_stopped=false WHERE singleton`)
	ready = ownerRequest(http.MethodGet, "/readyz", nil, nil)
	want("scoped readiness", ready, 200)
	if !strings.Contains(ready.Body.String(), `"inference":"scoped-fake"`) {
		t.Fatal("readiness did not distinguish scoped fake inference")
	}
	want("foreign account", request(current.Handler, http.MethodPost, "/api/drafts", other, otherCookies, input, keyHeader), 403)
	want("guest", request(current.Handler, http.MethodPost, "/api/drafts", "", nil, input, keyHeader), 401)
	want("caller key", ownerRequest(http.MethodPost, "/api/drafts", input, map[string]string{
		"X-CareerOS-Action-Key": hostedScopedDraftKey, "X-OpenAI-Api-Key": "synthetic-rejected",
	}), 400)
	if got := ownerRequest(http.MethodPost, "/api/drafts", input, map[string]string{"X-CareerOS-Action-Key": "unapproved-key"}); got.Code < 400 {
		t.Fatalf("different Action was admitted: %d", got.Code)
	}
	if got := ownerRequest(http.MethodPost, "/api/drafts", bytes.ReplaceAll(input, []byte("Java required."), []byte("Unapproved posting.")), keyHeader); got.Code < 400 {
		t.Fatalf("unapproved synthetic input accepted: %d", got.Code)
	}
	exec(`UPDATE careeros_294.careeros_control_state SET operations_binding='fixture-missing-observer' WHERE singleton`)
	want("required operations observer unavailable", ownerRequest(http.MethodGet, "/readyz", nil, nil), 503)
	if got := ownerRequest(http.MethodPost, "/api/drafts", input, keyHeader); got.Code < 400 {
		t.Fatalf("unavailable required operations observer admitted a Draft: %d", got.Code)
	}
	if hostedFixtureSession(t, current.Handler, ownerCookies) != owner {
		t.Fatal("closed Draft authority disabled independently valid Account access")
	}
	exec(`UPDATE careeros_294.careeros_control_state SET operations_binding='' WHERE singleton`)
	for _, path := range []string{"/api/qualification-gaps", "/.netlify/functions/qualification-gaps",
		"/api/profile/ingest", "/.netlify/functions/profile-ingestion", "/api/profile/review", "/.netlify/functions/profile-review",
		"/api/cv/generate", "/.netlify/functions/cv-generation", "/api/pilot/activate", "/.netlify/functions/account/api/pilot/activate"} {
		if got := ownerRequest(http.MethodPost, path, input, keyHeader); got.Code < 400 {
			t.Fatalf("non-Draft workflow available at %s: %d", path, got.Code)
		}
	}
	if sends.Load() != 0 {
		t.Fatalf("negative paths sent %d times", sends.Load())
	}
	created := ownerRequest(http.MethodPost, "/.netlify/functions/application-draft", input, keyHeader)
	if stopDuringHandoff {
		if stopDone == nil || sends.Load() != 1 {
			t.Fatalf("expected one handoff before the queued stop: %d (%d sends)", created.Code, sends.Load())
		}
		if err := <-stopDone; err != nil {
			t.Fatalf("operator stop did not complete: %v", err)
		}
		ready := ownerRequest(http.MethodGet, "/readyz", nil, nil)
		want("ready when scoped inference has stopped", ready, 200)
		if !strings.Contains(ready.Body.String(), `"inference":"closed"`) {
			t.Fatal("operator stop was not visible")
		}
		_ = ownerRequest(http.MethodPost, "/api/drafts", input, keyHeader)
		if sends.Load() != 1 {
			t.Fatalf("old Action replay sent across the winning stop: %d", sends.Load())
		}
		return
	}
	want("create scoped Draft", created, http.StatusCreated)
	var result draftrecovery.Result
	if json.Unmarshal(created.Body.Bytes(), &result) != nil || result.State != "ready" || result.Version != 1 ||
		result.Action.ID != hostedScopedDraftID || result.Bundle == nil || result.Delivered || sends.Load() != 4 {
		t.Fatalf("complete Draft and four bounded sends: %s (%d sends)", created.Body, sends.Load())
	}
	path := "/api/drafts/" + hostedScopedDraftKey + "/versions/1"
	issued := ownerRequest(http.MethodPost, path+"/opening", []byte("{}"), nil)
	want("issue opening", issued, 200)
	var receipt draftrecovery.OpeningReceipt
	if err := json.Unmarshal(issued.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	confirmation, err := json.Marshal(draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
	if err != nil {
		t.Fatal(err)
	}
	want("confirm opening", ownerRequest(http.MethodPost, path+"/opening/confirm", confirmation, nil), 200)
	want("confirm replay through alias", ownerRequest(http.MethodPost, "/.netlify/functions/account"+path+"/opening/confirm", confirmation, nil), 200)
	revision := 0
	for _, artifact := range []string{"summary", "resume", "cover", "answers"} {
		for _, operation := range []string{"review", "accept"} {
			body := fmt.Appendf(nil, `{"decisionKey":%q,"expectedRevision":%d,"bundleDigest":%q,"artifact":%q,"operation":%q}`,
				artifact+"-"+operation, revision, result.BundleDigest, artifact, operation)
			want(artifact+" "+operation, ownerRequest(http.MethodPost, path+"/review", body, nil), 200)
			revision++
		}
	}
	accepted := ownerRequest(http.MethodGet, "/api/drafts/accepted", nil, nil)
	want("accepted version", accepted, 200)
	exec(`UPDATE careeros_294.careeros_control_state SET inference_stopped=true WHERE singleton`)
	closeCurrent()
	current, closeCurrent, err = NewHostedServer(ctx, config)
	if err != nil {
		t.Fatalf("restart with stop enforced: %v", err)
	}
	defer closeCurrent()
	reopened := ownerRequest(http.MethodGet, "/api/drafts/accepted", nil, nil)
	want("accepted version after restart", reopened, 200)
	if !bytes.Equal(accepted.Body.Bytes(), reopened.Body.Bytes()) || sends.Load() != 4 {
		t.Fatalf("restart altered accepted output or sent again (%d)", sends.Load())
	}
	deletion := []byte(`{"operationId":"not-authorized-in-closed-fixture","kind":"account","profileId":"","revision":0,"confirm":true}`)
	for _, path := range []string{"/api/deletion/operations", "/api/rights/deletion/operations"} {
		if got := ownerRequest(http.MethodPost, path, deletion, nil); got.Code < 400 {
			t.Fatalf("closed fixture acknowledged an unprivileged deletion path: %s %d", path, got.Code)
		}
	}
	allowance := ownerRequest(http.MethodGet, "/api/draft-allowance", nil, nil)
	var counts draftrecovery.AllowanceStatus
	if json.Unmarshal(allowance.Body.Bytes(), &counts) != nil || counts.Consumed != 1 || counts.Reserved != 0 || counts.Remaining != 4 {
		t.Fatalf("exactly one allowance effect: %d %s", allowance.Code, allowance.Body.String())
	}
	var observed int
	var noOccupancy bool
	if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM careeros_294_draft.ai_fixture_dispatch d
JOIN careeros_294_draft.canonical_fake_attestations a ON d.namespace=a.namespace AND d.id=a.dispatch_id
WHERE d.action=$1 AND d.stage=a.stage AND d.provider=a.provider AND d.binding=a.request_digest
 AND d.state='observed' AND d.remote_state='terminal_proven'),
NOT EXISTS (SELECT 1 FROM careeros_294_draft.ai_fixture_scope WHERE occupied<>0)`, hostedScopedDraftID).
		Scan(&observed, &noOccupancy); err != nil || observed != 4 || !noOccupancy {
		t.Fatalf("four independent terminal observations and no occupancy: %d %t, %v", observed, noOccupancy, err)
	}
}

func queueHostedFixtureStop(t *testing.T, adminURL string, check *pgx.Conn) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
		defer cancel()
		connection, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			done <- err
			return
		}
		defer connection.Close(ctx)
		tx, err := connection.Begin(ctx)
		if err == nil {
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(72481312)")
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE careeros_294.careeros_control_state
SET inference_stopped=true,inference_epoch=inference_epoch+1 WHERE singleton`)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		done <- err
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := check.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks
WHERE locktype='advisory' AND classid=0 AND objid=72481312 AND objsubid=1 AND NOT granted AND mode='ExclusiveLock')`).
			Scan(&waiting); err != nil {
			t.Fatalf("observe queued stop: %v", err)
		}
		if waiting {
			return done
		}
		select {
		case <-ctx.Done():
			t.Fatal("stop never queued behind the current handoff")
		case <-ticker.C:
		}
	}
}

type fixtureAttestorTransport struct {
	host string
	base http.RoundTripper
}

func (f fixtureAttestorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.URL.Host = f.host
	return f.base.RoundTrip(request)
}
