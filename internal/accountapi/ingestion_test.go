package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ingestion "professional-information-repo/backend/profile-ingestion"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestManagedIngestionUsesInheritedQualificationAuthority(t *testing.T) {
	f, prior, provider, qualification := qualificationFixture(t)
	prior.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	_ = qualificationRequest(t, f, qualification, "/api/qualification-gaps", "prior-qualification", "", f.Body(t, "/api/qualification-gaps"))
	before, err := prior.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || before.Counted != 1344 || before.Occupied != 1 {
		t.Fatalf("prior hold %+v %v", before, err)
	}
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	cfg := aicontrol.IngestionSyntheticConfig("qualification", gate, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true })
	h, err := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	_ = h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		resp, e := provider.Respond(ctx, p, r)
		return resp, true, e
	})
	router := accountapi.NewSyntheticIngestion(f.Service, f.Store, h, []byte("synthetic-ingestion-signing-key-32bytes"))
	w := qualificationRequest(t, f, router, "/api/profile/ingest", "ingestion-blocked", "", ingestionBody(t, f, "plan", nil))
	if w.Code != 429 || len(provider.Calls) != 0 {
		t.Fatalf("inherited capacity bypass: %d %s %+v", w.Code, w.Body.String(), provider.Calls)
	}
	after, e := h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || after != before {
		t.Fatalf("pool copied/reset %+v %+v %v", before, after, e)
	}
	// Reopening the original qualification operation remains status-only.
	w = qualificationRequest(t, f, accountapi.NewSyntheticQualification(f.Service, f.Store, h), "/api/qualification-gaps", "prior-qualification", "", f.Body(t, "/api/qualification-gaps"))
	if w.Code != 202 || len(provider.Calls) != 0 {
		t.Fatalf("prior identity resent: %d %s", w.Code, w.Body.String())
	}
}

func TestManagedIngestionSharesInheritedQualificationLastMoney(t *testing.T) {
	f, prior, _, qualification := qualificationFixture(t)
	body := f.Body(t, "/api/qualification-gaps")
	first := qualificationRequest(t, f, qualification, "/api/qualification-gaps", "prior-money", "", body)
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	s, e := prior.Open(context.Background(), f.Principal, "prior-money", "qualification_gaps", body)
	if e != nil {
		t.Fatal(e)
	}
	status, e := prior.Status(context.Background(), f.Principal, s.ID())
	if e != nil {
		t.Fatal(e)
	}
	amount := int64(9974656) // deliberately artificial historical gross bill; leaves exactly1,344 in the shared pilot pool.
	var fieldGate aicontrol.Dispatch
	for _, d := range status.Dispatches {
		if d.Provider == "typesafe" {
			fieldGate = d
		}
	}
	if e = prior.SyntheticReviewer().ApplyEvidence(context.Background(), f.Principal, aicontrol.Evidence{ID: "prior-bill", Dispatch: fieldGate.ID, Provider: fieldGate.Provider, Kind: "bill", Amount: &amount, ReleaseFloor: true}); e != nil {
		t.Fatal(e)
	}
	// This confirmed historical bill is outside the rolling-day window but
	// permanently remains in the pilot total. The other unresolved qualification
	// legs continue counting in rolling pools despite their age.
	if _, e = f.DB.Exec(`UPDATE ai_fixture_dispatch SET intent_at=clock_timestamp()-interval '25 hours' WHERE namespace='qualification'`); e != nil {
		t.Fatal(e)
	}
	second := qualificationParticipant(t, f, "ingestion-second")
	third := qualificationParticipant(t, f, "ingestion-third")
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	cfg := aicontrol.IngestionSyntheticConfig("qualification", gate, func(provider string, _ []byte) (int64, bool) {
		if provider == "typesafe" {
			return 32000, true
		}
		return 64000, true
	})
	h, e := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	p := &aiadmission.Provider{T: t, Workflow: "profile_ingestion"}
	var mu sync.Mutex
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		mu.Lock()
		defer mu.Unlock()
		v, e := p.Respond(ctx, provider, r)
		return v, true, e
	})
	router := accountapi.NewSyntheticIngestion(f.Service, f.Store, h, []byte("synthetic-ingestion-signing-key-32bytes"))
	type run struct {
		f    *aiadmission.Fixture
		body []byte
	}
	runs := []run{}
	for _, who := range []*aiadmission.Fixture{second, third} {
		w := qualificationRequest(t, who, router, "/api/profile/ingest", "last-money", "", ingestionBody(t, who, "plan", nil))
		var plan ingestion.SubmissionPlanResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
			t.Fatal(w.Body.String())
		}
		runs = append(runs, run{who, ingestionBody(t, who, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})})
	}
	start := make(chan struct{})
	done := make(chan int, 2)
	for _, run := range runs {
		run := run
		go func() {
			<-start
			w := qualificationRequest(t, run.f, router, "/api/profile/ingest", "last-money", "", run.body)
			done <- w.Code
		}()
	}
	close(start)
	a, b := <-done, <-done
	scope, e := h.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || scope.Counted != 10000000 || len(p.Calls) != 1 || a != 429 || b != 429 {
		t.Fatalf("cross-workflow last-money race: %d/%d %+v sends=%+v %v", a, b, scope, p.Calls, e)
	}
	w := qualificationRequest(t, f, accountapi.NewSyntheticQualification(f.Service, f.Store, h), "/api/qualification-gaps", "prior-money", "", body)
	if w.Code != 202 || len(p.Calls) != 1 {
		t.Fatalf("old qualification replay contract changed: %d %s", w.Code, w.Body.String())
	}
}

func ingestionFixture(t *testing.T) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, http.Handler) {
	return ingestionBoundFixture(t, func(_ string, raw []byte) (int64, bool) { return int64(len(raw)) + 64, true })
}

func emptyIngestionSnapshot(t *testing.T, f *aiadmission.Fixture) map[string]any {
	t.Helper()
	if _, e := f.DB.Exec(`DELETE FROM careeros_profiles WHERE account_id=$1`, f.Principal.AccountID); e != nil {
		t.Fatal(e)
	}
	var doc map[string]any
	_ = json.Unmarshal(f.Document, &doc)
	doc["facts"] = []any{}
	doc["entities"] = []any{}
	doc["evidence"] = []any{}
	doc["links"] = []any{}
	raw, _ := json.Marshal(doc)
	if _, e := profiledocument.Decode(raw); e != nil {
		t.Fatal(e)
	}
	return map[string]any{"profile": profiledocument.View(profiledocument.Document{}), "document": json.RawMessage(raw)}
}

func ingestionContext(t *testing.T, f *aiadmission.Fixture, router http.Handler, key string, snapshot map[string]any) ingestion.SubmissionAssemblyResponse {
	t.Helper()
	post := func(kind string, extra map[string]any) *httptest.ResponseRecorder {
		fields := map[string]any{}
		for k, v := range snapshot {
			fields[k] = v
		}
		for k, v := range extra {
			fields[k] = v
		}
		return qualificationRequest(t, f, router, "/api/profile/ingest", key, "", ingestionBody(t, f, kind, fields))
	}
	var plan ingestion.SubmissionPlanResponse
	w := post("plan", nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatalf("plan %d %s", w.Code, w.Body.String())
	}
	var extracted ingestion.SubmissionExtractionResponse
	w = post("extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &extracted) != nil {
		t.Fatalf("extract %d %s", w.Code, w.Body.String())
	}
	var assembled ingestion.SubmissionAssemblyResponse
	w = post("assemble", map[string]any{"planReceipt": plan.PlanReceipt, "batchReceipts": []string{extracted.BatchReceipt}})
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &assembled) != nil {
		t.Fatalf("assemble %d %s", w.Code, w.Body.String())
	}
	return assembled
}

func TestManagedIngestionAbsentProfileFirstReviewAndExplicitSave(t *testing.T) {
	f, h, p, router := ingestionFixture(t)
	snapshot := emptyIngestionSnapshot(t, f)
	// Qualification retains its stricter saved/useful Profile precondition.
	w := qualificationRequest(t, f, accountapi.NewSyntheticQualification(f.Service, f.Store, h), "/api/qualification-gaps", "no-profile-qualification", "", f.Body(t, "/api/qualification-gaps"))
	if w.Code < 400 || len(p.Calls) != 0 {
		t.Fatalf("qualification accepted absent Profile: %d", w.Code)
	}
	assembled := ingestionContext(t, f, router, "empty-first-submission", snapshot)
	claimID := assembled.Context.Batches[0].ClaimIDs[0]
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, request *http.Request) (*http.Response, bool, error) {
		response, e := p.Respond(ctx, provider, request)
		if provider == "openai" {
			proposal := map[string]any{"operations": []any{map[string]any{"claimId": claimID, "target": "skills", "entryId": "", "field": "skills", "action": "add", "value": "Java", "finding": "addition", "supportingClaimIds": []string{claimID}}}, "outcomes": []any{map[string]any{"claimId": claimID, "kind": "change", "reason": "new_skill", "relatedFacts": []any{}, "relatedClaimIds": []any{}}}}
			rewriteSyntheticCompletion(t, response, func(content map[string]any) {
				for key, value := range proposal {
					content[key] = value
				}
			})
		}
		return response, true, e
	})
	fields := map[string]any{"contextReceipt": assembled.ContextReceipt, "focus": map[string]any{"claimIds": assembled.Context.Batches[0].ClaimIDs}}
	for k, v := range snapshot {
		fields[k] = v
	}
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "empty-first-submission", "", ingestionBody(t, f, "reconcile", fields))
	if w.Code != 200 || len(p.Calls) != 4 {
		t.Fatalf("first review %d %s calls=%d", w.Code, w.Body.String(), len(p.Calls))
	}
	var review struct {
		Operations []struct{ Value string }
		Outcomes   []struct{ Kind string }
	}
	if json.Unmarshal(w.Body.Bytes(), &review) != nil || len(review.Operations) != 1 || review.Operations[0].Value != "Java" {
		t.Fatalf("first reviewed addition missing: %s", w.Body.String())
	}
	var rows int
	if e := f.DB.QueryRow(`SELECT count(*) FROM careeros_profiles WHERE account_id=$1`, f.Principal.AccountID).Scan(&rows); e != nil || rows != 0 {
		t.Fatalf("implicit Profile persistence: %d %v", rows, e)
	}
	// An explicit reviewed candidate uses the actual account/pilot Save path;
	// neither planning nor inference fabricates a saved empty Profile first.
	command, _ := json.Marshal(map[string]any{"operationId": "reviewed-first-save", "expectedRevision": 0, "document": json.RawMessage(f.Document)})
	r := f.Request(t, context.Background(), "/api/profile/current", command)
	r.Method = http.MethodPut
	w = httptest.NewRecorder()
	accountapi.NewPilot(f.Service, f.Store, nil).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"operationId":"reviewed-first-save"`) || len(p.Calls) != 4 {
		t.Fatalf("acknowledged explicit Save %d %s", w.Code, w.Body.String())
	}
	if e := f.DB.QueryRow(`SELECT count(*) FROM careeros_profiles WHERE account_id=$1`, f.Principal.AccountID).Scan(&rows); e != nil || rows != 1 {
		t.Fatalf("first Profile row %d %v", rows, e)
	}
}

func TestManagedIngestionFrozenProfileAtFinalBufferedRelease(t *testing.T) {
	for _, kind := range []string{"existing_row_update", "first_row_creation", "existing_row_update_outer_flush", "first_row_creation_outer_flush"} {
		t.Run(kind, func(t *testing.T) {
			f, h, p, router := ingestionFixture(t)
			snapshot := map[string]any{}
			firstRow := strings.HasPrefix(kind, "first_row_creation")
			outerFlush := strings.HasSuffix(kind, "outer_flush")
			if firstRow {
				snapshot = emptyIngestionSnapshot(t, f)
			}
			assembled := ingestionContext(t, f, router, "late-profile-change", snapshot)
			changed := false
			change := func(ctx context.Context) {
				if firstRow {
					// A newly saved row with the SAME empty view must also
					// invalidate the frozen authoritative absence binding.
					if _, err := f.Store.SaveProfile(ctx, f.Principal, "concurrent-first-save", 0, snapshot["document"].(json.RawMessage)); err != nil {
						t.Fatal(err)
					}
				} else {
					var doc map[string]any
					_ = json.Unmarshal(f.Document, &doc)
					fact := doc["facts"].([]any)[0].(map[string]any)
					fact["value"] = "TypeScript"
					normal, _ := json.Marshal(profiledocument.NormalizeTerm("TypeScript"))
					var normalization any
					_ = json.Unmarshal(normal, &normalization)
					fact["normalization"] = normalization
					raw, _ := json.Marshal(doc)
					if _, err := profiledocument.Decode(raw); err != nil {
						t.Fatal(err)
					}
					if _, err := f.DB.Exec(`UPDATE careeros_profiles SET document=$2 WHERE account_id=$1`, f.Principal.AccountID, raw); err != nil {
						t.Fatal(err)
					}
				}
				changed = true
			}
			_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
				response, e := p.Respond(ctx, provider, r)
				if provider == "openai" && !outerFlush {
					change(ctx)
				}
				return response, true, e
			})
			if outerFlush {
				writes := 0
				h.SetFault("before_ingestion_result_lock", func() error {
					writes++
					if writes == 2 {
						change(context.Background())
					}
					return nil
				})
			}
			fields := map[string]any{"contextReceipt": assembled.ContextReceipt, "focus": map[string]any{"claimIds": assembled.Context.Batches[0].ClaimIDs}}
			for k, v := range snapshot {
				fields[k] = v
			}
			w := qualificationRequest(t, f, router, "/api/profile/ingest", "late-profile-change", "", ingestionBody(t, f, "reconcile", fields))
			if !changed || w.Code == 200 || strings.Contains(w.Body.String(), `"operations"`) {
				t.Fatalf("stale transient result released: changed=%v status=%d %s", changed, w.Code, w.Body.String())
			}
			status, e := h.IngestionStatus(context.Background(), f.Principal, "late-profile-change")
			if e != nil || status.ResultAvailable || status.RetryAllowed {
				t.Fatalf("late result status %+v %v", status, e)
			}
			scope, e := h.ScopeStatus(context.Background(), f.Principal, "global")
			if e != nil || scope.Counted <= 0 || scope.Occupied != 0 {
				t.Fatalf("terminal fake retained expense/released occupancy %+v %v", scope, e)
			}
		})
	}
}
func ingestionBoundFixture(t *testing.T, bound func(string, []byte) (int64, bool)) (*aiadmission.Fixture, *aicontrol.Harness, *aiadmission.Provider, http.Handler) {
	f, old, _, _ := qualificationFixture(t)
	_ = old.Close()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	cfg := aicontrol.IngestionSyntheticConfig("qualification", gate, bound)
	cfg.ExpiresAt = time.Now().Add(time.Hour)
	h, e := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	p := &aiadmission.Provider{T: t, Workflow: "profile_ingestion"}
	_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		v, e := p.Respond(ctx, provider, r)
		return v, true, e
	})
	return f, h, p, accountapi.NewSyntheticIngestion(f.Service, f.Store, h, []byte("synthetic-ingestion-signing-key-32bytes"))
}

func TestManagedIngestionCompleteRequestBoundsAndCumulativeTokens(t *testing.T) {
	for _, kind := range []string{"unavailable", "typesafe", "openai", "cumulative"} {
		t.Run(kind, func(t *testing.T) {
			f, _, p, router := ingestionBoundFixture(t, func(provider string, raw []byte) (int64, bool) {
				if len(raw) == 0 {
					t.Error("missing complete request")
				}
				if kind == "unavailable" {
					return 0, false
				}
				if provider == "typesafe" {
					if kind == "typesafe" {
						return 32001, true
					}
					return 32000, true
				}
				if kind == "openai" {
					return 64001, true
				}
				return 64000, true
			})
			extra := map[string]any{}
			if kind == "cumulative" {
				extra["input"] = strings.Repeat("I use Java.\n", 2499)
			}
			planBody := ingestionBody(t, f, "plan", extra)
			w := qualificationRequest(t, f, router, "/api/profile/ingest", "bounds", "", planBody)
			var plan ingestion.SubmissionPlanResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
				t.Fatal(w.Body.String())
			}
			count := 1
			if kind == "cumulative" {
				count = 15
				if len(plan.Plan.Batches) < 15 {
					t.Fatal("fixture needs15plannedbatches")
				}
			}
			for n := 0; n < count; n++ {
				bodyExtra := map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": n}
				if v, ok := extra["input"]; ok {
					bodyExtra["input"] = v
				}
				w = qualificationRequest(t, f, router, "/api/profile/ingest", "bounds", "", ingestionBody(t, f, "extract", bodyExtra))
				if kind == "cumulative" && n < 14 && w.Code != 200 {
					t.Fatalf("batch%d: %d %s", n, w.Code, w.Body.String())
				}
			}
			want := 0
			if kind == "openai" {
				want = 1
			}
			if kind == "cumulative" {
				want = 29
			}
			if w.Code < 400 || len(p.Calls) != want {
				t.Fatalf("%s bound not applied to next complete request: %d sends=%d", kind, w.Code, len(p.Calls))
			}
		})
	}
}

func TestManagedIngestionUnknownIntentRestartNeverResends(t *testing.T) {
	f, h, p, router := ingestionBoundFixture(t, func(provider string, _ []byte) (int64, bool) {
		if provider == "typesafe" {
			return 32000, true
		}
		return 64000, true
	})
	planBody := ingestionBody(t, f, "plan", nil)
	w := qualificationRequest(t, f, router, "/api/profile/ingest", "restart-ingest", "", planBody)
	var plan ingestion.SubmissionPlanResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatal(w.Body.String())
	}
	h.SetFault("after_intent_commit", func() error { return aicontrol.ErrUnavailable })
	extract := ingestionBody(t, f, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
	_ = qualificationRequest(t, f, router, "/api/profile/ingest", "restart-ingest", "", extract)
	_ = h.Close()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("qualification", 1)
	cfg := aicontrol.IngestionSyntheticConfig("qualification", gate, func(provider string, _ []byte) (int64, bool) {
		if provider == "typesafe" {
			return 32000, true
		}
		return 64000, true
	})
	reopened, e := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	_ = reopened.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
		v, e := p.Respond(ctx, provider, r)
		return v, true, e
	})
	router = accountapi.NewSyntheticIngestion(f.Service, f.Store, reopened, []byte("synthetic-ingestion-signing-key-32bytes"))
	for _, retry := range []string{"", "deliberate-unknown-retry"} {
		w = qualificationRequest(t, f, router, "/api/profile/ingest", "restart-ingest", retry, extract)
		if w.Code == 200 || len(p.Calls) != 0 {
			t.Fatalf("unknown resent %d %+v", w.Code, p.Calls)
		}
	}
	status, e := reopened.IngestionStatus(context.Background(), f.Principal, "restart-ingest")
	if e != nil || status.State != "unknown" || status.RetryAllowed {
		t.Fatalf("unknown status %+v %v", status, e)
	}
	scope, e := reopened.ScopeStatus(context.Background(), f.Principal, "global")
	if e != nil || scope.Counted != 1344 || scope.Occupied != 1 {
		t.Fatalf("restart lost hold %+v %v", scope, e)
	}
}

func TestManagedIngestionWholeSubmissionUsesOneActionAndNoResend(t *testing.T) {
	f, h, p, router := ingestionFixture(t)
	planBody := ingestionBody(t, f, "plan", nil)
	w := qualificationRequest(t, f, router, "/api/profile/ingest", "one-submission", "", planBody)
	var plan ingestion.SubmissionPlanResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil || len(plan.Plan.Batches) != 1 || len(p.Calls) != 0 {
		t.Fatalf("provider-free plan: %d %s %+v", w.Code, w.Body.String(), p.Calls)
	}
	extractBody := ingestionBody(t, f, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "one-submission", "", extractBody)
	var extracted ingestion.SubmissionExtractionResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &extracted) != nil || len(p.Calls) != 2 {
		t.Fatalf("actual whole-original extract: %d %s %+v", w.Code, w.Body.String(), p.Calls)
	}
	for _, path := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion", "/.netlify/functions/account/api/profile/ingest"} {
		w = qualificationRequest(t, f, router, path, "one-submission", "", extractBody)
		if w.Code != 202 || len(p.Calls) != 2 {
			t.Fatalf("lost reply/alias resend: %s %d %s %+v", path, w.Code, w.Body.String(), p.Calls)
		}
	}
	assemblyBody := ingestionBody(t, f, "assemble", map[string]any{"planReceipt": plan.PlanReceipt, "batchReceipts": []string{extracted.BatchReceipt}})
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "one-submission", "", assemblyBody)
	var assembled ingestion.SubmissionAssemblyResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &assembled) != nil || len(p.Calls) != 2 {
		t.Fatalf("provider-free assembly: %d %s", w.Code, w.Body.String())
	}
	ids := assembled.Context.Batches[0].ClaimIDs
	reconcileBody := ingestionBody(t, f, "reconcile", map[string]any{"contextReceipt": assembled.ContextReceipt, "focus": map[string]any{"claimIds": ids}})
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "one-submission", "", reconcileBody)
	if w.Code != 200 || len(p.Calls) != 4 {
		t.Fatalf("whole-original reconciliation: %d %s %+v", w.Code, w.Body.String(), p.Calls)
	}
	// Clarification is a separately explicit foreground operation, not a reset
	// of this submission's finished 300-second processing clock. The original
	// signed context remains within its unchanged real ten-minute expiry.
	if _, e := f.DB.Exec(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '301 seconds' WHERE stage LIKE 'managed_request_%'`); e != nil {
		t.Fatal(e)
	}
	var selected map[string]any
	rawClaim, _ := json.Marshal(extracted.Claims[0])
	_ = json.Unmarshal(rawClaim, &selected)
	selected["id"] = ids[0]
	privateAnswer := "Synthetic-private-answer-DO-NOT-LEDGER. I use Java professionally."
	for n, answer := range []string{privateAnswer, "I also use Java for projects.", "Java remains in use.", "I use Java at work."} {
		clarifyBody := ingestionBody(t, f, "clarify", map[string]any{"contextReceipt": assembled.ContextReceipt, "claim": selected, "answer": answer})
		before := len(p.Calls)
		w = qualificationRequest(t, f, router, "/api/profile/ingest", "one-submission", "", clarifyBody)
		if w.Code != 200 || len(p.Calls) != before+4 {
			t.Fatalf("clarification %d whole-original+answer gates: %d %s %+v", n, w.Code, w.Body.String(), p.Calls)
		}
		w = qualificationRequest(t, f, router, "/api/profile/ingest", "one-submission", "", clarifyBody)
		if w.Code != 202 || len(p.Calls) != before+4 {
			t.Fatal("clarification replay resent")
		}
	}
	before := len(p.Calls)
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "one-submission", "", ingestionBody(t, f, "clarify", map[string]any{"contextReceipt": assembled.ContextReceipt, "claim": selected, "answer": "Fifth deliberate answer."}))
	if w.Code != 429 || len(p.Calls) != before {
		t.Fatalf("fifth clarification not bounded: %d %s", w.Code, w.Body.String())
	}
	for _, table := range []string{"ai_fixture_action", "ai_fixture_stage", "ai_fixture_child", "ai_fixture_dispatch", "ai_fixture_observation", "ai_fixture_evidence"} {
		var raw string
		if e := f.DB.QueryRow("SELECT COALESCE(jsonb_agg(to_jsonb(t))::text,'[]') FROM " + table + " t").Scan(&raw); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(raw, privateAnswer) || strings.Contains(raw, "I use Java.") {
			t.Fatalf("career content entered %s", table)
		}
	}
	status, e := h.IngestionStatus(context.Background(), f.Principal, "one-submission")
	if e != nil || status.ResultAvailable {
		t.Fatalf("honest non-content status %+v %v", status, e)
	}
	pilot, e := f.Store.PilotStatus(context.Background(), f.Principal)
	if e != nil || pilot.DraftAllowance != 5 {
		t.Fatalf("Draft rights changed %+v %v", pilot, e)
	}
}

func TestManagedIngestionFailedClarificationChildAndDeliberateRetry(t *testing.T) {
	for _, mode := range []string{"changed_answer", "explicit_retry"} {
		t.Run(mode, func(t *testing.T) {
			f, h, p, router := ingestionFixture(t)
			key := "clarification-failure"
			post := func(kind, retry string, extra map[string]any) *httptest.ResponseRecorder {
				return qualificationRequest(t, f, router, "/api/profile/ingest", key, retry, ingestionBody(t, f, kind, extra))
			}
			var plan ingestion.SubmissionPlanResponse
			w := post("plan", "", nil)
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
				t.Fatal(w.Body.String())
			}
			var extracted ingestion.SubmissionExtractionResponse
			w = post("extract", "", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &extracted) != nil {
				t.Fatal(w.Body.String())
			}
			var assembled ingestion.SubmissionAssemblyResponse
			w = post("assemble", "", map[string]any{"planReceipt": plan.PlanReceipt, "batchReceipts": []string{extracted.BatchReceipt}})
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &assembled) != nil {
				t.Fatal(w.Body.String())
			}
			ids := assembled.Context.Batches[0].ClaimIDs
			w = post("reconcile", "", map[string]any{"contextReceipt": assembled.ContextReceipt, "focus": map[string]any{"claimIds": ids}})
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			var claim map[string]any
			raw, _ := json.Marshal(extracted.Claims[0])
			_ = json.Unmarshal(raw, &claim)
			claim["id"] = ids[0]
			body := func(answer string) map[string]any {
				return map[string]any{"contextReceipt": assembled.ContextReceipt, "claim": claim, "answer": answer}
			}
			calls := 0
			_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
				calls++
				if calls == 1 {
					v := httptest.NewRecorder()
					v.WriteHeader(429)
					_, _ = v.WriteString(`{"error":"synthetic_known_failure"}`)
					return v.Result(), true, nil
				}
				v, e := p.Respond(ctx, provider, r)
				return v, true, e
			})
			w = post("clarify", "", body("I use Java at work."))
			if w.Code < 400 || calls != 1 || w.Header().Get("X-CareerOS-Retry-Allowed") != "true" {
				t.Fatalf("known failed child: %d %s calls=%d", w.Code, w.Body.String(), calls)
			}
			before, e := h.IngestionStatus(context.Background(), f.Principal, key)
			if e != nil || before.Attempt != 1 || before.State != "known_failure" || !before.RetryAllowed {
				t.Fatalf("failed child status %+v %v", before, e)
			}
			w = post("clarify", "", body("I use Java at work."))
			if w.Code != 202 || calls != 1 {
				t.Fatalf("failed unchanged answer resent: %d calls=%d", w.Code, calls)
			}
			if mode == "changed_answer" {
				for n, answer := range []string{"I use Java for projects.", "I still use Java.", "Java is part of my work."} {
					w = post("clarify", "", body(answer))
					if w.Code != 200 || calls != 1+4*(n+1) {
						t.Fatalf("changed child %d: %d %s calls=%d", n, w.Code, w.Body.String(), calls)
					}
				}
				w = post("clarify", "", body("Fifth distinct answer."))
				if w.Code != 429 || calls != 13 {
					t.Fatalf("failed child did not count toward four: %d calls=%d", w.Code, calls)
				}
			} else {
				w = post("clarify", "stable-deliberate-retry", body("I use Java at work."))
				if w.Code != 200 || calls != 5 {
					t.Fatalf("explicit unchanged Retry: %d %s calls=%d", w.Code, w.Body.String(), calls)
				}
				w = post("clarify", "stable-deliberate-retry", body("I use Java at work."))
				if w.Code != 202 || calls != 5 {
					t.Fatalf("Retry response loss resent: %d %s", w.Code, w.Body.String())
				}
				w = post("clarify", "another-retry", body("I use Java at work."))
				if w.Code < 400 || calls != 5 {
					t.Fatalf("new Retry key allocated more rights: %d", w.Code)
				}
			}
			after, e := h.IngestionStatus(context.Background(), f.Principal, key)
			want := 1
			if mode == "explicit_retry" {
				want = 2
			}
			if e != nil || after.Attempt != want || after.ActionID != before.ActionID {
				t.Fatalf("financial binding changed %+v %+v %v", before, after, e)
			}
		})
	}
}

func TestManagedIngestionCredentialEnvelopesFailBeforeSend(t *testing.T) {
	f, _, p, router := ingestionFixture(t)
	body := ingestionBody(t, f, "plan", nil)
	for _, key := range []string{"apiKey", "typeSafeApiKey", "typesafeApiKey", "openaiApiKey", "openAIKey", "headers", "authorization", "attempt", "dispatchId"} {
		var fields map[string]any
		_ = json.Unmarshal(body, &fields)
		fields[key] = "caller-claimed-authority"
		raw, _ := json.Marshal(fields)
		w := qualificationRequest(t, f, router, "/api/profile/ingest", "body-credentials", "", raw)
		if w.Code != 400 || len(p.Calls) != 0 {
			t.Fatalf("envelope %s: %d sends=%d", key, w.Code, len(p.Calls))
		}
	}
}

func TestManagedIngestionAuthorityDenials(t *testing.T) {
	for _, kind := range []string{"stale_profile", "expired", "suspended", "revoked", "session", "wrong_account", "missing_session", "caller_credentials"} {
		t.Run(kind, func(t *testing.T) {
			f, _, p, router := ingestionFixture(t)
			body := ingestionBody(t, f, "plan", nil)
			if kind == "stale_profile" {
				body = ingestionBody(t, f, "plan", map[string]any{"profile": map[string]any{"skills": "Changed"}})
			}
			switch kind {
			case "expired":
				_, _ = f.DB.Exec(`UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id=$1`, f.Principal.AccountID)
			case "suspended", "revoked":
				_, _ = f.DB.Exec(`UPDATE careeros_pilot_access SET state=$2 WHERE account_id=$1`, f.Principal.AccountID, kind)
			case "session":
				_, _ = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.Principal.SessionID)
			}
			for _, path := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion", "/.netlify/functions/account/api/profile/ingest"} {
				r := f.Request(t, context.Background(), path, body)
				r.Header.Del("X-OpenAI-Api-Key")
				r.Header.Del("X-TypeSafe-Api-Key")
				r.Header.Set("X-CareerOS-Action-Key", "denied")
				switch kind {
				case "wrong_account":
					r.Header.Set("X-CareerOS-Account", "another-account")
				case "missing_session":
					r.Header.Del("Cookie")
				case "caller_credentials":
					r.Header.Set("Authorization", "caller")
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code < 400 || len(p.Calls) != 0 {
					t.Fatalf("%s %s: %d %s", kind, path, w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestManagedIngestionSnapshotComparedAfterAccountLock(t *testing.T) {
	f, _, p, router := ingestionFixture(t)
	body := ingestionBody(t, f, "plan", map[string]any{"document": json.RawMessage(f.Document)})
	blocker, e := f.DB.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback()
	if _, e = blocker.Exec(`SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, f.Principal.AccountID); e != nil {
		t.Fatal(e)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- qualificationRequest(t, f, router, "/api/profile/ingest", "stale-after-lock", "", body)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if e = f.DB.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request never blocked on account authority")
		}
		time.Sleep(5 * time.Millisecond)
	}
	var doc map[string]any
	_ = json.Unmarshal(f.Document, &doc)
	fact := doc["facts"].([]any)[0].(map[string]any)
	fact["value"] = "TypeScript"
	normalized, _ := json.Marshal(profiledocument.NormalizeTerm("TypeScript"))
	var normalization any
	_ = json.Unmarshal(normalized, &normalization)
	fact["normalization"] = normalization
	changed, _ := json.Marshal(doc)
	if _, e = profiledocument.Decode(changed); e != nil {
		t.Fatal(e)
	}
	if _, e = blocker.Exec(`UPDATE careeros_profiles SET document=$2 WHERE account_id=$1`, f.Principal.AccountID, changed); e != nil {
		t.Fatal(e)
	}
	if e = blocker.Commit(); e != nil {
		t.Fatal(e)
	}
	select {
	case w := <-done:
		if w.Code != 409 || len(p.Calls) != 0 {
			t.Fatalf("stale snapshot admitted after lock: %d %s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
	var count int
	if e = f.DB.QueryRow(`SELECT count(*) FROM ai_fixture_stage WHERE stage LIKE 'managed_request_%'`).Scan(&count); e != nil || count != 0 {
		t.Fatalf("stale snapshot created stages: %d %v", count, e)
	}
}

func TestManagedIngestionStageCountsIncludeFailedClaims(t *testing.T) {
	for _, kind := range []string{"extract", "reconcile"} {
		t.Run(kind, func(t *testing.T) {
			f, h, p, _ := ingestionFixture(t)
			body := ingestionBody(t, f, "plan", nil)
			root, _, _, _ := ingestion.ManagedEnvelope(body)
			s, e := h.Open(context.Background(), f.Principal, "stages", "profile_ingestion", root)
			if e != nil {
				t.Fatal(e)
			}
			for n := 0; n < 21; n++ {
				extra := map[string]any{"planReceipt": "synthetic-invalid-receipt", "batchIndex": n}
				if kind == "reconcile" {
					extra = map[string]any{"contextReceipt": "synthetic-invalid-context", "focus": map[string]any{"claimIds": []string{string(rune('a' + n))}}}
				}
				raw := ingestionBody(t, f, kind, extra)
				_, coordinate, stage, ok := ingestion.ManagedEnvelope(raw)
				if !ok {
					t.Fatal("fixture stage")
				}
				_, e = s.BeginIngestionStage(context.Background(), coordinate, stage, raw)
				if n < 20 && e != nil {
					t.Fatalf("claim %d: %v", n, e)
				}
				if n == 20 && e != aicontrol.ErrCapacity {
					t.Fatalf("21st claim %v", e)
				}
			}
			if len(p.Calls) != 0 {
				t.Fatal("stage bookkeeping sent a provider")
			}
		})
	}
}

func TestManagedIngestionReceiptAuthorityAndCapacityAreClosed(t *testing.T) {
	f, _, p, router := ingestionFixture(t)
	planRaw := ingestionBody(t, f, "plan", nil)
	w := qualificationRequest(t, f, router, "/api/profile/ingest", "source-action", "", planRaw)
	var plan ingestion.SubmissionPlanResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
		t.Fatal(w.Body.String())
	}
	duplicate := qualificationRequest(t, f, router, "/api/profile/ingest", "source-action", "", planRaw)
	if duplicate.Code != 200 || duplicate.Body.String() != w.Body.String() {
		t.Fatal("provider-free replay extended or changed receipt")
	}
	// An otherwise valid signed handoff is not transferable to another Action,
	// even with identical original/Profile/document values.
	borrow := ingestionBody(t, f, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "other-action", "", borrow)
	if w.Code < 400 || len(p.Calls) != 0 {
		t.Fatalf("mixed Action receipt sent: %d %s", w.Code, w.Body.String())
	}
	// A second valid signed-in, cleared account cannot read the first account's
	// status or redeem its receipt through any alias, even with the same Profile.
	second := qualificationParticipant(t, f, "receipt-second-account")
	statusRequest := second.Request(t, context.Background(), "/api/ai/ingestion/actions/source-action", nil)
	statusRequest.Method = http.MethodGet
	statusReply := httptest.NewRecorder()
	router.ServeHTTP(statusReply, statusRequest)
	if statusReply.Code < 400 {
		t.Fatalf("foreign account read status: %d %s", statusReply.Code, statusReply.Body.String())
	}
	for n, path := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion", "/.netlify/functions/account/api/profile/ingest"} {
		foreign := ingestionBody(t, second, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
		w = qualificationRequest(t, second, router, path, "foreign-receipt-"+string(rune('a'+n)), "", foreign)
		if w.Code < 400 || len(p.Calls) != 0 {
			t.Fatalf("foreign receipt %s: %d sends=%d", path, w.Code, len(p.Calls))
		}
	}
	for _, path := range []string{"/api/profile/ingest", "/.netlify/functions/profile-ingestion", "/.netlify/functions/account/api/profile/ingest"} {
		w = qualificationRequest(t, f, router, path, "legacy-closed", "", f.Body(t, "/api/profile/ingest"))
		if w.Code < 400 || len(p.Calls) != 0 {
			t.Fatalf("legacy bypass %s %d", path, w.Code)
		}
	}
	var expanded map[string]any
	_ = json.Unmarshal(planRaw, &expanded)
	expanded["input"] = strings.Repeat("\u0344x", 10000)
	body, _ := json.Marshal(expanded)
	w = qualificationRequest(t, f, router, "/api/profile/ingest", "expanded-plan", "", body)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "capacity") || len(p.Calls) != 0 {
		t.Fatalf("over20 extraction plan not explicit capacity: %d %s", w.Code, w.Body.String())
	}
}

func TestManagedIngestionForegroundDeadlineCannotBeReopened(t *testing.T) {
	for _, kind := range []string{"claimed_without_dispatch", "after_blocking_claim", "next_leg"} {
		t.Run(kind, func(t *testing.T) {
			f, h, p, router := ingestionFixture(t)
			planBody := ingestionBody(t, f, "plan", nil)
			w := qualificationRequest(t, f, router, "/api/profile/ingest", "deadline", "", planBody)
			var plan ingestion.SubmissionPlanResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
				t.Fatal(w.Body.String())
			}
			extractBody := ingestionBody(t, f, "extract", map[string]any{"planReceipt": plan.PlanReceipt, "batchIndex": 0})
			if kind == "claimed_without_dispatch" {
				root, coordinate, stage, ok := ingestion.ManagedEnvelope(extractBody)
				if !ok {
					t.Fatal("fixture envelope")
				}
				s, e := h.Open(context.Background(), f.Principal, "deadline", "profile_ingestion", root)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = s.BeginIngestionStage(context.Background(), coordinate, stage, extractBody); e != nil {
					t.Fatal(e)
				}
				if _, e = f.DB.Exec(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '301 seconds' WHERE stage LIKE 'managed_request_%'`); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "after_blocking_claim" {
				if _, e := f.DB.Exec(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '299 seconds' WHERE stage LIKE 'managed_request_%'`); e != nil {
					t.Fatal(e)
				}
				h.SetFault("before_claim", func() error { time.Sleep(1100 * time.Millisecond); return nil })
			}
			if kind == "next_leg" {
				_ = h.SetQualificationResponder(func(ctx context.Context, provider string, r *http.Request) (*http.Response, bool, error) {
					resp, e := p.Respond(ctx, provider, r)
					if provider == "typesafe" {
						if _, err := f.DB.Exec(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '301 seconds' WHERE stage LIKE 'managed_request_%'`); err != nil {
							t.Error(err)
						}
					}
					return resp, true, e
				})
			}
			w = qualificationRequest(t, f, router, "/api/profile/ingest", "deadline", "", extractBody)
			want := 0
			if kind == "next_leg" {
				want = 1
			}
			if w.Code < 400 || len(p.Calls) != want {
				t.Fatalf("%s crossed foreground deadline: %d sends=%+v", kind, w.Code, p.Calls)
			}
		})
	}
}

func ingestionBody(t *testing.T, f *aiadmission.Fixture, kind string, extra map[string]any) []byte {
	t.Helper()
	var body map[string]any
	_ = json.Unmarshal(f.Body(t, "/api/profile/ingest"), &body)
	body["stage"] = map[string]any{"version": 2, "kind": kind}
	for k, v := range extra {
		body[k] = v
	}
	raw, e := json.Marshal(body)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
