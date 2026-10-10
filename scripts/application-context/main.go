// Credential-free HTTP contract replay. Labeled candidate recall and per-field
// output checks are independent; replay does not measure live model quality.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"

	draft "professional-information-repo/backend/application-draft"
	eval "professional-information-repo/internal/semanticeval"
	"professional-information-repo/internal/testsupport"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "api.typesafe.ai" {
		return testsupport.AcceptedJob(), nil
	}
	return f(r)
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func respond(content json.RawMessage) *http.Response {
	raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(content)}}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}
}
func run(input any, content json.RawMessage, capture func([]byte)) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(input)
	req := httptest.NewRequest("POST", "/api/application-draft", strings.NewReader(string(raw)))
	req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic")
	req.Header.Set("X-TypeSafe-Api-Key", "synthetic")
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		capture(body)
		return respond(content), nil
	})}
	w := httptest.NewRecorder()
	draft.NewHandlerWithClient(client).ServeHTTP(w, req)
	return w
}
func main() {
	output := flag.String("output", "/tmp/application-context-report.json", "controlled report")
	flag.Parse()
	labelsRaw, err := os.ReadFile("docs/evaluations/qualification-evidence/cases.json")
	must(err)
	var labels struct {
		Cases []struct {
			ID, Requirement string
			Document        json.RawMessage
			Relevant        []string `json:"relevantFactIds"`
			Negative        []string `json:"negativeFactIds"`
		}
	}
	must(json.Unmarshal(labelsRaw, &labels))
	casesRaw, err := os.ReadFile("docs/evaluations/semantic-quality/cases.v2.json")
	must(err)
	var corpus struct{ Cases []eval.Case }
	must(json.Unmarshal(casesRaw, &corpus))
	baselineRaw, err := os.ReadFile("docs/evaluations/semantic-quality/controlled.v2.json")
	must(err)
	var baseline struct{ Runs []eval.RunResult }
	must(json.Unmarshal(baselineRaw, &baseline))
	rows := []any{}
	fields := []any{}
	content := json.RawMessage(`{"jobTitle":null,"company":null,"jobSummary":"Java required.","resume":"Java","coverLetter":{"greeting":"Dear team,","body":"I use Java.","closing":"Sincerely,"},"applicationAnswers":"Java."}`)
	for _, c := range labels.Cases {
		input := map[string]any{"repository": map[string]any{"careerGoals": "", "skills": "", "competencies": "", "experience": []any{}, "tools": "", "projects": []any{}, "employmentStatus": "", "currentSalary": "", "desiredSalary": "", "additionalInfo": ""}, "jobPosting": c.Requirement, "confirmedQualifications": []any{}, "profileEvidence": c.Document}
		outbound := 0
		w := run(input, content, func(raw []byte) { outbound = len(raw) })
		var result struct {
			Selection struct {
				Sources                           []string
				BudgetExcluded, RelevanceExcluded int
				Complete                          bool
			} `json:"contextSelection"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		kept := func(id string) bool {
			for _, ref := range result.Selection.Sources {
				if strings.Contains(ref, "/"+id+"@") {
					return true
				}
			}
			return false
		}
		missing, negative := []string{}, []string{}
		for _, id := range c.Relevant {
			if !kept(id) {
				missing = append(missing, id)
			}
		}
		for _, id := range c.Negative {
			if !kept(id) {
				negative = append(negative, id)
			}
		}
		rows = append(rows, map[string]any{"id": c.ID, "status": w.Code, "relevant": len(c.Relevant), "retrieved": len(c.Relevant) - len(missing), "materialMisses": missing, "negativeMisses": negative, "selection": result.Selection, "providerEnvelopeBytes": outbound, "baselineApprovedCandidateRecall": 1.0})
	}
	for _, c := range corpus.Cases {
		if c.Task != "application_draft" {
			continue
		}
		var input any
		json.Unmarshal(c.Request, &input)
		w := run(input, c.Provider[0], func([]byte) {})
		var current map[string]json.RawMessage
		json.Unmarshal(w.Body.Bytes(), &current)
		var previous map[string]json.RawMessage
		for _, b := range baseline.Runs {
			if b.ID == c.ID {
				json.Unmarshal(b.Output, &previous)
			}
		}
		for _, field := range []string{"jobSummary", "resume", "coverLetter", "applicationAnswers"} {
			scoped := c
			scoped.Gold = nil
			for _, a := range c.Gold {
				pattern, ok := a.Match["field"]
				if ok && regexp.MustCompile(pattern).MatchString(field) {
					scoped.Gold = append(scoped.Gold, a)
				}
			}
			raw, _ := json.Marshal(map[string]json.RawMessage{field: current[field]})
			now, e := eval.Score(scoped, raw)
			must(e)
			oldRaw, _ := json.Marshal(map[string]json.RawMessage{field: previous[field]})
			before, e := eval.Score(scoped, oldRaw)
			must(e)
			fields = append(fields, map[string]any{"id": c.ID, "field": field, "status": w.Code, "baseline": before, "currentControlledReplay": now})
		}
	}
	report := map[string]any{"mode": "controlled-offline", "fixtureHashes": map[string]string{"retrieval": eval.Hash(labelsRaw), "outputs": eval.Hash(casesRaw), "baseline": eval.Hash(baselineRaw)}, "candidateRecall": rows, "fieldResults": fields, "tokens": nil, "limitations": "Replays fixed outputs, including known bad outputs. Candidate labels are inherited from #45; baseline full approved candidate set has no retrieval omissions. Field metrics use #34 clause labels; unsupported candidates require adjudication. No live inference, billing, latency, quality or token improvement is established."}
	raw, err := json.MarshalIndent(report, "", "  ")
	must(err)
	must(os.WriteFile(*output, append(raw, '\n'), 0644))
	fmt.Println(*output)
}
