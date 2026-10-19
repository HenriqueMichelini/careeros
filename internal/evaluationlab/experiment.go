package evaluationlab

import (
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
	"sort"
	"strconv"
	"strings"
)

const ExperimentVersion = "lab-experiment-v1"

// DigestRef identifies inert content, never an executable path or authority to run.
type DigestRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

type VariantRef struct {
	ID                     string               `json:"id"`
	AdapterVersion         string               `json:"adapterVersion"`
	ImplementationManifest DigestRef            `json:"implementationManifest"`
	BuildReceipt           DigestRef            `json:"buildReceipt"`
	Configuration          DigestRef            `json:"configuration"`
	PromptManifest         DigestRef            `json:"promptManifest"`
	StrategyManifest       DigestRef            `json:"strategyManifest"`
	ProducerContract       DigestRef            `json:"producerContract"`
	Normalizer             DigestRef            `json:"normalizer"`
	SimulationScript       DigestRef            `json:"simulationScript"`
	Dependencies           map[string]DigestRef `json:"dependencies"`
}

// ExperimentSharedPolicy fixes equal end-to-end ceilings, not consumed calls or
// variant-local reservations. Those observations belong to execution evidence.
type ExperimentSharedPolicy struct {
	Reference      DigestRef     `json:"reference"`
	Mode           string        `json:"mode"`
	RuntimeVersion string        `json:"runtimeVersion"`
	ClockBoundary  string        `json:"clockBoundary"`
	SchedulePolicy string        `json:"schedulePolicy"`
	Concurrency    int           `json:"concurrency"`
	Caps           ExecutionCaps `json:"caps"`
	RetryPolicy    string        `json:"retryPolicy"`
	FallbackPolicy string        `json:"fallbackPolicy"`
	ResumePolicy   string        `json:"resumePolicy"`
}

// ConsequenceOf names the exact treatment field that caused this dependency
// identity change. An empty value declares an independent conceptual treatment.
type ExperimentDifference struct {
	Field         string `json:"field"`
	Left          string `json:"left"`
	Right         string `json:"right"`
	Rationale     string `json:"rationale"`
	ConsequenceOf string `json:"consequenceOf,omitempty"`
}

type ExperimentDefinition struct {
	Version      string                 `json:"version"`
	ID           string                 `json:"id"`
	Hash         string                 `json:"hash"`
	Kind         string                 `json:"kind"`
	TaskContract DigestRef              `json:"taskContract"`
	Cases        []TaskCaseRef          `json:"cases"`
	Variants     [2]VariantRef          `json:"variants"`
	Evaluator    DigestRef              `json:"evaluator"`
	SharedPolicy ExperimentSharedPolicy `json:"sharedPolicy"`
	Differences  []ExperimentDifference `json:"differences"`
	RepeatCount  int                    `json:"repeatCount"`
}

// ExperimentEvidence is resolved evidence, not a browser selection. Registry,
// build attestation, trust admission and execution are separate responsibilities.
type ExperimentEvidence struct {
	TaskContract DigestRef              `json:"taskContract"`
	Cases        []TaskCaseRef          `json:"cases"`
	Evaluator    DigestRef              `json:"evaluator"`
	SharedPolicy ExperimentSharedPolicy `json:"sharedPolicy"`
	Variant      VariantRef             `json:"variant"`
}

type ExperimentReason struct {
	Code   string `json:"code"`
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

type ExperimentCompatibility struct {
	Version     string                 `json:"version"`
	Status      string                 `json:"status"`
	Differences []ExperimentDifference `json:"differences"`
	Reasons     []ExperimentReason     `json:"reasons"`
}

// FreezeExperiment returns caller-independent content bound by its digest.
// Editing a frozen record requires a new freeze; it never edits an old receipt.
func FreezeExperiment(def ExperimentDefinition) (ExperimentDefinition, error) {
	def = cloneJSON(def)
	result := checkExperimentDefinition(def)
	if result.Status != "valid" {
		return ExperimentDefinition{}, fmt.Errorf("%s experiment: %v", result.Status, result.Reasons)
	}
	def.Hash = experimentDigest(def)
	return def, nil
}

// CheckExperiment checks structural comparability only. A valid result makes no
// execution, trust, semantic-quality, causal-effect or overall-winner claim.
func CheckExperiment(def ExperimentDefinition, left, right ExperimentEvidence) ExperimentCompatibility {
	result := checkExperimentDefinition(def)
	if def.Hash == "" {
		result.add("unavailable", "metadata_missing", "hash", "frozen definition hash unavailable")
	} else if def.Hash != experimentDigest(def) {
		result.add("invalid", "definition_changed", "hash", "definition differs from its frozen content")
	}
	if result.Status != "valid" {
		return result
	}
	result.Differences = variantDifferences(left.Variant, right.Variant)
	for i, evidence := range []ExperimentEvidence{left, right} {
		side := []string{"left", "right"}[i]
		validateExperimentFields(&result, side+".taskContract", digestFields(evidence.TaskContract))
		validateExperimentFields(&result, side+".evaluator", digestFields(evidence.Evaluator))
		validateExperimentPolicy(&result, side+".sharedPolicy", evidence.SharedPolicy)
		validateExperimentVariant(&result, side+".variant", evidence.Variant)
		compareExperimentFields(&result, side+".taskContract", "task_mismatch", digestFields(def.TaskContract), digestFields(evidence.TaskContract))
		compareExperimentFields(&result, side+".evaluator", "evaluator_mismatch", digestFields(def.Evaluator), digestFields(evidence.Evaluator))
		compareExperimentFields(&result, side+".sharedPolicy", "policy_mismatch", policyFields(def.SharedPolicy), policyFields(evidence.SharedPolicy))
		expected, actual := variantFields(def.Variants[i]), variantFields(evidence.Variant)
		expected["id"] = def.Variants[i].ID
		actual["id"] = evidence.Variant.ID
		if len(evidence.Variant.Dependencies) == 0 {
			for key := range expected {
				if strings.HasPrefix(key, "dependencies.") {
					delete(expected, key)
				}
			}
		}
		compareExperimentFields(&result, side+".variant", "unexpected_difference", expected, actual)
		compareKnownExperimentCases(&result, side+".cases", def.Cases, evidence.Cases)
		if validateExperimentCases(&result, side+".cases", evidence.Cases) {
			if _, err := PairTaskCases(def.Cases, evidence.Cases); err != nil {
				result.add("invalid", "case_mismatch", side+".cases", err.Error())
			}
		}
	}

	return result
}

func checkExperimentDefinition(def ExperimentDefinition) ExperimentCompatibility {
	result := ExperimentCompatibility{Version: ExperimentVersion, Status: "valid", Differences: variantDifferences(def.Variants[0], def.Variants[1]), Reasons: []ExperimentReason{}}
	validateExperimentFields(&result, "definition", map[string]string{"version": def.Version, "id": def.ID, "kind": def.Kind})
	validateExperimentFields(&result, "taskContract", digestFields(def.TaskContract))
	validateExperimentFields(&result, "evaluator", digestFields(def.Evaluator))
	validateExperimentPolicy(&result, "sharedPolicy", def.SharedPolicy)
	validateExperimentVariant(&result, "variants.left", def.Variants[0])
	validateExperimentVariant(&result, "variants.right", def.Variants[1])
	validateExperimentCases(&result, "cases", def.Cases)
	if result.Status != "valid" {
		return result
	}
	if def.Version != ExperimentVersion {
		result.add("invalid", "unsupported_version", "version", "unsupported experiment version")
	}
	if len(def.Differences) > 256 {
		result.add("invalid", "definition_bounds", "differences", "too many difference declarations")
		return result
	}

	actualFields := map[string]ExperimentDifference{}
	for _, actual := range result.Differences {
		actualFields[actual.Field] = actual
	}
	declarations := map[string]ExperimentDifference{}
	for _, difference := range def.Differences {
		if _, duplicate := declarations[difference.Field]; duplicate {
			result.add("invalid", "duplicate_difference", difference.Field, "difference declared more than once")
		}
		declarations[difference.Field] = difference
		actual, ok := actualFields[difference.Field]
		if !ok || strings.ContainsAny(difference.Field, "*?[]") || difference.Left != actual.Left || difference.Right != actual.Right || strings.TrimSpace(difference.Rationale) == "" {
			result.add("invalid", "invalid_declaration", difference.Field, "declaration must name an actual exact variant change with rationale")
		}
	}
	for _, actual := range result.Differences {
		if _, ok := declarations[actual.Field]; !ok {
			result.add("invalid", "unexpected_difference", actual.Field, "observed change has no declaration")
		}
	}
	roots := []string{}
	for _, difference := range def.Differences {
		if difference.ConsequenceOf == "" {
			roots = append(roots, difference.Field)
		}
	}
	switch def.Kind {
	case "single-dimension":
		if len(roots) != 1 {
			result.add("invalid", "treatment_count", "kind", "single-dimension requires one conceptual treatment")
		}
	case "complete-design":
		if len(roots) == 0 {
			result.add("invalid", "treatment_count", "kind", "complete-design requires an intentional treatment")
		}
	case "repeatability":
		if len(result.Differences) != 0 || len(def.Differences) != 0 {
			result.add("invalid", "repeatability_changed", "kind", "repeatability requires identical variant content")
		}
	default:
		result.add("invalid", "unsupported_kind", "kind", "unsupported experiment kind")
	}
	for _, difference := range def.Differences {
		if difference.ConsequenceOf == "" {
			continue
		}
		treatment, ok := declarations[difference.ConsequenceOf]
		if !ok || treatment.ConsequenceOf != "" || treatment.Field == difference.Field {
			result.add("invalid", "invalid_consequence", difference.Field, "consequence must identify a declared independent treatment")
		}
	}
	if _, err := PairTaskCases(def.Cases, def.Cases); err != nil {
		result.add("invalid", "case_mismatch", "cases", err.Error())
	}
	if def.RepeatCount < 1 || def.RepeatCount > 3 {
		result.add("invalid", "repeat_count", "repeatCount", "one to three frozen repetitions required")
	}
	counts := map[taskPairKey]int{}
	firstRefs := map[taskPairKey]TaskCaseRef{}
	for _, ref := range def.Cases {
		if ref.RepeatOrdinal > def.RepeatCount {
			result.add("invalid", "repeat_count", "cases", "case ordinal exceeds repeat count")
		}
		key := taskKey(ref)
		key.RepeatOrdinal = 0
		counts[key]++
		if first, ok := firstRefs[key]; ok {
			compareExperimentFields(&result, "cases", "repeat_metadata_mismatch", caseFields(first), caseFields(ref))
		} else {
			firstRefs[key] = ref
		}
	}
	for _, count := range counts {
		if count != def.RepeatCount {
			result.add("invalid", "repeat_count", "cases", "every task case requires all frozen ordinals")
		}
	}

	return result
}

func (result *ExperimentCompatibility) add(status, code, field, detail string) {
	if status == "invalid" || result.Status == "valid" {
		result.Status = status
	}
	result.Reasons = append(result.Reasons, ExperimentReason{Code: code, Field: field, Detail: detail})
}

func variantFields(variant VariantRef) map[string]string {
	fields := map[string]string{"adapterVersion": variant.AdapterVersion}
	refs := map[string]DigestRef{"implementationManifest": variant.ImplementationManifest, "buildReceipt": variant.BuildReceipt, "configuration": variant.Configuration, "promptManifest": variant.PromptManifest, "strategyManifest": variant.StrategyManifest, "producerContract": variant.ProducerContract, "normalizer": variant.Normalizer, "simulationScript": variant.SimulationScript}
	for name, ref := range variant.Dependencies {
		refs["dependencies."+name] = ref
	}
	for name, ref := range refs {
		fields[name+".id"] = ref.ID
		fields[name+".version"] = ref.Version
		fields[name+".hash"] = ref.Hash
	}
	return fields
}

func variantDifferences(left, right VariantRef) []ExperimentDifference {
	l, r := variantFields(left), variantFields(right)
	keys := map[string]bool{}
	for key := range l {
		keys[key] = true
	}
	for key := range r {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	differences := []ExperimentDifference{}
	for _, key := range ordered {
		lv, lok := l[key]
		rv, rok := r[key]
		if !lok {
			lv = "absent"
		}
		if !rok {
			rv = "absent"
		}
		if lv != rv {
			differences = append(differences, ExperimentDifference{Field: key, Left: lv, Right: rv})
		}
	}
	return differences
}

func experimentDigest(def ExperimentDefinition) string {
	def.Hash = ""
	raw, _ := json.Marshal(def)
	return semanticeval.Hash(raw)
}

func digestFields(ref DigestRef) map[string]string {
	return map[string]string{"id": ref.ID, "version": ref.Version, "hash": ref.Hash}
}

func validateExperimentFields(result *ExperimentCompatibility, prefix string, fields map[string]string) {
	for _, key := range sortedExperimentKeys(fields) {
		value := fields[key]
		if strings.TrimSpace(value) == "" {
			result.add("unavailable", "metadata_missing", prefix+"."+key, "required identity unavailable")
			continue
		}
		if len(value) > 256 || ((key == "hash" || strings.HasSuffix(key, ".hash")) && !evidenceHash.MatchString(value)) {
			result.add("invalid", "invalid_metadata", prefix+"."+key, "malformed identity")
		}
	}
}

func validateExperimentVariant(result *ExperimentCompatibility, prefix string, variant VariantRef) {
	fields := variantFields(variant)
	fields["id"] = variant.ID
	validateExperimentFields(result, prefix, fields)
	if len(variant.Dependencies) == 0 {
		result.add("unavailable", "metadata_missing", prefix+".dependencies", "exact implementation dependencies unavailable")
	}
	if len(variant.Dependencies) > 256 {
		result.add("invalid", "definition_bounds", prefix+".dependencies", "too many implementation dependencies")
	}
	for _, key := range sortedExperimentKeys(variant.Dependencies) {
		if strings.TrimSpace(key) == "" || len(key) > 256 || strings.ContainsAny(key, "*?[]") {
			result.add("invalid", "invalid_metadata", prefix+".dependencies", "dependency keys must be exact and bounded")
		}
	}
}

func policyFields(policy ExperimentSharedPolicy) map[string]string {
	fields := map[string]string{"mode": policy.Mode, "runtimeVersion": policy.RuntimeVersion, "clockBoundary": policy.ClockBoundary, "schedulePolicy": policy.SchedulePolicy, "retryPolicy": policy.RetryPolicy, "fallbackPolicy": policy.FallbackPolicy, "resumePolicy": policy.ResumePolicy}
	for key, value := range digestFields(policy.Reference) {
		fields["reference."+key] = value
	}
	for key, value := range map[string]int64{"concurrency": int64(policy.Concurrency), "maxCases": int64(policy.Caps.MaxCases), "maxHttpAttempts": int64(policy.Caps.MaxHTTPAttempts), "maxProviderCalls": int64(policy.Caps.MaxProviderCalls), "maxArtifactBytes": policy.Caps.MaxArtifactBytes, "deadlineMs": policy.Caps.DeadlineMS} {
		fields[key] = ""
		if value != 0 {
			fields[key] = strconv.FormatInt(value, 10)
		}
	}
	return fields
}

func validateExperimentPolicy(result *ExperimentCompatibility, prefix string, policy ExperimentSharedPolicy) {
	validateExperimentFields(result, prefix, policyFields(policy))
	for key, value := range map[string]string{"mode": policy.Mode, "schedulePolicy": policy.SchedulePolicy, "retryPolicy": policy.RetryPolicy, "fallbackPolicy": policy.FallbackPolicy, "resumePolicy": policy.ResumePolicy} {
		expected := "none"
		if key == "mode" {
			expected = "controlled"
		}
		if key == "schedulePolicy" {
			expected = "alternating-pairs-v1"
		}
		if value != "" && value != expected {
			result.add("invalid", "unsupported_policy", prefix+"."+key, "unsupported first-release execution policy")
		}
	}
	if policy.Concurrency != 0 && policy.Concurrency != 1 {
		result.add("invalid", "unsupported_policy", prefix+".concurrency", "sequential concurrency required")
	}
	caps := policy.Caps
	if caps.MaxCases < 0 || caps.MaxCases > 512 || caps.MaxHTTPAttempts < 0 || caps.MaxHTTPAttempts > 2048 || caps.MaxProviderCalls < 0 || caps.MaxProviderCalls > 4096 || caps.MaxArtifactBytes < 0 || caps.MaxArtifactBytes > MaxImportBytes || caps.DeadlineMS < 0 || caps.DeadlineMS > 60000 {
		result.add("invalid", "unsupported_policy", prefix+".caps", "ceilings outside supported planner bounds")
	}
}

func validateExperimentCases(result *ExperimentCompatibility, prefix string, cases []TaskCaseRef) bool {
	complete := true
	if len(cases) == 0 {
		result.add("unavailable", "metadata_missing", prefix, "task cohort unavailable")
		return false
	}
	for _, ref := range cases {
		fields := map[string]string{"suiteId": ref.SuiteID, "caseId": ref.CaseID, "taskVersion": ref.TaskVersion, "input.hash": ref.InputHash, "expectation.hash": ref.ExpectationHash, "originalFixture.hash": ref.OriginalFixtureHash, "language": ref.Language, "split": ref.Split}
		missing := ref.RepeatOrdinal == 0
		for _, value := range fields {
			if strings.TrimSpace(value) == "" {
				missing = true
			}
		}
		if missing {
			validateExperimentFields(result, prefix, fields)
			if ref.RepeatOrdinal == 0 {
				result.add("unavailable", "metadata_missing", prefix+".repeatOrdinal", "repetition unavailable")
			}
			complete = false
			continue
		}
		if err := validateTaskRef(ref); err != nil {
			result.add("invalid", "invalid_metadata", prefix, err.Error())
			complete = false
		}
	}
	return complete
}

func compareExperimentFields(result *ExperimentCompatibility, prefix, code string, expected, actual map[string]string) {
	keys := map[string]bool{}
	for key := range expected {
		keys[key] = true
	}
	for key := range actual {
		keys[key] = true
	}
	for _, key := range sortedExperimentKeys(keys) {
		want, wok := expected[key]
		got, gok := actual[key]
		if (wok && want == "") || (gok && got == "") {
			continue
		}
		if !wok {
			want = "absent"
		}
		if !gok {
			got = "absent"
		}
		if want != got {
			result.add("invalid", code, prefix+"."+key, "evidence differs from frozen identity")
		}
	}
}

func sortedExperimentKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Missing input/labels do not hide a known mismatch in the other fields. Match
// only the available declared case/repetition scope before checking those values.
func compareKnownExperimentCases(result *ExperimentCompatibility, prefix string, expected, actual []TaskCaseRef) {
	index := map[taskPairKey]TaskCaseRef{}
	for _, ref := range expected {
		key := taskKey(ref)
		key.InputHash = ""
		index[key] = ref
	}
	for _, ref := range actual {
		key := taskKey(ref)
		key.InputHash = ""
		if match, ok := index[key]; ok {
			compareExperimentFields(result, prefix, "case_mismatch", caseFields(match), caseFields(ref))
		}
	}
}

func caseFields(ref TaskCaseRef) map[string]string {
	group, _ := json.Marshal(ref.DependencyGroup)
	return map[string]string{"suiteId": ref.SuiteID, "taskVersion": ref.TaskVersion, "input.hash": ref.InputHash, "expectation.hash": ref.ExpectationHash, "split": ref.Split, "exposureGroup": ref.ExposureGroup, "dependencyGroup": string(group)}
}
