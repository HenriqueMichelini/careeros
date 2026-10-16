package semanticeval_test

import (
	"bytes"
	"encoding/json"
	"os"
	eval "professional-information-repo/internal/semanticeval"
	"testing"
)

func TestProfileReviewAdapterUsesScopedProductionRewriteAndSupportStages(t *testing.T) {
	raw, err := os.ReadFile("../../backend/profile-review/testdata/section-rewrite.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Request      json.RawMessage
			Proposal     json.RawMessage
			Verification json.RawMessage
		}
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	json.Unmarshal(corpus.Cases[0].Request, &request)
	document := request["document"].(map[string]any)
	facts := document["facts"].([]any)
	original, _ := json.Marshal(facts[0])
	var private map[string]any
	json.Unmarshal(original, &private)
	private["id"] = "private"
	private["field"] = "email"
	private["order"] = 1
	private["value"] = "private-sentinel@example.test"
	private["normalization"].(map[string]any)["observed"] = "private-sentinel@example.test"
	document["facts"] = append(facts, private)
	requestRaw, _ := json.Marshal(request)
	c := eval.Case{ID: "section-public-seam", Task: "profile_review", Language: "en", Split: "development", Request: requestRaw, Provider: []json.RawMessage{corpus.Cases[0].Proposal, corpus.Cases[0].Verification}, Gold: []eval.Atom{{ID: "go", Group: "patches", Match: map[string]string{"factId": "^f$", "text": "^I work with Go$"}, Source: "f:0-8"}}, Notes: "Controlled section proposal only; Apply and semantic adjudication remain separate."}
	result, err := eval.Run(c, "controlled", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != 200 || !result.NoStore || len(result.Calls) != 2 || result.Score.Matched != 1 || len(result.Score.Failures) != 0 {
		t.Fatalf("section contract drift: status%d calls%d score%+v", result.Status, len(result.Calls), result.Score)
	}
	if result.Diagnostics == nil || len(result.Diagnostics.Stages) != 2 || result.Diagnostics.Stages[0].Name != "section_review" || result.Diagnostics.Stages[1].Name != "section_support_check" {
		t.Fatal("production stages missing")
	}
	if len(result.RequestEnvelopes) != 2 || len(result.ProviderOutputs) != 2 {
		t.Fatal("controlled scope/judgment evidence missing")
	}
	for _, envelope := range result.RequestEnvelopes {
		if bytes.Contains(envelope, []byte("private-sentinel")) || bytes.Contains(envelope, []byte("sk-controlled")) {
			t.Fatal("private fact or key leaked")
		}
	}
	for _, call := range result.Calls {
		if call.Provider != "api.openai.com" || call.PromptHash == "" || call.SchemaHash == "" {
			t.Fatal("wrong provider boundary")
		}
	}
	if !bytes.Equal(c.Request, requestRaw) {
		t.Fatal("proposal mutated canonical input")
	}
}
