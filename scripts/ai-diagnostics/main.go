// Offline aggregation or controlled production-handler evidence; no live mode.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	d "professional-information-repo/internal/aidiagnostics"
	eval "professional-information-repo/internal/semanticeval"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("input", "", "JSON array of content-free attempts; offline only")
	controlled := flag.Bool("controlled", false, "use frozen synthetic fixtures and controlled transports; no network")
	fixtures := flag.String("fixtures", "docs/evaluations/semantic-quality/cases.v2.json", "synthetic corpus for controlled mode")
	pricing := flag.String("prices", "", "optional exact-model dated USD-per-million price assumptions")
	flag.Parse()
	if (*input == "") == (!*controlled) {
		return fmt.Errorf("select exactly one of --input or --controlled")
	}
	var attempts []d.Attempt
	if *controlled {
		raw, err := os.ReadFile(*fixtures)
		if err != nil {
			return err
		}
		var corpus struct {
			Cases []eval.Case `json:"cases"`
		}
		if err = json.Unmarshal(raw, &corpus); err != nil {
			return err
		}
		for _, c := range corpus.Cases {
			result, err := eval.Run(c, "controlled", "", "")
			if err != nil {
				return err
			}
			if result.Diagnostics == nil {
				return fmt.Errorf("missing diagnostics")
			}
			attempts = append(attempts, *result.Diagnostics)
		}
	} else {
		raw, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &attempts); err != nil {
			return err
		}
	}
	var prices []d.Price
	if *pricing != "" {
		raw, err := os.ReadFile(*pricing)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &prices); err != nil {
			return err
		}
	}
	summary, err := d.Summarize(attempts, prices)
	if err != nil {
		return err
	}
	mode := "offline"
	if *controlled {
		mode = "controlled: no live resource or quality claim"
	}
	output := struct {
		Mode     string      `json:"mode"`
		Summary  d.Summary   `json:"summary"`
		Attempts []d.Attempt `json:"attempts"`
	}{mode, summary, attempts}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
