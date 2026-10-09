package profileingestion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/profilevalidation"
)

func TestHandlerExplainsOmittedClaimWithoutAssumingDuplicate(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return jevResponse("professional_fact", "none"), nil
		case 2:
			return completion(`{"claims":[{"id":"c1","source":"I maintain Java services","text":"Uses Java","targets":["skills"],"question":""}]}`), nil
		default:
			return completion(`{"operations":[]}`), nil
		}
	})}
	w := send(t, NewHandlerWithClient(client), request{Input: "I maintain Java services", Profile: profile()})
	var got struct {
		Outcomes []struct {
			ClaimID string `json:"claimId"`
			Kind    string `json:"kind"`
			Reason  string `json:"reason"`
		} `json:"outcomes"`
		Coverage struct {
			DiscoveryComplete bool `json:"discoveryComplete"`
			ValidClaims       int  `json:"validClaims"`
		} `json:"coverage"`
	}
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || w.Code != 200 || len(got.Outcomes) != 1 || got.Outcomes[0].ClaimID != "c1" || got.Outcomes[0].Kind != "unresolved" || got.Outcomes[0].Reason == "" || got.Coverage.DiscoveryComplete || got.Coverage.ValidClaims != 1 {
		t.Fatalf("missing honest claim ledger: %d %s", w.Code, w.Body.String())
	}
}

func ledgerDocument(p profilevalidation.Profile) profiledocument.Document {
	doc := profiledocument.Document{Version: 2, ID: "profile", Revision: 1, NormalizationPolicy: "exact-alias-v1", Entities: []profiledocument.Entity{}, Facts: []profiledocument.Fact{}, Evidence: []profiledocument.Evidence{}, Links: []profiledocument.Link{}}
	var view map[string]any
	_ = json.Unmarshal(mustJSON(p), &view)
	add := func(owner profiledocument.Reference, field string, value any) {
		doc.Facts = append(doc.Facts, profiledocument.Fact{ID: owner.ID + "/" + field, Revision: 1, Owner: owner, Context: []profiledocument.Reference{}, Field: field, Order: int64(len(doc.Facts)), Kind: "legacy_block", Value: mustJSON(value), Assertion: "unknown", Intent: "unknown", Certainty: "unknown", Temporal: profiledocument.Temporal{Precision: "unknown"}, Normalization: profiledocument.Normalization{Observed: stringValue(value), Policy: "exact-alias-v1"}, Origin: profiledocument.Origin{Kind: "existing_profile", Original: "unknown"}, Approval: "unreviewed", Support: "unsupported"})
	}
	ref := func(id string) profiledocument.Reference {
		return profiledocument.Reference{ProfileID: doc.ID, ID: id, Revision: 1}
	}
	for field, value := range view {
		if entries, ok := value.([]any); ok {
			for i, raw := range entries {
				entry := raw.(map[string]any)
				id := "entity/" + entry["id"].(string)
				doc.Entities = append(doc.Entities, profiledocument.Entity{ID: id, Revision: 1, Kind: field, LegacyID: entry["id"].(string), Order: int64(i)})
				for key, v := range entry {
					if key != "id" {
						add(ref(id), key, v)
					}
				}
			}
		} else if value != nil {
			add(ref(doc.ID), field, value)
		}
	}
	return doc
}
func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return string(mustJSON(v))
}
func sendLedger(t *testing.T, p profilevalidation.Profile, input string, claims []claim, comparison string, edits ...func(*profiledocument.Document)) *httptest.ResponseRecorder {
	t.Helper()
	doc := ledgerDocument(p)
	for _, edit := range edits {
		edit(&doc)
	}
	if _, err := profiledocument.Decode(mustJSON(doc)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jevResponse("professional_fact", "none"), nil
		}
		if calls == 2 {
			return completion(string(mustJSON(extraction{Claims: claims}))), nil
		}
		return completion(comparison), nil
	})}
	r := httptest.NewRequest("POST", "/api/profile/ingest", bytes.NewReader(mustJSON(map[string]any{"input": input, "profile": p, "document": doc})))
	r.Header.Set("X-OpenAI-Api-Key", "sk-test")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic-typesafe")
	w := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(w, r)
	return w
}

func TestHandlerRecognizesIdenticalExplicitQualifiedAssertions(t *testing.T) {
	for _, tc := range []struct{ source, assertion, intent, certainty, time string }{
		{"I do not use Java", "negated", "actual", "certain", ""},
		{"I hope to learn Java", "affirmed", "aspiration", "certain", ""},
		{"Maybe I use Java", "affirmed", "actual", "uncertain", ""},
		{"I used Java in 2020", "affirmed", "actual", "certain", "2020"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			p := profile()
			p.Skills = tc.source
			m := &meaning{Assertion: tc.assertion, Intent: tc.intent, Certainty: tc.certainty}
			m.Temporal.Wording, m.Temporal.Precision = tc.time, "unknown"
			if tc.time != "" {
				m.Temporal.Precision = "exact"
			}
			c := claim{ID: "c1", Source: tc.source, Text: tc.source, Targets: []string{"skills"}, Meaning: m}
			comparison := `{"operations":[],"outcomes":[{"claimId":"c1","kind":"exact_duplicate","reason":"Identical original qualified statement","relatedFacts":[{"profileId":"profile","id":"profile/skills","revision":1}],"relatedClaimIds":[]}]}`
			w := sendLedger(t, p, tc.source, []claim{c}, comparison, func(doc *profiledocument.Document) {
				for i := range doc.Facts {
					f := &doc.Facts[i]
					if f.Field == "skills" {
						f.Kind = "statement"
						f.Assertion, f.Intent, f.Certainty = tc.assertion, tc.intent, tc.certainty
						f.Temporal = profiledocument.Temporal{Wording: m.Temporal.Wording, Precision: m.Temporal.Precision}
					}
				}
			})
			var got result
			_ = json.Unmarshal(w.Body.Bytes(), &got)
			if w.Code != 200 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "exact_duplicate" {
				t.Fatalf("identical qualified fact not recognized: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
func TestHandlerRejectsRelatedTechnologiesAndQualifiedDuplicateAssertions(t *testing.T) {
	for _, tc := range []struct {
		name, saved, source, text string
		meaning                   *meaning
	}{
		{"framework versus boot", "Spring Framework", "Spring Boot", "Spring Framework", nil},
		{"individual AWS service", "AWS", "S3", "AWS", nil},
		{"negation", "Java", "I do not use Java", "Java", nil},
		{"aspiration", "Java", "I hope to learn Java", "Java", nil},
		{"uncertain", "Java", "Maybe I use Java", "Java", nil},
		{"different time", "Java", "I used Java in 2015", "Java", nil},
		{"unverified abbreviation", "JavaScript", "JS", "JavaScript", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := profile()
			p.Skills = tc.saved
			c := claim{ID: "c1", Source: tc.source, Text: tc.text, Targets: []string{"skills"}, Meaning: tc.meaning}
			comparison := `{"operations":[],"outcomes":[{"claimId":"c1","kind":"exact_duplicate","reason":"Provider assumed equivalence","relatedFacts":[{"profileId":"profile","id":"profile/skills","revision":1}],"relatedClaimIds":[]}]}`
			w := sendLedger(t, p, tc.source, []claim{c}, comparison)
			var got result
			_ = json.Unmarshal(w.Body.Bytes(), &got)
			if w.Code != 200 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "unresolved" || len(got.Operations) != 0 {
				t.Fatalf("unsafe duplicate: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandlerKeepsSimilarlyNamedEmployerSeparate(t *testing.T) {
	p := profile()
	p.Experience[0].StartDate, p.Experience[0].EndDate = "2020", "2021"
	source := "Acme Labs Engineer 2020–2021: maintained Java services"
	c := claim{ID: "c1", Source: source, Text: "Maintained Java services", Targets: []string{"experience"}}
	comparison := `{"operations":[{"claimId":"c1","target":"experience","entryId":"e1","field":"responsibilities","action":"add","value":"Maintained Java services","finding":"in_place"}],"outcomes":[{"claimId":"c1","kind":"change","reason":"Similar employer","relatedFacts":[{"profileId":"profile","id":"entity/e1/company","revision":1}],"relatedClaimIds":[]}]}`
	w := sendLedger(t, p, source, []claim{c}, comparison)
	var got result
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || len(got.Operations) != 0 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "clarification" {
		t.Fatalf("distinct employer silently merged: %d %s", w.Code, w.Body.String())
	}
}

func TestHandlerPreservesCompetingStatementsAndInvalidOutcomeReferences(t *testing.T) {
	for _, kind := range []string{"contradiction", "correction", "overlap", "additional_support", "malformed", "missing"} {
		t.Run(kind, func(t *testing.T) {
			p := profile()
			p.Skills = "Java"
			input := "I use Java\nI do not use Java"
			claims := []claim{{ID: "c1", Source: "I use Java", Text: "Java", Targets: []string{"skills"}}, {ID: "c2", Source: "I do not use Java", Text: "Does not use Java", Targets: []string{"skills"}}}
			if kind == "overlap" {
				claims[0].Source = "I use Java for services"
				claims[0].Text = "Java for services"
				input = claims[0].Source + "\n" + claims[1].Source
			}
			outcome := map[string]any{"claimId": "c1", "kind": kind, "reason": "Compare both statements", "relatedFacts": []any{map[string]any{"profileId": "profile", "id": "profile/skills", "revision": 1}}, "relatedClaimIds": []string{"c2"}}
			ops := []operation{}
			if kind == "overlap" {
				ops = append(ops, operation{ClaimID: "c1", Target: "skills", Field: "skills", Action: "update", Value: "Java for services", Finding: "overlap"})
			}
			if kind == "additional_support" {
				ops = append(ops, operation{ClaimID: "c1", Target: "skills", Field: "skills", Action: "evidence", Value: "Java", Finding: "in_place"})
			}
			if kind == "malformed" {
				outcome["kind"] = "exact_duplicate"
				outcome["relatedFacts"] = []any{map[string]any{"profileId": "profile", "id": "profile/skills", "revision": 999}}
			}
			outcomes := []any{outcome, map[string]any{"claimId": "c2", "kind": "contradiction", "reason": "Preserve the negative statement", "relatedFacts": []any{}, "relatedClaimIds": []string{"c1"}}}
			if kind == "missing" {
				outcomes = outcomes[1:]
			}
			before := string(mustJSON(p))
			w := sendLedger(t, p, input, claims, string(mustJSON(map[string]any{"operations": ops, "outcomes": outcomes})))
			var got result
			_ = json.Unmarshal(w.Body.Bytes(), &got)
			want := kind
			if kind == "malformed" || kind == "missing" {
				want = "unresolved"
			}
			if w.Code != 200 || len(got.Outcomes) != 2 || got.Outcomes[0].Kind != want || got.Outcomes[1].Kind != "contradiction" || string(mustJSON(p)) != before {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if kind != "overlap" && kind != "additional_support" && len(got.Operations) != 0 {
				t.Fatal("conflict or malformed reference produced a change")
			}
		})
	}
}

func TestHandlerReportsInvalidClaimsAndPossibleCapacityExhaustion(t *testing.T) {
	claims := []claim{}
	for i := 0; i < 30; i++ {
		claims = append(claims, claim{ID: fmt.Sprintf("c%d", i), Source: "missing", Text: "Invalid source", Targets: []string{"skills"}})
	}
	w := sendLedger(t, profile(), "I use Java", claims, `{"operations":[]}`)
	var got result
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got.Coverage.DiscoveryComplete || got.Coverage.Capacity != "possibly_exhausted" || got.Coverage.InvalidClaims != 30 || len(got.SkippedClaims) != 30 || len(got.Outcomes) != 0 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for i, item := range got.SkippedClaims {
		if item.Index != i+1 || item.Reason != "source" || item.Text != "Invalid source" || item.Source != "" {
			t.Fatal("invalid claims disappeared")
		}
	}
}

func TestSkippedClaimRetainsResolvedOriginalExcerpt(t *testing.T) {
	c := claim{ID: "c1", Source: "I use Java", Text: "Uses Java", Targets: []string{"invalid-destination"}}
	w := sendLedger(t, profile(), "I use Java", []claim{c}, `{"operations":[]}`)
	var got result
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || len(got.SkippedClaims) != 1 || got.SkippedClaims[0].Source != "I use Java" || got.SkippedClaims[0].Text != "Uses Java" || got.SkippedClaims[0].Reason != "target" {
		t.Fatalf("identifiable skipped statement lost: %d %s", w.Code, w.Body.String())
	}
}

func TestRepeatedAcceptedSupportDoesNotPretendToBeNewEvidence(t *testing.T) {
	p := profile()
	p.Skills = "Java"
	c := claim{ID: "c1", Source: "I use Java", Text: "Java", Targets: []string{"skills"}}
	comparison := `{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"evidence","value":"Java","finding":"in_place"}],"outcomes":[{"claimId":"c1","kind":"additional_support","reason":"More evidence","relatedFacts":[{"profileId":"profile","id":"profile/skills","revision":1}],"relatedClaimIds":[]}]}`
	w := sendLedger(t, p, c.Source, []claim{c}, comparison, func(doc *profiledocument.Document) {
		doc.Evidence = append(doc.Evidence, profiledocument.Evidence{ID: "accepted-source", Revision: 1, Excerpt: c.Source, Origin: "professional_information", Approval: "approved"})
		doc.Links = append(doc.Links, profiledocument.Link{ID: "support-link", Kind: "supports", State: "active", From: profiledocument.Reference{ProfileID: doc.ID, ID: "profile/skills", Revision: 1}, To: profiledocument.Reference{ProfileID: doc.ID, ID: "accepted-source", Revision: 1}})
	})
	var got result
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "exact_duplicate" || len(got.Operations) != 0 {
		t.Fatalf("old support presented as new: %d %s", w.Code, w.Body.String())
	}
}

func TestHandlerRecognizesScopedRepeatWithAllIdentityEvidence(t *testing.T) {
	p := profile()
	p.Experience[0].StartDate, p.Experience[0].EndDate = "2020", "2021"
	c := claim{ID: "c1", Source: "Built a search tool", Text: "Built a search tool", Targets: []string{"experience"}, SupportingSources: []supportingSource{{Source: "Acme"}, {Source: "Engineer"}, {Source: "2020"}, {Source: "2021"}}}
	input := "Built a search tool\nAcme\nEngineer\n2020\n2021"
	comparison := `{"operations":[],"outcomes":[{"claimId":"c1","kind":"exact_duplicate","reason":"Same complete role and period","relatedFacts":[{"profileId":"profile","id":"entity/e1/achievements","revision":1}],"relatedClaimIds":[]}]}`
	w := sendLedger(t, p, input, []claim{c}, comparison)
	var got result
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "exact_duplicate" {
		t.Fatalf("complete identity evidence lost: %d %s", w.Code, w.Body.String())
	}
}

func TestExactRepeatPointsToStableFactAndRetainsProfile(t *testing.T) {
	raw, err := os.ReadFile("../../internal/profiledocument/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Document map[string]any `json:"document"`
	}
	if json.Unmarshal(raw, &fixtures) != nil {
		t.Fatal("fixtures")
	}
	doc := fixtures[0].Document
	fact := doc["facts"].([]any)[0].(map[string]any)
	fact["value"], fact["assertion"], fact["intent"], fact["certainty"] = "JavaScript", "affirmed", "actual", "certain"
	fact["temporal"] = map[string]any{"wording": "", "precision": "unknown"}
	fact["context"] = []any{}
	doc["links"] = []any{}
	fact["support"] = "unsupported"
	p := profile()
	p.Skills = "JavaScript"
	// The snapshot and compatibility view must describe the same facts.
	doc["entities"] = []any{}
	doc["facts"] = []any{fact}
	p.Experience = p.Experience[:0]
	p.Projects = p.Projects[:0]
	p.CurrentSalary, p.AdditionalInfo = "", ""
	body := mustJSON(map[string]any{"input": "I use Javascript", "profile": p, "document": doc})
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jevResponse("professional_fact", "none"), nil
		}
		if calls == 2 {
			return completion(`{"claims":[{"id":"c1","source":"I use Javascript","text":"Javascript","targets":["skills"],"question":"","meaning":{"assertion":"affirmed","intent":"actual","certainty":"certain","temporal":{"wording":"","precision":"unknown"}}}]}`), nil
		}
		return completion(`{"operations":[],"outcomes":[{"claimId":"c1","kind":"exact_duplicate","reason":"Same exact alias","relatedFacts":[{"profileId":"p","id":"skill","revision":1}],"relatedClaimIds":[]}]}`), nil
	})}
	r := httptest.NewRequest("POST", "/api/profile/ingest", bytes.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-test")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic-typesafe")
	w := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(w, r)
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	out := got["outcomes"].([]any)[0].(map[string]any)
	if out["kind"] != "exact_duplicate" || len(out["relatedFacts"].([]any)) != 1 || len(got["operations"].([]any)) != 0 {
		t.Fatalf("%s", w.Body.String())
	}
}
