package evaluationlab_test

import (
	"bytes"
	"encoding/json"
	"os"
	lab "professional-information-repo/internal/evaluationlab"
	eval "professional-information-repo/internal/semanticeval"
	"strings"
	"testing"
)

const endpointJava = `{"claims":[{"id":"c1","source":"I use Java","text":"Java","targets":["skills"],"question":""}],"operations":[{"claimId":"c1","target":"skills","field":"skills","entryId":"","action":"add","value":"Java","finding":"addition"}],"outcomes":[{"claimId":"c1","kind":"addition","reason":"supported","relatedFacts":[],"relatedClaimIds":[],"operationIndexes":[0]}],"unresolvedClaimIds":[],"unverifiedClaimCount":0,"unplacedOperationCount":0,"skippedClaims":[],"coverage":{"validClaims":1,"invalidClaims":0,"discoveryComplete":false,"capacity":"within_limit"}}`

func endpointRequest(t *testing.T, caseID string) lab.IngestionEndpointRequest {
	t.Helper()
	sidecar, err := lab.ExtractTaskSidecar("semantic-quality", taskFixture(t), caseID)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := map[string]string{}
	build, err := lab.CaptureBuildManifest("../..")
	if err != nil {
		t.Fatal(err)
	}
	dependencies["build-source-set"] = build.Hash
	for _, path := range []string{"internal/semanticeval/score.go", "internal/semanticeval/ingestion_observation.go", "internal/semanticeval/ingestion_submission_observation.go", "internal/evaluationlab/task_sidecar.go", "internal/evaluationlab/metrics.go", "internal/evaluationlab/endpoint_evaluation.go"} {
		raw, err := os.ReadFile("../../" + path)
		if err != nil {
			t.Fatal(err)
		}
		dependencies[path] = eval.Hash(raw)
	}
	return lab.IngestionEndpointRequest{Task: sidecar, EvaluatorDependencies: dependencies,
		Binding:  lab.EndpointEvaluationBinding{ExperimentHash: strings.Repeat("a", 64), VariantID: "legacy", RunID: "run-a", RunSnapshotHash: strings.Repeat("b", 64)},
		Evidence: lab.IngestionEndpointEvidence{Legacy: &eval.IngestionResponseEvidence{ID: "legacy", Status: 200, Body: []byte(endpointJava)}}}
}

// These scripts construct observed synthetic producer replies only. The expected
// counts below are independent literals from the unchanged curated gold.
func endpointScriptProposal(t *testing.T, caseID string, change func(map[string]any)) []byte {
	t.Helper()
	var corpus struct {
		Cases []struct {
			ID       string           `json:"id"`
			Provider []map[string]any `json:"provider"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(taskFixture(t), &corpus); err != nil {
		t.Fatal(err)
	}
	for _, c := range corpus.Cases {
		if c.ID != caseID {
			continue
		}
		claims := c.Provider[0]["claims"].([]any)
		operations := c.Provider[1]["operations"].([]any)
		outcomes := []any{}
		for _, claim := range claims {
			outcomes = append(outcomes, map[string]any{"claimId": claim.(map[string]any)["id"], "kind": "change", "reason": "synthetic reply", "relatedFacts": []any{}, "relatedClaimIds": []any{}})
		}
		proposal := map[string]any{"claims": claims, "operations": operations, "outcomes": outcomes, "unresolvedClaimIds": []any{}, "unverifiedClaimCount": 0, "unplacedOperationCount": 0, "skippedClaims": []any{}, "coverage": map[string]any{"validClaims": len(claims), "invalidClaims": 0, "discoveryComplete": false, "capacity": "within_limit"}}
		if change != nil {
			change(proposal)
		}
		raw, err := json.Marshal(proposal)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	t.Fatal("synthetic case absent")
	return nil
}

func TestEvaluateIngestionEndpointDetectsFixedGoldMutations(t *testing.T) {
	for _, tc := range []struct {
		name, caseID                               string
		change                                     func(map[string]any)
		matched, missing, relationships, forbidden int
	}{
		{"omitted operation", "en-fact", func(p map[string]any) { p["operations"] = []any{} }, 0, 1, 0, 0},
		{"wrong owner", "en-roles-projects", func(p map[string]any) { p["operations"].([]any)[0].(map[string]any)["entryId"] = "p1" }, 1, 1, 1, 0},
		{"stronger claim", "en-fact", func(p map[string]any) { p["operations"].([]any)[0].(map[string]any)["value"] = "Advanced Java" }, 0, 1, 0, 0},
		{"forbidden addition", "en-fact", func(p map[string]any) { p["operations"].([]any)[0].(map[string]any)["value"] = "Expert Java" }, 0, 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := endpointRequest(t, tc.caseID)
			gold := bytes.Clone(r.Task.Expectation)
			r.Evidence.Legacy.Body = endpointScriptProposal(t, tc.caseID, tc.change)
			got, err := lab.EvaluateIngestionEndpoint(r)
			if err != nil {
				t.Fatal(err)
			}
			counts := got.Facets["proposal-operations"].Counts
			if got.EndpointState != "available" || counts == nil || counts.Matched != tc.matched || len(counts.Missing) != tc.missing || counts.RelationshipErrors != tc.relationships || got.GeneratedWordingForbiddenAdditions == nil || *got.GeneratedWordingForbiddenAdditions != tc.forbidden {
				t.Fatalf("fixed-gold mutation hidden: %+v / %+v", counts, got)
			}
			if !bytes.Equal(gold, r.Task.Expectation) {
				t.Fatal("mutation regenerated expected labels")
			}
		})
	}
}

func TestEvaluateIngestionEndpointSeparatesCuratedForbiddenAndGeneratedWording(t *testing.T) {
	for _, tc := range []struct {
		name, caseID       string
		change             func(map[string]any)
		curated, generated int
	}{
		{"source metadata", "en-fact", func(p map[string]any) { p["claims"].([]any)[0].(map[string]any)["source"] = "An expert mentioned Java" }, 0, 0},
		{"owner identifier", "en-fact", func(p map[string]any) { p["operations"].([]any)[0].(map[string]any)["entryId"] = "expert-record" }, 1, 0},
		{"question", "en-fact", func(p map[string]any) { p["claims"].([]any)[0].(map[string]any)["question"] = "Are you an expert?" }, 1, 0},
		{"structural forbidden destination", "en-negation-aspiration", func(p map[string]any) { p["operations"].([]any)[0].(map[string]any)["target"] = "skills" }, 1, 0},
		{"structural forbidden overwrite", "en-proficiency-conflict", nil, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := endpointRequest(t, tc.caseID)
			r.Evidence.Legacy.Body = endpointScriptProposal(t, tc.caseID, tc.change)
			got, err := lab.EvaluateIngestionEndpoint(r)
			if err != nil {
				t.Fatal(err)
			}
			if got.EndpointState != "available" || got.CuratedForbiddenAdditions == nil || *got.CuratedForbiddenAdditions != tc.curated || got.GeneratedWordingForbiddenAdditions == nil || *got.GeneratedWordingForbiddenAdditions != tc.generated {
				t.Fatalf("mixed metadata with generated content: %+v", got)
			}
		})
	}
}

func endpointStagedJava() eval.SubmissionIngestionEvidence {
	return eval.SubmissionIngestionEvidence{
		Plan:     eval.IngestionResponseEvidence{ID: "plan", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"plan"},"planReceipt":"plan-receipt","expiresAt":1,"plan":{"protocolVersion":"profile-submission-v1","sourceId":"source","profileId":"profile","runId":"r","planVersion":"portions-v1","status":"ready","complete":true,"originalBytes":10,"preparedBytes":10,"batches":[{"index":0,"primary":[{"Start":0,"End":10}],"context":[],"preparedBytes":10,"payloadBytes":100}],"remaining":[]}}`)},
		Batches:  []eval.IngestionResponseEvidence{{ID: "extract", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"extract"},"batchReceipt":"batch","expiresAt":1,"batchIndex":0,"claims":[{"id":"c1","source":"I use Java","text":"Java","targets":["skills"],"question":""}],"unverifiedCount":0,"skippedClaims":[],"coverage":{"validClaims":1,"invalidClaims":0,"discoveryComplete":false,"capacity":"within_limit"}}`)}},
		Assembly: eval.IngestionResponseEvidence{ID: "assembly", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"assemble"},"contextReceipt":"context","expiresAt":1,"context":{"protocolVersion":"profile-submission-v1","sourceId":"source","profileId":"profile","runId":"r","planVersion":"portions-v1","batches":[{"index":0,"primary":[{"Start":0,"End":10}],"claimIds":["r/0/c1"],"claimCount":1,"skippedCount":0}],"coverage":{"validClaims":1,"invalidClaims":0,"discoveryComplete":false,"capacity":"within_limit"}}}`)},
		Focus:    []eval.IngestionResponseEvidence{{ID: "focus", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"reconcile"},"focus":{"claimIds":["r/0/c1"]},"operations":[{"claimId":"r/0/c1","target":"skills","entryId":"","field":"skills","action":"add","value":"Java","finding":"addition"}],"outcomes":[{"claimId":"r/0/c1","kind":"change","reason":"new skill","relatedFacts":[],"relatedClaimIds":[],"operationIndexes":[0]}]}`)}},
	}
}

func TestEvaluateIngestionEndpointUsesCommonScoringForBothProducerContracts(t *testing.T) {
	a := endpointRequest(t, "en-fact")
	b := endpointRequest(t, "en-fact")
	staged := endpointStagedJava()
	b.Evidence = lab.IngestionEndpointEvidence{Submission: &staged}
	left, err := lab.EvaluateIngestionEndpoint(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := lab.EvaluateIngestionEndpoint(b)
	if err != nil {
		t.Fatal(err)
	}
	if right.EndpointState != "available" || left.Evaluator != right.Evaluator || left.ExpectationHash != right.ExpectationHash || left.ObservationHash == right.ObservationHash || left.RawOutputHash == right.RawOutputHash {
		t.Fatalf("shared measurement or distinct evidence lost: %+v", right)
	}
	for _, facet := range []string{"claim-diagnostics", "proposal-operations"} {
		l, _ := json.Marshal(left.Facets[facet].Counts)
		r, _ := json.Marshal(right.Facets[facet].Counts)
		if string(l) != string(r) {
			t.Fatalf("producer-specific scoring %s", facet)
		}
	}
	staged.Focus = nil
	partial, err := lab.EvaluateIngestionEndpoint(b)
	if err != nil {
		t.Fatal(err)
	}
	if partial.EndpointState != "unavailable" || partial.Facets["proposal-operations"].Counts != nil {
		t.Fatal("missing staged focus scored as successful empty output")
	}
}

func TestEvaluateIngestionEndpointBindsSharedTaskAndSeparateCuratedCounts(t *testing.T) {
	request := endpointRequest(t, "en-fact")
	before, _ := json.Marshal(request)
	receipt, err := lab.EvaluateIngestionEndpoint(request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version != "lab-endpoint-evaluation-v1" || receipt.EndpointState != "available" || receipt.InputHash != request.Task.Ref.InputHash || receipt.ExpectationHash != request.Task.Ref.ExpectationHash || len(receipt.RawOutputHash) != 64 || len(receipt.ObservationHash) != 64 || len(receipt.Hash) != 64 || len(receipt.Evaluator.Hash) != 64 {
		t.Fatalf("missing immutable binding: %+v", receipt)
	}
	for _, facet := range []string{"claim-diagnostics", "proposal-operations"} {
		got := receipt.Facets[facet]
		if got.Counts == nil || got.Counts.Expected != 1 || got.Counts.Observed != 1 || got.Counts.Matched != 1 || got.Counts.Covered != 1 || got.Eligible != 1 || got.Evaluated != 1 || got.Missing != 0 {
			t.Fatalf("wrong %s counts: %+v", facet, got)
		}
	}
	if receipt.GeneratedWordingForbiddenAdditions == nil || *receipt.GeneratedWordingForbiddenAdditions != 0 || receipt.HumanReview != "pending" || receipt.SemanticReview != "unavailable" {
		t.Fatalf("unsupported semantic approval: %+v", receipt)
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) {
		t.Fatal("evaluation changed input evidence")
	}
}

func TestEvaluateIngestionEndpointRejectsForgedObservationAndKnownIdentityChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*lab.IngestionEndpointRequest)
	}{
		{"observation rows", func(r *lab.IngestionEndpointRequest) {
			o := eval.NormalizeLegacyIngestion(*r.Evidence.Legacy)
			o.Operations[0].Value = "Advanced Java"
			r.Observation = &o
		}},
		{"observation availability", func(r *lab.IngestionEndpointRequest) {
			r.Evidence.Legacy.Status = 500
			o := eval.NormalizeLegacyIngestion(*r.Evidence.Legacy)
			o.Availability["endpoint"] = eval.IngestionAvailability{Available: true, Reasons: []string{}}
			r.Observation = &o
		}},
		{"input hash", func(r *lab.IngestionEndpointRequest) { r.Task.Input = []byte(`{}`) }},
		{"expectation hash", func(r *lab.IngestionEndpointRequest) { r.Task.Expectation = []byte(`{}`) }},
		{"original fixture hash", func(r *lab.IngestionEndpointRequest) { r.Task.OriginalFixture.Original = []byte(`{}`) }},
		{"task metadata", func(r *lab.IngestionEndpointRequest) { r.Task.Ref.Language = "pt-BR" }},
		{"evaluator", func(r *lab.IngestionEndpointRequest) {
			r.ExpectedEvaluator = &lab.DigestRef{ID: "ingestion-endpoint", Version: "ingestion-endpoint-evaluator-v1", Hash: strings.Repeat("c", 64)}
		}},
		{"malformed dependency", func(r *lab.IngestionEndpointRequest) {
			r.EvaluatorDependencies["internal/semanticeval/score.go"] = "bad"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := endpointRequest(t, "en-fact")
			tc.change(&r)
			if _, err := lab.EvaluateIngestionEndpoint(r); err == nil {
				t.Fatal("known mismatch accepted")
			}
		})
	}
}

func TestEvaluateIngestionEndpointKeepsMissingEvidenceUnscored(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*lab.IngestionEndpointRequest)
	}{
		{"output", func(r *lab.IngestionEndpointRequest) { r.Evidence = lab.IngestionEndpointEvidence{} }},
		{"failed HTTP", func(r *lab.IngestionEndpointRequest) { r.Evidence.Legacy.Status = 500 }},
		{"missing required array", func(r *lab.IngestionEndpointRequest) { r.Evidence.Legacy.Body = []byte(`{"claims":[]}`) }},
		{"evaluator dependency", func(r *lab.IngestionEndpointRequest) {
			delete(r.EvaluatorDependencies, "internal/semanticeval/score.go")
		}},
		{"source dependency envelope", func(r *lab.IngestionEndpointRequest) { delete(r.EvaluatorDependencies, "build-source-set") }},
		{"original fixture", func(r *lab.IngestionEndpointRequest) { r.Task.OriginalFixture.Original = nil }},
		{"task bytes", func(r *lab.IngestionEndpointRequest) { r.Task.Input = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := endpointRequest(t, "en-fact")
			tc.change(&r)
			got, err := lab.EvaluateIngestionEndpoint(r)
			if err != nil {
				t.Fatal(err)
			}
			f := got.Facets["proposal-operations"]
			if got.EndpointState != "unavailable" || len(got.UnavailableReasons) == 0 || f.Counts != nil || f.Eligible != 1 || f.Evaluated != 0 || f.Missing != 1 || len(f.NotRecovered) != 1 || f.NotRecovered[0] != "java-destination" || got.CuratedForbiddenAdditions != nil || got.GeneratedWordingForbiddenAdditions != nil {
				t.Fatalf("missing evidence became numeric quality: %+v", got)
			}
		})
	}
}

func TestEvaluateIngestionEndpointDoesNotSilentlyInterpretUnsupportedLabels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"unknown group", func(c map[string]any) { c["gold"].([]any)[1].(map[string]any)["group"] = "invented" }},
		{"unprojected field", func(c map[string]any) {
			c["gold"].([]any)[1].(map[string]any)["match"].(map[string]any)["finding"] = "addition"
		}},
		{"unimplemented boundary invariant", func(c map[string]any) { c["boundaryInvariants"] = map[string]any{"claimMeaning": "preserved"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := mutateTaskFixture(t, taskFixture(t), tc.change)
			sidecar, err := lab.ExtractTaskSidecar("semantic-quality", raw, "en-fact")
			if err != nil {
				t.Fatal(err)
			}
			r := endpointRequest(t, "en-fact")
			r.Task = sidecar
			got, err := lab.EvaluateIngestionEndpoint(r)
			if err != nil {
				t.Fatal(err)
			}
			if got.EndpointState != "unavailable" || len(got.UnavailableReasons) == 0 || got.Facets["proposal-operations"].Counts != nil {
				t.Fatal("unsupported frozen expectation silently scored")
			}
		})
	}
	for _, change := range []func(map[string]any){
		func(c map[string]any) { c["gold"].([]any)[1].(map[string]any)["id"] = "java-fact" },
		func(c map[string]any) { c["gold"].([]any)[1].(map[string]any)["match"].(map[string]any)["value"] = "[" },
	} {
		raw := mutateTaskFixture(t, taskFixture(t), change)
		sidecar, err := lab.ExtractTaskSidecar("semantic-quality", raw, "en-fact")
		if err != nil {
			t.Fatal(err)
		}
		r := endpointRequest(t, "en-fact")
		r.Task = sidecar
		if _, err := lab.EvaluateIngestionEndpoint(r); err == nil {
			t.Fatal("malformed independent label contract accepted")
		}
	}
}

func TestEvaluateIngestionEndpointKeepsKnownContradictionsBesideMissingMetadata(t *testing.T) {
	r := endpointRequest(t, "en-fact")
	r.Task.OriginalFixture.Original = nil
	r.ExpectedEvaluator = &lab.DigestRef{ID: "ingestion-endpoint", Version: "ingestion-endpoint-evaluator-v1", Hash: strings.Repeat("c", 64)}
	if _, err := lab.EvaluateIngestionEndpoint(r); err == nil {
		t.Fatal("unrelated missing fixture hid known evaluator mismatch")
	}
}

func TestEvaluateIngestionEndpointPreservesZeroOpportunitiesAndStableMissingHashes(t *testing.T) {
	r := endpointRequest(t, "en-proficiency-conflict")
	r.Evidence.Legacy.Body = endpointScriptProposal(t, "en-proficiency-conflict", func(p map[string]any) { p["operations"] = []any{} })
	got, err := lab.EvaluateIngestionEndpoint(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Facets["proposal-operations"].Eligible != 0 {
		t.Fatal("invented endpoint opportunities")
	}
	for _, m := range got.Facets["proposal-operations"].Metrics {
		if m.Value != nil || m.UnavailableReason == "" {
			t.Fatal("zero denominator became perfect result")
		}
	}
	r.Evidence.Legacy.Body = []byte(`{"claims":[]}`)
	first, err := lab.EvaluateIngestionEndpoint(r)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		next, err := lab.EvaluateIngestionEndpoint(r)
		if err != nil {
			t.Fatal(err)
		}
		if next.Hash != first.Hash || next.ObservationHash != first.ObservationHash {
			t.Fatal("unavailable receipt hash depended on map discovery order")
		}
	}
}

func TestReevaluateIngestionPairAppendsBothReceiptsWithoutRewritingHistory(t *testing.T) {
	left := endpointRequest(t, "en-fact")
	right := endpointRequest(t, "en-fact")
	staged := endpointStagedJava()
	right.Evidence = lab.IngestionEndpointEvidence{Submission: &staged}
	right.Binding.VariantID = "staged"
	right.Binding.RunID = "run-b"
	a, err := lab.EvaluateIngestionEndpoint(left)
	if err != nil {
		t.Fatal(err)
	}
	b, err := lab.EvaluateIngestionEndpoint(right)
	if err != nil {
		t.Fatal(err)
	}
	history := []lab.EndpointEvaluationReceipt{a, b}
	before, _ := json.Marshal(history)
	revised := mutateTaskFixture(t, taskFixture(t), func(c map[string]any) {
		c["labelRevision"] = "revised-v2"
		c["gold"].([]any)[1].(map[string]any)["match"].(map[string]any)["value"] = "^Advanced Java$"
	})
	sidecar, err := lab.ExtractTaskSidecar("semantic-quality", revised, "en-fact")
	if err != nil {
		t.Fatal(err)
	}
	left.Task = sidecar
	right.Task = sidecar
	got, err := lab.ReevaluateIngestionPair(history, [2]lab.IngestionEndpointRequest{left, right})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("did not append two receipts: %d", len(got))
	}
	for i, old := range history {
		if got[i+2].PreviousReceiptHash != old.Hash || got[i+2].Hash == old.Hash || got[i+2].RawOutputHash != old.RawOutputHash || got[i+2].ExpectationHash == old.ExpectationHash || got[i+2].Facets["proposal-operations"].Counts.Matched != 0 || len(got[i+2].Facets["proposal-operations"].Counts.Missing) != 1 {
			t.Fatalf("incorrect revised measurement: %+v", got[i+2])
		}
	}
	retained, _ := json.Marshal(got[:2])
	after, _ := json.Marshal(history)
	if string(before) != string(retained) || string(before) != string(after) {
		t.Fatal("historical scores or caller history changed")
	}
	if got[2].Evaluator != got[3].Evaluator {
		t.Fatal("different common evaluators")
	}
	got[0].Facets["claim-diagnostics"].Counts.Expected = 99
	after, _ = json.Marshal(history)
	if string(before) != string(after) {
		t.Fatal("returned history aliases caller storage")
	}
}

func TestReevaluateIngestionPairRejectsKnownMismatchesAndKeepsMissingSide(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*lab.IngestionEndpointRequest, *lab.IngestionEndpointRequest, []lab.EndpointEvaluationReceipt)
		reject bool
	}{
		{"different labels", func(a, b *lab.IngestionEndpointRequest, h []lab.EndpointEvaluationReceipt) {
			b.Task.Ref.ExpectationHash = strings.Repeat("e", 64)
		}, true},
		{"different input", func(a, b *lab.IngestionEndpointRequest, h []lab.EndpointEvaluationReceipt) {
			b.Task.Ref.InputHash = strings.Repeat("e", 64)
		}, true},
		{"different evaluator", func(a, b *lab.IngestionEndpointRequest, h []lab.EndpointEvaluationReceipt) {
			b.EvaluatorDependencies["internal/semanticeval/score.go"] = strings.Repeat("e", 64)
		}, true},
		{"changed retained output", func(a, b *lab.IngestionEndpointRequest, h []lab.EndpointEvaluationReceipt) {
			b.Evidence.Legacy.Body = bytes.Replace(b.Evidence.Legacy.Body, []byte(`"value":"Java"`), []byte(`"value":"Advanced Java"`), 1)
		}, true},
		{"edited historical receipt", func(a, b *lab.IngestionEndpointRequest, h []lab.EndpointEvaluationReceipt) {
			h[0].Facets["claim-diagnostics"].Counts.Expected = 99
		}, true},
		{"known mismatch beside missing", func(a, b *lab.IngestionEndpointRequest, h []lab.EndpointEvaluationReceipt) {
			delete(a.EvaluatorDependencies, "internal/evaluationlab/metrics.go")
			b.EvaluatorDependencies["internal/semanticeval/score.go"] = strings.Repeat("e", 64)
		}, true},
		{"missing output", func(a, b *lab.IngestionEndpointRequest, h []lab.EndpointEvaluationReceipt) {
			b.Evidence = lab.IngestionEndpointEvidence{}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := endpointRequest(t, "en-fact")
			b := endpointRequest(t, "en-fact")
			b.Binding.VariantID = "staged"
			b.Binding.RunID = "run-b"
			x, err := lab.EvaluateIngestionEndpoint(a)
			if err != nil {
				t.Fatal(err)
			}
			y, err := lab.EvaluateIngestionEndpoint(b)
			if err != nil {
				t.Fatal(err)
			}
			h := []lab.EndpointEvaluationReceipt{x, y}
			tc.change(&a, &b, h)
			got, err := lab.ReevaluateIngestionPair(h, [2]lab.IngestionEndpointRequest{a, b})
			if tc.reject {
				if err == nil {
					t.Fatal("known contradiction accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 4 || got[2].EndpointState != "available" || got[3].EndpointState != "unavailable" || got[3].Facets["proposal-operations"].Counts != nil {
				t.Fatal("missing counterpart lost or became numeric")
			}
		})
	}
}

func TestEvaluateIngestionEndpointKeepsEvaluatorIdentityIndependentOfCasePatterns(t *testing.T) {
	a := endpointRequest(t, "en-fact")
	b := endpointRequest(t, "en-negation-aspiration")
	b.Evidence.Legacy.Body = endpointScriptProposal(t, "en-negation-aspiration", nil)
	x, err := lab.EvaluateIngestionEndpoint(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := lab.EvaluateIngestionEndpoint(b)
	if err != nil {
		t.Fatal(err)
	}
	if x.Evaluator != y.Evaluator || x.ExpectationHash == y.ExpectationHash {
		t.Fatal("case expectation patterns contaminated common evaluator identity")
	}
}
