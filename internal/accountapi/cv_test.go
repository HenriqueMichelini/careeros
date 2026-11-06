package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/cvsource"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
)

type cvProvider struct {
	calls      []string
	assessment string
}

func TestManagedCVFiniteRightsBoundsAndExplicitGenerationRetry(t *testing.T) {
	t.Run("maximum arithmetic", func(t *testing.T) {
		f, h, p, router := managedCVFixture(t, func(_ string, raw []byte) (int64, bool) {
			if !bytes.Contains(raw, []byte(`"max_completion_tokens":4000`)) || !bytes.Contains(raw, []byte(`"service_tier":"standard"`)) {
				t.Fatal("missing full output/tier")
			}
			return 64000, true
		})
		w := qualificationRequest(t, f, router, "/api/cv/generate", "max", "", managedCVBody())
		scope, e := h.ScopeStatus(context.Background(), f.Principal, "global")
		maximum := (int64(128000)*125 + int64(8000)*500) / 1000
		if e != nil || w.Code != 200 || len(p.calls) != 2 || scope.Counted != maximum || maximum != 20000 {
			t.Fatalf("%d %+v %v", w.Code, scope, e)
		}
		w = qualificationRequest(t, f, router, "/api/cv/generate", "explicit-another", "", managedCVBody())
		if w.Code != 200 || len(p.calls) != 4 {
			t.Fatal("equal-input deliberate new Action denied", w.Code, w.Body.String())
		}
	})
	t.Run("generation input bound", func(t *testing.T) {
		f, _, p, r := managedCVFixture(t, func(string, []byte) (int64, bool) { return 64001, true })
		w := qualificationRequest(t, f, r, "/api/cv/generate", "too-large", "", managedCVBody())
		if w.Code < 400 || len(p.calls) != 0 {
			t.Fatal("oversized input sent")
		}
	})
	t.Run("known failure retry", func(t *testing.T) {
		f, h, p, r := managedCVFixture(t, nil)
		n := 0
		_ = h.SetQualificationResponder(func(ctx context.Context, provider string, req *http.Request) (*http.Response, bool, error) {
			n++
			if n == 1 {
				return &http.Response{StatusCode: 429, Header: make(http.Header), Body: http.NoBody}, true, nil
			}
			return p.respond(ctx, provider, req)
		})
		w := qualificationRequest(t, f, r, "/api/cv/generate", "retry", "", managedCVBody())
		if w.Code < 400 || n != 1 || w.Header().Get("X-CareerOS-Retry-Allowed") != "true" {
			t.Fatalf("failure %d %s", w.Code, w.Body.String())
		}
		w = qualificationRequest(t, f, r, "/api/cv/generate", "retry", "", managedCVBody())
		if w.Code != 202 || n != 1 {
			t.Fatal("implicit retry")
		}
		w = qualificationRequest(t, f, r, "/api/cv/generate", "retry", "deliberate-retry", managedCVBody())
		if w.Code != 200 || n != 3 {
			t.Fatalf("retry %d %s", w.Code, w.Body.String())
		}
		w = qualificationRequest(t, f, r, "/api/cv/generate", "retry", "deliberate-retry", managedCVBody())
		if w.Code != 202 || n != 3 {
			t.Fatal("retry resend")
		}
	})
	t.Run("failed Actions count", func(t *testing.T) {
		f, h, _, r := managedCVFixture(t, nil)
		_ = h.SetQualificationResponder(func(context.Context, string, *http.Request) (*http.Response, bool, error) {
			return &http.Response{StatusCode: 429, Header: make(http.Header), Body: http.NoBody}, true, nil
		})
		for i := 0; i < 8; i++ {
			if i == 2 {
				w := qualificationRequest(t, f, r, "/api/cv/generate", "third-daily", "", managedCVBody())
				if w.Code != 429 {
					t.Fatal("daily cap bypass")
				}
			}
			if i >= 2 {
				_, _ = f.DB.Exec(`UPDATE ai_fixture_action SET created_at=clock_timestamp()-interval '25 hours'`)
			}
			w := qualificationRequest(t, f, r, "/api/cv/generate", fmt.Sprintf("failed-%d", i), "", managedCVBody())
			if w.Header().Get("X-CareerOS-Retry-Allowed") != "true" {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		w := qualificationRequest(t, f, r, "/api/cv/generate", "ninth", "", managedCVBody())
		if w.Code != 429 {
			t.Fatal("grant cap bypass", w.Code)
		}
	})
}

func TestManagedCVAuthorityAliasesAndSavedProjection(t *testing.T) {
	for _, mode := range []string{"no_profile", "foreign_reference", "changed_text", "partial", "extra_evidence", "credential", "envelope", "support_false", "support_omitted", "account", "session", "grant", "postlock", "late_profile", "late_policy", "outer_profile"} {
		t.Run(mode, func(t *testing.T) {
			f, h, p, r := managedCVFixture(t, nil)
			var in map[string]any
			_ = json.Unmarshal(managedCVBody(), &in)
			switch mode {
			case "no_profile":
				_, _ = f.DB.Exec(`DELETE FROM careeros_profiles WHERE account_id=$1`, f.Principal.AccountID)
			case "foreign_reference":
				in["facts"].([]any)[0].(map[string]any)["reference"].(map[string]any)["profileId"] = "foreign"
			case "changed_text":
				in["facts"].([]any)[0].(map[string]any)["text"] = "Invented"
			case "partial":
				in["facts"] = []any{}
			case "extra_evidence":
				in["facts"].([]any)[0].(map[string]any)["evidence"] = []any{}
			case "envelope":
				in["apiKey"] = "caller"
			case "support_false":
				in["statementSupport"] = false
			case "support_omitted":
				delete(in, "statementSupport")
			case "session":
				_, _ = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second'`)
			case "grant":
				_, _ = f.DB.Exec(`UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 second'`)
			case "postlock":
				h.SetFault("before_claim", func() error { time.Sleep(100 * time.Millisecond); return nil })
				_, _ = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '70 milliseconds'`)
			case "late_profile", "late_policy":
				_ = h.SetQualificationResponder(func(ctx context.Context, provider string, req *http.Request) (*http.Response, bool, error) {
					v, terminal, e := p.respond(ctx, provider, req)
					if len(p.calls) == 2 {
						if mode == "late_policy" {
							_, _ = f.DB.Exec(`UPDATE ai_fixture_policy SET units=units+1`)
						} else {
							_, _ = f.DB.Exec(`UPDATE careeros_profiles SET document=convert_to(replace(convert_from(document,'UTF8'),'"revision":1','"revision":2'),'UTF8') WHERE account_id=$1`, f.Principal.AccountID)
						}
					}
					return v, terminal, e
				})
			case "outer_profile":
				n := 0
				h.SetFault("before_ingestion_result_lock", func() error {
					n++
					if n == 2 {
						_, _ = f.DB.Exec(`UPDATE careeros_profiles SET document=convert_to(replace(convert_from(document,'UTF8'),'"revision":1','"revision":2'),'UTF8') WHERE account_id=$1`, f.Principal.AccountID)
					}
					return nil
				})
			}
			body, _ := json.Marshal(in)
			req := f.Request(t, context.Background(), "/.netlify/functions/account/api/cv/generate", body)
			req.Header.Del("X-OpenAI-Api-Key")
			req.Header.Del("X-TypeSafe-Api-Key")
			req.Header.Set("X-CareerOS-Action-Key", "authority")
			if mode == "credential" {
				req.Header.Set("X-OpenAI-Api-Key", "caller")
			}
			if mode == "account" {
				req.Header.Set("X-CareerOS-Account", "other")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			want := 0
			if strings.HasPrefix(mode, "late_") || mode == "outer_profile" {
				want = 2
			}
			if w.Code < 400 || len(p.calls) != want || strings.Contains(w.Body.String(), `"summary"`) {
				t.Fatalf("%d %s sends%v", w.Code, w.Body.String(), p.calls)
			}
		})
	}
}

func TestManagedCVSparseEducationAndRootOptionChanges(t *testing.T) {
	f, _, p, r := managedCVFixture(t, nil)
	var doc profiledocument.Document
	_ = json.Unmarshal(f.Document, &doc)
	doc.Entities = []profiledocument.Entity{{ID: "education", Revision: 1, Kind: "education", LegacyID: "old-ed", Order: 0}}
	doc.Facts[0].Field = "degree"
	doc.Facts[0].Value = json.RawMessage(`"Computer science"`)
	doc.Facts[0].Owner = profiledocument.Reference{ProfileID: doc.ID, ID: "education", Revision: 1}
	doc.Facts[0].Support = "unsupported"
	doc.Links = []profiledocument.Link{}
	doc.Evidence = []profiledocument.Evidence{}
	raw, _ := json.Marshal(doc)
	if _, e := profiledocument.Decode(raw); e != nil {
		t.Fatal(e)
	}
	_, _ = f.DB.Exec(`UPDATE careeros_profiles SET document=$2 WHERE account_id=$1`, f.Principal.AccountID, raw)
	body, _ := json.Marshal(map[string]any{"facts": cvsource.Facts(doc), "cvLanguage": "pt-BR", "density": "compact", "statementSupport": true})
	w := qualificationRequest(t, f, r, "/api/cv/generate", "sparse", "", body)
	if w.Code != 200 || len(p.calls) != 2 {
		t.Fatalf("sparse %d %s", w.Code, w.Body.String())
	}
	var input map[string]any
	_ = json.Unmarshal(body, &input)
	input["density"] = "detailed"
	changed, _ := json.Marshal(input)
	w = qualificationRequest(t, f, r, "/api/cv/generate", "sparse", "", changed)
	if w.Code != 409 || len(p.calls) != 2 {
		t.Fatal("changed density reused Action")
	}
	w = qualificationRequest(t, f, r, "/api/cv/generate", "sparse-detailed", "", changed)
	if w.Code != 200 || len(p.calls) != 4 {
		t.Fatal("new genuine choice denied", w.Code, w.Body.String())
	}
}

func (p *cvProvider) respond(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
	stage := aidiagnostics.StageName(ctx)
	p.calls = append(p.calls, stage)
	var in struct{ Messages []struct{ Content string } }
	if provider != "openai" || json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Messages) != 2 {
		return nil, false, io.ErrUnexpectedEOF
	}
	var content any
	if stage == "generation" {
		var root struct{ Facts []struct{ ID, Text string } }
		if json.Unmarshal([]byte(in.Messages[1].Content), &root) != nil || len(root.Facts) == 0 {
			return nil, false, io.ErrUnexpectedEOF
		}
		content = map[string]any{"summary": []any{map[string]any{"sourceIds": []string{root.Facts[0].ID}, "text": root.Facts[0].Text}}, "selected": []string{root.Facts[0].ID}, "wording": []any{}}
	} else {
		var data struct{ Statements []struct{ ID string } }
		_ = json.Unmarshal([]byte(in.Messages[1].Content), &data)
		judgments := []any{}
		for _, s := range data.Statements {
			status := p.assessment
			if status == "" {
				status = "supported"
			}
			judgments = append(judgments, map[string]any{"id": s.ID, "status": status, "reason": "Synthetic source"})
		}
		content = map[string]any{"judgments": judgments}
	}
	encoded, _ := json.Marshal(content)
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(encoded)}}}})
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, true, nil
}
func managedCVFixture(t *testing.T, bound func(string, []byte) (int64, bool)) (*aiadmission.Fixture, *aicontrol.Harness, *cvProvider, http.Handler) {
	t.Helper()
	f, _, _, _ := qualificationFixture(t)
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	if bound == nil {
		bound = func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true }
	}
	h, e := aicontrol.NewSynthetic(context.Background(), f.DSN, aicontrol.StandaloneCVSyntheticConfig("qualification", gate, bound))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	p := &cvProvider{}
	_ = h.SetQualificationResponder(p.respond)
	return f, h, p, accountapi.NewSyntheticStandaloneCV(f.Service, f.Store, h, []byte("synthetic-ingestion-signing-key-32bytes"))
}
func managedCVBody() []byte {
	return []byte(`{"cvLanguage":"en","density":"balanced","statementSupport":true,"facts":[{"id":"skill","section":"skills","entryId":"","field":"skills","text":"Java","reference":{"profileId":"p","id":"skill","revision":1},"owner":{"profileId":"p","id":"p","revision":1},"context":[],"kind":"statement","assertion":"affirmed","intent":"actual","certainty":"certain","support":"supported"}]}`)
}
func TestManagedCVUsesAssessedSavedFactsAndNoResend(t *testing.T) {
	f, _, p, router := managedCVFixture(t, nil)
	body := managedCVBody()
	w := qualificationRequest(t, f, router, "/api/cv/generate", "cv-action", "", body)
	if w.Code != 200 || len(p.calls) != 2 || !strings.Contains(w.Body.String(), `"schemaVersion":3`) {
		t.Fatalf("%d %s calls=%v", w.Code, w.Body.String(), p.calls)
	}
	w = qualificationRequest(t, f, router, "/.netlify/functions/cv-generation", "cv-action", "", body)
	if w.Code != 202 || len(p.calls) != 2 {
		t.Fatalf("replay %d %s calls=%v", w.Code, w.Body.String(), p.calls)
	}
}
func TestManagedCVRetainsCandidateWithoutAssessmentRetry(t *testing.T) {
	for _, mode := range []string{"unfit", "missing_bound", "unknown", "uncertain", "unsupported", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			f, h, p, router := managedCVFixture(t, func(_ string, raw []byte) (int64, bool) {
				calls++
				if calls == 2 && mode == "unfit" {
					return 64001, true
				}
				if calls == 2 && mode == "missing_bound" {
					return 0, false
				}
				return 64000, true
			})
			if mode == "uncertain" || mode == "unsupported" {
				p.assessment = mode
			}
			_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
				if aidiagnostics.StageName(ctx) == "statement_support_assessment" && mode == "unknown" {
					p.calls = append(p.calls, "unknown")
					return nil, false, context.DeadlineExceeded
				}
				v, terminal, e := p.respond(ctx, provider, r)
				if mode == "deadline" && len(p.calls) == 1 {
					_, e = f.DB.Exec(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '26 seconds' WHERE stage='managed_request_cv'`)
					if e != nil {
						t.Fatal(e)
					}
				}
				return v, terminal, e
			})
			body := managedCVBody()
			w := qualificationRequest(t, f, router, "/api/cv/generate", "candidate", "", body)
			var out struct {
				SchemaVersion int
				Support       struct {
					Assessment string
					Statements []struct{ Status string }
				}
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.SchemaVersion != 3 || len(out.Support.Statements) != 1 || out.Support.Statements[0].Status == "supported" || w.Header().Get("X-CareerOS-Retry-Allowed") != "" {
				t.Fatalf("candidate %d %s calls%v", w.Code, w.Body.String(), p.calls)
			}
			before := len(p.calls)
			for _, command := range []struct{ key, retry string }{{"candidate", ""}, {"candidate", "wrong-retry"}, {"new-key", ""}} {
				w = qualificationRequest(t, f, router, "/api/cv/generate", command.key, command.retry, body)
				if w.Code == 200 || len(p.calls) != before {
					t.Fatalf("review reran %d %s calls%v", w.Code, w.Body.String(), p.calls)
				}
			}
			var alias map[string]any
			_ = json.Unmarshal(body, &alias)
			alias["locale"] = alias["cvLanguage"]
			delete(alias, "cvLanguage")
			delete(alias, "density")
			aliasBody, _ := json.Marshal(alias)
			w = qualificationRequest(t, f, router, "/api/cv/generate", "new-alias-key", "", aliasBody)
			if w.Code == 200 || len(p.calls) != before {
				t.Fatal("legacy spelling bypassed review latch")
			}
			if mode == "uncertain" {
				alias["locale"] = "pt-BR"
				changed, _ := json.Marshal(alias)
				w = qualificationRequest(t, f, router, "/api/cv/generate", "genuine-language-choice", "", changed)
				if w.Code != 200 || len(p.calls) != before+2 {
					t.Fatal("genuine new language blocked", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestManagedCVDeadlineReplayDoesNotRestartClock(t *testing.T) {
	for _, mode := range []string{"claim_zero_dispatch_replay", "postlock", "between_legs"} {
		t.Run(mode, func(t *testing.T) {
			f, h, p, r := managedCVFixture(t, nil)
			expire := func() {
				if _, e := f.DB.Exec(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '26 seconds' WHERE stage='managed_request_cv'`); e != nil {
					t.Fatal(e)
				}
			}
			switch mode {
			case "claim_zero_dispatch_replay":
				normalized, _ := cvsource.Normalize(managedCVBody())
				s, e := h.Open(context.Background(), f.Principal, "deadline", "cv_generation", normalized)
				if e != nil {
					t.Fatal(e)
				}
				if e = s.BeginStandaloneCV(context.Background()); e != nil {
					t.Fatal(e)
				}
				expire()
			case "postlock":
				h.SetFault("before_claim", func() error { expire(); return nil })
			case "between_legs":
				_ = h.SetQualificationResponder(func(ctx context.Context, provider string, req *http.Request) (*http.Response, bool, error) {
					v, terminal, e := p.respond(ctx, provider, req)
					expire()
					return v, terminal, e
				})
			}
			w := qualificationRequest(t, f, r, "/api/cv/generate", "deadline", "", managedCVBody())
			if mode == "between_legs" {
				if w.Code != 200 || len(p.calls) != 1 || !strings.Contains(w.Body.String(), `"assessment":"unavailable"`) {
					t.Fatalf("candidate %d %s calls%v", w.Code, w.Body.String(), p.calls)
				}
			} else if w.Code < 400 || len(p.calls) != 0 {
				t.Fatal("clock restarted", w.Code, p.calls)
			}
		})
	}
}
