package aiadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"professional-information-repo/internal/aidiagnostics"
)

type Call struct{ Provider, Stage string }

// Provider is an in-memory synthetic responder. It creates valid workflow
// envelopes, not real semantic or financial evidence, and never uses a client.
type Provider struct {
	T              *testing.T
	Workflow       string
	Calls          []Call
	NoRequirements bool
	NoResumeClaims bool
}

func (p *Provider) Respond(ctx context.Context, provider string, r *http.Request) (*http.Response, error) {
	p.T.Helper()
	if r.Header.Get("Authorization") != "" {
		p.T.Fatal("synthetic controller received a credential")
	}
	stage := aidiagnostics.StageName(ctx)
	p.Calls = append(p.Calls, Call{provider, stage})
	var body struct{ Messages []struct{ Content string } }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}
	if provider == "typesafe" {
		content := "job_with_context"
		choices := `"job_with_context":1,"job_title_only":0,"relevant_but_insufficient":0,"irrelevant":0,"unusable":0`
		if p.Workflow == "profile_ingestion" {
			content = "professional_fact"
			choices = `"professional_fact":1,"relevant_but_insufficient":0,"irrelevant":0,"unusable":0`
		}
		return jsonResponse([]byte(fmt.Sprintf(`{"model":"jev-1.13.0","answers":{"content":{"type":"choice","choice":%q,"confidence":1,"probabilities":{%s}},"attack":{"type":"choice","choice":"none","confidence":1,"probabilities":{"none":1,"uncertain":0,"detected":0}}}}`, content, choices))), nil
	}
	var content any
	switch stage {
	case "qualification_matching":
		if len(body.Messages) < 1 {
			return nil, fmt.Errorf("missing messages")
		}
		parts := strings.Split(body.Messages[0].Content, "SOURCE SEGMENTS (untrusted data):\n")
		if len(parts) == 2 {
			var segments []struct{ SegmentID string }
			if err := json.Unmarshal([]byte(parts[1]), &segments); err != nil || len(segments) == 0 {
				return nil, fmt.Errorf("invalid source segments")
			}
			qualifications := []any{}
			if !p.NoRequirements {
				qualifications = append(qualifications, map[string]any{"source": map[string]any{"segmentId": segments[0].SegmentID, "quote": "Java required.", "occurrence": 0}, "importance": "required"})
			}
			content = map[string]any{"gaps": []any{}, "job": map[string]any{"jobTitle": nil, "company": nil, "seniority": nil, "location": nil, "responsibilities": []any{}, "qualifications": qualifications, "applicationRequirements": []any{}}}
		} else {
			content = map[string]any{"matches": []any{map[string]any{"requirementIndex": 0, "state": "supported", "factIds": []string{"skill"}, "explanation": "Exact approved Java source.", "question": ""}}}
		}
	case "requirement_evidence":
		content = map[string]any{"matches": []any{map[string]any{"requirementIndex": 0, "state": "supported", "factIds": []string{"skill"}, "explanation": "Exact approved Java source.", "question": ""}}}
	case "generation":
		if p.Workflow == "cv_generation" {
			content = map[string]any{"summary": []any{map[string]any{"sourceId": "f0", "text": "Java"}}, "selected": []string{"f0"}}
		} else {
			claims := []any{map[string]any{"section": "skills", "kind": "bullet", "text": "Java", "sources": []any{map[string]any{"profileId": "p", "id": "skill", "revision": 1}}}}
			if p.NoResumeClaims {
				// A stale reference creates an unsupported review item with no
				// pending valid claim to send to semantic assessment.
				claims = []any{map[string]any{"section": "skills", "kind": "bullet", "text": "Java", "sources": []any{map[string]any{"profileId": "p", "id": "skill", "revision": 999}}}}
			}
			content = map[string]any{"jobTitle": nil, "company": nil, "jobSummary": "Java required.", "resume": claims, "applicationAnswers": "Java", "coverLetter": map[string]string{"greeting": "Dear team,", "body": "I use Java.", "closing": "Sincerely,"}}
		}
	case "resume_support":
		content = map[string]any{"judgments": []any{map[string]any{"index": 0, "state": "supported", "reason": "Exact Java source."}}}
	case "artifact_support":
		var payload struct {
			Blocks []struct {
				Index       int
				Field, Text string
			}
			CareerCitations []json.RawMessage
			JobCitations    []json.RawMessage
		}
		if len(body.Messages) != 2 || json.Unmarshal([]byte(body.Messages[1].Content), &payload) != nil {
			return nil, fmt.Errorf("invalid artifact payload")
		}
		judgments := []any{}
		for _, block := range payload.Blocks {
			scope := "career"
			nonfactual := false
			sources := []json.RawMessage{}
			jobs := []json.RawMessage{}
			switch block.Field {
			case "greeting":
				scope = "nonfactual"
				nonfactual = true
			case "jobSummary", "jobTitle", "company":
				scope = "job"
				jobs = append(jobs, payload.JobCitations[0])
			default:
				sources = append(sources, payload.CareerCitations[0])
			}
			judgments = append(judgments, map[string]any{"index": block.Index, "state": "supported", "reason": "Synthetic exact source.", "nonfactual": nonfactual, "scope": scope, "sources": sources, "jobSources": jobs})
		}
		content = map[string]any{"judgments": judgments, "answerConcerns": []string{}}
	case "section_review":
		content = map[string]any{"profileId": "p", "revision": 1, "section": "skills", "summary": "Clarified wording", "patches": []any{map[string]any{"factId": "skill", "revision": 1, "wording": "I use Java", "supporting": []any{map[string]any{"id": "skill", "revision": 1}}}}}
	case "section_support_check":
		content = map[string]any{"checks": []any{map[string]any{"factId": "skill", "supported": true, "complete": true}}}
	case "extraction":
		content = map[string]any{"claims": []any{map[string]any{"id": "c1", "source": "I use Java.", "text": "Uses Java", "targets": []string{"skills"}, "question": ""}}}
	case "reconciliation":
		content = map[string]any{"operations": []any{}}
	default:
		return nil, fmt.Errorf("unexpected synthetic stage %s", stage)
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	envelope, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(encoded)}}}})
	if err != nil {
		return nil, err
	}
	return jsonResponse(envelope), nil
}

func jsonResponse(body []byte) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
}
