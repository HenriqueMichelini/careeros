package auth

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionReplacementRejectsMissingRevokedAndExpiredInitiator(t *testing.T) {
	for _, condition := range []string{"missing", "revoked", "expired"} {
		t.Run(condition, func(t *testing.T) {
			env := newTestEnv(t)
			original := env.login(t, nil)
			store := env.stores.Sessions
			previousID := original.Principal.SessionID
			now := env.now
			switch condition {
			case "missing":
				previousID = "unknown-session"
			case "revoked":
				if err := store.Revoke(context.Background(), previousID); err != nil {
					t.Fatal(err)
				}
			case "expired":
				now = original.ExpiresAt
			}
			replacement := Session{ID: "replacement", AccountID: original.Account.ID,
				TokenHash: hashSecret("replacement-token"), CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
			if err := store.Create(context.Background(), replacement, previousID, now); err != ErrInvalidTransaction {
				t.Fatalf("replacement with %s initiating session = %v", condition, err)
			}
			if _, ok, err := store.FindByTokenHash(context.Background(), replacement.TokenHash, now); err != nil || ok {
				t.Fatalf("replacement inserted on failure: exists=%v err=%v", ok, err)
			}
		})
	}
}

func TestConcurrentSessionReplacementsConsumeInitiatorOnce(t *testing.T) {
	env := newTestEnv(t)
	original := env.login(t, nil)
	const workers = 16
	var successes atomic.Int32
	var wg sync.WaitGroup
	ready := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			id := fmt.Sprintf("replacement-%d", i)
			replacement := Session{ID: id, AccountID: original.Account.ID, TokenHash: hashSecret(id),
				CreatedAt: env.now, ExpiresAt: env.now.Add(time.Hour)}
			err := env.stores.Sessions.Create(context.Background(), replacement, original.Principal.SessionID, env.now)
			if err == nil {
				successes.Add(1)
			} else if err != ErrInvalidTransaction {
				t.Errorf("replacement = %v", err)
			}
		}()
	}
	close(ready)
	wg.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful replacements = %d, want 1", got)
	}
	if got := env.stores.Sessions.(*MemorySessionStore).Len(); got != 1 {
		t.Fatalf("stored sessions = %d, want 1", got)
	}
	if _, err := env.service.Authenticate(context.Background(), original.SessionToken); err != ErrAuthenticationRequired {
		t.Fatalf("replaced session still authenticates: %v", err)
	}
}

func TestFailedSessionReplacementPreservesInitiator(t *testing.T) {
	env := newTestEnv(t)
	original := env.login(t, nil)
	duplicate := Session{ID: "replacement", AccountID: original.Account.ID,
		TokenHash: hashSecret(original.SessionToken), ExpiresAt: env.now.Add(time.Hour)}
	if err := env.stores.Sessions.Create(context.Background(), duplicate, original.Principal.SessionID, env.now); err == nil {
		t.Fatal("duplicate token accepted")
	}
	if _, err := env.service.Authenticate(context.Background(), original.SessionToken); err != nil {
		t.Fatalf("failed insertion revoked the initiating session: %v", err)
	}
}
