package accountapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
)

// These routes exist only when the explicit disposable opening repository was
// injected. Normal controlled/managed/default compositions stay unactivated.
func registerDraftOpening(mux *http.ServeMux, store *draftrecovery.Store) {
	mux.HandleFunc("GET /api/draft-allowance", func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromRequest(r)
		v, err := store.Allowance(r.Context(), p)
		if err != nil {
			draftRecoveryError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(v)
	})
	mux.HandleFunc("POST /api/drafts/{key}/versions/{version}/opening", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("version") != "1" {
			draftRecoveryError(w, draftrecovery.ErrNotFound)
			return
		}
		var fields map[string]json.RawMessage
		if !decodeOpening(r, &fields) || fields == nil || len(fields) != 0 {
			draftRecoveryError(w, draftrecovery.ErrInvalid)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		v, err := store.IssueOpening(r.Context(), p, r.PathValue("key"))
		if err != nil {
			draftRecoveryError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(v)
	})
	mux.HandleFunc("POST /api/drafts/{key}/versions/{version}/opening/confirm", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("version") != "1" {
			draftRecoveryError(w, draftrecovery.ErrNotFound)
			return
		}
		var command draftrecovery.OpeningConfirmation
		if !decodeOpening(r, &command) {
			draftRecoveryError(w, draftrecovery.ErrInvalid)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		if err := store.ConfirmOpening(r.Context(), p, r.PathValue("key"), command); err != nil {
			draftRecoveryError(w, err)
			return
		}
		writeDraftResult(store, w, r, r.PathValue("key"), false)
	})
}

func decodeOpening(r *http.Request, out any) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8193))
	if err != nil || len(raw) > 8192 || !accountstore.StrictJSON(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out) == nil
}
