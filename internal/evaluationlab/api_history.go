package evaluationlab

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func query(r *http.Request, keys ...string) (url.Values, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	for key, values := range q {
		if !contains(keys, key) || len(values) != 1 {
			return nil, fmt.Errorf("unsupported query")
		}
	}
	return q, nil
}
func queryInt(q url.Values, key string, fallback, maximum int) (int, error) {
	if q.Get(key) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(q.Get(key))
	if err != nil || value < 0 || value > maximum {
		return 0, fmt.Errorf("invalid query bound")
	}
	return value, nil
}
func (a *API) historyRoute(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.URL.Path == "/lab/api/runs" {
		a.list(w, r)
		return true
	}
	prefix := "/lab/api/runs/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if !storeID.MatchString(parts[0]) {
		a.error(w, 400, "invalid_identity", "Invalid run identity.")
		return true
	}
	switch {
	case len(parts) == 1:
		a.detail(w, r, parts[0])
	case len(parts) == 2 && parts[1] == "events":
		a.events(w, r, parts[0])
	case len(parts) == 2 && parts[1] == "exports":
		a.export(w, r, parts[0])
	case len(parts) == 3 && parts[1] == "cases":
		a.caseDetail(w, r, parts[0], parts[2])
	default:
		return false
	}
	return true
}
func (a *API) list(w http.ResponseWriter, r *http.Request) {
	q, err := query(r, "suite", "engineState", "offset", "limit")
	if err != nil {
		a.error(w, 400, "invalid_query", "Unsupported history query.")
		return
	}
	offset, e1 := queryInt(q, "offset", 0, 4096)
	limit, e2 := queryInt(q, "limit", 20, 100)
	if e1 != nil || e2 != nil || limit == 0 {
		a.error(w, 400, "invalid_query", "Invalid history pagination.")
		return
	}
	page, err := a.store.List(RunFilter{Suite: q.Get("suite"), EngineState: q.Get("engineState"), Offset: offset, Limit: limit})
	if err != nil {
		a.error(w, 503, "history_unavailable", "History is unavailable.")
		return
	}
	a.json(w, 200, page)
}
func (a *API) detail(w http.ResponseWriter, r *http.Request, id string) {
	if _, err := query(r); err != nil {
		a.error(w, 400, "invalid_query", "Unsupported run query.")
		return
	}
	if current, err := a.runner.Read(id); err == nil {
		if current.EngineState != "planned" && current.EngineState != "running" {
			if saved, saveErr := a.store.Load(id); saveErr == nil && saved.EngineState == current.EngineState {
				a.json(w, 200, saved)
				return
			}
		}
		a.json(w, 200, current)
		return
	}
	if saved, err := a.store.Load(id); err == nil {
		a.json(w, 200, saved)
		return
	}
	if imported, err := a.store.LoadImport(id); err == nil {
		a.json(w, 200, imported)
		return
	}
	a.error(w, 404, "history_unavailable", "Run or import evidence is unavailable.")
}
func (a *API) caseDetail(w http.ResponseWriter, r *http.Request, id, caseID string) {
	if _, err := query(r); err != nil || len(caseID) > 256 {
		a.error(w, 400, "invalid_query", "Invalid case lookup.")
		return
	}
	detail, err := a.store.Case(id, caseID)
	if err != nil {
		a.error(w, 404, "case_unavailable", "Case evidence is unavailable.")
		return
	}
	a.json(w, 200, detail)
}
func (a *API) events(w http.ResponseWriter, r *http.Request, id string) {
	q, err := query(r, "after", "limit", "epoch")
	if err != nil {
		a.error(w, 400, "invalid_query", "Unsupported event query.")
		return
	}
	after := uint64(0)
	if q.Get("after") != "" {
		after, err = strconv.ParseUint(q.Get("after"), 10, 64)
	}
	limit, limitErr := queryInt(q, "limit", 50, 256)
	if err != nil || limitErr != nil || limit == 0 {
		a.error(w, 400, "invalid_query", "Invalid event bounds.")
		return
	}
	if _, readErr := a.runner.Read(id); readErr == nil {
		if q.Get("epoch") != "" && q.Get("epoch") != a.epoch {
			a.error(w, 409, "event_epoch_changed", "Reconnect requires a fresh event cursor.")
			return
		}
		page, eventErr := a.runner.Events(id, after, limit)
		if eventErr != nil {
			a.error(w, 400, "invalid_cursor", "Event cursor is outside the observed sequence.")
			return
		}
		a.json(w, 200, struct {
			EventPage
			Epoch string `json:"epoch"`
			Kind  string `json:"kind"`
		}{page, a.epoch, "live-stage-ring"})
		return
	}
	epoch := "checkpoint-" + id
	if q.Get("epoch") != "" && q.Get("epoch") != epoch {
		a.error(w, 409, "event_epoch_changed", "Restarted history uses checkpoint events; reset the cursor.")
		return
	}
	events, eventErr := a.store.Events(id, after, limit)
	if eventErr != nil {
		a.error(w, 400, "events_unavailable", "Checkpoint events or cursor are unavailable.")
		return
	}
	next := after
	if len(events) > 0 {
		next = events[len(events)-1].Sequence
	}
	a.json(w, 200, struct {
		Events []StoreEvent `json:"events"`
		Epoch  string       `json:"epoch"`
		Kind   string       `json:"kind"`
		Gap    bool         `json:"gap"`
		Next   uint64       `json:"next"`
	}{events, epoch, "durable-checkpoints", false, next})
}
func (a *API) importEvidence(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Original     []byte   `json:"originalBytes"`
		Dependencies [][]byte `json:"dependencies"`
		Export       []byte   `json:"exportBundle"`
	}
	if err := a.decode(w, r, &input, 16<<20); err != nil {
		a.error(w, 400, "invalid_import", "A bounded byte import is required.")
		return
	}
	var id string
	var err error
	if len(input.Export) > 0 {
		if len(input.Original) > 0 || len(input.Dependencies) > 0 {
			a.error(w, 400, "invalid_import", "Choose one import format.")
			return
		}
		id, err = a.store.ImportExport(input.Export)
	} else {
		id, err = a.store.Import(input.Original, input.Dependencies)
	}
	if err != nil {
		a.error(w, 400, "invalid_import", "Import is invalid, exceeds bounds or cannot be saved.")
		return
	}
	a.json(w, 201, struct {
		ID string `json:"id"`
	}{id})
}
func (a *API) export(w http.ResponseWriter, r *http.Request, id string) {
	q, err := query(r, "format")
	if err != nil {
		a.error(w, 400, "invalid_query", "Unsupported export query.")
		return
	}
	var raw []byte
	kind := "application/json"
	extension := "json"
	switch q.Get("format") {
	case "json":
		raw, err = a.store.ExportJSON(id)
	case "original":
		raw, err = a.store.ExportOriginal(id)
	case "markdown":
		raw, err = a.store.ExportMarkdown(id)
		kind = "text/markdown; charset=utf-8"
		extension = "md"
	case "diagnostics":
		raw, err = a.store.ExportDiagnostics(id)
	default:
		a.error(w, 400, "invalid_format", "Select an explicit supported export format.")
		return
	}
	if err != nil {
		a.error(w, 409, "export_unavailable", "Approved synthetic export is unavailable.")
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", `attachment; filename="evaluation-`+id+`.`+extension+`"`)
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}
