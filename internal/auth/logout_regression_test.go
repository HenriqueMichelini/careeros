package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestOrdinaryLogoutFencesEarlierCallback(t *testing.T) {
	for _, exchanging := range []bool{false, true} {
		name := "callback_after_logout"
		if exchanging {
			name = "provider_response_after_logout"
		}
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			original := env.login(t, nil)
			otherDevice := env.login(t, nil)
			handler := NewHandler(env.service)
			startRequest := requestWithCookie(http.MethodGet, testOrigin+"/api/auth/google/start",
				&http.Cookie{Name: sessionCookieName, Value: original.SessionToken})
			started := runRequest(handler, startRequest).Result()
			defer started.Body.Close()
			if started.StatusCode != http.StatusFound {
				t.Fatalf("start = %d", started.StatusCode)
			}
			location, err := url.Parse(started.Header.Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			binding := responseCookie(t, started, oidcCookieName)
			callback := httptest.NewRequest(http.MethodGet, testOrigin+"/api/auth/google/callback?"+
				url.Values{"state": {location.Query().Get("state")}, "code": {"synthetic-code"}}.Encode(), nil)
			// Retain the actual callback cookie even if logout later clears browser cookies.
			callback.AddCookie(binding)
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			completed := make(chan *httptest.ResponseRecorder, 1)
			if exchanging {
				env.provider.beforeExchange = func() { close(entered); <-release }
				go func() { completed <- runRequest(handler, callback) }()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("provider exchange did not start")
				}
			}
			logout := requestWithCookie(http.MethodPost, testOrigin+"/api/auth/logout",
				&http.Cookie{Name: sessionCookieName, Value: original.SessionToken})
			logout.Header.Set("Origin", testOrigin)
			logout.Header.Set("X-CSRF-Token", original.CSRFToken)
			logout.AddCookie(&http.Cookie{Name: csrfCookieName, Value: original.CSRFToken})
			logout.AddCookie(binding)
			if out := runRequest(handler, logout); out.Code != http.StatusNoContent {
				t.Fatalf("logout = %d: %s", out.Code, out.Body.String())
			}
			if _, err := env.service.Authenticate(context.Background(), original.SessionToken); err != ErrAuthenticationRequired {
				t.Fatalf("original session after logout = %v", err)
			}
			var got *httptest.ResponseRecorder
			if exchanging {
				unblock()
				select {
				case got = <-completed:
				case <-time.After(3 * time.Second):
					t.Fatal("callback did not finish")
				}
			} else {
				got = runRequest(handler, callback)
			}
			if got.Code != http.StatusBadRequest {
				t.Errorf("earlier callback = %d, want 400 invalid_callback", got.Code)
			}
			if _, ok := findCookie(t, got.Result(), sessionCookieName); ok {
				t.Error("earlier callback issued a session after ordinary logout")
			}
			if got := env.stores.Sessions.(*MemorySessionStore).Len(); got != 1 {
				t.Errorf("remaining sessions = %d, want only the other device", got)
			}
			if _, err := env.service.Authenticate(context.Background(), otherDevice.SessionToken); err != nil {
				t.Fatalf("ordinary logout affected another device: %v", err)
			}
			// A new transaction after logout is allowed and reuses the account without a generation bump.
			env.provider.beforeExchange = nil
			_, freshCookie, _ := handlerLogin(t, env.service)
			fresh, err := env.service.Authenticate(context.Background(), freshCookie.Value)
			if err != nil {
				t.Fatalf("fresh sign-in after logout = %v", err)
			}
			if fresh.AccountID != original.Account.ID || fresh.Generation != original.Principal.Generation {
				t.Fatal("ordinary logout changed account identity or generation")
			}
		})
	}
}

// This barrier is after provider/account checks and immediately before the
// store commit. It catches a separate preflight check followed by insertion.
type pausedSessionCreation struct {
	SessionStore
	entered chan struct{}
	release chan struct{}
}

func (s *pausedSessionCreation) Create(ctx context.Context, session Session, previousID string, now time.Time) error {
	close(s.entered)
	<-s.release
	return s.SessionStore.Create(ctx, session, previousID, now)
}

func TestOrdinaryLogoutWinsBeforeSessionCommit(t *testing.T) {
	for _, switchAccount := range []bool{false, true} {
		name := "same_account"
		if switchAccount {
			name = "different_provider_subject"
		}
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			original := env.login(t, nil)
			start, state := env.start(t, &original.Principal)
			if switchAccount {
				env.provider.identity.Subject = "other-subject"
			}
			paused := &pausedSessionCreation{SessionStore: env.stores.Sessions,
				entered: make(chan struct{}), release: make(chan struct{})}
			env.service.sessions = paused
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(paused.release) }) }
			defer unblock()
			completed := make(chan error, 1)
			go func() {
				_, err := env.service.CompleteLogin(context.Background(), "google", state, "code", start.BindingToken)
				completed <- err
			}()
			select {
			case <-paused.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("session commit was not reached")
			}
			if err := env.service.Logout(context.Background(), original.SessionToken); err != nil {
				t.Fatal(err)
			}
			unblock()
			select {
			case err := <-completed:
				if err != ErrInvalidTransaction {
					t.Fatalf("callback after logout won before commit = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("callback did not finish")
			}
			if got := env.stores.Sessions.(*MemorySessionStore).Len(); got != 0 {
				t.Fatalf("sessions committed after revocation = %d", got)
			}
		})
	}
}
