package preprocessing_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"professional-information-repo/internal/fieldvalidation"
	p "professional-information-repo/internal/preprocessing"
)

func TestBoundedPreparationChecksOriginalBeforeCleanup(t *testing.T) {
	for _, tc := range []struct {
		field fieldvalidation.Field
		limit int
	}{
		{fieldvalidation.ProfessionalInformation, 30000},
		{fieldvalidation.JobPosting, 30720},
	} {
		for _, extra := range []int{0, 1} {
			raw := "a" + strings.Repeat(" ", tc.limit-2+extra) + "b"
			r, err := p.PrepareBounded(raw, tc.field)
			if err != nil {
				t.Fatal(err)
			}
			if r.Original != raw || r.OriginalBytes != tc.limit+extra {
				t.Fatal("original lost")
			}
			if extra == 0 {
				if r.Status != p.Ready || r.Source.Text() != "a b" || r.PreparedBytes != 3 {
					t.Fatalf("admitted source: %+v", r)
				}
			} else if r.Status != p.InsufficientCapacity || len(r.Unprocessed) != 1 || r.Unprocessed[0] != (p.Range{0, tc.limit + 1}) || r.Source.ID() != "" {
				t.Fatal("oversize source was prepared or lost its full range")
			}
		}
	}
}

func TestPlanningByteBoundariesExpansionAndNormalizationGroups(t *testing.T) {
	for _, field := range []fieldvalidation.Field{fieldvalidation.ProfessionalInformation, fieldvalidation.JobPosting} {
		limits, _ := p.LimitsFor(field)
		wantBody := 160 << 10
		if field == fieldvalidation.JobPosting {
			wantBody = 128 << 10
		}
		if limits.RequestBodyBytes != wantBody {
			t.Fatal("changed HTTP body limit")
		}
		for _, extra := range []int{0, 1} {
			// An emoji crosses the limit at its final UTF-8 byte.
			raw := strings.Repeat("x", limits.OriginalBytes-4+extra) + "😀"
			r, err := p.PrepareBounded(raw, field)
			if err != nil || (r.Status == p.Ready) != (extra == 0) || r.Original != raw {
				t.Fatal("multibyte raw boundary")
			}
		}
	}
	// NFC expands U+0344 from two bytes to four. These four bytes form one
	// indivisible normalization group and must not become two two-byte chunks.
	r, err := p.PrepareBounded("\u0344", fieldvalidation.JobPosting)
	if err != nil || r.OriginalBytes != 2 || r.PreparedBytes != 4 || r.Source.Text() != "\u0308\u0301" {
		t.Fatal("NFC expansion")
	}
	plan, err := r.Source.Plan(p.Budget{3, 10000, 10}, payload)
	if err != nil || plan.Complete || len(plan.Portions) != 0 || !reflect.DeepEqual(plan.Unprocessed, []p.Range{{0, 2}}) {
		t.Fatalf("split transformed group: %+v %v", plan, err)
	}
	r, _ = p.PrepareBounded(strings.Repeat("\u0344", 15000), fieldvalidation.ProfessionalInformation)
	if r.OriginalBytes != 30000 || r.PreparedBytes != 60000 {
		t.Fatal("expanded source sizes")
	}
	plan, err = r.Source.Plan(p.Budget{30000, 200000, 3}, payload)
	if err != nil || plan.Complete || len(plan.Portions) != 0 || plan.Status != p.InsufficientCapacity {
		t.Fatalf("expanded plan: %+v %v", plan, err)
	}
	assertCoverage(t, r.Source, plan)
	r, _ = p.PrepareBounded(strings.Repeat("\u0344x", 10000), fieldvalidation.ProfessionalInformation)
	plan, err = r.Source.Plan(p.Budget{30000, 200000, 3}, payload)
	if err != nil || !plan.Complete || r.PreparedBytes != 50000 {
		t.Fatal("separate expanded groups should fit two portions")
	}
	assertCoverage(t, r.Source, plan)
}

func TestPlanPreservesProtectedValuesAndEveryRepeatedOccurrence(t *testing.T) {
	for _, raw := range []string{
		"João 😀 e\u0301\r\n" + strings.Repeat("repeat\n\n", 30),
		"# A\n- parent\n  detail\n  detail\n\n# B\nrepeat\nrepeat\n",
		"prefix `a\nb\nc` suffix\n\nhttps://example.test/long\n",
		"quote\n\n> long protected quotation\n> more\n",
		"a | b\nc | d\n\nend\n",
	} {
		s, _ := p.Prepare(raw, fieldvalidation.ProfessionalInformation)
		before := s.Repetitions()
		r, err := s.Plan(p.Budget{18, 10000, 100}, payload)
		if err != nil {
			t.Fatal(err)
		}
		assertCoverage(t, s, r)
		if !reflect.DeepEqual(before, s.Repetitions()) || s.Original() != raw {
			t.Fatal("planning changed annotations/source")
		}
		guards := []string{"`a\nb\nc`", "https://example.test/long", "> long protected quotation\n> more\n", "a | b\nc | d\n"}
		for _, guard := range guards {
			start := strings.Index(s.Text(), guard)
			if start < 0 {
				continue
			}
			for _, part := range r.Portions {
				end := part.View.Normalized.End
				if end > start && end < start+len(guard) {
					t.Fatalf("bisected protected block %q", guard)
				}
			}
		}
	}
}

func TestPlanReportsWrapperCapacityAndSerializerFailure(t *testing.T) {
	s, _ := p.Prepare("# long heading\nx\n", fieldvalidation.JobPosting)
	r, err := s.Plan(p.Budget{15, 10000, 10}, payload)
	if err != nil || r.Complete || len(r.Portions) != 1 || r.Reason != p.IndivisibleCapacity {
		t.Fatalf("wrapper failure: %+v %v", r, err)
	}
	assertCoverage(t, s, r)
	failure := errors.New("serialize failed")
	_, err = s.Plan(p.Budget{100, 10000, 10}, func(p.View) ([]byte, error) { return nil, failure })
	if !errors.Is(err, failure) {
		t.Fatal("serializer failure swallowed")
	}
	for _, b := range []p.Budget{{0, 1, 1}, {1, 0, 1}, {1, 1, 0}} {
		if _, err = s.Plan(b, payload); err == nil {
			t.Fatal("invalid budget accepted")
		}
	}
	var zero p.Source
	if _, err = zero.Plan(p.Budget{1, 1, 1}, payload); err == nil {
		t.Fatal("invalid source accepted")
	}
}

func TestManySmallSegmentsHaveBoundedExactCoverage(t *testing.T) {
	s, _ := p.Prepare(strings.Repeat("a\n", 15000), fieldvalidation.ProfessionalInformation)
	r, err := s.Plan(p.Budget{1000, 10000, 30}, payload)
	if err != nil || !r.Complete || len(r.Portions) != 30 {
		t.Fatalf("many segments: complete=%v count=%d err=%v", r.Complete, len(r.Portions), err)
	}
	assertCoverage(t, s, r)
}

func assertCoverage(t *testing.T, s p.Source, plan p.Plan) {
	t.Helper()
	end := 0
	for _, part := range plan.Portions {
		if !utf8.ValidString(part.View.Text) {
			t.Fatal("split UTF-8")
		}
		for _, r := range part.View.Original {
			if r.Start != end || r.End <= r.Start {
				t.Fatalf("coverage gap/overlap at %d: %+v", end, r)
			}
			end = r.End
		}
		resolved, err := s.Resolve(part.View.Normalized)
		if err != nil || !reflect.DeepEqual(resolved, part.View.Original) {
			t.Fatal("split normalization group or incorrect source range")
		}
	}
	for _, r := range plan.Unprocessed {
		if r.Start != end {
			t.Fatal("unprocessed coverage gap")
		}
		end = r.End
	}
	if end != len(s.Original()) {
		t.Fatalf("covered %d of %d bytes", end, len(s.Original()))
	}
}

func TestPlanRetainsParagraphsAndLabelsRepeatedHeadingContext(t *testing.T) {
	s, _ := p.Prepare("# H\none\ntwo\n\nthree\nfour\n", fieldvalidation.ProfessionalInformation)
	b := p.Budget{PreparedBytes: 15, PayloadBytes: 10000, MaxPortions: 10}
	r, err := s.Plan(b, payload)
	if err != nil || !r.Complete || r.Status != p.InsufficientCapacity {
		t.Fatalf("plan: %+v %v", r, err)
	}
	if len(r.Portions) != 2 || r.Portions[0].View.Text != "# H\none\ntwo\n\n" || r.Portions[1].View.Text != "three\nfour\n" {
		t.Fatalf("paragraph boundaries: %+v", r.Portions)
	}
	c := r.Portions[1].View.Context
	if len(c) != 1 || c[0].Role != "context" || c[0].Text != "# H\n" || c[0].Original[0] != (p.Range{0, 4}) {
		t.Fatalf("context: %+v", c)
	}
	if r.Portions[1].PreparedBytes != 15 {
		t.Fatal("context omitted from byte budget")
	}
	assertCoverage(t, s, r)
	again, _ := s.Plan(b, payload)
	if !reflect.DeepEqual(r, again) {
		t.Fatal("nondeterministic plan")
	}
}

func TestOversizedProtectedBlockAndPortionCountReturnUnprocessedRanges(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		budget p.Budget
		reason p.CapacityReason
	}{
		{"ok\n\n```\nlong protected code\n```\n", p.Budget{10, 10000, 100}, p.IndivisibleCapacity},
		{strings.Repeat("a\n\n", 20), p.Budget{3, 10000, 2}, p.PortionCountCapacity},
	} {
		s, _ := p.Prepare(tc.raw, fieldvalidation.JobPosting)
		r, err := s.Plan(tc.budget, payload)
		if err != nil || r.Complete || r.Reason != tc.reason || len(r.Unprocessed) != 1 {
			t.Fatalf("failure: %+v %v", r, err)
		}
		if s.Original() != tc.raw {
			t.Fatal("lost original")
		}
		assertCoverage(t, s, r)
	}
}

func payload(v p.View) ([]byte, error) {
	return json.Marshal(struct {
		Prompt string
		Input  p.View
	}{"fixed prompt and caller context", v})
}

func TestPlanChecksPreparedAndSerializedCapacitySeparately(t *testing.T) {
	s, _ := p.Prepare("a\"b", fieldvalidation.ProfessionalInformation)
	wide := p.Budget{PreparedBytes: 3, PayloadBytes: 10000, MaxPortions: 10}
	full, err := s.Plan(wide, payload)
	if err != nil || full.Status != p.Ready || !full.Complete || len(full.Portions) != 1 {
		t.Fatalf("single view: %+v %v", full, err)
	}
	if full.Portions[0].PreparedBytes != 3 || full.SinglePayloadBytes <= 3 {
		t.Fatal("JSON/envelope bytes were not measured")
	}
	wide.PayloadBytes = full.SinglePayloadBytes - 1
	bounded, err := s.Plan(wide, payload)
	if err != nil || bounded.Status != p.InsufficientCapacity {
		t.Fatalf("payload overrun: %+v %v", bounded, err)
	}
	for _, part := range bounded.Portions {
		if part.PayloadBytes > wide.PayloadBytes {
			t.Fatal("serialized payload exceeds budget")
		}
	}
	wide.PayloadBytes = full.SinglePayloadBytes
	exact, err := s.Plan(wide, payload)
	if err != nil || exact.Status != p.Ready {
		t.Fatal("exact serialized boundary rejected")
	}
}

func TestTransportEnvelopeBudgetIsIndependentOfDecodedSource(t *testing.T) {
	for _, field := range []fieldvalidation.Field{fieldvalidation.ProfessionalInformation, fieldvalidation.JobPosting} {
		limits, _ := p.LimitsFor(field)
		for _, extra := range []int{0, 1} {
			// A small valid source does not exempt large surrounding JSON fields.
			body, err := json.Marshal(struct{ Input, Context string }{"Java", strings.Repeat("x", limits.RequestBodyBytes-29+extra)})
			if err != nil {
				t.Fatal(err)
			}
			// The literal envelope is 29 bytes, including the four-byte source.
			if len(body) != limits.RequestBodyBytes+extra {
				t.Fatalf("fixture envelope: %d", len(body))
			}
			status, err := p.CheckRequestBodySize(field, len(body))
			if err != nil || (status == p.Ready) != (extra == 0) {
				t.Fatal("transport boundary")
			}
		}
	}
	if _, err := p.CheckRequestBodySize(fieldvalidation.JobPosting, -1); err == nil {
		t.Fatal("negative body size")
	}
}

func FuzzBoundedPlanCoverage(f *testing.F) {
	for _, seed := range []string{"a\nb\n", "# A\n- parent\n  child\n", "😀 e\u0301\r\n\u0344", "```\ncode\n```\n", "`a\nb` end\n", ""} {
		f.Add(seed, uint16(12))
	}
	f.Fuzz(func(t *testing.T, raw string, size uint16) {
		if len(raw) > 1024 || !utf8.ValidString(raw) {
			t.Skip()
		}
		prepared, err := p.PrepareBounded(raw, fieldvalidation.JobPosting)
		if err != nil {
			t.Fatal(err)
		}
		b := p.Budget{int(size%128) + 1, 4096, 30}
		plan, err := prepared.Source.Plan(b, payload)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "" {
			assertCoverage(t, prepared.Source, plan)
		}
		for _, part := range plan.Portions {
			if part.PreparedBytes > b.PreparedBytes || part.PayloadBytes > b.PayloadBytes {
				t.Fatal("budget exceeded")
			}
		}
		again, err := prepared.Source.Plan(b, payload)
		if err != nil || !reflect.DeepEqual(plan, again) {
			t.Fatal("nondeterministic plan")
		}
	})
}
