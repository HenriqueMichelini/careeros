package profileingestion

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"testing"

	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/preprocessing"
)

func TestPreparedIngestionPreservesOriginalEvidence(t *testing.T) {
	var logs bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(oldOutput) })
	original := "# First\r\nCafe\u0301  Java\r\n# Second\r\nCafe\u0301   Java\r\n"
	source, err := preprocessing.Prepare(original, fieldvalidation.ProfessionalInformation)
	if err != nil {
		t.Fatal(err)
	}
	segment := source.Segments()[3]
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			var state map[string]string
			_ = json.Unmarshal(body["state"], &state)
			if state["submission"] != original {
				t.Fatal("classifier did not receive exact original")
			}
			return jevResponse("professional_fact", "none"), nil
		}
		var messages []struct{ Content string }
		_ = json.Unmarshal(body["messages"], &messages)
		if calls == 2 {
			if !strings.Contains(messages[0].Content, string(mustJSON(source.Text()))) || strings.Contains(messages[0].Content, `Cafe\u0301`) {
				t.Fatal("extraction did not receive full prepared view")
			}
			return completion(`{"claims":[{"id":"c1","source":"Café Java","segmentId":"` + segment.ID + `","text":"Java","targets":["skills"],"question":""}]}`), nil
		}
		if !strings.Contains(messages[0].Content, `Café   Java`) {
			t.Fatal("comparison must receive original excerpt")
		}
		return completion(`{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"add","value":"Java","finding":"addition"}]}`), nil
	})}
	w := send(t, NewHandlerWithClient(client), request{Input: original, Profile: profile()})
	if w.Code != 200 || calls != 3 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("source response cached")
	}
	for _, secret := range []string{original, source.Text(), "sk-test", "synthetic-typesafe", segment.ID} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("content, source identifier or credential leaked in logs")
		}
	}
	for _, forbidden := range []string{`"Mappings"`, `"normalizedText"`, `"preparedText"`, `"Original"`, "sk-test", "synthetic-typesafe"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatal("transient source/key leaked in response")
		}
	}
	var got struct {
		Claims []struct {
			Source          string
			SourceReference struct {
				Version                    int
				OriginalStart, OriginalEnd int
				SegmentID                  string `json:"segmentId"`
			} `json:"sourceReference"`
		}
		Operations []operation
	}
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Claims) != 1 || len(got.Operations) != 1 {
		t.Fatalf("body=%s", w.Body.String())
	}
	ref := got.Claims[0].SourceReference
	if got.Claims[0].Source != "Café   Java" || ref.Version != 1 || ref.SegmentID != segment.ID || ref.OriginalStart != 33 || original[ref.OriginalStart:ref.OriginalEnd] != got.Claims[0].Source {
		t.Fatalf("bad evidence: %#v", got.Claims)
	}
}

func TestPreparedIngestionWithholdsAmbiguousAndInvalidReferences(t *testing.T) {
	for _, tc := range []struct {
		name, input, excerpt string
		segment              int
		want                 int
	}{
		{"repeated without identity", "# A\r\nI use Java\r\n# B\r\nI use Java\r\n", "I use Java", -1, 0},
		{"wrong identity", "I use Java", "I use Java", -2, 0},
		{"same segment repeated", "Java Java", "Java", 0, 0},
		{"split normalization group", "\u0344 Java", "\u0308", 0, 0},
		{"invented source", "I use Java", "I use Ruby", 0, 0},
		{"second occurrence", "# A\r\nI use Java\r\n# B\r\nI use Java\r\n", "I use Java", 3, 1},
		{"wrapped source", "Worked with Java\r\nand PostgreSQL.", "Worked with Java and PostgreSQL.", 0, 1},
		{"protected tab", "Café\tJava", "Café\tJava", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := preparedTestSource(t, tc.input)
			id := ""
			if tc.segment >= 0 {
				id = source.Segments()[tc.segment].ID
			} else if tc.segment == -2 {
				id = strings.Repeat("0", 64)
			}
			downstream := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.typesafe.ai" {
					return jevResponse("professional_fact", "none"), nil
				}
				downstream++
				if downstream == 1 {
					return completion(string(mustJSON(extraction{Claims: []claim{{ID: "c1", Source: tc.excerpt, SegmentID: id, Text: "Java", Targets: []string{"skills"}}}}))), nil
				}
				return completion(`{"operations":[]}`), nil
			})}
			w := send(t, NewHandlerWithClient(client), request{Input: tc.input, Profile: profile()})
			var got result
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Claims) != tc.want || got.UnverifiedClaimCount != 1-tc.want || downstream != 1+tc.want {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, downstream, w.Body.String())
			}
			if tc.want == 1 && (got.Claims[0].SourceReference == nil || got.Claims[0].SourceReference.SegmentID != id) {
				t.Fatal("lost occurrence identity")
			}
		})
	}
}

func TestPreparedIngestionCapacityStopsDownstreamCalls(t *testing.T) {
	// Many physical segments fit the raw byte limit but their exact occurrence
	// metadata cannot fit the final extraction envelope. Never send a first-N view.
	input := strings.Repeat("Java\n", 6000)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.typesafe.ai" {
			t.Fatal("capacity failure reached extraction")
		}
		return jevResponse("professional_fact", "none"), nil
	})}
	w := send(t, NewHandlerWithClient(client), request{Input: input, Profile: profile()})
	if w.Code != 502 || calls != 1 || !strings.Contains(w.Body.String(), `"error":"capacity"`) {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
}

func TestPreparedSourcesKeepDifferentRawIdentities(t *testing.T) {
	a := preparedTestSource(t, "I  use Java\r\n")
	b := preparedTestSource(t, "I use Java\n")
	if a.Text() != b.Text() || a.ID() == b.ID() {
		t.Fatal("raw identity collapsed with formatting")
	}
}
