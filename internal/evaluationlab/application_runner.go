package evaluationlab

import (
	"context"
	"encoding/json"
	eval "professional-information-repo/internal/semanticeval"
	"regexp"
	"strings"
)

type ApplicationContextReport struct {
	Retrieval     []any             `json:"candidateRecall"`
	Fields        []any             `json:"fieldResults"`
	FixtureHashes map[string]string `json:"fixtureHashes"`
	Limitations   string            `json:"limitations"`
	Mode          string            `json:"mode"`
	Tokens        any               `json:"tokens"`
}

func runApplicationFixture(ctx context.Context, input any, content json.RawMessage, capture func(int)) (eval.RunResult, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return eval.RunResult{}, err
	}
	ctx = eval.WithRequestObservation(ctx, func(provider string, size int) {
		if provider == "api.openai.com" {
			capture(size)
		}
	})
	return eval.RunContext(ctx, eval.Case{Task: "application_draft", Request: raw, Provider: []json.RawMessage{content}}, "controlled", "", "")
}
func RunApplicationContext(ctx context.Context, labelsRaw, casesRaw, baselineRaw []byte, selected []string) (ApplicationContextReport, error) {
	var report ApplicationContextReport
	var labels struct {
		Cases []struct {
			ID, Requirement string
			Document        json.RawMessage
			Relevant        []string `json:"relevantFactIds"`
			Negative        []string `json:"negativeFactIds"`
		}
	}
	if err := json.Unmarshal(labelsRaw, &labels); err != nil {
		return report, err
	}
	var corpus struct{ Cases []eval.Case }
	if err := json.Unmarshal(casesRaw, &corpus); err != nil {
		return report, err
	}
	var baseline struct{ Runs []eval.RunResult }
	if err := json.Unmarshal(baselineRaw, &baseline); err != nil {
		return report, err
	}
	rows := []any{}
	fields := []any{}
	content := json.RawMessage(`{"jobTitle":null,"company":null,"jobSummary":"Java required.","resume":"Java","coverLetter":{"greeting":"Dear team,","body":"I use Java.","closing":"Sincerely,"},"applicationAnswers":"Java."}`)
	available := []string{}
	for _, c := range labels.Cases {
		available = append(available, c.ID)
	}
	for _, c := range corpus.Cases {
		if c.Task == "application_draft" {
			available = append(available, c.ID)
		}
	}
	wanted, err := selectedFixtureIDs(selected, available)
	if err != nil {
		return report, err
	}
	for _, c := range labels.Cases {
		if !wanted[c.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		input := map[string]any{"repository": map[string]any{"careerGoals": "", "skills": "", "competencies": "", "experience": []any{}, "tools": "", "projects": []any{}, "employmentStatus": "", "currentSalary": "", "desiredSalary": "", "additionalInfo": ""}, "jobPosting": c.Requirement, "confirmedQualifications": []any{}, "profileEvidence": c.Document}
		outbound := 0
		w, err := runApplicationFixture(ctx, input, content, func(size int) { outbound = size })
		if err != nil {
			return report, err
		}
		var result struct {
			Selection struct {
				Sources                           []string
				BudgetExcluded, RelevanceExcluded int
				Complete                          bool
			} `json:"contextSelection"`
		}
		json.Unmarshal(w.Output, &result)
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
		rows = append(rows, map[string]any{"id": c.ID, "status": w.Status, "relevant": len(c.Relevant), "retrieved": len(c.Relevant) - len(missing), "materialMisses": missing, "negativeMisses": negative, "selection": result.Selection, "providerEnvelopeBytes": outbound, "baselineApprovedCandidateRecall": 1.0})
	}
	for _, c := range corpus.Cases {
		if c.Task != "application_draft" || !wanted[c.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		var input any
		json.Unmarshal(c.Request, &input)
		w, err := runApplicationFixture(ctx, input, c.Provider[0], func(int) {})
		if err != nil {
			return report, err
		}
		var current map[string]json.RawMessage
		json.Unmarshal(w.Output, &current)
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
			if err := e; err != nil {
				return report, err
			}
			oldRaw, _ := json.Marshal(map[string]json.RawMessage{field: previous[field]})
			before, e := eval.Score(scoped, oldRaw)
			if err := e; err != nil {
				return report, err
			}
			fields = append(fields, map[string]any{"id": c.ID, "field": field, "status": w.Status, "baseline": before, "currentControlledReplay": now})
		}
	}
	report = ApplicationContextReport{Retrieval: rows, Fields: fields, FixtureHashes: map[string]string{"retrieval": eval.Hash(labelsRaw), "outputs": eval.Hash(casesRaw), "baseline": eval.Hash(baselineRaw)}, Limitations: "Replays fixed outputs, including known bad outputs. Candidate labels are inherited from #45; baseline full approved candidate set has no retrieval omissions. Field metrics use #34 clause labels; unsupported candidates require adjudication. No live inference, billing, latency, quality or token improvement is established.", Mode: "controlled-offline"}
	return report, nil
}
