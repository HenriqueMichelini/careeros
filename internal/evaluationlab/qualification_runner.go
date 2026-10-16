package evaluationlab

import (
	"context"
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/jobcontext"
	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/profilevalidation"
	"professional-information-repo/internal/qualificationmatching"
	"professional-information-repo/internal/semanticeval"
)

type qualificationLabel struct {
	ID          string                         `json:"id"`
	Language    string                         `json:"language"`
	Requirement string                         `json:"requirement"`
	Document    json.RawMessage                `json:"document"`
	Relevant    []string                       `json:"relevantFactIds"`
	Negative    []string                       `json:"negativeFactIds"`
	Expected    string                         `json:"expectedState"`
	Decision    qualificationmatching.Decision `json:"providerDecision"`
}
type QualificationRetrieval struct {
	Actual         string   `json:"controlledMatchState"`
	Correct        bool     `json:"controlledMatchCorrect"`
	ID             string   `json:"id"`
	Budget         int      `json:"budgetBytes"`
	Relevant       int      `json:"relevant"`
	Retrieved      int      `json:"retrieved"`
	Misses         []string `json:"materialMisses"`
	NegativeMisses []string `json:"negativeMisses"`
	Complete       bool     `json:"complete"`
}
type QualificationFinal struct {
	ID       string `json:"id"`
	Status   int    `json:"status"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Correct  bool   `json:"correct"`
}

type QualificationReport struct {
	Mode        string                   `json:"mode"`
	FixtureHash string                   `json:"fixtureHash"`
	Retrieval   []QualificationRetrieval `json:"candidateRetrieval"`
	Final       []QualificationFinal     `json:"controlledFinalMatching"`
	Limit       string                   `json:"limitation"`
}

func RunQualificationEvidence(ctx context.Context, raw []byte, selected []string) (QualificationReport, error) {
	var report QualificationReport
	var corpus struct {
		Version string               `json:"version"`
		Cases   []qualificationLabel `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		return report, err
	}
	report = QualificationReport{Mode: "controlled-offline", FixtureHash: semanticeval.Hash(raw), Limit: "Final matching replays labeled provider outputs through the production HTTP handler. It measures contract preservation, not live semantic model accuracy. Candidate recall is measured independently at each byte budget. No provider or billing calls."}
	wanted, err := selectedFixtureIDs(selected, qualificationIDs(corpus.Cases))
	if err != nil {
		return report, err
	}
	for _, c := range corpus.Cases {
		if !wanted[c.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return report, err
		}
		doc, err := qualificationmatching.Projection(c.Document)
		if err != nil {
			return report, err
		}
		for _, budget := range []int{2048, 8192, 16384} {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			selected := qualificationmatching.Select(doc, c.Requirement, 0, budget)
			kept := map[string]bool{}
			for _, f := range selected.Facts {
				kept[f.ID] = true
			}
			row := QualificationRetrieval{ID: c.ID, Budget: budget, Relevant: len(c.Relevant), Complete: selected.Complete, Misses: []string{}, NegativeMisses: []string{}}
			for _, id := range c.Relevant {
				if kept[id] {
					row.Retrieved++
				} else {
					row.Misses = append(row.Misses, id)
				}
			}
			for _, id := range c.Negative {
				if !kept[id] {
					row.NegativeMisses = append(row.NegativeMisses, id)
				}
			}
			requirements := []jobcontext.ResolvedItem{{Source: jobcontext.Evidence{Reference: jobcontext.Reference{Quote: c.Requirement}}, Importance: "required"}}
			resolved, resolveErr := qualificationmatching.Resolve(requirements, []qualificationmatching.Candidates{selected}, []qualificationmatching.Decision{c.Decision})
			row.Actual = "rejected_unresolved_reference"
			if resolveErr == nil {
				row.Actual = resolved[0].State
			}
			row.Correct = row.Actual == c.Expected
			report.Retrieval = append(report.Retrieval, row)
		}
		prepared, err := preprocessing.PrepareBoundedWithContext(ctx, c.Requirement, fieldvalidation.JobPosting)
		if err != nil {
			return report, err
		}
		source := prepared.Source
		job := jobcontext.Job{Responsibilities: []jobcontext.Item{}, ApplicationRequirements: []jobcontext.Item{}, Qualifications: []jobcontext.Item{{Source: jobcontext.Reference{SegmentID: source.Segments()[0].ID, Quote: source.Text(), Occurrence: 0}, Importance: "required"}}}
		repo := profilevalidation.Profile{Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
		request, _ := json.Marshal(map[string]any{"repository": repo, "jobPosting": c.Requirement, "understandJob": true, "profileEvidence": doc})
		extracted, _ := json.Marshal(map[string]any{"gaps": []any{}, "job": job})
		decision, _ := json.Marshal(map[string]any{"matches": []qualificationmatching.Decision{c.Decision}})
		run, err := semanticeval.RunContext(ctx, semanticeval.Case{ID: c.ID, Task: "qualification_gaps", Language: c.Language, Request: request, Provider: []json.RawMessage{extracted, decision}}, "controlled", "", "")
		if err != nil {
			return report, err
		}
		var result struct {
			Matches []qualificationmatching.Match `json:"matches"`
		}
		if err := json.Unmarshal(run.Output, &result); err != nil {
			return report, err
		}
		actual := "rejected"
		if len(result.Matches) == 1 {
			actual = result.Matches[0].State
		}
		report.Final = append(report.Final, QualificationFinal{c.ID, run.Status, c.Expected, actual, run.Status == 200 && actual == c.Expected})
	}
	return report, nil
}

func qualificationIDs(cases []qualificationLabel) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	return ids
}

func selectedFixtureIDs(selected, available []string) (map[string]bool, error) {
	known := map[string]bool{}
	for _, id := range available {
		known[id] = true
	}
	if len(selected) == 0 {
		return known, nil
	}
	wanted := map[string]bool{}
	for _, id := range selected {
		if !known[id] || wanted[id] {
			return nil, fmt.Errorf("unknown or duplicate selected fixture")
		}
		wanted[id] = true
	}
	return wanted, nil
}
