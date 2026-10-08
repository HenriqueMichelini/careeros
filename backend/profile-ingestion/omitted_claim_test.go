package profileingestion

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"professional-information-repo/internal/profilevalidation"
)

// Replay the observed provider failure through the public handler, including
// the real classifier boundary. An omitted fact must never look like a duplicate.
func TestOmittedNontrivialClaimIsUnresolved(t *testing.T) {
	for _, input := range []string{"I maintain Java services", "Mantenho serviços em Java", "Shopping: bread. I maintain Java services. Weekend: walking.", "Compras: pão. Mantenho serviços em Java. Fim de semana: caminhada."} {
		for _, populated := range []bool{false, true} {
			t.Run(input+string(mustJSON(populated)), func(t *testing.T) {
				p := profile()
				if !populated {
					p = profilevalidation.Profile{Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
				}
				before := string(mustJSON(p))
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					switch calls {
					case 1:
						return jevResponse("professional_fact", "none"), nil
					case 2:
						return completion(string(mustJSON(extraction{Claims: []claim{{ID: "c1", Source: input, Text: "Uses Java", Targets: []string{"skills"}}}}))), nil
					default:
						return completion(`{"operations":[]}`), nil
					}
				})}
				w := send(t, (app{client: client}).handler(), request{Input: input, Profile: p})
				var got result
				if json.Unmarshal(w.Body.Bytes(), &got) != nil || w.Code != 200 || calls != 3 || !reflect.DeepEqual(got.UnresolvedClaimIds, []string{"c1"}) || len(got.Operations) != 0 {
					t.Fatalf("omitted new fact must be unresolved: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
				}
				if string(mustJSON(p)) != before {
					t.Fatal("proposal processing changed Profile")
				}
			})
		}
	}
}

func TestOmittedClaimDisposition(t *testing.T) {
	for _, tc := range []struct {
		name       string
		claim      claim
		profile    profilevalidation.Profile
		unresolved bool
	}{
		{"exact saved skill", claim{ID: "c1", Text: "Java", Targets: []string{"skills"}}, profilevalidation.Profile{Skills: "React, Java"}, false},
		{"exact related tool", claim{ID: "c1", Text: "Java", Targets: []string{"skills"}}, profilevalidation.Profile{Tools: "java"}, false},
		{"substring is not duplicate", claim{ID: "c1", Text: "Java", Targets: []string{"skills"}}, profilevalidation.Profile{Skills: "JavaScript"}, true},
		{"semantic match not established", claim{ID: "c1", Text: "Uses Java", Targets: []string{"skills"}}, profilevalidation.Profile{Skills: "Java"}, true},
		{"new detail not duplicate", claim{ID: "c1", Text: "Java", Targets: []string{"skills", "experience"}}, profilevalidation.Profile{Skills: "Java"}, true},
		{"question is clarification", claim{ID: "c1", Text: "Java proficiency", Question: "Which level?"}, profilevalidation.Profile{}, false},
		{"missing destination", claim{ID: "c1", Text: "Java"}, profilevalidation.Profile{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return completion(`{"operations":[]}`), nil
			})}
			ops, unresolved, _, code := (app{client: client}).compare(context.Background(), "sk-test", []claim{tc.claim}, tc.profile)
			if code != "" || len(ops) != 0 || (len(unresolved) == 1 && unresolved[0] == "c1") != tc.unresolved {
				t.Fatalf("ops=%v unresolved=%v code=%s", ops, unresolved, code)
			}
		})
	}
}

func TestOmittedClaimDoesNotHideOtherProposal(t *testing.T) {
	claims := []claim{
		{ID: "c1", Source: "I use Go", Text: "Go", Targets: []string{"skills"}},
		{ID: "c2", Source: "I maintain Java APIs", Text: "Java", Targets: []string{"skills"}},
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return completion(`{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"add","value":"Go","finding":"addition"}]}`), nil
	})}
	ops, unresolved, unplaced, code := (app{client: client}).compare(context.Background(), "sk-test", claims, profile())
	if code != "" || unplaced != 0 || len(ops) != 1 || ops[0].Value != "Go" || !reflect.DeepEqual(unresolved, []string{"c2"}) {
		t.Fatalf("ops=%v unresolved=%v unplaced=%d code=%s", ops, unresolved, unplaced, code)
	}
}

func TestExplicitUseOmissionStillProposesSkill(t *testing.T) {
	for _, source := range []string{"I use Java", "Eu uso Java."} {
		for _, noisy := range []bool{false, true} {
			for _, saved := range []string{"", "React", "Java", "JavaScript"} {
				t.Run(source+string(mustJSON(noisy))+saved, func(t *testing.T) {
					p := profilevalidation.Profile{Skills: saved, Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
					before := string(mustJSON(p))
					input := source
					if noisy {
						input = "Shopping: bread. " + source + " Weekend: walking."
					}
					calls := 0
					client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
						calls++
						switch calls {
						case 1:
							return jevResponse("professional_fact", "none"), nil
						case 2:
							return completion(string(mustJSON(extraction{Claims: []claim{{ID: "c1", Source: source, Text: "Uses Java", Targets: []string{"skills"}}}}))), nil
						default:
							return completion(`{"operations":[]}`), nil
						}
					})}
					w := send(t, (app{client: client}).handler(), request{Input: input, Profile: p})
					var got result
					if json.Unmarshal(w.Body.Bytes(), &got) != nil || w.Code != 200 || calls != 3 || len(got.UnresolvedClaimIds) != 0 {
						t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
					}
					if saved == "Java" {
						if len(got.Operations) != 0 {
							t.Fatal("saved Java duplicated")
						}
					} else if len(got.Operations) != 1 || got.Operations[0] != (operation{ClaimID: "c1", Target: "skills", Field: "skills", Action: "add", Value: "Java", Finding: "addition"}) {
						t.Fatalf("missing exact reviewable skill: %#v", got.Operations)
					}
					if string(mustJSON(p)) != before {
						t.Fatal("Profile changed before explicit apply")
					}
				})
			}
		}
	}
}

func TestExplicitUseRecoveryLimits(t *testing.T) {
	for _, tc := range []struct {
		name       string
		claim      claim
		operations string
	}{
		{"negated", claim{ID: "c1", Source: "I do not use Java", Text: "Java", Targets: []string{"skills"}}, `{"operations":[]}`},
		{"negative token", claim{ID: "c1", Source: "I use nothing", Text: "nothing", Targets: []string{"skills"}}, `{"operations":[]}`},
		{"qualified", claim{ID: "c1", Source: "I use Java only in tutorials", Text: "Java", Targets: []string{"skills"}}, `{"operations":[]}`},
		{"multiple facts", claim{ID: "c1", Source: "I use Java and Go", Text: "Java and Go", Targets: []string{"skills"}}, `{"operations":[]}`},
		{"multiple targets", claim{ID: "c1", Source: "I use Java", Text: "Java", Targets: []string{"skills", "experience"}}, `{"operations":[]}`},
		{"rejected operation", claim{ID: "c1", Source: "I use Java", Text: "Java", Targets: []string{"skills"}}, `{"operations":[{"claimId":"c1","target":"currentSalary","entryId":"","field":"currentSalary","action":"update","value":"999","finding":"addition"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return completion(tc.operations), nil })}
			ops, unresolved, _, code := (app{client: client}).compare(context.Background(), "sk-test", []claim{tc.claim}, profile())
			if code != "" || len(ops) != 0 || !reflect.DeepEqual(unresolved, []string{"c1"}) {
				t.Fatalf("ops=%v unresolved=%v code=%s", ops, unresolved, code)
			}
		})
	}
}

func TestExplicitUseRecoveryPreservesDuplicatesAndClarification(t *testing.T) {
	claims := []claim{
		{ID: "c1", Source: "I use Go", Text: "Uses Go", Targets: []string{"skills"}},
		{ID: "c2", Source: "Eu uso Go", Text: "Usa Go", Targets: []string{"skills"}},
		{ID: "c3", Source: "I use Java", Text: "Java", Targets: []string{"skills"}, Question: "Which context?"},
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return completion(`{"operations":[]}`), nil })}
	p := profile()
	ops, unresolved, _, code := (app{client: client}).compare(context.Background(), "sk-test", claims, p)
	if code != "" || len(ops) != 1 || ops[0].Value != "Go" || len(unresolved) != 0 {
		t.Fatalf("ops=%v unresolved=%v code=%s", ops, unresolved, code)
	}
	p.Tools = "Go"
	ops, unresolved, _, code = (app{client: client}).compare(context.Background(), "sk-test", claims, p)
	if code != "" || len(ops) != 0 || len(unresolved) != 0 {
		t.Fatalf("saved related capability duplicated: ops=%v unresolved=%v code=%s", ops, unresolved, code)
	}
}

func TestUnverifiedExplicitUseCannotRecover(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jevResponse("professional_fact", "none"), nil
		}
		return completion(`{"claims":[{"id":"c1","source":"I use Java","text":"Java","targets":["skills"],"question":""}]}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "I use Go", Profile: profile()})
	var got result
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || w.Code != 200 || calls != 2 || len(got.Operations) != 0 || got.UnverifiedClaimCount != 1 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
}

func TestExplicitUseRecoveryUsesSourceRatherThanGeneratedSummary(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return completion(`{"operations":[]}`), nil
	})}
	claims := []claim{{ID: "c1", Source: "I use Go", Text: "React", Targets: []string{"skills"}}}
	ops, unresolved, _, code := (app{client: client}).compare(context.Background(), "sk-test", claims, profile())
	if code != "" || len(ops) != 1 || ops[0].Value != "Go" || len(unresolved) != 0 {
		t.Fatalf("source fact hidden by an incorrect summary: ops=%v unresolved=%v code=%s", ops, unresolved, code)
	}
}
