package evaluationlab_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"professional-information-repo/internal/semanticeval"
	"testing"
)

func cvReceipt(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../docs/evaluations/cv-lab/evidence.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCVOwnerContractDisagreementCannotRetainAPassingVerdict(t *testing.T) {
	raw := mutateCVReceipt(t, func(d map[string]any) {
		reportBytes, err := base64.StdEncoding.DecodeString(d["ownerReportBytes"].(string))
		if err != nil {
			t.Fatal(err)
		}
		var report map[string]any
		_ = json.Unmarshal(reportBytes, &report)
		report["runs"].([]any)[3].(map[string]any)["status"] = float64(503)
		changed, _ := json.Marshal(report)
		d["ownerReportBytes"] = base64.StdEncoding.EncodeToString(changed)
		d["ownerReportHash"] = semanticeval.Hash(changed)
	})
	rows, err := lab.DeriveCVEvidence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rows[3].AssertionVerdict != "fail" {
		t.Fatalf("owner expected 200; changed 503 retained verdict %s", rows[3].AssertionVerdict)
	}
}
func TestCVOwnerReceiptMapsExactJoinsAndOriginalFailures(t *testing.T) {
	raw := cvReceipt(t)
	rows, err := lab.DeriveCVEvidence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 24 {
		t.Fatalf("got %d rows", len(rows))
	}
	failures := 0
	for _, row := range rows {
		if row.Kind == "workflow-result" && row.AssertionVerdict == "fail" {
			failures++
		}
		if len(row.Source) == 0 || len(row.Expected) == 0 || len(row.Actual) == 0 {
			t.Fatalf("lost evidence: %s", row.ID)
		}
	}
	if failures != 8 {
		t.Fatalf("owner observed failures lost: %d", failures)
	}
	imported, err := lab.Import(raw)
	if err != nil || imported.Quarantined || !bytes.Equal(imported.Original, raw) {
		t.Fatalf("import: %v %+v", err, imported.Quarantined)
	}
	if len(lab.DispatchLedger(imported)) != 26 {
		t.Fatalf("retained dispatches counted twice or missing: %d", len(lab.DispatchLedger(imported)))
	}
}
func TestCVRetainedReplayUsesNoDispatchesAndKeepsSemanticFailures(t *testing.T) {
	raw := cvReceipt(t)
	planner, err := lab.NewPlanner(map[string][]byte{"cv-integrity-evidence": raw})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.Plan(lab.Selection{Suite: "cv-integrity-evidence", Mode: "offline", Configuration: "owner-retained-v1"}, lab.ExecutionCaps{MaxCases: 30, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 4 << 20, DeadlineMS: 10000})
	if err != nil {
		t.Fatal(err)
	}
	store, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "private"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := lab.NewRunner(planner, lab.RunnerOptions{Retain: store.Save, BeforeDispatch: store.Health})
	id, err := runner.Start(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	record, err := runner.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if record.EngineState != "completed" || record.AssertionVerdict != "fail" || len(record.Dispatches) != 0 || record.Progress.AttemptedHTTP != 0 {
		t.Fatalf("replay: %+v", record)
	}
	saved, err := store.Load(id)
	if err != nil || len(saved.Cases) != 24 || !saved.Durable {
		t.Fatalf("retention: %v", err)
	}
	original, err := store.ExportJSON(id)
	if err != nil || !json.Valid(original) {
		t.Fatalf("export: %v", err)
	}
}

func TestCVM15SeparatesScriptedAcceptanceFromPendingAdjudication(t *testing.T) {
	imported, err := lab.Import(cvReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	numerator, denominator, assertions, missing := 0, 0, 0, 0
	for _, m := range lab.Metrics(imported) {
		switch m.Measure {
		case "scripted-unsupported-active-machine-acceptance":
			numerator += *m.Numerator
			denominator += *m.Denominator
		case "adjudicated-unsupported-active-machine-acceptance":
			if m.Value != nil || m.Denominator == nil || *m.Denominator != 0 {
				t.Fatal("scripted labels became adjudicated")
			}
			missing += m.Missing
		case "explicit-user-assertion-cohort":
			assertions += int(*m.Value)
		}
	}
	if numerator != 0 || denominator != 6 || missing != 6 || assertions != 8 {
		t.Fatalf("counts n=%d d=%d pending=%d user=%d", numerator, denominator, missing, assertions)
	}
	imported.Cases = nil // Mutable derived rows cannot change trusted metrics.
	if len(lab.Metrics(imported)) == 0 {
		t.Fatal("metrics lost")
	}
}

func mutateCVReceipt(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(cvReceipt(t), &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestCVDerivationRejectsMismatchedAndDuplicateObservationIdentities(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(d map[string]any) { rows := d["observations"].([]any); d["observations"] = append(rows, rows[0]) },
		func(d map[string]any) {
			d["observations"].([]any)[0].(map[string]any)["units"].([]any)[0].(map[string]any)["text"] = "Changed wording"
		},
		func(d map[string]any) {
			d["observations"].([]any)[0].(map[string]any)["request"].(map[string]any)["facts"].([]any)[0].(map[string]any)["text"] = "Kubernetes"
		},
		func(d map[string]any) {
			d["observations"].([]any)[0].(map[string]any)["request"].(map[string]any)["cvLanguage"] = "pt-BR"
		},
		func(d map[string]any) { d["ownerReportHash"] = "changed" },
	} {
		raw := mutateCVReceipt(t, change)
		if _, err := lab.DeriveCVEvidence(raw); err == nil {
			t.Fatal("invalid join accepted")
		}
		imported, err := lab.Import(raw)
		if err != nil || !imported.Quarantined {
			t.Fatal("changed receipt acquired trusted import status")
		}
	}
}
func TestCVDerivationRetainsMissingAcceptanceWithoutInventingARejection(t *testing.T) {
	raw := mutateCVReceipt(t, func(d map[string]any) { delete(d["observations"].([]any)[0].(map[string]any), "acceptedAsIs") })
	rows, err := lab.DeriveCVEvidence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 24 {
		t.Fatal("missing acceptance dropped case")
	}
	metrics, err := lab.DeriveCVEvidenceMetrics(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metrics {
		if m.CaseID == "en-support-kubernetes-unsupported" && m.Measure == "scripted-unsupported-active-machine-acceptance" {
			if m.Value != nil || m.Missing != 1 || m.Evaluated != 0 || *m.Denominator != 1 {
				t.Fatalf("missing acceptance scored as rejection: %+v", m)
			}
		}
	}
	// Public offline runner can only execute exact approved bytes, not this mutation.
	if _, err := lab.NewPlanner(map[string][]byte{"cv-integrity-evidence": raw}); err == nil {
		t.Fatal("unapproved receipt executable")
	}
}

func TestCVUnknownOwnerAndProfileCannotJoinByFactIDAlone(t *testing.T) {
	raw := mutateCVReceipt(t, func(d map[string]any) {
		d["observations"].([]any)[0].(map[string]any)["request"].(map[string]any)["facts"].([]any)[0].(map[string]any)["reference"].(map[string]any)["profileId"] = "another-profile"
	})
	if _, err := lab.DeriveCVEvidence(raw); err == nil {
		t.Fatal("foreign profile reference joined by ID alone")
	}
}

func TestCVMetricsExposeUnsafeCaseAcceptanceAndKeepMissingness(t *testing.T) {
	raw := mutateCVReceipt(t, func(d map[string]any) { d["observations"].([]any)[0].(map[string]any)["acceptedAsIs"] = true })
	metrics, err := lab.DeriveCVEvidenceMetrics(raw)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range metrics {
		if m.CaseID == "en-support-kubernetes-unsupported" && m.Measure == "scripted-unsafe-case-acceptance" {
			found = true
			if m.Value == nil || *m.Value != 1 || *m.Numerator != 1 || *m.Denominator != 1 {
				t.Fatalf("unsafe case lost: %+v", m)
			}
		}
	}
	if !found {
		t.Fatal("unsafe-case acceptance unavailable")
	}
}

func TestCVWholeMissingObservationRetainsAllPendingOpportunities(t *testing.T) {
	raw := mutateCVReceipt(t, func(d map[string]any) { d["observations"] = d["observations"].([]any)[1:] })
	metrics, err := lab.DeriveCVEvidenceMetrics(raw)
	if err != nil {
		t.Fatal(err)
	}
	foundUnsafe, foundPending := false, false
	for _, m := range metrics {
		if m.CaseID != "en-support-kubernetes-unsupported" {
			continue
		}
		switch m.Measure {
		case "scripted-unsafe-case-acceptance":
			foundUnsafe = true
			if m.Value != nil || m.Missing != 1 || m.Eligible != 1 || m.Evaluated != 0 {
				t.Fatalf("missing unsafe case erased: %+v", m)
			}
		case "pending-scripted-unsupported-statements":
			foundPending = true
			if m.Value == nil || *m.Value != 1 {
				t.Fatalf("pending label erased: %+v", m)
			}
		}
	}
	if !foundUnsafe || !foundPending {
		t.Fatal("missing measures")
	}
}

func TestCVSnapshotAndReloadMustBelongToExactRequestAndText(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(d map[string]any) {
			d["observations"].([]any)[0].(map[string]any)["snapshot"].(map[string]any)["sourceProfileId"] = "foreign-profile"
		},
		func(d map[string]any) {
			d["observations"].([]any)[0].(map[string]any)["snapshot"] = d["observations"].([]any)[3].(map[string]any)["snapshot"]
		},
		func(d map[string]any) {
			d["observations"].([]any)[0].(map[string]any)["reloaded"].(map[string]any)["summary"] = "Changed reload wording"
		},
		func(d map[string]any) {
			d["observations"].([]any)[0].(map[string]any)["snapshot"].(map[string]any)["provenance"].([]any)[0].(map[string]any)["original"].(map[string]any)["text"] = "Foreign original text"
		},
	} {
		raw := mutateCVReceipt(t, change)
		if _, err := lab.DeriveCVEvidence(raw); err == nil {
			t.Fatal("foreign saved/reloaded identity joined")
		}
	}
}

func TestCVCanceledReplayCannotInventBlockedGenerationObservations(t *testing.T) {
	planner, err := lab.NewPlanner(map[string][]byte{"cv-integrity-evidence": cvReceipt(t)})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.Plan(lab.Selection{Suite: "cv-integrity-evidence", Mode: "offline", Configuration: "owner-retained-v1", CaseIDs: []string{"en-support-kubernetes-unsupported"}}, lab.ExecutionCaps{MaxCases: 1, MaxHTTPAttempts: 1, MaxProviderCalls: 1, MaxArtifactBytes: 1 << 20, DeadlineMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	runner := lab.NewRunner(planner, lab.RunnerOptions{})
	id, err := runner.Start(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	record, err := runner.Wait(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if record.EngineState != "canceled" {
		t.Fatal(record.EngineState)
	}
	for _, metric := range record.Metrics {
		if metric.Measure == "retained-blocked-generation" && metric.Value != nil {
			t.Fatal("unreached retained result became observed generation failure")
		}
	}
}

func TestCVMissingSnapshotAndReloadStayUnavailable(t *testing.T) {
	raw := mutateCVReceipt(t, func(d map[string]any) {
		row := d["observations"].([]any)[0].(map[string]any)
		delete(row, "snapshot")
		delete(row, "reloaded")
	})
	rows, err := lab.DeriveCVEvidence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows[0].UnavailableReasons) < 2 {
		t.Fatal("missing stages not retained")
	}
	metrics, err := lab.DeriveCVEvidenceMetrics(raw)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range metrics {
		if m.CaseID == rows[0].ID && m.Measure == "explicit-user-assertion-cohort" {
			found = true
			if m.Value != nil || m.Missing != 1 {
				t.Fatal("missing snapshot became zero user assertions")
			}
		}
	}
	if !found {
		t.Fatal("missing provenance metric")
	}
}
