package applicationdraft_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/testsupport"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// This fixture has no network delegate. It inspects actual serialized envelopes
// emitted by the public handler, including response-dependent stages. This is
// preparation evidence only, never a deployed egress-capture mechanism.
type releaseCaptureFixture struct {
	t                *testing.T
	posting, variant string
	hashes           []string
	sizes            []int
}

func TestControlledReleaseCapturesEachProviderEnvelopeBeforeAdmission(t *testing.T) {
	var baseline []string
	for _, variant := range []string{"literal", "paraphrase", "deny artifact envelope"} {
		t.Run(variant, func(t *testing.T) {
			fixture := &releaseCaptureFixture{t: t, variant: variant, posting: "Desenvolvedor Ruby.\nMantenha ferramentas CLI internas usando Ruby.\nRuby é obrigatório.\nExplique como você usa Ruby em português."}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(&http.Client{Transport: fixture}).ServeHTTP(w, fixture.request())
			fixture.assertResult(w)
			if variant == "literal" {
				baseline = append([]string{}, fixture.hashes...)
			}
			if variant == "paraphrase" {
				assertDependentCaptureHashes(t, baseline, fixture.hashes)
			}
			t.Logf("controlled only: 4 inspected envelopes; envelopeBytes=%v; hashes=%v", fixture.sizes, fixture.hashes)
		})
	}
}

func (f *releaseCaptureFixture) request() *http.Request {
	req := artifactRequest(f.t, map[string]any{"jobPosting": f.posting, "cvLanguage": "pt-BR"})
	var input map[string]any
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		f.t.Fatal(err)
	}
	doc := input["profileEvidence"].(map[string]any)
	fact := doc["facts"].([]any)[0].(map[string]any)
	fact["value"] = "Ruby"
	fact["normalization"].(map[string]any)["observed"] = "Ruby"
	fact["normalization"].(map[string]any)["canonical"] = nil
	doc["evidence"].([]any)[0].(map[string]any)["excerpt"] = "Uso Ruby para manter ferramentas CLI internas."
	input["repository"].(map[string]any)["skills"] = "Ruby"
	raw, _ := json.Marshal(input)
	req = httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(raw)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-capture-only")
	req.Header.Set("X-TypeSafe-Api-Key", "capture-only")
	return req
}

func (f *releaseCaptureFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	envelope := f.capture(r)
	switch len(f.hashes) - 1 {
	case 0:
		return f.fieldResponse(r, envelope), nil
	case 1:
		return f.draftResponse(f.openAIPayload(r, envelope, 8000)), nil
	case 2:
		payload := f.openAIPayload(r, envelope, 4000)
		assertCaptureFields(f.t, payload, "claims", "cvLanguage", "facts", "qualificationAnswers")
		return completionResponse(map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Fonte sintética aprovada."}}}), nil
	case 3:
		return f.artifactResponse(f.openAIPayload(r, envelope, 8000))
	default:
		return nil, errors.New("unplanned dispatch or automatic retry")
	}
}

func (f *releaseCaptureFixture) capture(r *http.Request) map[string]json.RawMessage {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	if strings.Contains(string(raw), "capture-only") {
		f.t.Fatal("credential leaked into provider body")
	}
	digest := sha256.Sum256(raw)
	f.hashes = append(f.hashes, hex.EncodeToString(digest[:]))
	f.sizes = append(f.sizes, len(raw))
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		f.t.Fatal(err)
	}
	return envelope
}

func (f *releaseCaptureFixture) fieldResponse(r *http.Request, envelope map[string]json.RawMessage) *http.Response {
	if r.URL.Host != "api.typesafe.ai" {
		f.t.Fatal("original field decision must run first")
	}
	assertCaptureFields(f.t, envelope, "model", "questions", "state")
	if string(envelope["model"]) != `"jev-1.13.0"` {
		f.t.Fatal("fixed field-validation model changed")
	}
	var state struct{ Submission string }
	if err := json.Unmarshal(envelope["state"], &state); err != nil {
		f.t.Fatal(err)
	}
	if state.Submission != f.posting {
		f.t.Fatal("Jev must receive the complete original posting")
	}
	return testsupport.AcceptedJob()
}

func (f *releaseCaptureFixture) openAIPayload(r *http.Request, envelope map[string]json.RawMessage, ceiling int) map[string]json.RawMessage {
	if r.URL.Host != "api.openai.com" {
		f.t.Fatal("unexpected provider host")
	}
	assertCaptureFields(f.t, envelope, "max_completion_tokens", "messages", "model", "reasoning_effort", "response_format")
	if string(envelope["model"]) != `"gpt-6-luna"` || string(envelope["reasoning_effort"]) != `"none"` {
		f.t.Fatal("fixed workflow settings changed")
	}
	if string(envelope["max_completion_tokens"]) != strconv.Itoa(ceiling) {
		f.t.Fatal("stage output ceiling changed")
	}
	var messages []struct{ Content string }
	if err := json.Unmarshal(envelope["messages"], &messages); err != nil {
		f.t.Fatal(err)
	}
	if len(messages) != 2 {
		f.t.Fatal("expected system and user messages")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(messages[1].Content), &payload); err != nil {
		f.t.Fatal(err)
	}
	return payload
}

func (f *releaseCaptureFixture) draftResponse(payload map[string]json.RawMessage) *http.Response {
	assertCaptureFields(f.t, payload, "careerEvidence", "confirmedQualifications", "jobPosting", "qualificationAnswers", "resumeApplicationEvidence", "selection")
	text, answer := "Ruby", "Uso Ruby para manter ferramentas CLI internas."
	if f.variant != "literal" {
		text, answer = "Ruby: manutenção de ferramentas CLI internas.", "Uso Ruby em ferramentas CLI internas."
	}
	draft := structuredDraft([]any{citedClaim(text)})
	draft["jobTitle"], draft["jobSummary"] = "Desenvolvedor Ruby", f.posting
	draft["coverLetter"].(map[string]string)["body"] = "Uso Ruby para manter ferramentas CLI internas."
	draft["applicationAnswers"] = answer
	return completionResponse(draft)
}

func (f *releaseCaptureFixture) artifactResponse(payload map[string]json.RawMessage) (*http.Response, error) {
	assertCaptureFields(f.t, payload, "applicationAnswers", "blocks", "careerCitations", "cvLanguage", "facts", "jobCitations", "jobPosting", "qualificationAnswers")
	if f.variant == "deny artifact envelope" {
		return nil, errors.New("dependent envelope not admitted")
	}
	var blocks []fixtureArtifactBlock
	if err := json.Unmarshal(payload["blocks"], &blocks); err != nil {
		f.t.Fatal(err)
	}
	jobSources := []any{}
	start := 0
	for _, line := range strings.Split(f.posting, "\n") {
		jobSources = append(jobSources, map[string]any{"start": start, "end": start + len(line), "quote": line})
		start += len(line) + 1
	}
	judgments := artifactFixtureJudgments(blocks, jobSources, "Reconstructed source-reviewed fixture.")
	return completionResponse(map[string]any{"judgments": judgments, "answerConcerns": []string{}}), nil
}

func (f *releaseCaptureFixture) assertResult(w *httptest.ResponseRecorder) {
	f.t.Helper()
	if len(f.hashes) != 4 || w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		f.t.Fatalf("unexpected bounded result: calls=%d status=%d", len(f.hashes), w.Code)
	}
	var result struct {
		ArtifactReview struct {
			Check  string
			Claims []struct {
				Field, State string
				JobSources   []json.RawMessage
			}
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		f.t.Fatal(err)
	}
	if f.variant == "deny artifact envelope" {
		if result.ArtifactReview.Check != "unavailable" {
			f.t.Fatal("denied support envelope must remain unavailable")
		}
		return
	}
	if result.ArtifactReview.Check != "complete" {
		f.t.Fatalf("controlled support unavailable: %s", w.Body.String())
	}
	summaries := 0
	for _, claim := range result.ArtifactReview.Claims {
		if claim.Field != "jobSummary" {
			continue
		}
		summaries++
		if claim.State != "supported" || len(claim.JobSources) != 4 {
			f.t.Fatal("multi-subclaim summary must preserve its four distinct supplied excerpts")
		}
	}
	if summaries != 1 {
		f.t.Fatal("summary review must remain present")
	}
}

func assertDependentCaptureHashes(t *testing.T, baseline, current []string) {
	t.Helper()
	if current[1] != baseline[1] || current[2] == baseline[2] || current[3] == baseline[3] {
		t.Fatal("a fixed initial envelope cannot preapprove a response-dependent support envelope")
	}
}

func assertCaptureFields(t *testing.T, value map[string]json.RawMessage, allowed ...string) {
	t.Helper()
	keys := []string{}
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sort.Strings(allowed)
	if !reflect.DeepEqual(keys, allowed) {
		t.Fatalf("outbound field allowlist mismatch: %v", keys)
	}
}
