package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Session is the server-side record for an opaque browser session. Only the
// SHA-256 hashes of the session and CSRF tokens are stored.
type Session struct {
	ID         string
	AccountID  string
	TokenHash  [32]byte
	CSRFHash   [32]byte
	Generation uint64
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

type SessionStore interface {
	Create(ctx context.Context, session Session) error
	FindByTokenHash(ctx context.Context, tokenHash [32]byte, now time.Time) (Session, bool, error)
	Revoke(ctx context.Context, id string) error
	RevokeAllForAccount(ctx context.Context, accountID string) (int, error)
}

type MemorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]Session
	byHash   map[[32]byte]string
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]Session),
		byHash:   make(map[[32]byte]string),
	}
}

func (s *MemorySessionStore) Create(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session.ID == "" || session.AccountID == "" {
		return fmt.Errorf("%w: incomplete session", ErrUnavailable)
	}
	if _, exists := s.sessions[session.ID]; exists {
		return fmt.Errorf("%w: duplicate session", ErrUnavailable)
	}
	if _, exists := s.byHash[session.TokenHash]; exists {
		return fmt.Errorf("%w: duplicate session token", ErrUnavailable)
	}
	s.sessions[session.ID] = session
	s.byHash[session.TokenHash] = session.ID
	return nil
}

func (s *MemorySessionStore) FindByTokenHash(_ context.Context, tokenHash [32]byte, now time.Time) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byHash[tokenHash]
	if !ok {
		return Session{}, false, nil
	}
	session, ok := s.sessions[id]
	if !ok {
		delete(s.byHash, tokenHash)
		return Session{}, false, nil
	}
	if !now.Before(session.ExpiresAt) {
		delete(s.sessions, id)
		delete(s.byHash, tokenHash)
		return Session{}, false, nil
	}
	return session, true, nil
}

func (s *MemorySessionStore) Revoke(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil
	}
	delete(s.sessions, id)
	delete(s.byHash, session.TokenHash)
	return nil
}

func (s *MemorySessionStore) RevokeAllForAccount(_ context.Context, accountID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	revoked := 0
	for id, session := range s.sessions {
		if session.AccountID != accountID {
			continue
		}
		delete(s.sessions, id)
		delete(s.byHash, session.TokenHash)
		revoked++
	}
	return revoked, nil
}

func (s *MemorySessionStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}
