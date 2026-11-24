package pilotruntime

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

const (
	hostedTestOrigin = "https://careeros.example.test"
	hostedTestIssuer = "https://accounts.google.com"
	hostedTestClient = "closed-fixture-client"
	fixtureRuntime   = "careeros_294_runtime"
	fixtureMigrator  = "careeros_294_migrator"
	fixturePassword  = "local-closed-fixture-only"
)

// This starts a new private localhost PostgreSQL server. The child process
// trusts its fixture certificate without changing the application's TLS code.
// No externally supplied connection string or identity enters either process.
func TestClosedHostedWithLocalTLSPostgres(t *testing.T) {
	bin := os.Getenv("CAREEROS_HOSTED_LOCAL_PG_BIN")
	if bin == "" {
		t.Skip("set CAREEROS_HOSTED_LOCAL_PG_BIN to the local PostgreSQL binary directory")
	}
	if !filepath.IsAbs(bin) {
		t.Fatal("PostgreSQL binary directory must be absolute")
	}
	root, err := os.MkdirTemp("/tmp", "ca294-")
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "pg")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	command := func(commandCtx context.Context, input string, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(commandCtx, filepath.Join(bin, name), args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C"}
		cmd.Stdin = bytes.NewBufferString(input)
		return cmd.CombinedOutput()
	}
	run := func(input string, name string, args ...string) {
		t.Helper()
		if output, e := command(ctx, input, name, args...); e != nil {
			t.Fatalf("local %s: %v\n%s", name, e, output)
		}
	}
	startAttempted := false
	t.Cleanup(func() {
		if startAttempted {
			stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if _, err := os.Stat(filepath.Join(data, "postmaster.pid")); !errors.Is(err, os.ErrNotExist) {
				if output, e := command(stopCtx, "", "pg_ctl", "-D", data, "-m", "fast", "-w", "stop"); e != nil {
					t.Errorf("retained local fixture at %s because shutdown was unverified: %v\n%s", root, e, output)
					return
				}
			}
		}
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})

	run("", "initdb", "-D", data, "--username=fixture_admin", "--no-locale", "--encoding=UTF8", "--auth-local=trust", "--auth-host=reject")
	certificate := hostedFixtureCertificate(t, data)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	writeHostedFixture(t, filepath.Join(data, "postgresql.auto.conf"), []byte(fmt.Sprintf(
		"listen_addresses = '127.0.0.1'\nport = %s\nunix_socket_directories = '%s'\nssl = on\nssl_min_protocol_version = 'TLSv1.2'\n", port, root)))
	writeHostedFixture(t, filepath.Join(data, "pg_hba.conf"), []byte("local all fixture_admin trust\nhostssl careeros_294 all 127.0.0.1/32 scram-sha-256\n"))
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	startAttempted = true
	run("", "pg_ctl", "-D", data, "-l", filepath.Join(root, "postgres.log"), "-w", "start")
	roleSQL := fmt.Sprintf(`
CREATE ROLE %[1]s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 8 PASSWORD '%[3]s';
CREATE ROLE %[2]s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 2 PASSWORD '%[3]s';
CREATE DATABASE careeros_294;
`, fixtureRuntime, fixtureMigrator, fixturePassword)
	psql := []string{"-X", "-v", "ON_ERROR_STOP=1", "-h", root, "-p", port, "-U", "fixture_admin", "-d"}
	run(roleSQL, "psql", append(psql, "postgres")...)
	run(accountstore.HostedPilotBootstrapSQL(), "psql", append(psql, "careeros_294")...)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(ctx, executable, "-test.run=^TestClosedHostedTLSFixtureChild$", "-test.v")
	child.Env = []string{"CAREEROS_HOSTED_TLS_FIXTURE_PORT=" + port, "SSL_CERT_FILE=" + certificate}
	output, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("local hosted integration: %v\n%s", err, output)
	}
	t.Logf("local hosted integration child\n%s", output)
}

func writeHostedFixture(t *testing.T, path string, value []byte) {
	t.Helper()
	if err := os.WriteFile(path, value, 0600); err != nil {
		t.Fatal(err)
	}
}

func hostedFixtureCertificate(t *testing.T, directory string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		BasicConstraintsValid: true, IsCA: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := filepath.Join(directory, "server.crt")
	writeHostedFixture(t, certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	writeHostedFixture(t, filepath.Join(directory, "server.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	return certificate
}

func TestClosedHostedTLSFixtureChild(t *testing.T) {
	port := os.Getenv("CAREEROS_HOSTED_TLS_FIXTURE_PORT")
	if port == "" {
		t.Skip("requires the isolated TLS fixture started by TestClosedHostedWithLocalTLSPostgres")
	}
	// Only the private certificate and literal localhost are accepted by this
	// test helper, even if the child test is invoked by itself.
	fixtureCert := regexp.MustCompile(`^/tmp/ca294-[0-9]+/pg/server\.crt$`)
	if !fixtureCert.MatchString(os.Getenv("SSL_CERT_FILE")) {
		t.Fatal("fixture CA path invalid")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1024 || number > 65535 {
		t.Fatal("fixture port invalid")
	}
	dsn := func(role string) string {
		return (&url.URL{Scheme: "postgresql", User: url.UserPassword(role, fixturePassword),
			Host: net.JoinHostPort("localhost", port), Path: "/careeros_294", RawQuery: "sslmode=verify-full"}).String()
	}
	if err = accountstore.MigrateHostedPilot(t.Context(), dsn(fixtureMigrator)); err != nil {
		t.Fatalf("closed Account migration: %v", err)
	}
	provider := newHostedFixtureGoogle(t)
	config := HostedConfig{
		Assets:      fstest.MapFS{"index.html": {Data: []byte("<html>closed hosted fixture</html>")}},
		DatabaseURL: dsn(fixtureRuntime),
		GoogleSubjects: `[{"issuer":"https://accounts.google.com","subject":"fixture-A"},` +
			`{"issuer":"https://accounts.google.com","subject":"fixture-B"}]`,
		Auth: auth.Config{
			PublicOrigin: hostedTestOrigin, SessionTTL: time.Hour, TransactionTTL: 5 * time.Minute,
			HTTPClient: &http.Client{Transport: provider},
			Providers:  []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(hostedTestOrigin, hostedTestClient, "inert-fixture-secret")},
		},
	}
	open := func() (*http.Server, func()) {
		t.Helper()
		server, closeStore, e := NewHostedServer(t.Context(), config)
		if e != nil {
			t.Fatal(e)
		}
		return server, closeStore
	}
	server, closeStore := open()
	defer closeStore()
	if got := hostedFixtureRequest(server.Handler, http.MethodGet, "/readyz", nil, nil); got.Code != http.StatusOK {
		t.Fatalf("initial readiness = %d %s", got.Code, got.Body.String())
	}
	checkHostedFixtureGrantDrift(t, server.Handler, dsn(fixtureMigrator))
	for _, test := range []struct {
		subject        string
		withoutBinding bool
	}{
		{subject: "fixture-not-listed"},
		{subject: "fixture-interrupted", withoutBinding: true},
	} {
		result := provider.login(t, server.Handler, test.subject, test.withoutBinding)
		if result.StatusCode < 400 || hostedFixtureCookie(result, "__Host-careeros_session") != nil {
			t.Fatalf("rejected callback %s issued a session: %d", test.subject, result.StatusCode)
		}
	}
	a := hostedFixtureLogin(t, provider, server.Handler, "fixture-A")
	b := hostedFixtureLogin(t, provider, server.Handler, "fixture-B")
	accountA := hostedFixtureSession(t, server.Handler, a)
	accountB := hostedFixtureSession(t, server.Handler, b)
	if accountA == "" || accountB == "" || accountB == accountA {
		t.Fatal("the two authorized sessions did not resolve to two separate Accounts")
	}
	if got := hostedFixtureRequest(server.Handler, http.MethodGet, "/", nil, nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "closed hosted fixture") {
		t.Fatalf("workspace response = %d", got.Code)
	}
	for _, route := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion",
		"/api/profile/review", "/.netlify/functions/profile-review", "/api/cv/generate", "/.netlify/functions/cv-generation",
		"/api/qualification-gaps", "/.netlify/functions/qualification-gaps", "/api/application-draft",
		"/.netlify/functions/application-draft", "/api/drafts", "/api/pilot/activate"} {
		got := hostedFixtureRequest(server.Handler, http.MethodPost, route, a, map[string]string{
			"X-CareerOS-Account": accountA, "X-API-Key": "inert-caller-key",
		})
		if got.Code != http.StatusForbidden && got.Code != http.StatusServiceUnavailable {
			t.Fatalf("closed route %s = %d %s", route, got.Code, got.Body.String())
		}
		if got = hostedFixtureRequest(server.Handler, http.MethodPost, route, nil, nil); got.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous route %s = %d %s", route, got.Code, got.Body.String())
		}
	}
	for _, account := range []string{accountA, accountB} {
		got := hostedFixtureRequest(server.Handler, http.MethodGet, "/api/profile/current", a, map[string]string{
			"X-CareerOS-Account": account,
		})
		if account == accountA && got.Code != http.StatusForbidden || account == accountB && got.Code != http.StatusConflict {
			t.Fatalf("protected Profile read = %d %s", got.Code, got.Body.String())
		}
	}
	fixtures, err := os.ReadFile("../profiledocument/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var profiles []struct{ Document json.RawMessage }
	if err := json.Unmarshal(fixtures, &profiles); err != nil || len(profiles) == 0 {
		t.Fatal("Profile fixture unavailable", err)
	}
	put := httptest.NewRequest(http.MethodPut, hostedTestOrigin+"/api/profile/current",
		io.MultiReader(strings.NewReader(`{"operationId":"closed-fixture-save","expectedRevision":0,"document":`),
			bytes.NewReader(profiles[0].Document), strings.NewReader(`}`)))
	put.Header.Set("Origin", hostedTestOrigin)
	put.Header.Set("X-CareerOS-Account", accountA)
	for _, cookie := range a {
		put.AddCookie(cookie)
		if cookie.Name == "__Host-careeros_csrf" {
			put.Header.Set("X-CSRF-Token", cookie.Value)
		}
	}
	putResult := httptest.NewRecorder()
	server.Handler.ServeHTTP(putResult, put)
	if putResult.Code != http.StatusForbidden {
		t.Fatalf("ungranted Profile save = %d %s", putResult.Code, putResult.Body.String())
	}
	closeStore()
	if got := hostedFixtureRequest(server.Handler, http.MethodGet, "/readyz", nil, nil); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness on lost dependency = %d", got.Code)
	}
	server, closeStore = open()
	defer closeStore()
	if hostedFixtureSession(t, server.Handler, a) != accountA || hostedFixtureSession(t, server.Handler, b) != accountB {
		t.Fatal("Account sessions changed across a runtime reopen")
	}
	if got := hostedFixtureRequest(server.Handler, http.MethodPost, "/api/auth/logout-all", a, map[string]string{
		"Origin": "https://wrong-origin.example.test",
	}); got.Code != http.StatusForbidden {
		t.Fatalf("wrong-origin logout = %d", got.Code)
	}
	if got := hostedFixtureRequest(server.Handler, http.MethodPost, "/api/auth/logout-all", a, map[string]string{
		"X-CSRF-Token": "wrong-csrf",
	}); got.Code != http.StatusForbidden {
		t.Fatalf("wrong-CSRF logout = %d", got.Code)
	}
	if got := hostedFixtureRequest(server.Handler, http.MethodPost, "/api/auth/logout-all", a, nil); got.Code != http.StatusNoContent {
		t.Fatalf("approved logout-all = %d %s", got.Code, got.Body.String())
	}
	if hostedFixtureSession(t, server.Handler, a) != "" || hostedFixtureSession(t, server.Handler, b) != accountB {
		t.Fatal("one Account's logout-all changed the other Account session or retained its own")
	}
	if provider.exchanges != 3 || provider.unexpected != 0 {
		t.Fatalf("synthetic token exchanges = %d, unexpected outbound requests = %d", provider.exchanges, provider.unexpected)
	}
}

func checkHostedFixtureGrantDrift(t *testing.T, handler http.Handler, dsn string) {
	t.Helper()
	migrator, err := pgx.Connect(t.Context(), dsn+"&sslrootcert="+url.QueryEscape(os.Getenv("SSL_CERT_FILE")))
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close(t.Context())
	for _, step := range []struct {
		statement string
		status    int
	}{
		{"REVOKE INSERT ON careeros_294.careeros_sessions FROM careeros_294_runtime", http.StatusServiceUnavailable},
		{"GRANT INSERT ON careeros_294.careeros_sessions TO careeros_294_runtime", http.StatusOK},
		{"GRANT UPDATE ON careeros_294.careeros_operations_roles TO careeros_294_runtime", http.StatusServiceUnavailable},
		{"REVOKE UPDATE ON careeros_294.careeros_operations_roles FROM careeros_294_runtime", http.StatusOK},
	} {
		if _, err := migrator.Exec(t.Context(), step.statement); err != nil {
			t.Fatal(err)
		}
		if got := hostedFixtureRequest(handler, http.MethodGet, "/readyz", nil, nil); got.Code != step.status {
			t.Fatalf("local fixture readiness after %q = %d, want %d", step.statement, got.Code, step.status)
		}
	}
}

func hostedFixtureRequest(handler http.Handler, method, path string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, hostedTestOrigin+path, nil)
	request.Header.Set("Origin", hostedTestOrigin)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
		if cookie.Name == "__Host-careeros_csrf" {
			request.Header.Set("X-CSRF-Token", cookie.Value)
		}
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	return result
}

func hostedFixtureCookie(result *http.Response, name string) *http.Cookie {
	for _, cookie := range result.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func hostedFixtureLogin(t *testing.T, provider *hostedFixtureGoogle, handler http.Handler, subject string) []*http.Cookie {
	t.Helper()
	result := provider.login(t, handler, subject, false)
	if result.StatusCode != http.StatusSeeOther || result.Header.Get("Location") != "/" {
		t.Fatalf("successful callback = %d, location %q", result.StatusCode, result.Header.Get("Location"))
	}
	session := hostedFixtureCookie(result, "__Host-careeros_session")
	csrf := hostedFixtureCookie(result, "__Host-careeros_csrf")
	for _, cookie := range []*http.Cookie{session, csrf} {
		if cookie == nil || cookie.Path != "/" || cookie.Domain != "" || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatal("callback did not issue both host-only secure cookies")
		}
	}
	if !session.HttpOnly || csrf.HttpOnly {
		t.Fatal("unexpected callback cookie HttpOnly policies")
	}
	return []*http.Cookie{session, csrf}
}

func hostedFixtureSession(t *testing.T, handler http.Handler, cookies []*http.Cookie) string {
	t.Helper()
	got := hostedFixtureRequest(handler, http.MethodGet, "/api/auth/session", cookies, nil)
	var session auth.SessionView
	if got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &session) != nil {
		t.Fatalf("session probe = %d %s", got.Code, got.Body.String())
	}
	if !session.Authenticated || session.Account == nil {
		return ""
	}
	return session.Account.ID
}

type hostedFixtureGoogle struct {
	key        *rsa.PrivateKey
	authorize  url.Values
	exchanges  int
	unexpected int
}

func newHostedFixtureGoogle(t *testing.T) *hostedFixtureGoogle {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &hostedFixtureGoogle{key: key}
}

func (g *hostedFixtureGoogle) login(t *testing.T, handler http.Handler, subject string, withoutBinding bool) *http.Response {
	t.Helper()
	start := hostedFixtureRequest(handler, http.MethodGet, "/api/auth/google/start?return_to=/", nil, nil).Result()
	defer start.Body.Close()
	location, err := start.Location()
	if err != nil || start.StatusCode != http.StatusFound || location.Host != "accounts.google.com" {
		t.Fatalf("authorization start = %d", start.StatusCode)
	}
	g.authorize = location.Query()
	if g.authorize.Get("redirect_uri") != hostedTestOrigin+"/api/auth/google/callback" || g.authorize.Get("code_challenge_method") != "S256" {
		t.Fatal("authorization start has incorrect redirect or PKCE")
	}
	var cookies []*http.Cookie
	if !withoutBinding {
		cookie := hostedFixtureCookie(start, "__Host-careeros_oidc")
		if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.Path != "/" {
			t.Fatal("authorization start did not issue secure binding cookie")
		}
		cookies = []*http.Cookie{cookie}
	}
	query := url.Values{"state": {g.authorize.Get("state")}, "code": {subject}}
	return hostedFixtureRequest(handler, http.MethodGet, "/api/auth/google/callback?"+query.Encode(), cookies, nil).Result()
}

func (g *hostedFixtureGoogle) RoundTrip(r *http.Request) (*http.Response, error) {
	var body any
	switch r.URL.String() {
	case auth.DefaultGoogleProviderConfig(hostedTestOrigin, hostedTestClient, "").JWKSURL:
		body = map[string]any{"keys": []any{map[string]string{
			"alg": "RS256", "kty": "RSA", "kid": "fixture-key", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(g.key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(g.key.E)).Bytes()),
		}}}
	case "https://oauth2.googleapis.com/token":
		g.exchanges++
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		proof := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("redirect_uri") != g.authorize.Get("redirect_uri") ||
			base64.RawURLEncoding.EncodeToString(proof[:]) != g.authorize.Get("code_challenge") {
			return nil, errors.New("synthetic token exchange rejected binding")
		}
		now := time.Now()
		claims, err := json.Marshal(map[string]any{
			"iss": hostedTestIssuer, "sub": r.Form.Get("code"), "aud": hostedTestClient,
			"nonce": g.authorize.Get("nonce"), "iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
		})
		if err != nil {
			return nil, err
		}
		input := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"fixture-key"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
		digest := sha256.Sum256([]byte(input))
		signature, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, digest[:])
		if err != nil {
			return nil, err
		}
		body = map[string]string{"id_token": input + "." + base64.RawURLEncoding.EncodeToString(signature)}
	default:
		g.unexpected++
		return nil, errors.New("unexpected outbound request from closed hosted candidate")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
}
