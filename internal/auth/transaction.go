package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const maxPendingTransactions = 10000

// Transaction is the server-side half of a one-use OIDC login attempt. The
// browser receives only the state (in the authorization URL) and a separate
// binding token cookie; this record stores hashes of both.
type Transaction struct {
	StateHash         [32]byte
	BindingHash       [32]byte
	ProviderID        string
	Nonce             string
	CodeVerifier      string
	RedirectURI       string
	ReturnTo          string
	AccountID         string
	Generation        uint64
	PreviousSessionID string
	ExpiresAt         time.Time
}

// TransactionStore must consume a state atomically. A lookup followed by a
// separate delete is not sufficient to prevent replay.
type TransactionStore interface {
	Create(ctx context.Context, transaction Transaction, now time.Time) error
	Consume(ctx context.Context, stateHash [32]byte, now time.Time) (Transaction, bool, error)
}

type MemoryTransactionStore struct {
	mu           sync.Mutex
	transactions map[[32]byte]Transaction
	capacity     int
}

func NewMemoryTransactionStore() *MemoryTransactionStore {
	return &MemoryTransactionStore{
		transactions: make(map[[32]byte]Transaction),
		capacity:     maxPendingTransactions,
	}
}

func (s *MemoryTransactionStore) Create(_ context.Context, transaction Transaction, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, existing := range s.transactions {
		if !now.Before(existing.ExpiresAt) {
			delete(s.transactions, key)
		}
	}
	if len(s.transactions) >= s.capacity {
		return ErrCapacity
	}
	if _, exists := s.transactions[transaction.StateHash]; exists {
		return fmt.Errorf("duplicate sign-in transaction")
	}
	s.transactions[transaction.StateHash] = transaction
	return nil
}

func (s *MemoryTransactionStore) Consume(_ context.Context, stateHash [32]byte, now time.Time) (Transaction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transaction, ok := s.transactions[stateHash]
	if !ok {
		return Transaction{}, false, nil
	}
	delete(s.transactions, stateHash)
	if !now.Before(transaction.ExpiresAt) {
		return Transaction{}, false, nil
	}
	return transaction, true, nil
}

func (s *MemoryTransactionStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.transactions)
}

func (s *MemoryTransactionStore) lookup(stateHash [32]byte) (Transaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transaction, ok := s.transactions[stateHash]
	return transaction, ok
}
