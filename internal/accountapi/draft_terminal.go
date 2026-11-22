package accountapi

import (
	"encoding/json"
	"net/http"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
)

// NewSyntheticTerminalDraft is the explicit local managed+opening composition.
// It does not configure any hosted startup or external provider transport.
func NewSyntheticTerminalDraft(service *auth.Service, profiles *accountstore.Store, h *aicontrol.Harness, drafts *draftrecovery.Store) http.Handler {
	if !drafts.TerminalEnabled() {
		return auth.UnavailableHandler()
	}
	return NewSyntheticDraft(service, profiles, h, drafts)
}

func registerDraftTerminal(mux *http.ServeMux, store *draftrecovery.Store) {
	mux.HandleFunc("POST /api/drafts/{key}/cancel", func(w http.ResponseWriter, r *http.Request) {
		var command struct {
			Action draftrecovery.Action `json:"action"`
		}
		if !decodeOpening(r, &command) {
			draftRecoveryError(w, draftrecovery.ErrInvalid)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		key := r.PathValue("key")
		if err := store.Cancel(r.Context(), p, key, command.Action); err != nil {
			draftRecoveryError(w, err)
			return
		}
		// Keep the protected write under the existing final authority fence.
		// A command acknowledgment is 200 even when its Result is terminal;
		// ordinary subsequent GET retains the existing 410 status contract.
		if err := store.WithResult(r.Context(), p, key, func(result draftrecovery.Result) error {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			return json.NewEncoder(w).Encode(result)
		}); err != nil {
			draftRecoveryError(w, err)
		}
	})
}
