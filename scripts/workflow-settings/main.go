// Bounded synthetic comparison through production handlers. Never imported by
// production. No retry, fallback, prompt padding or retention modification.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	eval "professional-information-repo/internal/semanticeval"
	"runtime/debug"
	"strings"
	"time"
)

type corpus struct {
	Version string      `json:"version"`
	Cases   []eval.Case `json:"cases"`
}
type report struct {
	HTTPAttempts   int                                           `json:"httpWorkflowAttempts"`
	ByTaskLanguage map[string]map[string]*eval.ComparisonSummary `json:"byTaskLanguage"`
	DerivedFrom    string                                        `json:"derivedFromSHA256,omitempty"`
	Version        string                                        `json:"version"`
	Mode           string                                        `json:"mode"`
	Started        string                                        `json:"started"`
	Commit         string                                        `json:"commit"`
	FixtureHash    string                                        `json:"fixtureHash"`
	FixtureVersion string                                        `json:"fixtureVersion"`
	SourceHashes   map[string]string                             `json:"sourceHashes"`
	Configurations []eval.Configuration                          `json:"configurations"`
	Rates          []eval.Rate                                   `json:"datedPriceAssumptions"`
	CallLimit      int                                           `json:"callLimit"`
	WorkflowLimit  int                                           `json:"workflowLimit"`
	Calls          int                                           `json:"callsDispatched"`
	Runs           []eval.ComparisonRun                          `json:"runs"`
	Summary        map[string]*eval.ComparisonSummary            `json:"summary"`
	Decision       string                                        `json:"decision"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "controlled", "controlled or live")
	authorize := flag.Bool("authorize-live", false, "explicit synthetic provider authorization")
	fixtures := flag.String("fixtures", "docs/evaluations/workflow-settings/cases.v1.json", "frozen fixtures")
	output := flag.String("output", "", "new report file")
	openai := flag.String("openai-key-file", "", "private assignment file")
	typesafe := flag.String("typesafe-key-file", "", "private assignment file")
	calls := flag.Int("max-calls", 300, "total dispatch cap, at most 300")
	workflows := flag.Int("max-workflows", 60, "HTTP workflow cap, at most 60")
	replay := flag.String("replay", "", "saved comparison for offline review; no provider calls")
	reviews := flag.String("reviews", "", "source-grounded review annotations")
	flag.Parse()
	if *replay != "" {
		if *mode != "controlled" || *authorize || *openai != "" || *typesafe != "" {
			return fmt.Errorf("offline review refuses credentials/live authorization")
		}
		return reassess(*replay, *reviews, *output)
	}
	if (*mode != "controlled" && *mode != "live") || (*mode == "live" && !*authorize) || *calls < 1 || *calls > 300 || *workflows < 1 || *workflows > 60 || *output == "" {
		return fmt.Errorf("invalid mode, authorization, limits or output")
	}
	if *mode == "controlled" && (*openai != "" || *typesafe != "" || *authorize) {
		return fmt.Errorf("controlled runs refuse credential/authorization arguments")
	}
	raw, err := os.ReadFile(*fixtures)
	if err != nil {
		return err
	}
	var data corpus
	if err = json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if data.Version == "" {
		return fmt.Errorf("fixture version required")
	}
	if err = eval.Validate(data.Cases); err != nil {
		return err
	}
	// Naturally shared instruction prefixes measure ordinary cache behavior.
	if workflowCount(data.Cases)*len(eval.Configurations()) > *workflows {
		return fmt.Errorf("matrix exceeds authorized workflow cap")
	}
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("report path must be new and writable")
	}
	defer file.Close()
	key, gate := "", ""
	if *mode == "live" {
		key, err = credential(*openai, "OPENAI_API_KEY")
		if err != nil {
			return err
		}
		gate, err = credential(*typesafe, "TYPESAFE_API_KEY")
		if err != nil {
			return err
		}
	}
	hashes, err := sourceHashes()
	if err != nil {
		return err
	}
	rep := report{Version: "workflow-settings-v1", Mode: *mode, Started: time.Now().UTC().Format(time.RFC3339), Commit: revision(), FixtureHash: eval.Hash(raw), FixtureVersion: data.Version, SourceHashes: hashes, Configurations: eval.Configurations(), Rates: []eval.Rate{{Model: "gpt-6-luna", AsOf: "2026-10-10", Input: 0.1, Cached: 0.01, Write: 0.125, Output: 0.5}, {Model: "gpt-6.1-sol", AsOf: "2026-10-10", Input: 2, Cached: 0.1, Write: 2.5, Output: 10}}, CallLimit: *calls, WorkflowLimit: *workflows, Runs: []eval.ComparisonRun{}, Decision: "retain current fixed settings; no candidate improvement demonstrated; acceptance review pending"}
	budget := &eval.Budget{Limit: *calls, WorkflowLimit: *workflows}
	if err = save(file, &rep); err != nil {
		return err
	}
	// Interleave configurations to reduce order/time bias. Heldout labels remain frozen.
	for _, c := range data.Cases {
		for _, config := range rep.Configurations {
			if err = attempt(&rep, file, c, config.ID, false, *mode, key, gate, budget); err != nil {
				return err
			}
		}
	}

	return nil
}
func attempt(rep *report, file *os.File, c eval.Case, config string, repeat bool, mode, key, gate string, budget *eval.Budget) error {
	run, err := eval.RunConfigured(c, mode, key, gate, config, budget)
	if err != nil {
		run.Status = 502
		run.Score.Failures = append(run.Score.Failures, "evaluation interrupted")
	}
	rep.Runs = append(rep.Runs, eval.ComparisonRun{Configuration: config, Repeat: repeat, Run: run})
	rep.Calls = budget.Used
	rep.HTTPAttempts = budget.Workflows
	fmt.Fprintf(os.Stderr, "configuration=%s case=%s repeat=%t status=%d calls=%d\n", config, c.ID, repeat, run.Status, budget.Used)
	if saveErr := save(file, rep); saveErr != nil {
		return saveErr
	}
	return err
}
func save(file *os.File, rep *report) error {
	summary, err := eval.Compare(rep.Runs, rep.Rates)
	if err != nil {
		return err
	}
	rep.Summary = summary
	rep.ByTaskLanguage = map[string]map[string]*eval.ComparisonSummary{}
	groups := map[string][]eval.ComparisonRun{}
	for _, run := range rep.Runs {
		key := run.Run.Task + "/" + run.Run.Language
		groups[key] = append(groups[key], run)
	}
	for key, runs := range groups {
		group, err := eval.Compare(runs, rep.Rates)
		if err != nil {
			return err
		}
		rep.ByTaskLanguage[key] = group
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if _, err = file.Seek(0, 0); err != nil {
		return err
	}
	if err = file.Truncate(0); err != nil {
		return err
	}
	if _, err = file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return file.Sync()
}
func credential(path, name string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s credential", name)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "export ")), "=")
		if ok && strings.TrimSpace(k) == name && strings.TrimSpace(v) != "" {
			return strings.Trim(strings.TrimSpace(v), "\"'"), nil
		}
	}
	return "", fmt.Errorf("missing %s assignment", name)
}
func revision() string {
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, s := range b.Settings {
			if s.Key == "vcs.revision" {
				return s.Value
			}
		}
	}
	return "unknown"
}
func sourceHashes() (map[string]string, error) {
	hashes := map[string]string{}
	for _, root := range []string{"backend", "internal", "scripts/workflow-settings"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hashes[path] = eval.Hash(raw)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

func workflowCount(cases []eval.Case) int {
	total := 0
	for _, c := range cases {
		total += max(1, len(c.Steps))
	}
	return total
}

func reassess(path, annotations, output string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var rep report
	if err = json.Unmarshal(raw, &rep); err != nil {
		return err
	}
	if rep.Version != "workflow-settings-v1" {
		return fmt.Errorf("unknown report version")
	}
	rep.DerivedFrom = eval.Hash(raw)
	var reviews []struct {
		Index  int         `json:"index"`
		Review eval.Review `json:"review"`
	}
	raw, err = os.ReadFile(annotations)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &reviews); err != nil {
		return err
	}
	seen := map[int]bool{}
	for _, r := range reviews {
		if r.Index < 0 || r.Index >= len(rep.Runs) || seen[r.Index] {
			return fmt.Errorf("invalid/duplicate review index")
		}
		seen[r.Index] = true
		copy := r.Review
		rep.Runs[r.Index].Review = &copy
	}
	rep.Decision = "retain current fixed settings; no candidate improvement demonstrated; see source-grounded review and limitations"
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("report path must be new and writable")
	}
	defer file.Close()
	return save(file, &rep)
}
