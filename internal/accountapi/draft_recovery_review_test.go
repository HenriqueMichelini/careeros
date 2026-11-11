package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/draftrecovery"
)

func TestControlledDraftFinalReadDecisionOnAccountAlias(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	r.create(t, "last-read")
	path := "/.netlify/functions/account/api/drafts/last-read/versions/1"
	before := r.request(t, "GET", path, "", nil)
	if before.Code != http.StatusOK {
		t.Fatal(before.Code)
	}

	original := r.config.Decisions
	calls := 0
	r.config.Decisions = draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
		d, err := original.Decide(ctx, a)
		calls++
		if calls == 3 {
			d.Read = false
		}
		return d, err
	})
	reviewed, err := draftrecovery.Open(context.Background(), r.f.DSN, r.config)
	if err != nil {
		t.Fatal(err)
	}
	defer reviewed.Close()
	r.router = accountapi.NewControlledDraftRecovery(r.f.Service, reviewed, func(context.Context, draftrecovery.Generation) (draftrecovery.Bundle, error) {
		t.Fatal("read invoked generation")
		return draftrecovery.Bundle{}, nil
	})
	rejected := r.request(t, "GET", path, "", nil)
	if calls != 3 || rejected.Code != http.StatusForbidden || bytes.Contains(rejected.Body.Bytes(), []byte(`"bundle"`)) {
		t.Fatalf("final decision denied read: decisions=%d status=%d exposed=%t", calls, rejected.Code, bytes.Contains(rejected.Body.Bytes(), []byte(`"bundle"`)))
	}
	restored := r.request(t, "GET", path, "", nil)
	if restored.Code != http.StatusOK || !bytes.Equal(restored.Body.Bytes(), before.Body.Bytes()) || r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatalf("nonterminal read denial changed exact result or callbacks: status=%d generation=%d checks=%d", restored.Code, r.sends.Load(), r.checks.Load())
	}
}

func TestControlledDraftSharedCancellationWinsRecoveryFences(t *testing.T) {
	for _, point := range []string{"before_witness", "during_generation", "during_completion"} {
		t.Run(point, func(t *testing.T) {
			r := newRecoveryFixture(t, nil)
			const key = "cancel-shared-draft"
			ctx := context.Background()
			p := r.f.Principal
			input := r.input(t)
			var id string
			var held aicontrol.ActionStatus
			var beforeScopes map[string]aicontrol.ScopeStatus
			var unknownSession *aicontrol.Session
			providerCalls := 0
			cancel := func() {
				t.Helper()
				if err := r.f.DB.QueryRow("SELECT id FROM ai_fixture_action WHERE owner=$1 AND creation_key=$2", p.AccountID, key).Scan(&id); err != nil {
					t.Fatal(err)
				}
				if err := r.actions.SyntheticReviewer().Customer(ctx, p, id, aicontrol.CustomerEvent{Kind: "cancel"}); err != nil {
					t.Fatal(err)
				}
				status, err := r.actions.Status(ctx, p, id)
				if err != nil || status.State != "canceled" {
					t.Fatalf("kernel cancellation did not win: %s %v", status.State, err)
				}
			}
			var wantGeneration, wantChecks int32
			switch point {
			case "before_witness":
				r.fault = func(stage string) error {
					if stage == "after_shared_action_commit" {
						cancel()
					}
					return nil
				}
			case "during_generation":
				wantGeneration = 1
				r.provider = func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
					// Exercise a real unknown hold on this same synthetic Action.
					// Send invokes the configured in-memory responder only.
					root, err := json.Marshal(input)
					if err != nil {
						t.Fatal(err)
					}
					unknownSession, err = r.actions.Open(ctx, p, key, "application_draft", root)
					if err != nil {
						t.Fatal(err)
					}
					r.actions.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
						providerCalls++
						return nil, errors.New("controlled unknown response")
					})
					stage, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(ctx, "application_draft", root), "field_validation", "jev-rubric-v1/policy-v1")
					request, err := http.NewRequest(http.MethodPost, "https://api.typesafe.ai/v1/systemone", strings.NewReader(`{"model":"jev-1.13.0","state":{"field":"job_posting","submission":"synthetic"}}`))
					if err != nil {
						t.Fatal(err)
					}
					if _, err = unknownSession.Send(stage, "typesafe", request); err == nil {
						t.Fatal("fixture outcome unexpectedly known")
					}
					held, err = r.actions.Status(ctx, p, unknownSession.ID())
					if err != nil || len(held.Dispatches) != 1 || held.Dispatches[0].Remote != "possibly_active" {
						t.Fatalf("fixture did not retain an unknown intent: %+v %v", held, err)
					}
					beforeScopes = map[string]aicontrol.ScopeStatus{}
					for _, scope := range []string{"global", "provider:typesafe", "account:" + p.AccountID} {
						value, err := r.actions.ScopeStatus(ctx, p, scope)
						if err != nil || value.Counted <= 0 || value.Occupied != 1 {
							t.Fatalf("fixture did not exercise unknown hold: %s %+v %v", scope, value, err)
						}
						beforeScopes[scope] = value
					}
					cancel()
					return completeRecoveryBundle(g), nil
				}
			case "during_completion":
				wantGeneration, wantChecks = 1, 1
				r.review = func(ctx context.Context, req draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
					cancel()
					return controlledRecoveryReview(ctx, req)
				}
			}

			response := r.request(t, "POST", "/api/drafts", key, input)
			status, err := r.actions.Status(ctx, p, id)
			if err != nil {
				t.Fatal(err)
			}
			if status.State != "canceled" || response.Code < 400 || bytes.Contains(response.Body.Bytes(), []byte(`"bundle"`)) || r.sends.Load() != wantGeneration || r.checks.Load() != wantChecks {
				t.Fatalf("shared cancellation lost: kernel=%s HTTP=%d bundle=%t generation=%d completion=%d", status.State, response.Code, bytes.Contains(response.Body.Bytes(), []byte(`"bundle"`)), r.sends.Load(), r.checks.Load())
			}

			recovered := r.request(t, "GET", "/api/drafts/"+key+"/versions/1", "", nil)
			var result draftrecovery.Result
			if recovered.Code != http.StatusGone || json.Unmarshal(recovered.Body.Bytes(), &result) != nil || result.Action.ID != id || result.State != "closed" || result.Bundle != nil || !result.CleanupQueued {
				t.Fatalf("canceled Action did not recover terminal status: %d %+v", recovered.Code, result)
			}
			var witnesses int
			if err := r.f.DB.QueryRow("SELECT count(*) FROM careeros_draft_actions WHERE action_id=$1", id).Scan(&witnesses); err != nil {
				t.Fatal(err)
			}
			if point == "before_witness" && witnesses != 0 {
				t.Fatal("pre-witness cancellation created a generation witness")
			}

			if err := r.store.Cleanup(ctx, p, key); err != nil {
				t.Fatal(err)
			}
			replay := r.request(t, "POST", "/api/drafts", key, input)
			if replay.Code < 400 || r.sends.Load() != wantGeneration || r.checks.Load() != wantChecks {
				t.Fatalf("replay reopened a canceled Action: %d", replay.Code)
			}
			after, err := r.actions.Status(ctx, p, id)
			if err != nil || after.State != "canceled" || after.CustomerEffect || after.CustomerReserved {
				t.Fatalf("cleanup/replay changed kernel cancellation or allowance: %+v %v", after, err)
			}
			if unknownSession != nil {
				if !reflect.DeepEqual(after.Dispatches, held.Dispatches) {
					t.Fatal("cancellation changed unknown dispatch evidence")
				}
				for scope, expected := range beforeScopes {
					value, err := r.actions.ScopeStatus(ctx, p, scope)
					if err != nil || value != expected {
						t.Fatalf("cancellation released unknown hold: %s %+v %v", scope, value, err)
					}
				}
				if _, err := unknownSession.Retry(ctx); err == nil || providerCalls != 1 {
					t.Fatalf("cancellation reopened unknown Retry or resent: %v count=%d", err, providerCalls)
				}
			}
		})
	}
}
