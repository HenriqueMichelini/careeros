package qualificationgaps

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func explainedRequest(t *testing.T, decision string, alter func(map[string]any)) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := os.ReadFile("../../internal/profiledocument/fixtures.json")
	var fixtures []struct{ Document json.RawMessage }
	json.Unmarshal(raw, &fixtures)
	input := sampleRequest()
	input.JobPosting = "Java required."
	data, _ := json.Marshal(input)
	var request map[string]any
	json.Unmarshal(data, &request)
	request["understandJob"] = true
	request["profileEvidence"] = fixtures[0].Document
	if alter != nil {
		alter(request)
	}
	data, _ = json.Marshal(request)
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var payload struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&payload)
		prompt := payload.Messages[0].Content
		content := decision
		if strings.Contains(prompt, "SOURCE SEGMENTS (untrusted data):\n") {
			parts := strings.Split(prompt, "SOURCE SEGMENTS (untrusted data):\n")
			var segments []struct{ SegmentID string }
			json.Unmarshal([]byte(parts[1]), &segments)
			content = `{"gaps":[],"job":{"jobTitle":null,"company":null,"seniority":null,"location":null,"responsibilities":[],"qualifications":[{"source":{"segmentId":"` + segments[0].SegmentID + `","quote":"Java required.","occurrence":0},"importance":"required"}],"applicationRequirements":[]}}`
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	req := httptest.NewRequest("POST", "/api/qualification-gaps", strings.NewReader(string(data)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	w := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(w, req)
	return w
}

const clarification = `{"matches":[{"requirementIndex":0,"state":"needs_clarification","factIds":["skill"],"explanation":"The Profile describes an uncertain aspiration, not Java experience.","question":"Have you used Java?"}]}`

func TestExplainedRequirementsResolveApprovedProfileFacts(t *testing.T) {
	w := explainedRequest(t, clarification, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"needs_clarification"`) || !strings.Contains(w.Body.String(), `"id":"skill"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestExplainedRequirementsRejectUnresolvedAndContradictoryReferences(t *testing.T) {
	for _, tc := range []struct{ name, output string }{
		{"foreign fact", strings.Replace(clarification, `"skill"`, `"unknown"`, 1)},
		{"negative aspiration cannot support", strings.Replace(clarification, `"needs_clarification"`, `"supported"`, 1)},
		{"missing requirement", `{"matches":[]}`},
		{"duplicate requirement", `{"matches":[` + strings.TrimSuffix(strings.TrimPrefix(clarification, `{"matches":[`), `]}`) + `,` + strings.TrimSuffix(strings.TrimPrefix(clarification, `{"matches":[`), `]}`) + `]}`},
		{"missing question", strings.Replace(clarification, `,"question":"Have you used Java?"`, "", 1)},
		{"unknown field", strings.Replace(clarification, `"question":"Have you used Java?"`, `"question":"Have you used Java?","score":99`, 1)},
		{"null index", strings.Replace(clarification, `"requirementIndex":0`, `"requirementIndex":null`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := explainedRequest(t, tc.output, nil)
			if w.Code != 502 || !strings.Contains(w.Body.String(), "invalid_output") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLimitedEvidenceCannotEstablishAbsence(t *testing.T) {
	decision := `{"matches":[{"requirementIndex":0,"state":"not_evidenced","factIds":[],"explanation":"No matching evidence was selected.","question":""}]}`
	w := explainedRequest(t, decision, func(request map[string]any) {
		var doc map[string]any
		json.Unmarshal(request["profileEvidence"].(json.RawMessage), &doc)
		f := doc["facts"].([]any)[0].(map[string]any)
		f["value"] = strings.Repeat("Unrelated experience. ", 1000)
		f["normalization"] = map[string]any{"observed": "unknown", "canonical": nil, "policy": "exact-alias-v1"}
		request["profileEvidence"] = doc
	})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"complete":false`) || !strings.Contains(w.Body.String(), `"state":"needs_clarification"`) || !strings.Contains(w.Body.String(), `"excluded":1`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPrivateProjectionRejectedBeforeProviders(t *testing.T) {
	w := explainedRequest(t, clarification, func(request map[string]any) {
		var doc map[string]any
		json.Unmarshal(request["profileEvidence"].(json.RawMessage), &doc)
		doc["facts"].([]any)[0].(map[string]any)["field"] = "email"
		request["profileEvidence"] = doc
	})
	if w.Code != 400 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
