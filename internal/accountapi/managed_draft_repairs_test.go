package accountapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
)

// This defensive schedule runs only against the repaired route. The original
// cancellation defect is retained as review evidence, with no downstream replay.
func TestManagedDraftQualificationCancellationWithholdsContextAndProof(t *testing.T) {
	f, h, provider, _, router, input := managedDraftFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	if err := h.SetQualificationResponder(func(ctx context.Context, name string, request *http.Request) (*http.Response, bool, error) {
		if aidiagnostics.StageName(ctx) == "requirement_evidence" {
			close(entered)
			<-release
		}
		response, err := provider.Respond(ctx, name, request)
		return response, true, err
	}); err != nil {
		t.Fatal(err)
	}
	const key = "canceled-qualification"
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- qualificationRequest(t, f, router, "/api/qualification-gaps", key, "", f.Body(t, "/api/qualification-gaps"))
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("final qualification check was not reached")
	}
	var action string
	if err := f.DB.QueryRow(`SELECT id FROM ai_fixture_action WHERE namespace=$1 AND creation_key=$2`, h.Namespace(), key).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if err := h.SyntheticReviewer().Customer(context.Background(), f.Principal, action, aicontrol.CustomerEvent{Kind: "cancel"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	released = true
	response := <-done
	if response.Code < 400 || response.Header().Get("X-CareerOS-Draft-Context-Proof") != "" ||
		strings.Contains(response.Body.String(), `"jobContext"`) || strings.Contains(response.Body.String(), `"matches"`) {
		t.Fatalf("canceled completion released protected context or proof: %d %s", response.Code, response.Body.String())
	}
	var state string
	var finished bool
	if err := f.DB.QueryRow(`SELECT state,execution_finished FROM ai_fixture_action WHERE namespace=$1 AND id=$2`, h.Namespace(), action).Scan(&state, &finished); err != nil || state != "canceled" || finished {
		t.Fatalf("finalization changed cancellation: state=%s finished=%t err=%v", state, finished, err)
	}
	scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || scope.Counted != 25_344 || len(provider.Calls) != 3 {
		t.Fatalf("cancellation erased reached expense: %+v calls=%v err=%v", scope, provider.Calls, err)
	}
	// The denied response supplies no usable handoff. A malformed attempted
	// handoff stays outside Draft admission and cannot invoke its first leg.
	raw, _ := json.Marshal(struct {
		draftrecovery.Input
		JobContext         json.RawMessage `json:"jobContext"`
		QualificationProof string          `json:"qualificationProof"`
	}{input, json.RawMessage("null"), response.Header().Get("X-CareerOS-Draft-Context-Proof")})
	denied := qualificationRequest(t, f, router, "/api/drafts", "denied-handoff", "", raw)
	var drafts int
	if err := f.DB.QueryRow(`SELECT count(*) FROM ai_fixture_action WHERE namespace=$1 AND workflow='application_draft'`, h.Namespace()).Scan(&drafts); err != nil || drafts != 0 || denied.Code < 400 || len(provider.Calls) != 3 {
		t.Fatalf("absent authority admitted Draft: code=%d drafts=%d calls=%v err=%v", denied.Code, drafts, provider.Calls, err)
	}
}

func TestManagedDraftCanonicalCompletedLetterMatchesRecoveryBoundary(t *testing.T) {
	for _, row := range []struct {
		name, separator string
		words           int
		valid           bool
	}{
		{"0085-nonspace", "\u0085", 400, true},
		{"space399", " ", 393, true},
		{"space400", " ", 394, false},
		{"FEFF399", "\uFEFF", 393, true},
		{"FEFF400", "\uFEFF", 394, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			f, h, provider, _, router, input := managedDraftFixture(t)
			var doc profiledocument.Document
			if err := json.Unmarshal(f.Document, &doc); err != nil {
				t.Fatal(err)
			}
			doc.Revision++
			for i := range doc.Facts {
				doc.Facts[i].Owner.Revision = doc.Revision
			}
			name := doc.Facts[0]
			name.ID, name.Field, name.Value = "letter-name", "fullName", json.RawMessage(`"Original Person"`)
			name.Normalization.Observed = "Original Person"
			name.Order = 2
			name.Support = "unsupported"
			name.Origin = profiledocument.Origin{Kind: "manual_edit", Original: "user"}
			doc.Facts = append(doc.Facts, name)
			saved, _ := json.Marshal(doc)
			if _, err := f.Store.SaveProfile(context.Background(), f.Principal, "letter-profile", input.Profile.Revision, saved); err != nil {
				t.Fatal(err)
			}
			input.Profile.Revision = doc.Revision
			body := strings.TrimSuffix(strings.Repeat("java"+row.separator, row.words), row.separator) + "."
			complete := "Dear hiring team,\n\n" + body + "\n\nSincerely,\nOriginal Person"
			if applicationdraft.ValidCompletedCoverLetter(complete, "Original Person") != row.valid {
				t.Fatal("fixture disagrees with the existing canonical recovery contract")
			}
			if err := h.SetQualificationResponder(func(ctx context.Context, name string, request *http.Request) (*http.Response, bool, error) {
				response, err := provider.Respond(ctx, name, request)
				if err != nil || aidiagnostics.StageName(ctx) != "generation" {
					return response, true, err
				}
				raw, err := io.ReadAll(response.Body)
				response.Body.Close()
				var envelope struct {
					Choices []struct {
						FinishReason string `json:"finish_reason"`
						Message      struct {
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
				}
				if err != nil || json.Unmarshal(raw, &envelope) != nil || len(envelope.Choices) != 1 {
					t.Fatal("invalid controlled response")
				}
				var output map[string]any
				if json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &output) != nil {
					t.Fatal("invalid controlled candidate")
				}
				output["coverLetter"] = map[string]string{"greeting": "Dear hiring team,", "body": body, "closing": "Sincerely,"}
				candidate, _ := json.Marshal(output)
				envelope.Choices[0].Message.Content = string(candidate)
				raw, _ = json.Marshal(envelope)
				response.Body = io.NopCloser(strings.NewReader(string(raw)))
				return response, true, nil
			}); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(input)
			response := qualificationRequest(t, f, router, "/api/drafts", "canonical-letter", "", raw)
			var result draftrecovery.Result
			if row.valid {
				if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Bundle == nil ||
					result.Bundle.CoverLetter != complete || len(provider.Calls) != 4 || provider.Calls[3].Stage != "artifact_support" {
					t.Fatalf("canonical complete letter did not reach artifact review and publication: %d %s calls=%v", response.Code, response.Body.String(), provider.Calls)
				}
			} else if response.Code != 422 || len(provider.Calls) != 3 {
				t.Fatalf("400-word candidate crossed the canonical boundary: %d %s calls=%v", response.Code, response.Body.String(), provider.Calls)
			}
		})
	}
}
