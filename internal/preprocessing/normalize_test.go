package preprocessing_test

import (
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/preprocessing"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizationRecoversOriginal(t *testing.T) {
	original := "Joa\u0303o\r\nJoa\u0303o"
	p, err := preprocessing.Normalize(original, fieldvalidation.ProfessionalInformation)
	if err != nil {
		t.Fatal(err)
	}
	if p.Original() != original || p.Text() != "João\nJoão" {
		t.Fatalf("unexpected source/text: %q / %q", p.Original(), p.Text())
	}
	ranges, err := p.Resolve(preprocessing.Range{Start: 0, End: 5})
	if err != nil || len(ranges) != 1 || original[ranges[0].Start:ranges[0].End] != "Joa\u0303o" {
		t.Fatalf("lost original: %v, %v", ranges, err)
	}
}

func TestProtectedLiterals(t *testing.T) {
	original := "Joa\u0303o\r\nhttps://EXAMPLE.com/Joa\u0303o?q=C++ JoA\u0303o@EXAMPLE.com\r\n`Joa\u0303o\r\nC#`\r\n```go\r\nJoa\u0303o\r\n```\r\nFim\r"
	want := "João\nhttps://EXAMPLE.com/Joa\u0303o?q=C++ JoA\u0303o@EXAMPLE.com\n`Joa\u0303o\r\nC#`\n```go\r\nJoa\u0303o\r\n```\r\nFim\n"
	p, err := preprocessing.Normalize(original, fieldvalidation.JobPosting)
	if err != nil {
		t.Fatal(err)
	}
	if p.Text() != want {
		t.Fatalf("literal changed:\ngot  %q\nwant %q", p.Text(), want)
	}
}

func TestGoldenPreservation(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"empty", "", ""},
		{"whitespace", " \t\r\n  \r\t\n", " \t\n  \n\t\n"},
		{"prose", "  - Joa\u0303o: C C++ C#; NÃO 2026-10-08 42.\r\n\r東京 👩‍💻 ❤️ ﬁ ①", "  - João: C C++ C#; NÃO 2026-10-08 42.\n\n東京 👩‍💻 ❤️ ﬁ ①"},
		{"reorder", "a\u0315\u0300", "à\u0315"},
		{"inline runs", "``a`e\u0301\r`` e\u0301", "``a`e\u0301\r`` é"},
		{"tilde fence", "  ~~~txt\ra\u0303\r  ~~~~\rde\u0301", "  ~~~txt\ra\u0303\r  ~~~~\rdé"},
		{"unclosed fence", "~~~\r\ne\u0301", "~~~\r\ne\u0301"},
		{"unclosed inline", "`e\u0301\r", "`e\u0301\r"},
		{"email and URL", "<Éx@EXAMPLE.COM> www.Example.com/e\u0301 mailto:Na\u0303o@Example.com e\u0301", "<Éx@EXAMPLE.COM> www.Example.com/e\u0301 mailto:Na\u0303o@Example.com é"},
		{"long combining sequence", "a" + strings.Repeat("\u0301", 35), "á" + strings.Repeat("\u0301", 34)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := preprocessing.Normalize(tc.input, fieldvalidation.JobPosting)
			if err != nil {
				t.Fatal(err)
			}
			if p.Text() != tc.want || p.Original() != tc.input {
				t.Fatalf("got %q, want %q", p.Text(), tc.want)
			}
			checkSource(t, p)
		})
	}
}

func checkSource(t *testing.T, p preprocessing.Source) {
	t.Helper()
	again, err := preprocessing.Normalize(p.Text(), p.Field())
	if err != nil || again.Text() != p.Text() {
		t.Fatalf("not idempotent: %q, %v", again.Text(), err)
	}
	repeat, err := preprocessing.Normalize(p.Original(), p.Field())
	if err != nil || !reflect.DeepEqual(repeat, p) {
		t.Fatal("not deterministic")
	}
	normalizedEnd, originalEnd := 0, 0
	for _, m := range p.Mappings() {
		if m.Normalized.Start != normalizedEnd || m.Original.Start != originalEnd || m.Normalized.End <= normalizedEnd || m.Original.End <= originalEnd || m.Normalized.End > len(p.Text()) || m.Original.End > len(p.Original()) {
			t.Fatalf("invalid coverage: %+v", m)
		}
		if !utf8.ValidString(p.Text()[m.Normalized.Start:m.Normalized.End]) || !utf8.ValidString(p.Original()[m.Original.Start:m.Original.End]) {
			t.Fatal("split UTF-8")
		}
		ranges, err := p.Resolve(m.Normalized)
		if err != nil || len(ranges) != 1 || ranges[0] != m.Original {
			t.Fatalf("mapping recovery failed: %v %v", ranges, err)
		}
		normalizedEnd, originalEnd = m.Normalized.End, m.Original.End
	}
	if normalizedEnd != len(p.Text()) || originalEnd != len(p.Original()) {
		t.Fatal("incomplete coverage")
	}
	if len(p.Text()) > 0 {
		ranges, err := p.Resolve(preprocessing.Range{Start: 0, End: len(p.Text())})
		if err != nil || len(ranges) != 1 || ranges[0] != (preprocessing.Range{Start: 0, End: len(p.Original())}) {
			t.Fatal("full excerpt lost")
		}
	}
}

func TestOccurrenceIdentityAndInvalidRanges(t *testing.T) {
	p, _ := preprocessing.Normalize("Joa\u0303o\r\nJoa\u0303o", fieldvalidation.ProfessionalInformation)
	first := preprocessing.Range{Start: 0, End: 5}
	second := preprocessing.Range{Start: 6, End: 11}
	a, _ := p.RangeID(first)
	b, _ := p.RangeID(second)
	if a == b || a == "" {
		t.Fatal("occurrences share identity")
	}
	ranges, err := p.Resolve(second)
	if err != nil || ranges[0] != (preprocessing.Range{Start: 8, End: 14}) {
		t.Fatalf("wrong repeated occurrence: %v %v", ranges, err)
	}
	for _, r := range []preprocessing.Range{{-1, 1}, {0, 0}, {3, 4}, {0, 99}, {5, 1}} {
		if _, err := p.Resolve(r); err == nil {
			t.Fatalf("accepted invalid range %v", r)
		}
	}
	p2, _ := preprocessing.Normalize("a\u0315\u0300", fieldvalidation.JobPosting)
	if _, err := p2.Resolve(preprocessing.Range{Start: 0, End: 2}); err == nil {
		t.Fatal("guessed partial reordered group")
	}
	other, _ := preprocessing.Normalize(p.Original(), fieldvalidation.JobPosting)
	canonical, _ := preprocessing.Normalize(p.Text(), p.Field())
	if other.ID() == p.ID() || canonical.ID() == p.ID() {
		t.Fatal("source identity ignores field or raw source")
	}
	mapping := p.Mappings()
	mapping[0].Original.End = 999
	if p.Mappings()[0].Original.End == 999 {
		t.Fatal("mutable mapping")
	}
	for _, input := range []string{string([]byte{0xff}), string([]byte{'a', 0xc3})} {
		if _, err := preprocessing.Normalize(input, p.Field()); err == nil {
			t.Fatal("accepted invalid UTF-8")
		}
	}
	if _, err := preprocessing.Normalize("hello", "unknown"); err == nil {
		t.Fatal("accepted unknown field")
	}
	if _, err := (preprocessing.Source{}).Resolve(first); err == nil {
		t.Fatal("accepted zero source")
	}
}

func TestCurrentByteLimits(t *testing.T) {
	for _, limit := range []int{30000, 30720} {
		input := strings.Repeat("é\r", limit/3)
		p, err := preprocessing.Normalize(input, fieldvalidation.ProfessionalInformation)
		if err != nil || len(p.Original()) != limit {
			t.Fatal("failed current byte limit")
		}
		checkSource(t, p)
	}
}

func FuzzSourceMapping(f *testing.F) {
	for _, seed := range []string{"", "Joa\u0303o\r\nJoa\u0303o", "a\u0315\u0300 👩‍💻", "`e\u0301\r`", "~~~\r\na\r\n~~~", "https://EXAMPLE.com/e\u0301"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if !utf8.ValidString(input) {
			return
		}
		p, err := preprocessing.Normalize(input, fieldvalidation.JobPosting)
		if err != nil {
			t.Fatal(err)
		}
		checkSource(t, p)
	})
}
