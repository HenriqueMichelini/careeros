package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/qualificationmatching"
)

// ProtectWorkflow protects every entrypoint, independently of frontend build
// flags. The account header fences a queued request after cookies switch users;
// only the verified session can authorize a Profile read.
func ProtectWorkflow(service *auth.Service, store *accountstore.Store, path string, next http.Handler) http.Handler {
	if service == nil || store == nil {
		return auth.UnavailableHandler()
	}
	return service.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromRequest(r)
		if r.Header.Get("X-CareerOS-Account") != p.AccountID {
			workflowError(w, http.StatusConflict, "account_changed")
			return
		}
		bound := path == "/api/application-draft" || path == "/api/qualification-gaps"
		var snapshot accountstore.ProfileResult
		if bound {
			var err error
			snapshot, err = store.ReadProfile(r.Context(), p, "")
			if err != nil {
				respond(w, nil, err)
				return
			}
			if len(snapshot.Document) == 0 || string(snapshot.Document) == "null" {
				workflowError(w, http.StatusUnprocessableEntity, "profile_required")
				return
			}
			raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
			if err != nil || !accountstore.StrictJSON(raw) {
				workflowError(w, 400, "input")
				return
			}
			var input map[string]json.RawMessage
			if json.Unmarshal(raw, &input) != nil || input == nil {
				workflowError(w, 400, "input")
				return
			}
			doc, err := profiledocument.Decode(snapshot.Document)
			if err != nil {
				respond(w, nil, accountstore.ErrValidation)
				return
			}
			evidence := qualificationmatching.ApprovedProjection(doc)
			if !qualificationmatching.HasUsefulFacts(evidence) {
				workflowError(w, 422, "profile_required")
				return
			}
			repo, qualifications := workflowProfile(doc)
			// Exact snapshot consistency, including career view and approved evidence.
			// Foreign IDs, changed facts and stale revisions never reach inference.
			if !sameJSON(input["profileEvidence"], evidence) || !sameJSON(input["repository"], repo) || !sameJSON(input["qualifications"], qualifications) {
				workflowError(w, http.StatusConflict, "profile_stale")
				return
			}
			if path == "/api/application-draft" {
				input["reviewResume"] = json.RawMessage("true")
				input["reviewArtifacts"] = json.RawMessage("true")
			}
			raw, _ = json.Marshal(input)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			r.ContentLength = int64(len(raw))
		}
		// Do not expose a late result after sign-out, expiry or account invalidation.
		// No database lock is held over inference; this is transient result delivery,
		// not an atomic durable Draft publication or cancellation receipt.
		result := httptest.NewRecorder()
		next.ServeHTTP(result, r)
		if r.Context().Err() != nil {
			return
		}
		if _, err := service.Revalidate(r.Context(), p); err != nil {
			respond(w, nil, err)
			return
		}
		if bound {
			current, err := store.ReadProfile(r.Context(), p, "")
			if err != nil {
				respond(w, nil, err)
				return
			}
			if !bytes.Equal(current.Document, snapshot.Document) {
				workflowError(w, 409, "profile_stale")
				return
			}
		}
		for key, values := range result.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(result.Code)
		_, _ = io.Copy(w, result.Body)
	}))
}

func sameJSON(raw []byte, expected any) bool {
	var a, b any
	encoded, err := json.Marshal(expected)
	return err == nil && json.Unmarshal(raw, &a) == nil && json.Unmarshal(encoded, &b) == nil && reflect.DeepEqual(a, b)
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
