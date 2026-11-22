package accountapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

func deletionRoutes(mux *http.ServeMux, store *accountstore.Store) {
	mux.HandleFunc("POST /api/deletion/operations", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil || !accountstore.StrictJSON(raw) {
			respond(w, nil, accountstore.ErrValidation)
			return
		}
		var in accountstore.DeletionRequest
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil {
			respond(w, nil, accountstore.ErrValidation)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		result, err := store.StartDeletion(r.Context(), p, in)
		if err != nil {
			respond(w, nil, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(result)
	})
	status := func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromRequest(r)
		started := false
		err := store.WithDeletionStatus(r.Context(), p, r.PathValue("operation"), func(v accountstore.DeletionStatus) error {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			started = true
			return json.NewEncoder(w).Encode(v)
		})
		if err != nil && !started {
			respond(w, nil, err)
		}
	}
	mux.HandleFunc("GET /api/deletion/operations/{operation}", status)
	mux.HandleFunc("GET /api/deletion/status", status)
}
