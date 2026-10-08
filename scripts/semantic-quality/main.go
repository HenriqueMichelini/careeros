// Local, opt-in evaluation of production workflow contracts. No hosted eval
// service, automatic retries, semantic model judge or credential persistence.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	eval "professional-information-repo/internal/semanticeval"
)

type corpus struct {
	Version string      `json:"version"`
	Cases   []eval.Case `json:"cases"`
}
type report struct {
	Version        string            `json:"version"`
	Mode           string            `json:"mode"`
	Started        string            `json:"started"`
	Commit         string            `json:"commit"`
	FixtureVersion string            `json:"fixtureVersion"`
	FixtureHash    string            `json:"fixtureHash"`
	FixturePath    string            `json:"fixturePath"`
	DerivedFrom    []evidenceRef     `json:"derivedFrom,omitempty"`
	SourceHashes   map[string]string `json:"sourceHashes"`
	Scope          string            `json:"scope"`
	Runs           []eval.RunResult  `json:"runs"`
}
type evidenceRef struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

func credential(path, name string) (string, error) {
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s credential file", name)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if key, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == name {
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			if value != "" {
				return value, nil
			}
		}
	}
	return "", fmt.Errorf("credential file must contain %s=value", name)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type options struct {
	mode, fixtures, replay, split, ids, output, openAIPath, typeSafePath string
	authorizeLive                                                        bool
	limit                                                                int
}
type credentials struct{ openAI, typeSafe string }

func parseOptions() options {
	var o options
	flag.StringVar(&o.mode, "mode", "controlled", "controlled or live")
	flag.BoolVar(&o.authorizeLive, "authorize-live", false, "explicit authorization for provider costs")
	flag.StringVar(&o.fixtures, "fixtures", "docs/evaluations/semantic-quality/cases.v2.json", "frozen synthetic corpus")
	flag.StringVar(&o.replay, "replay", "", "comma-separated saved reports to rescore offline; never calls providers")
	flag.StringVar(&o.split, "split", "development", "development or heldout; all requires controlled mode")
	flag.StringVar(&o.ids, "ids", "", "explicit comma-separated fixture IDs (overrides split)")
	flag.IntVar(&o.limit, "limit", 8, "maximum live workflows; 1 through 8")
	flag.StringVar(&o.output, "output", "", "new report file; includes only synthetic generated content")
	flag.StringVar(&o.openAIPath, "openai-key-file", "", "private assignment-format credential file")
	flag.StringVar(&o.typeSafePath, "typesafe-key-file", "", "private assignment-format credential file")
	flag.Parse()
	return o
}
func (o options) validate() error {
	if o.mode != "controlled" && o.mode != "live" {
		return fmt.Errorf("mode must be controlled or live")
	}
	if o.output == "" {
		return fmt.Errorf("new --output path required")
	}
	if o.replay != "" && (o.mode == "live" || o.authorizeLive || o.openAIPath != "" || o.typeSafePath != "") {
		return fmt.Errorf("offline replay cannot authorize live inference or read credentials")
	}
	if o.mode == "live" && (!o.authorizeLive || o.limit < 1 || o.limit > 8 || (o.split == "all" && o.ids == "")) {
		return fmt.Errorf("live requires --authorize-live, a limit of 1 through 8 and explicit split or IDs")
	}
	if o.split != "all" && o.split != "development" && o.split != "heldout" {
		return fmt.Errorf("unknown split")
	}
	return nil
}
func run() error {
	o := parseOptions()
	if err := o.validate(); err != nil {
		return err
	}
	data, raw, err := loadCorpus(o.fixtures)
	if err != nil {
		return err
	}
	saved, origins, err := loadSavedReports(o.replay)
	if err != nil {
		return err
	}
	selected, err := selectCases(data.Cases, o, saved)
	if err != nil {
		return err
	}
	// Resolve output and credentials before any provider request. Never overwrite evidence.
	file, err := os.OpenFile(o.output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("output must be new and writable")
	}
	defer file.Close()
	keys, err := loadCredentials(o)
	if err != nil {
		return err
	}
	result, err := newReport(o, data, raw, origins)
	if err != nil {
		return err
	}
	return executeCases(&result, file, selected, o, keys, saved)
}
func loadCorpus(path string) (corpus, []byte, error) {
	var data corpus
	raw, err := os.ReadFile(path)
	if err != nil {
		return data, nil, err
	}
	if err = json.Unmarshal(raw, &data); err != nil {
		return data, nil, err
	}
	if data.Version == "" {
		return data, nil, fmt.Errorf("fixture version required")
	}
	return data, raw, eval.Validate(data.Cases)
}
func loadSavedReports(paths string) (map[string]eval.RunResult, []evidenceRef, error) {
	saved := map[string]eval.RunResult{}
	origins := []evidenceRef{}
	if paths == "" {
		return saved, origins, nil
	}
	for _, path := range strings.Split(paths, ",") {
		original, ref, err := loadEvidence(strings.TrimSpace(path))
		if err != nil {
			return nil, nil, err
		}
		origins = append(origins, ref)
		for _, run := range original.Runs {
			if _, exists := saved[run.ID]; exists {
				return nil, nil, fmt.Errorf("duplicate original case %s", run.ID)
			}
			saved[run.ID] = run
		}
	}
	return saved, origins, nil
}
func loadEvidence(path string) (report, evidenceRef, error) {
	var original report
	ref := evidenceRef{Path: path}
	raw, err := os.ReadFile(path)
	if err != nil {
		return original, ref, err
	}
	if err = json.Unmarshal(raw, &original); err != nil {
		return original, ref, err
	}
	if original.Mode != "live" {
		return original, ref, fmt.Errorf("replay requires original live evidence")
	}
	ref.Hash = eval.Hash(raw)
	return original, ref, nil
}
func requestedIDs(ids string) map[string]bool {
	wanted := map[string]bool{}
	if ids == "" {
		return wanted
	}
	for _, id := range strings.Split(ids, ",") {
		wanted[strings.TrimSpace(id)] = true
	}
	return wanted
}
func selectCases(cases []eval.Case, o options, saved map[string]eval.RunResult) ([]eval.Case, error) {
	wanted := requestedIDs(o.ids)
	selected := []eval.Case{}
	for _, c := range cases {
		if o.replay != "" {
			prior, ok := saved[c.ID]
			if !ok {
				continue
			}
			if prior.Task != c.Task {
				return nil, fmt.Errorf("replay task mismatch for %s", c.ID)
			}
		}
		if selectedCase(c, wanted, o.split) {
			selected = append(selected, c)
		}
	}
	if len(selected) == 0 || (len(wanted) > 0 && len(selected) != len(wanted)) {
		return nil, fmt.Errorf("empty selection or unknown fixture ID")
	}
	if o.mode == "live" && len(selected) > o.limit {
		return nil, fmt.Errorf("selection exceeds authorized live limit")
	}
	return selected, nil
}
func selectedCase(c eval.Case, wanted map[string]bool, split string) bool {
	if len(wanted) > 0 {
		return wanted[c.ID]
	}
	return split == "all" || c.Split == split
}
func loadCredentials(o options) (credentials, error) {
	var keys credentials
	if o.mode != "live" {
		return keys, nil
	}
	var err error
	if keys.openAI, err = credential(o.openAIPath, "OPENAI_API_KEY"); err != nil {
		return keys, err
	}
	if keys.typeSafe, err = credential(o.typeSafePath, "TYPESAFE_API_KEY"); err != nil {
		return keys, err
	}
	if keys.openAI == "" || keys.typeSafe == "" {
		return keys, fmt.Errorf("both credential files required for live baseline")
	}
	return keys, nil
}
func newReport(o options, data corpus, raw []byte, origins []evidenceRef) (report, error) {
	result := report{Version: "semantic-quality-report-v2", Mode: o.mode, Started: time.Now().UTC().Format(time.RFC3339), Commit: buildRevision(), FixtureVersion: data.Version, FixtureHash: eval.Hash(raw), FixturePath: o.fixtures, DerivedFrom: origins, Scope: "local production HTTP handlers; synthetic inputs; no browser, deployment or universal factual guarantee", Runs: []eval.RunResult{}}
	if o.replay != "" {
		result.Mode = "replay"
		result.Scope = "offline rescoring of saved live responses; no new inference; original provider configuration/source hashes remain in derivedFrom reports"
	}
	hashes, err := sourceHashes()
	result.SourceHashes = hashes
	return result, err
}

// Read compiler-provided VCS metadata instead of executing a helper via PATH.
// go run -buildvcs=true records the revision; unavailable metadata stays explicit.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return "unknown"
}
func sourceHashes() (map[string]string, error) {
	hashes := map[string]string{}
	for _, path := range []string{"backend/profile-ingestion/main.go", "backend/qualification-gaps/main.go", "backend/cv-generation/main.go", "backend/application-draft/main.go", "internal/fieldvalidation/decision.go", "internal/fieldvalidation/questions.json", "internal/semanticeval/score.go", "internal/semanticeval/adapter.go", "scripts/semantic-quality/main.go"} {
		source, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, err
		}
		hashes[path] = eval.Hash(source)
	}
	return hashes, nil
}
func saveReport(file *os.File, result *report) error {
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
func evaluateCase(c eval.Case, o options, keys credentials, saved map[string]eval.RunResult) (eval.RunResult, error) {
	if o.replay == "" {
		return eval.Run(c, o.mode, keys.openAI, keys.typeSafe)
	}
	result := saved[c.ID]
	result.Mode = "replay"
	result.Role = c.Role
	return eval.Assess(c, result)
}
func executeCases(result *report, file *os.File, cases []eval.Case, o options, keys credentials, saved map[string]eval.RunResult) error {
	if err := saveReport(file, result); err != nil {
		return err
	}
	failed := 0
	for _, c := range cases {
		run, err := evaluateCase(c, o, keys, saved)
		if err != nil {
			return fmt.Errorf("fixture %s failed to evaluate: %w", c.ID, err)
		}
		result.Runs = append(result.Runs, run)
		if err = saveReport(file, result); err != nil {
			return err
		}
		fmt.Printf("%s %s HTTP=%d precision=%.3f completeness=%.3f forbidden=%d failures=%d\n", result.Mode, c.ID, run.Status, run.Score.Precision, run.Score.Completeness, run.Score.ForbiddenAdditions, len(run.Score.Failures))
		if len(run.Score.Failures) > 0 {
			failed++
		}
		if o.mode == "live" && run.Status != 200 {
			return fmt.Errorf("live run stopped on workflow failure; partial evidence saved")
		}
	}
	fmt.Printf("Saved %d cases; %d have label or contract failures. Human prose review remains separate.\n", len(result.Runs), failed)
	return nil // Baselines record quality failures; they are not CI pass claims.
}
