package preprocessing_test

import (
	"reflect"
	"strings"
	"testing"

	"professional-information-repo/internal/fieldvalidation"
	p "professional-information-repo/internal/preprocessing"
)

func TestRepetitionGoldenCases(t *testing.T) {
	cases := []struct {
		name, input       string
		exact, formatting [][]int
	}{
		{"identical", "Java developer.\nJava developer.\n", [][]int{{0, 1}}, nil},
		{"all occurrences and exact subgroup", "Uso Java.\nUso  Java.\nUso Java.\nUso\u00a0Java.\n", [][]int{{0, 2}}, [][]int{{0, 1, 2, 3}}},
		{"line endings", "AWS required.\r\nAWS required.\rAWS required.\n", nil, [][]int{{0, 1, 2}}},
		{"NFC", "Joa\u0303o usa Java.\nJoão usa Java.\n", nil, [][]int{{0, 1}}},
		{"headings and boilerplate", "# A\n## Details\nEqual opportunity.\n# B\n## Details\nEqual opportunity.\n", [][]int{{1, 4}, {2, 5}}, nil},
		{"employment periods", "# Company 2020–2021\n- Used Java.\n# Company 2023–2024\n- Used Java.\n", [][]int{{1, 3}}, nil},
		{"nested list context", "- A\n  - AWS\n- B\n  - AWS\n", [][]int{{1, 3}}, nil},
		{"punctuation", "Java.\nJava!\n", nil, nil},
		{"case", "Java\njava\n", nil, nil},
		{"technologies", "C++\nC#\nJava\nJavaScript\nJS\n", nil, nil},
		{"negation", "I use Java.\nI do not use Java.\nUso Java.\nNão uso Java.\n", nil, nil},
		{"dates amounts and metrics", "2020–2021\n2023–2024\nSaved $100.\nSaved $200.\nRaised 10%.\nRaised 20%.\n", nil, nil},
		{"markers and indentation", "- AWS\n+ AWS\n  - AWS\n", nil, nil},
		{"hard breaks and trailing newline", "Java.  \nJava.\nJava.", nil, nil},
		{"inline literals", "Use `a  b` now.\nUse `a b` now.\n", nil, nil},
		{"aligned text", "A  B  C\nA B C\n", nil, nil},
		{"quoted Unicode", "> Joa\u0303o\n> João\n", nil, nil},
		{"indented literals", "    x  y\n    x y\n", nil, nil},
		{"fenced repeats", "```\nx  y\nx  y\nx y\n```\n", [][]int{{0, 4}, {1, 2}}, nil},
		{"fenced line endings", "```txt\r\nx  y\r\n```\r\n```txt\nx  y\n```\n", nil, nil},
		{"protected URL", "https://example.com/Joa\u0303o\nhttps://example.com/João\n", nil, nil},
		{"blank and separator coverage", "\n---\n\n---\n", [][]int{{0, 2}, {1, 3}}, nil},
		{"empty", "", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := p.Prepare(tc.input, fieldvalidation.ProfessionalInformation)
			if err != nil {
				t.Fatal(err)
			}
			beforeSegments, beforeMappings, beforeText := s.Segments(), s.Mappings(), s.Text()
			groups := s.Repetitions()
			var exact, formatting [][]int
			segments := s.Segments()
			ids := map[string]bool{}
			for _, group := range groups {
				if group.SourceID != s.ID() || group.Version != p.RepetitionVersion || group.ID == "" || ids[group.ID] {
					t.Fatalf("invalid group identity: %+v", group)
				}
				ids[group.ID] = true
				var indices []int
				for _, occurrence := range group.Occurrences {
					found := false
					for i, seg := range segments {
						if occurrence.ID == seg.ID {
							if !reflect.DeepEqual(occurrence, seg) {
								t.Fatal("lost occurrence context")
							}
							indices = append(indices, i)
							found = true
						}
					}
					if !found {
						t.Fatal("unknown occurrence")
					}
				}
				switch group.Comparison {
				case p.ExactRepetition:
					exact = append(exact, indices)
				case p.FormattingEquivalence:
					formatting = append(formatting, indices)
				default:
					t.Fatal("unknown comparison")
				}
			}
			if !reflect.DeepEqual(exact, tc.exact) || !reflect.DeepEqual(formatting, tc.formatting) {
				t.Fatalf("groups exact=%v formatting=%v; want %v %v", exact, formatting, tc.exact, tc.formatting)
			}
			if !reflect.DeepEqual(groups, s.Repetitions()) {
				t.Fatal("nondeterministic annotations")
			}
			if s.Original() != tc.input || s.Text() != beforeText || !reflect.DeepEqual(s.Segments(), beforeSegments) || !reflect.DeepEqual(s.Mappings(), beforeMappings) {
				t.Fatal("annotations changed source")
			}
			checkPrepared(t, s) // Every original occurrence remains resolvable, in order.
		})
	}
}

func TestRepetitionsAreDefensiveAndSubmissionScoped(t *testing.T) {
	s, _ := p.Prepare("# A\n- Java\n# B\n- Java\n", fieldvalidation.ProfessionalInformation)
	want := s.Repetitions()
	got := s.Repetitions()
	if got[0].Occurrences[0].ParentID == got[0].Occurrences[1].ParentID {
		t.Fatal("lost distinct parents")
	}
	got[0].Occurrences[0].Original[0].Start = 999
	got[0].Occurrences[0].ParentID = "changed"
	got[0].ID = "changed"
	if !reflect.DeepEqual(want, s.Repetitions()) {
		t.Fatal("caller mutated source annotations")
	}
	for _, other := range []struct {
		input string
		field fieldvalidation.Field
	}{
		{s.Original() + "extra\n", s.Field()},
		{s.Original(), fieldvalidation.JobPosting},
	} {
		next, _ := p.Prepare(other.input, other.field)
		if next.Repetitions()[0].ID == want[0].ID {
			t.Fatal("group identity escaped submission scope")
		}
	}
	normalized, _ := p.Normalize("Java\nJava\n", s.Field())
	if len(normalized.Repetitions()) != 0 || len((p.Source{}).Repetitions()) != 0 {
		t.Fatal("groups without segments")
	}
}

func TestRepetitionInputBounds(t *testing.T) {
	for _, size := range []int{30000, 30720} {
		s, err := p.Prepare(strings.Repeat("x\n", size/2), fieldvalidation.JobPosting)
		if err != nil {
			t.Fatal(err)
		}
		groups := s.Repetitions()
		if len(groups) != 1 || len(groups[0].Occurrences) != size/2 {
			t.Fatal("lost repetitions at input bound")
		}
		if s.Original() != s.Text() || len(s.Text()) != size {
			t.Fatal("removed repeated content")
		}
	}
}
