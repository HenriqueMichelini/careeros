package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/draftrecovery"
)

func TestManagedTerminalOpeningAndCancellationPreserveTheWinningEffect(t *testing.T) {
	for _, openingFirst := range []bool{false, true} {
		name := "cancel_first"
		if openingFirst {
			name = "opening_first"
		}
		t.Run(name, func(t *testing.T) {
			f, _, provider, _, router, input := terminalDraftFixture(t)
			body, _ := json.Marshal(input)
			const key = "terminal-ordered-opening"
			created := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
			var ready draftrecovery.Result
			if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &ready) != nil || ready.Bundle == nil {
				t.Fatalf("complete candidate: %d %s", created.Code, created.Body.String())
			}
			issued := openingRequest(t, f, router, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening", struct{}{})
			var receipt draftrecovery.OpeningReceipt
			if issued.Code != http.StatusOK || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
				t.Fatalf("bound receipt: %d %s", issued.Code, issued.Body.String())
			}
			confirm := func() *httptest.ResponseRecorder {
				return openingRequest(t, f, router, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm",
					draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
			}
			var delivered draftrecovery.Result
			if openingFirst {
				w := confirm()
				if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &delivered) != nil || !delivered.Delivered {
					t.Fatalf("opening wins: %d %s", w.Code, w.Body.String())
				}
			}
			canceled := openingRequest(t, f, router, http.MethodPost, "/api/drafts/"+key+"/cancel", map[string]any{"action": ready.Action})
			var outcome draftrecovery.Result
			if canceled.Code != http.StatusOK || json.Unmarshal(canceled.Body.Bytes(), &outcome) != nil {
				t.Fatalf("cancel outcome: %d %s", canceled.Code, canceled.Body.String())
			}
			wantAllowance := openingAllowance{5, 0, 0, 5}
			if openingFirst {
				wantAllowance = openingAllowance{5, 0, 1, 4}
				if !reflect.DeepEqual(outcome, delivered) {
					t.Fatalf("later cancellation changed delivered work: %+v", outcome)
				}
			} else {
				if outcome.State != "closed" || outcome.Delivered || outcome.Bundle != nil || outcome.Opening == nil || outcome.Opening.State != "released" {
					t.Fatalf("cancellation did not close without consumption: %+v", outcome)
				}
				if w := confirm(); w.Code != http.StatusForbidden {
					t.Fatalf("late confirmation revived canceled work: %d %s", w.Code, w.Body.String())
				}
			}
			status := openingRequest(t, f, router, http.MethodGet, "/api/draft-allowance", nil)
			var allowance openingAllowance
			if status.Code != http.StatusOK || json.Unmarshal(status.Body.Bytes(), &allowance) != nil || allowance != wantAllowance || len(provider.Calls) != 4 {
				t.Fatalf("one authoritative outcome: %d %s sends%v", status.Code, status.Body.String(), provider.Calls)
			}
		})
	}
}

func TestManagedTerminalRunningWorkRecoversFirstAndKeepsUnknownProviderHold(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseProvider := func() { once.Do(func() { close(release) }) }
	defer releaseProvider()
	var sends atomic.Int32
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		n := sends.Add(1)
		if n == 2 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}
		response, err := provider.Respond(ctx, p, r)
		return response, n != 2, err // No independent terminal proof for the generation.
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(input)
	const key = "terminal-running-status"
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- qualificationRequest(t, f, router, "/api/drafts", key, "", body) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("controlled generation did not reach its in-flight boundary")
	}
	status := openingRequest(t, f, router, http.MethodGet, "/api/drafts/"+key, nil)
	var pending draftrecovery.Result
	if status.Code != http.StatusAccepted || json.Unmarshal(status.Body.Bytes(), &pending) != nil || pending.State != "pending" || pending.RetryAllowed || sends.Load() != 2 {
		t.Fatalf("running status is not a known failure: %d %s sends%d", status.Code, status.Body.String(), sends.Load())
	}
	for _, retry := range []string{"", "not-known-failure"} {
		response := qualificationRequest(t, f, router, "/api/drafts", key, retry, body)
		if (retry == "" && response.Code != http.StatusAccepted) || (retry != "" && response.Code != http.StatusForbidden) || sends.Load() != 2 {
			t.Fatalf("running/unknown replay cannot dispatch: %d %s sends%d", response.Code, response.Body.String(), sends.Load())
		}
	}
	canceled := openingRequest(t, f, router, http.MethodPost, "/api/drafts/"+key+"/cancel", map[string]any{"action": pending.Action})
	if canceled.Code != http.StatusOK {
		t.Fatalf("confirmed running cancellation: %d %s", canceled.Code, canceled.Body.String())
	}
	for _, scope := range []string{"account:" + f.Principal.AccountID, "global"} {
		s, err := h.ScopeStatus(context.Background(), f.Principal, scope)
		if err != nil || s.Counted != 13_344 || s.Occupied != 1 {
			t.Fatalf("customer cancellation erased unknown provider obligation: %s %+v %v", scope, s, err)
		}
	}
	releaseProvider()
	select {
	case late := <-done:
		if late.Code < 400 || sends.Load() != 2 {
			t.Fatalf("late work continued or published after cancellation: %d %s sends%d", late.Code, late.Body.String(), sends.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late controlled response did not finish")
	}
	recovered := openingRequest(t, f, router, http.MethodGet, "/api/drafts/"+key, nil)
	var closed draftrecovery.Result
	if recovered.Code != http.StatusGone || json.Unmarshal(recovered.Body.Bytes(), &closed) != nil || closed.State != "closed" || closed.Delivered || closed.Bundle != nil {
		t.Fatalf("late response revived closed content: %d %s", recovered.Code, recovered.Body.String())
	}
	for _, scope := range []string{"account:" + f.Principal.AccountID, "global"} {
		s, err := h.ScopeStatus(context.Background(), f.Principal, scope)
		if err != nil || s.Counted != 13_344 || s.Occupied != 1 {
			t.Fatalf("a late local response is not terminal proof: %s %+v %v", scope, s, err)
		}
	}
	allowanceReply := openingRequest(t, f, router, http.MethodGet, "/api/draft-allowance", nil)
	var allowance openingAllowance
	if allowanceReply.Code != http.StatusOK || json.Unmarshal(allowanceReply.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 0, 0, 5}) {
		t.Fatalf("running cancellation customer outcome: %d %s", allowanceReply.Code, allowanceReply.Body.String())
	}
}
