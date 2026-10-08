// Local, opt-in evaluation of production workflow contracts. No hosted eval
// service, automatic retries, semantic model judge or credential persistence.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	SourceHashes   map[string]string `json:"sourceHashes"`
	Scope          string            `json:"scope"`
	Runs           []eval.RunResult  `json:"runs"`
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
func run() error {
	mode := flag.String("mode", "controlled", "controlled or live")
	live := flag.Bool("authorize-live", false, "explicit authorization for provider costs")
	fixtures := flag.String("fixtures", "docs/evaluations/semantic-quality/cases.v1.json", "frozen synthetic corpus")
	split := flag.String("split", "development", "development or heldout; all requires controlled mode")
	ids := flag.String("ids", "", "explicit comma-separated fixture IDs (overrides split)")
	limit := flag.Int("limit", 8, "maximum live workflows; 1 through 8")
	output := flag.String("output", "", "new report file; includes only synthetic generated content")
	openAIPath := flag.String("openai-key-file", "", "private assignment-format credential file")
	typeSafePath := flag.String("typesafe-key-file", "", "private assignment-format credential file")
	flag.Parse()
	if *mode != "controlled" && *mode != "live" {
		return fmt.Errorf("mode must be controlled or live")
	}
	if *output == "" {
		return fmt.Errorf("new --output path required")
	}
	if *mode == "live" && (!*live || *limit < 1 || *limit > 8 || (*split == "all" && *ids == "")) {
		return fmt.Errorf("live requires --authorize-live, a limit of 1 through 8 and explicit split or IDs")
	}
	if *split != "all" && *split != "development" && *split != "heldout" {
		return fmt.Errorf("unknown split")
	}
	raw, err := os.ReadFile(*fixtures)
	if err != nil {
		return err
	}
	var data corpus
	if err = json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if err = eval.Validate(data.Cases); err != nil {
		return err
	}
	wanted := map[string]bool{}
	if *ids != "" {
		for _, id := range strings.Split(*ids, ",") {
			wanted[strings.TrimSpace(id)] = true
		}
	}
	selected := []eval.Case{}
	for _, c := range data.Cases {
		if (len(wanted) > 0 && wanted[c.ID]) || (len(wanted) == 0 && (*split == "all" || c.Split == *split)) {
			selected = append(selected, c)
		}
	}
	if len(selected) == 0 || (len(wanted) > 0 && len(selected) != len(wanted)) {
		return fmt.Errorf("empty selection or unknown fixture ID")
	}
	if *mode == "live" && len(selected) > *limit {
		return fmt.Errorf("selection exceeds authorized live limit")
	}
	// Resolve output and credentials before any provider request. Never overwrite evidence.
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("output must be new and writable")
	}
	defer file.Close()
	var openAIKey, typeSafeKey string
	if *mode == "live" {
		if openAIKey, err = credential(*openAIPath, "OPENAI_API_KEY"); err != nil {
			return err
		}
		if typeSafeKey, err = credential(*typeSafePath, "TYPESAFE_API_KEY"); err != nil {
			return err
		}
		if openAIKey == "" || typeSafeKey == "" {
			return fmt.Errorf("both credential files required for live baseline")
		}
	}
	commit, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	r := report{Version: "semantic-quality-report-v1", Mode: *mode, Started: time.Now().UTC().Format(time.RFC3339), Commit: strings.TrimSpace(string(commit)), FixtureVersion: data.Version, FixtureHash: eval.Hash(raw), SourceHashes: map[string]string{}, Scope: "local production HTTP handlers; synthetic inputs; no browser, deployment or universal factual guarantee", Runs: []eval.RunResult{}}
	for _, path := range []string{"backend/profile-ingestion/main.go", "backend/qualification-gaps/main.go", "backend/cv-generation/main.go", "backend/application-draft/main.go", "internal/fieldvalidation/decision.go", "internal/fieldvalidation/questions.json", "internal/semanticeval/score.go", "internal/semanticeval/adapter.go"} {
		source, e := os.ReadFile(filepath.Clean(path))
		if e != nil {
			return e
		}
		r.SourceHashes[path] = eval.Hash(source)
	}
	save := func() error {
		if _, e := file.Seek(0, 0); e != nil {
			return e
		}
		if e := file.Truncate(0); e != nil {
			return e
		}
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		return encoder.Encode(r)
	}
	if err = save(); err != nil {
		return err
	}
	failed := 0
	for _, c := range selected {
		result, e := eval.Run(c, *mode, openAIKey, typeSafeKey)
		if e != nil {
			return fmt.Errorf("fixture %s failed to evaluate: %w", c.ID, e)
		}
		r.Runs = append(r.Runs, result)
		if err = save(); err != nil {
			return err
		}
		fmt.Printf("%s %s HTTP=%d precision=%.3f completeness=%.3f forbidden=%d failures=%d\n", *mode, c.ID, result.Status, result.Score.Precision, result.Score.Completeness, result.Score.ForbiddenAdditions, len(result.Score.Failures))
		if len(result.Score.Failures) > 0 {
			failed++
		}
		if *mode == "live" && result.Status != 200 {
			return fmt.Errorf("live run stopped on workflow failure; partial evidence saved")
		}
	}
	fmt.Printf("Saved %d cases; %d have label or contract failures. Human prose review remains separate.\n", len(r.Runs), failed)
	return nil // Baselines record quality failures; they are not CI pass claims.
}
