package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	output := flag.String("output", "/tmp/application-context-report.json", "controlled report")
	flag.Parse()
	labels, err := os.ReadFile("docs/evaluations/qualification-evidence/cases.json")
	must(err)
	cases, err := os.ReadFile("docs/evaluations/semantic-quality/cases.v2.json")
	must(err)
	baseline, err := os.ReadFile("docs/evaluations/semantic-quality/controlled.v2.json")
	must(err)
	report, err := lab.RunApplicationContext(context.Background(), labels, cases, baseline, nil)
	must(err)
	raw, err := json.MarshalIndent(report, "", "  ")
	must(err)
	must(os.WriteFile(*output, append(raw, '\n'), 0644))
	fmt.Println(*output)
}
