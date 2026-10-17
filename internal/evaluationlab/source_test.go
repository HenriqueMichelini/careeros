package evaluationlab_test

import (
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestSourceSnapshotIncludesEmbeddedRubricAndRunnerCode(t *testing.T) {
	provenance, err := lab.CaptureSource("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"internal/fieldvalidation/questions.json", "internal/semanticeval/context.go", "internal/semanticeval/batch.go", "internal/evaluationlab/runner.go", "lab/api/main.go", "lab/process/probes.mjs", "lab/process/browser-diagnostics.mjs", "lab/ui/App.tsx", "lab/ui/style.css", "vite.lab.config.ts", "scripts/lab.mjs"} {
		if provenance.SourceHashes[path] == "" {
			t.Fatalf("missing source dependency %s", path)
		}
	}
}
