package profileingestion

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"

	"professional-information-repo/internal/preprocessing"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/profilevalidation"
)

type claimOutcome struct {
	ClaimID          string                      `json:"claimId"`
	Kind             string                      `json:"kind"`
	Reason           string                      `json:"reason"`
	RelatedFacts     []profiledocument.Reference `json:"relatedFacts"`
	RelatedClaimIDs  []string                    `json:"relatedClaimIds"`
	OperationIndexes []int                       `json:"operationIndexes,omitempty"`
}
type skippedClaim struct {
	Index     int    `json:"index"`
	Reason    string `json:"reason"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	Shortened bool   `json:"shortened"`
}
type processingCoverage struct {
	ValidClaims       int    `json:"validClaims"`
	InvalidClaims     int    `json:"invalidClaims"`
	DiscoveryComplete bool   `json:"discoveryComplete"`
	Capacity          string `json:"capacity"`
}
type comparisonFact struct {
	Reference profiledocument.Reference  `json:"reference"`
	Fact      profiledocument.Fact       `json:"fact"`
	Identity  []profiledocument.Fact     `json:"identity"`
	Evidence  []profiledocument.Evidence `json:"acceptedEvidence"`
}
type reconciliationContext struct {
	extractionInstruction string
	comparisonInstruction string
	view                  *preprocessing.View
	document              *profiledocument.Document
	candidates            []comparisonFact
	proposed              []claimOutcome
	outcomes              []claimOutcome
	skipped               []skippedClaim
	rejected              map[string]bool
}

var outcomeKinds = []string{"change", "exact_duplicate", "overlap", "additional_support", "contradiction", "correction", "clarification", "unsupported", "unresolved"}

const outcomeInstructions = `
Return outcomes for EVERY claim, including claims with no operation. Each outcome: {claimId,kind,reason,relatedFacts:[{profileId,id,revision}],relatedClaimIds:[]}. kind is change, exact_duplicate, overlap, additional_support, contradiction, correction, clarification, unsupported or unresolved. reason explains the comparison in the source language. Reference only stable existingFacts references and known claims; no invented references. A missing operation NEVER proves a duplicate. exact_duplicate requires identical wording or verified exact JavaScript/Javascript or TypeScript/Typescript aliases AND identical meaning, owner, role, employer and period. Spring Framework and Spring Boot, AWS and each service, and similarly named employers are distinct. Negated, aspirational, uncertain and temporally different claims are not equivalent. relatedClaimIds identifies competing or supporting submitted statements. overlap must retain all distinct information. additional_support may emit an evidence operation with the existing whole field value, target/entryId/field of that fact, when the claim repeats an assertion with new supporting evidence not already in acceptedEvidence; this does not change wording. Contradiction and correction/supersession are review candidates: emit no change and preserve both statements until the user resolves them. Ambiguous identity requires clarification, never silently merge distinct entries. Every comparison outcome needs relevant stable facts or related claim IDs; absent stable references use unresolved. Never choose a correction automatically. No operation for unsupported or unresolved claims.`

func coverageFor(valid, invalid int) processingCoverage {
	capacity := "within_limit"
	if valid+invalid == 30 {
		capacity = "possibly_exhausted"
	}
	return processingCoverage{ValidClaims: valid, InvalidClaims: invalid, Capacity: capacity}
}
func outcomeSchema() map[string]any {
	return objectSchema(map[string]any{
		"claimId": stringSchema(), "kind": map[string]any{"type": "string", "enum": outcomeKinds}, "reason": stringSchema(),
		"relatedFacts":    arraySchema(objectSchema(map[string]any{"profileId": stringSchema(), "id": stringSchema(), "revision": map[string]any{"type": "integer"}}, "profileId", "id", "revision")),
		"relatedClaimIds": arraySchema(stringSchema()),
	}, "claimId", "kind", "reason", "relatedFacts", "relatedClaimIds")
}
func proposalShape(raw []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || (len(fields) != 1 && len(fields) != 2) || fields["operations"] == nil {
		return false
	}
	var ops []json.RawMessage
	if json.Unmarshal(fields["operations"], &ops) != nil || string(fields["operations"]) == "null" {
		return false
	}
	if len(fields) == 2 {
		var outcomes []json.RawMessage
		return fields["outcomes"] != nil && string(fields["outcomes"]) != "null" && json.Unmarshal(fields["outcomes"], &outcomes) == nil
	}
	return true // Transitional operation-only responses receive unresolved outcomes.
}
func documentMatchesProfile(doc profiledocument.Document, p profilevalidation.Profile) bool {
	var view map[string]any
	_ = json.Unmarshal(mustJSON(p), &view)
	for _, f := range doc.Facts {
		var value any
		_ = json.Unmarshal(f.Value, &value)
		if f.Owner.ID == doc.ID {
			if !reflect.DeepEqual(view[f.Field], value) {
				return false
			}
		} else {
			var entity *profiledocument.Entity
			for i := range doc.Entities {
				if doc.Entities[i].ID == f.Owner.ID {
					entity = &doc.Entities[i]
					break
				}
			}
			if entity == nil {
				return false
			}
			found := false
			entries, _ := view[entity.Kind].([]any)
			for _, raw := range entries {
				e, _ := raw.(map[string]any)
				if e["id"] == entity.LegacyID {
					found = reflect.DeepEqual(e[f.Field], value)
				}
			}
			if !found {
				return false
			}
		}
	}
	// Reject extra nonempty compatibility fields or entries absent in the snapshot.
	for field, value := range view {
		if entries, ok := value.([]any); ok {
			for _, raw := range entries {
				e, _ := raw.(map[string]any)
				for key, v := range e {
					if key == "id" || v == "" || v == false {
						continue
					}
					found := false
					for _, entity := range doc.Entities {
						if entity.Kind == field && entity.LegacyID == e["id"] {
							for _, f := range doc.Facts {
								if f.Owner.ID == entity.ID && f.Field == key {
									found = true
								}
							}
						}
					}
					if !found {
						return false
					}
				}
			}
		} else if value != "" && value != nil {
			found := false
			for _, f := range doc.Facts {
				if f.Owner.ID == doc.ID && f.Field == field {
					found = true
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}
func candidateFacts(claims []claim, p profilevalidation.Profile, doc *profiledocument.Document) []comparisonFact {
	out := []comparisonFact{}
	if doc == nil {
		return out
	}
	var projected map[string]any
	_ = json.Unmarshal(mustJSON(projection(claims, p)), &projected)
	for _, f := range doc.Facts {
		var value string
		if json.Unmarshal(f.Value, &value) != nil || value == "" {
			continue
		}
		fieldValue, visible := projected[f.Field]
		if f.Owner.ID == doc.ID {
			if !visible {
				fieldValue, visible = projected["related_"+f.Field]
			}
		} else {
			visible = false
			for _, e := range doc.Entities {
				if e.ID != f.Owner.ID {
					continue
				}
				raw, _ := projected[e.Kind]
				entries, _ := raw.([]any)
				for _, rawEntry := range entries {
					entry, _ := rawEntry.(map[string]any)
					if entry["id"] == e.LegacyID {
						fieldValue, visible = entry[f.Field]
					}
				}
			}
		}
		if !visible || fieldValue != value {
			continue
		}
		identity := []profiledocument.Fact{}
		for _, other := range doc.Facts {
			contextOwner := other.Owner.ID == f.Owner.ID
			for _, context := range f.Context {
				contextOwner = contextOwner || context.ID == other.Owner.ID
			}
			if contextOwner && identityField(other.Field) {
				identity = append(identity, other)
			}
		}
		evidence := []profiledocument.Evidence{}
		for _, link := range doc.Links {
			if link.Kind == "supports" && link.State == "active" && link.From.ID == f.ID && link.From.Revision == f.Revision {
				for _, excerpt := range doc.Evidence {
					if excerpt.ID == link.To.ID && excerpt.Revision == link.To.Revision {
						evidence = append(evidence, excerpt)
					}
				}
			}
		}
		out = append(out, comparisonFact{Reference: profiledocument.Reference{ProfileID: doc.ID, ID: f.ID, Revision: f.Revision}, Fact: f, Identity: identity, Evidence: evidence})
	}
	return out
}
func identityField(field string) bool {
	return contains([]string{"company", "title", "startDate", "endDate", "current", "name", "degree", "institution", "issuer"}, field)
}
func exactClaimValue(c claim) string {
	if v := explicitUseSkill(c); v != "" {
		return exactAlias(v)
	}
	return exactAlias(strings.TrimSpace(c.Source))
}
func exactFact(c claim, f comparisonFact) bool {
	var value string
	if json.Unmarshal(f.Fact.Value, &value) != nil {
		return false
	}
	candidate := exactClaimValue(c)
	found := exactAlias(value) == candidate
	if f.Fact.Owner.ID == f.Reference.ProfileID && contains([]string{"skills", "tools", "competencies"}, f.Fact.Field) && len(c.Targets) == 1 && contains([]string{"skills", "tools", "competencies"}, c.Targets[0]) && len(f.Fact.Context) == 0 {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '\n' || r == ',' || r == ';' }) {
			if exactAlias(strings.TrimSpace(part)) == candidate {
				found = true
			}
		}
	}
	if !found || c.Question != "" {
		return false
	}
	if f.Fact.Kind == "statement" && c.Meaning != nil {
		m := c.Meaning
		if m.Assertion != f.Fact.Assertion || m.Intent != f.Fact.Intent || m.Certainty != f.Fact.Certainty || m.Temporal.Wording != f.Fact.Temporal.Wording || m.Temporal.Precision != f.Fact.Temporal.Precision {
			return false
		}
	} else if c.Meaning != nil {
		if c.Meaning.Assertion != "affirmed" || c.Meaning.Intent != "actual" || c.Meaning.Certainty != "certain" || c.Meaning.Temporal.Wording != "" {
			return false
		}
	}
	if f.Fact.Kind == "statement" && c.Meaning == nil && (f.Fact.Assertion != "affirmed" || f.Fact.Intent != "actual" || f.Fact.Certainty != "certain" || f.Fact.Temporal.Wording != "") {
		return false
	}
	if f.Fact.Owner.ID != f.Reference.ProfileID || len(f.Fact.Context) > 0 {
		if !identitySupported(c, f.Identity) {
			return false
		}
	}
	return true
}

func identitySupported(c claim, identity []profiledocument.Fact) bool {
	source := c.Source
	for _, support := range c.SupportingSources {
		source += "\n" + support.Source
	}
	values := []string{}
	for _, fact := range identity {
		var value string
		_ = json.Unmarshal(fact.Value, &value)
		if value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !identityOccurrence(source, value, values) {
			return false
		}
	}
	return true
}
func defaultOutcome(c claim) claimOutcome {
	return claimOutcome{ClaimID: c.ID, Kind: "unresolved", Reason: "no_validated_disposition", RelatedFacts: []profiledocument.Reference{}, RelatedClaimIDs: []string{}}
}
func reconcileOutcomes(claims []claim, ops []operation, unresolved []string, l *reconciliationContext) ([]operation, []claimOutcome, []string) {
	outcomes := make([]claimOutcome, 0, len(claims))
	for _, c := range claims {
		o := defaultOutcome(c)
		proposed := []claimOutcome{}
		for _, p := range l.proposed {
			if p.ClaimID == c.ID {
				proposed = append(proposed, p)
			}
		}
		if len(proposed) == 1 && validOutcome(proposed[0], claims, l.candidates) {
			o = proposed[0]
		}
		if c.Question != "" && !contains([]string{"contradiction", "correction"}, o.Kind) {
			o.Kind, o.Reason = "clarification", "source_ambiguity"
		}
		if l.rejected[c.ID] && o.Kind != "contradiction" && o.Kind != "correction" && o.Kind != "clarification" && o.Kind != "unsupported" {
			o = defaultOutcome(c)
		}
		if o.Kind == "exact_duplicate" || o.Kind == "additional_support" {
			matched := len(o.RelatedFacts) > 0
			for _, r := range o.RelatedFacts {
				match := false
				for _, f := range l.candidates {
					if f.Reference == r && exactFact(c, f) {
						match = true
					}
				}
				matched = matched && match
			}
			if !matched {
				o = defaultOutcome(c)
			}
			if matched && o.Kind == "additional_support" && !hasNewSupport(c, o, l.candidates) {
				o.Kind, o.Reason = "exact_duplicate", "support_already_retained"
			}
		}
		if contains([]string{"change", "overlap"}, o.Kind) {
			for _, f := range l.candidates {
				if exactFact(c, f) {
					o.Kind, o.Reason = "exact_duplicate", "verified_exact_alias_or_wording"
					o.RelatedFacts = []profiledocument.Reference{f.Reference}
					break
				}
			}
		}
		// Only verified exact matches can establish already represented without a patch.
		if o.Kind == "unresolved" && !l.rejected[c.ID] && len(l.proposed) == 0 {
			for _, f := range l.candidates {
				if exactFact(c, f) {
					o.Kind, o.Reason = "exact_duplicate", "verified_exact_alias_or_wording"
					o.RelatedFacts = append(o.RelatedFacts, f.Reference)
				}
			}
		}
		outcomes = append(outcomes, o)
	}
	filtered := make([]operation, 0, len(ops))
	for _, op := range ops {
		var outcome *claimOutcome
		for i := range outcomes {
			if outcomes[i].ClaimID == op.ClaimID {
				outcome = &outcomes[i]
			}
		}
		if outcome == nil {
			continue
		}
		if contains([]string{"contradiction", "correction", "clarification", "unsupported", "exact_duplicate"}, outcome.Kind) {
			continue
		}
		if len(l.proposed) > 0 && outcome.Kind == "unresolved" {
			continue
		}
		if l.document != nil && op.EntryID != "" && !strings.HasPrefix(op.EntryID, "new:") && !safeOwner(op, claims, l) {
			outcome.Kind, outcome.Reason = "clarification", "identity_not_established"
			continue
		}
		if op.Action == "evidence" && (outcome.Kind != "additional_support" || !evidenceOperation(op, *outcome, l)) {
			outcome.Kind, outcome.Reason = "unresolved", "invalid_evidence_update"
			continue
		}
		index := len(filtered)
		filtered = append(filtered, op)
		outcome.OperationIndexes = append(outcome.OperationIndexes, index)
		for _, f := range l.candidates {
			if f.Fact.Field != op.Field {
				continue
			}
			ownerMatches := f.Fact.Owner.ID == f.Reference.ProfileID && op.EntryID == "" && op.Target == op.Field
			if l.document != nil {
				for _, e := range l.document.Entities {
					ownerMatches = ownerMatches || (e.ID == f.Fact.Owner.ID && e.Kind == op.Target && e.LegacyID == op.EntryID)
				}
			}
			if ownerMatches {
				found := false
				for _, r := range outcome.RelatedFacts {
					found = found || r == f.Reference
				}
				if !found && len(outcome.RelatedFacts) < 12 {
					outcome.RelatedFacts = append(outcome.RelatedFacts, f.Reference)
				}
			}
		}
		if op.Action != "evidence" {
			if op.Finding == "overlap" && len(outcome.RelatedFacts)+len(outcome.RelatedClaimIDs) > 0 {
				outcome.Kind = "overlap"
			} else {
				outcome.Kind = "change"
			}
			if outcome.Reason == "no_validated_disposition" {
				outcome.Reason = "validated_operation"
			}
		}
	}
	filtered = completeOutcomeOperations(filtered, claims, outcomes)
	for i := range outcomes {
		outcomes[i].OperationIndexes = nil
	}
	for index, op := range filtered {
		for i := range outcomes {
			if outcomes[i].ClaimID == op.ClaimID {
				outcomes[i].OperationIndexes = append(outcomes[i].OperationIndexes, index)
			}
		}
	}
	unresolved = []string{}
	for i := range outcomes {
		o := &outcomes[i]
		if contains([]string{"change", "overlap", "additional_support"}, o.Kind) && len(o.OperationIndexes) == 0 {
			o.Kind, o.Reason = "unresolved", "no_validated_operation"
		}
		if o.Kind == "unresolved" {
			unresolved = append(unresolved, o.ClaimID)
		}
	}
	return filtered, outcomes, unresolved
}

// Withholding one patch must also withhold dependent patches and incomplete
// new entries. The resulting indexes describe only the actual returned patch.
func completeOutcomeOperations(ops []operation, claims []claim, outcomes []claimOutcome) []operation {
	for {
		retained := make([]operation, 0, len(ops))
		present := map[string]bool{}
		for _, op := range ops {
			for _, o := range outcomes {
				if o.ClaimID == op.ClaimID && contains([]string{"change", "overlap", "additional_support"}, o.Kind) {
					present[op.ClaimID] = true
				}
			}
		}
		for _, op := range ops {
			allowed := present[op.ClaimID]
			for _, id := range op.SupportingClaimIDs {
				allowed = allowed && present[id]
			}
			if allowed {
				retained = append(retained, op)
			}
		}
		retained, _ = filterUnsafeNewGroups(retained, ops, claims, map[string]bool{}, map[string]bool{})
		if len(retained) == len(ops) {
			return retained
		}
		ops = retained
	}
}
func validOutcome(o claimOutcome, claims []claim, facts []comparisonFact) bool {
	if !contains(outcomeKinds, o.Kind) || strings.TrimSpace(o.Reason) == "" || len(o.Reason) > 500 || o.RelatedFacts == nil || o.RelatedClaimIDs == nil || len(o.RelatedFacts) > 12 || len(o.RelatedClaimIDs) > 12 || len(o.OperationIndexes) > 0 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range o.RelatedFacts {
		found := false
		for _, f := range facts {
			if f.Reference == r {
				found = true
			}
		}
		if !found || seen[r.ID] {
			return false
		}
		seen[r.ID] = true
	}
	for _, id := range o.RelatedClaimIDs {
		found := false
		for _, c := range claims {
			if c.ID == id && id != o.ClaimID {
				found = true
			}
		}
		if !found || seen[id] {
			return false
		}
		seen[id] = true
	}
	if contains([]string{"exact_duplicate", "additional_support"}, o.Kind) && len(o.RelatedFacts) == 0 {
		return false
	}
	return !contains([]string{"overlap", "contradiction", "correction"}, o.Kind) || len(o.RelatedFacts)+len(o.RelatedClaimIDs) > 0
}
func safeOwner(op operation, claims []claim, l *reconciliationContext) bool {
	source := ""
	for _, c := range claims {
		if c.ID == op.ClaimID || contains(op.SupportingClaimIDs, c.ID) {
			source += "\n" + c.Source
			for _, s := range c.SupportingSources {
				source += "\n" + s.Source
			}
		}
	}
	for _, e := range l.document.Entities {
		if e.Kind != op.Target || e.LegacyID != op.EntryID {
			continue
		}
		identities := []string{}
		for _, f := range l.document.Facts {
			if f.Owner.ID == e.ID && identityField(f.Field) && f.Field != "current" {
				var value string
				_ = json.Unmarshal(f.Value, &value)
				if value != "" {
					identities = append(identities, value)
				}
			}
		}
		for _, f := range l.document.Facts {
			if f.Owner.ID != e.ID || !identityField(f.Field) || f.Field == "current" {
				continue
			}
			var value string
			_ = json.Unmarshal(f.Value, &value)
			if value != "" && !identityOccurrence(source, value, identities) {
				return false
			}
		}
		return true
	}
	return false
}

// Identity values must occupy a complete phrase, not a prefix/suffix of a
// similarly named employer or role. Unrecognized prose stays reviewable.
func identityOccurrence(source, value string, identities []string) bool {
	pattern := regexp.MustCompile(regexp.QuoteMeta(value))
	for _, at := range pattern.FindAllStringIndex(source, -1) {
		beforeRaw, afterRaw := source[:at[0]], source[at[1]:]
		before, after := strings.TrimSpace(beforeRaw), strings.TrimSpace(afterRaw)
		left := before == "" || regexp.MustCompile(`[,;:\n\r()—–]\s*$`).MatchString(beforeRaw) || regexp.MustCompile(`(?i)\b(?:at|for|as|from|in|em|na|no|como|de|company|empresa|project|projeto)$`).MatchString(before)
		right := after == "" || regexp.MustCompile(`^\s*[,;:\n\r()—–]`).MatchString(afterRaw) || regexp.MustCompile(`(?i)^(?:as|in|from|between|since|em|como|de|entre|desde)\b`).MatchString(after)
		for _, other := range identities {
			if other == value {
				continue
			}
			left = left || strings.HasSuffix(before, other)
			right = right || strings.HasPrefix(after, other)
		}
		if left && right {
			return true
		}
	}
	return false
}
func evidenceOperation(op operation, o claimOutcome, l *reconciliationContext) bool {
	for _, r := range o.RelatedFacts {
		for _, f := range l.candidates {
			if f.Reference != r {
				continue
			}
			var value string
			_ = json.Unmarshal(f.Fact.Value, &value)
			if value != op.Value || f.Fact.Field != op.Field {
				continue
			}
			if f.Fact.Owner.ID == r.ProfileID && op.EntryID == "" && op.Target == op.Field {
				return true
			}
			for _, e := range l.document.Entities {
				if e.ID == f.Fact.Owner.ID && e.Kind == op.Target && e.LegacyID == op.EntryID {
					return true
				}
			}
		}
	}
	return false
}

func hasNewSupport(c claim, o claimOutcome, candidates []comparisonFact) bool {
	sources := []string{c.Source}
	for _, s := range c.SupportingSources {
		sources = append(sources, s.Source)
	}
	for _, ref := range o.RelatedFacts {
		for _, f := range candidates {
			if f.Reference != ref {
				continue
			}
			for _, source := range sources {
				found := false
				for _, e := range f.Evidence {
					found = found || e.Excerpt == source
				}
				if !found {
					return true
				}
			}
		}
	}
	return false
}
