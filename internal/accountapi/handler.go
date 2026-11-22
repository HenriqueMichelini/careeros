// Package accountapi exposes the narrow new-account Profile boundary.
package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/workflowauth"
)

func New(service *auth.Service, store *accountstore.Store) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/auth/", auth.NewHandler(service))
	var profile http.Handler = auth.UnavailableHandler()
	if service != nil && store != nil {
		routes := http.NewServeMux()
		routes.HandleFunc("GET /api/profile/export", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RawQuery != "" {
				respond(w, nil, accountstore.ErrValidation)
				return
			}
			p, _ := auth.PrincipalFromRequest(r)
			started := false
			err := store.WithProfileExport(r.Context(), p, func(raw []byte) error {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Content-Disposition", `attachment; filename="careeros-profile-v1.json"`)
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				started = true
				_, err := w.Write(raw)
				return err
			})
			if err != nil && !started {
				respond(w, nil, err)
			}
		})
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
	mux.Handle("/api/profile/export", profile)
	mux.Handle("/api/profile/current", profile)
	mux.Handle("/api/profile/operations/", profile)
	return mux
}

func respond(w http.ResponseWriter, result any, err error) { workflowauth.Respond(w, result, err) }

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
