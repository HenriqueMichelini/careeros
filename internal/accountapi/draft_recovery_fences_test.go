package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/draftrecovery"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type stoppedDraftWriter struct {
	*httptest.ResponseRecorder
	entered, release chan struct{}
}

func (w *stoppedDraftWriter) Write(raw []byte) (int, error) {
	close(w.entered)
	<-w.release
	return w.ResponseRecorder.Write(raw)
}
func TestControlledDraftFinalAccountRouterWriteHoldsFence(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	ready := r.create(t, "outer-write")
	req := r.f.Request(t, context.Background(), "/.netlify/functions/account/api/drafts/outer-write/versions/1", nil)
	req.Method = "GET"
	w := &stoppedDraftWriter{httptest.NewRecorder(), make(chan struct{}), make(chan struct{})}
	done := make(chan struct{})
	go func() { defer close(done); r.router.ServeHTTP(w, req) }()
	select {
	case <-w.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual outer body write not reached")
	}
	// The real account lock must still be held while the OUTERMOST writer blocks.
	tx, err := r.f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SET LOCAL lock_timeout='50ms'`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, r.f.Principal.AccountID)
	tx.Rollback()
	close(w.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not finish")
	}
	if err == nil || !strings.Contains(err.Error(), "lock timeout") {
		t.Fatalf("final write lost account fence: %v", err)
	}
	var got draftrecovery.Result
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil || !reflect.DeepEqual(ready, got) {
		t.Fatal("read-first schedule changed complete version")
	}
	r.authority(t, `UPDATE controlled_draft_test_authority SET deleted=true,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID)
	denied := r.request(t, "GET", "/api/drafts/outer-write/versions/1", "", nil)
	if denied.Code != 410 || bytes.Contains(denied.Body.Bytes(), []byte(`"bundle"`)) || r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatalf("fence-first exposed content: %d %s", denied.Code, denied.Body.String())
	}
}
func TestControlledDraftFinalWriteRechecksExpiredSession(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	r.create(t, "session-final")
	if _, err := r.f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '300 milliseconds' WHERE id=$1`, r.f.Principal.SessionID); err != nil {
		t.Fatal(err)
	}
	r.fault = func(stage string) error {
		if stage == "before_result_write" {
			time.Sleep(400 * time.Millisecond)
		}
		return nil
	}
	w := r.request(t, "GET", "/api/drafts/session-final/versions/1", "", nil)
	if w.Code != 401 || bytes.Contains(w.Body.Bytes(), []byte(`"bundle"`)) || r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatalf("expired final session leaked: %d %s", w.Code, w.Body.String())
	}
}
func TestControlledDraftOriginalDeadlineAndInjectedLifecycle(t *testing.T) {
	for _, gate := range []string{"deadline", "grant", "revoked", "cancelled", "deleted"} {
		t.Run(gate, func(t *testing.T) {
			r := newRecoveryFixture(t, nil)
			ready := r.create(t, "lifecycle")
			if gate == "deadline" {
				if _, err := r.f.DB.Exec(`UPDATE careeros_draft_actions SET admitted_at=clock_timestamp()-interval '31 minutes',deadline=clock_timestamp()-interval '1 minute' WHERE owner=$1`, r.f.Principal.AccountID); err != nil {
					t.Fatal(err)
				}
			} else if gate == "grant" {
				r.authority(t, `UPDATE controlled_draft_test_authority SET grant_end=clock_timestamp()-interval '1 second',epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID)
			} else {
				r.authority(t, `UPDATE controlled_draft_test_authority SET `+gate+`=true,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID)
			}
			closed := r.request(t, "GET", "/api/drafts/lifecycle", "", nil)
			if closed.Code != 410 || bytes.Contains(closed.Body.Bytes(), []byte(`"bundle"`)) {
				t.Fatalf("early closure failed %d %s", closed.Code, closed.Body.String())
			}
			// A later apparent delivery/rights restoration cannot revive closed work.
			r.authority(t, `UPDATE controlled_draft_test_authority SET deleted=false,revoked=false,cancelled=false,grant_end=clock_timestamp()+interval '1 hour',delivered_action=$2,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID, ready.Action.ID)
			again := r.request(t, "GET", "/api/drafts/lifecycle/versions/1", "", nil)
			if again.Code != 410 || r.sends.Load() != 1 || r.checks.Load() != 1 {
				t.Fatalf("terminal outcome revived: %d %s", again.Code, again.Body.String())
			}
		})
	}
}
func TestControlledDraftDeliveredHistoricalWindowUsesOriginalGrantExpiry(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	ready := r.create(t, "historical-delivered")
	bumpRecoveryProfile(t, r)
	r.authority(t, `UPDATE controlled_draft_test_authority SET allow_generate=false,allow_publish=false,grant_end=clock_timestamp()-interval '29 days',delivered_action=$2,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID, ready.Action.ID)
	if _, err := r.f.DB.Exec(`UPDATE careeros_draft_actions SET grant_expires_at=clock_timestamp()-interval '29 days' WHERE owner=$1`, r.f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	got := r.request(t, "GET", "/api/drafts/historical-delivered/versions/1", "", nil)
	var result draftrecovery.Result
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &result) != nil || !result.Historical || !reflect.DeepEqual(ready.Bundle, result.Bundle) {
		t.Fatalf("historical generation rights imposed: %d %s", got.Code, got.Body.String())
	}
	// Extending TODAY's injected grant cannot extend the ORIGINAL retrieval clock.
	r.authority(t, `UPDATE controlled_draft_test_authority SET grant_end=clock_timestamp()+interval '1 year' WHERE owner=$1`, r.f.Principal.AccountID)
	if _, err := r.f.DB.Exec(`UPDATE careeros_draft_actions SET grant_expires_at=clock_timestamp()-interval '31 days' WHERE owner=$1`, r.f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	closed := r.request(t, "GET", "/api/drafts/historical-delivered", "", nil)
	if closed.Code != 410 || bytes.Contains(closed.Body.Bytes(), []byte(`"bundle"`)) || r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatalf("retrieval clock reset: %d %s", closed.Code, closed.Body.String())
	}
}

func TestControlledDraftFinalDecisionCannotUsePreAdapterClock(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	r.create(t, "adapter-time")
	original := r.config.Decisions
	var calls atomic.Int32
	r.config.Decisions = draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
		d, err := original.Decide(ctx, a)
		if calls.Add(1) == 3 {
			time.Sleep(400 * time.Millisecond)
		}
		return d, err
	})
	if err := r.store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	r.store, err = draftrecovery.Open(context.Background(), r.f.DSN, r.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.store.Close() })
	r.router = accountapi.NewControlledDraftRecovery(r.f.Service, r.store, func(context.Context, draftrecovery.Generation) (draftrecovery.Bundle, error) {
		t.Fatal("GET executed generation")
		return draftrecovery.Bundle{}, nil
	})
	r.authority(t, `UPDATE controlled_draft_test_authority SET grant_end=clock_timestamp()+interval '300 milliseconds' WHERE owner=$1`, r.f.Principal.AccountID)
	w := r.request(t, "GET", "/api/drafts/adapter-time/versions/1", "", nil)
	if calls.Load() != 3 || w.Code != 410 || bytes.Contains(w.Body.Bytes(), []byte(`"bundle"`)) {
		t.Fatalf("final blocked adapter used stale clock: callbacks=%d code=%d %s", calls.Load(), w.Code, w.Body.String())
	}
}
