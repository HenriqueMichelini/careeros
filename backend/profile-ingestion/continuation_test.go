package profileingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestContinuationClassifiesWholeOriginalAndExtractsOnlyOnePortion(t *testing.T) {
	original := "# Role A\r\n" + strings.Repeat("- I use Java for services.\r\n", 40) + "# Role B\r\nI use Ruby."
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if calls == 1 {
			var state map[string]string
			_ = json.Unmarshal(body["state"], &state)
			if state["submission"] != original {
				t.Fatal("whole original was not classified")
			}
			return jevResponse("professional_fact", "none"), nil
		}
		var messages []struct{ Content string }
		_ = json.Unmarshal(body["messages"], &messages)
		if strings.Contains(messages[0].Content, "I use Ruby") {
			t.Fatal("extraction leaked outside selected portion")
		}
		return completion(`{"claims":[]}`), nil
	})}
	raw := map[string]any{"input": original, "profile": profile(), "portion": map[string]any{"index": 0, "bytes": 800}}
	encoded, _ := json.Marshal(raw)
	r := httptest.NewRequest("POST", "/api/profile/ingest", strings.NewReader(string(encoded)))
	r.Header.Set("X-OpenAI-Api-Key", "sk-test")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic-typesafe")
	w := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(w, r)
	var got struct {
		Continuation struct {
			Index, Total int
			Processed    bool
			SourceID     string `json:"sourceId"`
		}
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || calls != 2 || got.Continuation.Total < 2 || got.Continuation.Index != 0 || !got.Continuation.Processed || got.Continuation.SourceID == "" {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
}

func sendPortion(t *testing.T, h http.Handler, input string, index, bytes int) *httptest.ResponseRecorder {
	t.Helper()
	return send(t, h, request{Input: input, Profile: profile(), Portion: &portionRequest{Index: index, Bytes: bytes}})
}

func TestContinuationBeyondClaimAndOperationCeilings(t *testing.T) {
	var original strings.Builder
	for i := 0; i < 80; i++ {
		original.WriteString(fmt.Sprintf("- I use Technology%02d for services.\r\n", i))
	}
	input := original.String()
	source := preparedTestSource(t, input)
	total, valid, operations := 0, 0, 0
	for index := 0; index == 0 || index < total; index++ {
		calls := 0
		var extracted []claim
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return jevResponse("professional_fact", "none"), nil
			}
			if calls == 2 {
				var body struct{ Messages []struct{ Content string } }
				_ = json.NewDecoder(r.Body).Decode(&body)
				prompt := body.Messages[0].Content
				extracted = []claim{}
				for _, seg := range source.Segments() {
					text := strings.TrimSpace(source.Text()[seg.Normalized.Start:seg.Normalized.End])
					if strings.Contains(prompt, string(mustJSON(text+"\n"))) || strings.Contains(prompt, string(mustJSON(source.Text()[seg.Normalized.Start:seg.Normalized.End]))) {
						technology := regexp.MustCompile(`Technology\d+`).FindString(text)
						if technology != "" {
							extracted = append(extracted, claim{ID: fmt.Sprintf("c%d", len(extracted)), Source: text, SegmentID: seg.ID, Text: technology, Targets: []string{"skills"}})
						}
					}
				}
				return completion(string(mustJSON(extraction{Claims: extracted}))), nil
			}
			ops := []operation{}
			for _, c := range extracted {
				ops = append(ops, operation{ClaimID: c.ID, Target: "skills", Field: "skills", Action: "add", Value: c.Text, Finding: "addition"})
			}
			return completion(string(mustJSON(proposal{Operations: ops}))), nil
		})}
		w := sendPortion(t, NewHandlerWithClient(client), input, index, 800)
		var got result
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if w.Code != 200 || got.Continuation == nil || !got.Continuation.Processed {
			t.Fatalf("index=%d status=%d body=%s", index, w.Code, w.Body.String())
		}
		total = got.Continuation.Total
		valid += len(got.Claims)
		operations += len(got.Operations)
		for _, c := range got.Claims {
			if c.SourceReference.SourceID != source.ID() || input[c.SourceReference.OriginalStart:c.SourceReference.OriginalEnd] != c.Source {
				t.Fatal("lost occurrence")
			}
		}
	}
	if valid != 80 || operations != 80 || total < 2 {
		t.Fatalf("claims=%d operations=%d portions=%d", valid, operations, total)
	}
}

func TestContinuationRejectsBlockedWholeSubmissionEveryTime(t *testing.T) {
	input := strings.Repeat("- I use Java.\n", 90)
	for _, index := range []int{0, 1} {
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls > 1 {
				t.Fatal("blocked submission reached extraction")
			}
			return jevResponse("professional_fact", "detected"), nil
		})}
		w := sendPortion(t, NewHandlerWithClient(client), input, index, 800)
		if w.Code != 200 || calls != 1 || strings.Contains(w.Body.String(), "continuation") {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
}

func TestContinuationTruncationAndRetriesNeverAdvanceCoverage(t *testing.T) {
	for _, broken := range []string{"length", "malformed"} {
		for attempt := 0; attempt < 2; attempt++ {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return jevResponse("professional_fact", "none"), nil
				}
				if broken == "malformed" {
					return completion(`{"claims":`), nil
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"length","message":{"content":"{\"claims\":[]}"}}]}`)), Header: http.Header{}}, nil
			})}
			w := sendPortion(t, NewHandlerWithClient(client), "I use Java.", 0, 800)
			if w.Code != 502 || calls != 2 || strings.Contains(w.Body.String(), "continuation") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		}
	}
}

func TestContinuationPreservesContextAndWithholdsOutOfPortionEvidence(t *testing.T) {
	input := "# Role A\r\n" + strings.Repeat("- I use Java for services.\r\n", 80) + "# Role B\r\nI use Ruby.\r\n"
	source := preparedTestSource(t, input)
	plan, err := planContinuation(source, portionRequest{Index: 1, Bytes: 800})
	if err != nil {
		t.Fatal(err)
	}
	var heading string
	for _, seg := range source.Segments() {
		if strings.Contains(source.Text()[seg.Normalized.Start:seg.Normalized.End], "# Role A") {
			heading = seg.ID
			break
		}
	}
	if len(plan.Portions[1].View.Context) == 0 {
		t.Fatal("missing required heading context")
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jevResponse("professional_fact", "none"), nil
		}
		var body struct{ Messages []struct{ Content string } }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !strings.Contains(body.Messages[0].Content, heading) || strings.Contains(body.Messages[0].Content, "I use Ruby") {
			t.Fatal("wrong portion context")
		}
		return completion(`{"claims":[{"id":"c1","source":"I use Ruby.","text":"Ruby","targets":["skills"],"question":""}]}`), nil
	})}
	w := sendPortion(t, NewHandlerWithClient(client), input, 1, 800)
	var got result
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got.Continuation == nil || got.Continuation.Processed || len(got.Claims) != 0 || len(got.SkippedClaims) != 1 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestContinuationClaimCeilingRemainsUnfinished(t *testing.T) {
	input := strings.Repeat("- Java\n", 30)
	source := preparedTestSource(t, input)
	extracted := []claim{}
	for i, seg := range source.Segments() {
		extracted = append(extracted, claim{ID: fmt.Sprintf("c%d", i), Source: "Java", SegmentID: seg.ID, Text: "Java", Targets: []string{"skills"}})
	}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jevResponse("professional_fact", "none"), nil
		}
		if calls == 2 {
			return completion(string(mustJSON(extraction{Claims: extracted}))), nil
		}
		return completion(`{"operations":[]}`), nil
	})}
	w := sendPortion(t, NewHandlerWithClient(client), input, 0, 800)
	var got result
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || got.Continuation == nil || got.Continuation.Processed || len(got.Claims) != 30 || got.Coverage.Capacity != "possibly_exhausted" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
