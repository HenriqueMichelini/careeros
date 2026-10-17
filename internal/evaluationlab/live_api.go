package evaluationlab

import (
	"net/http"
	"strings"
)

const LiveGuardVersion = "lab-live-guard-v1"

type LiveCapability struct {
	Version                string `json:"version"`
	Enabled                bool   `json:"enabled"`
	RealExecutionAvailable bool   `json:"realExecutionAvailable"`
	Reason                 string `json:"reason"`
}

func (a *API) liveRoute(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/lab/api/live/") {
		return false
	}
	if r.Method == http.MethodGet && r.URL.Path == "/lab/api/live/capability" {
		if _, err := query(r); err != nil {
			a.error(w, 400, "invalid_query", "Unsupported capability query.")
			return true
		}
		capability := LiveCapability{Version: LiveGuardVersion, Reason: "Disabled by default; no real provider transport is installed. Enablement never authorizes a run."}
		if a.options.LiveGuard != nil {
			capability = a.options.LiveGuard.Capability()
		}
		a.json(w, 200, capability)
		return true
	}
	if r.Method == http.MethodPost && r.URL.Path == "/lab/api/live/preflights" && a.options.LiveGuard != nil && a.options.LiveGuard.Capability().Enabled {
		var input LivePreflightRequest
		if err := a.decode(w, r, &input, 2<<20); err != nil {
			a.error(w, 400, "invalid_preflight", "Bounded owned preflight required; credentials are not accepted.")
			return true
		}
		preflight, err := a.options.LiveGuard.Preflight(input)
		if err != nil {
			a.error(w, 409, "preflight_rejected", "Owned fixed cohort, identities, caps and recipient/model schedule must match.")
			return true
		}
		a.json(w, 201, preflight)
		return true
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/lab/api/live/preflights/") && a.options.LiveGuard != nil {
		id := strings.TrimPrefix(r.URL.Path, "/lab/api/live/preflights/")
		if _, err := query(r); err != nil || !storeID.MatchString(id) {
			a.error(w, 400, "invalid_identity", "Exact preflight identity required.")
			return true
		}
		raw, err := a.options.LiveGuard.ExportPreflight(id)
		if err != nil {
			a.error(w, 404, "preflight_unavailable", "Owned preflight unavailable.")
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="live-preflight-`+id+`.json"`)
		w.WriteHeader(200)
		_, _ = w.Write(raw)
		return true
	}
	a.error(w, 403, "live_execution_disabled", "No live dispatch is available. A separately authorized experiment and transport are required.")
	return true
}

func (g *LiveGuard) Capability() LiveCapability {
	g.mu.Lock()
	defer g.mu.Unlock()
	return LiveCapability{Version: LiveGuardVersion, Enabled: g.options.Enabled && !g.closed, Reason: "Controlled guard preflight capability only; no real provider transport or experiment authorization is installed."}
}
