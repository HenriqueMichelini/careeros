package profileingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type admissionIngestion struct {
	fixture *aiadmission.Fixture
	harness *aicontrol.Harness
	handler http.Handler
	action  string
	session *aicontrol.Session
	child   *aicontrol.Session
}

// The real signed ingestion handler and both provider adapters run behind real
// opaque-session checks and PostgreSQL authority. The responder is an existing
// package-local roundTripFunc fixture, never an HTTP/network client.
func admittedIngestion(t *testing.T, a app, configure func(*aicontrol.SyntheticConfig)) *admissionIngestion {
	t.Helper()
	f := aiadmission.NewFixture(t)
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("ingestion", 1)
	cfg := aicontrol.DefaultSyntheticConfig("ingestion", gate)
	cfg.Limit, cfg.Capacity, cfg.MaxDispatches, cfg.MaxStages = 1000000, 128, 128, 128
	if configure != nil {
		configure(&cfg)
	}
	h, err := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	if err := h.SyntheticReviewer().SeedGrant(context.Background(), f.Principal, time.Now().Add(time.Hour), cfg.CustomerUnits); err != nil {
		t.Fatal(err)
	}
	fakeTransport, ok := a.client.Transport.(roundTripFunc)
	if !ok {
		t.Fatal("fixture responder is not the explicit in-memory adapter")
	}
	h.SetResponder(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
		return fakeTransport.RoundTrip(r)
	})
	a.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("managed ingestion fell back to supplied HTTP client")
		return nil, fmt.Errorf("forbidden fallback")
	})}
	result := &admissionIngestion{fixture: f, harness: h}
	controlled := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFromRequest(r)
		if !ok {
			t.Fatal("missing verified opaque-session principal")
		}
		var in request
		raw, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(raw, &in) != nil {
			t.Fatal(err)
		}
		// One Action spans the protocol's stage requests. Child handoff, answer
		// and exact provider inputs bind separately before their first send.
		root := mustJSON(request{Input: in.Input, Profile: in.Profile, Document: in.Document})
		session, err := h.Open(r.Context(), p, "explicit-synthetic-command", "profile_ingestion", root)
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		result.action = session.ID()
		result.session = session
		if result.child != nil {
			session = result.child
		}
		// Restore the exact request bytes for the real handler; the local
		// harness never persists this content or sends it to a network.
		restored := f.Request(t, aidispatch.WithController(r.Context(), session), "/api/profile/ingest", raw)
		restored.Header = r.Header.Clone()
		r = restored
		a.handler().ServeHTTP(w, r)
	})
	result.handler = accountapi.ProtectWorkflow(f.Service, f.Store, "/api/profile/ingest", controlled)
	return result
}

func (a *admissionIngestion) stageCount(t *testing.T, stage string) int {
	t.Helper()
	var count int
	if err := a.fixture.DB.QueryRow(`SELECT count(*) FROM ai_fixture_stage WHERE namespace='ingestion' AND action=$1 AND stage=$2`, a.action, stage).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (a *admissionIngestion) send(t *testing.T, in request) *httptest.ResponseRecorder {
	t.Helper()
	r := a.fixture.Request(t, context.Background(), "/api/profile/ingest", mustJSON(in))
	w := httptest.NewRecorder()
	a.handler.ServeHTTP(w, r)
	return w
}

func (a *admissionIngestion) dispatchCount(t *testing.T) int {
	t.Helper()
	status, err := a.harness.Status(context.Background(), a.fixture.Principal, a.action)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range status.Dispatches {
		if d.Maximum != 60 || d.Counted != 60 || d.IntentAt == nil || d.Remote != "possibly_active" {
			t.Fatal("reached ingestion leg lost conservative financial/capacity exposure")
		}
	}
	return len(status.Dispatches)
}

func TestA26_SubmissionStagesReplayBindBeforeEveryActualAdapter(t *testing.T) {
	t.Run("provider-free plan and assemble, extract duplicate and changed handoff", func(t *testing.T) {
		calls := &submissionCalls{}
		a := submissionApp(calls, `{"claims":[{"id":"c1","source":"I use Java.","text":"Uses Java","targets":["skills"],"question":""}]}`)
		controlled := admittedIngestion(t, a, nil)
		in := request{Input: "I use Java.", Profile: profile(), Stage: &stageRequest{SubmissionStageVersion, SubmissionStagePlan}}
		w := controlled.send(t, in)
		var plan SubmissionPlanResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil || calls.jev+calls.extract != 0 || controlled.dispatchCount(t) != 0 {
			t.Fatalf("provider-free plan: status=%d body=%s", w.Code, w.Body.String())
		}
		if controlled.stageCount(t, "provider_free_plan") != 1 {
			t.Fatal("provider-free plan has no durable stage binding")
		}
		replayedPlan := controlled.send(t, in)
		if replayedPlan.Code != 200 || replayedPlan.Body.String() != w.Body.String() || controlled.stageCount(t, "provider_free_plan") != 1 {
			t.Fatal("provider-free plan replay did not preserve its original receipt and allocation")
		}
		extract := extractSubmission(in, plan, 0)
		w = controlled.send(t, extract)
		var extracted SubmissionExtractionResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &extracted) != nil || calls.jev != 1 || calls.extract != 1 || controlled.dispatchCount(t) != 2 {
			t.Fatalf("extract status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
		}
		w = controlled.send(t, extract)
		if w.Code < 400 || calls.jev != 1 || calls.extract != 1 || controlled.dispatchCount(t) != 2 {
			t.Fatal("duplicate batch regenerated output")
		}
		// Credentials are headers, not immutable career input or fingerprint
		// material. Changing only either credential cannot mint a new dispatch
		// or conflict with the existing root binding.
		rotated := controlled.fixture.Request(t, context.Background(), "/api/profile/ingest", mustJSON(extract))
		rotated.Header.Set("X-OpenAI-Api-Key", "sk-rotated-synthetic-sentinel")
		rotated.Header.Set("X-TypeSafe-Api-Key", "rotated-typesafe-synthetic-sentinel")
		w = httptest.NewRecorder()
		controlled.handler.ServeHTTP(w, rotated)
		if w.Code == http.StatusConflict || w.Code < 400 || calls.jev+calls.extract != 2 || controlled.dispatchCount(t) != 2 {
			t.Fatal("credential rotation influenced immutable root identity or caused redispatch")
		}
		tampered := extract
		tampered.PlanReceipt = tamperReceipt(tampered.PlanReceipt)
		if w = controlled.send(t, tampered); w.Code < 400 || calls.jev+calls.extract != 2 {
			t.Fatal("changed signed handoff reached a provider")
		}
		w = controlled.send(t, assembleSubmission(in, plan, []string{extracted.BatchReceipt}))
		var assembled SubmissionAssemblyResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &assembled) != nil || assembled.ContextReceipt == "" || calls.jev+calls.extract != 2 || controlled.dispatchCount(t) != 2 {
			t.Fatalf("provider-free assemble: status=%d body=%s", w.Code, w.Body.String())
		}
		if controlled.stageCount(t, "provider_free_assemble") != 1 {
			t.Fatal("provider-free assembly has no durable stage binding")
		}
		replayedAssembly := controlled.send(t, assembleSubmission(in, plan, []string{extracted.BatchReceipt}))
		if replayedAssembly.Code != 200 || replayedAssembly.Body.String() != w.Body.String() || controlled.stageCount(t, "provider_free_assemble") != 1 || controlled.dispatchCount(t) != 2 {
			t.Fatal("provider-free assembly replay changed its handoff or spent again")
		}
		var changedBatch submissionBatchReceiptPayload
		if code := a.openSubmission(submissionBatchDomain, extracted.BatchReceipt, &changedBatch); code != "" {
			t.Fatal(code)
		}
		changedBatch.ExpiresAt++
		changedReceipt, code := a.sealSubmission(submissionBatchDomain, mustJSON(changedBatch))
		if code != "" {
			t.Fatal(code)
		}
		changedAssembly := controlled.send(t, assembleSubmission(in, plan, []string{changedReceipt}))
		if changedAssembly.Code != http.StatusConflict || controlled.stageCount(t, "provider_free_assemble") != 1 || controlled.dispatchCount(t) != 2 {
			t.Fatalf("changed authenticated assembly input escaped immutable binding: %d %s", changedAssembly.Code, changedAssembly.Body.String())
		}
	})
	t.Run("focus replay and changed authenticated child", func(t *testing.T) {
		calls := &submissionCalls{}
		in := request{Input: "I use Java.", Profile: profile()}
		selected := fixtureClaim("c1", "I use Java.", "skills")
		id := reconcileGID(0, "c1")
		a := reconcileApp(calls, "professional_fact", proposalJSON([]operation{reconcileOp(id, "skills", "", "skills", "add", "Java", "addition")}, []claimOutcome{reconcileOutcome(id, "change", "new_skill", nil)}))
		receipt := sealContext(t, a, in, [][]claim{{selected}})
		controlled := admittedIngestion(t, a, nil)
		focus := reconcileRequest(in, receipt, id)
		if w := controlled.send(t, focus); w.Code != 200 || calls.jev != 1 || calls.extract != 1 || controlled.dispatchCount(t) != 2 {
			t.Fatalf("focus: status=%d body=%s", w.Code, w.Body.String())
		}
		if w := controlled.send(t, focus); w.Code < 400 || calls.jev+calls.extract != 2 {
			t.Fatal("duplicate focus regenerated")
		}
		focus.ContextReceipt = tamperReceipt(focus.ContextReceipt)
		if w := controlled.send(t, focus); w.Code < 400 || calls.jev+calls.extract != 2 {
			t.Fatal("changed focus handoff reached provider")
		}
	})
	t.Run("clarification original and answer are independent bounded dispatches", func(t *testing.T) {
		calls := &submissionCalls{}
		in := request{Input: "Account owner unclear. Uses Java.", Profile: profile()}
		local := fixtureClaim("c1", "Account owner unclear.", "skills")
		selected := local
		selected.ID = reconcileGID(0, "c1")
		revised := claim{ID: "provider-id", Source: "I used Java at Acme.", Text: "I used Java at Acme", Targets: []string{"skills"}, Question: ""}
		a := clarifyApp(calls, "none", string(mustJSON(extraction{Claims: []claim{revised}})), proposalJSON([]operation{reconcileOp(selected.ID, "skills", "", "skills", "add", "Java", "addition")}, []claimOutcome{reconcileOutcome(selected.ID, "change", "new_skill", nil)}))
		receipt := sealContext(t, a, in, [][]claim{{local}})
		controlled := admittedIngestion(t, a, nil)
		clarify := clarifyRequest(in, receipt, selected, "I used Java at Acme.")
		if w := controlled.send(t, clarify); w.Code != 200 || calls.jev != 2 || calls.extract != 2 || controlled.dispatchCount(t) != 4 {
			t.Fatalf("clarify: status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
		}
		if w := controlled.send(t, clarify); w.Code < 400 || calls.jev+calls.extract != 4 {
			t.Fatal("duplicate clarification regenerated")
		}
		changedAnswer := "I used Java at Acme. I used Go there too."
		clarify.Answer = &changedAnswer
		if w := controlled.send(t, clarify); w.Code < 400 || calls.jev+calls.extract != 4 || controlled.dispatchCount(t) != 4 {
			t.Fatal("changed clarification answer escaped immutable child binding")
		}
		parent := dispatchCoordinate(clarify)
		child, err := controlled.session.NewClarification(context.Background(), parent, "")
		if err != nil {
			t.Fatal("explicit bounded clarification child was denied", err)
		}
		replayedChild, err := controlled.session.NewClarification(context.Background(), parent, "")
		if err != nil || child.ClarificationRevision() == "" || replayedChild.ClarificationRevision() != child.ClarificationRevision() {
			t.Fatal("replayed child creation minted another revision", err)
		}
		controlled.child = child
		// The answer is accepted only under the explicitly created server child.
		// Its revision is Go authority, never a caller-selected request field.
		if w := controlled.send(t, clarify); w.Code != 200 || calls.jev+calls.extract != 8 || controlled.dispatchCount(t) != 8 {
			t.Fatalf("explicit clarification child: status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
		}
		if w := controlled.send(t, clarify); w.Code < 400 || calls.jev+calls.extract != 8 || controlled.dispatchCount(t) != 8 {
			t.Fatal("completed clarification child regenerated")
		}
	})
}

func TestA27_LegacyV1V2AndWholeActionCaps(t *testing.T) {
	t.Run("provider-free planning consumes a finite stage allocation", func(t *testing.T) {
		calls := &submissionCalls{}
		a := submissionApp(calls, `{"claims":[]}`)
		controlled := admittedIngestion(t, a, func(cfg *aicontrol.SyntheticConfig) { cfg.MaxStages = 1 })
		in := request{Input: "I use Java.", Profile: profile(), Stage: &stageRequest{SubmissionStageVersion, SubmissionStagePlan}}
		w := controlled.send(t, in)
		var plan SubmissionPlanResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil || controlled.stageCount(t, "provider_free_plan") != 1 {
			t.Fatalf("provider-free allocation: %d %s", w.Code, w.Body.String())
		}
		if replay := controlled.send(t, in); replay.Code != 200 || replay.Body.String() != w.Body.String() || controlled.stageCount(t, "provider_free_plan") != 1 {
			t.Fatal("plan replay consumed another finite allocation")
		}
		if w = controlled.send(t, extractSubmission(in, plan, 0)); w.Code < 400 || calls.jev+calls.extract != 0 || controlled.dispatchCount(t) != 0 {
			t.Fatalf("provider-free stage was omitted from whole-Action bound: %d %s", w.Code, w.Body.String())
		}
	})
	for _, version := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("protocol-v%d", version), func(t *testing.T) {
			var calls [3]int
			a := stagedApp(t, &calls)
			controlled := admittedIngestion(t, a, nil)
			in := request{Input: "I use Java.", Profile: profile()}
			if version == 0 {
				if w := controlled.send(t, in); w.Code != 200 || calls != [3]int{1, 1, 1} || controlled.dispatchCount(t) != 3 {
					t.Fatalf("legacy: status=%d calls=%v body=%s", w.Code, calls, w.Body.String())
				}
				return
			}
			if version == 1 {
				in.Stage = &stageRequest{1, "extract"}
				w := controlled.send(t, in)
				var extract struct{ Receipt string }
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &extract) != nil || calls != [3]int{1, 1, 0} {
					t.Fatalf("v1 extract: status=%d body=%s", w.Code, w.Body.String())
				}
				in.Stage, in.Receipt = &stageRequest{1, "reconcile"}, extract.Receipt
				if w = controlled.send(t, in); w.Code != 200 || calls != [3]int{2, 1, 1} || controlled.dispatchCount(t) != 4 {
					t.Fatalf("v1 reconcile: status=%d calls=%v body=%s", w.Code, calls, w.Body.String())
				}
				return
			}
			in.Stage = &stageRequest{2, SubmissionStagePlan}
			w := controlled.send(t, in)
			var plan SubmissionPlanResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil || calls != [3]int{} {
				t.Fatal("v2 plan must remain provider-free")
			}
			if w = controlled.send(t, extractSubmission(in, plan, 0)); w.Code != 200 || calls != [3]int{1, 1, 0} || controlled.dispatchCount(t) != 2 {
				t.Fatalf("v2 extract: status=%d calls=%v body=%s", w.Code, calls, w.Body.String())
			}
		})
	}
	t.Run("last valid planned batch cannot bypass exhausted Action cap", func(t *testing.T) {
		calls := &submissionCalls{}
		a := submissionApp(calls, `{"claims":[]}`)
		controlled := admittedIngestion(t, a, func(cfg *aicontrol.SyntheticConfig) { cfg.MaxDispatches = 2 })
		in := request{Input: syntheticInput(3200), Profile: profile(), Stage: &stageRequest{2, SubmissionStagePlan}}
		w := controlled.send(t, in)
		var plan SubmissionPlanResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil || len(plan.Plan.Batches) < 2 {
			t.Fatal("missing multi-batch positive control")
		}
		last := len(plan.Plan.Batches) - 1
		if w = controlled.send(t, extractSubmission(in, plan, last)); w.Code != 200 || calls.jev != 1 || calls.extract != 1 || controlled.dispatchCount(t) != 2 {
			t.Fatalf("last batch: status=%d body=%s", w.Code, w.Body.String())
		}
		if w = controlled.send(t, extractSubmission(in, plan, 0)); w.Code < 400 || calls.jev+calls.extract != 2 || controlled.dispatchCount(t) != 2 {
			t.Fatal("new valid batch bypassed whole-Action cap using existing handoff")
		}
	})
	t.Run("maximum input and minimum legacy portion share the Action cap", func(t *testing.T) {
		calls := &submissionCalls{}
		a := submissionApp(calls, `{"claims":[]}`)
		controlled := admittedIngestion(t, a, func(cfg *aicontrol.SyntheticConfig) { cfg.MaxDispatches = 2 })
		in := request{Input: syntheticInput(6000), Profile: profile()}
		prepared, err := preprocessing.PrepareBoundedWithContext(context.Background(), in.Input, fieldvalidation.ProfessionalInformation)
		if err != nil || len(in.Input) != maxInput {
			t.Fatal("maximum original input fixture is invalid")
		}
		plan, err := planContinuation(prepared.Source, portionRequest{Index: 0, Bytes: 200})
		if err != nil || len(plan.Portions) < 2 || len(plan.Portions) > SubmissionPortionCap {
			t.Fatal("maximum-input/minimum-portion plan fixture is invalid")
		}
		in.Portion = &portionRequest{Index: len(plan.Portions) - 1, Bytes: 200}
		if w := controlled.send(t, in); w.Code != 200 || calls.jev+calls.extract != 2 || controlled.dispatchCount(t) != 2 {
			t.Fatalf("last legacy portion status=%d body=%s", w.Code, w.Body.String())
		}
		in.Portion = &portionRequest{Index: 0, Bytes: 200}
		if w := controlled.send(t, in); w.Code < 400 || calls.jev+calls.extract != 2 || controlled.dispatchCount(t) != 2 {
			t.Fatal("new valid legacy portion bypassed whole-Action cap")
		}
	})
	t.Run("maximum thirty-claim focus is bounded before a subsequent focus", func(t *testing.T) {
		calls := &submissionCalls{}
		batch0, batch1 := []claim{}, []claim{}
		for index := 0; index < 17; index++ {
			batch0 = append(batch0, fixtureClaim(fmt.Sprintf("c%02d", index), "Uses "+lowercaseKey(index), "skills"))
			batch1 = append(batch1, fixtureClaim(fmt.Sprintf("c%02d", index), "Uses "+lowercaseKey(index), "skills"))
		}
		focus, ops, outcomes := []string{}, []operation{}, []claimOutcome{}
		for index := 0; index < 30; index++ {
			batch, local := 0, index
			if index >= 17 {
				batch, local = 1, index-17
			}
			id := reconcileGID(batch, fmt.Sprintf("c%02d", local))
			focus = append(focus, id)
			ops = append(ops, reconcileOp(id, "skills", "", "skills", "add", lowercaseKey(index), "addition"))
			outcomes = append(outcomes, reconcileOutcome(id, "change", "new_skill", nil))
		}
		in := request{Input: "Whole submission reconciliation line. Whole submission reconciliation line.", Profile: profile()}
		a := reconcileApp(calls, "professional_fact", proposalJSON(ops, outcomes))
		receipt := sealContext(t, a, in, [][]claim{batch0, batch1})
		controlled := admittedIngestion(t, a, func(cfg *aicontrol.SyntheticConfig) { cfg.MaxDispatches = 2 })
		if w := controlled.send(t, reconcileRequest(in, receipt, focus...)); w.Code != 200 || calls.jev+calls.extract != 2 || controlled.dispatchCount(t) != 2 {
			t.Fatalf("maximum focus status=%d body=%s", w.Code, w.Body.String())
		}
		if w := controlled.send(t, reconcileRequest(in, receipt, reconcileGID(1, "c16"))); w.Code < 400 || calls.jev+calls.extract != 2 || controlled.dispatchCount(t) != 2 {
			t.Fatal("new valid focus bypassed whole-Action cap")
		}
		oversized := append(append([]string{}, focus...), reconcileGID(1, "c16"))
		if w := controlled.send(t, reconcileRequest(in, receipt, oversized...)); w.Code < 400 || calls.jev+calls.extract != 2 {
			t.Fatal("thirty-one-claim focus was not denied before provider send")
		}
	})
}
