package evaluationlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"professional-information-repo/internal/semanticeval"
)

const reviewDirectory = "source-reviews"

type storedReview struct {
	Version   string          `json:"version"`
	Hash      string          `json:"hash"`
	Authority bool            `json:"authority"`
	Worksheet ReviewWorksheet `json:"worksheet"`
}

func (s *FileStore) reviewBinding(runID, caseID string) (ReviewBinding, error) {
	if err := s.check(); err != nil {
		return ReviewBinding{}, err
	}
	manifest, err := s.manifest(runID)
	if err != nil {
		return ReviewBinding{}, err
	}
	if manifest.Kind != "" {
		return ReviewBinding{}, fmt.Errorf("an owned synthetic run is required for this review")
	}
	record, err := s.load(manifest)
	if err != nil {
		return ReviewBinding{}, err
	}
	if record.EngineState == "planned" || record.EngineState == "running" {
		return ReviewBinding{}, fmt.Errorf("wait for terminal immutable run evidence")
	}
	if err = validateSyntheticExport(record); err != nil {
		return ReviewBinding{}, err
	}
	c, err := s.caseEvidence(runID, caseID)
	if err != nil {
		return ReviewBinding{}, err
	}
	labels := record.Context.LabelHash
	if labels == "" {
		labels = "owned-label-bytes:" + semanticeval.Hash(c.Expected)
	}
	binding := ReviewBinding{RunID: runID, CaseID: caseID, LabelVersion: labels, SourceBytes: bytes.Clone(c.Source), OutputBytes: bytes.Clone(c.Actual), GoldBytes: bytes.Clone(c.Expected)}
	if _, err = reviewBindingHash(binding); err != nil {
		return ReviewBinding{}, err
	}
	return binding, nil
}
func (s *FileStore) CreateReviewWorksheet(runID, caseID string) (ReviewWorksheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, err := s.reviewBinding(runID, caseID)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	worksheet, err := NewReviewWorksheet(binding)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	if err = s.saveReviewWorksheet(worksheet); err != nil {
		return ReviewWorksheet{}, err
	}
	return worksheet, nil
}
func reviewWorksheetPath(runID, worksheetID string) (string, error) {
	if !storeID.MatchString(runID) || !storeID.MatchString(worksheetID) {
		return "", fmt.Errorf("invalid review identity")
	}
	return filepath.Join(runID, reviewDirectory, worksheetID+".json"), nil
}
func (s *FileStore) saveReviewWorksheet(worksheet ReviewWorksheet) error {
	content, err := ExportReviewWorksheet(worksheet)
	raw, marshalErr := json.Marshal(storedReview{Version: ReviewWorksheetVersion, Hash: semanticeval.Hash(content), Authority: worksheet.State == "reviewed", Worksheet: worksheet})
	if marshalErr != nil {
		return marshalErr
	}
	if err != nil {
		return err
	}
	path, err := reviewWorksheetPath(worksheet.Binding.RunID, worksheet.ID)
	if err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if err = s.guard("create-source-review"); err != nil {
		return err
	}
	if err = s.safe(worksheet.Binding.RunID); err != nil {
		return err
	}
	if err = s.root.Mkdir(parent, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err = s.safe(parent); err != nil {
		return err
	}
	prior, err := s.read(path, MaxReviewWorksheetBytes+4096)
	if err == nil {
		if !bytes.Equal(prior, raw) {
			return fmt.Errorf("immutable review version cannot be overwritten")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory, openErr := s.root.Open(parent)
	if openErr != nil {
		return openErr
	}
	entries, readErr := directory.ReadDir(257)
	directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if len(entries) >= 256 {
		return fmt.Errorf("review version capacity reached")
	}
	if err = s.atomic(path, raw); err != nil {
		return err
	}
	return s.syncDir(worksheet.Binding.RunID)
}
func (s *FileStore) loadReviewWorksheet(runID, worksheetID string) (ReviewWorksheet, error) {
	path, err := reviewWorksheetPath(runID, worksheetID)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	raw, err := s.read(path, MaxReviewWorksheetBytes+4096)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	var envelope storedReview
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return ReviewWorksheet{}, err
	}
	worksheet := envelope.Worksheet
	content, err := ExportReviewWorksheet(worksheet)
	if err != nil || envelope.Version != ReviewWorksheetVersion || envelope.Hash != semanticeval.Hash(content) {
		return ReviewWorksheet{}, fmt.Errorf("stored review integrity mismatch")
	}
	binding, err := s.reviewBinding(runID, worksheet.Binding.CaseID)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	if err = validateReviewWorksheet(worksheet, binding); err != nil {
		return ReviewWorksheet{}, err
	}
	if worksheet.State == "reviewed" && !envelope.Authority {
		return ReviewWorksheet{}, fmt.Errorf("owned adjudication receipt required")
	}
	if worksheet.Binding.RunID != runID || worksheet.ID != worksheetID {
		return ReviewWorksheet{}, fmt.Errorf("review filename identity mismatch")
	}
	return worksheet, nil
}
func (s *FileStore) LoadReviewWorksheet(runID, worksheetID string) (ReviewWorksheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadReviewWorksheet(runID, worksheetID)
}
func (s *FileStore) ExportReviewWorksheet(runID, worksheetID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	worksheet, err := s.loadReviewWorksheet(runID, worksheetID)
	if err != nil {
		return nil, err
	}
	return ExportReviewWorksheet(worksheet)
}
func (s *FileStore) ImportReviewWorksheet(runID, caseID string, raw []byte) (ReviewWorksheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, err := s.reviewBinding(runID, caseID)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	worksheet, err := ImportReviewWorksheet(raw, binding)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	var uploaded ReviewWorksheet
	if err = json.Unmarshal(raw, &uploaded); err != nil {
		return ReviewWorksheet{}, err
	}
	canonical, err := ExportReviewWorksheet(uploaded)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	// Only an exact version already owned by this store retains its local receipt.
	if prior, loadErr := s.loadReviewWorksheet(runID, worksheet.ID); loadErr == nil {
		content, exportErr := ExportReviewWorksheet(prior)
		if exportErr == nil && bytes.Equal(content, canonical) {
			return prior, nil
		}
		if exportErr != nil {
			return ReviewWorksheet{}, exportErr
		}
		worksheet.ID = newLabID()
		worksheet.ParentID = prior.ID
		worksheet.ParentHash = semanticeval.Hash(content)
		worksheet.Revision = prior.Revision + 1
	}
	if err = s.saveReviewWorksheet(worksheet); err != nil {
		return ReviewWorksheet{}, err
	}
	return worksheet, nil
}

func (s *FileStore) ProposeReviewWorksheet(runID, worksheetID string, annotation ReviewAnnotation) (ReviewWorksheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.loadReviewWorksheet(runID, worksheetID)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	proposed, err := ProposeSourceReview(old, old.Binding, annotation)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	raw, err := ExportReviewWorksheet(old)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	proposed.ID = newLabID()
	proposed.Revision = old.Revision + 1
	proposed.ParentID = old.ID
	proposed.ParentHash = semanticeval.Hash(raw)
	if err = s.saveReviewWorksheet(proposed); err != nil {
		return ReviewWorksheet{}, err
	}
	return proposed, nil
}
func reviewApproval(worksheet ReviewWorksheet, reviewer string) (ReviewApprovalRequest, error) {
	if reviewer == "" || len(reviewer) > 128 || worksheet.State != "pending" || len(worksheet.Annotations) == 0 {
		return ReviewApprovalRequest{}, fmt.Errorf("configured human reviewer and pending annotations required")
	}
	raw, err := ExportReviewWorksheet(worksheet)
	if err != nil {
		return ReviewApprovalRequest{}, err
	}
	hash := semanticeval.Hash(raw)
	return ReviewApprovalRequest{Reviewer: reviewer, ContextHash: worksheet.ContextHash, WorksheetHash: hash, LabelVersion: worksheet.Binding.LabelVersion, Confirmation: "Approve " + hash + " as " + reviewer}, nil
}
func (s *FileStore) adjudicateReviewWorksheet(runID, worksheetID string, input ReviewAdjudicationRequest, reviewer, origin string) (ReviewWorksheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.loadReviewWorksheet(runID, worksheetID)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	approval, err := reviewApproval(old, reviewer)
	if err != nil {
		return ReviewWorksheet{}, err
	}
	if input.Actor.Type != "human" || input.Actor.ID != reviewer || input.ContextHash != approval.ContextHash || input.WorksheetHash != approval.WorksheetHash || input.Confirmation != approval.Confirmation || input.Rationale == "" || len(input.Rationale) > 4096 {
		return ReviewWorksheet{}, fmt.Errorf("explicit bound human attestation required")
	}
	if origin == "" {
		origin = "local-human-attestation"
	}
	if !contains([]string{"local-human-attestation", "controlled-test"}, origin) {
		return ReviewWorksheet{}, fmt.Errorf("invalid review origin")
	}
	if err = validateReviewDecisions(input.Decisions, old.Annotations); err != nil {
		return ReviewWorksheet{}, err
	}
	result := cloneJSON(old)
	result.ID = newLabID()
	result.Revision++
	result.ParentID = old.ID
	result.ParentHash = approval.WorksheetHash
	receipt := ReviewAdjudication{Actor: input.Actor, Origin: origin, WorksheetHash: approval.WorksheetHash, ContextHash: old.ContextHash, Rationale: input.Rationale, Decisions: cloneJSON(input.Decisions), Verified: true}
	result.Adjudications = append(result.Adjudications, receipt)
	result.State = "reviewed"
	for _, decision := range input.Decisions {
		if decision.Verdict == "unresolved" {
			result.State = "pending"
		}
	}
	if result.State == "reviewed" {
		raw, _ := json.Marshal(receipt)
		result.DerivedLabelVersion = "review-label-v1:" + semanticeval.Hash(raw)
	}
	if err = s.saveReviewWorksheet(result); err != nil {
		return ReviewWorksheet{}, err
	}
	return result, nil
}
func validateReviewDecisions(decisions []ReviewDecision, annotations []ReviewAnnotation) error {
	if len(decisions) != len(annotations) || len(decisions) == 0 {
		return fmt.Errorf("each annotation requires a human decision")
	}
	known := map[string]bool{}
	for _, a := range annotations {
		known[a.ID] = true
	}
	for _, d := range decisions {
		if !known[d.AnnotationID] || d.Rationale == "" || len(d.Rationale) > 4096 || !contains([]string{"supported", "unsupported", "omission", "forbidden-implication", "unresolved"}, d.Verdict) {
			return fmt.Errorf("invalid or duplicate human decision")
		}
		delete(known, d.AnnotationID)
	}
	return nil
}
