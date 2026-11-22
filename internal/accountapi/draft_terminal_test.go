package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func terminalDraftFixture(t *testing.T) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, *draftrecovery.Store, http.Handler, draftrecovery.Input) {
	t.Helper()
	return configuredTerminalDraftFixture(t, draftrecovery.Config{})
}

func configuredTerminalDraftFixture(t *testing.T, cfg draftrecovery.Config) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, *draftrecovery.Store, http.Handler, draftrecovery.Input) {
	t.Helper()
	f, h, provider, _, _, input := managedDraftFixture(t)
	cfg.Actions, cfg.BindingKey = h, []byte("synthetic-managed-draft-binding-key")
	store, err := draftrecovery.OpenManagedTerminal(context.Background(), f.DSN, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err = store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f, h, provider, store, accountapi.NewSyntheticTerminalDraft(f.Service, f.Store, h, store), input
}

func TestManagedTerminalRetryPreservesOriginalActionWindowAndOneDelivery(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	sends := 0
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		sends++
		if sends == 2 {
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
	const key = "terminal-known-failure"
	first := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	if first.Code < 400 || sends != 2 {
		t.Fatalf("known initial failure: %d %s sends%d", first.Code, first.Body.String(), sends)
	}
	status := openingRequest(t, f, router, http.MethodGet, "/api/drafts/"+key, nil)
	var before draftrecovery.Result
	if status.Code != http.StatusAccepted || json.Unmarshal(status.Body.Bytes(), &before) != nil || before.State != "incomplete" || !before.RetryAllowed || before.Attempt != 1 || before.Opening == nil || before.Opening.State != "reserved" || sends != 2 {
		t.Fatalf("recover known failure before deliberate Retry: %d %s sends%d", status.Code, status.Body.String(), sends)
	}
	const retryKey = "one-deliberate-terminal-retry"
	retried := qualificationRequest(t, f, router, "/api/drafts", key, retryKey, body)
	var ready draftrecovery.Result
	if retried.Code != http.StatusCreated || json.Unmarshal(retried.Body.Bytes(), &ready) != nil || ready.State != "ready" || ready.Attempt != 2 || ready.Action != before.Action || !ready.AdmittedAt.Equal(before.AdmittedAt) || !ready.Deadline.Equal(before.Deadline) || sends != 6 {
		t.Fatalf("same Action Retry preserves original window: %d %s sends%d", retried.Code, retried.Body.String(), sends)
	}
	issued := openingRequest(t, f, router, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if issued.Code != http.StatusOK || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
		t.Fatalf("complete Retry receipt: %d %s", issued.Code, issued.Body.String())
	}
	confirmation := draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest}
	confirmed := openingRequest(t, f, router, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm", confirmation)
	var delivered draftrecovery.Result
	if confirmed.Code != http.StatusOK || json.Unmarshal(confirmed.Body.Bytes(), &delivered) != nil || !delivered.Delivered || delivered.Action != before.Action || sends != 6 {
		t.Fatalf("one eventual Retry delivery: %d %s sends%d", confirmed.Code, confirmed.Body.String(), sends)
	}
	for _, retry := range []string{retryKey, "forbidden-third-attempt"} {
		response := qualificationRequest(t, f, router, "/api/drafts", key, retry, body)
		if sends != 6 || (retry == retryKey && response.Code != http.StatusOK) || (retry != retryKey && response.Code < 400) {
			t.Fatalf("Retry identity/replay sent again: key%s status%d sends%d", retry, response.Code, sends)
		}
	}
	allowanceReply := openingRequest(t, f, router, http.MethodGet, "/api/draft-allowance", nil)
	var allowance openingAllowance
	if allowanceReply.Code != http.StatusOK || json.Unmarshal(allowanceReply.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 0, 1, 4}) {
		t.Fatalf("attempt count is not delivery count: %d %s", allowanceReply.Code, allowanceReply.Body.String())
	}
	counted, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || counted.Counted != 48_688 || counted.Occupied != 0 {
		t.Fatalf("both reached attempts retain independent expense: %+v %v", counted, err)
	}
	another := qualificationRequest(t, f, router, "/api/drafts", "explicit-another-same-input", "", body)
	var newAction draftrecovery.Result
	if another.Code != http.StatusCreated || json.Unmarshal(another.Body.Bytes(), &newAction) != nil || newAction.Action == before.Action || sends != 10 {
		t.Fatalf("deliberate generate-another is a new Action: %d %s sends%d", another.Code, another.Body.String(), sends)
	}
	recovered := openingRequest(t, f, router, http.MethodGet, "/api/drafts/"+key, nil)
	var old draftrecovery.Result
	if recovered.Code != http.StatusOK || json.Unmarshal(recovered.Body.Bytes(), &old) != nil || !reflect.DeepEqual(old, delivered) || sends != 10 {
		t.Fatalf("new candidate replaced original delivered work: %d %s sends%d", recovered.Code, recovered.Body.String(), sends)
	}
}

func TestManagedDraftCancellationClosesUnopenedWithoutConsumption(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	const key = "terminal-cancel-tracer"
	created := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
	var ready draftrecovery.Result
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &ready) != nil || ready.State != "ready" || ready.Bundle == nil || ready.Delivered {
		t.Fatalf("complete unopened managed Action: %d %s", created.Code, created.Body.String())
	}
	if len(provider.Calls) != 4 {
		t.Fatalf("initial managed graph sent %d calls, want four", len(provider.Calls))
	}
	command := map[string]any{"action": ready.Action}
	path := "/api/drafts/" + key + "/cancel"
	canceled := openingRequest(t, f, router, http.MethodPost, path, command)
	var terminal draftrecovery.Result
	if canceled.Code != http.StatusOK || json.Unmarshal(canceled.Body.Bytes(), &terminal) != nil || terminal.Action != ready.Action ||
		terminal.State != "closed" || terminal.Bundle != nil || terminal.Delivered || !terminal.CleanupQueued || terminal.Opening == nil || terminal.Opening.State != "released" {
		t.Fatalf("authoritative pre-opening cancellation: %d %s", canceled.Code, canceled.Body.String())
	}
	for i := 0; i < 2; i++ {
		replay := openingRequest(t, f, router, http.MethodPost, path, command)
		var recovered draftrecovery.Result
		if replay.Code != http.StatusOK || json.Unmarshal(replay.Body.Bytes(), &recovered) != nil || !reflect.DeepEqual(recovered, terminal) {
			t.Fatalf("same cancellation effect on replay: %d %s", replay.Code, replay.Body.String())
		}
	}
	recoveredReply := openingRequest(t, f, router, http.MethodGet, "/api/drafts/"+key+"/versions/1", nil)
	var recovered draftrecovery.Result
	if recoveredReply.Code != http.StatusGone || json.Unmarshal(recoveredReply.Body.Bytes(), &recovered) != nil || !reflect.DeepEqual(recovered, terminal) {
		t.Fatalf("terminal exact-version recovery: %d %s", recoveredReply.Code, recoveredReply.Body.String())
	}
	late := openingRequest(t, f, router, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening", struct{}{})
	if late.Code != http.StatusForbidden {
		t.Fatalf("late opening must remain ineligible: %d %s", late.Code, late.Body.String())
	}
	allowanceReply := openingRequest(t, f, router, http.MethodGet, "/api/draft-allowance", nil)
	var allowance openingAllowance
	if allowanceReply.Code != http.StatusOK || json.Unmarshal(allowanceReply.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 0, 0, 5}) {
		t.Fatalf("terminal no-consumption releases only the customer right: %d %s", allowanceReply.Code, allowanceReply.Body.String())
	}
	for _, scope := range []string{"account:" + f.Principal.AccountID, "global"} {
		status, err := h.ScopeStatus(context.Background(), f.Principal, scope)
		if err != nil || status.Counted != 35_344 || status.Occupied != 0 {
			t.Fatalf("provider evidence is independent of customer cancellation: %s %+v %v", scope, status, err)
		}
	}
	if len(provider.Calls) != 4 {
		t.Fatalf("cancellation/recovery sent extra inference: %v", provider.Calls)
	}
}
