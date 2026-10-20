package semanticeval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const IngestionObservationVersion = "ingestion-proposal-observation-v1"

// IngestionResponseEvidence contains producer evidence only, never a Case,
// expectation package or score. Body is bytes so malformed JSON remains exportable.
type IngestionResponseEvidence struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}
type IngestionRawEvidence struct {
	Response IngestionResponseEvidence `json:"response"`
	SHA256   string                    `json:"sha256"`
}
type IngestionMeaning struct {
	Assertion string `json:"assertion"`
	Intent    string `json:"intent"`
	Certainty string `json:"certainty"`
	Temporal  struct {
		Wording   string `json:"wording"`
		Precision string `json:"precision"`
	} `json:"temporal"`
}
type IngestionSourceReference struct {
	Version            int    `json:"version"`
	SourceID           string `json:"sourceId"`
	PreparationVersion string `json:"preparationVersion"`
	SegmentID          string `json:"segmentId"`
	OccurrenceID       string `json:"occurrenceId"`
	OriginalStart      int    `json:"originalStart"`
	OriginalEnd        int    `json:"originalEnd"`
}
type IngestionSupportingSource struct {
	Origin          string                    `json:"origin,omitempty"`
	Source          string                    `json:"source"`
	SegmentID       string                    `json:"segmentId,omitempty"`
	SourceReference *IngestionSourceReference `json:"sourceReference,omitempty"`
}
type IngestionClaim struct {
	ID                string                      `json:"id"`
	Source            string                      `json:"source"`
	Text              string                      `json:"text"`
	Targets           []string                    `json:"targets"`
	Question          string                      `json:"question"`
	Meaning           *IngestionMeaning           `json:"meaning,omitempty"`
	SupportingSources []IngestionSupportingSource `json:"supportingSources,omitempty"`
	SegmentID         string                      `json:"segmentId,omitempty"`
	SourceReference   *IngestionSourceReference   `json:"sourceReference,omitempty"`
}
type IngestionOperation struct {
	SupportingClaimIDs []string `json:"supportingClaimIds,omitempty"`
	ClaimID            string   `json:"claimId"`
	Target             string   `json:"target"`
	EntryID            string   `json:"entryId"`
	Field              string   `json:"field"`
	Action             string   `json:"action"`
	Value              string   `json:"value"`
	Finding            string   `json:"finding"`
}
type IngestionFactReference struct {
	ProfileID string `json:"profileId"`
	ID        string `json:"id"`
	Revision  int    `json:"revision"`
}
type IngestionDisposition struct {
	ClaimID          string                   `json:"claimId"`
	Kind             string                   `json:"kind"`
	Reason           string                   `json:"reason"`
	RelatedFacts     []IngestionFactReference `json:"relatedFacts"`
	RelatedClaimIDs  []string                 `json:"relatedClaimIds"`
	OperationIndexes []int                    `json:"operationIndexes,omitempty"`
}
type IngestionSkippedClaim struct {
	Index     int    `json:"index"`
	Reason    string `json:"reason"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	Shortened bool   `json:"shortened"`
}
type IngestionCoverage struct {
	ValidClaims       int    `json:"validClaims"`
	InvalidClaims     int    `json:"invalidClaims"`
	DiscoveryComplete bool   `json:"discoveryComplete"`
	Capacity          string `json:"capacity"`
}
type IngestionCoverageObservation struct {
	ResponseID string            `json:"responseId"`
	Value      IngestionCoverage `json:"value"`
}
type IngestionIDMapping struct {
	ResponseID   string `json:"responseId"`
	Path         string `json:"path"`
	Kind         string `json:"kind"`
	RawID        string `json:"rawId"`
	NormalizedID string `json:"normalizedId"`
}
type IngestionAvailability struct {
	Available bool     `json:"available"`
	Reasons   []string `json:"reasons"`
}
type IngestionNormalizationIssue struct {
	Code       string   `json:"code"`
	ResponseID string   `json:"responseId"`
	Path       string   `json:"path"`
	Affects    []string `json:"affects"`
}

// IngestionProposalObservation describes the reviewable proposal boundary only.
// Availability means complete mechanical projection, never semantic correctness,
// exhaustive discovery, receipt authentication, user acceptance or durable save.
type IngestionProposalObservation struct {
	Version                string                           `json:"version"`
	ProducerContract       string                           `json:"producerContract"`
	Claims                 []IngestionClaim                 `json:"claims"`
	Operations             []IngestionOperation             `json:"operations"`
	Outcomes               []IngestionDisposition           `json:"outcomes"`
	UnresolvedClaimIDs     []string                         `json:"unresolvedClaimIds"`
	SkippedClaims          []IngestionSkippedClaim          `json:"skippedClaims"`
	UnverifiedClaimCount   *int                             `json:"unverifiedClaimCount,omitempty"`
	UnplacedOperationCount *int                             `json:"unplacedOperationCount,omitempty"`
	Coverage               []IngestionCoverageObservation   `json:"coverage"`
	Availability           map[string]IngestionAvailability `json:"availability"`
	Issues                 []IngestionNormalizationIssue    `json:"issues"`
	Mappings               []IngestionIDMapping             `json:"mappings"`
	Raw                    []IngestionRawEvidence           `json:"raw"`
}

var ingestionFacets = []string{"endpoint", "claims", "operations", "dispositions", "coverage"}

func newIngestionObservation(contract string) IngestionProposalObservation {
	o := IngestionProposalObservation{Version: IngestionObservationVersion, ProducerContract: contract,
		Claims: []IngestionClaim{}, Operations: []IngestionOperation{}, Outcomes: []IngestionDisposition{},
		UnresolvedClaimIDs: []string{}, SkippedClaims: []IngestionSkippedClaim{}, Coverage: []IngestionCoverageObservation{},
		Availability: map[string]IngestionAvailability{}, Issues: []IngestionNormalizationIssue{}, Mappings: []IngestionIDMapping{}, Raw: []IngestionRawEvidence{}}
	for _, facet := range ingestionFacets {
		o.Availability[facet] = IngestionAvailability{Available: true, Reasons: []string{}}
	}
	return o
}
func (o *IngestionProposalObservation) unavailable(code, responseID, path string, facets ...string) {
	if len(facets) == 0 {
		facets = ingestionFacets
	}
	o.Issues = append(o.Issues, IngestionNormalizationIssue{code, responseID, path, append([]string{}, facets...)})
	for _, facet := range append(append([]string{}, facets...), "endpoint") {
		a := o.Availability[facet]
		a.Available = false
		found := false
		for _, r := range a.Reasons {
			found = found || r == code
		}
		if !found {
			a.Reasons = append(a.Reasons, code)
		}
		o.Availability[facet] = a
	}
}
func (o *IngestionProposalObservation) retain(r IngestionResponseEvidence) {
	r.Body = bytes.Clone(r.Body)
	for _, previous := range o.Raw {
		if r.ID == "" || previous.Response.ID == r.ID {
			o.unavailable("response_identity", r.ID, "")
			break
		}
	}
	if r.ID == "" && len(o.Raw) == 0 {
		o.unavailable("response_identity", r.ID, "")
	}
	o.Raw = append(o.Raw, IngestionRawEvidence{r, Hash(r.Body)})
}
func (o *IngestionProposalObservation) mapping(responseID, path, kind, rawID, normalizedID string) {
	o.Mappings = append(o.Mappings, IngestionIDMapping{responseID, path, kind, rawID, normalizedID})
}

type legacyIngestionWire struct {
	Claims                 []IngestionClaim        `json:"claims"`
	Operations             []IngestionOperation    `json:"operations"`
	Outcomes               []IngestionDisposition  `json:"outcomes"`
	SkippedClaims          []IngestionSkippedClaim `json:"skippedClaims"`
	Coverage               IngestionCoverage       `json:"coverage"`
	UnresolvedClaimIDs     []string                `json:"unresolvedClaimIds"`
	UnverifiedClaimCount   *int                    `json:"unverifiedClaimCount"`
	UnplacedOperationCount *int                    `json:"unplacedOperationCount"`
	Decision               json.RawMessage         `json:"decision"`
	Continuation           json.RawMessage         `json:"continuation"`
	ApplyAnswer            json.RawMessage         `json:"applyAnswer"`
}

// NormalizeLegacyIngestion projects an observed legacy response without gold or
// semantic repair. The original bytes remain the source of every projected unit.
func NormalizeLegacyIngestion(response IngestionResponseEvidence) IngestionProposalObservation {
	o := newIngestionObservation("ingestion-legacy-response-v1")
	o.retain(response)
	var wire legacyIngestionWire
	if !o.decode(response, &wire) {
		return o
	}
	o.Claims = wire.Claims
	o.Operations = wire.Operations
	o.Outcomes = wire.Outcomes
	o.SkippedClaims = wire.SkippedClaims
	o.UnresolvedClaimIDs = wire.UnresolvedClaimIDs
	o.UnverifiedClaimCount = wire.UnverifiedClaimCount
	o.UnplacedOperationCount = wire.UnplacedOperationCount
	o.Coverage = append(o.Coverage, IngestionCoverageObservation{response.ID, wire.Coverage})
	for field, facet := range map[string]string{"claims": "claims", "operations": "operations", "outcomes": "dispositions", "unresolvedClaimIds": "dispositions", "skippedClaims": "coverage"} {
		o.requireArray(response, response.Body, "", field, facet)
	}
	o.requireFields(response, response.Body, "", "coverage", "coverage", "unverifiedClaimCount", "unplacedOperationCount")
	o.requireFields(response, rawField(response.Body, "coverage"), "/coverage", "coverage", "validClaims", "invalidClaims", "discoveryComplete", "capacity")
	o.validateRecords(response, wire.Claims, wire.Operations, wire.Outcomes, nil)
	o.skippedMappings(response, wire.SkippedClaims, 0)
	o.validateCoverage(response, wire.Coverage, len(wire.Claims), len(wire.SkippedClaims))
	if !completeLegacyIngestionScope(wire.Continuation) {
		o.unavailable("unsupported_continuation_scope", response.ID, "/continuation")
	}
	if wire.UnverifiedClaimCount != nil && (*wire.UnverifiedClaimCount < 0 || *wire.UnverifiedClaimCount != wire.Coverage.InvalidClaims) {
		o.unavailable("coverage_mismatch", response.ID, "/unverifiedClaimCount", "coverage")
	}
	if wire.UnplacedOperationCount != nil && *wire.UnplacedOperationCount < 0 {
		o.unavailable("coverage_mismatch", response.ID, "/unplacedOperationCount", "coverage")
	}
	seen := map[string]bool{}
	for _, id := range wire.UnresolvedClaimIDs {
		if id == "" || seen[id] || !hasIngestionClaim(wire.Claims, id) {
			o.unavailable("dangling_or_duplicate_reference", response.ID, "/unresolvedClaimIds", "dispositions")
		}
		seen[id] = true
	}
	for i, c := range o.Claims {
		o.mapping(response.ID, "/claims/"+strconv.Itoa(i), "claim", c.ID, c.ID)
	}
	for i := range o.Operations {
		id := "operation:" + strconv.Itoa(i)
		o.mapping(response.ID, "/operations/"+strconv.Itoa(i), "operation", strconv.Itoa(i), id)
	}
	for i, v := range o.Outcomes {
		o.mapping(response.ID, "/outcomes/"+strconv.Itoa(i), "disposition", v.ClaimID, v.ClaimID)
	}
	return o
}

// A legacy response observes one portion only. A complete plan with multiple
// portions does not establish that any other portion was processed. Keep this
// check separate from decoding the proposal so unsupported metadata cannot erase
// returned rows or their raw-to-normalized identities.
func completeLegacyIngestionScope(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return true
	}
	type region struct {
		Start *int `json:"Start"`
		End   *int `json:"End"`
	}
	var continuation struct {
		SourceID     *string    `json:"sourceId"`
		Index        *int       `json:"index"`
		Total        *int       `json:"total"`
		Bytes        *int       `json:"bytes"`
		Regions      [][]region `json:"regions"`
		Remaining    []region   `json:"remaining"`
		PlanComplete *bool      `json:"planComplete"`
		Processed    *bool      `json:"processed"`
	}
	if !exactIngestionFields(raw, reflect.TypeOf(continuation)) || json.Unmarshal(raw, &continuation) != nil {
		return false
	}
	if continuation.SourceID == nil || *continuation.SourceID == "" || continuation.Index == nil || *continuation.Index != 0 || continuation.Total == nil || *continuation.Total != 1 || continuation.Bytes == nil || *continuation.Bytes < 200 || *continuation.Bytes > 2000 || continuation.PlanComplete == nil || !*continuation.PlanComplete || continuation.Processed == nil || !*continuation.Processed || len(continuation.Regions) != 1 || len(continuation.Regions[0]) == 0 {
		return false
	}
	// Source.Plan leaves Unprocessed nil on success, so accept the actual
	// producer's explicit null as well as an empty range array.
	// Missing metadata is unknown; Bytes is a portion budget, not source length.
	if len(rawField(raw, "remaining")) == 0 || len(continuation.Remaining) != 0 {
		return false
	}
	for _, r := range continuation.Regions[0] {
		if r.Start == nil || r.End == nil || *r.Start < 0 || *r.End <= *r.Start {
			return false
		}
	}
	return true
}

func (o *IngestionProposalObservation) decode(r IngestionResponseEvidence, target any) bool {
	if r.Status != 200 {
		o.unavailable("http_status", r.ID, "")
		return false
	}
	if len(bytes.TrimSpace(r.Body)) == 0 {
		o.unavailable("missing_output", r.ID, "")
		return false
	}
	if !losslessIngestionUnicode(r.Body) {
		o.unavailable("lossy_unicode", r.ID, "")
		return false
	}
	if bytes.TrimSpace(r.Body)[0] != '{' || !uniqueIngestionJSON(json.NewDecoder(bytes.NewReader(r.Body))) {
		o.unavailable("malformed_or_unknown_schema", r.ID, "")
		return false
	}
	if !exactIngestionFields(r.Body, reflect.TypeOf(target)) {
		o.unavailable("malformed_or_unknown_schema", r.ID, "")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(r.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		o.unavailable("malformed_or_unknown_schema", r.ID, "")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		o.unavailable("malformed_or_unknown_schema", r.ID, "")
		return false
	}
	return true
}

// Match only the names the known producer structs actually emit. encoding/json
// otherwise folds case, allowing an alias to overwrite a canonical field. Opaque
// RawMessage metadata remains raw; it has no projected schema to interpret here.
func exactIngestionFields(raw []byte, schema reflect.Type) bool {
	for schema.Kind() == reflect.Pointer {
		schema = schema.Elem()
	}
	if schema == reflect.TypeOf(json.RawMessage{}) {
		return true
	}
	switch schema.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < schema.NumField(); i++ {
			field := schema.Field(i)
			if !field.IsExported() {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
		for name, value := range object {
			field, known := fields[name]
			if !known || !exactIngestionFields(value, field) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil {
			return false
		}
		for _, value := range values {
			if !exactIngestionFields(value, schema.Elem()) {
				return false
			}
		}
	}
	return true
}

// Kept local to the projector: mechanical errors are evidence availability,
// rather than producer or semantic judgments.
func ingestionPath(field string, index int) string { return fmt.Sprintf("/%s/%d", field, index) }

func rawField(raw []byte, key string) json.RawMessage {
	var object map[string]json.RawMessage
	_ = json.Unmarshal(raw, &object)
	return object[key]
}
func (o *IngestionProposalObservation) requireFields(r IngestionResponseEvidence, raw []byte, path, facet string, keys ...string) {
	var facets []string
	if facet != "" {
		facets = []string{facet}
	}
	for _, key := range keys {
		value := rawField(raw, key)
		if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			o.unavailable("missing_field", r.ID, path+"/"+key, facets...)
		}
	}
}
func (o *IngestionProposalObservation) requireArray(r IngestionResponseEvidence, raw []byte, path, field, facet string) {
	value := bytes.TrimSpace(rawField(raw, field))
	if len(value) == 0 || value[0] != '[' {
		o.unavailable("missing_array", r.ID, path+"/"+field, facet)
	}
}

// encoding/json accepts duplicate object keys. Reject that lossy decoding before
// selecting any field, including duplicate keys inside nested producer records.
func uniqueIngestionJSON(d *json.Decoder) bool {
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	if delim != '{' && delim != '[' {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return false
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return false
			}
			seen[s] = true
		}
		if !uniqueIngestionJSON(d) {
			return false
		}
	}
	_, err = d.Token()
	return err == nil
}

func hasIngestionClaim(claims []IngestionClaim, id string) bool {
	for _, c := range claims {
		if c.ID == id {
			return true
		}
	}
	return false
}
func (o *IngestionProposalObservation) validateRecords(r IngestionResponseEvidence, claims []IngestionClaim, ops []IngestionOperation, outcomes []IngestionDisposition, universe []IngestionClaim) {
	if universe == nil {
		universe = claims
	}
	rawClaims := rawIngestionArray(r.Body, "claims")
	o.validateClaimMetadata(r, rawClaims)
	seen := map[string]bool{}
	for i, c := range claims {
		path := ingestionPath("claims", i)
		if c.ID == "" || seen[c.ID] {
			o.unavailable("duplicate_or_missing_claim_id", r.ID, path, "claims")
		}
		seen[c.ID] = true
		if i < len(rawClaims) {
			o.requireFields(r, rawClaims[i], path, "claims", "id", "source", "text", "targets", "question")
			o.requireArray(r, rawClaims[i], path, "targets", "claims")
		}
	}
	rawOps := rawIngestionArray(r.Body, "operations")
	for i, op := range ops {
		path := ingestionPath("operations", i)
		if i < len(rawOps) {
			o.requireFields(r, rawOps[i], path, "operations", "claimId", "target", "entryId", "field", "action", "value", "finding")
		}
		if !hasIngestionClaim(claims, op.ClaimID) {
			o.unavailable("dangling_or_duplicate_reference", r.ID, path, "operations")
		}
		supports := map[string]bool{}
		for _, id := range op.SupportingClaimIDs {
			if !hasIngestionClaim(universe, id) || supports[id] {
				o.unavailable("dangling_or_duplicate_reference", r.ID, path, "operations")
			}
			supports[id] = true
		}
		if len(op.EntryID) > 4 && op.EntryID[:4] == "new:" && !hasIngestionClaim(universe, op.EntryID[4:]) {
			o.unavailable("dangling_or_duplicate_reference", r.ID, path+"/entryId", "operations")
		}
	}
	rawOutcomes := rawIngestionArray(r.Body, "outcomes")
	seen = map[string]bool{}
	for i, v := range outcomes {
		path := ingestionPath("outcomes", i)
		if i < len(rawOutcomes) {
			o.requireFields(r, rawOutcomes[i], path, "dispositions", "claimId", "kind", "reason", "relatedFacts", "relatedClaimIds")
			o.requireArray(r, rawOutcomes[i], path, "relatedFacts", "dispositions")
			o.requireArray(r, rawOutcomes[i], path, "relatedClaimIds", "dispositions")
			for j, fact := range rawIngestionArray(rawOutcomes[i], "relatedFacts") {
				o.requireFields(r, fact, path+"/relatedFacts/"+strconv.Itoa(j), "dispositions", "profileId", "id", "revision")
			}
		}
		if !hasIngestionClaim(claims, v.ClaimID) || seen[v.ClaimID] {
			o.unavailable("dangling_or_duplicate_reference", r.ID, path, "dispositions")
		}
		seen[v.ClaimID] = true
		for _, id := range v.RelatedClaimIDs {
			if !hasIngestionClaim(universe, id) {
				o.unavailable("dangling_or_duplicate_reference", r.ID, path, "dispositions")
			}
		}
		for _, index := range v.OperationIndexes {
			if index < 0 || index >= len(ops) {
				o.unavailable("operation_index", r.ID, path+"/operationIndexes", "dispositions")
			}
		}
	}
	for _, c := range claims {
		if !seen[c.ID] {
			o.unavailable("missing_disposition", r.ID, "/outcomes", "dispositions")
		}
	}
}
func (o *IngestionProposalObservation) validateCoverage(r IngestionResponseEvidence, c IngestionCoverage, claims, skipped int) {
	if c.ValidClaims != claims || c.InvalidClaims != skipped || c.DiscoveryComplete || (c.Capacity != "within_limit" && c.Capacity != "possibly_exhausted") {
		o.unavailable("coverage_mismatch", r.ID, "/coverage", "coverage")
	}
}
func rawIngestionArray(raw []byte, field string) []json.RawMessage {
	var values []json.RawMessage
	_ = json.Unmarshal(rawField(raw, field), &values)
	return values
}

func (o *IngestionProposalObservation) validateClaimMetadata(r IngestionResponseEvidence, claims []json.RawMessage) {
	for i, raw := range claims {
		path := ingestionPath("claims", i)
		if m := rawField(raw, "meaning"); len(m) > 0 {
			o.requireFields(r, m, path+"/meaning", "claims", "assertion", "intent", "certainty", "temporal")
			o.requireFields(r, rawField(m, "temporal"), path+"/meaning/temporal", "claims", "wording", "precision")
		}
		o.validateSourceReference(r, rawField(raw, "sourceReference"), path+"/sourceReference")
		for j, support := range rawIngestionArray(raw, "supportingSources") {
			p := path + "/supportingSources/" + strconv.Itoa(j)
			o.requireFields(r, support, p, "claims", "source")
			o.validateSourceReference(r, rawField(support, "sourceReference"), p+"/sourceReference")
		}
	}
}
func (o *IngestionProposalObservation) validateSourceReference(r IngestionResponseEvidence, raw []byte, path string) {
	if len(raw) > 0 {
		o.requireFields(r, raw, path, "claims", "version", "sourceId", "preparationVersion", "segmentId", "occurrenceId", "originalStart", "originalEnd")
	}
}
func (o *IngestionProposalObservation) skippedMappings(r IngestionResponseEvidence, skipped []IngestionSkippedClaim, offset int) {
	raw := rawIngestionArray(r.Body, "skippedClaims")
	for i, c := range skipped {
		path := ingestionPath("skippedClaims", i)
		o.requireFields(r, raw[i], path, "coverage", "index", "reason", "text", "source", "shortened")
		o.mapping(r.ID, path, "skipped", strconv.Itoa(c.Index), "skipped:"+strconv.Itoa(offset+i))
	}
}

// encoding/json replaces invalid UTF-8 and unpaired UTF-16 escapes with U+FFFD.
// Decline those lossy inputs while allowing actual replacement characters,
// valid surrogate pairs, and literal escaped backslashes to remain unchanged.
func losslessIngestionUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) || raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		value, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if value >= 0xDC00 && value <= 0xDFFF {
			return false
		}
		if value >= 0xD800 && value <= 0xDBFF {
			if i+6 >= len(raw) || string(raw[i+1:i+3]) != `\u` {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return false
			}
			i += 6
		}
	}
	return true
}
