package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
)

func main() {
	input := flag.String("fixtures", "docs/evaluations/qualification-evidence/cases.json", "labeled synthetic corpus")
	output := flag.String("output", "/tmp/qualification-evidence-report.json", "report path")
	flag.Parse()
	raw, err := os.ReadFile(*input)
	must(err)
	report, err := lab.RunQualificationEvidence(context.Background(), raw, nil)
	must(err)
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
