package evaluationlab

import (
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
	"sort"
)

type ComparisonContract struct {
	FixtureHash     string `json:"fixtureHash"`
	LabelHash       string `json:"labelHash"`
	SourceVersion   string `json:"sourceVersion"`
	PromptVersion   string `json:"promptVersion"`
	SchemaVersion   string `json:"schemaVersion"`
	ScorerVersion   string `json:"scorerVersion"`
	StrategyVersion string `json:"strategyVersion"`
	BudgetVersion   string `json:"budgetVersion"`
	RuntimeVersion  string `json:"runtimeVersion"`
	Mode            string `json:"mode"`
	Configuration   string `json:"configuration"`
}

type Compatibility struct {
	Version          string   `json:"version"`
	ChangedDimension string   `json:"changedDimension"`
	Compatible       bool     `json:"compatible"`
	Reasons          []string `json:"reasons"`
}

type CaseDelta struct {
	Language                string `json:"language"`
	Workflow                string `json:"workflow"`
	Kind                    string `json:"kind"`
	CaseID                  string `json:"caseId"`
	LeftIdentity            string `json:"leftIdentity"`
	RightIdentity           string `json:"rightIdentity"`
	ForbiddenAdditionsDelta *int   `json:"curatedForbiddenAdditionsDelta"`
	MissingLabelsDelta      *int   `json:"curatedMissingLabelsDelta"`
	RelationshipErrorsDelta *int   `json:"curatedRelationshipErrorsDelta"`
	OperationalDisagreement bool   `json:"operationalDisagreement"`
	AssertionDisagreement   bool   `json:"assertionDisagreement"`
	UnavailableReason       string `json:"unavailableReason,omitempty"`
}

type CaseComparison struct {
	Compatibility Compatibility `json:"compatibility"`
	Partial       bool          `json:"partial"`
	Deltas        []CaseDelta   `json:"deltas"`
	Pooling       string        `json:"pooling"`
}

// CompareCases compares observed curated units only, after a complete pairing
// contract. It does not pool languages, translated pairs or quality dimensions.
func CompareCases(a, b ComparisonContract, dimension string, left, right []ImportedCase) CaseComparison {
	r := CaseComparison{Compatibility: CheckComparison(a, b, dimension), Deltas: []CaseDelta{}, Pooling: "none: per-case finite evidence only"}
	if !r.Compatibility.Compatible {
		return r
	}
	index := func(cases []ImportedCase, configuration string) (map[string]ImportedCase, error) {
		rows := map[string]ImportedCase{}
		for _, c := range cases {
			if c.Configuration != "" && c.Configuration != configuration {
				continue
			}
			var scope struct {
				Field  string `json:"field"`
				Budget int    `json:"budgetBytes"`
			}
			if json.Unmarshal(c.Actual, &scope) != nil {
				return nil, fmt.Errorf("invalid case evidence")
			}
			key := fmt.Sprintf("%s/%s/%s/%s/%d", c.ID, c.Kind, c.Language, scope.Field, scope.Budget)
			if _, exists := rows[key]; exists {
				return nil, fmt.Errorf("ambiguous repeated case: %s", c.ID)
			}
			rows[key] = c
		}
		return rows, nil
	}
	l, err := index(left, a.Configuration)
	if err != nil {
		r.Compatibility.Compatible = false
		r.Compatibility.Reasons = append(r.Compatibility.Reasons, err.Error())
		return r
	}
	rightIndex, err := index(right, b.Configuration)
	if err != nil {
		r.Compatibility.Compatible = false
		r.Compatibility.Reasons = append(r.Compatibility.Reasons, err.Error())
		return r
	}
	keys := map[string]bool{}
	for key := range l {
		keys[key] = true
	}
	for key := range rightIndex {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	if len(ordered) == 0 {
		r.Partial = true
		r.Compatibility.Reasons = append(r.Compatibility.Reasons, "no paired cases")
	}
	for _, key := range ordered {
		lc, lok := l[key]
		rc, rok := rightIndex[key]
		scope := lc
		if !lok {
			scope = rc
		}
		delta := CaseDelta{CaseID: lc.ID, LeftIdentity: lc.Identity, RightIdentity: rc.Identity, Language: scope.Language, Workflow: scope.Workflow, Kind: scope.Kind}
		if delta.CaseID == "" {
			delta.CaseID = rc.ID
		}
		if !lok || !rok {
			r.Partial = true
			delta.UnavailableReason = "case missing from one comparison cohort"
			r.Compatibility.Reasons = append(r.Compatibility.Reasons, key+": missing counterpart")
			r.Deltas = append(r.Deltas, delta)
			continue
		}
		delta.OperationalDisagreement = lc.OperationalOutcome != rc.OperationalOutcome
		delta.AssertionDisagreement = lc.AssertionVerdict != rc.AssertionVerdict
		score := func(c ImportedCase) (*semanticeval.Result, bool) {
			var observed struct {
				Status  int                  `json:"status"`
				Score   *semanticeval.Result `json:"score"`
				Current *semanticeval.Result `json:"currentControlledReplay"`
			}
			if json.Unmarshal(c.Actual, &observed) != nil {
				return nil, false
			}
			if c.Kind == "workflow-result" {
				var run semanticeval.RunResult
				_ = json.Unmarshal(c.Actual, &run)
				if !semanticeval.Completed(run) {
					return nil, false
				}
			} else if observed.Status != 200 {
				return nil, false
			}
			if observed.Score == nil {
				observed.Score = observed.Current
			}
			return observed.Score, observed.Score != nil
		}
		ls, la := score(lc)
		rs, ra := score(rc)
		if la && ra {
			forbidden := rs.ForbiddenAdditions - ls.ForbiddenAdditions
			missing := len(rs.Missing) - len(ls.Missing)
			relationships := rs.RelationshipErrors - ls.RelationshipErrors
			delta.ForbiddenAdditionsDelta = &forbidden
			delta.MissingLabelsDelta = &missing
			delta.RelationshipErrorsDelta = &relationships
		} else {
			r.Partial = true
			delta.UnavailableReason = "curated score unavailable for a failed, incomplete or unscored case"
			r.Compatibility.Reasons = append(r.Compatibility.Reasons, key+": score unavailable")
		}
		r.Deltas = append(r.Deltas, delta)
	}
	if r.Partial {
		r.Compatibility.Compatible = false
	}
	return r
}

// CheckComparison requires complete evidence of controlled pairing. Missing
// metadata is a reason to decline comparison, never a wildcard match.
func CheckComparison(a, b ComparisonContract, changedDimension string) Compatibility {
	r := Compatibility{Version: "lab-comparison-v1", ChangedDimension: changedDimension, Reasons: []string{}}
	if !contains([]string{"configuration", "source", "prompt", "schema", "scorer", "strategy", "budget"}, changedDimension) {
		r.Reasons = append(r.Reasons, "unsupported changed dimension")
	}
	changed := false
	for _, field := range []struct{ name, left, right string }{
		{"fixture", a.FixtureHash, b.FixtureHash}, {"labels", a.LabelHash, b.LabelHash},
		{"source", a.SourceVersion, b.SourceVersion}, {"prompt", a.PromptVersion, b.PromptVersion},
		{"schema", a.SchemaVersion, b.SchemaVersion}, {"scorer", a.ScorerVersion, b.ScorerVersion},
		{"strategy", a.StrategyVersion, b.StrategyVersion}, {"budget", a.BudgetVersion, b.BudgetVersion},
		{"runtime", a.RuntimeVersion, b.RuntimeVersion}, {"mode", a.Mode, b.Mode}, {"configuration", a.Configuration, b.Configuration},
	} {
		if field.left == "" || field.right == "" {
			r.Reasons = append(r.Reasons, field.name+": unavailable compatibility evidence")
			continue
		}
		if field.left == field.right {
			continue
		}
		if field.name != changedDimension {
			r.Reasons = append(r.Reasons, field.name+": incompatible values")
		} else {
			changed = true
		}
	}
	if !changed {
		r.Reasons = append(r.Reasons, "declared dimension did not change")
	}
	r.Compatible = len(r.Reasons) == 0
	return r
}
