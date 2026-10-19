package profileingestion

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"professional-information-repo/internal/preprocessing"
)

// protocolFixture is the shared normative fixture schema read by both the Go and
// TypeScript contract tests. Fixtures set the expected outcome independently of
// any handler output.
type protocolFixture struct {
	Name       string              `json:"name"`
	Check      string              `json:"check"`
	Stage      SubmissionStage     `json:"stage"`
	Bytes      int                 `json:"bytes"`
	Status     string              `json:"status"`
	Batches    int                 `json:"batches"`
	Claims     int                 `json:"claims"`
	Operations int                 `json:"operations"`
	Value      json.RawMessage     `json:"value"`
	Allowed    []string            `json:"allowed"`
	Left       json.RawMessage     `json:"left"`
	Right      json.RawMessage     `json:"right"`
	Original   string              `json:"original"`
	Manifest   SubmissionPlan      `json:"manifest"`
	Aggregate  SubmissionAggregate `json:"aggregate"`
	Focus      SubmissionFocus     `json:"focus"`
	Result     SubmissionResult    `json:"result"`
	Selection  SubmissionSelection `json:"selection"`
	Session    SubmissionSession   `json:"session"`
	Expect     struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	} `json:"expect"`
}

func rawBinding(raw json.RawMessage) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "decode_error"
	}
	return canonicalBinding(value)
}

func protocolFixtureResult(f protocolFixture) (bool, string) {
	var code string
	switch f.Check {
	case "stage":
		code = SubmissionStageError(f.Stage)
	case "portion":
		code = PortionBudgetError(f.Bytes)
	case "status":
		code = SubmissionStatusError(f.Status)
	case "selection":
		code = SubmissionSelectionError(f.Selection.Status)
	case "counts":
		code = SubmissionCountsError(f.Batches, f.Claims, f.Operations)
	case "strict":
		code = SubmissionFieldsError(f.Value, f.Allowed...)
	case "binding":
		if rawBinding(f.Left) != rawBinding(f.Right) {
			code = SubmissionErrStale
		}
	case "manifest":
		code = SubmissionManifestError(f.Manifest, f.Original)
	case "aggregate":
		code = SubmissionAggregateError(f.Aggregate, f.Manifest)
	case "focus":
		code = SubmissionFocusError(f.Focus, f.Aggregate)
	case "result":
		code = SubmissionResultError(f.Result)
	case "session":
		code = SubmissionSessionError(f.Session)
	default:
		return false, "unknown_check"
	}
	return code == "", code
}

func TestSubmissionProtocolFixtures(t *testing.T) {
	files, err := filepath.Glob("testdata/submission-protocol/*.json")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no submission protocol fixtures found")
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var fixture protocolFixture
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&fixture); err != nil {
			t.Fatalf("decode %s: %v", file, err)
		}
		ok, code := protocolFixtureResult(fixture)
		if ok != fixture.Expect.OK || code != fixture.Expect.Error {
			t.Errorf("%s: got ok=%v code=%q want ok=%v code=%q", fixture.Name, ok, code, fixture.Expect.OK, fixture.Expect.Error)
		}
	}
}

func TestSubmissionBindingIgnoresObjectKeyOrder(t *testing.T) {
	left := map[string]any{"a": 1, "b": map[string]any{"c": 2, "d": 3}, "e": []any{1, 2}}
	right := map[string]any{"e": []any{1, 2}, "b": map[string]any{"d": 3, "c": 2}, "a": 1}
	if canonicalBinding(left) != canonicalBinding(right) {
		t.Fatal("object key order changed the binding")
	}
	if canonicalBinding(left) == canonicalBinding(map[string]any{"a": 1}) {
		t.Fatal("distinct values produced the same binding")
	}
}

// The whole-submission aggregate legitimately exceeds the per-batch 30-claim
// cap; only each bounded batch is capped.
func TestSubmissionAggregateAllowsManyBatches(t *testing.T) {
	plan := SubmissionPlan{
		ProtocolVersion: SubmissionProtocolVersion,
		SourceID:        "src", ProfileID: "prof", RunID: "run",
		PlanVersion: preprocessing.PlanningVersion, Status: string(preprocessing.Ready),
		Complete: true, OriginalBytes: 10, PreparedBytes: 10,
		Batches: []SubmissionBatch{
			{Index: 0, Primary: []preprocessing.Range{{Start: 0, End: 5}}, PreparedBytes: 5, PayloadBytes: 40},
			{Index: 1, Primary: []preprocessing.Range{{Start: 5, End: 10}}, PreparedBytes: 5, PayloadBytes: 40},
		},
	}
	batch := func(index, count, skipped int) SubmissionAggregateBatch {
		ids := make([]string, 0, count)
		for i := 0; i < count; i++ {
			ids = append(ids, GlobalClaimID("run", index, "c"+string(rune('a'+i))))
		}
		return SubmissionAggregateBatch{Index: index, Primary: plan.Batches[index].Primary, ClaimIDs: ids, ClaimCount: count, SkippedCount: skipped}
	}
	aggregate := SubmissionAggregate{
		ProtocolVersion: SubmissionProtocolVersion,
		SourceID:        "src", ProfileID: "prof", RunID: "run", PlanVersion: preprocessing.PlanningVersion,
		Batches:  []SubmissionAggregateBatch{batch(0, 20, 1), batch(1, 20, 0)},
		Coverage: SubmissionCoverage{ValidClaims: 40, InvalidClaims: 1, Capacity: SubmissionCapacityExhausted},
	}
	if code := SubmissionAggregateError(aggregate, plan); code != "" {
		t.Fatalf("aggregate of 40 claims rejected: %q", code)
	}
	if code := SubmissionManifestError(plan, "abcdefghij"); code != "" {
		t.Fatalf("valid plan rejected: %q", code)
	}
}

func TestSubmissionErrorStatusMapping(t *testing.T) {
	cases := map[string]int{
		SubmissionErrVersion:  400,
		SubmissionErrStage:    400,
		SubmissionErrState:    400,
		SubmissionErrManifest: 400,
		SubmissionErrBatch:    400,
		SubmissionErrContext:  400,
		SubmissionErrCoverage: 400,
		SubmissionErrFocus:    400,
		SubmissionErrUnicode:  400,
		SubmissionErrCapacity: 400,
		SubmissionErrStale:    409,
		"configuration":       503,
		"timeout":             504,
		"key":                 401,
	}
	for code, want := range cases {
		if got := SubmissionErrorStatus(code); got != want {
			t.Errorf("SubmissionErrorStatus(%q) = %d, want %d", code, got, want)
		}
	}
}
