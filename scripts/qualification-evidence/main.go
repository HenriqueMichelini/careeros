// Credential-free, labeled retrieval and controlled production-handler replay.
// Contract replay accuracy is separate from (unmeasured) live model accuracy.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/jobcontext"
	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/profilevalidation"
	"professional-information-repo/internal/qualificationmatching"
	"professional-information-repo/internal/semanticeval"
)

type label struct {
	ID          string                         `json:"id"`
	Language    string                         `json:"language"`
	Requirement string                         `json:"requirement"`
	Document    json.RawMessage                `json:"document"`
	Relevant    []string                       `json:"relevantFactIds"`
	Negative    []string                       `json:"negativeFactIds"`
	Expected    string                         `json:"expectedState"`
	Decision    qualificationmatching.Decision `json:"providerDecision"`
}
type retrieval struct {
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
type final struct {
	ID       string `json:"id"`
	Status   int    `json:"status"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Correct  bool   `json:"correct"`
}

func main() {
	input := flag.String("fixtures", "docs/evaluations/qualification-evidence/cases.json", "labeled synthetic corpus")
	output := flag.String("output", "/tmp/qualification-evidence-report.json", "report path")
	flag.Parse()
	raw, err := os.ReadFile(*input)
	must(err)
	var corpus struct {
		Version string  `json:"version"`
		Cases   []label `json:"cases"`
	}
	must(json.Unmarshal(raw, &corpus))
	report := struct {
		Mode        string      `json:"mode"`
		FixtureHash string      `json:"fixtureHash"`
		Retrieval   []retrieval `json:"candidateRetrieval"`
		Final       []final     `json:"controlledFinalMatching"`
		Limit       string      `json:"limitation"`
	}{Mode: "controlled-offline", FixtureHash: semanticeval.Hash(raw), Limit: "Final matching replays labeled provider outputs through the production HTTP handler. It measures contract preservation, not live semantic model accuracy. Candidate recall is measured independently at each byte budget. No provider or billing calls."}
	for _, c := range corpus.Cases {
		doc, err := qualificationmatching.Projection(c.Document)
		must(err)
		for _, budget := range []int{2048, 8192, 16384} {
			selected := qualificationmatching.Select(doc, c.Requirement, 0, budget)
			kept := map[string]bool{}
			for _, f := range selected.Facts {
				kept[f.ID] = true
			}
			row := retrieval{ID: c.ID, Budget: budget, Relevant: len(c.Relevant), Complete: selected.Complete, Misses: []string{}, NegativeMisses: []string{}}
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
		prepared, err := preprocessing.PrepareBoundedWithContext(context.Background(), c.Requirement, fieldvalidation.JobPosting)
		must(err)
		source := prepared.Source
		job := jobcontext.Job{Responsibilities: []jobcontext.Item{}, ApplicationRequirements: []jobcontext.Item{}, Qualifications: []jobcontext.Item{{Source: jobcontext.Reference{SegmentID: source.Segments()[0].ID, Quote: source.Text(), Occurrence: 0}, Importance: "required"}}}
		repo := profilevalidation.Profile{Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
		request, _ := json.Marshal(map[string]any{"repository": repo, "jobPosting": c.Requirement, "understandJob": true, "profileEvidence": doc})
		extracted, _ := json.Marshal(map[string]any{"gaps": []any{}, "job": job})
		decision, _ := json.Marshal(map[string]any{"matches": []qualificationmatching.Decision{c.Decision}})
		run, err := semanticeval.Run(semanticeval.Case{ID: c.ID, Task: "qualification_gaps", Language: c.Language, Request: request, Provider: []json.RawMessage{extracted, decision}}, "controlled", "", "")
		must(err)
		var result struct {
			Matches []qualificationmatching.Match `json:"matches"`
		}
		must(json.Unmarshal(run.Output, &result))
		actual := "rejected"
		if len(result.Matches) == 1 {
			actual = result.Matches[0].State
		}
		report.Final = append(report.Final, final{c.ID, run.Status, c.Expected, actual, run.Status == 200 && actual == c.Expected})
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	must(err)
	must(os.WriteFile(*output, append(encoded, '\n'), 0600))
	correct := 0
	for _, row := range report.Final {
		if row.Correct {
			correct++
		}
	}
	fmt.Printf("%d retrieval rows; %d/%d controlled final decisions preserved; report %s\n", len(report.Retrieval), correct, len(report.Final), *output)
	if correct != len(report.Final) {
		os.Exit(1)
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
