package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Stores groups the three persistence seams. The production backend remains
// unselected; the memory implementations are the reference used by tests and
// local single-process runs.
type Stores struct {
	Sessions     SessionStore
	Accounts     AccountStore
	Transactions TransactionStore
}

func NewMemoryStores() Stores {
	return Stores{
		Sessions:     NewMemorySessionStore(),
		Accounts:     NewMemoryAccountStore(),
		Transactions: NewMemoryTransactionStore(),
	}
}

// Principal is the server-derived identity attached to a protected request.
// The unexported hashes are never serialized.
type Principal struct {
	AccountID   string
	SessionID   string
	Generation  uint64
	Email       string
	DisplayName string
	ExpiresAt   time.Time

	tokenHash [32]byte
	csrfHash  [32]byte
}

type LoginStart struct {
	URL          string
	BindingToken string
	ExpiresAt    time.Time
}

type LoginResult struct {
	Principal    Principal
	Account      Account
	SessionToken string
	CSRFToken    string
	ReturnTo     string
	ExpiresAt    time.Time
}

type AccountView struct {
	ID          string `json:"id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type SessionView struct {
	Authenticated bool         `json:"authenticated"`
	Account       *AccountView `json:"account,omitempty"`
	ExpiresAt     *time.Time   `json:"expiresAt,omitempty"`
}

// Service owns the sign-in transaction, account mapping and session
// authority. Provider tokens are never stored.
type Service struct {
	config          Config
	providers       *Registry
	providerConfigs map[string]ProviderConfig
	sessions        SessionStore
	accounts        AccountStore
	transactions    TransactionStore
	now             func() time.Time
}

func NewService(config Config, providers *Registry, stores Stores) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if providers == nil {
		return nil, fmt.Errorf("%w: provider registry is required", ErrUnavailable)
	}
	if stores.Sessions == nil || stores.Accounts == nil || stores.Transactions == nil {
		return nil, fmt.Errorf("%w: all auth stores are required", ErrUnavailable)
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	providerConfigs := make(map[string]ProviderConfig, len(config.Providers))
	for _, provider := range config.Providers {
		if _, ok := providers.Get(provider.ID); !ok {
			return nil, fmt.Errorf("%w: provider %q is configured but not registered", ErrUnavailable, provider.ID)
		}
		providerConfigs[provider.ID] = provider
	}
	if len(providerConfigs) != len(providers.IDs()) {
		return nil, fmt.Errorf("%w: unconfigured provider is registered", ErrUnavailable)
	}
	return &Service{
		config:          config,
		providers:       providers,
		providerConfigs: providerConfigs,
		sessions:        stores.Sessions,
		accounts:        stores.Accounts,
		transactions:    stores.Transactions,
		now:             now,
	}, nil
}

func (s *Service) PublicOrigin() string      { return strings.TrimRight(s.config.PublicOrigin, "/") }
func (s *Service) SessionTTL() time.Duration { return s.config.SessionTTL }

// BeginLogin creates one server-side transaction and returns the exact
// authorization URL. The caller must set the binding token as an HttpOnly
// cookie before navigating the browser.
func (s *Service) BeginLogin(ctx context.Context, providerID, returnTo string, current *Principal) (LoginStart, error) {
	provider, ok := s.providers.Get(providerID)
	if !ok {
		return LoginStart{}, ErrProviderNotAllowed
	}
	state, err := randomToken(32)
	if err != nil {
		return LoginStart{}, err
	}
	nonce, err := randomToken(32)
	if err != nil {
		return LoginStart{}, err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return LoginStart{}, err
	}
	binding, err := randomToken(32)
	if err != nil {
		return LoginStart{}, err
	}
	now := s.now().UTC()
	transaction := Transaction{
		StateHash:    hashSecret(state),
		BindingHash:  hashSecret(binding),
		ProviderID:   providerID,
		Nonce:        nonce,
		CodeVerifier: verifier,
		RedirectURI:  s.providerConfigs[providerID].RedirectURI,
		ReturnTo:     safeReturnTo(returnTo),
		ExpiresAt:    now.Add(s.config.TransactionTTL),
	}
	if current != nil {
		transaction.AccountID = current.AccountID
		transaction.Generation = current.Generation
		transaction.PreviousSessionID = current.SessionID
	}
	if err := s.transactions.Create(ctx, transaction, now); err != nil {
		if err == ErrCapacity {
			return LoginStart{}, fmt.Errorf("%w: too many pending sign-ins", ErrUnavailable)
		}
		return LoginStart{}, err
	}
	challenge := base64.RawURLEncoding.EncodeToString(sha256Bytes(verifier))
	return LoginStart{
		URL:          provider.AuthorizationURL(state, nonce, challenge, transaction.RedirectURI),
		BindingToken: binding,
		ExpiresAt:    transaction.ExpiresAt,
	}, nil
}

// CancelLogin consumes a pending transaction when the provider reports an
// error. Consumption is best effort but still one-use.
func (s *Service) CancelLogin(ctx context.Context, state string) {
	if len(state) == 0 || len(state) > 128 {
		return
	}
	_, _, _ = s.transactions.Consume(ctx, hashSecret(state), s.now().UTC())
}

// CompleteLogin consumes the transaction, validates the provider assertion,
// maps the provider subject to an account and issues a fresh opaque session.
// It never links by email and never reuses a provider token as a session.
func (s *Service) CompleteLogin(ctx context.Context, providerID, state, code, bindingToken string) (LoginResult, error) {
	provider, transaction, now, err := s.consumeLogin(ctx, providerID, state, code, bindingToken)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.requireTransactionAccount(ctx, transaction); err != nil {
		return LoginResult{}, err
	}

	identity, err := provider.Exchange(ctx, code, transaction.CodeVerifier, transaction.RedirectURI, transaction.Nonce)
	if err != nil {
		return LoginResult{}, err
	}
	if identity.Provider != providerID || identity.Issuer == "" || identity.Subject == "" {
		return LoginResult{}, ErrInvalidCallback
	}
	account, err := s.resolveAccount(ctx, transaction, identity, now)
	if err != nil {
		return LoginResult{}, err
	}
	return s.issueSession(ctx, transaction, account, now)
}

func (s *Service) consumeLogin(ctx context.Context, providerID, state, code, bindingToken string) (Provider, Transaction, time.Time, error) {
	provider, ok := s.providers.Get(providerID)
	if !ok {
		return nil, Transaction{}, time.Time{}, ErrProviderNotAllowed
	}
	if len(state) == 0 || len(state) > 128 {
		return nil, Transaction{}, time.Time{}, ErrInvalidTransaction
	}
	if len(code) == 0 || len(code) > 4096 {
		return nil, Transaction{}, time.Time{}, ErrInvalidCallback
	}
	if len(bindingToken) == 0 || len(bindingToken) > 128 {
		return nil, Transaction{}, time.Time{}, ErrInvalidTransaction
	}
	now := s.now().UTC()
	transaction, ok, err := s.transactions.Consume(ctx, hashSecret(state), now)
	if err != nil {
		return nil, Transaction{}, time.Time{}, err
	}
	if !ok {
		return nil, Transaction{}, time.Time{}, ErrInvalidTransaction
	}
	if transaction.ProviderID != providerID {
		return nil, Transaction{}, time.Time{}, ErrInvalidTransaction
	}
	bindingHash := hashSecret(bindingToken)
	if subtle.ConstantTimeCompare(bindingHash[:], transaction.BindingHash[:]) != 1 {
		return nil, Transaction{}, time.Time{}, ErrInvalidTransaction
	}
	return provider, transaction, now, nil
}

func (s *Service) requireTransactionAccount(ctx context.Context, transaction Transaction) error {
	if transaction.AccountID == "" {
		return nil
	}
	current, ok, err := s.accounts.Get(ctx, transaction.AccountID)
	if err != nil {
		return err
	}
	if !ok || current.Status != AccountActive || current.Generation != transaction.Generation {
		return ErrInvalidTransaction
	}
	return nil
}

func (s *Service) resolveAccount(ctx context.Context, transaction Transaction, identity Identity, now time.Time) (Account, error) {
	account, _, err := s.accounts.FindOrCreateBySubject(ctx, IdentityRef{
		Provider: identity.Provider,
		Issuer:   identity.Issuer,
		Subject:  identity.Subject,
	}, identity.Email, identity.Name, now)
	if err != nil {
		return Account{}, err
	}
	if account.Status != AccountActive {
		return Account{}, ErrForbidden
	}
	// A logout-all that raced the provider exchange must invalidate this
	// attempt even though the new account row now carries a newer generation.
	if transaction.AccountID != "" && account.ID == transaction.AccountID && account.Generation != transaction.Generation {
		return Account{}, ErrInvalidTransaction
	}
	return account, nil
}

func (s *Service) issueSession(ctx context.Context, transaction Transaction, account Account, now time.Time) (LoginResult, error) {
	if transaction.PreviousSessionID != "" {
		if err := s.sessions.Revoke(ctx, transaction.PreviousSessionID); err != nil {
			return LoginResult{}, fmt.Errorf("%w: previous session", ErrUnavailable)
		}
	}

	sessionToken, err := randomToken(32)
	if err != nil {
		return LoginResult{}, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return LoginResult{}, err
	}
	sessionID, err := randomToken(16)
	if err != nil {
		return LoginResult{}, err
	}
	expiresAt := now.Add(s.config.SessionTTL)
	session := Session{
		ID:         sessionID,
		AccountID:  account.ID,
		TokenHash:  hashSecret(sessionToken),
		CSRFHash:   hashSecret(csrfToken),
		Generation: account.Generation,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		Principal: Principal{
			AccountID:   account.ID,
			SessionID:   session.ID,
			Generation:  session.Generation,
			Email:       account.Email,
			DisplayName: account.DisplayName,
			ExpiresAt:   session.ExpiresAt,
			tokenHash:   session.TokenHash,
			csrfHash:    session.CSRFHash,
		},
		Account:      account,
		SessionToken: sessionToken,
		CSRFToken:    csrfToken,
		ReturnTo:     transaction.ReturnTo,
		ExpiresAt:    expiresAt,
	}, nil
}

// Authenticate resolves a raw session token to a live principal. Revoked,
// expired, suspended, deleted and stale-generation sessions all return the
// same authentication error.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if len(token) == 0 || len(token) > 128 {
		return Principal{}, ErrAuthenticationRequired
	}
	now := s.now().UTC()
	session, ok, err := s.sessions.FindByTokenHash(ctx, hashSecret(token), now)
	if err != nil {
		return Principal{}, err
	}
	if !ok {
		return Principal{}, ErrAuthenticationRequired
	}
	account, ok, err := s.accounts.Get(ctx, session.AccountID)
	if err != nil {
		return Principal{}, err
	}
	if !ok || account.Status != AccountActive || account.Generation != session.Generation {
		return Principal{}, ErrAuthenticationRequired
	}
	return Principal{
		AccountID:   account.ID,
		SessionID:   session.ID,
		Generation:  session.Generation,
		Email:       account.Email,
		DisplayName: account.DisplayName,
		ExpiresAt:   session.ExpiresAt,
		tokenHash:   session.TokenHash,
		csrfHash:    session.CSRFHash,
	}, nil
}

// Revalidate re-reads the session and account. Protected handlers must call
// this immediately before an authoritative write so a logout/revocation that
// raced request processing wins.
func (s *Service) Revalidate(ctx context.Context, principal Principal) (Principal, error) {
	if principal.SessionID == "" || principal.AccountID == "" {
		return Principal{}, ErrAuthenticationRequired
	}
	now := s.now().UTC()
	ctxSession, ok, err := s.sessions.FindByTokenHash(ctx, principal.tokenHash, now)
	if err != nil {
		return Principal{}, err
	}
	if !ok || ctxSession.ID != principal.SessionID {
		return Principal{}, ErrAuthenticationRequired
	}
	account, ok, err := s.accounts.Get(ctx, ctxSession.AccountID)
	if err != nil {
		return Principal{}, err
	}
	if !ok || account.Status != AccountActive || account.Generation != ctxSession.Generation {
		return Principal{}, ErrAuthenticationRequired
	}
	return Principal{
		AccountID:   account.ID,
		SessionID:   ctxSession.ID,
		Generation:  ctxSession.Generation,
		Email:       account.Email,
		DisplayName: account.DisplayName,
		ExpiresAt:   ctxSession.ExpiresAt,
		tokenHash:   ctxSession.TokenHash,
		csrfHash:    ctxSession.CSRFHash,
	}, nil
}

// Logout revokes one session. It is idempotent so an expired or already
// revoked browser can still clear its cookies.
func (s *Service) Logout(ctx context.Context, token string) error {
	if len(token) == 0 || len(token) > 128 {
		return nil
	}
	now := s.now().UTC()
	session, ok, err := s.sessions.FindByTokenHash(ctx, hashSecret(token), now)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return s.sessions.Revoke(ctx, session.ID)
}

// LogoutAll increments the account authorization generation before revoking
// every session. A session issued before the increment can never pass
// Authenticate, even if a concurrent revoke misses it.
func (s *Service) LogoutAll(ctx context.Context, principal Principal) (int, error) {
	principal, err := s.Revalidate(ctx, principal)
	if err != nil {
		return 0, err
	}
	if _, err := s.accounts.BumpGeneration(ctx, principal.AccountID, s.now().UTC()); err != nil {
		return 0, err
	}
	return s.sessions.RevokeAllForAccount(ctx, principal.AccountID)
}

// ValidateCSRF checks the per-session CSRF secret against an unsafe request's
// header. Origin checking is performed by the guard as a separate gate.
func (s *Service) ValidateCSRF(principal Principal, token string) bool {
	if len(token) == 0 || len(token) > 128 || principal.csrfHash == ([32]byte{}) {
		return false
	}
	hash := hashSecret(token)
	return subtle.ConstantTimeCompare(hash[:], principal.csrfHash[:]) == 1
}

// AuthorizeOwner is the server-enforced resource ownership check. Callers
// must pass an owner derived from trusted server state, never a client field.
func (s *Service) AuthorizeOwner(principal Principal, ownerAccountID string) error {
	if ownerAccountID == "" || principal.AccountID == "" || principal.AccountID != ownerAccountID {
		return ErrForbidden
	}
	return nil
}

func (s *Service) SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	expected, err := url.Parse(s.PublicOrigin())
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, expected.Scheme) && strings.EqualFold(parsed.Host, expected.Host)
}

func (s *Service) View(principal *Principal) SessionView {
	if principal == nil {
		return SessionView{Authenticated: false}
	}
	expires := principal.ExpiresAt
	return SessionView{
		Authenticated: true,
		Account: &AccountView{
			ID:          principal.AccountID,
			Email:       principal.Email,
			DisplayName: principal.DisplayName,
		},
		ExpiresAt: &expires,
	}
}

func safeReturnTo(raw string) string {
	if raw == "" || len(raw) > 512 {
		return "/"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "/"
	}
	if parsed.Path != "/" {
		return "/"
	}
	return "/"
}

func sha256Bytes(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
