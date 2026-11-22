package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/testsupport/aiadmission"
)

// A terminal customer/content state is not evidence that an in-flight local
// callback returned, nor that its remote obligation ended.
func TestDeletionKeepsLocalActiveSlotUntilCallbackReturns(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	second := qualificationParticipant(t, f, "capacity-second")
	third := qualificationParticipant(t, f, "capacity-third")
	entered := make(chan string, 2)
	releases := map[string]chan struct{}{f.Principal.AccountID: make(chan struct{}), second.Principal.AccountID: make(chan struct{})}
	done := make(chan *httptest.ResponseRecorder, 2)
	started, returned := 0, 0
	var mu sync.Mutex
	// The only first call is whole-original TypeSafe. Attribute by the callback
	// schedule: first participant is launched and observed before the second.
	var calls int
	if err := h.SetQualificationResponder(func(ctx context.Context, name string, request *http.Request) (*http.Response, bool, error) {
		mu.Lock()
		owner := f.Principal.AccountID
		if calls == 1 {
			owner = second.Principal.AccountID
		}
		calls++
		mu.Unlock()
		entered <- owner
		<-releases[owner]
		mu.Lock()
		defer mu.Unlock()
		response, err := provider.Respond(ctx, name, request)
		return response, false, err // no remote terminal proof
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, release := range releases {
			select {
			case <-release:
			default:
				close(release)
			}
		}
		for returned < started {
			select {
			case <-done:
				returned++
			case <-time.After(5 * time.Second):
				t.Error("local callback did not exit during fixture cleanup")
				return
			}
		}
	}()
	for _, who := range []*aiadmission.Fixture{f, second} {
		body := who.Body(t, "/api/qualification-gaps")
		started++
		go func(who *aiadmission.Fixture) {
			done <- qualificationRequest(t, who, router, "/api/qualification-gaps", "running-deletion", "", body)
		}(who)
		select {
		case owner := <-entered:
			if owner != who.Principal.AccountID {
				t.Fatal("wrong callback owner")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("callback did not enter")
		}
	}
	root := third.Body(t, "/api/qualification-gaps")
	if _, err := h.Open(context.Background(), third.Principal, "before-deletion", "qualification_gaps", root); err != aicontrol.ErrCapacity {
		t.Fatalf("two running callbacks must hold global slots: %v", err)
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.StartDeletion(context.Background(), f.Principal, accountstore.DeletionRequest{OperationID: "capacity-delete", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Open(context.Background(), third.Principal, "after-deletion", "qualification_gaps", root); err != aicontrol.ErrCapacity {
		t.Fatalf("deletion freed still-running global slot: %v", err)
	}
	close(releases[f.Principal.AccountID])
	select {
	case response := <-done:
		returned++
		if response.Code < 400 || response.Header().Get("X-CareerOS-Draft-Context-Proof") != "" {
			t.Fatalf("deleted result/proof released: %d", response.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deleted callback did not return")
	}
	if _, err := h.Open(context.Background(), second.Principal, "same-account", "qualification_gaps", second.Body(t, "/api/qualification-gaps")); err != aicontrol.ErrCapacity {
		t.Fatalf("remaining callback must retain its account slot: %v", err)
	}
	if _, err := h.Open(context.Background(), third.Principal, "after-return", "qualification_gaps", root); err != nil {
		t.Fatalf("truthful local return did not release only its global slot: %v", err)
	}
	if response := qualificationRequest(t, third, router, "/api/qualification-gaps", "after-return", "", root); response.Code < 400 {
		t.Fatalf("local return released unknown remote capacity: %d", response.Code)
	}
	for _, scope := range []string{"global", "account:" + f.Principal.AccountID, "provider:typesafe"} {
		status, err := h.ScopeStatus(context.Background(), f.Principal, scope)
		wantCount, wantOccupied := int64(2688), 2
		if scope == "account:"+f.Principal.AccountID {
			wantCount, wantOccupied = 1344, 1
		}
		if err != nil || status.Counted != wantCount || status.Occupied != wantOccupied {
			t.Fatalf("local return changed %s remote/money holds: %+v %v", scope, status, err)
		}
	}
	var state string
	var finished bool
	if err := f.DB.QueryRow(`SELECT state,execution_finished FROM ai_fixture_action WHERE owner=$1 AND creation_key='running-deletion'`, f.Principal.AccountID).Scan(&state, &finished); err != nil || state != "deleted" || finished {
		t.Fatalf("local return changed deleted execution outcome: %s %t %v", state, finished, err)
	}
	close(releases[second.Principal.AccountID])
	<-done
	returned++
}

func TestDeletionLocalReturnRequiresEveryInvocation(t *testing.T) {
	for _, kind := range []string{"profile", "account"} {
		t.Run(kind, func(t *testing.T) {
			f, h, _, _ := qualificationFixture(t)
			second := qualificationParticipant(t, f, "return-second")
			third := qualificationParticipant(t, f, "return-third")
			ctx := context.Background()
			session, err := h.Open(ctx, f.Principal, "local-invocations", "qualification_gaps", f.Body(t, "/api/qualification-gaps"))
			if err != nil {
				t.Fatal(err)
			}
			first, err := session.BeginLocalExecution(ctx)
			if err != nil {
				t.Fatal(err)
			}
			secondReturn, err := session.BeginLocalExecution(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = h.Open(ctx, second.Principal, "other-active", "qualification_gaps", second.Body(t, "/api/qualification-gaps")); err != nil {
				t.Fatal(err)
			}
			var doc struct {
				ID       string `json:"id"`
				Revision int64  `json:"revision"`
			}
			if err = json.Unmarshal(f.Document, &doc); err != nil {
				t.Fatal(err)
			}
			request := accountstore.DeletionRequest{OperationID: "local-delete", Kind: kind, Confirm: true}
			if kind == "profile" {
				request.ProfileID, request.Revision = doc.ID, doc.Revision
			}
			if _, err = f.Store.StartDeletion(ctx, f.Principal, request); err != nil {
				t.Fatal(err)
			}
			first()
			first() // duplicate return cannot retire the other invocation
			root := third.Body(t, "/api/qualification-gaps")
			if _, err = h.Open(ctx, third.Principal, "one-return", "qualification_gaps", root); err != aicontrol.ErrCapacity {
				t.Fatalf("another invocation was released: %v", err)
			}
			if _, err = session.BeginLocalExecution(ctx); err == nil {
				t.Fatal("deleted/closed authority started another invocation")
			}
			secondReturn() // code-owned local return still works after account closure
			if _, err = h.Open(ctx, third.Principal, "all-returned", "qualification_gaps", root); err != nil {
				t.Fatalf("exact local returns did not release terminal slot: %v", err)
			}
			var running, returned int
			if err = f.DB.QueryRow(`SELECT count(*) FILTER(WHERE receipt_id=''),count(*) FILTER(WHERE receipt_id='returned') FROM ai_fixture_stage WHERE namespace=$1 AND action=$2 AND stage='local_execution'`, h.Namespace(), session.ID()).Scan(&running, &returned); err != nil || running != 0 || returned != 2 {
				t.Fatalf("wrong exact invocation bookkeeping: %d %d %v", running, returned, err)
			}
		})
	}
}

func TestDeletionOlderActionWithoutLocalReturnEvidenceStaysHeld(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	second := qualificationParticipant(t, f, "legacy-second")
	third := qualificationParticipant(t, f, "legacy-third")
	ctx := context.Background()
	for _, who := range []*aiadmission.Fixture{f, second} {
		if _, err := h.Open(ctx, who.Principal, "inherited-no-return", "qualification_gaps", who.Body(t, "/api/qualification-gaps")); err != nil {
			t.Fatal(err)
		}
	}
	// Literal pre-upgrade shape: old Actions have no local tracking/return
	// witnesses. No timestamp inference or migration fabricates their return.
	if _, err := f.DB.Exec(`UPDATE ai_fixture_action SET local_execution_tracking=false`); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Store.StartDeletion(ctx, f.Principal, accountstore.DeletionRequest{OperationID: "older-delete", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Open(ctx, third.Principal, "no-return-proof", "qualification_gaps", third.Body(t, "/api/qualification-gaps")); err != aicontrol.ErrCapacity {
		t.Fatalf("missing inherited/crash return evidence freed slot: %v", err)
	}
}

func TestDeletionBeforeLocalExecutionDoesNotLeakSlot(t *testing.T) {
	for _, retry := range []bool{false, true} {
		name := "initial"
		if retry {
			name = "attempt2"
		}
		t.Run(name, func(t *testing.T) {
			f, h, provider, router := qualificationFixture(t)
			second := qualificationParticipant(t, f, "gap-second")
			third := qualificationParticipant(t, f, "gap-third")
			ctx := context.Background()
			root := f.Body(t, "/api/qualification-gaps")
			if retry {
				if err := h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
					w := httptest.NewRecorder()
					w.WriteHeader(429)
					return w.Result(), true, nil
				}); err != nil {
					t.Fatal(err)
				}
				if response := qualificationRequest(t, f, router, "/api/qualification-gaps", "not-started", "", root); response.Code < 400 || response.Header().Get("X-CareerOS-Retry-Allowed") != "true" {
					t.Fatal("expected known initial failure", response.Code)
				}
			}
			session, err := h.Open(ctx, f.Principal, "not-started", "qualification_gaps", root)
			if err != nil {
				t.Fatal(err)
			}
			if retry {
				session, err = session.RetryQualification(ctx, "deliberate-attempt2")
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err = h.Open(ctx, second.Principal, "other-admission", "qualification_gaps", second.Body(t, "/api/qualification-gaps")); err != nil {
				t.Fatal(err)
			}
			var doc struct {
				ID       string `json:"id"`
				Revision int64  `json:"revision"`
			}
			if err = json.Unmarshal(f.Document, &doc); err != nil {
				t.Fatal(err)
			}
			if _, err = f.Store.StartDeletion(ctx, f.Principal, accountstore.DeletionRequest{OperationID: "gap-delete", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true}); err != nil {
				t.Fatal(err)
			}
			if _, err = session.BeginLocalExecution(ctx); err == nil {
				t.Fatal("admission-gap deletion started callback")
			}
			if _, err = h.Open(ctx, third.Principal, "gap-recovered", "qualification_gaps", third.Body(t, "/api/qualification-gaps")); err != nil {
				t.Fatalf("known never-started invocation leaked capacity: %v", err)
			}
			var intents, running int
			if err = f.DB.QueryRow(`SELECT (SELECT count(*) FROM ai_fixture_dispatch),(SELECT count(*) FROM ai_fixture_stage WHERE stage='local_execution' AND receipt_id='')`).Scan(&intents, &running); err != nil {
				t.Fatal(err)
			}
			want := 0
			if retry {
				want = 1
			}
			if intents != want || running != 0 || len(provider.Calls) != 0 {
				t.Fatalf("denied start created work: %d %d", intents, running)
			}
		})
	}
}

func TestDeletionInheritedRowIgnoresNewerLocalReturn(t *testing.T) {
	f, h, _, _ := qualificationFixture(t)
	second := qualificationParticipant(t, f, "untracked-second")
	third := qualificationParticipant(t, f, "untracked-third")
	ctx := context.Background()
	session, err := h.Open(ctx, f.Principal, "inherited-execution", "qualification_gaps", f.Body(t, "/api/qualification-gaps"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.DB.Exec(`UPDATE ai_fixture_action SET local_execution_tracking=false WHERE namespace=$1 AND id=$2`, h.Namespace(), session.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = h.Open(ctx, second.Principal, "other-execution", "qualification_gaps", second.Body(t, "/api/qualification-gaps")); err != nil {
		t.Fatal(err)
	}
	returned, err := session.BeginLocalExecution(ctx)
	if err != nil {
		t.Fatal(err)
	}
	returned() // this invocation cannot attest to a pre-upgrade untracked one
	var doc struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err = json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store.StartDeletion(ctx, f.Principal, accountstore.DeletionRequest{OperationID: "untracked-delete", Kind: "profile", ProfileID: doc.ID, Revision: doc.Revision, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.Open(ctx, third.Principal, "not-all-returned", "qualification_gaps", third.Body(t, "/api/qualification-gaps")); err != aicontrol.ErrCapacity {
		t.Fatalf("newer return purported to finish inherited unknown execution: %v", err)
	}
}
