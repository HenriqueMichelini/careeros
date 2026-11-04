package accountapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/workflowauth"
	"sync"
)

func ProtectWorkflow(service *auth.Service, store *accountstore.Store, path string, next http.Handler) http.Handler {
	return workflowauth.Protect(service, store, path, next)
}

func workflowError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// WorkflowFromEnv is used by each separate Function. Missing configuration
// fails closed; no direct Function URL can select the old guest handler.
func WorkflowFromEnv(path string, next http.Handler) http.Handler {
	var mu sync.Mutex
	var runtime *Runtime
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if runtime == nil {
			runtime, _ = FromEnv(context.Background())
		}
		current := runtime
		mu.Unlock()
		if current == nil {
			auth.UnavailableHandler().ServeHTTP(w, r)
			return
		}
		ProtectWorkflow(current.Service, current.Store, path, next).ServeHTTP(w, r)
	})
}

type resultWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w resultWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}
