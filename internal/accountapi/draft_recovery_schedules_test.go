package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
)

func TestControlledDraftLostAcknowledgmentsNeverRepeatGenerationOrCheck(t *testing.T) {
	for _, stage := range []string{"after_create_commit", "after_publish_commit"} {
		t.Run(stage, func(t *testing.T) {
			r := newRecoveryFixture(t, nil)
			var lost atomic.Bool
			r.fault = func(point string) error {
				if point == stage && lost.CompareAndSwap(false, true) {
					return errors.New("synthetic lost acknowledgment")
				}
				return nil
			}
			first := r.request(t, "POST", "/api/drafts", "lost-response", r.input(t))
			if first.Code != 503 {
				t.Fatalf("fault did not hide acknowledgment: %d %s", first.Code, first.Body.String())
			}
			before := r.request(t, "GET", "/api/drafts/lost-response/versions/1", "", nil)
			var result draftrecovery.Result
			if json.Unmarshal(before.Body.Bytes(), &result) != nil {
				t.Fatal(before.Body.String())
			}
			wantCalls := int32(0)
			wantState := "pending"
			if stage == "after_publish_commit" {
				wantCalls = 1
				wantState = "ready"
			}
			if result.State != wantState || r.sends.Load() != wantCalls || r.checks.Load() != wantCalls {
				t.Fatalf("lost response changed work: state=%s generation=%d check=%d", result.State, r.sends.Load(), r.checks.Load())
			}
			if err := r.store.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			r.store, err = draftrecovery.Open(context.Background(), r.f.DSN, r.config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { r.store.Close() })
			r.router = accountapi.NewControlledDraftRecovery(r.f.Service, r.store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
				r.sends.Add(1)
				return completeRecoveryBundle(g), nil
			})
			for _, method := range []string{"GET", "POST"} {
				path := "/api/drafts/lost-response/versions/1"
				var input any
				if method == "POST" {
					path = "/api/drafts"
					input = r.input(t)
				}
				got := r.request(t, method, path, "lost-response", input)
				if !bytes.Equal(before.Body.Bytes(), got.Body.Bytes()) || r.sends.Load() != wantCalls || r.checks.Load() != wantCalls {
					t.Fatalf("reopen/replay changed exact result or executed work: %s %d %s", method, got.Code, got.Body.String())
				}
			}
		})
	}
}

func TestControlledDraftCleanupPreservesSharedUnknownExpenseCapacityAndRetryDenial(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	root := []byte(`{"synthetic":"unrelated unknown call"}`)
	unknown, err := r.actions.Open(context.Background(), r.f.Principal, "unknown-hold", "cv_generation", root)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	r.actions.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("synthetic unknown provider outcome")
	})
	ctx, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(context.Background(), "cv_generation", root), "generation", "cv-policy-v2/schema-v2")
	request, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", strings.NewReader(`{"model":"gpt-6-luna","reasoning_effort":"none","max_completion_tokens":4,"messages":[{"role":"user","content":"synthetic"}]}`))
	if _, err = unknown.Send(ctx, "openai", request); err == nil {
		t.Fatal("fixture outcome unexpectedly known")
	}
	before, err := r.actions.Status(context.Background(), r.f.Principal, unknown.ID())
	if err != nil || len(before.Dispatches) != 1 || before.Dispatches[0].Remote != "possibly_active" {
		t.Fatalf("unknown intent not exercised: %+v %v", before, err)
	}
	scopes := []string{"global", "provider:openai", "account:" + r.f.Principal.AccountID}
	prior := map[string]any{}
	for _, scope := range scopes {
		v, e := r.actions.ScopeStatus(context.Background(), r.f.Principal, scope)
		if e != nil || v.Counted <= 0 || v.Occupied != 1 {
			t.Fatalf("unknown authority not occupied: %s %+v %v", scope, v, e)
		}
		prior[scope] = v
	}
	if _, err = unknown.Retry(context.Background()); err == nil {
		t.Fatal("unknown outcome allowed Retry before cleanup")
	}
	r.create(t, "purpose-with-unrelated-hold")
	r.authority(t, `UPDATE controlled_draft_test_authority SET revoked=true,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID)
	_ = r.request(t, "GET", "/api/drafts/purpose-with-unrelated-hold", "", nil)
	if err = r.store.Cleanup(context.Background(), r.f.Principal, "purpose-with-unrelated-hold"); err != nil {
		t.Fatal(err)
	}
	after, err := r.actions.Status(context.Background(), r.f.Principal, unknown.ID())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("cleanup changed unknown status: before=%+v after=%+v err=%v", before, after, err)
	}
	for _, scope := range scopes {
		v, e := r.actions.ScopeStatus(context.Background(), r.f.Principal, scope)
		if e != nil || !reflect.DeepEqual(prior[scope], v) {
			t.Fatalf("cleanup released shared authority %s: before=%+v after=%+v %v", scope, prior[scope], v, e)
		}
	}
	if _, err = unknown.Retry(context.Background()); err == nil || calls.Load() != 1 {
		t.Fatalf("cleanup reopened unknown Retry/resend: err=%v calls=%d", err, calls.Load())
	}
}

func TestControlledDraftUsesSharedActionAndOriginalAdmissionClock(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	in := r.input(t)
	raw, _ := json.Marshal(in)
	shared, err := r.actions.Open(context.Background(), r.f.Principal, "original-admission", "application_draft", raw)
	if err != nil {
		t.Fatal(err)
	}
	ref := shared.ActionReference()
	var original time.Time
	if err = r.f.DB.QueryRow("SELECT clock_timestamp()-interval '5 minutes'").Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err = r.f.DB.Exec(`UPDATE ai_fixture_action SET created_at=$3 WHERE namespace=$1 AND id=$2`, ref.Namespace, ref.ID, original); err != nil {
		t.Fatal(err)
	}
	got := r.create(t, "original-admission")
	if got.Action.Namespace != ref.Namespace || got.Action.ID != ref.ID || got.Action.Owner != ref.Owner || got.Action.Workflow != ref.Workflow || !got.AdmittedAt.Equal(original) || !got.Deadline.Equal(original.Add(30*time.Minute)) {
		t.Fatalf("parallel identity or reset admission: %+v shared=%+v original=%s", got, ref, original)
	}
	if _, err = r.f.DB.Exec(`UPDATE careeros_draft_actions SET action_id='unlinked-result-action' WHERE namespace=$1 AND action_id=$2`, ref.Namespace, ref.ID); err == nil {
		t.Fatal("result Action is not constrained to the actual shared identity")
	}
	state, err := r.actions.Status(context.Background(), r.f.Principal, ref.ID)
	if err != nil || state.CustomerReserved || state.CustomerEffect || len(state.Dispatches) != 0 {
		t.Fatalf("controlled recovery invented delivery/allowance/provider expense: %+v %v", state, err)
	}
}

func bumpRecoveryProfile(t *testing.T, r *recoveryFixture) {
	t.Helper()
	var doc profiledocument.Document
	if err := json.Unmarshal(r.f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	expected := doc.Revision
	doc.Revision++
	for i := range doc.Facts {
		if doc.Facts[i].Owner.ID == doc.ID {
			doc.Facts[i].Owner.Revision = doc.Revision
		}
	}
	raw, _ := json.Marshal(doc)
	if _, err := r.f.Store.SaveProfile(context.Background(), r.f.Principal, "profile-change", expected, raw); err != nil {
		t.Fatal(err)
	}
	r.f.Document = raw
}
func TestControlledDraftRejectsStalePublicationButKeepsPriorVersion(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	prior := r.create(t, "prior-version")
	r.provider = func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		bumpRecoveryProfile(t, r)
		return completeRecoveryBundle(g), nil
	}
	w := r.request(t, "POST", "/api/drafts", "changed-source", r.input(t))
	if w.Code != 409 {
		t.Fatalf("stale publication: %d %s", w.Code, w.Body.String())
	}
	status := r.request(t, "GET", "/api/drafts/changed-source", "", nil)
	var incomplete draftrecovery.Result
	if json.Unmarshal(status.Body.Bytes(), &incomplete) != nil || incomplete.Bundle != nil || incomplete.Version != 0 {
		t.Fatalf("stale candidate became ready: %s", status.Body.String())
	}
	recovered := r.request(t, "GET", "/api/drafts/prior-version/versions/1", "", nil)
	var current draftrecovery.Result
	if recovered.Code != 200 || json.Unmarshal(recovered.Body.Bytes(), &current) != nil || !current.Historical || !reflect.DeepEqual(prior.Bundle, current.Bundle) || r.sends.Load() != 2 || r.checks.Load() != 1 {
		t.Fatalf("prior complete version lost or regenerated: %d %s", recovered.Code, recovered.Body.String())
	}
}

func TestControlledDraftClosureRemovesOnlyQueuedPurposeContent(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	ready := r.create(t, "cleanup-purpose")
	r.authority(t, `UPDATE controlled_draft_test_authority SET revoked=true,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID)
	w := r.request(t, "GET", "/api/drafts/cleanup-purpose/versions/1", "", nil)
	var closed draftrecovery.Result
	if w.Code != 410 || json.Unmarshal(w.Body.Bytes(), &closed) != nil || closed.State != "closed" || !closed.CleanupQueued || closed.Bundle != nil {
		t.Fatalf("closure exposed content: %d %s", w.Code, w.Body.String())
	}
	if err := r.store.Cleanup(context.Background(), r.f.Principal, "cleanup-purpose"); err != nil {
		t.Fatal(err)
	}
	profile, err := r.f.Store.ReadProfile(context.Background(), r.f.Principal, "")
	if err != nil || !bytes.Equal(profile.Document, r.f.Document) {
		t.Fatalf("cleanup touched independent Profile: %v", err)
	}
	state, err := r.actions.Status(context.Background(), r.f.Principal, ready.Action.ID)
	if err != nil || state.State != "closed" || state.CustomerEffect {
		t.Fatalf("shared terminal status not preserved: %+v %v", state, err)
	}
	if replay := r.request(t, "POST", "/api/drafts", "cleanup-purpose", r.input(t)); replay.Code != 410 || r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatalf("closed creation revived: %d %s", replay.Code, replay.Body.String())
	}
}

func TestControlledDraftAdmissionGapRecoveryAndTerminalEpoch(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprint(terminal), func(t *testing.T) {
			r := newRecoveryFixture(t, nil)
			var once atomic.Bool
			r.fault = func(stage string) error {
				if stage == "after_shared_action_commit" && once.CompareAndSwap(false, true) {
					if terminal {
						r.authority(t, `UPDATE controlled_draft_test_authority SET revoked=true,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID)
						r.authority(t, `UPDATE controlled_draft_test_authority SET revoked=false,epoch=epoch+1 WHERE owner=$1`, r.f.Principal.AccountID)
					}
					return errors.New("lost between shared admission and recovery witness")
				}
				return nil
			}
			first := r.request(t, "POST", "/api/drafts", "admission-gap", r.input(t))
			if first.Code != 503 {
				t.Fatalf("gap fault not reached: %d %s", first.Code, first.Body.String())
			}
			var id string
			var admitted time.Time
			if err := r.f.DB.QueryRow(`SELECT id,created_at FROM ai_fixture_action WHERE owner=$1 AND creation_key='admission-gap'`, r.f.Principal.AccountID).Scan(&id, &admitted); err != nil {
				t.Fatal(err)
			}
			recovered := r.request(t, "GET", "/api/drafts/admission-gap", "", nil)
			var status draftrecovery.Result
			if json.Unmarshal(recovered.Body.Bytes(), &status) != nil || status.Action.ID != id || !status.AdmittedAt.Equal(admitted) || r.sends.Load() != 0 || r.checks.Load() != 0 {
				t.Errorf("gap lost shared identity/status: %d %s", recovered.Code, recovered.Body.String())
			}
			replay := r.request(t, "POST", "/api/drafts", "admission-gap", r.input(t))
			if terminal {
				if recovered.Code != 410 || replay.Code != 410 || r.sends.Load() != 0 || r.checks.Load() != 0 {
					t.Fatalf("terminal epoch revived: status=%d replay=%d %s gen=%d check=%d", recovered.Code, replay.Code, replay.Body.String(), r.sends.Load(), r.checks.Load())
				}
			} else {
				var ready draftrecovery.Result
				if replay.Code != 201 || json.Unmarshal(replay.Body.Bytes(), &ready) != nil || ready.Action.ID != id || !ready.AdmittedAt.Equal(admitted) || r.sends.Load() != 1 || r.checks.Load() != 1 {
					t.Fatalf("unchanged gap did not resume exactly once: %d %s", replay.Code, replay.Body.String())
				}
				duplicate := r.request(t, "POST", "/api/drafts", "admission-gap", r.input(t))
				if duplicate.Code != 200 || r.sends.Load() != 1 || r.checks.Load() != 1 {
					t.Fatal("gap replay repeated controlled work")
				}
			}
		})
	}
}
