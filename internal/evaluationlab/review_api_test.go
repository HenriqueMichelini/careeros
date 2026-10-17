package evaluationlab_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
)

func TestWorksheetAPIExportsPendingEvidenceOnly(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record := controlledHistoryRecord(t, lab.RunnerOptions{Retain: store.Save})
	planner := semanticPlanner(t)
	api, err := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/lab/api/runs/" + record.ID + "/cases/" + record.Cases[0].Identity + "/worksheet"
	out := reviewAPIRequest(api, "POST", path, []byte(`{}`))
	if out.Code != 201 {
		t.Fatalf("worksheet create=%d: %s", out.Code, out.Body.String())
	}
	var worksheet lab.ReviewWorksheet
	if json.Unmarshal(out.Body.Bytes(), &worksheet) != nil || worksheet.State != "pending" {
		t.Fatal("new worksheet acquired approval")
	}
	out = reviewAPIRequest(api, "GET", "/lab/api/runs/"+record.ID+"/worksheets/"+worksheet.ID, nil)
	if out.Code != 200 {
		t.Fatalf("worksheet export=%d", out.Code)
	}
	imported, err := lab.ImportReviewWorksheet(out.Body.Bytes(), worksheet.Binding)
	if err != nil || imported.ContextHash != worksheet.ContextHash {
		t.Fatal("export changed original binding")
	}
	body, _ := json.Marshal(map[string]any{"worksheetBytes": out.Body.Bytes()})
	out = reviewAPIRequest(api, "POST", path+"/import", body)
	if out.Code != 201 {
		t.Fatalf("pending import=%d: %s", out.Code, out.Body.String())
	}
	out = reviewAPIRequest(api, "POST", path, []byte(`{"humanApproved":true}`))
	if out.Code != 400 {
		t.Fatal("arbitrary approval flag accepted")
	}
}
func reviewAPIRequest(api *lab.API, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Host = "127.0.0.1:8123"
	req.Header.Set("Authorization", api.Session().Authorization)
	if method != "GET" {
		req.Header.Set("Origin", "http://127.0.0.1:8123")
		req.Header.Set("X-Lab-CSRF", api.Session().CSRF)
		req.Header.Set("Content-Type", "application/json")
	}
	out := httptest.NewRecorder()
	api.ServeHTTP(out, req)
	return out
}

func TestAgentCannotFinalizeSourceGold(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record := controlledHistoryRecord(t, lab.RunnerOptions{Retain: store.Save})
	worksheet, err := store.CreateReviewWorksheet(record.ID, record.Cases[0].Identity)
	if err != nil {
		t.Fatal(err)
	}
	planner := semanticPlanner(t)
	api, err := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"agent", "model"} {
		body, _ := json.Marshal(map[string]any{"actor": lab.ReviewActor{ID: "assistant", Type: kind}, "contextHash": worksheet.ContextHash, "confirmation": "humanApproved", "rationale": "agent cannot declare final gold"})
		out := reviewAPIRequest(api, "POST", "/lab/api/runs/"+record.ID+"/worksheets/"+worksheet.ID+"/adjudicate", body)
		if out.Code != 403 {
			t.Fatalf("%s adjudication status=%d", kind, out.Code)
		}
	}
	loaded, err := store.LoadReviewWorksheet(record.ID, worksheet.ID)
	if err != nil || loaded.State != "pending" {
		t.Fatal("agent acquired final authority")
	}
}

func TestExplicitHumanCorrectionAppendsVersionAndForeignImportStaysPending(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record := controlledHistoryRecord(t, lab.RunnerOptions{Retain: store.Save})
	original, err := store.ExportJSON(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	worksheet, err := store.CreateReviewWorksheet(record.ID, record.Cases[0].Identity)
	if err != nil {
		t.Fatal(err)
	}
	annotation := lab.ReviewAnnotation{ID: "java-support", Reviewer: lab.ReviewActor{ID: "controlled-assistant", Type: "agent"}, SourceSpan: lab.ReviewSpan{Pointer: "/input", StartByte: 6, EndByte: 10, Text: "Java"}, OutputAtom: lab.ReviewSpan{Pointer: "/output/claims/0/text", StartByte: 0, EndByte: 4, Text: "Java"}, Owner: "unspecified", Certainty: "asserted", Intent: "current fact", Materiality: "required", Qualifier: "No employer, duration or proficiency", ProposedVerdict: "needs-human-review", Rationale: "Source-linked wording still requires explicit domain adjudication."}
	pending, err := store.ProposeReviewWorksheet(record.ID, worksheet.ID, annotation)
	if err != nil {
		t.Fatal(err)
	}
	planner := semanticPlanner(t)
	api, err := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123", HumanReviewer: "controlled-human-example", ReviewOrigin: "controlled-test"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/lab/api/runs/" + record.ID + "/worksheets/" + pending.ID
	out := reviewAPIRequest(api, "GET", path+"/approval", nil)
	if out.Code != 200 {
		t.Fatalf("approval preflight=%d", out.Code)
	}
	var request lab.ReviewApprovalRequest
	if json.Unmarshal(out.Body.Bytes(), &request) != nil {
		t.Fatal("approval preflight unavailable")
	}
	body, _ := json.Marshal(lab.ReviewAdjudicationRequest{Actor: lab.ReviewActor{ID: "controlled-human-example", Type: "human"}, ContextHash: request.ContextHash, WorksheetHash: request.WorksheetHash, Confirmation: request.Confirmation, Rationale: "Controlled example of the approval protocol, not real human gold.", Decisions: []lab.ReviewDecision{{AnnotationID: "java-support", Verdict: "supported", Rationale: "Exact source supports Java without any proficiency or employer claim."}}})
	out = reviewAPIRequest(api, "POST", path+"/adjudicate", body)
	if out.Code != 201 {
		t.Fatalf("human correction=%d: %s", out.Code, out.Body.String())
	}
	var reviewed lab.ReviewWorksheet
	if json.Unmarshal(out.Body.Bytes(), &reviewed) != nil || reviewed.State != "reviewed" || reviewed.ID == pending.ID || reviewed.DerivedLabelVersion == "" || reviewed.DerivedLabelVersion == pending.Binding.LabelVersion || reviewed.ParentID != pending.ID {
		t.Fatal("correction did not append a new derived label version")
	}
	prior, err := store.LoadReviewWorksheet(record.ID, pending.ID)
	if err != nil || prior.State != "pending" {
		t.Fatal("approval rewrote pending history")
	}
	after, err := store.ExportJSON(record.ID)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("approval rewrote original output/gold")
	}
	exported, err := store.ExportReviewWorksheet(record.ID, reviewed.ID)
	if err != nil {
		t.Fatal(err)
	}
	var pretty bytes.Buffer
	if err = json.Indent(&pretty, exported, "", "  "); err != nil {
		t.Fatal(err)
	}
	local, err := store.ImportReviewWorksheet(record.ID, record.Cases[0].Identity, pretty.Bytes())
	if err != nil || local.State != "reviewed" {
		t.Fatal("exact owned approval receipt lost on local round-trip")
	}
	report, reportErr := store.ReviewReport(record.ID, reviewed.ID)
	if reportErr != nil || !report.MeasurementOnly || report.OriginalLabelVersion != pending.Binding.LabelVersion || report.DerivedLabelVersion != reviewed.DerivedLabelVersion || report.Classifications["supported"] != 1 || report.FinalGoldEligible {
		t.Fatal("controlled correction became product improvement or real human gold")
	}
	edited := pending
	edited.Annotations[0].Rationale = "Offline proposed wording correction; still requires human review."
	draft, draftErr := lab.ExportReviewWorksheet(edited)
	if draftErr != nil {
		t.Fatal(draftErr)
	}
	importedDraft, draftErr := store.ImportReviewWorksheet(record.ID, record.Cases[0].Identity, draft)
	if draftErr != nil || importedDraft.ID == pending.ID || importedDraft.ParentID != pending.ID || importedDraft.State != "pending" {
		t.Fatal("offline worksheet edit did not append a pending version")
	}
	if directory := os.Getenv("LAB_REVIEW_EVIDENCE_DIR"); directory != "" {
		if err = os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		pendingBytes, _ := lab.ExportReviewWorksheet(pending)
		reportBytes, _ := json.MarshalIndent(report, "", "  ")
		for name, raw := range map[string][]byte{"agent-proposal.pending.v1.json": pendingBytes, "controlled-human-protocol.reviewed.v1.json": exported, "measurement-only.v1.json": reportBytes} {
			if err = os.WriteFile(filepath.Join(directory, name), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	foreign, err := lab.ImportReviewWorksheet(exported, reviewed.Binding)
	if err != nil || foreign.State != "pending" || foreign.DerivedLabelVersion != "" {
		t.Fatal("foreign human claim became trusted final gold")
	}
}

func TestCaseViewingRecordsExposureBeforeReturningBlindedContent(t *testing.T) {
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record := controlledHistoryRecord(t, lab.RunnerOptions{Retain: store.Save})
	evidence, err := store.Case(record.ID, record.Cases[0].Identity)
	if err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(evidence.Source))
	if err = store.RegisterCohort(lab.CohortEntry{ContentHash: hash, HistoricalName: "controlled-new-en", Split: "newly-authored-blinded", ExposureGroup: "controlled-pair", Language: "en", Authorship: "controlled protocol fixture; no new benchmark claim"}); err != nil {
		t.Fatal(err)
	}
	planner := semanticPlanner(t)
	api, err := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	metadata := reviewAPIRequest(api, "GET", "/lab/api/runs/"+record.ID+"/view", nil)
	if metadata.Code != 200 {
		t.Fatal("metadata unavailable")
	}
	untouched, statusErr := store.CohortStatus(hash)
	if statusErr != nil || !untouched.Untouched {
		t.Fatal("metadata exposed blinded content")
	}
	out := reviewAPIRequest(api, "GET", "/lab/api/runs/"+record.ID+"/cases/"+evidence.Identity, nil)
	if out.Code != 200 {
		t.Fatalf("view=%d", out.Code)
	}
	status, err := store.CohortStatus(hash)
	if err != nil || status.Untouched || len(status.Events) != 1 || status.Events[0].Actor.Type != "unknown" {
		t.Fatal("API exposed blinded content without recording exposure")
	}
}

func TestExposureWriteFailureWithholdsContentButKeepsRunMetadata(t *testing.T) {
	blocked := false
	store, err := lab.OpenFileStore(t.TempDir(), lab.StoreOptions{BeforeWrite: func(operation string) error {
		if blocked && operation == "record-exposure" {
			return fmt.Errorf("controlled exposure failure")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record := controlledHistoryRecord(t, lab.RunnerOptions{Retain: store.Save})
	planner := semanticPlanner(t)
	api, err := lab.NewAPI(planner, lab.NewRunner(planner, lab.RunnerOptions{}), store, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	blocked = true
	base := "/lab/api/runs/" + record.ID
	out := reviewAPIRequest(api, "GET", base+"/cases/"+record.Cases[0].Identity, nil)
	if out.Code != 409 || bytes.Contains(out.Body.Bytes(), []byte("I use Java")) {
		t.Fatal("failed exposure disclosed case content")
	}
	out = reviewAPIRequest(api, "GET", base, nil)
	var metadata lab.RunRecord
	if json.Unmarshal(out.Body.Bytes(), &metadata) != nil || out.Code != 200 || metadata.EngineState != record.EngineState || len(metadata.Cases[0].Source) > 0 || (len(metadata.Cases[0].Actual) > 0 && string(metadata.Cases[0].Actual) != "null") || len(metadata.Cases[0].Expected) > 0 {
		t.Fatal("failed exposure hid recovery state or leaked content")
	}
	out = reviewAPIRequest(api, "GET", base+"/exports?format=json", nil)
	if out.Code != 409 {
		t.Fatal("failed exposure allowed content export")
	}
}
