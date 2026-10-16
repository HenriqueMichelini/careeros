package evaluationlab

import (
	"context"
	"io"
	"io/fs"
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

const assetUnavailable = "Lab asset unavailable."

func (a *API) asset(w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/lab/assets/"
	if !strings.HasPrefix(r.URL.Path, prefix) || r.Method != http.MethodGet {
		return false
	}
	if _, err := query(r); err != nil || a.options.Assets == nil {
		a.error(w, 404, "not_found", assetUnavailable)
		return true
	}
	var f fs.File
	var err error
	kind := "text/javascript; charset=utf-8"
	switch r.URL.Path {
	case "/lab/assets/lab.js":
		f, err = a.options.Assets.Open("lab.js")
	case "/lab/assets/lab.css":
		f, err = a.options.Assets.Open("lab.css")
		kind = "text/css; charset=utf-8"
	default:
		a.error(w, 404, "not_found", assetUnavailable)
		return true
	}
	if err != nil {
		a.error(w, 404, "not_found", assetUnavailable)
		return true
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		a.error(w, 404, "not_found", assetUnavailable)
		return true
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
