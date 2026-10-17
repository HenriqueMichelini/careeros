package evaluationlab

import (
	"net/http"
	"strings"
)

func (a *API) reviewRoute(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/lab/api/runs/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) == 4 && parts[1] == "worksheets" && parts[3] == "report" && r.Method == http.MethodGet {
		if _, err := query(r); err != nil {
			a.error(w, 400, "invalid_query", "Unsupported report query.")
			return true
		}
		report, err := a.store.ReviewReport(parts[0], parts[2])
		if err != nil {
			a.error(w, 404, "review_unavailable", "Owned review report unavailable.")
			return true
		}
		a.json(w, 200, report)
		return true
	}
	if len(parts) == 4 && parts[1] == "worksheets" && parts[3] == "propose" && r.Method == http.MethodPost {
		var annotation ReviewAnnotation
		if err := a.decode(w, r, &annotation, 16<<10); err != nil {
			a.error(w, 400, "invalid_review", "Bounded pending annotation required.")
			return true
		}
		if !a.exposeContent(w, parts[0], "", "worksheet") {
			return true
		}
		worksheet, err := a.store.ProposeReviewWorksheet(parts[0], parts[2], annotation)
		if err != nil {
			a.error(w, 409, "proposal_rejected", "Exact source and output spans with reviewer provenance required.")
			return true
		}
		a.json(w, 201, worksheet)
		return true
	}
	if len(parts) == 4 && parts[1] == "worksheets" && parts[3] == "approval" && r.Method == http.MethodGet {
		a.reviewApproval(w, r, parts[0], parts[2])
		return true
	}
	if len(parts) == 4 && parts[1] == "worksheets" && parts[3] == "adjudicate" && r.Method == http.MethodPost {
		a.adjudicateReviewWorksheet(w, r, parts[0], parts[2])
		return true
	}
	if len(parts) == 3 && parts[1] == "worksheets" && r.Method == http.MethodGet {
		a.exportReviewWorksheet(w, r, parts[0], parts[2])
		return true
	}
	if len(parts) < 4 || parts[1] != "cases" || parts[3] != "worksheet" || r.Method != http.MethodPost {
		return false
	}
	switch {
	case len(parts) == 4:
		a.createReviewWorksheet(w, r, parts[0], parts[2])
		return true
	case len(parts) == 5 && parts[4] == "import":
		a.importReviewWorksheet(w, r, parts[0], parts[2])
		return true
	default:
		return false
	}
}
func (a *API) createReviewWorksheet(w http.ResponseWriter, r *http.Request, runID, caseID string) {
	var input struct{}
	if err := a.decode(w, r, &input, 1024); err != nil {
		a.error(w, 400, "invalid_review", "Worksheet creation requires an empty JSON object.")
		return
	}
	if !a.exposeContent(w, runID, caseID, "worksheet") {
		return
	}
	worksheet, err := a.store.CreateReviewWorksheet(runID, caseID)
	if err != nil {
		a.error(w, 409, "review_unavailable", "Owned immutable synthetic case evidence is required.")
		return
	}
	a.json(w, 201, worksheet)
}
func (a *API) exportReviewWorksheet(w http.ResponseWriter, r *http.Request, runID, worksheetID string) {
	if _, err := query(r); err != nil {
		a.error(w, 400, "invalid_query", "Unsupported worksheet export query.")
		return
	}
	raw, err := a.store.ExportReviewWorksheet(runID, worksheetID)
	if err != nil {
		a.error(w, 404, "review_unavailable", "Worksheet evidence is unavailable.")
		return
	}
	if !a.exposeContent(w, runID, "", "export") {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="source-review-`+worksheetID+`.json"`)
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}
func (a *API) importReviewWorksheet(w http.ResponseWriter, r *http.Request, runID, caseID string) {
	var input struct {
		Worksheet []byte `json:"worksheetBytes"`
	}
	if err := a.decode(w, r, &input, 2*MaxReviewWorksheetBytes); err != nil {
		a.error(w, 400, "invalid_review", "A bounded worksheet byte import is required.")
		return
	}
	if !a.exposeContent(w, runID, caseID, "worksheet") {
		return
	}
	worksheet, err := a.store.ImportReviewWorksheet(runID, caseID, input.Worksheet)
	if err != nil {
		a.error(w, 400, "invalid_review", "Worksheet text, source, labels or authority do not match this case.")
		return
	}
	a.json(w, 201, worksheet)
}

func (a *API) reviewApproval(w http.ResponseWriter, r *http.Request, runID, worksheetID string) {
	if _, err := query(r); err != nil {
		a.error(w, 400, "invalid_query", "Unsupported approval query.")
		return
	}
	worksheet, err := a.store.LoadReviewWorksheet(runID, worksheetID)
	if err != nil {
		a.error(w, 404, "review_unavailable", "Worksheet evidence is unavailable.")
		return
	}
	request, err := reviewApproval(worksheet, a.options.HumanReviewer)
	if err != nil {
		a.error(w, 409, "human_review_disabled", "Configure Henrique or a designated human domain reviewer explicitly.")
		return
	}
	a.json(w, 200, request)
}
func (a *API) adjudicateReviewWorksheet(w http.ResponseWriter, r *http.Request, runID, worksheetID string) {
	var input ReviewAdjudicationRequest
	if err := a.decode(w, r, &input, 64<<10); err != nil {
		a.error(w, 400, "invalid_review", "Explicit bounded review provenance is required.")
		return
	}
	if input.Actor.Type != "human" {
		a.error(w, 403, "human_review_required", "Henrique or a designated human domain reviewer must decide final gold.")
		return
	}
	if !a.exposeContent(w, runID, "", "worksheet") {
		return
	}
	worksheet, err := a.store.adjudicateReviewWorksheet(runID, worksheetID, input, a.options.HumanReviewer, a.options.ReviewOrigin)
	if err != nil {
		a.error(w, 409, "approval_rejected", "Reviewer, exact worksheet/source/label identity, confirmation and rationale must match.")
		return
	}
	a.json(w, 201, worksheet)
}
