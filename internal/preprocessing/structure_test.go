package preprocessing_test

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"professional-information-repo/internal/fieldvalidation"
	p "professional-information-repo/internal/preprocessing"
)

func TestStructuralGoldenCases(t *testing.T) {
	cases := []struct {
		name, input, want string
		kinds             []p.Kind
		parents           []int // -1 means no parent
	}{
		{"mixed lists", "# Work\r\n- Java  developer\r\n  1. AWS\r\n     wrapped\r\n  continuation\r\n\r\n- Go\r\n", "# Work\n- Java developer\n  1. AWS\n     wrapped\n  continuation\n\n- Go\n", []p.Kind{p.Heading, p.ListItem, p.ListItem, p.ListContinuation, p.ListContinuation, p.Blank, p.ListItem}, []int{-1, 0, 1, 2, 1, 1, 0}},
		{"headings", "# Skills\n## Skills\nplain\n# Skills\nALL CAPITALS\n", "# Skills\n## Skills\nplain\n# Skills\nALL CAPITALS\n", []p.Kind{p.Heading, p.Heading, p.Text, p.Heading, p.Text}, []int{-1, 0, 1, -1, 3}},
		{"protected", "> quote  aligned\n\n    literal  value\n```txt\r\nx  y\r\n```\r\n| A | B |\n---\n", "> quote  aligned\n\n    literal  value\n```txt\r\nx  y\r\n```\r\n| A | B |\n---\n", []p.Kind{p.Quotation, p.Blank, p.Literal, p.Literal, p.Literal, p.Literal, p.Table, p.Separator}, []int{-1, -1, -1, -1, -1, -1, -1, -1}},
		{"spacing", "I\u00a0use   Java.  \r\nUso\u00a0 Java e `C  C++` hoje.\nA  B  C\nA\tB\n", "I use Java.  \nUso Java e `C  C++` hoje.\nA  B  C\nA\tB\n", []p.Kind{p.Text, p.Text, p.Table, p.Table}, []int{-1, -1, -1, -1}},
		{"English", "Java developer. AWS required.", "Java developer. AWS required.", []p.Kind{p.Text}, []int{-1}},
		{"Portuguese", "Desenvolvedor Java. AWS obrigatório.", "Desenvolvedor Java. AWS obrigatório.", []p.Kind{p.Text}, []int{-1}},
		{"unknown", "EXPERIENCE\nStrange:: ???\n==\n", "EXPERIENCE\nStrange:: ???\n==\n", []p.Kind{p.Text, p.Text, p.Text}, []int{-1, -1, -1}},
		{"tabs and empty", "\tcode\r\n\u00a0\r\n", "\tcode\r\n\u00a0\n", []p.Kind{p.Literal, p.Blank}, []int{-1, -1}},
		{"literal Unicode", "    Joa\u0303o  x\r\n| e\u0301 | x |\r\n", "    Joa\u0303o  x\r\n| e\u0301 | x |\r\n", []p.Kind{p.Literal, p.Table}, []int{-1, -1}},
		{"list with literal", "- Java\n  ```\n  x  y\n  ```\n", "- Java\n  ```\n  x  y\n  ```\n", []p.Kind{p.ListItem, p.Literal, p.Literal, p.Literal}, []int{-1, 0, 0, 0}},
		{"empty", "", "", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := p.Prepare(tc.input, fieldvalidation.ProfessionalInformation)
			if err != nil {
				t.Fatal(err)
			}
			if source.Text() != tc.want {
				t.Fatalf("got %q want %q", source.Text(), tc.want)
			}
			segments := source.Segments()
			if len(segments) != len(tc.kinds) {
				t.Fatalf("segments: %+v", segments)
			}
			for i, seg := range segments {
				if seg.Kind != tc.kinds[i] {
					t.Errorf("segment %d: got %s want %s", i, seg.Kind, tc.kinds[i])
				}
				wantParent := ""
				if tc.parents[i] >= 0 {
					wantParent = segments[tc.parents[i]].ID
				}
				if seg.ParentID != wantParent {
					t.Errorf("segment %d parent %q want %q", i, seg.ParentID, wantParent)
				}
			}
			checkPrepared(t, source)
		})
	}
}

func checkPrepared(t *testing.T, source p.Source) {
	t.Helper()
	again, err := p.Prepare(source.Text(), source.Field())
	if err != nil || again.Text() != source.Text() {
		t.Fatalf("not idempotent: %q => %q (%v)", source.Text(), again.Text(), err)
	}
	repeat, err := p.Prepare(source.Original(), source.Field())
	if err != nil || !reflect.DeepEqual(repeat, source) {
		t.Fatal("not deterministic")
	}
	n, o := 0, 0
	ids := map[string]bool{}
	for _, seg := range source.Segments() {
		if seg.Normalized.Start != n || seg.Normalized.End <= n || ids[seg.ID] {
			t.Fatalf("incomplete/duplicate segments: %+v", seg)
		}
		if seg.ParentID != "" && !ids[seg.ParentID] {
			t.Fatal("parent must precede child")
		}
		ids[seg.ID] = true
		resolved, err := source.Resolve(seg.Normalized)
		if err != nil || !reflect.DeepEqual(resolved, seg.Original) {
			t.Fatal("source recovery failed")
		}
		for _, r := range seg.Original {
			if r.Start != o || r.End <= o {
				t.Fatal("original gap")
			}
			o = r.End
		}
		n = seg.Normalized.End
	}
	if n != len(source.Text()) || o != len(source.Original()) {
		t.Fatal("incomplete segment coverage")
	}
	n, o = 0, 0
	for _, m := range source.Mappings() {
		if m.Normalized.Start != n || m.Original.Start != o || m.Normalized.End <= n || m.Original.End <= o {
			t.Fatal("mapping gap")
		}
		if !utf8.ValidString(source.Text()[m.Normalized.Start:m.Normalized.End]) || !utf8.ValidString(source.Original()[m.Original.Start:m.Original.End]) {
			t.Fatal("split UTF-8")
		}
		n, o = m.Normalized.End, m.Original.End
	}
	if n != len(source.Text()) || o != len(source.Original()) {
		t.Fatal("incomplete mapping coverage")
	}
}

func TestPreparedExcerptAndDefensiveCopies(t *testing.T) {
	original := "Joa\u0303o\u00a0  Java\r\nJoa\u0303o\u00a0  Java"
	source, err := p.Prepare(original, fieldvalidation.JobPosting)
	if err != nil {
		t.Fatal(err)
	}
	if source.Text() != "João Java\nJoão Java" {
		t.Fatal(source.Text())
	}
	segments := source.Segments()
	if segments[0].ID == segments[1].ID {
		t.Fatal("repeated occurrences share ID")
	}
	r := segments[1].Original[0]
	if original[r.Start:r.End] != "Joa\u0303o\u00a0  Java" {
		t.Fatal("wrong repeated excerpt")
	}
	segments[0].Original[0].End = 999
	segments[0].ParentID = "mutated"
	if source.Segments()[0].Original[0].End == 999 || source.Segments()[0].ParentID != "" {
		t.Fatal("mutable source")
	}
	other, _ := p.Normalize(original, source.Field())
	if other.ID() == source.ID() || source.Version() != p.StructureVersion {
		t.Fatal("identity lacks structure version")
	}
	checkPrepared(t, source)
}

func TestPreparedCurrentByteLimits(t *testing.T) {
	for _, limit := range []int{30000, 30720} {
		source, err := p.Prepare(strings.Repeat("a  b\n", limit/5), fieldvalidation.JobPosting)
		if err != nil {
			t.Fatal(err)
		}
		checkPrepared(t, source)
	}
}

func FuzzPreparedCoverage(f *testing.F) {
	for _, seed := range []string{"", "- Java\n  - AWS\n    wrapped", "# Skills\r\nJoa\u0303o  Java", "```\r\na  b\r\n```", "a\u00a0b", "a\u0315\u0300\r\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if !utf8.ValidString(input) {
			return
		}
		source, err := p.Prepare(input, fieldvalidation.JobPosting)
		if err != nil {
			t.Fatal(err)
		}
		checkPrepared(t, source)
	})
}
