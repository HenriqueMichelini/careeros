package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	applicationdraft "professional-information-repo/backend/application-draft"
	cvgeneration "professional-information-repo/backend/cv-generation"
	profileingestion "professional-information-repo/backend/profile-ingestion"
	profilereview "professional-information-repo/backend/profile-review"
	qualificationgaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA29AllAuthenticatedWorkflowConstructors(t *testing.T) {
	f := aiadmission.NewFixture(t)
	h := aiadmission.NewHarness(t, f, "all_workflows")
	for _, tc := range []struct {
		path    string
		handler func() http.Handler
	}{
		{"/api/profile/ingest", profileingestion.NewHandler}, {"/api/profile/review", profilereview.NewHandler}, {"/api/cv/generate", cvgeneration.NewHandler}, {"/api/qualification-gaps", qualificationgaps.NewHandler}, {"/api/application-draft", applicationdraft.NewHandler},
	} {
		t.Run(tc.path, func(t *testing.T) {
			body := f.Body(t, tc.path)
			workflow := aiadmission.Workflow(tc.path)
			provider := &aiadmission.Provider{T: t, Workflow: workflow}
			h.SetResponder(provider.Respond)
			session, err := h.Open(context.Background(), f.Principal, workflow, workflow, aiadmission.EffectiveRoot(t, tc.path, body))
			if err != nil {
				t.Fatal(err)
			}
			handler := accountapi.ProtectWorkflow(f.Service, f.Store, tc.path, tc.handler())
			ctx := aidispatch.WithController(context.Background(), session)
			request := f.Request(t, ctx, tc.path, body)
			request.Header.Del("Cookie")
			denied := httptest.NewRecorder()
			handler.ServeHTTP(denied, request)
			if denied.Code != 401 || len(provider.Calls) != 0 {
				t.Fatalf("anonymous caller reached adapter: %d %s %v", denied.Code, denied.Body.String(), provider.Calls)
			}
			missing := httptest.NewRecorder()
			handler.ServeHTTP(missing, f.Request(t, aidispatch.RequireManaged(context.Background()), tc.path, body))
			if missing.Code < 400 || len(provider.Calls) != 0 {
				t.Fatalf("missing managed authority bypass: %d %s %v", missing.Code, missing.Body.String(), provider.Calls)
			}
			positive := httptest.NewRecorder()
			handler.ServeHTTP(positive, f.Request(t, ctx, tc.path, body))
			if positive.Code != 200 || len(provider.Calls) == 0 {
				t.Fatalf("valid fake-provider request failed: %d %s calls=%v", positive.Code, positive.Body.String(), provider.Calls)
			}
			status, err := h.Status(context.Background(), f.Principal, session.ID())
			if err != nil || len(status.Dispatches) != len(provider.Calls) || status.CustomerEffect {
				t.Fatalf("unaccounted provider leg or delivery claim: %v %+v", err, status)
			}
		})
	}
}

func TestA28StructuredAnalysisThenReviewedDraftSevenDispatches(t *testing.T) {
	f := aiadmission.NewFixture(t)
	h := aiadmission.NewHarness(t, f, "analysis_draft")
	var all []aiadmission.Call
	var contextArtifact json.RawMessage
	for _, tc := range []struct {
		path    string
		handler http.Handler
	}{
		{"/api/qualification-gaps", qualificationgaps.NewHandler()}, {"/api/application-draft", applicationdraft.NewHandler()},
	} {
		body := f.Body(t, tc.path)
		if contextArtifact != nil {
			var request map[string]json.RawMessage
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			request["jobContext"] = contextArtifact
			request["reviewResume"] = json.RawMessage("false")
			request["reviewArtifacts"] = json.RawMessage("false")
			body, _ = json.Marshal(request)
		}
		workflow := aiadmission.Workflow(tc.path)
		provider := &aiadmission.Provider{T: t, Workflow: workflow}
		h.SetResponder(provider.Respond)
		session, err := h.Open(context.Background(), f.Principal, workflow, workflow, aiadmission.EffectiveRoot(t, tc.path, body))
		if err != nil {
			t.Fatal(err)
		}
		result := httptest.NewRecorder()
		accountapi.ProtectWorkflow(f.Service, f.Store, tc.path, tc.handler).ServeHTTP(result, f.Request(t, aidispatch.WithController(context.Background(), session), tc.path, body))
		if result.Code != 200 {
			t.Fatalf("%s: %d %s calls=%v", tc.path, result.Code, result.Body.String(), provider.Calls)
		}
		var output map[string]json.RawMessage
		if err = json.Unmarshal(result.Body.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		if tc.path == "/api/qualification-gaps" {
			contextArtifact = output["jobContext"]
			if len(contextArtifact) == 0 {
				t.Fatal("analysis did not produce authentic reusable context")
			}
		} else if len(output["resumeReview"]) == 0 || len(output["artifactReview"]) == 0 {
			t.Fatal("direct false flags bypassed required reviews")
		}
		status, err := h.Status(context.Background(), f.Principal, session.ID())
		if err != nil || len(status.Dispatches) != len(provider.Calls) || status.CustomerEffect {
			t.Fatalf("wrong persisted dispatch/delivery counts: %v %+v", err, status)
		}
		all = append(all, provider.Calls...)
	}
	want := []aiadmission.Call{{"typesafe", "field_validation"}, {"openai", "qualification_matching"}, {"openai", "requirement_evidence"}, {"typesafe", "field_validation"}, {"openai", "generation"}, {"openai", "resume_support"}, {"openai", "artifact_support"}}
	got, _ := json.Marshal(all)
	expected, _ := json.Marshal(want)
	if string(got) != string(expected) {
		t.Fatalf("reached provider legs = %s; want %s", got, expected)
	}
	scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || scope.Counted != 420 || scope.Occupied != 7 {
		t.Fatalf("seven M60 obligations: counted=%d occupied=%d; want 420/7: %v", scope.Counted, scope.Occupied, err)
	}
}

func TestA28SkippedLegsHaveNoInventedDispatch(t *testing.T) {
	for _, kind := range []string{"reused_job", "zero_requirements", "no_pending_resume_claims"} {
		t.Run(kind, func(t *testing.T) {
			f := aiadmission.NewFixture(t)
			h := aiadmission.NewHarness(t, f, kind)
			path := "/api/qualification-gaps"
			var next http.Handler = qualificationgaps.NewHandler()
			if kind == "no_pending_resume_claims" {
				path = "/api/application-draft"
				next = applicationdraft.NewHandler()
			}
			body := f.Body(t, path)
			provider := &aiadmission.Provider{T: t, Workflow: aiadmission.Workflow(path), NoRequirements: kind == "zero_requirements", NoResumeClaims: kind == "no_pending_resume_claims"}
			h.SetResponder(provider.Respond)
			call := func(key string, payload []byte) *httptest.ResponseRecorder {
				session, err := h.Open(context.Background(), f.Principal, key, aiadmission.Workflow(path), aiadmission.EffectiveRoot(t, path, payload))
				if err != nil {
					t.Fatal(err)
				}
				result := httptest.NewRecorder()
				accountapi.ProtectWorkflow(f.Service, f.Store, path, next).ServeHTTP(result, f.Request(t, aidispatch.WithController(context.Background(), session), path, payload))
				if result.Code != 200 {
					t.Fatalf("%s: %d %s calls=%v", kind, result.Code, result.Body.String(), provider.Calls)
				}
				status, err := h.Status(context.Background(), f.Principal, session.ID())
				if err != nil || len(status.Dispatches) != len(provider.Calls) || status.CustomerEffect {
					t.Fatalf("skipped/reached count mismatch: %v %+v calls=%v", err, status, provider.Calls)
				}
				return result
			}
			if kind == "reused_job" {
				first := call("fresh", body)
				var output map[string]json.RawMessage
				if err := json.Unmarshal(first.Body.Bytes(), &output); err != nil {
					t.Fatal(err)
				}
				var input map[string]json.RawMessage
				json.Unmarshal(body, &input)
				input["jobContext"] = output["jobContext"]
				body, _ = json.Marshal(input)
				provider.Calls = nil
			}
			result := call("skip", body)
			want := []aiadmission.Call{{"typesafe", "field_validation"}, {"openai", "qualification_matching"}}
			if kind == "reused_job" {
				want = []aiadmission.Call{{"typesafe", "field_validation"}, {"openai", "requirement_evidence"}}
			}
			if kind == "no_pending_resume_claims" {
				want = []aiadmission.Call{{"typesafe", "field_validation"}, {"openai", "generation"}, {"openai", "artifact_support"}}
				var output struct {
					ResumeReview struct {
						Check  string
						Claims []struct{ State string }
					}
				}
				json.Unmarshal(result.Body.Bytes(), &output)
				if output.ResumeReview.Check != "not_needed" || len(output.ResumeReview.Claims) != 1 || output.ResumeReview.Claims[0].State != "unsupported" {
					t.Fatal("stale resume claim lost its unsupported quality outcome", result.Body.String())
				}
			}
			got, _ := json.Marshal(provider.Calls)
			expected, _ := json.Marshal(want)
			if string(got) != string(expected) {
				t.Fatalf("skipped leg invented/reached: got %s want %s", got, expected)
			}
		})
	}
}
