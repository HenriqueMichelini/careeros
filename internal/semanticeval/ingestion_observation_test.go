package semanticeval_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	eval "professional-information-repo/internal/semanticeval"
)

const legacyProposal = `{
 "claims":[{"id":"c1","source":"At Acme I may use Java next year.","text":"May use Java","targets":[],"question":"Which role?","meaning":{"assertion":"affirmed","intent":"aspiration","certainty":"uncertain","temporal":{"wording":"next year","precision":"approximate"}},"segmentId":"s1","sourceReference":{"version":1,"sourceId":"source","preparationVersion":"prep","segmentId":"s1","occurrenceId":"o1","originalStart":0,"originalEnd":34},"supportingSources":[{"origin":"original","source":"At Acme","segmentId":"s1"}]}],
 "operations":[],
 "outcomes":[{"claimId":"c1","kind":"clarification","reason":"owner unknown","relatedFacts":[],"relatedClaimIds":[]}],
 "unresolvedClaimIds":["c1"],"unverifiedClaimCount":0,"unplacedOperationCount":0,
 "skippedClaims":[],"coverage":{"validClaims":1,"invalidClaims":0,"discoveryComplete":false,"capacity":"within_limit"},
 "decision":{"outcome":{"kind":"accept"}}
}`

func TestNormalizeLegacyIngestionPreservesReviewableEvidence(t *testing.T) {
	raw := []byte(legacyProposal)
	got := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "legacy", Status: 200, Body: raw})
	if got.Version != "ingestion-proposal-observation-v1" || !got.Availability["endpoint"].Available {
		t.Fatalf("unavailable proposal: %+v", got)
	}
	if len(got.Claims) != 1 || got.Claims[0].ID != "c1" || got.Claims[0].Question != "Which role?" || got.Claims[0].Meaning.Intent != "aspiration" || got.Claims[0].Meaning.Certainty != "uncertain" || got.Claims[0].Meaning.Temporal.Wording != "next year" || got.Claims[0].SourceReference.OccurrenceID != "o1" || got.Claims[0].SupportingSources[0].Source != "At Acme" {
		t.Fatalf("lost uncertainty/owner/source: %+v", got.Claims)
	}
	if len(got.Operations) != 0 || got.Outcomes[0].Kind != "clarification" || !reflect.DeepEqual(got.UnresolvedClaimIDs, []string{"c1"}) || got.Coverage[0].Value.DiscoveryComplete {
		t.Fatalf("invented proposal/discovery: %+v", got)
	}
	if len(got.Mappings) != 2 || got.Mappings[0].ResponseID != "legacy" || got.Mappings[0].Path != "/claims/0" || got.Mappings[0].NormalizedID != "c1" {
		t.Fatalf("lost raw identity: %+v", got.Mappings)
	}
	if string(got.Raw[0].Response.Body) != legacyProposal || len(got.Raw[0].SHA256) != 64 {
		t.Fatal("raw evidence not retained")
	}
	raw[0] = 'x'
	if string(got.Raw[0].Response.Body) != legacyProposal {
		t.Fatal("raw evidence aliases caller memory")
	}
	if _, err := json.Marshal(got); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeIngestionRejectsCaseAliasesBeforeProjection(t *testing.T) {
	for _, tc := range []struct{ name, from, to string }{
		{"claim alias only", `"text":"May use Java"`, `"Text":"I am a Java expert"`},
		{"claim canonical and alias", `"text":"May use Java"`, `"text":"May use Java","Text":"I am a Java expert"`},
		{"meaning", `"certainty":"uncertain"`, `"certainty":"uncertain","Certainty":"certain"`},
		{"temporal", `"precision":"approximate"`, `"Precision":"approximate"`},
		{"source reference", `"originalStart":0`, `"originalStart":0,"OriginalStart":7`},
		{"supporting source", `"origin":"original"`, `"Origin":"original"`},
		{"disposition", `"reason":"owner unknown"`, `"reason":"owner unknown","Reason":"owner known"`},
		{"coverage", `"validClaims":1`, `"validClaims":1,"ValidClaims":1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := bytes.Replace([]byte(legacyProposal), []byte(tc.from), []byte(tc.to), 1)
			o := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "alias", Status: 200, Body: raw})
			if o.Availability["endpoint"].Available || len(o.Issues) == 0 || len(o.Claims) != 0 {
				t.Fatalf("case alias admitted as exact producer evidence: issues=%+v claims=%+v", o.Issues, o.Claims)
			}
			if !bytes.Equal(o.Raw[0].Response.Body, raw) {
				t.Fatal("lost raw alias evidence")
			}
		})
	}
}

func TestNormalizeSubmissionIngestionUsesExactProducerFieldNames(t *testing.T) {
	control := eval.NormalizeSubmissionIngestion(submissionEvidence())
	if !control.Availability["endpoint"].Available {
		t.Fatalf("canonical staged control unavailable: %+v", control.Issues)
	}
	for _, tc := range []struct{ name, stage, from, alias string }{
		{"stage", "plan", `"version":2`, `"Version":2`},
		{"plan", "plan", `"originalBytes":34`, `"OriginalBytes":34`},
		{"plan batch", "plan", `"payloadBytes":100`, `"PayloadBytes":100`},
		{"plan primary range", "plan", `"Start":0`, `"start":0`},
		{"assembly", "assembly", `"profileId":"profile"`, `"ProfileId":"profile"`},
		{"assembly batch", "assembly", `"skippedCount":0`, `"SkippedCount":0`},
		{"assembly primary range", "assembly", `"End":12`, `"end":12`},
		{"extraction", "extract", `"batchIndex":0`, `"BatchIndex":0`},
		{"extracted claim", "extract", `"text":"Uses Java"`, `"Text":"Uses Java"`},
		{"focus", "focus", `"claimIds":["r/0/c1"]`, `"ClaimIds":["r/0/c1"]`},
		{"operation", "focus", `"value":"Java"`, `"Value":"Java"`},
	} {
		for _, duplicate := range []bool{false, true} {
			t.Run(tc.name+"/duplicate="+fmt.Sprint(duplicate), func(t *testing.T) {
				e := submissionEvidence()
				var response *eval.IngestionResponseEvidence
				switch tc.stage {
				case "plan":
					response = &e.Plan
				case "assembly":
					response = &e.Assembly
				case "extract":
					response = &e.Batches[0]
				case "focus":
					response = &e.Focus[0]
				}
				replacement := tc.alias
				if duplicate {
					replacement = tc.from + "," + tc.alias
				}
				response.Body = bytes.Replace(response.Body, []byte(tc.from), []byte(replacement), 1)
				o := eval.NormalizeSubmissionIngestion(e)
				if o.Availability["endpoint"].Available || !hasIngestionIssue(o, response.ID, "malformed_or_unknown_schema", "") {
					t.Fatalf("case alias admitted: %+v", o.Issues)
				}
				assertIngestionRaw(t, o, *response)
			})
		}
	}
}

func hasIngestionIssue(o eval.IngestionProposalObservation, responseID, code, path string) bool {
	for _, issue := range o.Issues {
		if issue.ResponseID == responseID && issue.Code == code && issue.Path == path {
			return true
		}
	}
	return false
}

func assertIngestionRaw(t *testing.T, o eval.IngestionProposalObservation, response eval.IngestionResponseEvidence) {
	t.Helper()
	for _, raw := range o.Raw {
		if raw.Response.ID == response.ID && bytes.Equal(raw.Response.Body, response.Body) && raw.SHA256 == eval.Hash(response.Body) {
			return
		}
	}
	t.Fatal("lost original raw evidence or hash")
}

func TestNormalizeIngestionRequiresCompleteRelatedFactReferences(t *testing.T) {
	for _, staged := range []bool{false, true} {
		for _, key := range []string{"profileId", "id", "revision"} {
			for _, null := range []bool{false, true} {
				t.Run(fmt.Sprintf("staged=%v/%s/null=%v", staged, key, null), func(t *testing.T) {
					r := eval.IngestionResponseEvidence{ID: "fact", Status: 200, Body: []byte(legacyProposal)}
					e := submissionEvidence()
					if staged {
						r = e.Focus[0]
					}
					// Known wrong ownership and explicit zero revision are evidence for
					// a later evaluator, rather than grounds for semantic repair.
					r.Body = bytes.Replace(r.Body, []byte(`"relatedFacts":[]`), []byte(`"relatedFacts":[{"profileId":"wrong-owner","id":"f1","revision":0}]`), 1)
					normalize := func(response eval.IngestionResponseEvidence) eval.IngestionProposalObservation {
						if staged {
							e.Focus[0] = response
							return eval.NormalizeSubmissionIngestion(e)
						}
						return eval.NormalizeLegacyIngestion(response)
					}
					control := normalize(r)
					if !control.Availability["endpoint"].Available || control.Outcomes[0].RelatedFacts[0].ProfileID != "wrong-owner" || control.Outcomes[0].RelatedFacts[0].Revision != 0 {
						t.Fatalf("complete observed fact reference changed: %+v", control.Issues)
					}
					r.Body = changeJSON(t, r.Body, func(v map[string]any) {
						fact := v["outcomes"].([]any)[0].(map[string]any)["relatedFacts"].([]any)[0].(map[string]any)
						if null {
							fact[key] = nil
						} else {
							delete(fact, key)
						}
					})
					o := normalize(r)
					if o.Availability["endpoint"].Available || o.Availability["dispositions"].Available || !hasIngestionIssue(o, r.ID, "missing_field", "/outcomes/0/relatedFacts/0/"+key) {
						t.Fatalf("missing/null fact reference became a known value: %+v", o.Issues)
					}
					assertIngestionRaw(t, o, r)
				})
			}
		}
	}
}

func TestNormalizeSubmissionIngestionRequiresNestedProducerMetadata(t *testing.T) {
	for _, tc := range []struct{ stage, path, keys string }{
		{"plan", "/stage", "version kind"},
		{"plan", "/plan", "protocolVersion sourceId profileId runId planVersion status complete originalBytes preparedBytes batches remaining"},
		{"plan", "/plan/batches/0", "index primary context preparedBytes payloadBytes"},
		{"plan", "/plan/batches/0/primary/0", "Start End"},
		{"plan", "/plan/batches/0/context/0", "Start End"},
		{"plan", "/plan/remaining/0", "Start End"},
		{"assembly", "/stage", "version kind"},
		{"assembly", "/context", "protocolVersion sourceId profileId runId planVersion batches coverage"},
		{"assembly", "/context/batches/0", "index primary claimIds claimCount skippedCount"},
		{"assembly", "/context/batches/0/primary/0", "Start End"},
		{"assembly", "/context/coverage", "validClaims invalidClaims discoveryComplete capacity"},
		{"extract", "/stage", "version kind"},
		{"focus", "/stage", "version kind"},
	} {
		for _, key := range strings.Fields(tc.keys) {
			for _, null := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s%s/%s/null=%v", tc.stage, tc.path, key, null), func(t *testing.T) {
					e := submissionEvidence()
					e.Plan.Body = changeJSON(t, e.Plan.Body, func(v map[string]any) {
						jsonRecord(t, v, "/plan/batches/0")["context"] = []any{map[string]any{"Start": 0, "End": 3}}
						if tc.path == "/plan/remaining/0" {
							jsonRecord(t, v, "/plan")["remaining"] = []any{map[string]any{"Start": 12, "End": 34}}
						}
					})
					control := eval.NormalizeSubmissionIngestion(e)
					if tc.path != "/plan/remaining/0" && !control.Availability["endpoint"].Available {
						t.Fatalf("canonical metadata unavailable: %+v", control.Issues)
					}
					var r *eval.IngestionResponseEvidence
					switch tc.stage {
					case "plan":
						r = &e.Plan
					case "assembly":
						r = &e.Assembly
					case "extract":
						r = &e.Batches[0]
					case "focus":
						r = &e.Focus[0]
					}
					r.Body = changeJSON(t, r.Body, func(v map[string]any) {
						record := jsonRecord(t, v, tc.path)
						if null {
							record[key] = nil
						} else {
							delete(record, key)
						}
					})
					o := eval.NormalizeSubmissionIngestion(e)
					if o.Availability["endpoint"].Available || !hasIngestionIssue(o, r.ID, "missing_field", tc.path+"/"+key) {
						t.Fatalf("missing/null producer metadata became a known value: %+v", o.Issues)
					}
					assertIngestionRaw(t, o, *r)
				})
			}
		}
	}
}

func jsonRecord(t *testing.T, object map[string]any, path string) map[string]any {
	t.Helper()
	var current any = object
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if array, ok := current.([]any); ok {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(array) {
				t.Fatalf("invalid test fixture array path %s", path)
			}
			current = array[index]
		} else {
			current = current.(map[string]any)[part]
		}
	}
	return current.(map[string]any)
}

func TestNormalizeLegacyIngestionDistinguishesKnownEmptyFromUnavailable(t *testing.T) {
	empty := []byte(`{"claims":[],"operations":[],"outcomes":[],"unresolvedClaimIds":[],"skippedClaims":[],"unverifiedClaimCount":0,"unplacedOperationCount":0,"coverage":{"validClaims":0,"invalidClaims":0,"discoveryComplete":false,"capacity":"within_limit"}}`)
	got := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "empty", Status: 200, Body: empty})
	if !got.Availability["endpoint"].Available || len(got.Claims) != 0 || len(got.Operations) != 0 {
		t.Fatalf("known empty unavailable: %+v", got)
	}
	for _, tc := range []struct {
		name   string
		body   []byte
		status int
	}{
		{"missing operations", changeJSON(t, empty, func(v map[string]any) { delete(v, "operations") }), 200},
		{"null claims", changeJSON(t, empty, func(v map[string]any) { v["claims"] = nil }), 200},
		{"absent output", nil, 200}, {"null output", []byte(`null`), 200}, {"malformed", []byte(`{"claims":`), 200},
		{"failed", empty, 500}, {"unknown field", changeJSON(t, empty, func(v map[string]any) { v["unknownProposal"] = true }), 200},
		{"duplicate key", []byte(`{"claims":[],"claims":[],"operations":[]}`), 200},
		{"lossy UTF-8", bytes.Replace([]byte(legacyProposal), []byte("At Acme"), []byte{'A', 0xff}, 1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: tc.name, Status: tc.status, Body: tc.body})
			if o.Availability["endpoint"].Available || len(o.Issues) == 0 || !reflect.DeepEqual(o.Raw[0].Response.Body, tc.body) {
				t.Fatalf("unavailable evidence became success: %+v", o)
			}
			if _, err := json.Marshal(o); err != nil {
				t.Fatalf("malformed evidence cannot be exported: %v", err)
			}
		})
	}
}
func changeJSON(t *testing.T, raw []byte, edit func(map[string]any)) []byte {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	edit(v)
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func legacyOperationProposal(t *testing.T) []byte {
	return changeJSON(t, []byte(legacyProposal), func(v map[string]any) {
		v["operations"] = []any{map[string]any{"claimId": "c1", "supportingClaimIds": []any{"c1"}, "target": "experience", "entryId": "e2", "field": "achievements", "action": "update", "value": "Reduced latency 20%", "finding": "overlap"}}
		v["outcomes"] = []any{map[string]any{"claimId": "c1", "kind": "overlap", "reason": "existing owner", "relatedFacts": []any{map[string]any{"profileId": "p", "id": "fact", "revision": 3}}, "relatedClaimIds": []any{}, "operationIndexes": []any{0}}}
		v["unresolvedClaimIds"] = []any{}
	})
}
func TestNormalizeLegacyIngestionPreservesOperationsWithoutRepairingOwnership(t *testing.T) {
	got := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "owner", Status: 200, Body: legacyOperationProposal(t)})
	if !got.Availability["endpoint"].Available || got.Operations[0].EntryID != "e2" || got.Operations[0].Value != "Reduced latency 20%" || got.Outcomes[0].RelatedFacts[0].Revision != 3 || !reflect.DeepEqual(got.Outcomes[0].OperationIndexes, []int{0}) {
		t.Fatalf("changed owner/value/disposition: %+v", got)
	}
	// Normalization deliberately cannot decide whether Acme's source belongs to e2.
	// That independent expectation belongs to the later common evaluator.
	if got.Claims[0].Source != "At Acme I may use Java next year." {
		t.Fatal("source owner repaired")
	}
}
func TestNormalizeLegacyIngestionDeclinesBrokenReferencesAndCoverage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(map[string]any)
		facet string
	}{
		{"duplicate claim", func(v map[string]any) { v["claims"] = append(v["claims"].([]any), v["claims"].([]any)[0]) }, "claims"},
		{"missing text", func(v map[string]any) { delete(v["claims"].([]any)[0].(map[string]any), "text") }, "claims"},
		{"unknown support", func(v map[string]any) {
			v["operations"].([]any)[0].(map[string]any)["supportingClaimIds"] = []any{"missing"}
		}, "operations"},
		{"unknown new anchor", func(v map[string]any) { v["operations"].([]any)[0].(map[string]any)["entryId"] = "new:missing" }, "operations"},
		{"missing outcome", func(v map[string]any) { v["outcomes"] = []any{} }, "dispositions"},
		{"invalid operation index", func(v map[string]any) { v["outcomes"].([]any)[0].(map[string]any)["operationIndexes"] = []any{1} }, "dispositions"},
		{"unknown unresolved", func(v map[string]any) { v["unresolvedClaimIds"] = []any{"missing"} }, "dispositions"},
		{"coverage mismatch", func(v map[string]any) { v["coverage"].(map[string]any)["validClaims"] = 2 }, "coverage"},
		{"missing coverage bool", func(v map[string]any) { delete(v["coverage"].(map[string]any), "discoveryComplete") }, "coverage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "bad", Status: 200, Body: changeJSON(t, legacyOperationProposal(t), tc.edit)})
			if o.Availability[tc.facet].Available || o.Availability["endpoint"].Available {
				t.Fatalf("broken evidence admitted: %+v", o)
			}
		})
	}
}

func submissionEvidence() eval.SubmissionIngestionEvidence {
	return eval.SubmissionIngestionEvidence{
		Plan: eval.IngestionResponseEvidence{ID: "plan", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"plan"},"planReceipt":"plan-receipt","expiresAt":1,"plan":{"protocolVersion":"profile-submission-v1","sourceId":"source","profileId":"profile","runId":"r","planVersion":"portions-v1","status":"ready","complete":true,"originalBytes":34,"preparedBytes":34,"batches":[{"index":0,"primary":[{"Start":0,"End":12}],"context":[],"preparedBytes":12,"payloadBytes":100},{"index":1,"primary":[{"Start":12,"End":34}],"context":[],"preparedBytes":22,"payloadBytes":100}],"remaining":[]}}`)},
		Batches: []eval.IngestionResponseEvidence{
			{ID: "extract0", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"extract"},"batchReceipt":"batch0","expiresAt":1,"batchIndex":0,"claims":[{"id":"c1","source":"I use Java.","text":"Uses Java","targets":["skills"],"question":""}],"unverifiedCount":0,"skippedClaims":[],"coverage":{"validClaims":1,"invalidClaims":0,"discoveryComplete":false,"capacity":"within_limit"}}`)},
			{ID: "extract1", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"extract"},"batchReceipt":"batch1","expiresAt":1,"batchIndex":1,"claims":[{"id":"c1","source":"At Atlas I use Python.","text":"Uses Python at Atlas","targets":["projects"],"question":""}],"unverifiedCount":0,"skippedClaims":[],"coverage":{"validClaims":1,"invalidClaims":0,"discoveryComplete":false,"capacity":"possibly_exhausted"}}`)},
		},
		Assembly: eval.IngestionResponseEvidence{ID: "assembly", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"assemble"},"contextReceipt":"context","expiresAt":1,"context":{"protocolVersion":"profile-submission-v1","sourceId":"source","profileId":"profile","runId":"r","planVersion":"portions-v1","batches":[{"index":0,"primary":[{"Start":0,"End":12}],"claimIds":["r/0/c1"],"claimCount":1,"skippedCount":0},{"index":1,"primary":[{"Start":12,"End":34}],"claimIds":["r/1/c1"],"claimCount":1,"skippedCount":0}],"coverage":{"validClaims":2,"invalidClaims":0,"discoveryComplete":false,"capacity":"possibly_exhausted"}}}`)},
		Focus: []eval.IngestionResponseEvidence{
			{ID: "focus0", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"reconcile"},"focus":{"claimIds":["r/0/c1"]},"operations":[{"claimId":"r/0/c1","supportingClaimIds":["r/0/c1"],"target":"skills","entryId":"","field":"skills","action":"add","value":"Java","finding":"addition"}],"outcomes":[{"claimId":"r/0/c1","kind":"change","reason":"new skill","relatedFacts":[],"relatedClaimIds":[],"operationIndexes":[0]}]}`)},
			{ID: "focus1", Status: 200, Body: []byte(`{"stage":{"version":2,"kind":"reconcile"},"focus":{"claimIds":["r/1/c1"]},"operations":[{"claimId":"r/1/c1","supportingClaimIds":["r/1/c1"],"target":"projects","entryId":"new:r/1/c1","field":"name","action":"add","value":"Atlas","finding":"addition"}],"outcomes":[{"claimId":"r/1/c1","kind":"change","reason":"new project","relatedFacts":[],"relatedClaimIds":[],"operationIndexes":[0]}]}`)},
		},
	}
}
func TestNormalizeSubmissionIngestionPreservesBatchOwnershipAndGlobalMappings(t *testing.T) {
	got := eval.NormalizeSubmissionIngestion(submissionEvidence())
	if !got.Availability["endpoint"].Available {
		t.Fatalf("complete submission unavailable: %+v", got.Issues)
	}
	if len(got.Claims) != 2 || got.Claims[0].ID != "r/0/c1" || got.Claims[1].ID != "r/1/c1" || got.Claims[1].Source != "At Atlas I use Python." || got.Operations[1].EntryID != "new:r/1/c1" {
		t.Fatalf("collapsed batch identity/owner: %+v", got)
	}
	if !reflect.DeepEqual(got.Outcomes[0].OperationIndexes, []int{0}) || !reflect.DeepEqual(got.Outcomes[1].OperationIndexes, []int{1}) {
		t.Fatalf("did not remap focus-local operation indexes: %+v", got.Outcomes)
	}
	if len(got.Raw) != 6 || len(got.Coverage) != 3 || got.Coverage[1].Value.Capacity != "possibly_exhausted" || got.Coverage[2].Value.DiscoveryComplete {
		t.Fatalf("lost raw stage/coverage evidence: %+v", got)
	}
	found := false
	for _, m := range got.Mappings {
		found = found || (m.ResponseID == "extract1" && m.Path == "/claims/0" && m.RawID == "c1" && m.NormalizedID == "r/1/c1")
	}
	if !found {
		t.Fatalf("lost local/global ID trace: %+v", got.Mappings)
	}
}

func TestNormalizeSubmissionIngestionKeepsIncompleteStagesUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*eval.SubmissionIngestionEvidence)
		facet string
	}{
		{"missing plan", func(e *eval.SubmissionIngestionEvidence) { e.Plan.Body = nil }, "claims"},
		{"missing assembly", func(e *eval.SubmissionIngestionEvidence) { e.Assembly.Body = nil }, "claims"},
		{"missing extraction", func(e *eval.SubmissionIngestionEvidence) { e.Batches = e.Batches[:1] }, "claims"},
		{"duplicate extraction", func(e *eval.SubmissionIngestionEvidence) { e.Batches = append(e.Batches, e.Batches[0]) }, "claims"},
		{"missing focus", func(e *eval.SubmissionIngestionEvidence) { e.Focus = e.Focus[:1] }, "operations"},
		{"failed focus", func(e *eval.SubmissionIngestionEvidence) { e.Focus[1].Status = 504 }, "operations"},
		{"duplicate focus", func(e *eval.SubmissionIngestionEvidence) { e.Focus = append(e.Focus, e.Focus[0]) }, "operations"},
		{"unknown focus", func(e *eval.SubmissionIngestionEvidence) {
			e.Focus[1].Body = changeJSON(t, e.Focus[1].Body, func(v map[string]any) { v["focus"].(map[string]any)["claimIds"] = []any{"r/9/c1"} })
		}, "operations"},
		{"unknown assembly membership", func(e *eval.SubmissionIngestionEvidence) {
			e.Assembly.Body = changeJSON(t, e.Assembly.Body, func(v map[string]any) {
				v["context"].(map[string]any)["batches"].([]any)[1].(map[string]any)["claimIds"] = []any{"r/1/wrong"}
			})
		}, "claims"},
		{"assembly hides exhausted capacity", func(e *eval.SubmissionIngestionEvidence) {
			e.Assembly.Body = changeJSON(t, e.Assembly.Body, func(v map[string]any) {
				v["context"].(map[string]any)["coverage"].(map[string]any)["capacity"] = "within_limit"
			})
		}, "coverage"},
		{"empty receipt", func(e *eval.SubmissionIngestionEvidence) {
			e.Assembly.Body = changeJSON(t, e.Assembly.Body, func(v map[string]any) { v["contextReceipt"] = "" })
		}, "endpoint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := submissionEvidence()
			tc.edit(&e)
			o := eval.NormalizeSubmissionIngestion(e)
			if o.Availability[tc.facet].Available || o.Availability["endpoint"].Available {
				t.Fatalf("incomplete evidence admitted: %s: %+v", tc.name, o.Issues)
			}
			if len(o.Raw) != 2+len(e.Batches)+len(e.Focus) {
				t.Fatal("lost failed or unreturned stage evidence")
			}
			if tc.name == "missing focus" && (!o.Availability["claims"].Available || len(o.Claims) != 2 || len(o.Operations) != 1) {
				t.Fatalf("lost usable extraction beside missing focus: %+v", o.Availability)
			}
		})
	}
}
func TestNormalizeIngestionDoesNotRestoreOmittedFacts(t *testing.T) {
	legacy := changeJSON(t, []byte(legacyProposal), func(v map[string]any) {
		v["claims"] = []any{}
		v["outcomes"] = []any{}
		v["unresolvedClaimIds"] = []any{}
		v["coverage"].(map[string]any)["validClaims"] = 0
	})
	a := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "omitted", Status: 200, Body: legacy})
	if !a.Availability["endpoint"].Available || len(a.Claims) != 0 || len(a.Operations) != 0 {
		t.Fatal("legacy omission repaired or treated as absent output")
	}
	e := submissionEvidence()
	e.Batches[0].Body = changeJSON(t, e.Batches[0].Body, func(v map[string]any) { v["claims"] = []any{}; v["coverage"].(map[string]any)["validClaims"] = 0 })
	e.Assembly.Body = changeJSON(t, e.Assembly.Body, func(v map[string]any) {
		c := v["context"].(map[string]any)
		b := c["batches"].([]any)[0].(map[string]any)
		b["claimIds"] = []any{}
		b["claimCount"] = 0
		c["coverage"].(map[string]any)["validClaims"] = 1
	})
	e.Focus = e.Focus[1:]
	b := eval.NormalizeSubmissionIngestion(e)
	if !b.Availability["endpoint"].Available || len(b.Claims) != 1 || b.Claims[0].ID != "r/1/c1" || len(b.Operations) != 1 {
		t.Fatalf("staged omission repaired or treated as missing stage: %+v", b.Issues)
	}
	// The still-present original source is raw evidence; no gold or desired Java
	// fact enters either call, so the omission cannot be repaired by normalization.
}

func TestNormalizeLegacyIngestionDoesNotDefaultMalformedUncertaintyOrSourceMetadata(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"missing uncertainty", func(v map[string]any) {
			delete(v["claims"].([]any)[0].(map[string]any)["meaning"].(map[string]any), "certainty")
		}},
		{"missing original offset", func(v map[string]any) {
			delete(v["claims"].([]any)[0].(map[string]any)["sourceReference"].(map[string]any), "originalStart")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "metadata", Status: 200, Body: changeJSON(t, []byte(legacyProposal), tc.edit)})
			if o.Availability["claims"].Available || o.Availability["endpoint"].Available {
				t.Fatalf("incomplete metadata became known zero value: %+v", o.Issues)
			}
		})
	}
}
func TestNormalizeIngestionPreservesSkippedDispositionsAndTheirRawMapping(t *testing.T) {
	raw := changeJSON(t, []byte(legacyProposal), func(v map[string]any) {
		v["skippedClaims"] = []any{map[string]any{"index": 4, "reason": "source_ambiguous", "source": "same phrase", "text": "uncertain owner", "shortened": true}}
		v["unverifiedClaimCount"] = 1
		v["coverage"].(map[string]any)["invalidClaims"] = 1
	})
	o := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "skipped", Status: 200, Body: raw})
	if !o.Availability["endpoint"].Available || len(o.SkippedClaims) != 1 || !o.SkippedClaims[0].Shortened || o.SkippedClaims[0].Index != 4 {
		t.Fatalf("lost skipped evidence: %+v", o.Issues)
	}
	found := false
	for _, m := range o.Mappings {
		found = found || (m.ResponseID == "skipped" && m.Path == "/skippedClaims/0" && m.RawID == "4" && m.NormalizedID == "skipped:0")
	}
	if !found {
		t.Fatalf("lost skipped raw trace: %+v", o.Mappings)
	}
}

func TestNormalizeLegacyIngestionDeclinesUnpairedEscapesWithoutLosingValidUnicode(t *testing.T) {
	for _, tc := range []struct {
		name, encoded, want string
		available           bool
	}{
		{"unpaired high", `\ud800`, "", false}, {"unpaired low", `\udfff`, "", false},
		{"valid pair", `\ud83d\ude80`, "🚀", true}, {"literal replacement", "�", "�", true},
		{"escaped replacement", `\ufffd`, "�", true}, {"literal escape", `\\ud800`, "\\ud800", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := bytes.Replace([]byte(legacyProposal), []byte("May use Java"), []byte(tc.encoded), 1)
			o := eval.NormalizeLegacyIngestion(eval.IngestionResponseEvidence{ID: "unicode", Status: 200, Body: raw})
			if o.Availability["endpoint"].Available != tc.available {
				t.Fatalf("lossy conversion availability=%v, want %v", o.Availability["endpoint"].Available, tc.available)
			}
			if tc.available && o.Claims[0].Text != tc.want {
				t.Fatalf("Unicode text=%q, want %q", o.Claims[0].Text, tc.want)
			}
			if !bytes.Equal(o.Raw[0].Response.Body, raw) {
				t.Fatal("lost original escaped bytes")
			}
		})
	}
}
