package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBeginLoginRequiresAllowlistedProvider(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.service.BeginLogin(context.Background(), "github", "/", nil); err != ErrProviderNotAllowed {
		t.Fatalf("error = %v, want ErrProviderNotAllowed", err)
	}
	if got := env.stores.Transactions.(*MemoryTransactionStore).Len(); got != 0 {
		t.Fatalf("pending transactions = %d, want 0", got)
	}
}

func TestBeginLoginCreatesPKCEAndNonceTransaction(t *testing.T) {
	env := newTestEnv(t)
	start, state := env.start(t, nil)
	parsed, err := url.Parse(start.URL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("challenge method = %q", query.Get("code_challenge_method"))
	}
	if query.Get("code_challenge") == "" || query.Get("nonce") == "" || query.Get("state") == "" {
		t.Fatalf("missing PKCE or nonce parameters: %s", start.URL)
	}
	if query.Get("redirect_uri") != testOrigin+"/api/auth/google/callback" {
		t.Fatalf("redirect_uri = %q", query.Get("redirect_uri"))
	}
	transaction, ok := env.stores.Transactions.(*MemoryTransactionStore).lookup(hashSecret(state))
	if !ok {
		t.Fatal("server-side transaction missing")
	}
	if transaction.Nonce != query.Get("nonce") {
		t.Fatalf("server nonce does not match authorization URL")
	}
	if transaction.CodeVerifier == "" || transaction.CodeVerifier == query.Get("code_challenge") {
		t.Fatalf("code verifier is missing or was reused as a challenge")
	}
	challenge := sha256.Sum256([]byte(transaction.CodeVerifier))
	if base64.RawURLEncoding.EncodeToString(challenge[:]) != query.Get("code_challenge") {
		t.Fatalf("challenge is not S256(code_verifier)")
	}
	if transaction.ProviderID != "google" {
		t.Fatalf("provider = %q", transaction.ProviderID)
	}
	if transaction.BindingHash != hashSecret(start.BindingToken) {
		t.Fatalf("binding hash does not match binding token")
	}
}

func TestCallbackBindingAndReplayAreRejected(t *testing.T) {
	env := newTestEnv(t)
	start, state := env.start(t, nil)
	if _, err := env.service.CompleteLogin(context.Background(), "google", state, "code", "wrong-binding"); err != ErrInvalidTransaction {
		t.Fatalf("wrong binding error = %v, want ErrInvalidTransaction", err)
	}
	if _, err := env.service.CompleteLogin(context.Background(), "google", state, "code", start.BindingToken); err != ErrInvalidTransaction {
		t.Fatalf("replay error = %v, want ErrInvalidTransaction after consumption", err)
	}
	if env.provider.callCount() != 0 {
		t.Fatalf("provider was called %d times after invalid binding", env.provider.callCount())
	}
}

func TestCallbackStateTamperingDoesNotConsumeRealTransaction(t *testing.T) {
	env := newTestEnv(t)
	start, state := env.start(t, nil)
	tampered := state[:len(state)-1] + "x"
	if tampered == state {
		tampered = state[:len(state)-1] + "y"
	}
	if _, err := env.service.CompleteLogin(context.Background(), "google", tampered, "code", start.BindingToken); err != ErrInvalidTransaction {
		t.Fatalf("tampered state error = %v", err)
	}
	if _, err := env.service.CompleteLogin(context.Background(), "google", state, "code", start.BindingToken); err != nil {
		t.Fatalf("real transaction was consumed by a tampered state: %v", err)
	}
}

func TestConcurrentCallbackConsumesTransactionOnce(t *testing.T) {
	env := newTestEnv(t)
	start, state := env.start(t, nil)
	const workers = 16
	var successes atomic.Int32
	var wg sync.WaitGroup
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := env.service.CompleteLogin(context.Background(), "google", state, "code", start.BindingToken); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful callbacks = %d, want exactly 1", got)
	}
}

func TestProviderIdentityMapsBySubjectNotEmail(t *testing.T) {
	env := newTestEnv(t)
	first := env.loginAs(t, "same-subject", "same@example.com")
	second := env.loginAs(t, "different-subject", "same@example.com")
	if first.Account.ID == second.Account.ID {
		t.Fatal("two provider subjects with the same email were linked")
	}
	if len(env.stores.Accounts.(*MemoryAccountStore).accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", env.stores.Accounts.(*MemoryAccountStore).Len())
	}
	if _, err := env.service.Authenticate(context.Background(), first.SessionToken); err != nil {
		t.Fatalf("first account session invalid: %v", err)
	}
	if _, err := env.service.Authenticate(context.Background(), second.SessionToken); err != nil {
		t.Fatalf("second account session invalid: %v", err)
	}
}

func TestSessionTokenIsStoredOnlyAsHash(t *testing.T) {
	env := newTestEnv(t)
	result := env.login(t, nil)
	if result.SessionToken == "" || result.CSRFToken == "" {
		t.Fatal("missing session or CSRF token")
	}
	store := env.stores.Sessions.(*MemorySessionStore)
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.sessions) != 1 {
		t.Fatalf("sessions = %d", len(store.sessions))
	}
	for id, session := range store.sessions {
		if id == result.SessionToken {
			t.Fatal("raw session token used as a map key")
		}
		if session.TokenHash != hashSecret(result.SessionToken) {
			t.Fatal("stored token hash does not match")
		}
		if _, ok := store.byHash[hashSecret(result.SessionToken)]; !ok {
			t.Fatal("session hash index missing")
		}
		if session.CSRFHash != hashSecret(result.CSRFToken) {
			t.Fatal("stored CSRF hash does not match")
		}
		if session.TokenHash == hashSecret(result.CSRFToken) {
			t.Fatal("session and CSRF secrets are not independent")
		}
	}
}

func TestLogoutRevokesSingleSession(t *testing.T) {
	env := newTestEnv(t)
	result := env.login(t, nil)
	if err := env.service.Logout(context.Background(), result.SessionToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := env.service.Authenticate(context.Background(), result.SessionToken); err != ErrAuthenticationRequired {
		t.Fatalf("after logout error = %v", err)
	}
	// Idempotent second logout.
	if err := env.service.Logout(context.Background(), result.SessionToken); err != nil {
		t.Fatalf("second Logout: %v", err)
	}
}

func TestLogoutAllInvalidatesAllSessionsAndGeneration(t *testing.T) {
	env := newTestEnv(t)
	first := env.login(t, nil)
	second := env.login(t, &first.Principal)
	if first.SessionToken == second.SessionToken {
		t.Fatal("re-login reused a session token")
	}
	if _, err := env.service.Authenticate(context.Background(), first.SessionToken); err == nil {
		t.Fatal("old session remained valid after re-login")
	}
	if _, err := env.service.Authenticate(context.Background(), second.SessionToken); err != nil {
		t.Fatalf("new session invalid: %v", err)
	}
	count, err := env.service.LogoutAll(context.Background(), second.Principal)
	if err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	if count != 1 {
		t.Fatalf("revoked = %d, want 1", count)
	}
	if _, err := env.service.Authenticate(context.Background(), second.SessionToken); err != ErrAuthenticationRequired {
		t.Fatalf("after all-device logout error = %v", err)
	}
	account, ok := env.stores.Accounts.(*MemoryAccountStore).accounts[first.Account.ID]
	if !ok || account.Generation != 2 {
		t.Fatalf("account generation = %d, want 2", account.Generation)
	}
}

func TestExpiredAndSuspendedSessionsFailClosed(t *testing.T) {
	env := newTestEnv(t)
	result := env.login(t, nil)
	env.now = env.now.Add(env.config.SessionTTL + time.Second)
	if _, err := env.service.Authenticate(context.Background(), result.SessionToken); err != ErrAuthenticationRequired {
		t.Fatalf("expired session error = %v", err)
	}

	env = newTestEnv(t)
	result = env.login(t, nil)
	env.stores.Accounts.(*MemoryAccountStore).SetStatus(result.Account.ID, AccountSuspended)
	if _, err := env.service.Authenticate(context.Background(), result.SessionToken); err != ErrAuthenticationRequired {
		t.Fatalf("suspended session error = %v", err)
	}
	env.stores.Accounts.(*MemoryAccountStore).SetStatus(result.Account.ID, AccountDeleted)
	if _, err := env.service.Authenticate(context.Background(), result.SessionToken); err != ErrAuthenticationRequired {
		t.Fatalf("deleted session error = %v", err)
	}
}

func TestLateLoginAfterLogoutAllIsRejected(t *testing.T) {
	env := newTestEnv(t)
	first := env.login(t, nil)
	start, state := env.start(t, &first.Principal)
	if _, err := env.service.LogoutAll(context.Background(), first.Principal); err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	if _, err := env.service.CompleteLogin(context.Background(), "google", state, "code", start.BindingToken); err != ErrInvalidTransaction {
		t.Fatalf("late login error = %v, want ErrInvalidTransaction", err)
	}
}

func TestLogoutRacingProviderExchangeIsRejected(t *testing.T) {
	env := newTestEnv(t)
	first := env.login(t, nil)
	start, state := env.start(t, &first.Principal)
	done := make(chan struct{})
	env.provider.beforeExchange = func() {
		<-done
	}
	result := make(chan error, 1)
	go func() {
		_, err := env.service.CompleteLogin(context.Background(), "google", state, "code", start.BindingToken)
		result <- err
	}()
	if _, err := env.service.LogoutAll(context.Background(), first.Principal); err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	close(done)
	if err := <-result; err != ErrInvalidTransaction {
		t.Fatalf("login that raced logout error = %v, want ErrInvalidTransaction", err)
	}
}

func TestNewLoginAfterLogoutAllIsValid(t *testing.T) {
	env := newTestEnv(t)
	first := env.login(t, nil)
	if _, err := env.service.LogoutAll(context.Background(), first.Principal); err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	second := env.login(t, nil)
	if second.Account.ID != first.Account.ID {
		t.Fatal("new sign-in created a second account for the same provider subject")
	}
	if second.Account.Generation != 2 {
		t.Fatalf("generation = %d, want 2", second.Account.Generation)
	}
	if _, err := env.service.Authenticate(context.Background(), second.SessionToken); err != nil {
		t.Fatalf("new session invalid: %v", err)
	}
}

func TestOwnershipIsServerDerived(t *testing.T) {
	env := newTestEnv(t)
	first := env.loginAs(t, "subject-a", "a@example.com")
	second := env.loginAs(t, "subject-b", "a@example.com")
	if err := env.service.AuthorizeOwner(first.Principal, first.Account.ID); err != nil {
		t.Fatalf("owner rejected: %v", err)
	}
	if err := env.service.AuthorizeOwner(second.Principal, first.Account.ID); err != ErrForbidden {
		t.Fatalf("cross-account error = %v, want ErrForbidden", err)
	}
	if err := env.service.AuthorizeOwner(second.Principal, ""); err != ErrForbidden {
		t.Fatalf("empty owner error = %v, want ErrForbidden", err)
	}
}

func TestRevalidateCatchesRevocationBeforeCommit(t *testing.T) {
	env := newTestEnv(t)
	result := env.login(t, nil)
	if _, err := env.service.Revalidate(context.Background(), result.Principal); err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if _, err := env.service.LogoutAll(context.Background(), result.Principal); err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	if _, err := env.service.Revalidate(context.Background(), result.Principal); err != ErrAuthenticationRequired {
		t.Fatalf("post-revocation Revalidate error = %v", err)
	}
}

func TestCSRFValidationUsesPerSessionSecret(t *testing.T) {
	env := newTestEnv(t)
	first := env.login(t, nil)
	second := env.loginAs(t, "subject-two", "two@example.com")
	if !env.service.ValidateCSRF(first.Principal, first.CSRFToken) {
		t.Fatal("valid CSRF token rejected")
	}
	if env.service.ValidateCSRF(first.Principal, second.CSRFToken) {
		t.Fatal("CSRF token from another session accepted")
	}
	if env.service.ValidateCSRF(first.Principal, "") {
		t.Fatal("empty CSRF token accepted")
	}
}

func TestSafeReturnToRejectsOpenRedirects(t *testing.T) {
	cases := map[string]string{
		"":                          "/",
		"/":                         "/",
		"https://evil.example":      "/",
		"//evil.example":            "/",
		"/\\evil.example":           "/",
		"/?next=https://evil":       "/",
		"javascript:alert(1)":       "/",
		"/home":                     "/",
		"https://careeros.example/": "/",
	}
	for input, want := range cases {
		if got := safeReturnTo(input); got != want {
			t.Errorf("safeReturnTo(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSameOriginRequiresExactOrigin(t *testing.T) {
	env := newTestEnv(t)
	cases := map[string]bool{
		testOrigin:                   true,
		"http://127.0.0.1:8787/":     false,
		"http://127.0.0.1:9999":      false,
		"https://127.0.0.1:8787":     false,
		"http://127.0.0.1:8787/path": false,
		"null":                       false,
		"":                           false,
	}
	for origin, want := range cases {
		request := requestWithCookie("POST", testOrigin+"/api/auth/logout", nil)
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if got := env.service.SameOrigin(request); got != want {
			t.Errorf("SameOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}
