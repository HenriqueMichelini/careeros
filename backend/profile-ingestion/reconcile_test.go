package profileingestion

// Reconciliation-focus fixtures (#186) exercise the version-2 `reconcile`
// stage: one whole-original Jev classification, exactly one bounded generation
// call and the focus/provisional split over the authenticated whole-submission
// context.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/profilevalidation"
)

// reconcileRun is the deterministic run id used by the fixture context receipts.
const reconcileRun = "run-reconcile"

func reconcileGID(index int, local string) string { return GlobalClaimID(reconcileRun, index, local) }

// reconcileApp builds an app whose classifier returns jev and whose generation
// returns generation, counting and recording both dispatches.
func reconcileApp(calls *submissionCalls, jev, generation string) app {
	return app{signingKey: []byte(strings.Repeat("s", 32)), client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Host == "api.typesafe.ai" {
			calls.jev++
			calls.jevBodies = append(calls.jevBodies, string(raw))
			return jevResponse(jev, "none"), nil
		}
		calls.extract++
		calls.extractBodies = append(calls.extractBodies, string(raw))
		return completion(generation), nil
	})}}
}

// tileRanges splits total bytes into count contiguous ranges so a reconstructed
// focus plan covers the exact original.
func tileRanges(total, count int) [][]preprocessing.Range {
	ranges := make([][]preprocessing.Range, count)
	for i := 0; i < count; i++ {
		ranges[i] = []preprocessing.Range{{Start: total * i / count, End: total * (i + 1) / count}}
	}
	return ranges
}

// sealContext builds and signs an assembly-style context handoff whose batches
// carry the namespaced claims, exactly as assembly would.
func sealContext(t *testing.T, a app, in request, batches [][]claim) string {
	t.Helper()
	ranges := tileRanges(len(in.Input), len(batches))
	documentID, _ := submissionDocument(in)
	contextBatches := make([]submissionContextBatch, 0, len(batches))
	valid := 0
	for index := range batches {
		claims := make([]claim, 0, len(batches[index]))
		ids := make([]string, 0, len(batches[index]))
		for _, c := range batches[index] {
			mapped := namespaceClaim(reconcileRun, index, c)
			claims = append(claims, mapped)
			ids = append(ids, mapped.ID)
		}
		contextBatches = append(contextBatches, submissionContextBatch{
			Index: index, Primary: ranges[index],
			ClaimIDs: ids, Claims: claims, SkippedClaims: []skippedClaim{},
			Coverage: SubmissionCoverage{ValidClaims: len(claims), InvalidClaims: 0, Capacity: SubmissionCapacityWithin},
		})
		valid += len(claims)
	}
	payload := submissionContextReceiptPayload{
		Version: submissionReceiptVersion, Binding: submissionRequestBinding(in),
		ExpiresAt: time.Now().Add(SubmissionReceiptTTL).UnixMilli(),
		SourceID:  "source", ProfileID: canonicalBinding(in.Profile), DocumentID: documentID,
		RunID: reconcileRun, PlanVersion: preprocessing.PlanningVersion,
		Batches:  contextBatches,
		Coverage: SubmissionCoverage{ValidClaims: valid, InvalidClaims: 0, DiscoveryComplete: false, Capacity: SubmissionCapacityWithin},
	}
	receipt, code := a.sealSubmission(submissionContextDomain, mustJSON(payload))
	if code != "" {
		t.Fatalf("seal context receipt: %q", code)
	}
	return receipt
}

// resignContext re-signs a context receipt after mutating its payload.
func resignContext(t *testing.T, a app, receipt string, mutate func(*submissionContextReceiptPayload)) string {
	t.Helper()
	parts := strings.Split(receipt, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var parsed submissionContextReceiptPayload
	if json.Unmarshal(payload, &parsed) != nil {
		t.Fatal("context receipt payload is not JSON")
	}
	mutate(&parsed)
	payload = mustJSON(parsed)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(submissionSignature(submissionContextDomain, a.signingKey, payload))
}

// reconcileRequest frames one focus batch against its context receipt.
func reconcileRequest(in request, receipt string, ids ...string) request {
	in.Stage = &stageRequest{SubmissionStageVersion, SubmissionStageReconcile}
	in.ContextReceipt = receipt
	in.Focus = &SubmissionFocus{ClaimIDs: ids}
	return in
}

func fixtureClaim(id, source, target string) claim {
	return claim{ID: id, Source: source, Text: source, Targets: []string{target}, Question: ""}
}

// lowercaseKey builds a distinct, protected-token-free value for index i.
func lowercaseKey(i int) string {
	return "cap" + string([]byte{'a' + byte(i/676%26), 'a' + byte(i/26%26), 'a' + byte(i%26)})
}

func reconcileOp(claimID, target, entryID, field, action, value, finding string, supporting ...string) operation {
	op := operation{ClaimID: claimID, Target: target, EntryID: entryID, Field: field, Action: action, Value: value, Finding: finding}
	if len(supporting) > 0 {
		op.SupportingClaimIDs = supporting
	}
	return op
}

func reconcileOutcome(claimID, kind, reason string, relatedFacts []profiledocument.Reference, relatedClaims ...string) claimOutcome {
	if relatedFacts == nil {
		relatedFacts = []profiledocument.Reference{}
	}
	if relatedClaims == nil {
		relatedClaims = []string{}
	}
	return claimOutcome{ClaimID: claimID, Kind: kind, Reason: reason, RelatedFacts: relatedFacts, RelatedClaimIDs: relatedClaims}
}

// proposalJSON is the bounded generation envelope returned by the fixture model.
func proposalJSON(ops []operation, outcomes []claimOutcome) string {
	return string(mustJSON(proposal{Operations: ops, Outcomes: outcomes}))
}

// A protected reconciliation request classifies the exact complete original with
// Jev, makes exactly one bounded generation call, and accounts only for the
// focused claim: a context-only claim never receives its own operation.
func TestSubmissionReconcileKeepsFocusOnlyAndAccountsContext(t *testing.T) {
	calls := &submissionCalls{}
	in := request{Input: "Alpha beta gamma delta epsilon zeta", Profile: profile()}
	focus := reconcileGID(0, "c1")
	provisional := reconcileGID(0, "c2")
	a := reconcileApp(calls, "professional_fact", proposalJSON(
		[]operation{
			reconcileOp(focus, "skills", "", "skills", "add", "Alphabeta", "addition"),
			reconcileOp(provisional, "skills", "", "skills", "add", "Gammadelta", "addition"),
		},
		[]claimOutcome{
			reconcileOutcome(focus, "change", "new_skill", nil),
			reconcileOutcome(provisional, "unresolved", "provisional_context", nil),
		},
	))
	receipt := sealContext(t, a, in, [][]claim{{fixtureClaim("c1", "Alpha beta", "skills"), fixtureClaim("c2", "Gamma delta", "skills")}})

	w := send(t, a.handler(), reconcileRequest(in, receipt, focus))
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("reconciliation response must not be cached")
	}
	if calls.jev != 1 || calls.extract != 1 {
		t.Fatalf("expected one Jev and one generation call: %+v", calls)
	}
	var got SubmissionReconciliationResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatalf("reconcile status=%d body=%s", w.Code, w.Body.String())
	}
	if len(got.Focus.ClaimIDs) != 1 || got.Focus.ClaimIDs[0] != focus || got.Stage.Kind != SubmissionStageReconcile {
		t.Fatalf("unexpected envelope %+v", got)
	}
	if len(got.Operations) != 1 || got.Operations[0].ClaimID != focus {
		t.Fatalf("focus operations must exclude the context claim: %+v", got.Operations)
	}
	if len(got.Outcomes) != 1 || got.Outcomes[0].ClaimID != focus || got.Outcomes[0].Kind != "change" {
		t.Fatalf("focus outcomes must exclude the context claim: %+v", got.Outcomes)
	}
	var jev struct {
		State struct {
			Submission string `json:"submission"`
			Field      string `json:"field"`
		} `json:"state"`
	}
	if json.Unmarshal([]byte(calls.jevBodies[0]), &jev) != nil || jev.State.Submission != in.Input || jev.State.Field != "professional_information" {
		t.Fatalf("Jev must classify the exact complete original: %s", calls.jevBodies[0])
	}
	body := calls.extractBodies[0]
	if !strings.Contains(body, "provisionalClaims") || !strings.Contains(body, "Focus reconciliation") {
		t.Fatalf("generation request must scope focus against provisional claims: %s", body)
	}
}

// A non-accepting whole-original classification stops before generation and
// returns the existing decision envelope.
func TestSubmissionReconcileStopsBeforeGenerationOnNonAccept(t *testing.T) {
	calls := &submissionCalls{}
	in := request{Input: "Alpha beta gamma", Profile: profile()}
	focus := reconcileGID(0, "c1")
	a := reconcileApp(calls, "irrelevant", `{"operations":[],"outcomes":[]}`)
	receipt := sealContext(t, a, in, [][]claim{{fixtureClaim("c1", "Alpha beta", "skills")}})

	w := send(t, a.handler(), reconcileRequest(in, receipt, focus))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"kind":"irrelevant"`) || strings.Contains(w.Body.String(), "operations") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if calls.jev != 1 || calls.extract != 0 {
		t.Fatalf("non-accept must not dispatch generation: %+v", calls)
	}
}

// A focus batch names at most thirty of the whole submission's aggregate claims.
// Earlier and later claims from other batches remain provisional task data while
// the batch still returns at most sixty bounded operations.
func TestSubmissionReconcileBoundsFocusAcrossBatches(t *testing.T) {
	calls := &submissionCalls{}
	batch0 := make([]claim, 0, 17)
	batch1 := make([]claim, 0, 17)
	for j := 0; j < 17; j++ {
		batch0 = append(batch0, fixtureClaim(fmt.Sprintf("c%02d", j), "Uses "+lowercaseKey(j), "skills"))
		batch1 = append(batch1, fixtureClaim(fmt.Sprintf("c%02d", j), "Uses "+lowercaseKey(j), "skills"))
	}
	var focus []string
	for j := 0; j < 17; j++ {
		focus = append(focus, reconcileGID(0, fmt.Sprintf("c%02d", j)))
	}
	for j := 0; j < 13; j++ {
		focus = append(focus, reconcileGID(1, fmt.Sprintf("c%02d", j)))
	}
	ops := make([]operation, 0, 60)
	outcomes := make([]claimOutcome, 0, 30)
	for k, id := range focus {
		ops = append(ops,
			reconcileOp(id, "skills", "", "skills", "add", lowercaseKey(2*k), "addition"),
			reconcileOp(id, "skills", "", "skills", "add", lowercaseKey(2*k+1), "addition"),
		)
		outcomes = append(outcomes, reconcileOutcome(id, "change", "new_skill", nil))
	}
	in := request{Input: strings.Repeat("Whole submission reconciliation line.\n", 4), Profile: profile()}
	a := reconcileApp(calls, "professional_fact", proposalJSON(ops, outcomes))
	receipt := sealContext(t, a, in, [][]claim{batch0, batch1})

	w := send(t, a.handler(), reconcileRequest(in, receipt, focus...))
	if w.Code != 200 || calls.jev != 1 || calls.extract != 1 {
		t.Fatalf("status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
	}
	var got SubmissionReconciliationResponse
	if json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatal(w.Body.String())
	}
	if len(got.Operations) != 60 || len(got.Operations) > SubmissionBatchOpCap {
		t.Fatalf("expected sixty bounded operations: %d", len(got.Operations))
	}
	if len(got.Outcomes) != 30 {
		t.Fatalf("expected one outcome per focus claim: %d", len(got.Outcomes))
	}
	focused := map[string]bool{}
	for _, id := range focus {
		focused[id] = true
	}
	for _, op := range got.Operations {
		if !focused[op.ClaimID] {
			t.Fatalf("operation leaked a provisional claim: %+v", op)
		}
	}
}

// Every authenticity, binding and focus failure stops before provider dispatch.
func TestSubmissionReconcileRejectsBeforeDispatch(t *testing.T) {
	calls := &submissionCalls{}
	a := reconcileApp(calls, "professional_fact", `{"operations":[],"outcomes":[]}`)
	in := request{Input: strings.Repeat("Reconciliation rejection line.\n", 3), Profile: profile()}
	batch := make([]claim, 0, 31)
	for j := 0; j < 31; j++ {
		batch = append(batch, fixtureClaim(fmt.Sprintf("c%02d", j), "Uses alpha", "skills"))
	}
	receipt := sealContext(t, a, in, [][]claim{batch[:16], batch[16:]})
	var focusAll []string
	for j := 0; j < 16; j++ {
		focusAll = append(focusAll, reconcileGID(0, fmt.Sprintf("c%02d", j)))
	}
	for j := 0; j < 15; j++ {
		focusAll = append(focusAll, reconcileGID(1, fmt.Sprintf("c%02d", j)))
	}
	base := reconcileRequest(in, receipt, reconcileGID(0, "c00"))

	for _, kind := range []string{"signature", "expired", "version", "binding", "input", "profile", "focus-unknown", "focus-duplicate", "focus-empty", "focus-oversized", "receipt-oversized", "field", "missing-key"} {
		t.Run(kind, func(t *testing.T) {
			candidate := base
			candidate.Focus = &SubmissionFocus{ClaimIDs: []string{reconcileGID(0, "c00")}}
			candidateReceipt := receipt
			candidateApp := a
			want := 400
			switch kind {
			case "signature":
				candidateReceipt = tamperReceipt(receipt)
			case "expired":
				candidateReceipt = resignContext(t, a, receipt, func(p *submissionContextReceiptPayload) {
					p.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
				})
			case "version":
				candidateReceipt = resignContext(t, a, receipt, func(p *submissionContextReceiptPayload) { p.Version = "old" })
			case "binding":
				candidateReceipt = resignContext(t, a, receipt, func(p *submissionContextReceiptPayload) {
					p.Binding = "0000000000000000000000000000000000000000000000000000000000000000"
				})
			case "input":
				candidate.Input = in.Input + " extra"
			case "profile":
				candidate.Profile.Skills = "Go"
			case "focus-unknown":
				candidate.Focus = &SubmissionFocus{ClaimIDs: []string{reconcileGID(0, "ghost")}}
			case "focus-duplicate":
				candidate.Focus = &SubmissionFocus{ClaimIDs: []string{reconcileGID(0, "c00"), reconcileGID(0, "c00")}}
			case "focus-empty":
				candidate.Focus = &SubmissionFocus{}
			case "focus-oversized":
				candidate.Focus = &SubmissionFocus{ClaimIDs: focusAll}
			case "receipt-oversized":
				candidateReceipt = strings.Repeat("x", 180000)
			case "field":
				candidate.PlanReceipt = "x"
			case "missing-key":
				candidateApp.signingKey = nil
				want = 503
			}
			candidate.ContextReceipt = candidateReceipt
			before := calls.jev + calls.extract
			w := send(t, candidateApp.handler(), candidate)
			if w.Code != want || calls.jev+calls.extract != before {
				t.Fatalf("kind=%s status=%d want=%d dispatch delta=%d body=%s", kind, w.Code, want, calls.jev+calls.extract-before, w.Body.String())
			}
		})
	}
}

func runReconcile(t *testing.T, generation string, batches [][]claim, focus []string) (int, SubmissionReconciliationResponse, *submissionCalls) {
	t.Helper()
	calls := &submissionCalls{}
	a := reconcileApp(calls, "professional_fact", generation)
	in := request{Input: strings.Repeat("Reconciliation rejection line.\n", 3), Profile: profile()}
	receipt := sealContext(t, a, in, batches)
	w := send(t, a.handler(), reconcileRequest(in, receipt, focus...))
	var got SubmissionReconciliationResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return w.Code, got, calls
}

// The strict provider schema requires supportingClaimIds on every operation. A
// schema-compliant empty array means the operation is supported by its own
// claim; it must not be confused with an owned operation that omits itself from
// an explicit support list.
func TestSubmissionReconcileTreatsEmptySupportingClaimIdsAsOwner(t *testing.T) {
	focus := reconcileGID(0, "c1")
	generation := fmt.Sprintf(
		`{"operations":[{"claimId":%q,"supportingClaimIds":[],"target":"skills","entryId":"","field":"skills","action":"add","value":"Alphabeta","finding":"addition"}],"outcomes":[{"claimId":%q,"kind":"change","reason":"new_skill","relatedFacts":[],"relatedClaimIds":[]}]}`,
		focus, focus,
	)
	code, got, calls := runReconcile(t, generation, [][]claim{{fixtureClaim("c1", "Alpha beta", "skills")}}, []string{focus})
	if code != 200 || calls.extract != 1 {
		t.Fatalf("code=%d calls=%+v body=%+v", code, calls, got)
	}
	if !strings.Contains(calls.extractBodies[0], "supportingClaimIds containing its own claimId") {
		t.Fatal("focus reconciliation prompt must require the owning claim in supportingClaimIds")
	}
	if len(got.Operations) != 1 || got.Operations[0].ClaimID != focus || got.Operations[0].Value != "Alphabeta" {
		t.Fatalf("empty support was not resolved to the owning claim: %+v", got.Operations)
	}
	if len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "change" || len(got.Outcomes[0].OperationIndexes) != 1 {
		t.Fatalf("outcome did not retain the supported operation: %+v", got.Outcomes)
	}
}

// Normalizing an empty provider support list must not weaken the rule for an
// explicit list. An operation that names another claim but omits its owner is
// still withheld.
func TestSubmissionReconcileRejectsSupportThatOmitsOwner(t *testing.T) {
	focus := reconcileGID(0, "c1")
	other := reconcileGID(0, "c2")
	generation := fmt.Sprintf(
		`{"operations":[{"claimId":%q,"supportingClaimIds":[%q],"target":"skills","entryId":"","field":"skills","action":"add","value":"Alphabeta","finding":"addition"}],"outcomes":[{"claimId":%q,"kind":"change","reason":"new_skill","relatedFacts":[],"relatedClaimIds":[]}]}`,
		focus, other, focus,
	)
	code, got, _ := runReconcile(t, generation, [][]claim{{fixtureClaim("c1", "Alpha beta", "skills"), fixtureClaim("c2", "Gamma delta", "skills")}}, []string{focus})
	if code != 200 || len(got.Operations) != 0 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "unresolved" {
		t.Fatalf("code=%d ops=%+v outcomes=%+v", code, got.Operations, got.Outcomes)
	}
}

// Dangling, out-of-focus and unsupported focus operations are withheld and leave
// the affected focus claim unresolved; malformed and oversized generations are
// rejected rather than truncated.
func TestSubmissionReconcileRejectsBadFocusOperations(t *testing.T) {
	focusClaim := fixtureClaim("c1", "Alpha beta", "skills")
	change := reconcileOutcome(reconcileGID(0, "c1"), "change", "new_skill", nil)

	t.Run("dangling-claim", func(t *testing.T) {
		code, got, calls := runReconcile(t,
			proposalJSON([]operation{reconcileOp("ghost", "skills", "", "skills", "add", "Alphabeta", "addition")}, []claimOutcome{change}),
			[][]claim{{focusClaim}}, []string{reconcileGID(0, "c1")})
		if code != 200 || len(got.Operations) != 0 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "unresolved" || calls.extract != 1 {
			t.Fatalf("code=%d ops=%+v outcomes=%+v calls=%+v", code, got.Operations, got.Outcomes, calls)
		}
	})

	t.Run("out-of-focus-claim", func(t *testing.T) {
		code, got, calls := runReconcile(t,
			proposalJSON(
				[]operation{reconcileOp(reconcileGID(0, "c2"), "skills", "", "skills", "add", "Gammadelta", "addition")},
				[]claimOutcome{change},
			),
			[][]claim{{focusClaim, fixtureClaim("c2", "Gamma delta", "skills")}}, []string{reconcileGID(0, "c1")})
		if code != 200 || len(got.Operations) != 0 || len(got.Outcomes) != 1 || got.Outcomes[0].ClaimID != reconcileGID(0, "c1") || got.Outcomes[0].Kind != "unresolved" || calls.extract != 1 {
			t.Fatalf("code=%d ops=%+v outcomes=%+v calls=%+v", code, got.Operations, got.Outcomes, calls)
		}
	})

	t.Run("invented-entry", func(t *testing.T) {
		code, got, _ := runReconcile(t,
			proposalJSON([]operation{reconcileOp(reconcileGID(0, "c1"), "projects", "invented", "name", "add", "Harbor", "addition")}, []claimOutcome{change}),
			[][]claim{{fixtureClaim("c1", "Alpha beta", "projects")}}, []string{reconcileGID(0, "c1")})
		if code != 200 || len(got.Operations) != 0 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "unresolved" {
			t.Fatalf("code=%d ops=%+v outcomes=%+v", code, got.Operations, got.Outcomes)
		}
	})

	t.Run("unsupported-qualifier", func(t *testing.T) {
		code, got, _ := runReconcile(t,
			proposalJSON([]operation{reconcileOp(reconcileGID(0, "c1"), "skills", "", "skills", "add", "Kubernetes", "addition")}, []claimOutcome{change}),
			[][]claim{{focusClaim}}, []string{reconcileGID(0, "c1")})
		if code != 200 || len(got.Operations) != 0 || len(got.Outcomes) != 1 || got.Outcomes[0].Kind != "unresolved" {
			t.Fatalf("code=%d ops=%+v outcomes=%+v", code, got.Operations, got.Outcomes)
		}
	})

	t.Run("malformed-shape", func(t *testing.T) {
		code, _, calls := runReconcile(t, `{"operations":5}`, [][]claim{{focusClaim}}, []string{reconcileGID(0, "c1")})
		if code != 502 || calls.extract != 1 {
			t.Fatalf("code=%d calls=%+v", code, calls)
		}
	})

	t.Run("operation-cap", func(t *testing.T) {
		ops := make([]operation, 0, 61)
		for i := 0; i < 61; i++ {
			ops = append(ops, reconcileOp(reconcileGID(0, "c1"), "skills", "", "skills", "add", lowercaseKey(i), "addition"))
		}
		code, _, calls := runReconcile(t, proposalJSON(ops, nil), [][]claim{{focusClaim}}, []string{reconcileGID(0, "c1")})
		if code != 400 || calls.extract != 1 {
			t.Fatalf("code=%d calls=%+v", code, calls)
		}
	})

	t.Run("outcome-cap", func(t *testing.T) {
		outcomes := make([]claimOutcome, 0, 31)
		for i := 0; i < 31; i++ {
			outcomes = append(outcomes, reconcileOutcome(reconcileGID(0, "c1"), "unresolved", "no_validated_disposition", nil))
		}
		code, _, calls := runReconcile(t, proposalJSON([]operation{}, outcomes), [][]claim{{focusClaim}}, []string{reconcileGID(0, "c1")})
		if code != 502 || calls.extract != 1 {
			t.Fatalf("code=%d calls=%+v", code, calls)
		}
	})
}

// Exact duplicate, additional support, overlap, correction, contradiction,
// unsupported and unresolved stay separate across batches, context-only claims
// only support a focus decision, and conflicting statements are never resolved
// by batch order.
func TestSubmissionReconcileKeepsDispositionsSeparate(t *testing.T) {
	calls := &submissionCalls{}
	doc := ledgerDocument(profile())
	in := request{Input: strings.Repeat("Reconciliation dispositions line.\n", 3), Profile: profile(), Document: mustJSON(doc)}
	a1, a2, a3 := reconcileGID(0, "e1"), reconcileGID(0, "e2"), reconcileGID(0, "e3")
	actx := reconcileGID(0, "ctx")
	a4, a5, a6, a7, a8 := reconcileGID(1, "e4"), reconcileGID(1, "e5"), reconcileGID(1, "e6"), reconcileGID(1, "e7"), reconcileGID(1, "e8")
	skill := []profiledocument.Reference{{ProfileID: "profile", ID: "profile/skills", Revision: 1}}
	a := reconcileApp(calls, "professional_fact", proposalJSON(
		[]operation{
			reconcileOp(a2, "skills", "", "skills", "evidence", "React", "in_place"),
			reconcileOp(a3, "skills", "", "skills", "add", "Gammadelta", "overlap"),
			reconcileOp(a8, "skills", "", "skills", "add", "Sigmatau", "addition", a8, actx),
		},
		[]claimOutcome{
			reconcileOutcome(a1, "exact_duplicate", "verified_exact_alias_or_wording", skill),
			reconcileOutcome(a2, "additional_support", "new_supporting_evidence", skill),
			reconcileOutcome(a3, "overlap", "retains_distinct_detail", nil, a4),
			reconcileOutcome(a4, "contradiction", "competing_statement", nil, a3),
			reconcileOutcome(a5, "correction", "later_correction", nil, a4),
			reconcileOutcome(a6, "unsupported", "no_source_support", nil),
			reconcileOutcome(a7, "unresolved", "no_validated_disposition", nil),
			reconcileOutcome(a8, "change", "new_skill", nil),
		},
	))
	receipt := sealContext(t, a, in, [][]claim{
		{fixtureClaim("e1", "React", "skills"), fixtureClaim("e2", "React", "skills"), fixtureClaim("e3", "Gamma delta", "skills"), fixtureClaim("ctx", "Theta iota", "skills")},
		{fixtureClaim("e4", "Kappa lambda", "skills"), fixtureClaim("e5", "Mu nu", "skills"), fixtureClaim("e6", "Xi omicron", "skills"), fixtureClaim("e7", "Pi rho", "skills"), fixtureClaim("e8", "Sigma tau", "skills")},
	})
	w := send(t, a.handler(), reconcileRequest(in, receipt, a1, a2, a3, a4, a5, a6, a7, a8))
	if w.Code != 200 || calls.jev != 1 || calls.extract != 1 {
		t.Fatalf("status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
	}
	var got SubmissionReconciliationResponse
	if json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatal(w.Body.String())
	}
	kinds := map[string]string{}
	for _, o := range got.Outcomes {
		kinds[o.ClaimID] = o.Kind
	}
	want := map[string]string{a1: "exact_duplicate", a2: "additional_support", a3: "overlap", a4: "contradiction", a5: "correction", a6: "unsupported", a7: "unresolved", a8: "change"}
	if len(kinds) != len(want) {
		t.Fatalf("unexpected outcomes %+v", got.Outcomes)
	}
	for id, kind := range want {
		if kinds[id] != kind {
			t.Fatalf("claim %s kind=%q want=%q", id, kinds[id], kind)
		}
	}
	if len(got.Operations) != 3 {
		t.Fatalf("only supported dispositions carry operations: %+v", got.Operations)
	}
	owners := map[string]operation{}
	for _, op := range got.Operations {
		owners[op.ClaimID] = op
		if op.ClaimID == actx {
			t.Fatalf("context-only claim received an operation: %+v", op)
		}
	}
	if _, ok := owners[a4]; ok {
		t.Fatal("contradiction must not silently resolve into a change")
	}
	if _, ok := owners[a5]; ok {
		t.Fatal("correction must not silently resolve into a change")
	}
	supported, ok := owners[a8]
	if !ok || len(supported.SupportingClaimIDs) != 2 || supported.SupportingClaimIDs[1] != actx {
		t.Fatalf("focus change must accept a context-only supporting claim: %+v", supported)
	}
}

// A provisional claim from an earlier focus batch is an established anchor, so a
// later focus batch may add detail to its stable new-entry anchor without
// re-supplying the whole identity.
func TestSubmissionReconcileEstablishesAnchorAcrossFocusBatches(t *testing.T) {
	calls := &submissionCalls{}
	in := request{Input: strings.Repeat("Anchor reconciliation line.\n", 3), Profile: profile()}
	anchor := reconcileGID(0, "anchor")
	detail := reconcileGID(1, "detail")
	a := reconcileApp(calls, "professional_fact", proposalJSON(
		[]operation{reconcileOp(detail, "experience", "new:"+anchor, "responsibilities", "add", "built alpha systems", "addition")},
		[]claimOutcome{reconcileOutcome(detail, "change", "new_detail", nil)},
	))
	receipt := sealContext(t, a, in, [][]claim{
		{fixtureClaim("anchor", "Acme Corp Beta", "experience")},
		{fixtureClaim("detail", "Built Alpha systems", "experience")},
	})
	w := send(t, a.handler(), reconcileRequest(in, receipt, detail))
	if w.Code != 200 || calls.extract != 1 {
		t.Fatalf("status=%d calls=%+v body=%s", w.Code, calls, w.Body.String())
	}
	var got SubmissionReconciliationResponse
	if json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatal(w.Body.String())
	}
	if len(got.Operations) != 1 || got.Operations[0].EntryID != "new:"+anchor {
		t.Fatalf("established anchor was not reused across focus batches: %+v", got.Operations)
	}
}

// Oversized provider context is rejected as `capacity`, never silently
// truncated, before any generation call.
func TestSubmissionReconcileRejectsOversizedProviderContext(t *testing.T) {
	calls := &submissionCalls{}
	a := reconcileApp(calls, "professional_fact", `{"operations":[],"outcomes":[]}`)
	p := profilevalidation.Profile{Skills: "React", Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
	for i := 0; i < 40; i++ {
		p.Experience = append(p.Experience, profilevalidation.Experience{
			ID: fmt.Sprintf("x%d", i), Company: "Company", Title: "Role",
			Description: strings.Repeat("a", 2400), Responsibilities: strings.Repeat("b", 2400), Achievements: strings.Repeat("c", 2400),
		})
	}
	in := request{Input: strings.Repeat("Experience reconciliation line.\n", 3), Profile: p}
	receipt := sealContext(t, a, in, [][]claim{{fixtureClaim("c1", "Experience source", "experience")}})
	body, _ := json.Marshal(reconcileRequest(in, receipt, reconcileGID(0, "c1")))
	if len(body) > maxStageBody {
		t.Fatalf("fixture body %d exceeds the transport bound", len(body))
	}
	w := send(t, a.handler(), reconcileRequest(in, receipt, reconcileGID(0, "c1")))
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"`+SubmissionErrCapacity+`"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if calls.jev != 1 || calls.extract != 0 {
		t.Fatalf("oversized provider context must not reach generation: %+v", calls)
	}
}
