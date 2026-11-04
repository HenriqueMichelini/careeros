package workflowauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/qualificationmatching"
)

// ProtectWorkflow protects every entrypoint, independently of frontend build
// flags. The account header fences a queued request after cookies switch users;
// only the verified session can authorize a Profile read.
func Protect(service *auth.Service, store *accountstore.Store, path string, next http.Handler) http.Handler {
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
				Respond(w, nil, err)
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
				Respond(w, nil, accountstore.ErrValidation)
				return
			}
			evidence := qualificationmatching.ApprovedProjection(doc)
			if !qualificationmatching.HasUsefulFacts(evidence) {
				workflowError(w, 422, "profile_required")
				return
			}
			repo, qualifications := CareerProjection(doc)
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
			Respond(w, nil, err)
			return
		}
		if bound {
			current, err := store.ReadProfile(r.Context(), p, "")
			if err != nil {
				Respond(w, nil, err)
				return
			}
			if !bytes.Equal(current.Document, snapshot.Document) {
				workflowError(w, 409, "profile_stale")
				return
			}
		}
		started := false
		err := aidispatch.WriteResult(r.Context(), func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
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
	}))
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

func Respond(w http.ResponseWriter, result any, err error) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err == nil {
		json.NewEncoder(w).Encode(result)
		return
	}
	code, status := "storage_unavailable", http.StatusServiceUnavailable
	switch {
	case errors.Is(err, accountstore.ErrConflict):
		code, status = "stale", http.StatusConflict
	case errors.Is(err, accountstore.ErrValidation):
		code, status = "validation", http.StatusUnprocessableEntity
	case errors.Is(err, accountstore.ErrNotFound):
		code, status = "not_found", http.StatusNotFound
	case errors.Is(err, auth.ErrAuthenticationRequired):
		code, status = "authentication_required", http.StatusUnauthorized
	case errors.Is(err, auth.ErrCapacity):
		code, status = "pilot_capacity", http.StatusConflict
	case errors.Is(err, auth.ErrForbidden):
		code, status = "forbidden", http.StatusForbidden
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}
