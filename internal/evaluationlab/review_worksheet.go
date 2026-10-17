package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"professional-information-repo/internal/semanticeval"
	"strconv"
	"strings"
	"unicode/utf8"
)

const ReviewWorksheetVersion = "lab-source-review-v1"
const MaxReviewWorksheetBytes = 1 << 20

// Bindings are supplied by the owned immutable case, never taken as authority
// from an uploaded worksheet. Bytes preserve the original text/JSON exactly.
type ReviewBinding struct {
	RunID        string `json:"runId"`
	CaseID       string `json:"caseId"`
	LabelVersion string `json:"labelVersion"`
	SourceBytes  []byte `json:"sourceBytes"`
	OutputBytes  []byte `json:"outputBytes"`
	GoldBytes    []byte `json:"goldBytes"`
}
type ReviewActor struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}
type ReviewSpan struct {
	Pointer   string `json:"pointer"`
	StartByte int    `json:"startByte"`
	EndByte   int    `json:"endByte"`
	Text      string `json:"text"`
}
type ReviewAnnotation struct {
	ID               string      `json:"id"`
	Dimension        string      `json:"dimension,omitempty"`
	Reviewer         ReviewActor `json:"reviewer"`
	State            string      `json:"state"`
	SourceSpan       ReviewSpan  `json:"sourceSpan"`
	OutputAtom       ReviewSpan  `json:"outputAtom"`
	OutputAbsent     bool        `json:"outputAbsent,omitempty"`
	Owner            string      `json:"owner"`
	Context          string      `json:"context"`
	Polarity         string      `json:"polarity"`
	Certainty        string      `json:"certainty"`
	Intent           string      `json:"intent"`
	Time             string      `json:"time"`
	Qualifier        string      `json:"qualifier"`
	Materiality      string      `json:"materiality"`
	ProposedVerdict  string      `json:"proposedVerdict"`
	Rationale        string      `json:"rationale"`
	VerdictOrigin    string      `json:"verdictOrigin,omitempty"`
	RawScorerVerdict []byte      `json:"rawScorerVerdict,omitempty"`
	RawModelVerdict  []byte      `json:"rawModelVerdict,omitempty"`
}
type ReviewDecision struct {
	AnnotationID string `json:"annotationId"`
	Verdict      string `json:"verdict"`
	Rationale    string `json:"rationale"`
}
type ReviewAdjudication struct {
	Actor         ReviewActor      `json:"actor"`
	Origin        string           `json:"origin"`
	WorksheetHash string           `json:"worksheetHash"`
	ContextHash   string           `json:"contextHash"`
	Rationale     string           `json:"rationale"`
	Decisions     []ReviewDecision `json:"decisions"`
	Verified      bool             `json:"verified"`
}
type ReviewApprovalRequest struct {
	Reviewer      string `json:"reviewer"`
	ContextHash   string `json:"contextHash"`
	WorksheetHash string `json:"worksheetHash"`
	LabelVersion  string `json:"labelVersion"`
	Confirmation  string `json:"confirmation"`
}
type ReviewAdjudicationRequest struct {
	Actor         ReviewActor      `json:"actor"`
	ContextHash   string           `json:"contextHash"`
	WorksheetHash string           `json:"worksheetHash"`
	Confirmation  string           `json:"confirmation"`
	Rationale     string           `json:"rationale"`
	Decisions     []ReviewDecision `json:"decisions"`
}
type ReviewWorksheet struct {
	RetainedScorerVerdict json.RawMessage      `json:"retainedScorerVerdict,omitempty"`
	RetainedModelVerdicts []json.RawMessage    `json:"retainedModelVerdicts,omitempty"`
	Revision              int                  `json:"revision"`
	ParentID              string               `json:"parentId,omitempty"`
	ParentHash            string               `json:"parentHash,omitempty"`
	DerivedLabelVersion   string               `json:"derivedLabelVersion,omitempty"`
	ClaimedLabelVersion   string               `json:"claimedLabelVersion,omitempty"`
	Adjudications         []ReviewAdjudication `json:"adjudications,omitempty"`
	Version               string               `json:"version"`
	ID                    string               `json:"id"`
	ContextHash           string               `json:"contextHash"`
	State                 string               `json:"state"`
	Binding               ReviewBinding        `json:"binding"`
	Annotations           []ReviewAnnotation   `json:"annotations"`
}

func reviewBindingHash(binding ReviewBinding) (string, error) {
	if binding.RunID == "" || binding.CaseID == "" || binding.LabelVersion == "" || len(binding.RunID) > 128 || len(binding.CaseID) > 128 || len(binding.LabelVersion) > 128 {
		return "", fmt.Errorf("review case and label identities required")
	}
	if len(binding.SourceBytes)+len(binding.OutputBytes)+len(binding.GoldBytes) > MaxReviewWorksheetBytes/2 {
		return "", fmt.Errorf("review source bounds exceeded")
	}
	for _, raw := range [][]byte{binding.SourceBytes, binding.OutputBytes, binding.GoldBytes} {
		if err := validateJSON(raw); err != nil {
			return "", fmt.Errorf("review binding must contain original JSON bytes")
		}
	}
	raw, err := json.Marshal(binding)
	return semanticeval.Hash(raw), err
}
func NewReviewWorksheet(binding ReviewBinding) (ReviewWorksheet, error) {
	hash, err := reviewBindingHash(binding)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	worksheet := ReviewWorksheet{Version: ReviewWorksheetVersion, Revision: 1, ID: newLabID(), ContextHash: hash, State: "pending", Binding: cloneJSON(binding), Annotations: []ReviewAnnotation{}}
	var retained struct {
		Score           json.RawMessage   `json:"score"`
		ProviderOutputs []json.RawMessage `json:"providerOutputs"`
	}
	_ = json.Unmarshal(binding.OutputBytes, &retained)
	worksheet.RetainedScorerVerdict = retained.Score
	worksheet.RetainedModelVerdicts = retained.ProviderOutputs
	return worksheet, nil
}
func ProposeSourceReview(worksheet ReviewWorksheet, binding ReviewBinding, annotation ReviewAnnotation) (ReviewWorksheet, error) {
	if err := validateReviewWorksheet(worksheet, binding); err != nil {
		return ReviewWorksheet{}, err
	}
	if worksheet.State != "pending" || len(worksheet.Adjudications) > 0 {
		return ReviewWorksheet{}, fmt.Errorf("create a new pending worksheet before proposing after adjudication")
	}
	if len(worksheet.Annotations) >= 128 {
		return ReviewWorksheet{}, fmt.Errorf("review annotation bound exceeded")
	}
	annotation.State = "pending"
	if len(annotation.RawScorerVerdict) > 0 || len(annotation.RawModelVerdict) > 0 {
		annotation.VerdictOrigin = "reviewer-supplied-unverified"
	}
	if err := validateReviewAnnotation(annotation, binding); err != nil {
		return ReviewWorksheet{}, err
	}
	for _, old := range worksheet.Annotations {
		if old.ID == annotation.ID {
			return ReviewWorksheet{}, fmt.Errorf("duplicate annotation identity")
		}
	}
	result := cloneJSON(worksheet)
	result.Annotations = append(result.Annotations, cloneJSON(annotation))
	return result, nil
}
func ExportReviewWorksheet(worksheet ReviewWorksheet) ([]byte, error) {
	if err := validateReviewWorksheet(worksheet, worksheet.Binding); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(worksheet)
	if len(raw) > MaxReviewWorksheetBytes {
		return nil, fmt.Errorf("worksheet exceeds export bounds")
	}
	return raw, err
}
func ImportReviewWorksheet(raw []byte, binding ReviewBinding) (ReviewWorksheet, error) {
	if len(raw) == 0 || len(raw) > MaxReviewWorksheetBytes {
		return ReviewWorksheet{}, fmt.Errorf("worksheet exceeds import bounds")
	}
	if err := validateJSON(raw); err != nil {
		return ReviewWorksheet{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var worksheet ReviewWorksheet
	if err := decoder.Decode(&worksheet); err != nil {
		return ReviewWorksheet{}, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ReviewWorksheet{}, fmt.Errorf("trailing worksheet data")
	}
	if err := validateReviewWorksheet(worksheet, binding); err != nil {
		return ReviewWorksheet{}, err
	}
	// Uploaded identity/type is a claim, not a local human approval receipt.
	if worksheet.DerivedLabelVersion != "" {
		worksheet.ClaimedLabelVersion = worksheet.DerivedLabelVersion
		worksheet.DerivedLabelVersion = ""
	}
	worksheet.State = "pending"
	for i := range worksheet.Adjudications {
		worksheet.Adjudications[i].Verified = false
	}
	return worksheet, nil
}
func validateReviewWorksheet(worksheet ReviewWorksheet, binding ReviewBinding) error {
	expected, err := reviewBindingHash(binding)
	if err != nil {
		return err
	}
	actual, err := reviewBindingHash(worksheet.Binding)
	if err != nil {
		return err
	}
	if worksheet.Version != ReviewWorksheetVersion || !storeID.MatchString(worksheet.ID) || worksheet.ContextHash != expected || actual != expected || !contains([]string{"pending", "reviewed"}, worksheet.State) || worksheet.Revision < 1 || worksheet.Revision > 256 || len(worksheet.Annotations) > 128 || len(worksheet.Adjudications) > 128 {
		return fmt.Errorf("worksheet identity, original text or label version changed")
	}
	if worksheet.State == "reviewed" && (worksheet.DerivedLabelVersion == "" || len(worksheet.Adjudications) == 0) {
		return fmt.Errorf("reviewed label receipt required")
	}
	for _, review := range worksheet.Adjudications {
		if review.Actor.Type != "human" || review.Actor.ID == "" || review.ContextHash != worksheet.ContextHash || review.WorksheetHash == "" || review.Rationale == "" || !contains([]string{"local-human-attestation", "controlled-test"}, review.Origin) {
			return fmt.Errorf("invalid adjudicator provenance")
		}
	}
	var retained struct {
		Score           json.RawMessage   `json:"score"`
		ProviderOutputs []json.RawMessage `json:"providerOutputs"`
	}
	_ = json.Unmarshal(binding.OutputBytes, &retained)
	expectedVerdicts, _ := json.Marshal(retained.ProviderOutputs)
	actualVerdicts, _ := json.Marshal(worksheet.RetainedModelVerdicts)
	expectedScore, _ := json.Marshal(retained.Score)
	actualScore, _ := json.Marshal(worksheet.RetainedScorerVerdict)
	if !bytes.Equal(expectedScore, actualScore) || !bytes.Equal(expectedVerdicts, actualVerdicts) {
		return fmt.Errorf("retained raw verdicts changed")
	}
	seen := map[string]bool{}
	for _, annotation := range worksheet.Annotations {
		if seen[annotation.ID] {
			return fmt.Errorf("duplicate annotation identity")
		}
		seen[annotation.ID] = true
		if err := validateReviewAnnotation(annotation, binding); err != nil {
			return err
		}
	}
	return nil
}
func validateReviewAnnotation(annotation ReviewAnnotation, binding ReviewBinding) error {
	if annotation.ID == "" || len(annotation.ID) > 128 || annotation.Reviewer.ID == "" || len(annotation.Reviewer.ID) > 128 || !contains([]string{"human", "agent", "model"}, annotation.Reviewer.Type) || annotation.State != "pending" || annotation.Rationale == "" {
		return fmt.Errorf("explicit pending reviewer provenance required")
	}
	if (len(annotation.RawScorerVerdict) > 0 || len(annotation.RawModelVerdict) > 0) && annotation.VerdictOrigin != "reviewer-supplied-unverified" {
		return fmt.Errorf("uploaded verdicts cannot claim retained authority")
	}
	if annotation.Dimension != "" && !contains([]string{"statement-support", "meaning-preservation", "retrieval-relevance", "qualification-sufficiency"}, annotation.Dimension) {
		return fmt.Errorf("unsupported review dimension")
	}
	raw, err := json.Marshal(annotation)
	if err != nil || len(raw) > 8192 {
		return fmt.Errorf("annotation bounds exceeded")
	}
	if err := validateReviewSpan(binding.SourceBytes, annotation.SourceSpan); err != nil {
		return err
	}
	if annotation.OutputAbsent {
		if annotation.ProposedVerdict != "omission" || annotation.OutputAtom != (ReviewSpan{}) {
			return fmt.Errorf("absent output requires an explicit omission without a fabricated span")
		}
		return nil
	}
	return validateReviewSpan(binding.OutputBytes, annotation.OutputAtom)
}
func validateReviewSpan(raw []byte, span ReviewSpan) error {
	if len(span.Pointer) > 512 || span.StartByte < 0 || span.EndByte <= span.StartByte || span.Text == "" {
		return fmt.Errorf("exact source/output span required")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if span.Pointer != "" {
		if !strings.HasPrefix(span.Pointer, "/") {
			return fmt.Errorf("invalid JSON pointer")
		}
		for _, part := range strings.Split(span.Pointer[1:], "/") {
			for i := 0; i < len(part); i++ {
				if part[i] == '~' {
					if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
						return fmt.Errorf("invalid JSON pointer escape")
					}
					i++
				}
			}
			key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			switch node := value.(type) {
			case map[string]any:
				value = node[key]
			case []any:
				index, err := strconv.Atoi(key)
				if err != nil || index < 0 || index >= len(node) {
					return fmt.Errorf("source pointer unavailable")
				}
				value = node[index]
			default:
				return fmt.Errorf("source pointer unavailable")
			}
		}
	}
	text, ok := value.(string)
	if !ok || span.EndByte > len(text) {
		return fmt.Errorf("source pointer is not retained text")
	}
	if !utf8.ValidString(text[:span.StartByte]) || !utf8.ValidString(text[:span.EndByte]) || text[span.StartByte:span.EndByte] != span.Text {
		return fmt.Errorf("source span text changed")
	}
	return nil
}
