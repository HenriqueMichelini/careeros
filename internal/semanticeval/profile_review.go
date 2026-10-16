package semanticeval

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

func validateSectionReviewCase(c Case) error {
	if c.Adjudication != "pending" || c.ExpectedCalls < 1 || c.ExpectedCalls > 2 || len(c.Provider) > 2 || len(c.ProviderEnvelopes) > 2 || len(c.ProviderStatuses) > len(c.Provider) || len(c.Request) > 64<<10 || len(c.Gold) > 512 || len(c.SourceSpans) == 0 || len(c.SourceSpans) > 256 {
		return fmt.Errorf("invalid bounded section fixture: %s", c.ID)
	}
	for _, status := range c.ProviderStatuses {
		switch status {
		case 200, 401, 403, 429, 500, 503, 504:
		default:
			return fmt.Errorf("invalid controlled provider status")
		}
	}
	var request struct {
		Locale   string `json:"locale"`
		Document struct {
			Facts []struct {
				ID    string          `json:"id"`
				Value json.RawMessage `json:"value"`
			} `json:"facts"`
		} `json:"document"`
	}
	if json.Unmarshal(c.Request, &request) != nil || request.Locale != c.Language {
		return fmt.Errorf("section fixture language mismatch")
	}
	facts := map[string]string{}
	for _, fact := range request.Document.Facts {
		var text string
		if json.Unmarshal(fact.Value, &text) == nil {
			facts[fact.ID] = text
		}
	}
	spans := map[string]bool{}
	for _, span := range c.SourceSpans {
		text, ok := facts[span.FactID]
		if !ok || span.ID == "" || spans[span.ID] || span.StartByte < 0 || span.EndByte <= span.StartByte || span.EndByte > len(text) {
			return fmt.Errorf("invalid source span")
		}
		excerpt := text[span.StartByte:span.EndByte]
		if excerpt != span.Text || !utf8.ValidString(excerpt) {
			return fmt.Errorf("source span does not recover exact UTF-8 source")
		}
		spans[span.ID] = true
	}
	for _, atom := range c.Gold {
		if !spans[atom.Source] {
			return fmt.Errorf("section atom has no exact source span")
		}
	}
	return nil
}

// Section label facets keep material patch clauses separate from optional style
// metadata. All counts remain finite matching evidence with pending adjudication.
func sectionLabelFacets(c Case, output map[string]any) (map[string]Result, error) {
	facets := map[string]Result{}
	for _, group := range []string{"patches", "sectionSummary"} {
		scoped := c
		scoped.Gold = nil
		scoped.Forbidden = nil
		for _, atom := range c.Gold {
			if atom.Group == group {
				scoped.Gold = append(scoped.Gold, atom)
			}
		}
		view := map[string]any{}
		if group == "patches" {
			view["patches"] = output["patches"]
			scoped.Forbidden = c.Forbidden
		} else {
			view["summary"] = output["summary"]
		}
		raw, _ := json.Marshal(view)
		score, err := Score(scoped, raw)
		if err != nil {
			return nil, err
		}
		facets[group] = score
	}
	return facets, nil
}
