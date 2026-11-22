package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/draftrecovery"
)

func TestManagedTerminalRetryKeepsItsFirstObservedTerminalFence(t *testing.T) {
	for _, test := range []struct{ fence, phase string }{
		{"cancel", "before_retry"}, {"delete", "before_retry"},
		{"cancel", "after_shared_retry_commit"}, {"delete", "after_shared_retry_commit"},
	} {
		t.Run(test.phase+"/"+test.fence, func(t *testing.T) {
			var terminal atomic.Bool
			cfg := draftrecovery.Config{Decisions: draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
				var expires time.Time
				err := a.Transaction.QueryRowContext(ctx, `SELECT expires_at FROM careeros_pilot_access WHERE account_id=$1`, a.Principal.AccountID).Scan(&expires)
				return draftrecovery.Decision{Generate: true, Publish: true, Read: true, GrantExpiresAt: expires,
					Cancelled: test.fence == "cancel" && terminal.Load(), Deleted: test.fence == "delete" && terminal.Load()}, err
			}), Fault: func(stage string) error {
				if test.phase == "after_shared_retry_commit" && stage == test.phase {
					terminal.Store(true)
				}
				return nil
			}}
			f, h, provider, _, router, input := configuredTerminalDraftFixture(t, cfg)
			var sends atomic.Int32
			if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
				if sends.Add(1) == 2 {
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.WriteString(`{"error":"synthetic_rate_limit"}`)
					return w.Result(), true, nil
				}
				response, err := provider.Respond(ctx, p, r)
				return response, true, err
			}); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(input)
			const key = "retry-observes-terminal-first"
			first := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
			if first.Code < 400 || sends.Load() != 2 {
				t.Fatalf("initial known failure: %d %s sends%d", first.Code, first.Body.String(), sends.Load())
			}
			before := openingRequest(t, f, router, http.MethodGet, "/api/drafts/"+key, nil)
			var recoverable draftrecovery.Result
			if before.Code != http.StatusAccepted || json.Unmarshal(before.Body.Bytes(), &recoverable) != nil || !recoverable.RetryAllowed {
				t.Fatalf("recoverable known failure before fence: %d %s", before.Code, before.Body.String())
			}
			terminal.Store(test.phase == "before_retry")
			denied := qualificationRequest(t, f, router, "/api/drafts", key, "deliberate-retry", body)
			if denied.Code != http.StatusForbidden || sends.Load() != 2 {
				t.Fatalf("Retry must observe the winning terminal fence: %d %s sends%d", denied.Code, denied.Body.String(), sends.Load())
			}
			// A later apparently permissive adapter cannot erase the terminal
			// observation made by the rejected command itself.
			terminal.Store(false)
			after := openingRequest(t, f, router, http.MethodGet, "/api/drafts/"+key, nil)
			var closed draftrecovery.Result
			wantState := "closed"
			if test.fence == "delete" {
				wantState = "deleted"
			}
			if after.Code != http.StatusGone || json.Unmarshal(after.Body.Bytes(), &closed) != nil || closed.State != wantState || closed.Action != recoverable.Action || closed.RetryAllowed || closed.Delivered || closed.Bundle != nil {
				t.Fatalf("rejected Retry lost its terminal observation: %d %s", after.Code, after.Body.String())
			}
			for range 2 {
				retry := qualificationRequest(t, f, router, "/api/drafts", key, "deliberate-retry", body)
				if retry.Code != http.StatusForbidden || sends.Load() != 2 {
					t.Fatalf("terminal Action revived: %d %s sends%d", retry.Code, retry.Body.String(), sends.Load())
				}
			}
			status := openingRequest(t, f, router, http.MethodGet, "/api/draft-allowance", nil)
			var allowance openingAllowance
			if status.Code != http.StatusOK || json.Unmarshal(status.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 0, 0, 5}) {
				t.Fatalf("terminal Retry released its customer reservation once: %d %s", status.Code, status.Body.String())
			}
		})
	}
}
