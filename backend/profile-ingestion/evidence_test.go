package profileingestion

import (
	"encoding/json"
	"net/http"
	"professional-information-repo/internal/profilevalidation"
	"strings"
	"testing"
)

func TestAcceptedCompositeEvidenceAndIdentityContext(t *testing.T) {
	input := "At Acme I was an Engineer.\nI used TypeScript around 2020."
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"I used TypeScript around 2020.","text":"Used TypeScript at Acme around 2020","targets":["experience"],"question":"","meaning":{"assertion":"affirmed","intent":"actual","certainty":"certain","temporal":{"wording":"around 2020","precision":"approximate"}},"supportingSources":[{"source":"At Acme I was an Engineer.","segmentId":""}]}]}`), nil
		}
		var body struct{ Messages []struct{ Content string } }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !strings.Contains(body.Messages[0].Content, `"startDate":"2019"`) || !strings.Contains(body.Messages[0].Content, `"endDate":"2021"`) {
			t.Error("comparison lost identity-critical period without a date detector match")
		}
		return completion(`{"operations":[{"claimId":"c1","supportingClaimIds":["c1"],"target":"experience","entryId":"job","field":"responsibilities","action":"add","value":"Used TypeScript around 2020","finding":"addition"}]}`), nil
	})}
	p := profile()
	p.Experience = []profilevalidation.Experience{{ID: "job", Company: "Acme", Title: "Engineer", StartDate: "2019", EndDate: "2021"}}
	w := send(t, NewHandlerWithClient(withAcceptedField(client)), request{Input: input, Profile: p})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"supportingSources"`) || !strings.Contains(w.Body.String(), `"approximate"`) || !strings.Contains(w.Body.String(), `"supportingClaimIds":["c1"]`) {
		t.Fatalf("lost evidence: %d %s", w.Code, w.Body.String())
	}
}

func TestIngestionWithholdsUnsupportedProtectedValues(t *testing.T) {
	for _, value := range []string{"TypeScript and Kubernetes", "TypeScript for 20 years", "JavaScript"} {
		t.Run(value, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return completion(`{"claims":[{"id":"c1","source":"I use TypeScript.","text":"TypeScript","targets":["skills"],"question":""}]}`), nil
				}
				return completion(`{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"add","value":"` + value + `","finding":"addition"}]}`), nil
			})}
			w := send(t, NewHandlerWithClient(withAcceptedField(client)), request{Input: "I use TypeScript.", Profile: profile()})
			var got result
			if json.Unmarshal(w.Body.Bytes(), &got) != nil || w.Code != 200 || len(got.Operations) != 0 || len(got.UnresolvedClaimIds) != 1 {
				t.Fatalf("unsafe operation: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLexicalExperienceCandidatesKeepStintIdentity(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"I use TypeScript.","text":"TypeScript","targets":["skills"],"question":""}]}`), nil
		}
		var body struct{ Messages []struct{ Content string } }
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, identity := range []string{`"company":"Acme"`, `"title":"Engineer"`, `"startDate":"2019"`, `"endDate":"2021"`, `"id":"job"`} {
			if !strings.Contains(body.Messages[0].Content, identity) {
				t.Errorf("candidate omitted %s", identity)
			}
		}
		return completion(`{"operations":[]}`), nil
	})}
	p := profile()
	p.Experience = []profilevalidation.Experience{{ID: "job", Company: "Acme", Title: "Engineer", StartDate: "2019", EndDate: "2021", Responsibilities: "Used TypeScript"}}
	w := send(t, NewHandlerWithClient(withAcceptedField(client)), request{Input: "I use TypeScript.", Profile: p})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
