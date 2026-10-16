package evaluationlab_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	eval "professional-information-repo/internal/semanticeval"
	"testing"
)

func TestQualificationModulePreservesRetainedReportShape(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/qualification-evidence/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := lab.RunQualificationEvidence(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Retrieval) != 39 || len(r.Final) != 13 {
		t.Fatalf("shape drift: retrieval=%d final=%d", len(r.Retrieval), len(r.Final))
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if len(object) != 5 || object["fixtureHash"] == nil || object["controlledFinalMatching"] == nil {
		t.Fatal("serializer contract changed")
	}
}

func TestApplicationModulePreservesRetrievalAndFieldReportShapes(t *testing.T) {
	read := func(path string) []byte {
		raw, err := os.ReadFile("../../docs/evaluations/" + path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	r, err := lab.RunApplicationContext(context.Background(), read("qualification-evidence/cases.json"), read("semantic-quality/cases.v2.json"), read("semantic-quality/controlled.v2.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Retrieval) != 13 || len(r.Fields) != 40 {
		t.Fatalf("shape drift: retrieval=%d fields=%d", len(r.Retrieval), len(r.Fields))
	}
}

func TestApplicationCancellationPreservesCompletedEvidence(t *testing.T) {
	read := func(path string) []byte {
		t.Helper()
		raw, err := os.ReadFile("../../docs/evaluations/" + path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for _, stopAfter := range []int{1, 14} {
		t.Run(fmt.Sprint(stopAfter), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			completed := 0
			ctx = eval.WithExecutionPolicy(ctx, eval.ExecutionPolicy{ObserveWorkflow: func(eval.Case, eval.RunResult) {
				completed++
				if completed == stopAfter {
					cancel()
				}
			}})
			report, err := lab.RunApplicationContext(ctx, read("qualification-evidence/cases.json"), read("semantic-quality/cases.v2.json"), read("semantic-quality/controlled.v2.json"), nil)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation, got %v", err)
			}
			wantRows, wantFields := 1, 0
			if stopAfter == 14 {
				wantRows, wantFields = 13, 4
			}
			if len(report.Retrieval) != wantRows || len(report.Fields) != wantFields || len(report.FixtureHashes) != 3 {
				t.Fatalf("lost partial evidence: rows=%d fields=%d hashes=%d", len(report.Retrieval), len(report.Fields), len(report.FixtureHashes))
			}
		})
	}
}
