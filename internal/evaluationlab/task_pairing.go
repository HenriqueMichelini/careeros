package evaluationlab

import "fmt"

type TaskCasePair struct {
	Left  TaskCaseRef `json:"left"`
	Right TaskCaseRef `json:"right"`
}

type taskPairKey struct {
	TaskVersion   string
	CaseID        string
	InputHash     string
	Language      string
	RepeatOrdinal int
}

func taskKey(ref TaskCaseRef) taskPairKey {
	return taskPairKey{ref.TaskVersion, ref.CaseID, ref.InputHash, ref.Language, ref.RepeatOrdinal}
}

// PairTaskCases admits complete shared task cohorts, preserving left-side order.
// Execution IDs and variant-local stage positions never participate in pairing.
// Missing observations belong to later execution evidence, not a new cohort.
func PairTaskCases(left, right []TaskCaseRef) ([]TaskCasePair, error) {
	if len(left) == 0 || len(left) != len(right) || len(left) > 512 {
		return nil, fmt.Errorf("matching bounded nonempty task cohorts required")
	}
	index := map[taskPairKey]TaskCaseRef{}
	for _, ref := range right {
		if err := validateTaskRef(ref); err != nil {
			return nil, err
		}
		key := taskKey(ref)
		if _, exists := index[key]; exists {
			return nil, fmt.Errorf("duplicate right task/repetition key")
		}
		index[key] = ref
	}
	seen := map[taskPairKey]bool{}
	pairs := make([]TaskCasePair, 0, len(left))
	for _, ref := range left {
		if err := validateTaskRef(ref); err != nil {
			return nil, err
		}
		key := taskKey(ref)
		if seen[key] {
			return nil, fmt.Errorf("duplicate left task/repetition key")
		}
		seen[key] = true
		counterpart, ok := index[key]
		if !ok {
			return nil, fmt.Errorf("unpaired shared task input or repetition")
		}
		if ref.ExpectationHash != counterpart.ExpectationHash {
			return nil, fmt.Errorf("shared task expectation mismatch")
		}
		if ref.SuiteID != counterpart.SuiteID || ref.Split != counterpart.Split || ref.ExposureGroup != counterpart.ExposureGroup || !sameDependencyGroup(ref.DependencyGroup, counterpart.DependencyGroup) {
			return nil, fmt.Errorf("shared task cohort metadata mismatch")
		}
		pairs = append(pairs, TaskCasePair{cloneTaskRef(ref), cloneTaskRef(counterpart)})
	}
	return pairs, nil
}

func validateTaskRef(ref TaskCaseRef) error {
	if ref.SuiteID == "" || len(ref.SuiteID) > 256 || ref.CaseID == "" || len(ref.CaseID) > 256 || ref.TaskVersion == "" || len(ref.TaskVersion) > 256 || ref.RepeatOrdinal < 1 || !evidenceHash.MatchString(ref.InputHash) || !evidenceHash.MatchString(ref.ExpectationHash) || !evidenceHash.MatchString(ref.OriginalFixtureHash) || !contains([]string{"en", "pt-BR"}, ref.Language) || ref.Split == "" || len(ref.Split) > 256 || len(ref.ExposureGroup) > 256 || (ref.DependencyGroup != nil && len(*ref.DependencyGroup) > 256) {
		return fmt.Errorf("invalid task/repetition reference")
	}
	return nil
}

func sameDependencyGroup(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func cloneTaskRef(ref TaskCaseRef) TaskCaseRef {
	if ref.DependencyGroup != nil {
		group := *ref.DependencyGroup
		ref.DependencyGroup = &group
	}
	return ref
}
