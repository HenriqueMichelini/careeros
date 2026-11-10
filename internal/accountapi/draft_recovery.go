package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
)

// NewControlledDraftRecovery is an explicit local composition. No environment
// or Function startup mounts it. Its injected provider is an in-memory controlled
// function, never a caller credential or a provider network client.
func NewControlledDraftRecovery(service *auth.Service, store *draftrecovery.Store, provider draftrecovery.ControlledProvider) http.Handler {
	if service == nil || store == nil || provider == nil {
		return auth.UnavailableHandler()
	}
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, r *http.Request, key string, created bool) {
		p, _ := auth.PrincipalFromRequest(r)
		err := store.WithResult(r.Context(), p, key, func(result draftrecovery.Result) error {
			status := http.StatusOK
			if result.State == "pending" || result.State == "incomplete" {
				status = http.StatusAccepted
			} else if result.State == "closed" || result.State == "deleted" {
				status = http.StatusGone
			} else if created {
				status = http.StatusCreated
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(status)
			return json.NewEncoder(w).Encode(result)
		})
		if err != nil {
			draftRecoveryError(w, err)
		}
	}
	mux.HandleFunc("POST /api/drafts", func(w http.ResponseWriter, r *http.Request) {
		for name := range r.Header {
			lower := strings.ToLower(name)
			if strings.Contains(lower, "api-key") || lower == "authorization" || lower == "x-careeros-dispatch" {
				draftRecoveryError(w, draftrecovery.ErrInvalid)
				return
			}
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
		if err != nil || !accountstore.StrictJSON(raw) {
			draftRecoveryError(w, draftrecovery.ErrInvalid)
			return
		}
		var in draftrecovery.Input
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil {
			draftRecoveryError(w, draftrecovery.ErrInvalid)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		key := r.Header.Get("X-CareerOS-Action-Key")
		result, g, winning, err := store.Create(r.Context(), p, key, in)
		if err != nil {
			draftRecoveryError(w, err)
			return
		}
		if winning {
			// A browser disconnect is not an authoritative cancellation. A complete
			// controlled result may still publish under fresh authority for recovery.
			deadline := time.Now().Add(25 * time.Second)
			if result.Deadline.Before(deadline) {
				deadline = result.Deadline
			}
			ctx, cancel := context.WithDeadline(context.WithoutCancel(r.Context()), deadline)
			bundle, generateErr := provider(ctx, g)
			var publication draftrecovery.Publication
			if generateErr == nil {
				publication, generateErr = store.Prepare(ctx, p, key, bundle)
			}
			if generateErr == nil {
				generateErr = ctx.Err()
			}
			cancel()
			publishCtx, done := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			if generateErr == nil {
				err = store.Publish(publishCtx, p, key, publication)
			} else {
				err = generateErr
			}
			if err != nil {
				_ = store.Incomplete(publishCtx, p, key)
				done()
				draftRecoveryError(w, err)
				return
			}
			done()
		}
		write(w, r, key, winning)
	})
	mux.HandleFunc("GET /api/drafts/{key}", func(w http.ResponseWriter, r *http.Request) { write(w, r, r.PathValue("key"), false) })
	mux.HandleFunc("GET /api/drafts/{key}/versions/{version}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("version") != "1" {
			draftRecoveryError(w, draftrecovery.ErrNotFound)
			return
		}
		write(w, r, r.PathValue("key"), false)
	})
	protected := service.Protect(pilotAccount(mux))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.netlify/functions/account/") {
			http.StripPrefix("/.netlify/functions/account", protected).ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
func draftRecoveryError(w http.ResponseWriter, err error) {
	code, status := "draft_unavailable", http.StatusServiceUnavailable
	switch {
	case errors.Is(err, draftrecovery.ErrInvalid):
		code, status = "draft_incomplete", 422
	case errors.Is(err, draftrecovery.ErrConflict):
		code, status = "draft_stale", 409
	case errors.Is(err, draftrecovery.ErrNotFound):
		code, status = "draft_not_found", 404
	case errors.Is(err, auth.ErrAuthenticationRequired):
		code, status = "authentication_required", 401
	case errors.Is(err, draftrecovery.ErrDenied), errors.Is(err, auth.ErrForbidden):
		code, status = "draft_denied", 403
	}
	workflowError(w, status, code)
}
