package evaluationlab

import "sort"

func appendUnique(values []string, value string) []string {
	if value != "" && !contains(values, value) {
		values = append(values, value)
	}
	return values
}

func summaryMetadata(summary RunSummary, cases []ImportedCase, dispatches []Dispatch, versions []string) RunSummary {
	for _, c := range cases {
		summary.Workflows = appendUnique(summary.Workflows, c.Workflow)
		summary.Languages = appendUnique(summary.Languages, c.Language)
		summary.Outcomes = appendUnique(summary.Outcomes, c.OperationalOutcome)
		for _, category := range c.FailureFamilies {
			summary.Categories = appendUnique(summary.Categories, category)
		}
	}
	for _, d := range dispatches {
		summary.Models = appendUnique(summary.Models, d.Call.Model)
		summary.ReturnedModels = appendUnique(summary.ReturnedModels, d.Call.ReturnedModel)
	}
	for _, version := range versions {
		summary.Versions = appendUnique(summary.Versions, version)
	}
	for _, values := range [][]string{summary.Workflows, summary.Languages, summary.Categories, summary.Outcomes, summary.Models, summary.ReturnedModels, summary.Versions} {
		sort.Strings(values)
	}
	return summary
}

func matchesSummary(s RunSummary, f RunFilter) bool {
	if f.Suite != "" && s.Suite != f.Suite || f.EngineState != "" && s.EngineState != f.EngineState || f.Mode != "" && s.Mode != f.Mode {
		return false
	}
	for _, match := range []struct {
		value  string
		values []string
	}{{f.Workflow, s.Workflows}, {f.Language, s.Languages}, {f.Category, s.Categories}, {f.Outcome, append(append([]string{}, s.Outcomes...), s.AssertionVerdict, s.EngineState)}, {f.Model, s.Models}, {f.ReturnedModel, s.ReturnedModels}, {f.Version, s.Versions}} {
		if match.value != "" && !contains(match.values, match.value) {
			return false
		}
	}
	return true
}
