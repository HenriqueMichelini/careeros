package evaluationlab

import (
	"context"
	"io"
	"net/http"
	"strings"
)

// Shutdown cancels admitted work and waits for partial evidence retention before
// the launch process closes its private store. Context controls the wait only.
func (a *API) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	ids := []string{}
	for _, admitted := range a.admissions {
		ids = append(ids, admitted.RunID)
	}
	a.mu.Unlock()
	for _, id := range ids {
		_ = a.runner.Cancel(id)
		if _, err := a.runner.Wait(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (a *API) asset(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/lab/assets/"
	if !strings.HasPrefix(r.URL.Path, prefix) || r.Method != http.MethodGet {
		return false
	}
	name := strings.TrimPrefix(r.URL.Path, prefix)
	if _, err := query(r); err != nil || !contains([]string{"lab.js", "lab.css"}, name) || a.options.Assets == nil {
		a.error(w, 404, "not_found", "Lab asset unavailable.")
		return true
	}
	f, err := a.options.Assets.Open(name)
	if err != nil {
		a.error(w, 404, "not_found", "Lab asset unavailable.")
		return true
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		a.error(w, 404, "not_found", "Lab asset unavailable.")
		return true
	}
	kind := "text/javascript; charset=utf-8"
	if name == "lab.css" {
		kind = "text/css; charset=utf-8"
	}
	w.Header().Set("Content-Type", kind)
	w.WriteHeader(200)
	_, _ = io.Copy(w, io.LimitReader(f, 1<<20))
	return true
}
func (a *API) assetTags() string {
	if a.options.Assets == nil {
		return ""
	}
	return `<link rel="stylesheet" href="/lab/assets/lab.css"><script type="module" src="/lab/assets/lab.js"></script>`
}
