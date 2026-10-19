package profileingestion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/preprocessing"
)

// syntheticInput builds a personal-data-free multi-portion paste of fixed-width
// digits. It contains no real personal data.
func syntheticInput(iterations int) string {
	var builder strings.Builder
	for i := 0; i < iterations; i++ {
		fmt.Fprintf(&builder, "%05d", i)
	}
	return builder.String()
}

func syntheticClaim(id string) claim {
	return claim{ID: id, Source: "Synthetic source", Text: "Synthetic text", Targets: []string{"skills"}, Question: ""}
}

func assembleSubmission(in request, plan SubmissionPlanResponse, receipts []string) request {
	in.Stage = &stageRequest{SubmissionStageVersion, SubmissionStageAssemble}
	in.PlanReceipt = plan.PlanReceipt
	in.BatchReceipts = receipts
	return in
}

func sendRaw(t *testing.T, h http.Handler, raw []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/profile/ingest", bytes.NewReader(raw))
	r.Header.Set("X-OpenAI-Api-Key", "sk-test")
	r.Header.Set("X-TypeSafe-Api-Key", "synthetic-typesafe")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// padJSON frames a request as a JSON object followed by inert whitespace so the
// transport body limit can be exercised independently of the payload.
func padJSON(t *testing.T, in request, size int) []byte {
	t.Helper()
	raw := mustJSON(in)
	if len(raw) > size {
		t.Fatalf("request body %d exceeds pad size %d", len(raw), size)
	}
	out := make([]byte, size)
	copy(out, raw)
	for i := len(raw); i < len(out); i++ {
		out[i] = ' '
	}
	return out
}

// submissionBatchPayload builds an independently-constructed batch receipt
// payload for one plan batch. Tests mutate it to cover each rejection.
func submissionBatchPayload(plan SubmissionPlan, in request, index int, claims []claim, skipped []skippedClaim, capacity string) submissionBatchReceiptPayload {
	return submissionBatchReceiptPayload{
		Version: submissionReceiptVersion, Binding: submissionRequestBinding(in),
		ExpiresAt: time.Now().Add(SubmissionReceiptTTL).UnixMilli(),
		SourceID:  plan.SourceID, ProfileID: plan.ProfileID, DocumentID: plan.DocumentID, RunID: plan.RunID,
		PlanVersion: plan.PlanVersion, BatchIndex: index,
		Primary: plan.Batches[index].Primary, Context: plan.Batches[index].Context,
		Claims: claims, UnverifiedCount: len(skipped), SkippedClaims: skipped,
		Coverage: SubmissionCoverage{ValidClaims: len(claims), InvalidClaims: len(skipped), Capacity: capacity},
	}
}

func sealBatch(t *testing.T, a app, payload submissionBatchReceiptPayload) string {
	t.Helper()
	receipt, code := a.sealSubmission(submissionBatchDomain, mustJSON(payload))
	if code != "" {
		t.Fatalf("seal batch receipt: %q", code)
	}
	return receipt
}

func tamperReceipt(receipt string) string {
	last := receipt[len(receipt)-1]
	if last == 'A' {
		return receipt[:len(receipt)-1] + "B"
	}
	return receipt[:len(receipt)-1] + "A"
}

// A complete set of independently-constructed batch receipts assembles into a
// valid aggregate with globally-namespaced, unique claim ids. Assembly is
// provider-free, preserves each batch's distinct context overlap, shortens
// nothing and never asserts discovery.
func TestSubmissionAssembleCompleteSetNamespacesClaimIDs(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, "")
	input := syntheticInput(3200)
	in := request{Input: input, Profile: profile()}
	plan := planSubmission(t, a, in)
	if len(plan.Plan.Batches) < 2 {
		t.Fatalf("expected a multi-batch plan, got %d", len(plan.Plan.Batches))
	}
	receipts := make([]string, len(plan.Plan.Batches))
	for index := range plan.Plan.Batches {
		// The local ids repeat across batches by construction.
		claims := []claim{syntheticClaim("c1"), syntheticClaim("c2")}
		skipped := []skippedClaim{{Index: 0, Reason: "claim_text", Text: "Synthetic skipped"}}
		receipts[index] = sealBatch(t, a, submissionBatchPayload(plan.Plan, in, index, claims, skipped, SubmissionCapacityWithin))
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("constructing the manifest dispatched provider calls: %+v", calls)
	}

	w := send(t, a.handler(), assembleSubmission(in, plan, receipts))
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("assembly response must not be cached")
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("assembly dispatched provider calls: %+v", calls)
	}
	var response SubmissionAssemblyResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatalf("assemble status=%d body=%s", w.Code, w.Body.String())
	}
	if response.Stage.Version != SubmissionStageVersion || response.Stage.Kind != SubmissionStageAssemble {
		t.Fatalf("unexpected stage %+v", response.Stage)
	}
	if response.ContextReceipt == "" || response.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("missing context receipt: %+v", response)
	}
	if code := SubmissionAggregateError(response.Context, plan.Plan); code != "" {
		t.Fatalf("issued aggregate violates the contract: %q", code)
	}
	seen := map[string]bool{}
	for index, batch := range response.Context.Batches {
		if batch.Index != index || batch.ClaimCount != 2 || batch.SkippedCount != 1 {
			t.Fatalf("unexpected aggregate batch %+v", batch)
		}
		if len(batch.ClaimIDs) != 2 {
			t.Fatalf("batch %d claim ids %v", index, batch.ClaimIDs)
		}
		prefix := GlobalClaimID(plan.Plan.RunID, index, "")
		for _, id := range batch.ClaimIDs {
			if !strings.HasPrefix(id, prefix) {
				t.Fatalf("claim id %q is not namespaced for batch %d", id, index)
			}
			if seen[id] {
				t.Fatalf("duplicate global claim id %q", id)
			}
			seen[id] = true
		}
	}
	if !seen[GlobalClaimID(plan.Plan.RunID, 0, "c1")] || !seen[GlobalClaimID(plan.Plan.RunID, 1, "c1")] {
		t.Fatal("repeated local claim id was not namespaced into distinct global ids")
	}
	if response.Context.Coverage.DiscoveryComplete || response.Context.Coverage.Capacity != SubmissionCapacityWithin {
		t.Fatalf("assembly must never assert discovery: %+v", response.Context.Coverage)
	}

	// The signed handoff carries the namespaced claims for reconciliation.
	var payload submissionContextReceiptPayload
	if code := a.openSubmission(submissionContextDomain, response.ContextReceipt, &payload); code != "" {
		t.Fatalf("context receipt did not verify: %q", code)
	}
	if payload.Binding != submissionRequestBinding(in) || payload.RunID != plan.Plan.RunID ||
		payload.SourceID != plan.Plan.SourceID || payload.ProfileID != plan.Plan.ProfileID ||
		payload.PlanVersion != plan.Plan.PlanVersion || payload.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("context receipt identity mismatch: %+v", payload)
	}
	if len(payload.Batches) != len(plan.Plan.Batches) {
		t.Fatalf("handoff batch count %d", len(payload.Batches))
	}
	for index, batch := range payload.Batches {
		if !sameRanges(batch.Context, plan.Plan.Batches[index].Context) {
			t.Fatalf("batch %d context overlap was not preserved", index)
		}
		if len(batch.SkippedClaims) != 1 || batch.SkippedClaims[0].Reason != "claim_text" {
			t.Fatalf("batch %d skipped metadata was not preserved: %+v", index, batch.SkippedClaims)
		}
		for _, c := range batch.Claims {
			if !seen[c.ID] {
				t.Fatalf("handoff claim %q is not a namespaced aggregate claim", c.ID)
			}
			if c.Source != "Synthetic source" || c.Text != "Synthetic text" {
				t.Fatalf("handoff claim occurrence was altered: %+v", c)
			}
		}
	}
}

// Every invalid batch set is rejected before any further work and returns no
// aggregate and no provider dispatch.
func TestSubmissionAssembleRejectsBadBatchSets(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, "")
	input := syntheticInput(3200)
	in := request{Input: input, Profile: profile()}
	plan := planSubmission(t, a, in)
	if len(plan.Plan.Batches) < 2 {
		t.Fatalf("expected a multi-batch plan, got %d", len(plan.Plan.Batches))
	}
	empty := []claim{syntheticClaim("c1")}
	base := func() []string {
		receipts := make([]string, len(plan.Plan.Batches))
		for index := range plan.Plan.Batches {
			receipts[index] = sealBatch(t, a, submissionBatchPayload(plan.Plan, in, index, empty, nil, SubmissionCapacityWithin))
		}
		return receipts
	}
	mutate := func(index int, apply func(*submissionBatchReceiptPayload)) string {
		payload := submissionBatchPayload(plan.Plan, in, index, empty, nil, SubmissionCapacityWithin)
		apply(&payload)
		return sealBatch(t, a, payload)
	}
	cases := []struct {
		name string
		want string
		set  func() []string
	}{
		{"missing", SubmissionErrManifest, func() []string { all := base(); return all[:len(all)-1] }},
		{"duplicated", SubmissionErrManifest, func() []string { all := base(); all[len(all)-1] = all[0]; return all }},
		{"extra", SubmissionErrManifest, func() []string {
			return append(base(), sealBatch(t, a, submissionBatchPayload(plan.Plan, in, 0, nil, nil, SubmissionCapacityWithin)))
		}},
		{"other-plan", SubmissionErrBatch, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) { p.RunID = "other-run" })
			return all
		}},
		{"other-source", SubmissionErrBatch, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) { p.SourceID = "other-source" })
			return all
		}},
		{"other-profile", SubmissionErrBatch, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) { p.ProfileID = "other-profile" })
			return all
		}},
		{"other-document", SubmissionErrBatch, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) { p.DocumentID = "other-document" })
			return all
		}},
		{"other-portion", SubmissionErrBatch, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) { p.Primary = []preprocessing.Range{{Start: 0, End: 1}} })
			return all
		}},
		{"wrong-version", SubmissionErrReceipt, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) { p.Version = "profile-ingestion-submission-v0" })
			return all
		}},
		{"wrong-binding", SubmissionErrReceipt, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) {
				p.Binding = "0000000000000000000000000000000000000000000000000000000000000000"
			})
			return all
		}},
		{"expired", SubmissionErrReceiptExpired, func() []string {
			all := base()
			all[0] = mutate(0, func(p *submissionBatchReceiptPayload) { p.ExpiresAt = time.Now().Add(-time.Second).UnixMilli() })
			return all
		}},
		{"tampered", SubmissionErrReceipt, func() []string { all := base(); all[0] = tamperReceipt(all[0]); return all }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.jev + calls.extract
			w := send(t, a.handler(), assembleSubmission(in, plan, tc.set()))
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"`+tc.want+`"`) {
				t.Fatalf("status=%d want=%s body=%s", w.Code, tc.want, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "contextReceipt") || strings.Contains(w.Body.String(), `"context"`) {
				t.Fatalf("rejection returned an aggregate: %s", w.Body.String())
			}
			if calls.jev+calls.extract != before {
				t.Fatalf("rejection dispatched provider calls: %+v", calls)
			}
		})
	}
}

// A plan with any uncovered region (`remaining`) is `manifest_incomplete`:
// assembly must not silently assemble a subset of the submission.
func TestSubmissionAssembleRequiresCompletePlan(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, "")
	in := request{Input: "I use Java.", Profile: profile()}
	total := len(in.Input)
	plan := planSubmission(t, a, in)
	receipt := resignPlan(t, a, plan.PlanReceipt, func(p *submissionPlanReceipt) {
		p.Plan.Batches[0].Primary = []preprocessing.Range{{Start: 0, End: 4}}
		p.Plan.Batches[0].Context = nil
		p.Plan.Remaining = []preprocessing.Range{{Start: 4, End: total}}
		p.Plan.Complete = false
		p.Plan.Status = string(preprocessing.InsufficientCapacity)
		p.Plan.Capacity = "portion_count"
	})
	// The mutated plan still satisfies the manifest contract, so the rejection
	// must come from assembly's completeness requirement.
	var parsed submissionPlanReceipt
	if code := a.openSubmission(submissionPlanDomain, receipt, &parsed); code != "" {
		t.Fatalf("resigned plan did not open: %q", code)
	}
	if code := SubmissionManifestError(parsed.Plan, in.Input); code != "" {
		t.Fatalf("mutated plan is not a valid manifest: %q", code)
	}
	candidate := plan
	candidate.PlanReceipt = receipt
	w := send(t, a.handler(), assembleSubmission(in, candidate, nil))
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"`+SubmissionErrManifest+`"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("incomplete plan dispatched provider calls: %+v", calls)
	}
}

// An aggregate over 128 KiB is rejected as `capacity` rather than truncated.
func TestSubmissionAssembleRejectsOversizeAggregate(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, "")
	in := request{Input: syntheticInput(900), Profile: profile()}
	plan := planSubmission(t, a, in)
	batches := len(plan.Plan.Batches)
	if batches < 2 {
		t.Fatalf("expected a multi-batch plan, got %d", batches)
	}
	totalClaims := batches * SubmissionBatchClaimCap
	idLength := SubmissionContextBytes/totalClaims + 200
	receipts := make([]string, batches)
	for index := range plan.Plan.Batches {
		claims := make([]claim, 0, SubmissionBatchClaimCap)
		for j := 0; j < SubmissionBatchClaimCap; j++ {
			claims = append(claims, syntheticClaim(fmt.Sprintf("c%03d", j)+strings.Repeat("x", idLength)))
		}
		receipts[index] = sealBatch(t, a, submissionBatchPayload(plan.Plan, in, index, claims, nil, SubmissionCapacityWithin))
	}
	w := send(t, a.handler(), assembleSubmission(in, plan, receipts))
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"`+SubmissionErrCapacity+`"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "contextReceipt") {
		t.Fatalf("oversize aggregate was returned: %s", w.Body.String())
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("oversize aggregate dispatched provider calls: %+v", calls)
	}
}

// A context receipt payload over 128 KiB is rejected as `capacity` rather than
// truncated, even though the aggregate membership is within its own bound.
func TestSubmissionAssembleRejectsOversizeContextReceipt(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, "")
	in := request{Input: syntheticInput(900), Profile: profile()}
	plan := planSubmission(t, a, in)
	batches := len(plan.Plan.Batches)
	if batches < 2 {
		t.Fatalf("expected a multi-batch plan, got %d", batches)
	}
	text := strings.Repeat("t", 180000/(batches*SubmissionBatchClaimCap))
	receipts := make([]string, batches)
	for index := range plan.Plan.Batches {
		claims := make([]claim, 0, SubmissionBatchClaimCap)
		for j := 0; j < SubmissionBatchClaimCap; j++ {
			c := syntheticClaim(fmt.Sprintf("c%02d", j))
			c.Text = text
			claims = append(claims, c)
		}
		receipts[index] = sealBatch(t, a, submissionBatchPayload(plan.Plan, in, index, claims, nil, SubmissionCapacityWithin))
	}
	w := send(t, a.handler(), assembleSubmission(in, plan, receipts))
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"`+SubmissionErrCapacity+`"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	// Prove the aggregate itself is valid; the authenticated receipt payload,
	// which carries the namespaced claims, is the bound that overflows.
	opened, code := a.openSubmissionBatches(assembleSubmission(in, plan, receipts), plan.Plan)
	if code != "" {
		t.Fatalf("batch set did not open: %q", code)
	}
	aggregate, contextBatches, code := submissionAssembly(plan.Plan, opened)
	if code != "" {
		t.Fatalf("aggregate was rejected: %q", code)
	}
	if len(mustJSON(aggregate)) > SubmissionContextBytes {
		t.Fatalf("aggregate unexpectedly exceeds its own bound")
	}
	payload := mustJSON(submissionContextReceiptPayload{
		Version: submissionReceiptVersion, Binding: submissionRequestBinding(in), ExpiresAt: time.Now().UnixMilli(),
		SourceID: plan.Plan.SourceID, ProfileID: plan.Plan.ProfileID, DocumentID: plan.Plan.DocumentID,
		RunID: plan.Plan.RunID, PlanVersion: plan.Plan.PlanVersion, Batches: contextBatches, Coverage: aggregate.Coverage,
	})
	if len(payload) <= SubmissionReceiptBytes {
		t.Fatalf("expected an over-limit receipt payload, got %d bytes", len(payload))
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("oversize receipt dispatched provider calls: %+v", calls)
	}
}

// The transport gate allows the 320 KiB assembly body while planning,
// extraction and legacy requests keep the retained 160 KiB bound.
func TestSubmissionAssembleBodyLimit(t *testing.T) {
	calls := &submissionCalls{}
	a := submissionApp(calls, `{"claims":[]}`)
	in := request{Input: "I use Java.", Profile: profile()}
	plan := planSubmission(t, a, in)
	receipts := []string{sealBatch(t, a, submissionBatchPayload(plan.Plan, in, 0, []claim{syntheticClaim("c1")}, nil, SubmissionCapacityWithin))}

	if w := sendRaw(t, a.handler(), padJSON(t, assembleSubmission(in, plan, receipts), 200<<10)); w.Code != 200 {
		t.Fatalf("assemble at 200 KiB must be accepted: status=%d body=%s", w.Code, w.Body.String())
	}
	if w := sendRaw(t, a.handler(), padJSON(t, assembleSubmission(in, plan, receipts), 340<<10)); w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"input"`) {
		t.Fatalf("assemble at 340 KiB must be rejected: status=%d body=%s", w.Code, w.Body.String())
	}
	planRequest := in
	planRequest.Stage = &stageRequest{SubmissionStageVersion, SubmissionStagePlan}
	extractRequest := extractSubmission(in, plan, 0)
	v1Extract := in
	v1Extract.Stage = &stageRequest{1, "extract"}
	for name, candidate := range map[string]request{"plan": planRequest, "extract": extractRequest, "v1-extract": v1Extract, "legacy": in} {
		w := sendRaw(t, a.handler(), padJSON(t, candidate, 200<<10))
		if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"input"`) {
			t.Fatalf("%s at 200 KiB must keep the 160 KiB bound: status=%d body=%s", name, w.Code, w.Body.String())
		}
	}
	if calls.jev != 0 || calls.extract != 0 {
		t.Fatalf("body-limit checks dispatched provider calls: %+v", calls)
	}
}

// New-entry anchors and claim-id-bearing references use the same boundary rule
// as claim ids, so references stay consistent across the complete submission.
func TestSubmissionAssemblyNamespacesAnchorsAndReferences(t *testing.T) {
	run := "run"
	if got := namespaceAnchor(run, 1, "new:c1"); got != "new:"+GlobalClaimID(run, 1, "c1") {
		t.Fatalf("new-entry anchor %q", got)
	}
	if got := namespaceAnchor(run, 1, "existing-entry"); got != "existing-entry" {
		t.Fatalf("copied existing entry id must not change: %q", got)
	}
	refs := namespaceClaimReferences(run, 2, []string{"new:c1", "c2"})
	want := []string{"new:" + GlobalClaimID(run, 2, "c1"), GlobalClaimID(run, 2, "c2")}
	if len(refs) != len(want) {
		t.Fatalf("references %v", refs)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("reference %d = %q, want %q", i, refs[i], want[i])
		}
	}
	if namespaceClaimReferences(run, 0, nil) != nil {
		t.Fatal("empty references must stay empty")
	}
}
