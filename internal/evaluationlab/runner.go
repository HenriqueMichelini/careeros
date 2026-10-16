package evaluationlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/semanticeval"
	"sync"
	"time"
)

var ErrRunnerBusy = errors.New("an evaluation run is already active")
var errRunCapacity = errors.New("evaluation capacity exhausted")

type RunnerOptions struct {
	BeforeDispatch func(context.Context) error
	Retain         func(RunRecord) error
	ControlledBody func(context.Context, io.ReadCloser) io.ReadCloser
	MaxEvents      int
}
type RunEvent struct {
	Sequence         uint64 `json:"sequence"`
	Kind             string `json:"kind"`
	CaseIdentity     string `json:"caseIdentity,omitempty"`
	DispatchIdentity string `json:"dispatchIdentity,omitempty"`
	Stage            string `json:"stage,omitempty"`
	Outcome          string `json:"outcome,omitempty"`
}
type EventPage struct {
	Events   []RunEvent `json:"events"`
	Earliest uint64     `json:"earliest"`
	Next     uint64     `json:"next"`
	Gap      bool       `json:"gap"`
}
type runState struct {
	record          RunRecord
	plan            Plan
	cancel          context.CancelFunc
	done            chan struct{}
	events          []RunEvent
	sequence        uint64
	caseIndex       int
	leafIdentity    string
	pendingDispatch string
	storageErr      error
	ctx             context.Context
	artifactBytes   int64
	resourceErr     error
}
type Runner struct {
	mu      sync.Mutex
	planner *Planner
	options RunnerOptions
	runs    map[string]*runState
	active  string
}

func NewRunner(planner *Planner, options RunnerOptions) *Runner {
	if options.MaxEvents < 1 || options.MaxEvents > 4096 {
		options.MaxEvents = 512
	}
	return &Runner{planner: planner, options: options, runs: map[string]*runState{}}
}

// Start admits a verified plan atomically and returns before execution finishes.
// This runner has no live mode or credential/file discovery path.
func (r *Runner) Start(parent context.Context, plan Plan) (string, error) {
	if r.planner == nil {
		return "", fmt.Errorf("planner unavailable")
	}
	plan = clonePlan(plan)
	if err := r.planner.ValidatePlan(plan); err != nil {
		return "", err
	}
	cases, err := r.plannedCases(plan)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active != "" {
		return "", ErrRunnerBusy
	}
	if len(r.runs) >= 128 {
		return "", fmt.Errorf("in-memory run limit reached")
	}
	id := newLabID()
	ctx, cancel := context.WithTimeout(parent, time.Duration(plan.Caps.DeadlineMS)*time.Millisecond)
	record := RunRecord{Version: ResultVersion, ID: id, EngineState: "planned", AssertionVerdict: "unavailable", Selection: plan.Selection, Provenance: cloneJSON(plan.Provenance), Cases: cases, Dispatches: []Dispatch{}, OperationalOutcomes: map[string]int{}, UnavailableReasons: []string{"pending local evidence; durable persistence is not provided by the memory runner"}, Context: ExecutionContext{Mode: plan.Selection.Mode, RuntimeVersion: plan.RuntimeVersion, Concurrency: 1, Order: append([]string{}, plan.CaseOrder...), MaxLogicalCases: plan.Caps.MaxCases, MaxHTTPAttempts: plan.Caps.MaxHTTPAttempts, MaxProviderCalls: plan.Caps.MaxProviderCalls, MaxArtifactBytes: plan.Caps.MaxArtifactBytes, DeadlineMS: plan.Caps.DeadlineMS, ConfigurationVersion: plan.SuiteVersion, SchemaVersion: ResultVersion, ScorerVersion: plan.Provenance.SourceHashes["internal/semanticeval/score.go"], UnavailableReasons: []string{"source/prompt/label completeness depends on supplied launch provenance"}}, Progress: RunProgress{PlannedCases: len(cases), PlannedHTTP: plan.RequiredHTTPAttempts}}
	record.Context.LabelHash = versionedHash("lab-label-fixture-set-v1", plan.InputHashes)
	if len(plan.Provenance.SourceHashes) > 0 {
		// Conservative source-envelope versions: equal means the captured source
		// collection is identical; a mismatch never isolates a prompt/strategy edit.
		record.Context.PromptVersion = versionedHash("lab-prompt-source-envelope-v1", plan.Provenance.SourceHashes)
		record.Context.StrategyVersion = versionedHash("lab-strategy-source-envelope-v1", plan.Provenance.SourceHashes)
	}
	state := &runState{record: record, plan: plan, cancel: cancel, done: make(chan struct{}), ctx: ctx}
	r.runs[id] = state
	r.active = id
	r.eventLocked(state, RunEvent{Kind: "run_admitted"})
	go r.execute(ctx, state)
	return id, nil
}

func (r *Runner) Read(id string) (RunRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.runs[id]
	if !ok {
		return RunRecord{}, fmt.Errorf("unknown run")
	}
	return cloneJSON(state.record), nil
}
func (r *Runner) Wait(ctx context.Context, id string) (RunRecord, error) {
	r.mu.Lock()
	state, ok := r.runs[id]
	r.mu.Unlock()
	if !ok {
		return RunRecord{}, fmt.Errorf("unknown run")
	}
	select {
	case <-ctx.Done():
		return RunRecord{}, ctx.Err()
	case <-state.done:
		return r.Read(id)
	}
}
func (r *Runner) Cancel(id string) error {
	r.mu.Lock()
	state, ok := r.runs[id]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("unknown run")
	}
	cancel := state.cancel
	r.mu.Unlock()
	cancel()
	return nil
}
func (r *Runner) Events(id string, after uint64, limit int) (EventPage, error) {
	if limit < 1 || limit > 256 {
		return EventPage{}, fmt.Errorf("event page limit outside bounds")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.runs[id]
	if !ok {
		return EventPage{}, fmt.Errorf("unknown run")
	}
	page := EventPage{Events: []RunEvent{}, Next: after}
	if after > state.sequence {
		return EventPage{}, fmt.Errorf("event cursor exceeds observed sequence")
	}
	if len(state.events) > 0 {
		page.Earliest = state.events[0].Sequence
		page.Gap = after+1 < page.Earliest
	}
	for _, event := range state.events {
		if event.Sequence > after && len(page.Events) < limit {
			page.Events = append(page.Events, event)
			page.Next = event.Sequence
		}
	}
	return page, nil
}

func (r *Runner) eventLocked(state *runState, event RunEvent) {
	state.sequence++
	event.Sequence = state.sequence
	state.events = append(state.events, event)
	if len(state.events) > r.options.MaxEvents {
		state.events = append([]RunEvent{}, state.events[len(state.events)-r.options.MaxEvents:]...)
	}
}
func (r *Runner) retain(state *runState) error {
	r.mu.Lock()
	if state.storageErr != nil || r.options.Retain == nil {
		err := state.storageErr
		r.mu.Unlock()
		return err
	}
	snapshot := cloneJSON(state.record)
	r.mu.Unlock()
	return r.retainSnapshot(state, snapshot)
}
func (r *Runner) retainSnapshot(state *runState, snapshot RunRecord) error {
	r.mu.Lock()
	priorFailure := state.storageErr
	r.mu.Unlock()
	if priorFailure != nil {
		return priorFailure
	}
	if r.options.Retain == nil {
		return nil
	}
	if err := r.options.Retain(snapshot); err != nil {
		r.mu.Lock()
		state.storageErr = err
		r.mu.Unlock()
		return err
	}
	return nil
}
func (r *Runner) health(ctx context.Context, state *runState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	storageErr := state.storageErr
	resourceErr := state.resourceErr
	full := state.artifactBytes >= state.plan.Caps.MaxArtifactBytes
	r.mu.Unlock()
	if storageErr != nil {
		return storageErr
	}
	if resourceErr != nil {
		return resourceErr
	}
	if full {
		return errRunCapacity
	}
	if r.options.BeforeDispatch != nil {
		if err := r.options.BeforeDispatch(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			r.mu.Lock()
			state.storageErr = err
			r.mu.Unlock()
			return err
		}
	}
	return ctx.Err()
}

func (r *Runner) policy(state *runState) semanticeval.ExecutionPolicy {
	return semanticeval.ExecutionPolicy{
		BeforeWorkflow: func(ctx context.Context, c semanticeval.Case) error {
			if err := r.health(ctx, state); err != nil {
				return err
			}
			r.mu.Lock()
			defer r.mu.Unlock()
			if state.record.Progress.AttemptedHTTP >= state.plan.Caps.MaxHTTPAttempts {
				return errRunCapacity
			}
			root := &state.record.Cases[state.caseIndex]
			leaf := root
			for i := range root.Children {
				if root.Children[i].ID == c.ID {
					leaf = &root.Children[i]
					break
				}
			}
			state.leafIdentity = leaf.LeafIdentity
			leaf.EngineState = "running"
			root.AttemptedSteps++
			if leaf != root {
				leaf.AttemptedSteps++
			}
			state.record.Progress.AttemptedHTTP++
			r.eventLocked(state, RunEvent{Kind: "workflow_started", CaseIdentity: state.leafIdentity})
			return nil
		},
		ObserveWorkflow: func(c semanticeval.Case, result semanticeval.RunResult) {
			r.mu.Lock()
			defer r.mu.Unlock()
			root := &state.record.Cases[state.caseIndex]
			leaf := root
			for i := range root.Children {
				if root.Children[i].ID == c.ID {
					leaf = &root.Children[i]
					break
				}
			}
			if len(root.Children) > 0 {
				actual, _ := json.Marshal(result)
				leaf.ArtifactHash = semanticeval.Hash(actual)
				if state.artifactBytes+int64(len(actual)) > state.plan.Caps.MaxArtifactBytes {
					state.resourceErr = errRunCapacity
					leaf.UnavailableReasons = append(leaf.UnavailableReasons, "leaf artifact exceeds configured cap; only its hash retained")
				} else {
					leaf.Actual = actual
					state.artifactBytes += int64(len(actual))
				}
			}
			leaf.EngineState = "completed"
			leaf.OperationalOutcome = "unknown"
			if result.Diagnostics != nil {
				leaf.OperationalOutcome = aidiagnostics.OperationalOutcome(result.Diagnostics.Outcome)
				for _, stage := range result.Diagnostics.Stages {
					r.eventLocked(state, RunEvent{Kind: "stage_finished", CaseIdentity: leaf.LeafIdentity, Stage: stage.Name, Outcome: stage.Outcome})
				}
			}
			if result.Status == 0 {
				leaf.EngineState = "failed"
			}
			if state.ctx.Err() != nil {
				leaf.EngineState = "failed"
				if errors.Is(state.ctx.Err(), context.Canceled) {
					leaf.EngineState = "canceled"
				}
				leaf.AssertionVerdict = "blocked"
			} else if len(root.Children) > 0 {
				interpretRetainedAssertion(leaf)
			}
			state.record.Progress.FinishedHTTP++
			r.eventLocked(state, RunEvent{Kind: "workflow_finished", CaseIdentity: leaf.LeafIdentity, Outcome: leaf.OperationalOutcome})
		},
		BeforeDispatch: func(ctx context.Context) error {
			if err := r.health(ctx, state); err != nil {
				return err
			}
			r.mu.Lock()
			if state.record.Progress.AttemptedDispatches >= state.plan.Caps.MaxProviderCalls {
				r.mu.Unlock()
				return errRunCapacity
			}
			state.pendingDispatch = newLabID()
			root := state.record.Cases[state.caseIndex]
			state.record.Dispatches = append(state.record.Dispatches, Dispatch{Identity: state.pendingDispatch, CaseIdentity: state.leafIdentity, LogicalCaseIdentity: root.Identity, Origin: "simulated", State: "planned"})
			r.mu.Unlock()
			if err := r.retain(state); err != nil {
				return err
			}
			return ctx.Err()
		},
		NewDispatchID: func() string { r.mu.Lock(); defer r.mu.Unlock(); return state.pendingDispatch },
		ObserveDispatch: func(observation semanticeval.DispatchObservation) {
			r.mu.Lock()
			for i := range state.record.Dispatches {
				d := &state.record.Dispatches[i]
				if d.Identity != observation.Call.DispatchID {
					continue
				}
				d.Call = observation.Call
				if observation.Phase == semanticeval.DispatchStarted {
					d.State = "attempted"
					state.record.Progress.AttemptedDispatches++
				} else {
					d.State = "completed"
					if observation.Err != nil {
						d.State = "unknown-final"
						d.UnavailableReasons = append(d.UnavailableReasons, "dispatch ended without a fully consumed response")
					}
				}
				r.eventLocked(state, RunEvent{Kind: "dispatch_" + string(observation.Phase), CaseIdentity: d.CaseIdentity, DispatchIdentity: d.Identity})
				break
			}
			r.mu.Unlock()
			if observation.Phase == semanticeval.DispatchFinished {
				_ = r.retain(state)
			}
		}, ControlledBody: r.options.ControlledBody,
	}
}

func (r *Runner) execute(parent context.Context, state *runState) {
	defer state.cancel()
	ctx := semanticeval.WithExecutionPolicy(parent, r.policy(state))
	r.mu.Lock()
	state.record.EngineState = "running"
	r.eventLocked(state, RunEvent{Kind: "run_started"})
	r.mu.Unlock()
	err := r.retain(state)
	budget := &semanticeval.Budget{Limit: state.plan.Caps.MaxProviderCalls, WorkflowLimit: state.plan.Caps.MaxHTTPAttempts}
	for index, id := range state.plan.CaseOrder {
		if err != nil {
			break
		}
		if err = r.health(ctx, state); err != nil {
			break
		}
		r.mu.Lock()
		state.caseIndex = index
		state.record.Cases[index].EngineState = "running"
		state.record.Progress.StartedCases++
		r.eventLocked(state, RunEvent{Kind: "case_started", CaseIdentity: state.record.Cases[index].Identity})
		r.mu.Unlock()
		var actual json.RawMessage
		switch state.plan.Selection.Suite {
		case "semantic-quality", "workflow-settings", "section-review":
			c, lookupErr := r.semanticCase(state.plan.Selection.Suite, id)
			if lookupErr != nil {
				err = lookupErr
				break
			}
			err = semanticeval.ExecuteBatch(ctx, semanticeval.Batch{Cases: []semanticeval.Case{c}, Mode: "controlled", Configurations: []string{state.plan.Selection.Configuration}, Budget: budget}, func(_ semanticeval.Case, _ string, result semanticeval.RunResult, evaluationErr error) error {
				actual, _ = json.Marshal(result)
				return evaluationErr
			})
		case "qualification-evidence":
			var report QualificationReport
			report, err = RunQualificationEvidence(ctx, r.planner.assets["qualification-evidence"], []string{id})
			actual, _ = json.Marshal(report)
		case "application-context":
			var report ApplicationContextReport
			report, err = RunApplicationContext(ctx, r.planner.assets["application-context"], r.planner.assets["semantic-quality"], r.planner.assets["application-baseline"], []string{id})
			actual, _ = json.Marshal(report)
		default:
			err = fmt.Errorf("unsupported execution suite")
		}
		r.mu.Lock()
		root := &state.record.Cases[index]
		if state.resourceErr != nil {
			err = state.resourceErr
		}
		root.ArtifactHash = semanticeval.Hash(actual)
		if state.artifactBytes+int64(len(actual)) > state.plan.Caps.MaxArtifactBytes {
			err = errRunCapacity
			root.UnavailableReasons = append(root.UnavailableReasons, "artifact exceeds configured cap; only its hash retained")
		} else {
			root.Actual = actual
			state.artifactBytes += int64(len(actual))
		}
		root.EngineState = "completed"
		root.AssertionVerdict = "unavailable"
		if err != nil {
			root.EngineState = "failed"
			if errors.Is(ctx.Err(), context.Canceled) {
				root.EngineState = "canceled"
			}
		} else {
			r.interpretCase(root, state.plan.Selection.Suite)
		}
		root.SkippedSteps = max(0, root.PlannedSteps-root.AttemptedSteps)
		state.record.Progress.FinishedCases++
		r.eventLocked(state, RunEvent{Kind: "case_finished", CaseIdentity: root.Identity, Outcome: root.OperationalOutcome})
		r.mu.Unlock()
		if retentionErr := r.retain(state); retentionErr != nil {
			err = retentionErr
		}
	}
	r.mu.Lock()
	state.record.EngineState = "completed"
	state.record.AssertionVerdict = "pass"
	if state.storageErr != nil {
		state.record.EngineState = "failed"
		state.record.UnavailableReasons = append(state.record.UnavailableReasons, "storage_failure")
	} else if errors.Is(ctx.Err(), context.Canceled) {
		state.record.EngineState = "canceled"
	} else if err != nil || ctx.Err() != nil {
		state.record.EngineState = "failed"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			state.record.UnavailableReasons = append(state.record.UnavailableReasons, "deadline_exceeded")
		} else if errors.Is(err, errRunCapacity) {
			state.record.UnavailableReasons = append(state.record.UnavailableReasons, "capacity_exhausted")
		}
	}
	if state.record.EngineState != "completed" {
		state.record.AssertionVerdict = "blocked"
	}
	stopReason := "prior_leg_stopped"
	switch {
	case state.storageErr != nil:
		stopReason = "storage_failure"
	case errors.Is(ctx.Err(), context.Canceled):
		stopReason = "canceled"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		stopReason = "deadline_exceeded"
	case errors.Is(err, errRunCapacity):
		stopReason = "capacity_exhausted"
	}
	for i := range state.record.Cases {
		c := &state.record.Cases[i]
		finalizeUnreached(c, stopReason)
		if c.AssertionVerdict == "fail" {
			state.record.AssertionVerdict = "fail"
		}
		state.record.OperationalOutcomes[c.OperationalOutcome]++
	}
	for i := range state.record.Dispatches {
		if state.record.Dispatches[i].State == "planned" {
			state.record.Dispatches[i].State = "not_dispatched"
		}
	}
	state.record.Progress.UnreachedHTTP = max(0, state.record.Progress.PlannedHTTP-state.record.Progress.AttemptedHTTP)
	state.record.Metrics = nativeMetrics(state.record, state.plan)
	terminalRecord := cloneJSON(state.record)
	state.record.EngineState = "running"
	state.record.AssertionVerdict = "unavailable"
	r.eventLocked(state, RunEvent{Kind: "run_retaining"})
	r.mu.Unlock()
	terminalRetentionErr := r.retainSnapshot(state, terminalRecord)
	r.mu.Lock()
	state.record = terminalRecord
	if terminalRetentionErr != nil {
		state.record.EngineState = "failed"
		if state.record.AssertionVerdict != "fail" {
			state.record.AssertionVerdict = "blocked"
		}
		state.record.UnavailableReasons = append(state.record.UnavailableReasons, "terminal_storage_failure")
		r.eventLocked(state, RunEvent{Kind: "run_storage_failed", Outcome: "failed"})
	}
	r.eventLocked(state, RunEvent{Kind: "run_finished", Outcome: state.record.EngineState})
	r.active = ""
	close(state.done)
	r.mu.Unlock()
}

func (r *Runner) semanticCase(suite, id string) (semanticeval.Case, error) {
	var corpus struct {
		Cases []semanticeval.Case `json:"cases"`
	}
	if err := json.Unmarshal(r.planner.assets[suite], &corpus); err != nil {
		return semanticeval.Case{}, err
	}
	for _, c := range corpus.Cases {
		if c.ID == id {
			return c, nil
		}
	}
	return semanticeval.Case{}, fmt.Errorf("case not present in frozen fixture")
}
func (r *Runner) plannedCases(plan Plan) ([]ImportedCase, error) {
	var corpus struct {
		Cases []json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(r.planner.assets[plan.Selection.Suite], &corpus); err != nil {
		return nil, err
	}
	if plan.Selection.Suite == "application-context" {
		var outputs struct {
			Cases []json.RawMessage `json:"cases"`
		}
		_ = json.Unmarshal(r.planner.assets["semantic-quality"], &outputs)
		corpus.Cases = append(corpus.Cases, outputs.Cases...)
	}
	makeCase := func(raw json.RawMessage) (ImportedCase, error) { return plannedCase(raw, plan.Selection.Configuration) }
	result := []ImportedCase{}
	for _, id := range plan.CaseOrder {
		found := false
		for _, raw := range corpus.Cases {
			var identity struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(raw, &identity)
			if identity.ID != id {
				continue
			}
			c, err := makeCase(raw)
			if err != nil {
				return nil, err
			}
			if c.Workflow == "" {
				c.Workflow = "qualification_gaps"
				if plan.Selection.Suite == "application-context" {
					c.Workflow = "application_draft"
				}
			}
			result = append(result, c)
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("missing planned case")
		}
	}
	return result, nil
}
func plannedCase(raw json.RawMessage, configuration string) (ImportedCase, error) {
	var fixture struct {
		ID       string            `json:"id"`
		Task     string            `json:"task"`
		Language string            `json:"language"`
		Steps    []json.RawMessage `json:"steps"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return ImportedCase{}, err
	}
	c := ImportedCase{ID: fixture.ID, Identity: newLabID(), LeafIdentity: newLabID(), Kind: "workflow-result", Workflow: fixture.Task, OriginalLanguage: fixture.Language, Language: fixture.Language, Configuration: configuration, EngineState: "planned", AssertionVerdict: "skipped", OperationalOutcome: "unknown", UnavailableReasons: []string{}}
	if c.Language == "pt" {
		c.Language = "pt-BR"
	}
	for _, step := range fixture.Steps {
		child, err := plannedCase(step, configuration)
		if err != nil {
			return c, err
		}
		child.ParentIdentity = c.Identity
		c.Children = append(c.Children, child)
	}
	c.PlannedSteps = max(1, len(c.Children))
	bindFixtureEvidence(&c, raw)
	return c, nil
}
func (r *Runner) interpretCase(c *ImportedCase, suite string) {
	if suite == "semantic-quality" || suite == "workflow-settings" || suite == "section-review" {
		var run semanticeval.RunResult
		_ = json.Unmarshal(c.Actual, &run)
		interpretRetainedAssertion(c)
		c.OperationalOutcome = semanticeval.OperationalOutcome(run)
		return
	}
	c.AssertionVerdict = "pass"
	if suite == "qualification-evidence" {
		var report QualificationReport
		_ = json.Unmarshal(c.Actual, &report)
		for _, row := range report.Final {
			if !row.Correct {
				c.AssertionVerdict = "fail"
			}
		}
	} else {
		var report struct {
			Fields []struct {
				Current semanticeval.Result `json:"currentControlledReplay"`
			} `json:"fieldResults"`
		}
		_ = json.Unmarshal(c.Actual, &report)
		failures := []string{}
		for _, row := range report.Fields {
			failures = append(failures, row.Current.Failures...)
		}
		observation, _ := json.Marshal(map[string]any{"score": semanticeval.Result{Failures: failures}})
		saved := c.Actual
		c.Actual = observation
		interpretRetainedAssertion(c)
		c.Actual = saved
	}
}
func cloneJSON[T any](value T) T {
	raw, _ := json.Marshal(value)
	var copy T
	_ = json.Unmarshal(raw, &copy)
	return copy
}

// finalizeUnreached records why every planned leg remained unattempted.
func finalizeUnreached(c *ImportedCase, reason string) {
	if c.EngineState == "planned" {
		c.EngineState = "skipped"
		c.AssertionVerdict = "skipped"
		c.SkippedSteps = max(0, c.PlannedSteps-c.AttemptedSteps)
		c.UnavailableReasons = append(c.UnavailableReasons, reason)
	}
	for i := range c.Children {
		finalizeUnreached(&c.Children[i], reason)
	}
}

func versionedHash(version string, value any) string {
	raw, _ := json.Marshal(value)
	return version + ":" + semanticeval.Hash(raw)
}
