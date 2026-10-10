package semanticeval

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func safeEvaluationModel(model string) string {
	for _, base := range []string{"gpt-6-luna", "gpt-6.1-sol", "jev-1.13.0"} {
		if model == base {
			return model
		}
		if suffix, ok := strings.CutPrefix(model, base+"-"); ok {
			if _, err := time.Parse("2006-01-02", suffix); err == nil {
				return model
			}
		}
	}
	return ""
}

// Budget is shared by every attempt in a deliberate, sequential evaluation run.
// It counts dispatch attempts, including transport failures and field validation.
type Budget struct{ Limit, Used, WorkflowLimit, Workflows int }

type Configuration struct {
	ID        string `json:"id"`
	Model     string `json:"model"`
	Reasoning string `json:"reasoning"`
}

func Configurations() []Configuration {
	return []Configuration{{"current", "gpt-6-luna", "none"}, {"luna-low", "gpt-6-luna", "low"}, {"sol-low", "gpt-6.1-sol", "low"}}
}

func configuration(id string) (Configuration, error) {
	for _, c := range Configurations() {
		if c.ID == id {
			return c, nil
		}
	}
	return Configuration{}, fmt.Errorf("unknown evaluation configuration %q", id)
}

// RunConfigured changes only the evaluation transport's fixed OpenAI settings.
// Production handlers, instruction text, output bounds and Jev remain unchanged.
func RunConfigured(c Case, mode, openAIKey, typeSafeKey, id string, budget *Budget) (RunResult, error) {
	config, err := configuration(id)
	if err != nil {
		return RunResult{}, err
	}
	if budget == nil || budget.Limit < 1 || budget.Used >= budget.Limit {
		return RunResult{}, fmt.Errorf("provider call budget exhausted")
	}
	if len(c.Steps) > 0 {
		return runSteps(c, mode, openAIKey, typeSafeKey, &config, budget)
	}
	return run(c, mode, openAIKey, typeSafeKey, &config, budget)
}

func runSteps(c Case, mode, key, gate string, config *Configuration, budget *Budget) (RunResult, error) {
	result := RunResult{ID: c.ID, Task: c.Task, Language: c.Language, Split: c.Split, Role: c.Role, Mode: mode, Status: 200, NoStore: true}
	combined := map[string][]json.RawMessage{"claims": {}, "operations": {}}
	for _, step := range c.Steps {
		part, err := run(step, mode, key, gate, config, budget)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, part)
		result.Calls = append(result.Calls, part.Calls...)
		result.NoStore = result.NoStore && part.NoStore
		var out map[string]json.RawMessage
		_ = json.Unmarshal(part.Output, &out)
		for _, field := range []string{"claims", "operations"} {
			var units []json.RawMessage
			_ = json.Unmarshal(out[field], &units)
			combined[field] = append(combined[field], units...)
		}
		if part.Status != 200 {
			result.Status = part.Status
			break
		}
	}
	result.Output, _ = json.Marshal(combined)
	return Assess(c, result)
}
