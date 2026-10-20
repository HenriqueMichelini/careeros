package profileingestion

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"professional-information-repo/internal/profiledocument"
)

// submissionClarifyFields names the exact transport keys a protected focused
// clarification accepts. Any other key is `input`. The complete original remains
// the primary Jev gate; the answer is classified independently.
var submissionClarifyFields = []string{"stage", "input", "profile", "document", "contextReceipt", "claim", "answer"}

// SubmissionClarificationResponse replaces exactly one authenticated claim. The
// client keeps every other extracted claim, outcome and selection, and merges
// only this claim's revised operations/outcome into its review.
type SubmissionClarificationResponse struct {
	Stage      SubmissionStage `json:"stage"`
	Claim      claim           `json:"claim"`
	Focus      SubmissionFocus `json:"focus"`
	Operations []operation     `json:"operations"`
	Outcomes   []claimOutcome  `json:"outcomes"`
}

// validSubmissionClarification validates the selected statement, question,
// prior support and answer against the original submission and the retained
// legacy limits. The claim id is authenticated against the signed context before
// this check, so the legacy 40-byte temporary id bound does not apply to the v2
// global id.
func validSubmissionClarification(input string, c claim, answer string) bool {
	if c.ID == "" || strings.TrimSpace(c.Source) == "" || len(c.Source) > 1000 || !strings.Contains(input, c.Source) || len(c.Text) > 1000 || len(c.Question) > 500 || len(c.SupportingSources) > 12 || len(c.Targets) > 18 || !utf8.ValidString(answer) || strings.TrimSpace(answer) == "" || len(answer) > SubmissionClarifyAnswerBytes {
		return false
	}
	for _, s := range c.SupportingSources {
		if s.Source == "" || len(s.Source) > 1000 || (s.Origin != "" && s.Origin != "clarification_answer") || (s.Origin == "" && !strings.Contains(input, s.Source)) {
			return false
		}
	}
	return true
}

// submissionClarificationScope finds the selected claim in the authenticated
// aggregate and validates its immutable statement identity. A revised claim may
// already carry earlier clarification-answer support, so the returned claim
// retains every authenticated original support, keeps only structurally valid
// answer-origin additions and drops duplicate evidence. It never borrows an
// original support from another claim.
func submissionClarificationScope(receipt submissionContextReceiptPayload, selected claim) (claim, claim, string) {
	byID := map[string]claim{}
	for _, batch := range receipt.Batches {
		for _, c := range batch.Claims {
			if _, seen := byID[c.ID]; seen {
				continue
			}
			byID[c.ID] = c
		}
	}
	original, ok := byID[selected.ID]
	if !ok ||
		selected.Source != original.Source ||
		string(canonicalForm(selected.SourceReference)) != string(canonicalForm(original.SourceReference)) ||
		len(selected.SupportingSources) > 12 {
		return claim{}, claim{}, SubmissionErrFocus
	}
	originalSupport := map[string]bool{}
	for _, support := range original.SupportingSources {
		originalSupport[string(canonicalForm(support))] = true
	}
	selectedSupport := map[string]bool{}
	for _, support := range selected.SupportingSources {
		key := string(canonicalForm(support))
		if support.Origin == "" {
			if !originalSupport[key] {
				return claim{}, claim{}, SubmissionErrFocus
			}
		} else if support.Origin != "clarification_answer" || support.Source == "" || len(support.Source) > 1000 {
			return claim{}, claim{}, SubmissionErrFocus
		}
		selectedSupport[key] = true
	}
	for key := range originalSupport {
		if !selectedSupport[key] {
			return claim{}, claim{}, SubmissionErrFocus
		}
	}
	merged := selected
	merged.ID = original.ID
	merged.Source = original.Source
	merged.SourceReference = original.SourceReference
	merged.SupportingSources = make([]supportingSource, 0, len(original.SupportingSources)+len(selected.SupportingSources))
	seen := map[string]bool{}
	for _, support := range append(append([]supportingSource{}, original.SupportingSources...), selected.SupportingSources...) {
		key := string(canonicalForm(support))
		if seen[key] {
			continue
		}
		seen[key] = true
		merged.SupportingSources = append(merged.SupportingSources, support)
	}
	if len(merged.SupportingSources) > 12 {
		return claim{}, claim{}, SubmissionErrFocus
	}
	return original, merged, ""
}

// submissionClarificationClaims replaces the authenticated claim in the complete
// aggregate and marks every other claim provisional. The revised claim is the
// only focus claim, so its outcome cannot borrow another claim's operation.
func submissionClarificationClaims(receipt submissionContextReceiptPayload, revised claim) (focus []claim, universe, provisionalClaims []claim, provisional map[string]bool) {
	seen := map[string]bool{}
	focus = []claim{revised}
	provisional = map[string]bool{}
	provisionalClaims = []claim{}
	for _, batch := range receipt.Batches {
		for _, c := range batch.Claims {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			if c.ID == revised.ID {
				c = revised
			} else {
				provisional[c.ID] = true
				provisionalClaims = append(provisionalClaims, c)
			}
			universe = append(universe, c)
		}
	}
	return focus, universe, provisionalClaims, provisional
}

// submissionClarify verifies the signed context and exact request binding, the
// selected claim identity, both whole-original and answer Jev gates, revises only
// that claim, then runs the established focused reconciliation. It makes no
// automatic retry or model fallback, stores nothing and never saves.
func (a app) submissionClarify(ctx context.Context, w http.ResponseWriter, r *http.Request, in request, raw []byte, key string, stage SubmissionStage, status *int, outcome *string, fail func(int, string)) {
	if code := SubmissionFieldsError(raw, submissionClarifyFields...); code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if len(a.signingKey) < 32 {
		fail(503, "configuration")
		return
	}
	if in.Claim == nil || in.Answer == nil {
		fail(400, SubmissionErrInput)
		return
	}
	if _, ok := submissionDocument(in); !ok {
		fail(400, SubmissionErrInput)
		return
	}
	var document *profiledocument.Document
	if len(in.Document) > 0 {
		decoded, err := profiledocument.Decode(in.Document)
		if err != nil || !documentMatchesProfile(decoded, in.Profile) {
			fail(400, SubmissionErrInput)
			return
		}
		document = &decoded
	}
	var receipt submissionContextReceiptPayload
	if httpStatus, code := a.openVerifiedReceipt(submissionContextDomain, in.ContextReceipt, in, &receipt); code != "" {
		fail(httpStatus, code)
		return
	}
	plan, aggregate := submissionFocusContext(receipt, in.Input)
	if code := SubmissionManifestError(plan, in.Input); code != "" {
		fail(400, SubmissionErrContext)
		return
	}
	if code := SubmissionAggregateError(aggregate, plan); code != "" {
		fail(400, SubmissionErrContext)
		return
	}
	original, selected, code := submissionClarificationScope(receipt, *in.Claim)
	if code != "" {
		fail(SubmissionErrorStatus(code), code)
		return
	}
	if !validSubmissionClarification(in.Input, selected, *in.Answer) {
		fail(400, SubmissionErrInput)
		return
	}
	if ctx.Err() != nil {
		fail(504, "timeout")
		return
	}
	// The complete original remains the primary gate. The independently elicited
	// answer is classified separately and cannot launder a blocked original.
	if !a.submissionDecision(ctx, w, r, in, status, outcome) {
		return
	}
	clarification := clarificationRequest{Claim: selected, Answer: *in.Answer}
	if !a.submissionClassify(ctx, w, r, clarificationValidationText(clarification), status, outcome) {
		return
	}
	ledger := &reconciliationContext{document: document, skipped: []skippedClaim{}}
	revisedClaims, skipped, code := a.clarify(ctx, key, clarification, ledger)
	if code != "" {
		fail(stageCodeStatus(code), code)
		return
	}
	if skipped != 0 || len(revisedClaims) != 1 {
		fail(502, "invalid_output")
		return
	}
	revised := revisedClaims[0]
	if revised.ID != original.ID || revised.Source != original.Source || string(canonicalForm(revised.SourceReference)) != string(canonicalForm(original.SourceReference)) {
		fail(502, "invalid_output")
		return
	}
	// Provenance is part of the protected contract: prior support is retained and
	// the answer is supplied only with the clarification origin.
	prior := map[string]bool{}
	for _, support := range selected.SupportingSources {
		prior[string(canonicalForm(support))] = true
	}
	answerSeen := false
	for _, support := range revised.SupportingSources {
		if support.Origin == "clarification_answer" && support.Source == *in.Answer {
			answerSeen = true
			continue
		}
		if !prior[string(canonicalForm(support))] {
			fail(502, "invalid_output")
			return
		}
	}
	if !answerSeen || len(revised.SupportingSources) > 12 {
		fail(502, "invalid_output")
		return
	}
	focus, universe, provisionalClaims, provisional := submissionClarificationClaims(receipt, revised)
	ops, outcomes, code := a.submissionCompare(ctx, key, focus, universe, provisionalClaims, provisional, in.Profile, document, ledger)
	if code != "" {
		fail(stageCodeStatus(code), code)
		return
	}
	if ctx.Err() != nil {
		fail(504, "timeout")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(SubmissionClarificationResponse{
		Stage:      stage,
		Claim:      revised,
		Focus:      SubmissionFocus{ClaimIDs: []string{revised.ID}},
		Operations: ops,
		Outcomes:   outcomes,
	})
}
