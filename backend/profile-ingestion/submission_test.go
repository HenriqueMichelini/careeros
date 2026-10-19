package profileingestion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/profiledocument"
)

// submissionCalls counts and records provider dispatch so tests can assert the
// no-call invariants independently of handler output.
type submissionCalls struct {
	jev           int
	extract       int
	jevBodies     []string
	extractBodies []string
}

func submissionApp(calls *submissionCalls, claims string) app {
	return app{signingKey: []byte(strings.Repeat("s", 32)), client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Host == "api.typesafe.ai" {
			calls.jev++
			calls.jevBodies = append(calls.jevBodies, string(raw))
			return jevResponse("professional_fact", "none"), nil
		}
		calls.extract++
		calls.extractBodies = append(calls.extractBodies, string(raw))
		return completion(claims), nil
	})}}
}

func intPtr(value int) *int { return &value }

func planSubmission(t *testing.T, a app, in request) SubmissionPlanResponse {
	t.Helper()
	in.Stage = &stageRequest{SubmissionStageVersion, SubmissionStagePlan}
	w := send(t, a.handler(), in)
	var response SubmissionPlanResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("plan status=%d body=%s", w.Code, w.Body.String())
	}
	return response
}

func extractSubmission(in request, plan SubmissionPlanResponse, index int) request {
	in.Stage = &stageRequest{SubmissionStageVersion, SubmissionStageExtract}
	in.PlanReceipt = plan.PlanReceipt
	in.BatchIndex = intPtr(index)
	return in
}

func resignPlan(t *testing.T, a app, receipt string, mutate func(*submissionPlanReceipt)) string {
	t.Helper()
	parts := strings.Split(receipt, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var parsed submissionPlanReceipt
	if json.Unmarshal(payload, &parsed) != nil {
		t.Fatal("plan receipt payload is not JSON")
	}
	mutate(&parsed)
	payload = mustJSON(parsed)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(submissionSignature(submissionPlanDomain, a.signingKey, payload))
}

// Planning is provider-free, issues a ten-minute receipt and returns a plan that
// independently satisfies the manifest contract.
func TestSubmissionPlanMakesNoProviderCallsAndIssuesValidManifest(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, "")
	response := planSubmission(t, a, request{Input: "I use Java.", Profile: profile()})
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("planning dispatched provider calls: %+v", calls)
	}
	if response.PlanReceipt == "" || response.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("plan receipt missing or expired: %s %d", response.PlanReceipt, response.ExpiresAt)
	}
	if response.Stage.Version != SubmissionStageVersion || response.Stage.Kind != SubmissionStagePlan {
		t.Fatalf("unexpected stage %+v", response.Stage)
	}
	if code := SubmissionManifestError(response.Plan, "I use Java."); code != "" {
		t.Fatalf("issued plan violates the manifest contract: %q", code)
	}
	if response.Plan.ProtocolVersion != SubmissionProtocolVersion || response.Plan.PlanVersion != preprocessing.PlanningVersion {
		t.Fatalf("unexpected plan versions %+v", response.Plan)
	}
	if response.Plan.SourceID == "" || response.Plan.ProfileID == "" || response.Plan.RunID == "" {
		t.Fatalf("missing plan identity %+v", response.Plan)
	}
	if response.Plan.OriginalBytes != len("I use Java.") || response.Plan.PreparedBytes <= 0 {
		t.Fatalf("unexpected plan sizes %+v", response.Plan)
	}
}

// Extraction verifies pre-dispatch facts, classifies the whole original with
// Jev and then makes exactly one bounded call for the selected batch.
func TestSubmissionExtractClassifiesWholeOriginalThenExtractsOneBatch(t *testing.T) {
	claims := `{"claims":[{"id":"c1","source":"I use Java.","text":"Uses Java","targets":["skills"],"question":""}]}`
	calls := &submissionCalls{}
	a := submissionApp(calls, claims)
	plan := planSubmission(t, a, request{Input: "I use Java.", Profile: profile()})
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatal(calls)
	}
	w := send(t, a.handler(), extractSubmission(request{Input: "I use Java.", Profile: profile()}, plan, 0))
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("response must not be cached")
	}
	var response SubmissionExtractionResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("extract status=%d body=%s", w.Code, w.Body.String())
	}
	if calls.jev != 1 || calls.extract != 1 {
		t.Fatalf("expected one Jev and one extraction call: %+v", calls)
	}
	var jev struct {
		State struct {
			Field      string `json:"field"`
			Submission string `json:"submission"`
		} `json:"state"`
	}
	if json.Unmarshal([]byte(calls.jevBodies[0]), &jev) != nil || jev.State.Submission != "I use Java." || jev.State.Field != "professional_information" {
		t.Fatalf("Jev body must carry the exact complete original: %s", calls.jevBodies[0])
	}
	if !strings.Contains(calls.extractBodies[0], `"name":"profile_claims"`) || !strings.Contains(calls.extractBodies[0], "I use Java.") {
		t.Fatalf("extraction body must carry the selected batch: %s", calls.extractBodies[0])
	}
	if len(response.Claims) != 1 || response.Claims[0].Source != "I use Java." || response.UnverifiedCount != 0 {
		t.Fatalf("unexpected claims %+v", response)
	}
	if response.Coverage.DiscoveryComplete || response.Coverage.Capacity != SubmissionCapacityWithin {
		t.Fatalf("coverage must never assert discovery: %+v", response.Coverage)
	}
	if response.BatchIndex != 0 || response.BatchReceipt == "" || response.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("missing stable batch identity: %+v", response)
	}
	var payload submissionBatchReceiptPayload
	if code := a.openSubmission(submissionBatchDomain, response.BatchReceipt, &payload); code != "" {
		t.Fatalf("batch receipt did not verify: %q", code)
	}
	if payload.BatchIndex != 0 || payload.RunID != plan.Plan.RunID || payload.SourceID != plan.Plan.SourceID || payload.ProfileID != plan.Plan.ProfileID {
		t.Fatalf("batch receipt identity mismatch: %+v", payload)
	}
	if payload.Binding != submissionRequestBinding(request{Input: "I use Java.", Profile: profile()}) {
		t.Fatalf("batch receipt binding mismatch: %+v", payload)
	}
}

// Every verification failure stops before provider dispatch.
func TestSubmissionExtractRejectsBeforeDispatch(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, `{"claims":[]}`)
	plan := planSubmission(t, a, request{Input: "I use Java.", Profile: profile()})
	base := extractSubmission(request{Input: "I use Java.", Profile: profile()}, plan, 0)
	document := profiledocument.Document{Version: 2, ID: "profile", Revision: 1, NormalizationPolicy: "exact-alias-v1", Entities: []profiledocument.Entity{}, Facts: []profiledocument.Fact{}, Evidence: []profiledocument.Evidence{}, Links: []profiledocument.Link{}}
	for _, kind := range []string{"signature", "expired", "version", "input", "profile", "document", "portion", "batch", "oversized", "missing-key"} {
		t.Run(kind, func(t *testing.T) {
			in := base
			in.Profile = profile()
			in.Input = "I use Java."
			in.Document = nil
			in.BatchIndex = intPtr(0)
			candidate := a
			switch kind {
			case "signature":
				in.PlanReceipt = "A" + in.PlanReceipt[1:]
			case "expired":
				in.PlanReceipt = resignPlan(t, a, in.PlanReceipt, func(p *submissionPlanReceipt) { p.ExpiresAt = time.Now().Add(-time.Second).UnixMilli() })
			case "version":
				in.PlanReceipt = resignPlan(t, a, in.PlanReceipt, func(p *submissionPlanReceipt) { p.Version = "old" })
			case "input":
				in.Input = "I use Java. More."
			case "profile":
				in.Profile.Skills = "React, SQL"
			case "document":
				in.Document = mustJSON(document)
			case "portion":
				in.Portion = &portionRequest{Index: 0, Bytes: 800}
			case "batch":
				in.BatchIndex = intPtr(99)
			case "oversized":
				in.PlanReceipt = strings.Repeat("x", 180000)
			case "missing-key":
				candidate.signingKey = nil
			}
			before := calls.jev + calls.extract
			w := send(t, candidate.handler(), in)
			if w.Code < 400 || calls.jev+calls.extract != before {
				t.Fatalf("status=%d dispatch delta=%d body=%s", w.Code, calls.jev+calls.extract-before, w.Body.String())
			}
		})
	}
}

// A multi-batch plan re-derives deterministically and each extraction receives
// only its selected source region, never another batch's coverage.
func TestSubmissionExtractBoundsSelectedBatchOnly(t *testing.T) {
	var builder strings.Builder
	for i := 0; i < 3200; i++ {
		fmt.Fprintf(&builder, "%05d", i)
	}
	input := builder.String()
	calls := &submissionCalls{}
	a := submissionApp(calls, `{"claims":[]}`)
	plan := planSubmission(t, a, request{Input: input, Profile: profile()})
	if len(plan.Plan.Batches) < 2 {
		t.Fatalf("expected a multi-batch plan, got %d", len(plan.Plan.Batches))
	}
	selected := plan.Plan.Batches[1].Primary
	if len(selected) == 0 {
		t.Fatal("selected batch has no primary coverage")
	}
	w := send(t, a.handler(), extractSubmission(request{Input: input, Profile: profile()}, plan, 1))
	if w.Code != 200 || calls.jev != 1 || calls.extract != 1 {
		t.Fatalf("status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
	}
	var jev struct {
		State struct {
			Submission string `json:"submission"`
		} `json:"state"`
	}
	if json.Unmarshal([]byte(calls.jevBodies[0]), &jev) != nil || jev.State.Submission != input {
		t.Fatal("Jev must classify the exact complete original")
	}
	first := plan.Plan.Batches[0].Primary[0]
	body := calls.extractBodies[0]
	if !strings.Contains(body, input[selected[0].Start:selected[0].End]) {
		t.Fatal("selected batch source region missing from the extraction request")
	}
	if strings.Contains(body, input[first.Start:first.End]) {
		t.Fatal("extraction request leaked another batch's source region")
	}
}

// A complete multi-portion plan is reported as `ready`: the planner's single-view
// status must not mislabel a routine multi-batch submission as explicitly
// partial (manifest invariant 8). Complete plans are derived from mechanical
// completeness, not from `preprocessing.Plan.Status`.
func TestSubmissionPlanReportsCompleteMultiBatchAsReady(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, "")
	var builder strings.Builder
	for i := 0; i < 3200; i++ {
		fmt.Fprintf(&builder, "%05d", i)
	}
	input := builder.String()
	response := planSubmission(t, a, request{Input: input, Profile: profile()})
	if len(response.Plan.Batches) < 2 {
		t.Fatalf("expected a multi-batch plan, got %d", len(response.Plan.Batches))
	}
	if response.Plan.Status != string(preprocessing.Ready) {
		t.Fatalf("complete multi-batch plan status = %q, want %q", response.Plan.Status, preprocessing.Ready)
	}
	if !response.Plan.Complete || response.Plan.Capacity != "" || len(response.Plan.Remaining) != 0 {
		t.Fatalf("complete multi-batch plan mislabeled as partial: %+v", response.Plan)
	}
	if code := SubmissionManifestError(response.Plan, input); code != "" {
		t.Fatalf("complete multi-batch plan violates the manifest: %q", code)
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("planning dispatched provider calls: %+v", calls)
	}
}

// The 27-second handler deadline stops before any provider dispatch.
func TestSubmissionDeadlineStopsBeforeProvider(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, `{"claims":[]}`)
	plan := planSubmission(t, a, request{Input: "I use Java.", Profile: profile()})
	in := extractSubmission(request{Input: "I use Java.", Profile: profile()}, plan, 0)
	r := httptest.NewRequest("POST", "/api/profile/ingest", strings.NewReader(string(mustJSON(in))))
	r.Header.Set("X-OpenAI-Api-Key", "sk-inert")
	r.Header.Set("X-TypeSafe-Api-Key", "inert")
	ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(-time.Second))
	defer cancel()
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r.WithContext(ctx))
	if w.Code != 504 || calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
	}
}

type cancellingSubmissionBody struct{ cancel context.CancelFunc }

func (b cancellingSubmissionBody) Read([]byte) (int, error) { b.cancel(); return 0, context.Canceled }
func (cancellingSubmissionBody) Close() error               { return nil }

// A client that cancels while the body is read still dispatches nothing.
func TestSubmissionBodyCancellationStopsBeforeProvider(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, `{"claims":[]}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest("POST", "/api/profile/ingest", nil)
	r.Body = cancellingSubmissionBody{cancel: cancel}
	r.Header.Set("X-OpenAI-Api-Key", "sk-inert")
	r.Header.Set("X-TypeSafe-Api-Key", "inert")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r.WithContext(ctx))
	if w.Code < 400 || calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
	}
}

// Multi-batch Unicode input keeps every planned range on a rune boundary and
// extracts the selected batch without splitting a multibyte rune.
func TestSubmissionExtractHandlesUnicodeBoundaries(t *testing.T) {
	input := strings.Repeat("ação🙂 trabalhei com Java e Go.\n", 120)
	calls := &submissionCalls{}
	a := submissionApp(calls, `{"claims":[]}`)
	plan := planSubmission(t, a, request{Input: input, Profile: profile()})
	if code := SubmissionManifestError(plan.Plan, input); code != "" {
		t.Fatalf("unicode plan violated the manifest: %q", code)
	}
	if len(plan.Plan.Batches) < 2 {
		t.Fatalf("expected a multi-batch unicode plan, got %d", len(plan.Plan.Batches))
	}
	for _, batch := range plan.Plan.Batches {
		for _, r := range batch.Primary {
			if !runeBoundary(input, r.Start) || !runeBoundary(input, r.End) {
				t.Fatalf("batch %d cut inside a rune: %+v", batch.Index, r)
			}
		}
	}
	last := len(plan.Plan.Batches) - 1
	w := send(t, a.handler(), extractSubmission(request{Input: input, Profile: profile()}, plan, last))
	if w.Code != 200 || calls.jev != 1 || calls.extract != 1 {
		t.Fatalf("status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
	}
}

// Stage kinds owned by later tickets are rejected without touching v1 behavior.
func TestSubmissionUnsupportedStagesAreRejected(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, `{"claims":[]}`)
	for _, kind := range []string{SubmissionStageReconcile, "compare"} {
		in := request{Input: "I use Java.", Profile: profile(), Stage: &stageRequest{SubmissionStageVersion, kind}}
		w := send(t, a.handler(), in)
		if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"`+SubmissionErrStage+`"`) {
			t.Fatalf("kind=%s status=%d body=%s", kind, w.Code, w.Body.String())
		}
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("unsupported stages dispatched provider calls: %+v", calls)
	}
}
