package evaluationlab

import (
	"encoding/json"
	"fmt"
	"math"
	"professional-information-repo/internal/semanticeval"
	"strings"
	"sync"
	"time"
)

type LiveGuardOptions struct {
	Enabled       bool
	HumanReviewer string
	Origin        string
	Now           func() time.Time
}
type GuardDispatchSpec struct {
	MaxOutputTokens  int64  `json:"maxOutputTokens"`
	Reasoning        string `json:"reasoning"`
	LeafID           string `json:"leafId"`
	PayloadKind      string `json:"payloadKind"`
	ID               string `json:"id"`
	CaseID           string `json:"caseId"`
	HTTPID           string `json:"httpId"`
	Stage            string `json:"stage"`
	Recipient        string `json:"recipient"`
	Model            string `json:"model"`
	TokenReservation int64  `json:"tokenReservation"`
	PayloadHash      string `json:"payloadHash"`
}
type LivePreflightRequest struct {
	PriceSnapshotVersion string              `json:"priceSnapshotVersion,omitempty"`
	Prices               []semanticeval.Rate `json:"prices,omitempty"`
	Plan                 Plan                `json:"plan"`
	MaxTokenUnits        int64               `json:"maxTokenUnits"`
	MaxOutputTokens      int64               `json:"maxOutputTokens"`
	ApprovalLifetimeMS   int64               `json:"approvalLifetimeMs"`
	Dispatches           []GuardDispatchSpec `json:"dispatches"`
}
type LivePreflight struct {
	PriceSnapshotVersion string              `json:"priceSnapshotVersion,omitempty"`
	Prices               []semanticeval.Rate `json:"prices,omitempty"`
	Version              string              `json:"version"`
	ID                   string              `json:"id"`
	Hash                 string              `json:"hash"`
	Instance             string              `json:"publicInstanceIdentity"`
	Origin               string              `json:"origin"`
	ExpiresAt            string              `json:"expiresAt"`
	Plan                 Plan                `json:"plan"`
	Identities           map[string]string   `json:"identities"`
	MaxTokenUnits        int64               `json:"maxTokenUnits"`
	MaxOutputTokens      int64               `json:"maxOutputTokens"`
	Dispatches           []GuardDispatchSpec `json:"dispatches"`
	Recipients           []string            `json:"recipients"`
	Policy               string              `json:"policy"`
	Retention            string              `json:"retention"`
}
type guardRecord struct {
	Comparison         *GuardComparison      `json:"comparison,omitempty"`
	Approval           *GuardApprovalReceipt `json:"approval,omitempty"`
	Calls              []GuardRetainedCall   `json:"calls,omitempty"`
	ReservedTokenUnits int64                 `json:"reservedTokenUnits"`
	Next               int                   `json:"nextScheduleIndex"`
	Version            string                `json:"version"`
	Preflight          LivePreflight         `json:"preflight"`
	State              string                `json:"state"`
}
type LiveGuard struct {
	active   string
	secrets  map[string]*guardSecrets
	mu       sync.Mutex
	planner  *Planner
	store    *FileStore
	options  LiveGuardOptions
	instance string
	records  map[string]guardRecord
	closed   bool
}

func NewLiveGuard(planner *Planner, store *FileStore, options LiveGuardOptions) (*LiveGuard, error) {
	if planner == nil || store == nil || len(options.HumanReviewer) > 128 {
		return nil, fmt.Errorf("guard dependencies unavailable")
	}
	if options.Origin == "" {
		options.Origin = "local-human-attestation"
	}
	if !contains([]string{"local-human-attestation", "controlled-test"}, options.Origin) {
		return nil, fmt.Errorf("invalid guard authority origin")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	instance := newLabID()
	if err := store.claimGuard(instance); err != nil {
		return nil, err
	}
	records, err := store.loadGuardRecords()
	if err != nil {
		store.releaseGuard(instance)
		return nil, err
	}
	for id, record := range records {
		if record.State == "authorized" || record.State == "running" {
			record.State = "unknown-final"
			if err = store.saveGuard(record); err != nil {
				store.releaseGuard(instance)
				return nil, err
			}
			records[id] = record
		}
	}
	return &LiveGuard{planner: planner, store: store, options: options, instance: instance, records: records, secrets: map[string]*guardSecrets{}}, nil
}
func (g *LiveGuard) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	draining := false
	for id, secret := range g.secrets {
		draining = draining || secret.inFlight
		secret.cancel()
		clear(secret.credentials)
		delete(g.secrets, id)
	}
	g.active = ""
	if !draining {
		g.store.releaseGuard(g.instance)
	}
}
func (g *LiveGuard) Preflight(request LivePreflightRequest) (LivePreflight, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || !g.options.Enabled || g.active != "" {
		return LivePreflight{}, fmt.Errorf("guard disabled")
	}
	if err := g.planner.ValidatePlan(request.Plan); err != nil {
		return LivePreflight{}, err
	}
	if request.MaxTokenUnits < 1 || request.MaxTokenUnits > 1000000 || request.MaxOutputTokens < 1 || request.MaxOutputTokens > 16384 || request.ApprovalLifetimeMS < 1 || request.ApprovalLifetimeMS > 600000 || len(request.Dispatches) < 1 || len(request.Dispatches) > 4096 || len(g.records) >= 128 {
		return LivePreflight{}, fmt.Errorf("guard planning bounds exceeded")
	}
	if len(request.Prices) > 64 || len(request.PriceSnapshotVersion) > 128 || len(request.Prices) > 0 && request.PriceSnapshotVersion == "" {
		return LivePreflight{}, fmt.Errorf("bounded named price snapshot required")
	}
	seenPrices := map[string]bool{}
	for _, rate := range request.Prices {
		if safeGuardModel(rate.Model) == "" || seenPrices[rate.Model] {
			return LivePreflight{}, fmt.Errorf("unsupported or duplicate priced model")
		}
		if _, err := time.Parse("2006-01-02", rate.AsOf); err != nil {
			return LivePreflight{}, fmt.Errorf("dated exact-model price required")
		}
		for _, value := range []float64{rate.Input, rate.Cached, rate.Write, rate.Output} {
			if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return LivePreflight{}, fmt.Errorf("invalid price")
			}
		}
		seenPrices[rate.Model] = true
	}
	sourceRaw, _ := json.Marshal(request.Plan.Provenance.SourceHashes)
	scorer := request.Plan.Provenance.SourceHashes["internal/semanticeval/score.go"]
	if len(request.Plan.Provenance.SourceHashes) == 0 || !evidenceHash.MatchString(scorer) {
		return LivePreflight{}, fmt.Errorf("complete source and scorer identities required")
	}
	preflight := LivePreflight{PriceSnapshotVersion: request.PriceSnapshotVersion, Prices: cloneJSON(request.Prices), Version: LiveGuardVersion, ID: newLabID(), Instance: g.instance, Origin: "controlled-verification-only", ExpiresAt: g.options.Now().Add(time.Duration(request.ApprovalLifetimeMS) * time.Millisecond).UTC().Format(time.RFC3339Nano), Plan: clonePlan(request.Plan), MaxTokenUnits: request.MaxTokenUnits, MaxOutputTokens: request.MaxOutputTokens, Dispatches: cloneJSON(request.Dispatches), Recipients: []string{}, Policy: "one-active-run; concurrency=1; exact-order; stop-on-first-failure; no-retry/repair/fallback/resume; nonrefundable-reservations; unknown-total-cost-unavailable", Retention: "private bounded append-only guard evidence; no credentials or authorization tokens"}
	source := semanticeval.Hash(sourceRaw)
	preflight.Identities = map[string]string{"fixture": request.Plan.Provenance.FixtureHash, "labels": versionedHash("lab-label-fixture-set-v1", request.Plan.InputHashes), "source": source, "prompt": "lab-prompt-source-envelope-v1:" + source, "scorer": scorer, "schema": ResultVersion, "strategy": source, "runtime": request.Plan.RuntimeVersion}
	fixtures, leafOrder, err := g.guardFixtures(request.Plan)
	if err != nil {
		return LivePreflight{}, err
	}
	cases := map[string]bool{}
	httpIDs := map[string]string{}
	stages := map[string]bool{}
	leafHTTP := map[string]string{}
	recipients := map[string]bool{}
	nextLeaf := 0
	previousLeaf := ""
	for i := range preflight.Dispatches {
		call := &preflight.Dispatches[i]
		if call.LeafID == "" {
			call.LeafID = call.CaseID
		}
		leafKey := call.CaseID + "/" + call.LeafID
		if leafKey != previousLeaf {
			if nextLeaf >= len(leafOrder) || leafOrder[nextLeaf] != leafKey {
				return LivePreflight{}, fmt.Errorf("dispatch leaf order does not match frozen plan")
			}
			nextLeaf++
			previousLeaf = leafKey
		}
		fixture, ok := fixtures[leafKey]
		if !ok || call.HTTPID == "" || len(call.HTTPID) > 128 || call.Stage == "" || len(call.Stage) > 128 || call.ID != "" || call.PayloadHash != "" || call.PayloadKind != "" || call.TokenReservation < request.MaxOutputTokens || call.TokenReservation > 1000000 || call.MaxOutputTokens != 0 && call.MaxOutputTokens != request.MaxOutputTokens {
			return LivePreflight{}, fmt.Errorf("exact bounded owned dispatch required")
		}
		if call.Recipient == "api.typesafe.ai" {
			if call.Model != "jev-1.13.0" || call.Reasoning != "" {
				return LivePreflight{}, fmt.Errorf("unsupported recipient model")
			}
		} else if call.Recipient == "api.openai.com" {
			allowed := false
			for _, config := range semanticeval.Configurations() {
				if config.ID == request.Plan.Selection.Configuration && config.Model == call.Model {
					allowed = true
					if call.Reasoning != "" && call.Reasoning != config.Reasoning {
						return LivePreflight{}, fmt.Errorf("changed configured reasoning")
					}
					call.Reasoning = config.Reasoning
				}
			}
			if !allowed {
				return LivePreflight{}, fmt.Errorf("unsupported configured model")
			}
		} else {
			return LivePreflight{}, fmt.Errorf("unsupported recipient")
		}
		key := call.HTTPID + "/" + call.Stage
		if stages[key] || httpIDs[call.HTTPID] != "" && httpIDs[call.HTTPID] != leafKey || leafHTTP[leafKey] != "" && leafHTTP[leafKey] != call.HTTPID {
			return LivePreflight{}, fmt.Errorf("duplicate or mixed dispatch leg")
		}
		stages[key] = true
		httpIDs[call.HTTPID] = leafKey
		leafHTTP[leafKey] = call.HTTPID
		cases[call.CaseID] = true
		recipients[call.Recipient] = true
		call.MaxOutputTokens = request.MaxOutputTokens
		call.ID = fmt.Sprintf("%s:%d", preflight.ID, i)
		call.PayloadHash = semanticeval.Hash(fixture)
		call.PayloadKind = "frozen-synthetic-step-input; not a captured provider request"
	}
	if len(cases) != len(request.Plan.CaseOrder) || len(httpIDs) != request.Plan.RequiredHTTPAttempts || len(leafHTTP) != len(fixtures) || len(preflight.Dispatches) > request.Plan.MaximumPlannedProviderCalls {
		return LivePreflight{}, fmt.Errorf("dispatch cohort does not match frozen plan")
	}
	for _, recipient := range []string{"api.openai.com", "api.typesafe.ai"} {
		if recipients[recipient] {
			preflight.Recipients = append(preflight.Recipients, recipient)
		}
	}
	models, _ := json.Marshal(preflight.Dispatches)
	preflight.Identities["models-and-order"] = semanticeval.Hash(models)
	preflight.Hash = livePreflightHash(preflight)
	record := guardRecord{Version: LiveGuardVersion, Preflight: preflight, State: "planned"}
	if err = g.store.saveGuard(record); err != nil {
		return LivePreflight{}, err
	}
	g.records[preflight.ID] = record
	return cloneJSON(preflight), nil
}
func (g *LiveGuard) guardFixtures(plan Plan) (map[string]json.RawMessage, []string, error) {
	var corpus struct {
		Cases []semanticeval.Case `json:"cases"`
	}
	if err := json.Unmarshal(g.planner.assets[plan.Selection.Suite], &corpus); err != nil {
		return nil, nil, err
	}
	result := map[string]json.RawMessage{}
	order := []string{}
	var collect func(semanticeval.Case, string) error
	collect = func(c semanticeval.Case, root string) error {
		if c.Task == "cv_generation" {
			return fmt.Errorf("CV guard calibration awaits delivered owner contracts")
		}
		if len(c.Steps) > 0 {
			for _, step := range c.Steps {
				if err := collect(step, root); err != nil {
					return err
				}
			}
			return nil
		}
		if err := validateJSON(c.Request); err != nil {
			return err
		}
		result[root+"/"+c.ID] = c.Request
		order = append(order, root+"/"+c.ID)
		return nil
	}
	for _, id := range plan.CaseOrder {
		found := false
		for _, c := range corpus.Cases {
			if c.ID == id {
				if err := collect(c, c.ID); err != nil {
					return nil, nil, err
				}
				found = true
				break
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("owned semantic cohort required")
		}
	}
	return result, order, nil
}
func livePreflightHash(preflight LivePreflight) string {
	preflight.Hash = ""
	raw, _ := json.Marshal(preflight)
	return semanticeval.Hash(raw)
}
func (g *LiveGuard) ExportPreflight(id string) ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	record, ok := g.records[id]
	if !ok {
		return nil, fmt.Errorf("unknown preflight")
	}
	raw, err := json.Marshal(record.Preflight)
	if len(raw) > 1<<20 {
		return nil, fmt.Errorf("preflight export exceeds bounds")
	}
	return raw, err
}

func safeGuardModel(model string) string {
	for _, base := range []string{"gpt-6-luna", "gpt-6.1-sol", "jev-1.13.0"} {
		if model == base {
			return model
		}
		if suffix, ok := strings.CutPrefix(model, base+"-"); ok {
			if _, err := time.Parse("2006-01-02", suffix); err == nil {
				return model
			}
		}
	}
	return ""
}
