package evaluationlab

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"professional-information-repo/internal/semanticeval"
)

type suiteAvailability struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func caseMetadata(cases []ImportedCase) []ImportedCase {
	result := make([]ImportedCase, len(cases))
	for i, c := range cases {
		c.Source = nil
		c.Expected = nil
		c.Actual = nil
		c.Review = nil
		c.Children = caseMetadata(c.Children)
		result[i] = c
	}
	return result
}

func (a *API) dashboardView(w http.ResponseWriter, r *http.Request, id string) {
	if _, err := query(r); err != nil {
		a.error(w, 400, "invalid_query", "Unsupported view query.")
		return
	}
	var record RunRecord
	var err error
	if current, readErr := a.runner.Read(id); readErr == nil {
		record = current
		if current.EngineState != "planned" && current.EngineState != "running" {
			if saved, saveErr := a.store.Load(id); saveErr == nil && saved.EngineState == current.EngineState {
				record = saved
			}
		}
	} else {
		record, err = a.store.Load(id)
	}
	if err == nil {
		record.Cases = caseMetadata(record.Cases)
		a.json(w, 200, record)
		return
	}
	imported, err := a.store.LoadImport(id)
	if err != nil {
		a.error(w, 404, "history_unavailable", "Run or import evidence is unavailable.")
		return
	}
	metrics := Metrics(imported)
	imported.Cases = caseMetadata(imported.Cases)
	imported.Original = nil
	imported.Dependencies = nil
	a.json(w, 200, struct {
		ImportResult
		ID      string   `json:"id"`
		Durable bool     `json:"durable"`
		Metrics []Metric `json:"metrics"`
	}{imported, id, true, metrics})
}

func (a *API) dashboardRoute(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/lab/api/status" {
		return false
	}
	if _, err := query(r); err != nil {
		a.error(w, 400, "invalid_query", "Unsupported status query.")
		return true
	}
	usage, usageErr := a.store.Usage()
	ready := a.store.Health(r.Context()) == nil && usageErr == nil
	process := ProcessCapabilities{RuntimeVersions: map[string]string{}, UnavailableReasons: []string{"Process adapter is unavailable in this launch."}}
	if a.runner.options.Processes != nil {
		process = a.runner.options.Processes.Capabilities()
	}
	suites := []suiteAvailability{}
	for _, suite := range Catalog() {
		available := len(a.planner.assets[suite.ID]) > 0
		if suite.ID == "application-context" {
			available = available && len(a.planner.assets["semantic-quality"]) > 0 && len(a.planner.assets["application-baseline"]) > 0
		}
		reason := ""
		if suite.ID == "test-adapters" && !process.IsolationAvailable {
			available = false
			reason = "Private process isolation is unavailable; no unsandboxed fallback."
		}
		if !available && reason == "" {
			reason = "Required approved fixture assets are unavailable in this launch."
		}
		suites = append(suites, suiteAvailability{suite.ID, available, reason})
	}
	a.json(w, 200, struct {
		Process       ProcessCapabilities `json:"processCapabilities"`
		Version       string              `json:"version"`
		StorageReady  bool                `json:"storageReady"`
		Usage         StoreUsage          `json:"usage"`
		Suites        []suiteAvailability `json:"suites"`
		Metrics       []MetricDefinition  `json:"metricDefinitions"`
		Build         any                 `json:"build"`
		HumanReviewer string              `json:"humanReviewer,omitempty"`
	}{process, APIVersion, ready, usage, suites, MetricRegistry(), a.dashboardBuild(), a.options.HumanReviewer})
	return true
}

func (a *API) dashboardCatalog() []Suite {
	suites := Catalog()
	for i := range suites {
		var document struct {
			Cases []struct {
				ID   string   `json:"id"`
				Tags []string `json:"tags"`
			} `json:"cases"`
		}
		if json.Unmarshal(a.planner.assets[suites[i].ID], &document) != nil {
			continue
		}
		tags := map[string][]string{}
		for _, c := range document.Cases {
			tags[c.ID] = c.Tags
		}
		for j := range suites[i].Cases {
			suites[i].Cases[j].Categories = append([]string{}, tags[suites[i].Cases[j].ID]...)
		}
	}
	return suites
}

func (a *API) dashboardBuild() any {
	hashes := map[string]string{}
	if a.options.Assets != nil {
		for _, name := range []string{"lab.js", "lab.css"} {
			// Only fixed local code assets, never a client-supplied filename.
			var file fs.File
			var err error
			if name == "lab.js" {
				file, err = a.options.Assets.Open("lab.js")
			} else {
				file, err = a.options.Assets.Open("lab.css")
			}
			if err != nil {
				continue
			}
			info, err := file.Stat()
			if err == nil && info.Mode().IsRegular() && info.Size() <= 1<<20 {
				raw, readErr := io.ReadAll(io.LimitReader(file, 1<<20))
				if readErr == nil {
					hashes[name] = semanticeval.Hash(raw)
				}
			}
			file.Close()
		}
	}
	return struct {
		AssetHashes map[string]string `json:"assetHashes"`
		Provenance  Provenance        `json:"launchSource"`
		Limit       string            `json:"limit"`
	}{hashes, a.planner.provenance, "Exact local asset/source bytes; hashes alone do not prove deployed behavior, binary correspondence or semantic quality."}
}

func (a *API) deleteEvidence(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		ConfirmID string `json:"confirmId"`
	}
	if err := a.decode(w, r, &input, 1024); err != nil || !storeID.MatchString(id) || input.ConfirmID != id {
		a.error(w, 400, "confirmation_required", "Confirm the exact run identity to delete private evidence.")
		return
	}
	// Keep admission and terminal cleanup atomic. A running record is never
	// deleted, so retention cannot recreate a partly deleted run.
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runner.mu.Lock()
	defer a.runner.mu.Unlock()
	if state, ok := a.runner.runs[id]; ok && (state.record.EngineState == "planned" || state.record.EngineState == "running") {
		a.error(w, 409, "active_run", "Cancel and wait for terminal retention before deletion.")
		return
	}
	if err := a.store.Delete(id); err != nil {
		a.error(w, 409, "delete_unavailable", "Private evidence cannot be deleted.")
		return
	}
	delete(a.runner.runs, id)
	a.deleted[id] = true
	a.json(w, 200, struct {
		ID string `json:"deletedId"`
	}{id})
}
