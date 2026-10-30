package accountapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	applicationdraft "professional-information-repo/backend/application-draft"
	qualificationgaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/testsupport/aiadmission"
)

// These exercise the existing parsers/reviews rather than declaring a synthetic
// result complete merely because a provider returned 200. No delivery authority
// or real customer allowance is installed by this suite.
func TestA23ActualDraftQualityDoesNotBecomeSupplierExpense(t *testing.T) {
	for _, kind := range []string{"cv_only", "incomplete_answers", "resume_review_unavailable", "artifact_review_unavailable", "unsupported_answer", "honest_gap"} {
		t.Run(kind, func(t *testing.T) {
			f := aiadmission.NewFixture(t)
			h := aiadmission.NewHarness(t, f, kind)
			path := "/api/application-draft"
			body := f.Body(t, path)
			if kind == "honest_gap" {
				analysis := &aiadmission.Provider{T: t, Workflow: "qualification_gaps"}
				h.SetResponder(func(ctx context.Context, name string, r *http.Request) (*http.Response, error) {
					resp, err := analysis.Respond(ctx, name, r)
					if err == nil && aidiagnostics.StageName(ctx) == "requirement_evidence" {
						rewriteSyntheticCompletion(t, resp, func(content map[string]any) {
							content["matches"] = []any{map[string]any{"requirementIndex": 0, "state": "not_evidenced", "factIds": []string{}, "explanation": "The candidate honestly has no evidence for this requirement.", "question": ""}}
						})
					}
					return resp, err
				})
				analysisBody := f.Body(t, "/api/qualification-gaps")
				s, err := h.Open(context.Background(), f.Principal, "gap-analysis", "qualification_gaps", analysisBody)
				if err != nil {
					t.Fatal(err)
				}
				rec := httptest.NewRecorder()
				accountapi.ProtectWorkflow(f.Service, f.Store, "/api/qualification-gaps", qualificationgaps.NewHandler()).ServeHTTP(rec, f.Request(t, aidispatch.WithController(context.Background(), s), "/api/qualification-gaps", analysisBody))
				if rec.Code != 200 || !strings.Contains(rec.Body.String(), "not_evidenced") {
					t.Fatalf("honest gap was not preserved: %d %s", rec.Code, rec.Body.String())
				}
				var out, input map[string]json.RawMessage
				if json.Unmarshal(rec.Body.Bytes(), &out) != nil || json.Unmarshal(body, &input) != nil {
					t.Fatal("invalid fixture envelope")
				}
				input["jobContext"] = out["jobContext"]
				body, _ = json.Marshal(input)
			}
			provider := &aiadmission.Provider{T: t, Workflow: "application_draft"}
			h.SetResponder(func(ctx context.Context, name string, r *http.Request) (*http.Response, error) {
				resp, err := provider.Respond(ctx, name, r)
				if err != nil {
					return resp, err
				}
				stage := aidiagnostics.StageName(ctx)
				if kind == "resume_review_unavailable" && stage == "resume_support" || kind == "artifact_review_unavailable" && stage == "artifact_support" {
					resp.StatusCode = http.StatusServiceUnavailable
					return resp, nil
				}
				if stage == "generation" && (kind == "cv_only" || kind == "incomplete_answers") {
					rewriteSyntheticCompletion(t, resp, func(content map[string]any) {
						if kind == "cv_only" {
							delete(content, "coverLetter")
						} else {
							content["applicationAnswers"] = ""
						}
					})
				}
				if stage == "artifact_support" && kind == "unsupported_answer" {
					rewriteSyntheticCompletion(t, resp, func(content map[string]any) {
						content["answerConcerns"] = []string{"The requested answer is not supported by approved candidate evidence."}
					})
				}
				return resp, nil
			})
			s, err := h.Open(context.Background(), f.Principal, "draft", "application_draft", aiadmission.EffectiveRoot(t, path, body))
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			accountapi.ProtectWorkflow(f.Service, f.Store, path, applicationdraft.NewHandler()).ServeHTTP(rec, f.Request(t, aidispatch.WithController(context.Background(), s), path, body))
			if kind == "cv_only" || kind == "incomplete_answers" || kind == "resume_review_unavailable" {
				if rec.Code < 400 {
					t.Fatalf("incomplete/required-review failure became success: %d %s", rec.Code, rec.Body.String())
				}
			} else {
				if rec.Code != 200 {
					t.Fatalf("existing quality contract changed: %d %s", rec.Code, rec.Body.String())
				}
				if kind == "artifact_review_unavailable" && !strings.Contains(rec.Body.String(), "unavailable") {
					t.Fatal("unavailable artifact review was hidden")
				}
				if kind == "unsupported_answer" && !strings.Contains(rec.Body.String(), "not supported") {
					t.Fatal("unsupported answer concern was hidden")
				}
			}
			status, err := h.Status(context.Background(), f.Principal, s.ID())
			if err != nil || status.CustomerEffect || status.CustomerReserved || len(status.Dispatches) != len(provider.Calls) {
				t.Fatalf("quality outcome invented delivery/expense: %+v %v calls=%v", status, err, provider.Calls)
			}
			for _, d := range status.Dispatches {
				if d.Counted != 60 || d.Category != "unknown_maximum" {
					t.Fatalf("quality failure erased supplier exposure: %+v", d)
				}
			}
		})
	}
}

func rewriteSyntheticCompletion(t *testing.T, resp *http.Response, mutate func(map[string]any)) {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil {
		t.Fatal("invalid synthetic response")
	}
	message := envelope["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	var content map[string]any
	if json.Unmarshal([]byte(message["content"].(string)), &content) != nil {
		t.Fatal("invalid synthetic content")
	}
	mutate(content)
	encoded, _ := json.Marshal(content)
	message["content"] = string(encoded)
	encoded, _ = json.Marshal(envelope)
	resp.Body = io.NopCloser(strings.NewReader(string(encoded)))
}
