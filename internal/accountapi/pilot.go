package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

// NewPilot is the explicit, local pilot composition. No environment fallback
// selects it and it never mounts the legacy caller-key path. Each injected
// workflow must be a local fake in this slice; managed dispatch is separate.
// Missing dependencies/handlers fail closed. Operator decisions have no HTTP API.
func NewPilot(service *auth.Service, store *accountstore.Store, workflows map[string]http.Handler) http.Handler {
	if service == nil || store == nil {
		return auth.UnavailableHandler()
	}
	mux := http.NewServeMux()
	profiles := New(service, store.PilotProfiles())
	mux.Handle("/api/auth/", profiles)
	mux.Handle("/api/profile/export", profiles)
	mux.Handle("/api/profile/current", profiles)
	mux.Handle("/api/profile/operations/", profiles)
	mux.Handle("/api/deletion/", profiles)
	mux.Handle("/api/lifecycle/status", profiles)
	mux.Handle("/api/rights/", profiles)
	pilot := http.NewServeMux()
	pilot.HandleFunc("GET /api/pilot/status", func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromRequest(r)
		result, err := store.PilotStatus(r.Context(), p)
		respond(w, result, err)
	})
	pilot.HandleFunc("POST /api/pilot/activate", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			OperationID  string `json:"operationId"`
			InvitationID string `json:"invitationId"`
			Confirm      bool   `json:"confirm"`
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
		if err != nil || !accountstore.StrictJSON(raw) {
			respond(w, nil, accountstore.ErrValidation)
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			respond(w, nil, accountstore.ErrValidation)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		result, err := store.ActivatePilot(r.Context(), p, input.OperationID, input.InvitationID, input.Confirm)
		respond(w, result, err)
	})
	mux.Handle("/api/pilot/", service.Protect(pilotAccount(pilot)))
	routes := map[string]string{
		"/api/application-draft":  "application-draft",
		"/api/qualification-gaps": "qualification-gaps",
		"/api/cv/generate":        "cv-generation",
		"/api/profile/ingest":     "profile-ingestion",
		"/api/profile/review":     "profile-review",
	}
	for path, name := range routes {
		next := workflows[path]
		if next == nil {
			next = auth.UnavailableHandler()
		}
		right := accountstore.PilotStandalone
		if path == "/api/application-draft" {
			right = accountstore.PilotDraft
		}
		guarded := protectPilotWorkflow(service, store, right, ProtectWorkflow(service, store.PilotProfiles(), path, next))
		mux.Handle(path, guarded)
		mux.Handle("/.netlify/functions/"+name, guarded)
	}
	// Account Function aliases pass through this same router, never a legacy handler.
	mux.Handle("/.netlify/functions/account/", http.StripPrefix("/.netlify/functions/account", mux))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ops := store.LocalOperationsObserver()
		path := strings.TrimPrefix(r.URL.Path, "/.netlify/functions/account")
		var fault accountstore.OperationsFault
		switch {
		case strings.HasPrefix(path, "/api/auth/"):
			fault = accountstore.OperationsAuthFault
		case path == "/api/pilot/status" || path == "/api/pilot/activate":
			fault = accountstore.OperationsAuthorityFault
		}
		if ops == nil || fault == "" {
			mux.ServeHTTP(w, r)
			return
		}
		observation := &operationsResponse{ResponseWriter: w}
		mux.ServeHTTP(observation, r)
		if observation.failure {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			_ = ops.ObserveFault(ctx, fault)
		}
	})
}

type operationsResponse struct {
	http.ResponseWriter
	wroteHeader bool
	failure     bool
}

func (w *operationsResponse) WriteHeader(status int) {
	if !w.wroteHeader {
		w.wroteHeader, w.failure = true, status >= 500
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *operationsResponse) Write(data []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(data)
}

func (w *operationsResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func pilotAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromRequest(r)
		if r.Header.Get("X-CareerOS-Account") != p.AccountID {
			workflowError(w, http.StatusConflict, "account_changed")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func protectPilotWorkflow(service *auth.Service, store *accountstore.Store, right accountstore.PilotRight, next http.Handler) http.Handler {
	return service.Protect(pilotAccount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		if err := store.AuthorizePilot(r.Context(), p, right); err != nil {
			respond(w, nil, err)
			return
		}

		result := httptest.NewRecorder()
		next.ServeHTTP(result, r)
		if err := store.AuthorizePilot(r.Context(), p, right); err != nil {
			respond(w, nil, err)
			return
		}
		started := false
		err := aidispatch.WriteResult(r.Context(), func(ctx context.Context) error {
			for key, values := range result.Header() {
				w.Header()[key] = values
			}
			started = true
			w.WriteHeader(result.Code)
			_, err := io.Copy(resultWriter{ctx, w}, result.Body)
			return err
		})
		if err != nil && !started {
			workflowError(w, http.StatusServiceUnavailable, "managed_result_unavailable")
		}
	})))
}
