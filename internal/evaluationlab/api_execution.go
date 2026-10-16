package evaluationlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var idempotencyID = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

func (a *API) decode(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	if r.URL.RawQuery != "" {
		return fmt.Errorf("unexpected query")
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("JSON content type required")
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		return fmt.Errorf("request too large")
	}
	if err = validateJSON(raw); err != nil {
		return fmt.Errorf("invalid bounded JSON")
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return fmt.Errorf("JSON object required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid request fields")
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("one JSON object required")
	}
	return nil
}
func (a *API) plan(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Selection Selection     `json:"selection"`
		Caps      ExecutionCaps `json:"caps"`
	}
	if err := a.decode(w, r, &input, 64<<10); err != nil {
		a.error(w, 400, "invalid_request", "A bounded catalog selection and explicit caps are required.")
		return
	}
	plan, err := a.planner.Plan(input.Selection, input.Caps)
	if err != nil {
		a.error(w, 400, "invalid_plan", "Unsupported selection, configuration or execution caps.")
		return
	}
	a.json(w, 200, plan)
}
func (a *API) start(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Plan  Plan   `json:"plan"`
		Token string `json:"idempotencyToken"`
	}
	if err := a.decode(w, r, &input, 256<<10); err != nil || !idempotencyID.MatchString(input.Token) {
		a.error(w, 400, "invalid_request", "A frozen plan and bounded idempotency token are required.")
		return
	}
	if err := a.planner.ValidatePlan(input.Plan); err != nil {
		a.error(w, 400, "invalid_plan", "Plan was altered or is unavailable in this session.")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if prior, ok := a.admissions[input.Token]; ok {
		if prior.PlanHash != input.Plan.Hash {
			a.error(w, 409, "idempotency_conflict", "Token was already used for another plan.")
			return
		}
		a.json(w, 200, struct {
			ID string `json:"id"`
		}{prior.RunID})
		return
	}
	if err := a.store.Health(r.Context()); err != nil {
		a.error(w, 503, "persistence_unavailable", "Private history cannot safely retain another run.")
		return
	}
	if len(a.admissions) >= 1024 {
		a.error(w, 409, "session_capacity", "Session admission capacity reached.")
		return
	}
	id, err := a.runner.Start(a.options.Context, input.Plan)
	if errors.Is(err, ErrRunnerBusy) {
		a.error(w, 409, "busy", "One run is already active.")
		return
	}
	if err != nil {
		a.error(w, 409, "execution_unavailable", "Local execution is unavailable.")
		return
	}
	a.admissions[input.Token] = admission{PlanHash: input.Plan.Hash, RunID: id}
	a.json(w, 201, struct {
		ID string `json:"id"`
	}{id})
}
func (a *API) cancel(w http.ResponseWriter, r *http.Request, id string) {
	var input struct{}
	if err := a.decode(w, r, &input, 1024); err != nil {
		a.error(w, 400, "invalid_request", "Cancellation requires an empty JSON object.")
		return
	}
	if !storeID.MatchString(id) {
		a.error(w, 400, "invalid_identity", "Invalid run identity.")
		return
	}
	if err := a.runner.Cancel(id); err != nil {
		if _, storedErr := a.store.Load(id); storedErr != nil {
			a.error(w, 404, "not_found", "Run unavailable.")
			return
		}
	}
	a.json(w, 200, struct {
		ID        string `json:"id"`
		Requested bool   `json:"cancelRequested"`
	}{id, true})
}
func (a *API) executionRoute(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/lab/api/comparisons":
		a.compare(w, r)
		return true
	case "/lab/api/imports":
		a.importEvidence(w, r)
		return true
	case "/lab/api/plans":
		a.plan(w, r)
		return true
	case "/lab/api/runs":
		a.start(w, r)
		return true
	}
	prefix := "/lab/api/runs/"
	if strings.HasPrefix(r.URL.Path, prefix) && strings.HasSuffix(r.URL.Path, "/cancel") {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/cancel")
		a.cancel(w, r, id)
		return true
	}
	return false
}
