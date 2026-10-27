// Package accountapi exposes the narrow new-account Profile boundary.
package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

func New(service *auth.Service, store *accountstore.Store) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/auth/", auth.NewHandler(service))
	var profile http.Handler = auth.UnavailableHandler()
	if service != nil && store != nil {
		routes := http.NewServeMux()
		routes.HandleFunc("GET /api/profile/current", func(w http.ResponseWriter, r *http.Request) {
			p, _ := auth.PrincipalFromRequest(r)
			result, err := store.ReadProfile(r.Context(), p, "")
			respond(w, result, err)
		})
		routes.HandleFunc("GET /api/profile/operations/{operation}", func(w http.ResponseWriter, r *http.Request) {
			p, _ := auth.PrincipalFromRequest(r)
			result, err := store.ReadProfile(r.Context(), p, r.PathValue("operation"))
			respond(w, result, err)
		})
		routes.HandleFunc("PUT /api/profile/current", func(w http.ResponseWriter, r *http.Request) {
			// Reject client ownership fields, ambiguous JSON and extra documents.
			raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, accountstore.MaxDocumentBytes+1024))
			if err != nil || !accountstore.StrictJSON(raw) {
				respond(w, nil, accountstore.ErrValidation)
				return
			}
			var input struct {
				OperationID      string          `json:"operationId"`
				ExpectedRevision int64           `json:"expectedRevision"`
				Document         json.RawMessage `json:"document"`
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&input); err != nil {
				respond(w, nil, accountstore.ErrValidation)
				return
			}
			p, _ := auth.PrincipalFromRequest(r)
			result, err := store.SaveProfile(r.Context(), p, input.OperationID, input.ExpectedRevision, input.Document)
			respond(w, result, err)
		})
		profile = service.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := auth.PrincipalFromRequest(r)
			// This is a client/account consistency precondition, never owner authority.
			// It fences requests queued by account A but sent after cookies switch to B.
			if r.Header.Get("X-CareerOS-Account") != p.AccountID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				io.WriteString(w, `{"error":"account_changed"}`)
				return
			}
			routes.ServeHTTP(w, r)
		}))
	}
	mux.Handle("/api/profile/current", profile)
	mux.Handle("/api/profile/operations/", profile)
	return mux
}

func respond(w http.ResponseWriter, result any, err error) {
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
	case errors.Is(err, auth.ErrForbidden):
		code, status = "forbidden", http.StatusForbidden
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

type Runtime struct {
	Service *auth.Service
	Store   *accountstore.Store
	Handler http.Handler
}

// FromEnv never substitutes process memory for account authority and never
// migrates/provisions storage. Credentials remain process-local.
func FromEnv(ctx context.Context) (*Runtime, error) {
	cfg, err := auth.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	store, err := accountstore.Open(ctx, os.Getenv("CAREEROS_DATABASE_URL"))
	if err != nil {
		return nil, err
	}
	provider, err := auth.NewGoogleProvider(cfg.Providers[0], cfg.HTTPClient, cfg.Now, cfg.JWKSCacheTTL)
	if err != nil {
		store.Close()
		return nil, err
	}
	registry, err := auth.NewRegistry(provider)
	if err != nil {
		store.Close()
		return nil, err
	}
	service, err := auth.NewService(cfg, registry, store.AuthStores())
	if err != nil {
		store.Close()
		return nil, err
	}
	return &Runtime{service, store, New(service, store)}, nil
}
