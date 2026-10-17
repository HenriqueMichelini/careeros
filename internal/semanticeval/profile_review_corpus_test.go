package semanticeval_test

import (
	"encoding/json"
	"os"
	eval "professional-information-repo/internal/semanticeval"
	"strings"
	"testing"
)

func TestBilingualSectionReviewCorpusPreservesContractsAndPendingAdjudication(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/section-review/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Version string
		Cases   []eval.Case
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "section-review-fixtures-v1" || len(corpus.Cases) != 44 {
		t.Fatal("bounded bilingual inventory changed")
	}
	if err = eval.Validate(corpus.Cases); err != nil {
		t.Fatal(err)
	}
	languages := map[string]int{}
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			languages[c.Language]++
			result, err := eval.Run(c, "controlled", "", "")
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != c.ExpectedStatus || len(result.Calls) != c.ExpectedCalls || !result.NoStore {
				t.Fatalf("contract: status%d calls%d want%d/%d", result.Status, len(result.Calls), c.ExpectedStatus, c.ExpectedCalls)
			}
			if c.Adjudication != "pending" || !strings.HasPrefix(result.Score.HumanReview, "pending") {
				t.Fatal("agent fixture promoted semantic review")
			}
			allowed := func(failure string) bool {
				for _, prefix := range c.ExpectedFailures {
					if strings.HasPrefix(failure, prefix) {
						return true
					}
				}
				return false
			}
			for _, failure := range result.Score.Failures {
				if !allowed(failure) {
					t.Fatalf("unexpected label failure: %s", failure)
				}
			}
			for _, prefix := range c.ExpectedFailures {
				found := false
				for _, failure := range result.Score.Failures {
					found = found || strings.HasPrefix(failure, prefix)
				}
				if !found {
					t.Fatalf("negative control did not trigger%s", prefix)
				}
			}
		})
	}
	if languages["en"] != 22 || languages["pt-BR"] != 22 {
		t.Fatal("language opportunities lost")
	}
}

func TestSectionReviewLabelsRequireExactSourceSpansAndBoundedFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../docs/evaluations/section-review/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []eval.Case }
	json.Unmarshal(raw, &corpus)
	invalid := corpus.Cases[0]
	invalid.SourceSpans = append([]eval.SourceSpan{}, invalid.SourceSpans...)
	invalid.SourceSpans[0].EndByte++
	if err = eval.Validate([]eval.Case{invalid}); err == nil {
		t.Fatal("inexact source span accepted")
	}
	invalid = corpus.Cases[0]
	invalid.ProviderStatuses = []int{0}
	if err = eval.Validate([]eval.Case{invalid}); err == nil {
		t.Fatal("unbounded status fixture accepted")
	}
	invalid = corpus.Cases[0]
	invalid.Adjudication = "accepted-by-agent"
	if err = eval.Validate([]eval.Case{invalid}); err == nil {
		t.Fatal("agent promoted adjudication")
	}
}
