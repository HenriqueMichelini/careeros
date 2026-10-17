package evaluationlab

import (
	"net/http"
	"strings"
)

func (a *API) exposureRoute(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/lab/api/cohorts" && r.Method == http.MethodPost {
		var entry CohortEntry
		if err := a.decode(w, r, &entry, 4096); err != nil {
			a.error(w, 400, "invalid_cohort", "Bounded cohort provenance required.")
			return true
		}
		if err := a.store.RegisterCohort(entry); err != nil {
			a.error(w, 409, "cohort_rejected", "Cohort declaration is invalid or conflicts with immutable history.")
			return true
		}
		status, err := a.store.CohortStatus(entry.ContentHash)
		if err != nil {
			a.error(w, 503, "exposure_unavailable", "Exposure status unavailable.")
			return true
		}
		a.json(w, 201, status)
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/lab/api/cohorts/") && r.Method == http.MethodGet {
		hash := strings.TrimPrefix(r.URL.Path, "/lab/api/cohorts/")
		if _, err := query(r); err != nil || !evidenceHash.MatchString(hash) {
			a.error(w, 400, "invalid_cohort", "Exact content hash required.")
			return true
		}
		status, err := a.store.CohortStatus(hash)
		if err != nil {
			a.error(w, 503, "exposure_unavailable", "Exposure status unavailable.")
			return true
		}
		a.json(w, 200, status)
		return true
	}
	if r.URL.Path == "/lab/api/exposures" && r.Method == http.MethodPost {
		var input struct {
			ContentHash string      `json:"contentHash"`
			Actor       ReviewActor `json:"actor"`
			Use         string      `json:"use"`
		}
		if err := a.decode(w, r, &input, 2048); err != nil {
			a.error(w, 400, "invalid_exposure", "Bounded exposure declaration required.")
			return true
		}
		if err := a.store.RecordExposure(input.ContentHash, input.Actor, input.Use); err != nil {
			a.error(w, 409, "exposure_rejected", "Exposure could not be recorded.")
			return true
		}
		a.json(w, 201, struct {
			Recorded bool `json:"recorded"`
		}{true})
		return true
	}
	return false
}
