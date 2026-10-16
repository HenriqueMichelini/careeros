package evaluationlab

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"professional-information-repo/internal/semanticeval"
	"runtime"
	"sync"
)

const PlanVersion = "lab-plan-v1"

type ExecutionCaps struct {
	MaxCases         int   `json:"maxCases"`
	MaxHTTPAttempts  int   `json:"maxHttpAttempts"`
	MaxProviderCalls int   `json:"maxProviderCalls"`
	MaxArtifactBytes int64 `json:"maxArtifactBytes"`
	DeadlineMS       int64 `json:"deadlineMs"`
}
type Plan struct {
	Provenance                  Provenance        `json:"provenance"`
	Version                     string            `json:"version"`
	ID                          string            `json:"id"`
	Hash                        string            `json:"hash"`
	Selection                   Selection         `json:"selection"`
	SuiteVersion                string            `json:"suiteVersion"`
	InputHashes                 map[string]string `json:"inputHashes"`
	CaseOrder                   []string          `json:"caseOrder"`
	RequiredHTTPAttempts        int               `json:"requiredHttpAttempts"`
	MaximumPlannedProviderCalls int               `json:"maximumPlannedProviderCalls"`
	Caps                        ExecutionCaps     `json:"caps"`
	RuntimeVersion              string            `json:"runtimeVersion"`
	Concurrency                 int               `json:"concurrency"`
}
type Planner struct {
	provenance Provenance
	mu         sync.Mutex
	assets     map[string][]byte
	plans      map[string]Plan
}

// NewPlanner owns verified synthetic bytes. Caller changes cannot alter a plan.
// Asset IDs are fixed catalog IDs, not paths or provider addresses.
func NewPlanner(assets map[string][]byte, provenance ...Provenance) (*Planner, error) {
	if len(provenance) > 1 {
		return nil, fmt.Errorf("one source snapshot expected")
	}
	approved := map[string]string{}
	for _, suite := range suiteCatalog {
		approved[suite.ID] = suite.FixtureHash
	}
	for _, suite := range suiteCatalog {
		if suite.ID == "application-context" {
			approved["application-baseline"] = suite.FixtureHashes["baseline"]
		}
	}
	p := &Planner{assets: map[string][]byte{}, plans: map[string]Plan{}}
	if len(provenance) == 1 {
		p.provenance = cloneJSON(provenance[0])
	}
	for id, raw := range assets {
		if expected, ok := approved[id]; !ok || semanticeval.Hash(raw) != expected {
			return nil, fmt.Errorf("unknown or changed fixture asset")
		}
		if len(raw) > MaxImportBytes {
			return nil, fmt.Errorf("fixture asset exceeds bounds")
		}
		if err := validateJSON(raw); err != nil {
			return nil, err
		}
		p.assets[id] = bytes.Clone(raw)
	}
	return p, nil
}

func (p *Planner) Plan(selection Selection, caps ExecutionCaps) (Plan, error) {
	if err := ValidateSelection(selection); err != nil {
		return Plan{}, err
	}
	if caps.MaxCases < 1 || caps.MaxCases > 512 || caps.MaxHTTPAttempts < 1 || caps.MaxHTTPAttempts > 2048 || caps.MaxProviderCalls < 1 || caps.MaxProviderCalls > 4096 || caps.MaxArtifactBytes < 1 || caps.MaxArtifactBytes > MaxImportBytes || caps.DeadlineMS < 1 || caps.DeadlineMS > 60000 {
		return Plan{}, fmt.Errorf("execution caps outside supported bounds")
	}
	var suite Suite
	for _, candidate := range suiteCatalog {
		if candidate.ID == selection.Suite {
			suite = candidate
			break
		}
	}
	inputs := []string{selection.Suite}
	if selection.Suite == "application-context" {
		inputs = append(inputs, "semantic-quality", "application-baseline")
	}
	hashes := map[string]string{}
	for _, id := range inputs {
		raw, ok := p.assets[id]
		if !ok {
			return Plan{}, fmt.Errorf("required fixture asset unavailable")
		}
		hashes[id] = semanticeval.Hash(raw)
	}
	order := append([]string{}, selection.CaseIDs...)
	if len(order) == 0 {
		for _, c := range suite.Cases {
			order = append(order, c.ID)
		}
	}
	selection.CaseIDs = append([]string{}, order...)
	plan := Plan{Version: PlanVersion, ID: newLabID(), Selection: selection, SuiteVersion: suite.Version, InputHashes: hashes, CaseOrder: order, Caps: caps, RuntimeVersion: runtime.Version(), Concurrency: 1, Provenance: cloneJSON(p.provenance)}
	for _, id := range order {
		for _, c := range suite.Cases {
			if c.ID == id {
				plan.RequiredHTTPAttempts += c.RequiredSteps
				plan.MaximumPlannedProviderCalls += c.MaximumCalls
			}
		}
	}
	if len(order) > caps.MaxCases || plan.RequiredHTTPAttempts > caps.MaxHTTPAttempts {
		return Plan{}, fmt.Errorf("selection exceeds case or HTTP cap")
	}
	plan.Hash = planDigest(plan)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.plans) >= 128 {
		return Plan{}, fmt.Errorf("too many frozen plans")
	}
	p.plans[plan.ID] = clonePlan(plan)
	return clonePlan(plan), nil
}

func (p *Planner) ValidatePlan(plan Plan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	stored, ok := p.plans[plan.ID]
	if !ok || plan.Version != PlanVersion || plan.Hash != stored.Hash || planDigest(plan) != stored.Hash {
		return fmt.Errorf("unknown or altered frozen plan")
	}
	for id, hash := range stored.InputHashes {
		if semanticeval.Hash(p.assets[id]) != hash {
			return fmt.Errorf("fixture changed since planning")
		}
	}
	return nil
}

func newLabID() string {
	var raw [16]byte
	_, err := rand.Read(raw[:])
	if err != nil {
		panic("secure evaluation identity unavailable")
	}
	return hex.EncodeToString(raw[:])
}
func planDigest(plan Plan) string {
	plan.Hash = ""
	raw, _ := json.Marshal(plan)
	return semanticeval.Hash(raw)
}
func clonePlan(plan Plan) Plan {
	raw, _ := json.Marshal(plan)
	var copy Plan
	_ = json.Unmarshal(raw, &copy)
	return copy
}
