package evaluationlab

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

const APIVersion = "lab-api-v1"

type APIOptions struct {
	LiveGuard     *LiveGuard
	HumanReviewer string
	ReviewOrigin  string
	Origin        string
	Context       context.Context
	Assets        fs.FS
}
type APISession struct {
	Authorization string `json:"authorization"`
	CSRF          string `json:"csrf"`
}
type admission struct{ PlanHash, RunID string }
type API struct {
	planner    *Planner
	runner     *Runner
	store      *FileStore
	options    APIOptions
	host       string
	session    APISession
	nonce      string
	epoch      string
	mu         sync.Mutex
	admissions map[string]admission
	deleted    map[string]bool
}

func NewAPI(planner *Planner, runner *Runner, store *FileStore, options APIOptions) (*API, error) {
	origin, err := url.Parse(options.Origin)
	if err != nil {
		return nil, fmt.Errorf("invalid local origin")
	}
	ip := net.ParseIP(origin.Hostname())
	port, portErr := strconv.Atoi(origin.Port())
	if origin.Scheme != "http" || ip == nil || !ip.IsLoopback() || portErr != nil || port < 1 || port > 65535 || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, fmt.Errorf("explicit loopback origin required")
	}
	if planner == nil || runner == nil || store == nil {
		return nil, fmt.Errorf("local execution/history unavailable")
	}
	if len(options.HumanReviewer) > 128 || options.ReviewOrigin != "" && !contains([]string{"local-human-attestation", "controlled-test"}, options.ReviewOrigin) {
		return nil, fmt.Errorf("invalid human review configuration")
	}
	if options.LiveGuard != nil && (options.LiveGuard.planner != planner || options.LiveGuard.store != store) {
		return nil, fmt.Errorf("guard must own this planner and store")
	}
	if options.Context == nil {
		options.Context = context.Background()
	}
	return &API{planner: planner, runner: runner, store: store, options: options, host: origin.Host, session: APISession{Authorization: "Bearer " + newLabID(), CSRF: newLabID()}, nonce: newLabID(), epoch: newLabID(), admissions: map[string]admission{}, deleted: map[string]bool{}}, nil
}

// Session is for the trusted local launch/test harness only. Never log or store.
func (a *API) Session() APISession { return a.session }
func secretEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Host != a.host || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != a.options.Origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		a.error(w, http.StatusForbidden, "forbidden", "Local origin required.")
		return
	}

	if r.URL.Path == "/lab/" && r.Method == http.MethodGet {
		if _, err := query(r); err != nil {
			a.error(w, 400, "invalid_query", "Unsupported launch query.")
			return
		}
		a.shell(w)
		return
	}
	if a.asset(w, r) {
		return
	}
	if !secretEqual(r.Header.Get("Authorization"), a.session.Authorization) {
		a.error(w, 403, "forbidden", "Session authorization required.")
		return
	}
	if r.Method != http.MethodGet && (!secretEqual(r.Header.Get("X-Lab-CSRF"), a.session.CSRF) || r.Header.Get("Origin") != a.options.Origin) {
		a.error(w, 403, "forbidden", "Mutation origin and CSRF authorization required.")
		return
	}
	if r.URL.Path == "/lab/api/suites" && r.Method == http.MethodGet {
		if _, err := query(r); err != nil {
			a.error(w, 400, "invalid_query", "Unsupported catalog query.")
			return
		}
		a.json(w, 200, struct {
			Version string  `json:"version"`
			Suites  []Suite `json:"suites"`
		}{APIVersion, a.dashboardCatalog()})
		return
	}
	if a.liveRoute(w, r) {
		return
	}
	if a.exposureRoute(w, r) {
		return
	}
	if a.reviewRoute(w, r) {
		return
	}
	if a.historyRoute(w, r) {
		return
	}
	if a.dashboardRoute(w, r) {
		return
	}
	if a.executionRoute(w, r) {
		return
	}
	a.error(w, 404, "not_found", "Unknown local endpoint.")
}
func (a *API) json(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (a *API) error(w http.ResponseWriter, status int, code, message string) {
	a.json(w, status, struct {
		Version string `json:"version"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{APIVersion, code, message})
}
func (a *API) shell(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; connect-src 'self'; script-src 'self' 'nonce-"+a.nonce+"'; style-src 'self'")
	session, _ := json.Marshal(a.session)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>CareerOS Evaluation Lab</title><body><div id="lab"><h1>Evaluation Lab</h1><p>Private local controlled evaluation. No live provider access.</p></div><script nonce="%s">window.__LAB_SESSION__=%s;</script>%s</body></html>`, a.nonce, session, a.assetTags())
}
