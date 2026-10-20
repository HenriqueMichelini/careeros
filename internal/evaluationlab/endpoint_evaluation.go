package evaluationlab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
	"sort"
)

const EndpointEvaluationVersion = "lab-endpoint-evaluation-v1"
const IngestionEndpointEvaluatorVersion = "ingestion-endpoint-evaluator-v1"

type EndpointEvaluationBinding struct {
	ExperimentHash  string `json:"experimentHash"`
	VariantID       string `json:"variantId"`
	RunID           string `json:"runId"`
	RunSnapshotHash string `json:"runSnapshotHash"`
}

// Evidence contains returned bytes only. Neither normalizer receives Task or gold.
type IngestionEndpointEvidence struct {
	Legacy     *semanticeval.IngestionResponseEvidence   `json:"legacy,omitempty"`
	Submission *semanticeval.SubmissionIngestionEvidence `json:"submission,omitempty"`
}

type IngestionEndpointRequest struct {
	Task                  TaskSidecar                                `json:"task"`
	Binding               EndpointEvaluationBinding                  `json:"binding"`
	Evidence              IngestionEndpointEvidence                  `json:"evidence"`
	Observation           *semanticeval.IngestionProposalObservation `json:"observation,omitempty"`
	EvaluatorDependencies map[string]string                          `json:"evaluatorDependencies"`
	ExpectedEvaluator     *DigestRef                                 `json:"expectedEvaluator,omitempty"`
}

// The manifest pins what is measured as well as the dependency identities.
// Dependency hashes are resolver-supplied provenance; this pure derivation does
// not attest a build, approve an import, or authenticate execution receipts.
type EndpointEvaluatorManifest struct {
	Version                string              `json:"version"`
	Scorer                 string              `json:"scorer"`
	ObservationVersion     string              `json:"observationVersion"`
	Projections            map[string][]string `json:"projections"`
	ForbiddenPatternSource string              `json:"forbiddenPatternSource"`
	Dependencies           map[string]string   `json:"dependencies"`
	DependencyKind         string              `json:"dependencyKind"`
}

// Ratios live in Metric, where zero opportunities stay unavailable. These are
// finite curated counts, never general semantic precision or correctness.
type EndpointCuratedCounts struct {
	Expected              int      `json:"expected"`
	Observed              int      `json:"observed"`
	Matched               int      `json:"matched"`
	Covered               int      `json:"covered"`
	UnsupportedCandidates int      `json:"unsupportedClaimCandidates"`
	Duplicates            int      `json:"duplicates"`
	RelationshipErrors    int      `json:"relationshipErrors"`
	Missing               []string `json:"missing"`
	Unmatched             []string `json:"unmatched"`
	Failures              []string `json:"failures"`
}

type EndpointFacetEvaluation struct {
	Scope              string                 `json:"scope"`
	Eligible           int                    `json:"eligible"`
	Evaluated          int                    `json:"evaluated"`
	Missing            int                    `json:"missing"`
	NotRecovered       []string               `json:"notRecovered"`
	Counts             *EndpointCuratedCounts `json:"counts"`
	Metrics            []Metric               `json:"metrics"`
	UnavailableReasons []string               `json:"unavailableReasons"`
}

type EndpointEvaluationReceipt struct {
	Version                            string                             `json:"version"`
	Hash                               string                             `json:"hash"`
	PreviousReceiptHash                string                             `json:"previousReceiptHash,omitempty"`
	Binding                            EndpointEvaluationBinding          `json:"binding"`
	CaseRef                            TaskCaseRef                        `json:"caseRef"`
	InputHash                          string                             `json:"inputHash"`
	ExpectationHash                    string                             `json:"expectationHash"`
	ForbiddenPatternsHash              string                             `json:"forbiddenPatternsHash"`
	ForbiddenPatternCount              int                                `json:"forbiddenPatternCount"`
	RawOutputHash                      string                             `json:"rawOutputHash"`
	ObservationHash                    string                             `json:"observationHash"`
	Evaluator                          DigestRef                          `json:"evaluator"`
	EvaluatorManifest                  EndpointEvaluatorManifest          `json:"evaluatorManifest"`
	EndpointState                      string                             `json:"endpointState"`
	Facets                             map[string]EndpointFacetEvaluation `json:"facets"`
	CuratedForbiddenAdditions          *int                               `json:"curatedForbiddenAdditions"`
	GeneratedWordingForbiddenAdditions *int                               `json:"generatedWordingForbiddenAdditions"`
	UnavailableReasons                 []string                           `json:"unavailableReasons"`
	HumanReview                        string                             `json:"humanReview"`
	SemanticReview                     string                             `json:"semanticReview"`
	Limitations                        []string                           `json:"limitations"`
}

// EvaluateIngestionEndpoint is pure offline derivation. It reprojects producer
// evidence before applying one frozen expectation package to explicit facets.
func EvaluateIngestionEndpoint(request IngestionEndpointRequest) (EndpointEvaluationReceipt, error) {
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > MaxImportBytes {
		return EndpointEvaluationReceipt{}, fmt.Errorf("endpoint evaluation exceeds bounds")
	}
	if err := validateTaskRef(request.Task.Ref); err != nil {
		return EndpointEvaluationReceipt{}, err
	}
	if (len(request.Task.Input) > 0 && semanticeval.Hash(request.Task.Input) != request.Task.Ref.InputHash) || (len(request.Task.Expectation) > 0 && semanticeval.Hash(request.Task.Expectation) != request.Task.Ref.ExpectationHash) {
		return EndpointEvaluationReceipt{}, fmt.Errorf("task dependency hash mismatch")
	}
	var expectation struct {
		Version string `json:"version"`
		Data    struct {
			Gold             []semanticeval.Atom `json:"gold"`
			Forbidden        []string            `json:"forbidden"`
			Role             string              `json:"role"`
			ExpectedFailures []string            `json:"expectedFailures"`
		} `json:"data"`
	}
	if len(request.Task.Expectation) > 0 {
		if err := validateJSON(request.Task.Expectation); err != nil {
			return EndpointEvaluationReceipt{}, err
		}
		if err := validateTaskJSON(request.Task.Expectation); err != nil {
			return EndpointEvaluationReceipt{}, err
		}
		if err := json.Unmarshal(request.Task.Expectation, &expectation); err != nil || expectation.Version != "lab-task-expectation-v1" {
			return EndpointEvaluationReceipt{}, fmt.Errorf("unsupported expectation package")
		}
		c := semanticeval.Case{ID: request.Task.Ref.CaseID, Task: "profile_ingestion", Language: request.Task.Ref.Language, Split: request.Task.Ref.Split, Request: json.RawMessage(`{}`), Notes: "Frozen independent expectation package", Gold: expectation.Data.Gold, Forbidden: expectation.Data.Forbidden, Role: expectation.Data.Role, ExpectedFailures: expectation.Data.ExpectedFailures}
		if err := semanticeval.Validate([]semanticeval.Case{c}); err != nil {
			return EndpointEvaluationReceipt{}, err
		}
	}
	reasons, err := checkEndpointDependencies(request)
	if err != nil {
		return EndpointEvaluationReceipt{}, err
	}
	var packageFields struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(request.Task.Expectation, &packageFields)
	if _, present := packageFields.Data["boundaryInvariants"]; present {
		reasons = append(reasons, "unsupported_boundary_invariants")
	}
	for _, atom := range expectation.Data.Gold {
		allowed := []string{}
		switch atom.Group {
		case "claims":
			allowed = []string{"text", "targets", "question"}
		case "operations":
			allowed = []string{"value", "target", "field", "entryId", "action"}
		default:
			reasons = append(reasons, "unsupported_gold_group:"+atom.ID)
		}
		for field := range atom.Match {
			if !contains(allowed, field) {
				reasons = append(reasons, "unsupported_gold_field:"+atom.ID+":"+field)
			}
		}
	}
	sort.Strings(reasons)
	manifest := EndpointEvaluatorManifest{Version: IngestionEndpointEvaluatorVersion, Scorer: "semanticeval.Score/curated-v1", ObservationVersion: semanticeval.IngestionObservationVersion,
		Projections: map[string][]string{
			"claim-diagnostics":           {"claims.text", "claims.targets", "claims.question"},
			"proposal-operations":         {"operations.value", "operations.target", "operations.field", "operations.entryId", "operations.action"},
			"curated-forbidden":           {"claims.text", "claims.targets", "claims.question", "operations.value", "operations.target", "operations.field", "operations.entryId", "operations.action"},
			"generated-wording-forbidden": {"claims.text", "operations.value"},
		}, ForbiddenPatternSource: "frozen-expectation.data.forbidden", Dependencies: cloneJSON(request.EvaluatorDependencies), DependencyKind: "source-envelope"}
	receipt := EndpointEvaluationReceipt{Version: EndpointEvaluationVersion, Binding: request.Binding, CaseRef: cloneTaskRef(request.Task.Ref), InputHash: request.Task.Ref.InputHash, ExpectationHash: request.Task.Ref.ExpectationHash,
		EvaluatorManifest: manifest, EndpointState: "available", Facets: map[string]EndpointFacetEvaluation{}, UnavailableReasons: []string{}, HumanReview: "pending", SemanticReview: "unavailable",
		Limitations: []string{"Finite curated label agreement only; unmatched wording requires source-grounded review.", "Claim diagnostics and proposal-operation measures have separate units and denominators.", "Curated forbidden patterns scan selected scorer units; generated-wording forbidden patterns scan text/value only. Counts are not pooled.", "No signature, no-store, build, source-truth, human acceptance, Apply/Save, provider quality or import-trust attestation."}}
	receipt.Evaluator = DigestRef{ID: "ingestion-endpoint", Version: IngestionEndpointEvaluatorVersion, Hash: endpointHash(manifest)}
	receipt.ForbiddenPatternsHash = endpointHash(expectation.Data.Forbidden)
	receipt.ForbiddenPatternCount = len(expectation.Data.Forbidden)
	if request.ExpectedEvaluator != nil {
		if request.ExpectedEvaluator.ID != receipt.Evaluator.ID || request.ExpectedEvaluator.Version != receipt.Evaluator.Version || !evidenceHash.MatchString(request.ExpectedEvaluator.Hash) {
			return EndpointEvaluationReceipt{}, fmt.Errorf("evaluator identity mismatch")
		}
		complete := true
		for _, name := range endpointEvaluatorDependencies {
			complete = complete && request.EvaluatorDependencies[name] != ""
		}
		if complete && request.ExpectedEvaluator.Hash != receipt.Evaluator.Hash {
			return EndpointEvaluationReceipt{}, fmt.Errorf("evaluator identity mismatch")
		}
	}
	var observation semanticeval.IngestionProposalObservation
	switch {
	case request.Evidence.Legacy != nil && request.Evidence.Submission != nil:
		return EndpointEvaluationReceipt{}, fmt.Errorf("ambiguous producer evidence")
	case request.Evidence.Legacy != nil:
		observation = semanticeval.NormalizeLegacyIngestion(*request.Evidence.Legacy)
	case request.Evidence.Submission != nil:
		observation = semanticeval.NormalizeSubmissionIngestion(*request.Evidence.Submission)
	default:
		reasons = append(reasons, "producer_evidence_missing")
	}
	observation = canonicalEndpointObservation(observation)
	if request.Evidence.Legacy != nil || request.Evidence.Submission != nil {
		receipt.RawOutputHash = endpointHash(request.Evidence)
	}
	receipt.ObservationHash = endpointHash(observation)
	if request.Observation != nil && endpointHash(canonicalEndpointObservation(*request.Observation)) != receipt.ObservationHash {
		return EndpointEvaluationReceipt{}, fmt.Errorf("normalized observation differs from raw reprojection")
	}
	receipt.UnavailableReasons = append(receipt.UnavailableReasons, reasons...)
	if !observation.Availability["endpoint"].Available {
		receipt.EndpointState = "unavailable"
		receipt.UnavailableReasons = append(receipt.UnavailableReasons, observation.Availability["endpoint"].Reasons...)
	}
	if len(receipt.UnavailableReasons) > 0 {
		receipt.EndpointState = "unavailable"
	}
	claims := []map[string]any{}
	operations := []map[string]any{}
	wordingClaims := []map[string]string{}
	wordingOperations := []map[string]string{}
	for _, c := range observation.Claims {
		claims = append(claims, map[string]any{"text": c.Text, "targets": c.Targets, "question": c.Question})
		wordingClaims = append(wordingClaims, map[string]string{"text": c.Text})
	}
	for _, o := range observation.Operations {
		operations = append(operations, map[string]any{"value": o.Value, "target": o.Target, "field": o.Field, "entryId": o.EntryID, "action": o.Action})
		wordingOperations = append(wordingOperations, map[string]string{"value": o.Value})
	}
	for _, facet := range []struct {
		name, group string
		units       any
	}{{"claim-diagnostics", "claims", claims}, {"proposal-operations", "operations", operations}} {
		c := semanticeval.Case{}
		for _, atom := range expectation.Data.Gold {
			if atom.Group == facet.group {
				c.Gold = append(c.Gold, atom)
			}
		}
		raw, _ := json.Marshal(map[string]any{facet.group: facet.units})
		score, err := semanticeval.Score(c, raw)
		if err != nil {
			return EndpointEvaluationReceipt{}, err
		}
		f := EndpointFacetEvaluation{Scope: facet.name, Eligible: score.Expected, Evaluated: score.Expected, NotRecovered: []string{}, Metrics: []Metric{}, UnavailableReasons: []string{}}
		if receipt.EndpointState == "available" {
			f.Counts = &EndpointCuratedCounts{score.Expected, score.Observed, score.Matched, score.Covered, score.UnsupportedCandidates, score.Duplicates, score.RelationshipErrors, score.Missing, score.Unmatched, score.Failures}
			f.Metrics = append(f.Metrics, CountMetric("M03", "curated-label-precision", score.Matched, score.Observed, "offline-derived", receipt.ObservationHash), CountMetric("M03", "curated-label-completeness", score.Covered, score.Expected, "offline-derived", receipt.ObservationHash))
		} else {
			f.Evaluated = 0
			f.Missing = f.Eligible
			for _, atom := range c.Gold {
				if !atom.Optional {
					f.NotRecovered = append(f.NotRecovered, atom.ID)
				}
			}
			f.UnavailableReasons = append(f.UnavailableReasons, receipt.UnavailableReasons...)
		}
		for i := range f.Metrics {
			f.Metrics[i].Scope = facet.name
			f.Metrics[i].LabelHash = receipt.ExpectationHash
			f.Metrics[i].CaseID = receipt.CaseRef.CaseID
			f.Metrics[i].Language = receipt.CaseRef.Language
			f.Metrics[i].Workflow = "profile_ingestion"
		}
		receipt.Facets[facet.name] = f
	}
	if receipt.EndpointState == "available" {
		for _, check := range []struct {
			output any
			dest   **int
		}{{map[string]any{"claims": claims, "operations": operations}, &receipt.CuratedForbiddenAdditions}, {map[string]any{"claims": wordingClaims, "operations": wordingOperations}, &receipt.GeneratedWordingForbiddenAdditions}} {
			raw, _ := json.Marshal(check.output)
			score, err := semanticeval.Score(semanticeval.Case{Forbidden: expectation.Data.Forbidden}, raw)
			if err != nil {
				return EndpointEvaluationReceipt{}, err
			}
			n := score.ForbiddenAdditions
			*check.dest = &n
		}
	}
	receipt.Hash = endpointReceiptHash(receipt)
	return receipt, nil
}

func endpointHash(value any) string { raw, _ := json.Marshal(value); return semanticeval.Hash(raw) }
func endpointReceiptHash(receipt EndpointEvaluationReceipt) string {
	receipt.Hash = ""
	return endpointHash(receipt)
}

func checkEndpointDependencies(request IngestionEndpointRequest) ([]string, error) {
	reasons := []string{}
	if request.Task.Version != TaskSidecarVersion || request.Task.CanonicalVersion != TaskCanonicalVersion || request.Task.Ref.TaskVersion != IngestionTaskVersion || request.Task.Ref.SuiteID != "semantic-quality" {
		return nil, fmt.Errorf("unsupported task contract")
	}
	if len(request.Task.Input) == 0 {
		reasons = append(reasons, "task_input_missing")
	}
	if len(request.Task.Expectation) == 0 {
		reasons = append(reasons, "expectation_missing")
	}
	if request.Task.OriginalFixture.Hash != "" && request.Task.OriginalFixture.Hash != request.Task.Ref.OriginalFixtureHash {
		return nil, fmt.Errorf("original fixture hash mismatch")
	}
	if len(request.Task.OriginalFixture.Original) == 0 {
		reasons = append(reasons, "original_fixture_missing")
	} else {
		if request.Task.OriginalFixture.Hash == "" || semanticeval.Hash(request.Task.OriginalFixture.Original) != request.Task.Ref.OriginalFixtureHash {
			return nil, fmt.Errorf("original fixture hash mismatch")
		}
		canonical, err := ExtractTaskSidecar(request.Task.Ref.SuiteID, request.Task.OriginalFixture.Original, request.Task.Ref.CaseID)
		if err != nil {
			return nil, err
		}
		canonical.Ref.RepeatOrdinal = request.Task.Ref.RepeatOrdinal
		if endpointHash(canonical.Ref) != endpointHash(request.Task.Ref) || (len(request.Task.Input) > 0 && !bytes.Equal(canonical.Input, request.Task.Input)) || (len(request.Task.Expectation) > 0 && !bytes.Equal(canonical.Expectation, request.Task.Expectation)) {
			return nil, fmt.Errorf("task sidecar differs from frozen fixture")
		}
	}
	if len(request.EvaluatorDependencies) > 512 {
		return nil, fmt.Errorf("evaluator dependency bounds exceeded")
	}
	for name, hash := range request.EvaluatorDependencies {
		if name == "" || len(name) > 256 || !evidenceHash.MatchString(hash) {
			return nil, fmt.Errorf("invalid evaluator dependency identity")
		}
	}
	for _, name := range endpointEvaluatorDependencies {
		if request.EvaluatorDependencies[name] == "" {
			reasons = append(reasons, "evaluator_dependency_missing:"+name)
		}
	}
	if request.Binding.ExperimentHash == "" || request.Binding.RunSnapshotHash == "" || request.Binding.VariantID == "" || request.Binding.RunID == "" {
		reasons = append(reasons, "execution_binding_missing")
	}
	for _, hash := range []string{request.Binding.ExperimentHash, request.Binding.RunSnapshotHash} {
		if hash != "" && !evidenceHash.MatchString(hash) {
			return nil, fmt.Errorf("invalid execution binding hash")
		}
	}
	if len(request.Binding.VariantID) > 256 || len(request.Binding.RunID) > 256 {
		return nil, fmt.Errorf("execution binding exceeds bounds")
	}
	return reasons, nil
}

var endpointEvaluatorDependencies = []string{"build-source-set", "internal/semanticeval/score.go", "internal/semanticeval/ingestion_observation.go", "internal/semanticeval/ingestion_submission_observation.go", "internal/evaluationlab/task_sidecar.go", "internal/evaluationlab/metrics.go", "internal/evaluationlab/endpoint_evaluation.go"}

// ReevaluateIngestionPair appends two derived receipts to caller-independent
// history. Exact retained output and task inputs stay fixed; revised labels or
// evaluator dependencies must be common to both sides. Persistence is separate.
func ReevaluateIngestionPair(history []EndpointEvaluationReceipt, requests [2]IngestionEndpointRequest) ([]EndpointEvaluationReceipt, error) {
	if len(history) < 2 || len(history) > 510 {
		return nil, fmt.Errorf("bounded paired receipt history required")
	}
	raw, err := json.Marshal(history)
	if err != nil || len(raw) > MaxImportBytes {
		return nil, fmt.Errorf("receipt history exceeds bounds")
	}
	for _, r := range history {
		if r.Version != EndpointEvaluationVersion || r.Hash != endpointReceiptHash(r) {
			return nil, fmt.Errorf("historical receipt changed")
		}
	}
	if _, err := PairTaskCases([]TaskCaseRef{requests[0].Task.Ref}, []TaskCaseRef{requests[1].Task.Ref}); err != nil {
		return nil, err
	}
	if requests[0].Binding.ExperimentHash != requests[1].Binding.ExperimentHash || requests[0].Binding.VariantID == requests[1].Binding.VariantID || requests[0].Binding.RunID == requests[1].Binding.RunID {
		return nil, fmt.Errorf("distinct paired execution bindings required")
	}
	for name, left := range requests[0].EvaluatorDependencies {
		if right, ok := requests[1].EvaluatorDependencies[name]; ok && left != right {
			return nil, fmt.Errorf("common evaluator dependency mismatch: %s", name)
		}
	}
	if requests[0].ExpectedEvaluator != nil && requests[1].ExpectedEvaluator != nil && *requests[0].ExpectedEvaluator != *requests[1].ExpectedEvaluator {
		return nil, fmt.Errorf("common evaluator reference mismatch")
	}
	result := cloneJSON(history)
	for _, request := range requests {
		next, err := EvaluateIngestionEndpoint(request)
		if err != nil {
			return nil, err
		}
		var previous *EndpointEvaluationReceipt
		for i := len(history) - 1; i >= 0; i-- {
			old := history[i]
			if old.Binding == request.Binding && taskKey(old.CaseRef) == taskKey(request.Task.Ref) {
				if previous == nil {
					copy := old
					previous = &copy
				}
				if next.RawOutputHash != "" && old.RawOutputHash != "" && next.RawOutputHash != old.RawOutputHash {
					return nil, fmt.Errorf("retained raw output changed")
				}
			}
		}
		if previous == nil {
			return nil, fmt.Errorf("retained counterpart receipt missing")
		}
		if previous.InputHash != next.InputHash || previous.CaseRef.SuiteID != next.CaseRef.SuiteID || previous.CaseRef.Split != next.CaseRef.Split || previous.CaseRef.ExposureGroup != next.CaseRef.ExposureGroup || !sameDependencyGroup(previous.CaseRef.DependencyGroup, next.CaseRef.DependencyGroup) {
			return nil, fmt.Errorf("retained task input or cohort changed")
		}
		if next.RawOutputHash != "" && previous.RawOutputHash != "" && next.RawOutputHash != previous.RawOutputHash {
			return nil, fmt.Errorf("retained raw output changed")
		}
		next.PreviousReceiptHash = previous.Hash
		next.Hash = endpointReceiptHash(next)
		result = append(result, next)
	}
	leftComplete, rightComplete := true, true
	for _, name := range endpointEvaluatorDependencies {
		leftComplete = leftComplete && requests[0].EvaluatorDependencies[name] != ""
		rightComplete = rightComplete && requests[1].EvaluatorDependencies[name] != ""
	}
	if leftComplete && rightComplete && result[len(result)-2].Evaluator != result[len(result)-1].Evaluator {
		return nil, fmt.Errorf("common evaluator manifest mismatch")
	}
	return result, nil
}

// Producer row order stays intact. Only issue/reason sets are canonicalized:
// the mechanical normalizer can discover independent missing fields in map order.
func canonicalEndpointObservation(o semanticeval.IngestionProposalObservation) semanticeval.IngestionProposalObservation {
	o = cloneJSON(o)
	for key, a := range o.Availability {
		sort.Strings(a.Reasons)
		o.Availability[key] = a
	}
	for i := range o.Issues {
		sort.Strings(o.Issues[i].Affects)
	}
	sort.Slice(o.Issues, func(i, j int) bool { return endpointHash(o.Issues[i]) < endpointHash(o.Issues[j]) })
	return o
}
