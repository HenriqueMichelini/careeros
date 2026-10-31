package aicontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
)

func TestA27_ProviderStageAndClarificationShareOneFiniteLimit(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.MaxStages = 2; c.Limit = 1000; c.Capacity = 10 })
	s, e := f.h.Open(context.Background(), f.p, "combined_limit", "profile_ingestion", []byte(`{"input":"synthetic","profile":{}}`))
	if e != nil {
		t.Fatal(e)
	}
	// Seed the durable no-send child allocation to isolate the shared SQL limit.
	// Full protocol creation/replay is exercised by the ingestion integration test.
	if _, e = f.db.Exec(`INSERT INTO ai_fixture_child(namespace,action,id,parent_coordinate,parent_revision,attempt,binding) VALUES($1,$2,'issued_child','ingest/v2/clarify/claim/c1','',1,$3)`, f.cfg.Namespace, s.ID(), s.root); e != nil {
		t.Fatal(e)
	}
	body := []byte(`{"model":"jev-1.13.0","state":{"field":"professional_information","submission":"synthetic"},"questions":{}}`)
	leg := Leg{Provider: "typesafe", Coordinate: "ingest/legacy/original", Stage: "field_validation", Number: 1, Body: body}
	d, e := s.Reserve(context.Background(), leg)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(context.Background(), leg); e != nil {
		t.Fatal("duplicate consumed allocation", e)
	}
	leg.Coordinate = "ingest/legacy/clarification/c1/original"
	if _, e = s.Reserve(context.Background(), leg); !errors.Is(e, ErrCapacity) {
		t.Fatal("child omitted from stage ceiling", e)
	}
	var stages, children, dispatches int
	if e = f.db.QueryRow(`SELECT (SELECT count(*) FROM ai_fixture_stage),(SELECT count(*) FROM ai_fixture_child),(SELECT count(*) FROM ai_fixture_dispatch)`).Scan(&stages, &children, &dispatches); e != nil {
		t.Fatal(e)
	}
	if stages != 1 || children != 1 || dispatches != 1 || d.Counted != 60 || f.scope(t).Counted != 60 || f.calls.Load() != 0 {
		t.Fatal(stages, children, dispatches, d)
	}
}

func TestA17_ConservativeClosureAuditIsDurableAndImmutable(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "audited_closure")
	d := f.sent(t, s)
	f.clock.set(d.DurableBound.Add(168*time.Hour), true)
	r := f.h.SyntheticReviewer()
	if e := r.ReviewConservativeClosure(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
	var first []byte
	if e := f.db.QueryRow(`SELECT data FROM ai_fixture_evidence WHERE kind='conservative_closure'`).Scan(&first); e != nil {
		t.Fatal(e)
	}
	var audit struct {
		Reviewer, Disposition, PolicyVersion string
		Amount                               int64
		ReviewedAt, IntentAt, DurableBound   time.Time
	}
	if json.Unmarshal(first, &audit) != nil || audit.Reviewer != f.p.AccountID || audit.Disposition != "unresolved_conservative_maximum" || audit.PolicyVersion != "synthetic-seven-day-review-v1" || audit.Amount != 60 || !audit.IntentAt.Equal(*d.IntentAt) || !audit.DurableBound.Equal(*d.DurableBound) {
		t.Fatalf("closure audit %+v", audit)
	}
	f.clock.set(d.DurableBound.Add(200*time.Hour), true)
	// Reconstructing the review capability cannot renew audit time or identity.
	if e := f.h.SyntheticReviewer().ReviewConservativeClosure(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
	if e := r.Compact(context.Background(), f.p, d.ID); e != nil {
		t.Fatal(e)
	}
	var count int
	var after []byte
	if e := f.db.QueryRow(`SELECT count(*) FROM ai_fixture_evidence WHERE kind='conservative_closure'`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	if e := f.db.QueryRow(`SELECT data FROM ai_fixture_evidence WHERE kind='conservative_closure'`).Scan(&after); e != nil || !bytes.Equal(first, after) {
		t.Fatal("audit changed", e)
	}
	for _, sql := range []string{`UPDATE ai_fixture_evidence SET data='{}' WHERE kind='conservative_closure'`, `DELETE FROM ai_fixture_evidence WHERE kind='conservative_closure'`} {
		if _, e := f.h.reviewerDB.Exec(sql); e == nil {
			t.Fatal("review role mutated immutable audit")
		}
	}
	state := f.status(t, s).Dispatches[0]
	if state.Counted != 60 || state.Expense != nil || state.Category != "reviewed_conservative_maximum" || f.scope(t).Occupied != 1 {
		t.Fatal("audit/compaction altered rights", state)
	}
	if err := r.RetireNamespace(context.Background(), f.p); err != nil {
		t.Fatal(err)
	}
	proof := SyntheticRetirementProof{Namespace: f.cfg.Namespace, Epoch: 1, CapabilitiesExpired: true, RestoreHorizonExpired: true, SendersExcluded: true, AllEntrypointsClosed: true}
	if err := r.PruneNamespaceMarkers(context.Background(), f.p, proof); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT data FROM ai_fixture_evidence WHERE kind='conservative_closure'`).Scan(&after); err != nil || !bytes.Equal(first, after) {
		t.Fatal("namespace pruning removed or changed the closure audit", err)
	}
	var counted, occupied int64
	if err := f.db.QueryRow(`SELECT counted,occupied FROM ai_fixture_scope WHERE namespace=$1 AND scope='global'`, f.cfg.Namespace).Scan(&counted, &occupied); err != nil || counted != 60 || occupied != 1 {
		t.Fatal("namespace pruning released a retained obligation", counted, occupied, err)
	}
}

func TestA03_FrozenExecutionPolicyRejectsChangedReplay(t *testing.T) {
	for _, kind := range []string{"execution_version", "rate_version", "dispatch_limit", "stage_limit", "attempt_limit", "input_limit", "output_limit", "customer_units", "scope_budget", "scope_capacity", "expiry"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t, nil)
			s := f.open(t, "frozen")
			switch kind {
			case "execution_version":
				f.h.cfg.ExecutionPolicyVersion = "synthetic-execution-v2"
			case "rate_version":
				f.h.cfg.RatePolicyVersion = "synthetic-unit-v2"
			default:
				query := map[string]string{"dispatch_limit": `UPDATE ai_fixture_policy SET max_dispatches=max_dispatches+1`, "stage_limit": `UPDATE ai_fixture_policy SET max_stages=max_stages+1`, "attempt_limit": `UPDATE ai_fixture_policy SET max_attempts=max_attempts+1`, "input_limit": `UPDATE ai_fixture_policy SET max_input_bytes=max_input_bytes+1`, "output_limit": `UPDATE ai_fixture_policy SET max_output_tokens=max_output_tokens+1`, "customer_units": `UPDATE ai_fixture_policy SET units=units+1`, "scope_budget": `UPDATE ai_fixture_scope SET limit_amount=limit_amount+1 WHERE scope='global'`, "scope_capacity": `UPDATE ai_fixture_scope SET capacity=capacity+1 WHERE scope='global'`, "expiry": `UPDATE ai_fixture_policy SET expires_at=expires_at+interval '1 second'`}[kind]
				if _, e := f.db.Exec(query); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := f.h.Open(context.Background(), f.p, "frozen", s.workflow, s.rootInput); e == nil {
				t.Fatal("recovery reinterpreted frozen policy")
			}
			if _, e := s.Reserve(context.Background(), Leg{Provider: "openai", Coordinate: "request", Stage: "generation", Number: 1, Body: fixtureBody}); e == nil {
				t.Fatal("direct reserve ignored changed policy")
			}
			if r, e := send(s); e == nil {
				closeResp(r)
				t.Fatal("changed policy sent")
			}
			if e := f.h.SyntheticReviewer().MarkFailed(context.Background(), f.p, s.ID()); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Retry(context.Background()); e == nil {
				t.Fatal("retry reinterpreted frozen policy")
			}
			var count int
			if e := f.db.QueryRow(`SELECT count(*) FROM ai_fixture_dispatch`).Scan(&count); e != nil || count != 0 || f.calls.Load() != 0 {
				t.Fatal("drift partially allocated", count, e)
			}
		})
	}
}

func TestA03_ActualStagePromptSchemaModelAndTierMustMatchIdentity(t *testing.T) {
	for _, kind := range []string{"prompt", "schema", "model", "tier"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t, nil)
			s := f.open(t, "stage_identity")
			version := "cv-policy-v2/schema-v2"
			body := fixtureBody
			switch kind {
			case "prompt":
				version = "cv-policy-v3/schema-v2"
			case "schema":
				version = "cv-policy-v2/schema-v3"
			case "model":
				body = []byte(`{"model":"other","max_completion_tokens":4000}`)
			case "tier":
				body = []byte(`{"model":"gpt-6-luna","service_tier":"priority","max_completion_tokens":4000}`)
			}
			ctx, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(context.Background(), s.workflow, s.rootInput), "generation", version)
			request, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
			if r, e := s.Send(ctx, "openai", request); e == nil {
				closeResp(r)
				t.Fatal("request drift admitted")
			}
			if f.calls.Load() != 0 || f.scope(t).Counted != 0 {
				t.Fatal("drift acquired exposure")
			}
		})
	}
}

func TestA03_DelayedHandoffCannotUseChangedExecutionPolicy(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "delayed_policy")
	f.h.SetFault("before_invoke", func() error { f.h.cfg.ExecutionPolicyVersion = "synthetic-execution-v2"; return nil })
	if r, e := send(s); e == nil {
		closeResp(r)
		t.Fatal("delayed handoff reinterpreted policy")
	}
	if f.calls.Load() != 0 || f.scope(t).Counted != 60 || f.scope(t).Occupied != 1 {
		t.Fatal("denial lost intent")
	}
}
