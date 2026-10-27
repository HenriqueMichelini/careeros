package accountapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

const origin = "http://127.0.0.1:8787"

type fakeOIDC struct{ before func() }

func (p fakeOIDC) ID() string { return "google" }
func (p fakeOIDC) AuthorizationURL(state, nonce, challenge, redirect string) string {
	return "https://synthetic.invalid/authorize?state=" + url.QueryEscape(state)
}
func (p fakeOIDC) Exchange(_ context.Context, code, verifier, redirect, nonce string) (auth.Identity, error) {
	if p.before != nil {
		p.before()
	}
	return auth.Identity{Provider: "google", Issuer: "https://accounts.google.com", Subject: code, Email: "synthetic@example.invalid", Name: "Synthetic", EmailVerified: true}, nil
}
func service(t *testing.T, s *accountstore.Store, p fakeOIDC) *auth.Service {
	t.Helper()
	cfg := auth.Config{PublicOrigin: origin, SessionTTL: time.Hour, TransactionTTL: time.Minute, Providers: []auth.ProviderConfig{auth.DefaultGoogleProviderConfig(origin, "synthetic-client", "synthetic-secret")}}
	registry, err := auth.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.NewService(cfg, registry, s.AuthStores())
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func database(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("CAREEROS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CAREEROS_TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("careeros_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	db, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return parsed.String(), db
}
func open(t *testing.T, dsn, name string) *accountstore.Store {
	t.Helper()
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("application_name", name)
	u.RawQuery = q.Encode()
	s, err := accountstore.Open(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func login(t *testing.T, s *auth.Service, subject string) *auth.LoginResult {
	t.Helper()
	start, err := s.BeginLogin(context.Background(), "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	v, err := s.CompleteLogin(context.Background(), "google", u.Query().Get("state"), subject, start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	return &v
}
func fixture(t *testing.T, revision int64) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "profiledocument", "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Document map[string]any }
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	doc := fixtures[0].Document
	doc["revision"] = revision
	fact := doc["facts"].([]any)[0].(map[string]any)
	fact["owner"].(map[string]any)["revision"] = revision
	// Opaque finite values, NUL, supplementary Unicode and original trust labels.
	fact["field"] = "custom"
	fact["value"] = map[string]any{"wording": "ação\x00😀", "max": float64(9007199254740991), "large": 1e308, "small": 5e-324, "nested": []any{nil, false, "e\u0301"}}
	raw, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func api(t *testing.T, h http.Handler, who *auth.LoginResult, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if who != nil {
		r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: who.SessionToken})
		r.AddCookie(&http.Cookie{Name: "__Host-careeros_csrf", Value: who.CSRFToken})
		r.Header.Set("X-CSRF-Token", who.CSRFToken)
		r.Header.Set("X-CareerOS-Account", who.Account.ID)
	}
	r.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func command(op string, base int64, doc []byte) []byte {
	v, _ := json.Marshal(struct {
		OperationID      string          `json:"operationId"`
		ExpectedRevision int64           `json:"expectedRevision"`
		Document         json.RawMessage `json:"document"`
	}{op, base, doc})
	return v
}

type profileScenario struct {
	db   *sql.DB
	a    *accountstore.Store
	sa   *auth.Service
	ha   http.Handler
	hb   http.Handler
	one  *auth.LoginResult
	two  *auth.LoginResult
	doc1 []byte
	doc2 []byte
	doc3 []byte
}

func TestPostgresProfileOwnershipConcurrencyAndRecovery(t *testing.T) {
	dsn, db := database(t)
	a := open(t, dsn, "careeros_instance_a")
	if err := a.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	b := open(t, dsn, "careeros_instance_b")
	sa, sb := service(t, a, fakeOIDC{}), service(t, b, fakeOIDC{})
	ha, hb := New(sa, a), New(sb, b)
	one, two := login(t, sa, "account-a"), login(t, sb, "account-b")
	if one.Account.ID == two.Account.ID {
		t.Fatal("same-email subjects were linked")
	}
	for _, who := range []*auth.LoginResult{one, two} {
		if w := api(t, hb, who, "GET", "/api/profile/current", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"document":null`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	doc1, doc2, doc3 := fixture(t, 1), fixture(t, 2), fixture(t, 3)
	if w := api(t, ha, one, "PUT", "/api/profile/current", command("op-one", 0, doc1)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := api(t, hb, one, "GET", "/api/profile/current", nil)
	var saved accountstore.ProfileResult
	json.Unmarshal(w.Body.Bytes(), &saved)
	var wanted, got any
	json.Unmarshal(doc1, &wanted)
	json.Unmarshal(saved.Document, &got)
	if fmt.Sprint(wanted) != fmt.Sprint(got) {
		t.Fatal("representation/trust changed")
	}
	var raw []byte
	if err := db.QueryRow("SELECT document FROM careeros_profiles WHERE account_id=$1", one.Account.ID).Scan(&raw); err != nil || !bytes.Equal(raw, doc1) {
		t.Fatal("raw bytes did not round trip", err)
	}
	s := profileScenario{db: db, a: a, sa: sa, ha: ha, hb: hb, one: one, two: two, doc1: doc1, doc2: doc2, doc3: doc3}
	t.Run("direct isolation and CSRF", s.directIsolation)
	t.Run("concurrent saves and duplicates", s.concurrentSaves)
	t.Run("invalid representation and retained data", s.invalidRepresentation)
	t.Run("expiry status and revocation", s.revocation)
	t.Run("outage preserves session and fails closed", s.outage)
}

func (s *profileScenario) directIsolation(t *testing.T) {
	if w := api(t, s.hb, s.two, "GET", "/api/profile/operations/op-one", nil); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := api(t, s.hb, s.two, "PUT", "/api/profile/current", command("substitute-id", 1, s.doc2)); w.Code != 409 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "/api/profile/current", nil)
	r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: s.one.SessionToken})
	r.Header.Set("X-CareerOS-Account", s.two.Account.ID)
	w := httptest.NewRecorder()
	s.ha.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("PUT", "/api/profile/current", bytes.NewReader(command("no-csrf", 1, s.doc2)))
	r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: s.one.SessionToken})
	r.Header.Set("Origin", origin)
	w = httptest.NewRecorder()
	s.ha.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := api(t, s.ha, nil, "GET", "/api/profile/current", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	// B can create its own document even with A's domain Profile ID: IDs are not authority.
	if w := api(t, s.hb, s.two, "PUT", "/api/profile/current", command("own-b", 0, fixture(t, 1))); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func (s *profileScenario) concurrentSaves(t *testing.T) {
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i, h := range []http.Handler{s.ha, s.hb} {
		wg.Add(1)
		go func(i int, h http.Handler) {
			defer wg.Done()
			codes <- api(t, h, s.one, "PUT", "/api/profile/current", command(fmt.Sprintf("concurrent-%d", i), 1, s.doc2)).Code
		}(i, h)
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for c := range codes {
		counts[c]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal(counts)
	}
	// The identical operation sent concurrently through both instances has
	// one publication and two recoveries, even after its base becomes stale.
	var winning string
	if err := s.db.QueryRow("SELECT operation_id FROM careeros_profile_operations WHERE account_id=$1 AND committed_revision=2", s.one.Account.ID).Scan(&winning); err != nil {
		t.Fatal(err)
	}
	duplicates := make(chan int, 2)
	for _, h := range []http.Handler{s.ha, s.hb} {
		wg.Add(1)
		go func(h http.Handler) {
			defer wg.Done()
			duplicates <- api(t, h, s.one, "PUT", "/api/profile/current", command(winning, 1, s.doc2)).Code
		}(h)
	}
	wg.Wait()
	close(duplicates)
	for code := range duplicates {
		if code != 200 {
			t.Fatal(code)
		}
	}
	if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", command("third", 2, s.doc3)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	// A lost acknowledgment is reconciled by its historical receipt, not by rewriting revision 1.
	w := api(t, s.hb, s.one, "PUT", "/api/profile/current", command("op-one", 0, s.doc1))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var reply accountstore.ProfileResult
	json.Unmarshal(w.Body.Bytes(), &reply)
	if reply.Outcome.CommittedRevision != 1 || !bytes.Contains(reply.Document, []byte(`"revision":3`)) {
		t.Fatal(w.Body.String())
	}
	if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", command("op-one", 0, fixture(t, 2))); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", command("new-stale", 1, s.doc2)); w.Code != 409 {
		t.Fatal(w.Code)
	}
}

func (s *profileScenario) invalidRepresentation(t *testing.T) {
	oversize := bytes.Replace(fixture(t, 4), []byte("ação"), []byte(strings.Repeat("x", accountstore.MaxDocumentBytes)), 1)
	if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", command("oversize", 3, oversize)); w.Code != 422 {
		t.Fatal("size bound", w.Code)
	}
	injected := append([]byte(`{"accountId":"`+s.two.Account.ID+`",`), command("client-owner", 3, fixture(t, 4))[1:]...)
	if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", injected); w.Code != 422 {
		t.Fatal("accepted client owner", w.Code)
	}
	for _, bad := range [][]byte{bytes.Replace(fixture(t, 4), []byte(`ação`), []byte(`\ud800`), 1), bytes.Replace(fixture(t, 4), []byte(`1e+308`), []byte(`1e400`), 1), bytes.Replace(fixture(t, 4), []byte(`ação`), []byte{0xff}, 1), bytes.Repeat([]byte(" "), accountstore.MaxDocumentBytes+1)} {
		if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", command("invalid", 3, bad)); w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var d map[string]any
	json.Unmarshal(fixture(t, 4), &d)
	d["facts"] = []any{}
	d["links"] = []any{}
	bad, _ := json.Marshal(d)
	if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", command("orphan", 3, bad)); w.Code != 422 {
		t.Fatal("retained unused excerpt", w.Code)
	}
	d["evidence"] = []any{}
	clean, _ := json.Marshal(d)
	if w := api(t, s.ha, s.one, "PUT", "/api/profile/current", command("delete-fact", 3, clean)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func (s *profileScenario) revocation(t *testing.T) {
	for _, status := range []string{"suspended", "deleted"} {
		s.db.Exec("UPDATE careeros_accounts SET status=$1 WHERE id=$2", status, s.one.Account.ID)
		if w := api(t, s.ha, s.one, "GET", "/api/profile/current", nil); w.Code != 401 {
			t.Fatal(status, w.Code)
		}
		s.db.Exec("UPDATE careeros_accounts SET status='active' WHERE id=$1", s.one.Account.ID)
	}
	s.db.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", s.one.Principal.SessionID)
	if w := api(t, s.hb, s.one, "GET", "/api/profile/current", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	fresh := login(t, s.sa, "account-a")
	if _, err := s.sa.LogoutAll(context.Background(), fresh.Principal); err != nil {
		t.Fatal(err)
	}
	if w := api(t, s.hb, fresh, "GET", "/api/profile/current", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := api(t, s.hb, s.two, "GET", "/api/profile/current", nil); w.Code != 200 {
		t.Fatal("other account denied", w.Code)
	}
}

func (s *profileScenario) outage(t *testing.T) {
	s.a.Close()
	w := api(t, s.ha, s.two, "PUT", "/api/profile/current", command("outage", 1, s.doc2))
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = api(t, s.ha, s.two, "GET", "/api/auth/session", nil)
	if w.Code != 503 || len(w.Result().Cookies()) != 0 {
		t.Fatal("outage was reported as logout", w.Code, w.Body.String())
	}
	if w := api(t, s.hb, s.two, "GET", "/api/profile/current", nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func waitBlocked(t *testing.T, db *sql.DB, name, event string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		err := db.QueryRow("SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND ($2='' OR wait_event=$2)", name, event).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("operation did not reach its deterministic database lock", name, event)
}
func TestPostgresRevocationVersusCommitAndLogin(t *testing.T) {
	for _, kind := range []string{"ordinary", "generation", "suspended", "deleted", "expiry"} {
		t.Run(kind+" wins", func(t *testing.T) {
			assertRevocationWins(t, kind)
		})
	}
	t.Run("commit wins before ordinary logout", assertCommitBeforeLogout)
	t.Run("ordinary logout fences late provider response", assertLogoutFencesProvider)
}

// Separate application processes, same engine. The only OIDC response source
// is the synthetic provider above; no Google endpoint is contacted.
func TestAccountProcessHelper(t *testing.T) {
	if os.Getenv("CAREEROS_TEST_PROCESS") != "1" {
		return
	}
	s, err := accountstore.Open(context.Background(), os.Getenv("CAREEROS_TEST_PROCESS_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	svc := service(t, s, fakeOIDC{})
	server := httptest.NewServer(New(svc, s))
	defer server.Close()
	fmt.Println(server.URL)
	io.Copy(io.Discard, os.Stdin)
}

type process struct {
	url  string
	stop func()
}

func startProcess(t *testing.T, dsn string) process {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestAccountProcessHelper$")
	cmd.Env = append(os.Environ(), "CAREEROS_TEST_PROCESS=1", "CAREEROS_TEST_PROCESS_DSN="+dsn)
	input, _ := cmd.StdinPipe()
	output, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scan := bufio.NewScanner(output)
	if !scan.Scan() {
		t.Fatal("child did not start", stderr.String())
	}
	url := scan.Text()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			input.Close()
			if e := cmd.Wait(); e != nil {
				t.Error(e, stderr.String())
			}
		})
	}
	t.Cleanup(stop)
	return process{url, stop}
}
func request(t *testing.T, base, method, path string, body []byte, cookies []*http.Cookie, account string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Origin", origin)
	r.Header.Set("X-CareerOS-Account", account)
	for _, c := range cookies {
		r.AddCookie(c)
		if c.Name == "__Host-careeros_csrf" {
			r.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	c := http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func TestPostgresSurvivesAllApplicationProcessRestarts(t *testing.T) {
	dsn, _ := database(t)
	s := open(t, dsn, "careeros_restart_setup")
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Close()
	a, b := startProcess(t, dsn), startProcess(t, dsn)
	start := request(t, a.url, "GET", "/api/auth/google/start", nil, nil, "")
	u, _ := url.Parse(start.Header.Get("Location"))
	binding := start.Cookies()
	start.Body.Close()
	signed := request(t, b.url, "GET", "/api/auth/google/callback?code=restart-subject&state="+url.QueryEscape(u.Query().Get("state")), nil, binding, "")
	cookies := signed.Cookies()
	signed.Body.Close()
	if signed.StatusCode != 303 {
		t.Fatal(signed.StatusCode)
	}
	session := request(t, a.url, "GET", "/api/auth/session", nil, cookies, "")
	var view auth.SessionView
	json.NewDecoder(session.Body).Decode(&view)
	session.Body.Close()
	if !view.Authenticated {
		t.Fatal("session did not cross processes")
	}
	doc := fixture(t, 1)
	saved := request(t, b.url, "PUT", "/api/profile/current", command("restart-save", 0, doc), cookies, view.Account.ID)
	io.Copy(io.Discard, saved.Body)
	saved.Body.Close()
	if saved.StatusCode != 200 {
		t.Fatal(saved.StatusCode)
	}
	// Leave a second one-use transaction pending across the complete restart.
	pending := request(t, a.url, "GET", "/api/auth/google/start", nil, nil, "")
	pendingURL, _ := url.Parse(pending.Header.Get("Location"))
	pendingCookies := pending.Cookies()
	pending.Body.Close()
	a.stop()
	b.stop() // ALL application processes are now gone, DB remains.
	c, d := startProcess(t, dsn), startProcess(t, dsn)
	resumed := request(t, d.url, "GET", "/api/auth/google/callback?code=restart-subject&state="+url.QueryEscape(pendingURL.Query().Get("state")), nil, pendingCookies, "")
	resumed.Body.Close()
	if resumed.StatusCode != 303 {
		t.Fatal("pending transaction did not survive restart", resumed.StatusCode)
	}
	for _, p := range []process{c, d} {
		response := request(t, p.url, "GET", "/api/profile/operations/restart-save", nil, cookies, view.Account.ID)
		var result accountstore.ProfileResult
		err := json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || result.AccountID != view.Account.ID || result.Outcome == nil || result.Outcome.CommittedRevision != 1 {
			t.Fatal(response.StatusCode, result, err)
		}
		var a, b any
		json.Unmarshal(doc, &a)
		json.Unmarshal(result.Document, &b)
		if fmt.Sprint(a) != fmt.Sprint(b) {
			t.Fatal("profile changed after process restart")
		}
	}
	// One-use transactions also survive restart and cannot be consumed twice.
	replay := request(t, c.url, "GET", "/api/auth/google/callback?code=restart-subject&state="+url.QueryEscape(u.Query().Get("state")), nil, binding, "")
	replay.Body.Close()
	if replay.StatusCode != 400 {
		t.Fatal(replay.StatusCode)
	}
	logout := request(t, c.url, "POST", "/api/auth/logout", nil, cookies, view.Account.ID)
	logout.Body.Close()
	if logout.StatusCode != 204 {
		t.Fatal(logout.StatusCode)
	}
	denied := request(t, d.url, "GET", "/api/profile/current", nil, cookies, view.Account.ID)
	denied.Body.Close()
	if denied.StatusCode != 401 {
		t.Fatal(denied.StatusCode)
	}
}

func TestPostgresOneUseTransactionsAndAtomicSessionReplacement(t *testing.T) {
	dsn, db := database(t)
	s := open(t, dsn, "careeros_session_replace")
	s.Migrate(context.Background())
	other := open(t, dsn, "careeros_other_session")
	svc := service(t, s, fakeOIDC{})
	who := login(t, svc, "subject")
	another := login(t, svc, "subject")
	start, err := svc.BeginLogin(context.Background(), "google", "/", &who.Principal)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	state := sha256.Sum256([]byte(u.Query().Get("state")))
	var count int
	var wg sync.WaitGroup
	found := make(chan bool, 2)
	for _, store := range []*accountstore.Store{s, other} {
		wg.Add(1)
		go func(s *accountstore.Store) {
			defer wg.Done()
			_, ok, e := s.AuthStores().Transactions.Consume(context.Background(), state, time.Now())
			if e != nil {
				t.Error(e)
			}
			found <- ok
		}(store)
	}
	wg.Wait()
	close(found)
	for ok := range found {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
	// Stop Create at its account-lock boundary, then revoke the initiating session.
	tx, _ := db.Begin()
	defer tx.Rollback()
	tx.Exec("SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE", who.Account.ID)
	replacement := auth.Session{ID: "replacement", AccountID: who.Account.ID, Generation: who.Principal.Generation, TokenHash: sha256.Sum256([]byte("replacement-token")), CSRFHash: sha256.Sum256([]byte("replacement-csrf")), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	done := make(chan error, 1)
	go func() {
		done <- s.AuthStores().Sessions.Create(context.Background(), replacement, who.Principal.SessionID, time.Now())
	}()
	waitBlocked(t, db, "careeros_session_replace", "")
	tx.Exec("DELETE FROM careeros_sessions WHERE id=$1", who.Principal.SessionID)
	tx.Commit()
	if err = <-done; !errors.Is(err, auth.ErrInvalidTransaction) {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(context.Background(), another.SessionToken); err != nil {
		t.Fatal(err)
	}
	login(t, svc, "subject")
}

func TestPostgresRollbackAndActualLostHTTPResponse(t *testing.T) {
	dsn, db := database(t)
	s := open(t, dsn, "careeros_lost_response")
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, s, fakeOIDC{})
	who := login(t, svc, "loss-subject")
	h := New(svc, s)
	// Failure AFTER the Profile statement must roll back both tables.
	_, err := db.Exec(`CREATE FUNCTION fail_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic storage failure'; END $$; CREATE TRIGGER fail_receipt BEFORE INSERT ON careeros_profile_operations FOR EACH ROW EXECUTE FUNCTION fail_receipt()`)
	if err != nil {
		t.Fatal(err)
	}
	body := command("lost-http", 0, fixture(t, 1))
	if w := api(t, h, who, "PUT", "/api/profile/current", body); w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
	var profiles, receipts int
	db.QueryRow("SELECT count(*) FROM careeros_profiles").Scan(&profiles)
	db.QueryRow("SELECT count(*) FROM careeros_profile_operations").Scan(&receipts)
	if profiles != 0 || receipts != 0 {
		t.Fatal("partial commit", profiles, receipts)
	}
	if _, err = db.Exec("DROP TRIGGER fail_receipt ON careeros_profile_operations"); err != nil {
		t.Fatal(err)
	}
	// Close the actual TCP connection ONLY AFTER the handler committed, before
	// any HTTP acknowledgment reaches its caller. No mock database is involved.
	loss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture := httptest.NewRecorder()
		h.ServeHTTP(capture, r)
		if capture.Code != 200 {
			t.Errorf("commit before loss failed: %d %s", capture.Code, capture.Body.String())
		}
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			t.Error(e)
			return
		}
		conn.Close()
	}))
	defer loss.Close()
	r, _ := http.NewRequest("PUT", loss.URL+"/api/profile/current", bytes.NewReader(body))
	r.Header.Set("Origin", origin)
	r.Header.Set("X-CareerOS-Account", who.Account.ID)
	r.Header.Set("X-CSRF-Token", who.CSRFToken)
	r.AddCookie(&http.Cookie{Name: "__Host-careeros_session", Value: who.SessionToken})
	r.AddCookie(&http.Cookie{Name: "__Host-careeros_csrf", Value: who.CSRFToken})
	if response, e := http.DefaultClient.Do(r); e == nil {
		response.Body.Close()
		t.Fatal("HTTP acknowledgment unexpectedly arrived")
	}
	for i := 0; i < 2; i++ {
		w := api(t, h, who, "GET", "/api/profile/operations/lost-http", nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		w = api(t, h, who, "PUT", "/api/profile/current", body)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	db.QueryRow("SELECT count(*) FROM careeros_profiles").Scan(&profiles)
	db.QueryRow("SELECT count(*) FROM careeros_profile_operations").Scan(&receipts)
	if profiles != 1 || receipts != 1 {
		t.Fatal("duplicate effect", profiles, receipts)
	}
}

func TestPostgresUnsignedGenerationAndSafeRevisionBounds(t *testing.T) {
	dsn, db := database(t)
	s := open(t, dsn, "careeros_bounds")
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, s, fakeOIDC{})
	who := login(t, svc, "bounds")
	if _, err := db.Exec("UPDATE careeros_accounts SET generation=18446744073709551615 WHERE id=$1", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	boundedAccount := login(t, svc, "bounds")
	if boundedAccount.Principal.Generation != ^uint64(0) {
		t.Fatal("generation was truncated", boundedAccount.Principal.Generation)
	}
	if _, err := svc.Authenticate(context.Background(), boundedAccount.SessionToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthStores().Accounts.BumpGeneration(context.Background(), who.Account.ID, time.Now()); !errors.Is(err, auth.ErrUnavailable) {
		t.Fatal("generation overflow", err)
	}
	const limit = int64(9007199254740991)
	if _, err := s.SaveProfile(context.Background(), boundedAccount.Principal, "max-root", 0, fixture(t, limit)); err != nil {
		t.Fatal("max safe revision rejected", err)
	}
	if _, err := s.SaveProfile(context.Background(), boundedAccount.Principal, "exhausted", limit, fixture(t, limit)); !errors.Is(err, accountstore.ErrValidation) {
		t.Fatal("revision exhaustion", err)
	}
}

func assertRevocationWins(t *testing.T, kind string) {
	t.Helper()

	dsn, db := database(t)
	s := open(t, dsn, "careeros_racing_save")
	s.Migrate(context.Background())
	svc := service(t, s, fakeOIDC{})
	who := login(t, svc, "race")
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE", who.Account.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, e := s.SaveProfile(context.Background(), who.Principal, "racing-save", 0, fixture(t, 1))
		done <- e
	}()
	waitBlocked(t, db, "careeros_racing_save", "")
	switch kind {
	case "ordinary":
		_, err = tx.Exec("DELETE FROM careeros_sessions WHERE id=$1", who.Principal.SessionID)
	case "generation":
		_, err = tx.Exec("UPDATE careeros_accounts SET generation=generation+1 WHERE id=$1", who.Account.ID)
	case "expiry":
		_, err = tx.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", who.Principal.SessionID)
	default:
		_, err = tx.Exec("UPDATE careeros_accounts SET status=$1 WHERE id=$2", kind, who.Account.ID)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, auth.ErrAuthenticationRequired) {
		t.Fatal(err)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM careeros_profiles").Scan(&n)
	if n != 0 {
		t.Fatal("revoked publication", n)
	}
}

func assertCommitBeforeLogout(t *testing.T) {
	t.Helper()
	dsn, db := database(t)
	s := open(t, dsn, "careeros_commit_first")
	s.Migrate(context.Background())
	other := open(t, dsn, "careeros_logout_second")
	svc := service(t, s, fakeOIDC{})
	who := login(t, svc, "race")
	device := login(t, svc, "race")
	lock, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	lock.ExecContext(context.Background(), "SELECT pg_advisory_lock(72481999)")
	defer lock.ExecContext(context.Background(), "SELECT pg_advisory_unlock(72481999)")
	_, err = db.Exec(`CREATE FUNCTION pause_profile() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(72481999); RETURN NEW; END $$; CREATE TRIGGER pause_profile BEFORE INSERT ON careeros_profiles FOR EACH ROW EXECUTE FUNCTION pause_profile()`)
	if err != nil {
		t.Fatal(err)
	}
	save := make(chan error, 1)
	go func() {
		_, e := s.SaveProfile(context.Background(), who.Principal, "committed-once", 0, fixture(t, 1))
		save <- e
	}()
	waitBlocked(t, db, "careeros_commit_first", "advisory")
	revoke := make(chan error, 1)
	go func() { revoke <- other.AuthStores().Sessions.Revoke(context.Background(), who.Principal.SessionID) }()
	waitBlocked(t, db, "careeros_logout_second", "")
	lock.ExecContext(context.Background(), "SELECT pg_advisory_unlock(72481999)")
	if err = <-save; err != nil {
		t.Fatal(err)
	}
	if err = <-revoke; err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadProfile(context.Background(), who.Principal, "committed-once"); !errors.Is(err, auth.ErrAuthenticationRequired) {
		t.Fatal(err)
	}
	reply, err := other.ReadProfile(context.Background(), device.Principal, "committed-once")
	if err != nil || reply.Outcome.CommittedRevision != 1 {
		t.Fatal(reply, err)
	}
}

func assertLogoutFencesProvider(t *testing.T) {
	t.Helper()
	dsn, _ := database(t)
	s := open(t, dsn, "careeros_login_race")
	s.Migrate(context.Background())
	base := service(t, s, fakeOIDC{})
	who := login(t, base, "race")
	otherDevice := login(t, base, "race")
	entered, resume := make(chan struct{}), make(chan struct{})
	svc := service(t, s, fakeOIDC{before: func() { close(entered); <-resume }})
	start, err := svc.BeginLogin(context.Background(), "google", "/", &who.Principal)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	done := make(chan error, 1)
	go func() {
		_, e := svc.CompleteLogin(context.Background(), "google", u.Query().Get("state"), "race", start.BindingToken)
		done <- e
	}()
	<-entered
	if err = base.Logout(context.Background(), who.SessionToken); err != nil {
		t.Fatal(err)
	}
	close(resume)
	if err = <-done; !errors.Is(err, auth.ErrInvalidTransaction) {
		t.Fatal(err)
	}
	if _, err = base.Authenticate(context.Background(), otherDevice.SessionToken); err != nil {
		t.Fatal(err)
	}
	login(t, base, "race")
}
