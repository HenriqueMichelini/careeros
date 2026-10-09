package preprocessing_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"professional-information-repo/internal/fieldvalidation"
	p "professional-information-repo/internal/preprocessing"
)

// These are preservation fixtures, not classifier or semantic-quality labels.
func TestMilestonePreservationGolden(t *testing.T) {
	var fixture struct {
		Version string
		Cases   []struct{ ID, Language, Original, Prepared string }
	}
	data, err := os.ReadFile("testdata/preservation.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "preprocessing-preservation-v1" || len(fixture.Cases) == 0 {
		t.Fatal("missing fixture version/cases")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			for _, field := range []fieldvalidation.Field{fieldvalidation.ProfessionalInformation, fieldvalidation.JobPosting} {
				source, err := p.Prepare(tc.Original, field)
				if err != nil {
					t.Fatal(err)
				}
				if source.Text() != tc.Prepared {
					t.Fatalf("%s: got %q want %q", tc.Language, source.Text(), tc.Prepared)
				}
				checkPrepared(t, source)
				plan, err := source.Plan(p.Budget{p.MaxPreparedWorkflowBytes, p.MaxWorkflowPayloadBytes, 1}, payload)
				if err != nil || plan.Status != p.Ready {
					t.Fatalf("golden capacity: %+v %v", plan, err)
				}
				assertCoverage(t, source, plan)
			}
		})
	}
}

// Reuse existing development AND held-out inputs without querying providers,
// changing their labels, or treating preservation as semantic correctness.
func TestExistingEvaluationSourcesPreserved(t *testing.T) {
	files := []string{"field-validation-cases.json", "field-validation-followup-cases.json", "field-validation-confirmatory-cases.json", "field-validation-workflow-cases.json", "semantic-quality/cases.v2.json"}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile("../../docs/evaluations/" + file)
			if err != nil {
				t.Fatal(err)
			}
			type fixture struct {
				ID, Text, Field, Task string
				Request               struct{ Input, JobPosting string }
			}
			var cases []fixture
			if strings.HasPrefix(file, "semantic-quality/") {
				var wrapped struct {
					Version string
					Cases   []fixture
				}
				if err = json.Unmarshal(data, &wrapped); err != nil {
					t.Fatal(err)
				}
				if wrapped.Version != "semantic-quality-v2" {
					t.Fatal("unexpected semantic fixture version")
				}
				cases = wrapped.Cases
			} else if err = json.Unmarshal(data, &cases); err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, tc := range cases {
				raw := tc.Text
				if raw == "" {
					raw = tc.Request.Input
				}
				if raw == "" {
					raw = tc.Request.JobPosting
				}
				if raw == "" {
					continue
				}
				field := fieldvalidation.ProfessionalInformation
				if tc.Field == "job_posting" || tc.Request.JobPosting != "" {
					field = fieldvalidation.JobPosting
				}
				source, err := p.Prepare(raw, field)
				if err != nil {
					t.Fatalf("%s: %v", tc.ID, err)
				}
				checkPrepared(t, source)
				count++
			}
			if count == 0 {
				t.Fatal("no sources exercised")
			}
			t.Logf("preserved %d existing sources", count)
		})
	}
}

func FuzzPreparedRangeRequests(f *testing.F) {
	for _, raw := range []string{"Joa\u0303o\r\nI  use Java", "# Role\n- C++\n  - não C# 👩‍💻", "https://example.test/e\u0301 `x\r\n`"} {
		f.Add(raw, -1, 4)
	}
	f.Fuzz(func(t *testing.T, raw string, start, end int) {
		if len(raw) > 2048 || !utf8.ValidString(raw) {
			t.Skip()
		}
		source, err := p.Prepare(raw, fieldvalidation.JobPosting)
		if err != nil {
			t.Fatal(err)
		}
		ranges, err := source.Resolve(p.Range{Start: start, End: end})
		if err != nil {
			return
		}
		if start < 0 || end <= start || end > len(source.Text()) || !utf8.ValidString(source.Text()[start:end]) {
			t.Fatal("invalid range accepted")
		}
		again, err := source.Resolve(p.Range{Start: start, End: end})
		if err != nil || !reflect.DeepEqual(ranges, again) {
			t.Fatal("unstable recovery")
		}
		for _, r := range ranges {
			if r.Start < 0 || r.End <= r.Start || r.End > len(raw) || !utf8.ValidString(raw[r.Start:r.End]) {
				t.Fatal("invalid original range")
			}
		}
	})
}

func BenchmarkWorkflowPreparation(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{{"small", 120}, {"typical", 6000}, {"maximum", 30000}, {"maximum-job", 30720}} {
		// Synthetic prose/list corpus, padded to the exact admitted size.
		line := "# Role\n- Café uses C++ and Java; not C#.\n"
		raw := strings.Repeat(line, size.bytes/len(line)) + strings.Repeat("x", size.bytes%len(line))
		b.Run(size.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for i := 0; i < b.N; i++ {
				field := fieldvalidation.ProfessionalInformation
				if size.bytes == 30720 {
					field = fieldvalidation.JobPosting
				}
				prepared, err := p.PrepareBounded(raw, field)
				if err != nil || prepared.Status != p.Ready {
					b.Fatal(err)
				}
				plan, err := prepared.Source.Plan(p.Budget{p.MaxPreparedWorkflowBytes, p.MaxWorkflowPayloadBytes, 1}, payload)
				if err != nil || plan.Status != p.Ready {
					b.Fatal("plan", err)
				}
				b.ReportMetric(float64(len(prepared.Source.Repetitions())), "repeat-groups/op")
				b.ReportMetric(float64(len(prepared.Source.Segments())), "segments/op")
				b.ReportMetric(float64(len(plan.Portions)), "portions/op")
			}
		})
	}
}
