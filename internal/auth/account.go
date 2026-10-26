package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AccountStatus is a closed set. Only active accounts can authenticate.
type AccountStatus string

const (
	AccountActive    AccountStatus = "active"
	AccountSuspended AccountStatus = "suspended"
	AccountDeleted   AccountStatus = "deleted"
)

// IdentityRef is an opaque provider subject. Email is never part of this key.
type IdentityRef struct {
	Provider  string
	Issuer    string
	Subject   string
	CreatedAt time.Time
}

// Account is a CareerOS-owned account. Generation is the authorization epoch:
// all-device logout increments it and invalidates every session issued under
// an older value.
type Account struct {
	ID          string
	Status      AccountStatus
	Generation  uint64
	CreatedAt   time.Time
	Email       string
	DisplayName string
	Identities  []IdentityRef
}

type AccountStore interface {
	FindOrCreateBySubject(ctx context.Context, identity IdentityRef, email, displayName string, now time.Time) (Account, bool, error)
	Get(ctx context.Context, id string) (Account, bool, error)
	BumpGeneration(ctx context.Context, id string, now time.Time) (Account, error)
}

type MemoryAccountStore struct {
	mu        sync.Mutex
	accounts  map[string]Account
	bySubject map[string]string
}

func NewMemoryAccountStore() *MemoryAccountStore {
	return &MemoryAccountStore{
		accounts:  make(map[string]Account),
		bySubject: make(map[string]string),
	}
}

func accountSubjectKey(identity IdentityRef) string {
	return identity.Provider + "\x00" + identity.Issuer + "\x00" + identity.Subject
}

func (s *MemoryAccountStore) FindOrCreateBySubject(_ context.Context, identity IdentityRef, email, displayName string, now time.Time) (Account, bool, error) {
	if identity.Provider == "" || identity.Issuer == "" || identity.Subject == "" {
		return Account{}, false, fmt.Errorf("%w: incomplete identity", ErrInvalidCallback)
	}
	key := accountSubjectKey(identity)
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.bySubject[key]; ok {
		account, exists := s.accounts[id]
		if !exists {
			return Account{}, false, fmt.Errorf("%w: identity points to a missing account", ErrUnavailable)
		}
		return cloneAccount(account), false, nil
	}
	id, err := randomToken(16)
	if err != nil {
		return Account{}, false, err
	}
	account := Account{
		ID:          id,
		Status:      AccountActive,
		Generation:  1,
		CreatedAt:   now.UTC(),
		Email:       email,
		DisplayName: displayName,
		Identities:  []IdentityRef{{Provider: identity.Provider, Issuer: identity.Issuer, Subject: identity.Subject, CreatedAt: now.UTC()}},
	}
	s.accounts[id] = account
	s.bySubject[key] = id
	return cloneAccount(account), true, nil
}

func (s *MemoryAccountStore) Get(_ context.Context, id string) (Account, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[id]
	if !ok {
		return Account{}, false, nil
	}
	return cloneAccount(account), true, nil
}

func (s *MemoryAccountStore) BumpGeneration(_ context.Context, id string, _ time.Time) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[id]
	if !ok {
		return Account{}, fmt.Errorf("%w: account not found", ErrAuthenticationRequired)
	}
	if account.Generation == ^uint64(0) {
		return Account{}, fmt.Errorf("%w: generation exhausted", ErrUnavailable)
	}
	account.Generation++
	s.accounts[id] = account
	return cloneAccount(account), nil
}

// SetStatus is a test and operator seam. No HTTP route exposes it in this
// slice; account suspension/deletion policy remains unselected.
func (s *MemoryAccountStore) SetStatus(id string, status AccountStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.accounts[id]
	if !ok {
		return
	}
	account.Status = status
	s.accounts[id] = account
}

func (s *MemoryAccountStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.accounts)
}

func cloneAccount(account Account) Account {
	clone := account
	clone.Identities = append([]IdentityRef(nil), account.Identities...)
	return clone
}
