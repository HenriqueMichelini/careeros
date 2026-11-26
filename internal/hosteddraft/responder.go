package hosteddraft

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/profiledocument"
)

// FixtureVersion identifies the exact response set independently checked by
// the attestor. Only the pinned English Java fixture has a positive path.
const FixtureVersion = "closed-draft-java-v1"

const (
	fixturePosting  = "Java required."
	fixtureGreeting = "Dear team,"
	fixtureLetter   = "I use Java."
	maxFakeRequest  = 512 << 10
)

const fixtureFieldResponse = `{"model":"jev-1.13.0","answers":{"content":{"type":"choice","choice":"job_with_context","confidence":1,"probabilities":{"job_with_context":1,"job_title_only":0,"relevant_but_insufficient":0,"irrelevant":0,"unusable":0}},"attack":{"type":"choice","choice":"none","confidence":1,"probabilities":{"none":1,"uncertain":0,"detected":0}}}}`

const fixtureGeneration = `{"jobTitle":null,"company":null,"jobSummary":"Java required.","resume":[{"section":"skills","kind":"bullet","text":"Java","sources":[{"profileId":"p","id":"skill","revision":1}]}],"applicationAnswers":"","coverLetter":{"greeting":"Dear team,","body":"I use Java.","closing":"Sincerely,"}}`

const fixtureResume = `{"judgments":[{"index":0,"state":"supported","reason":"Synthetic exact Java source."}]}`

const fixtureArtifacts = `{"judgments":[{"index":0,"state":"supported","reason":"Synthetic exact job source.","nonfactual":false,"scope":"job","sources":[],"jobSources":[{"start":0,"end":14,"quote":"Java required."}]},{"index":1,"state":"supported","reason":"Synthetic greeting.","nonfactual":true,"scope":"nonfactual","sources":[],"jobSources":[]},{"index":2,"state":"supported","reason":"Synthetic exact Java source.","nonfactual":false,"scope":"career","sources":[{"profileId":"p","id":"skill","revision":1}],"jobSources":[]}],"answerConcerns":[],"answerRequests":[]}`

var fixtureSkill = profiledocument.Reference{ProfileID: "p", ID: "skill", Revision: 1}

type fixtureRange struct {
	Start, End int
	Quote      string
}

var fixtureJobRange = fixtureRange{0, len(fixturePosting), fixturePosting}

func stageProvider(stage string) string {
	switch stage {
	case "field_validation":
		return "typesafe"
	case "generation", "resume_support", "artifact_support":
		return "openai"
	default:
		return ""
	}
}

func fixtureResponse(stage string) ([]byte, error) {
	var content string
	switch stage {
	case "field_validation":
		return []byte(fixtureFieldResponse), nil
	case "generation":
		content = fixtureGeneration
	case "resume_support":
		content = fixtureResume
	case "artifact_support":
		content = fixtureArtifacts
	default:
		return nil, errInvalidAttestation
	}
	return json.Marshal(map[string]any{"choices": []any{map[string]any{
		"finish_reason": "stop", "message": map[string]string{"content": content},
	}}})
}

// NewFakeResponder installs only the local response set. The only network path
// is the bounded metadata-only attestation request to the exact attestor origin.
func NewFakeResponder(attestor *AttestorClient) func(context.Context, aicontrol.HostedDraftCall) (*http.Response, error) {
	return func(ctx context.Context, call aicontrol.HostedDraftCall) (*http.Response, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attestor == nil || !matchesFixtureRequest(call.Stage, call.Provider, call.Request) {
			return nil, errInvalidAttestation
		}
		body, err := fixtureResponse(call.Stage)
		if err != nil {
			return nil, err
		}
		evidence := dispatchAttestation(call, sha256.Sum256(body))
		if err := attestor.record(ctx, evidence); err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": {"application/json"}},
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
		}, nil
	}
}

func matchesFixtureRequest(stage, provider string, req *http.Request) bool {
	if stageProvider(stage) != provider || provider == "" || req == nil || req.URL == nil || req.Body == nil ||
		req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.RawQuery != "" ||
		req.URL.Fragment != "" || req.URL.RawPath != "" || len(req.Header) != 1 || req.Header.Get("Content-Type") != "application/json" {
		return false
	}
	defer req.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(req.Body, maxFakeRequest+1))
	if err != nil || len(raw) > maxFakeRequest || !accountstore.StrictJSON(raw) {
		return false
	}
	if provider == "typesafe" {
		var gate struct {
			Model string
			State struct{ Field, Submission string }
		}
		return req.URL.Host == "api.typesafe.ai" && req.URL.Path == "/v1/systemone" &&
			json.Unmarshal(raw, &gate) == nil && gate.Model == fieldvalidation.Model && gate.State.Field == string(fieldvalidation.JobPosting) && gate.State.Submission == fixturePosting
	}
	var completion struct {
		Model       string
		ServiceTier string `json:"service_tier"`
		Messages    []struct{ Role, Content string }
	}
	if req.URL.Host != "api.openai.com" || req.URL.Path != "/v1/chat/completions" || json.Unmarshal(raw, &completion) != nil ||
		completion.Model != "gpt-6-luna" || completion.ServiceTier != "standard" || len(completion.Messages) != 2 ||
		completion.Messages[0].Role != "system" || completion.Messages[1].Role != "user" {
		return false
	}
	return matchesFixturePrompt(stage, completion.Messages[0].Content, []byte(completion.Messages[1].Content))
}

func matchesFixturePrompt(stage, policy string, raw []byte) bool {
	if !accountstore.StrictJSON(raw) {
		return false
	}
	var prompt struct {
		JobPosting string
		CVLanguage string
		Selection  struct{ Sources []string }
		Claims     []struct {
			Section, Kind, Text string
			Sources             []profiledocument.Reference
		}
		Facts []struct {
			ID       string
			Revision int64
			Value    json.RawMessage
		}
		Blocks []struct {
			Index       int
			Field, Text string
		}
		CareerCitations []profiledocument.Reference
		JobCitations    []fixtureRange
	}
	if json.Unmarshal(raw, &prompt) != nil {
		return false
	}
	switch stage {
	case "generation":
		return prompt.JobPosting == fixturePosting && strings.HasPrefix(policy, "CV language: en. ") && slices.Contains(prompt.Selection.Sources, "p/skill@1")
	case "resume_support":
		if prompt.CVLanguage != "en" || len(prompt.Claims) != 1 {
			return false
		}
		claim := prompt.Claims[0]
		return claim.Section == "skills" && claim.Kind == "bullet" && claim.Text == "Java" &&
			slices.Equal(claim.Sources, []profiledocument.Reference{fixtureSkill}) && slices.ContainsFunc(prompt.Facts, func(fact struct {
			ID       string
			Revision int64
			Value    json.RawMessage
		}) bool {
			var value string
			return fact.ID == fixtureSkill.ID && fact.Revision == fixtureSkill.Revision && json.Unmarshal(fact.Value, &value) == nil && value == "Java"
		})
	case "artifact_support":
		return prompt.CVLanguage == "en" && prompt.JobPosting == fixturePosting &&
			slices.Contains(prompt.CareerCitations, fixtureSkill) && slices.Contains(prompt.JobCitations, fixtureJobRange) &&
			len(prompt.Blocks) == 3 && prompt.Blocks[0].Index == 0 && prompt.Blocks[0].Field == "jobSummary" && prompt.Blocks[0].Text == fixturePosting &&
			prompt.Blocks[1].Index == 1 && prompt.Blocks[1].Field == "greeting" && prompt.Blocks[1].Text == fixtureGreeting &&
			prompt.Blocks[2].Index == 2 && prompt.Blocks[2].Field == "body" && prompt.Blocks[2].Text == fixtureLetter
	default:
		return false
	}
}
