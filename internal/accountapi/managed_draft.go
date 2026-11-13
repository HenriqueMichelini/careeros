package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
)

// NewSyntheticDraft explicitly composes the existing pilot with local managed
// Draft generation. The outer router never buffers a protected Draft response
// or falls through to a caller-key Draft handler.
func NewSyntheticDraft(service *auth.Service, profiles *accountstore.Store, h *aicontrol.Harness, drafts *draftrecovery.Store) http.Handler {
	if service == nil || profiles == nil || !drafts.UsesManagedAuthority(h) {
		return auth.UnavailableHandler()
	}
	protected := http.NewServeMux()
	create := func(w http.ResponseWriter, r *http.Request) {
		deadline := time.Now().Add(25 * time.Second)
		requestCtx, cancelRequest := context.WithDeadline(r.Context(), deadline)
		defer cancelRequest()
		r = r.WithContext(requestCtx)
		for name := range r.Header {
			lower := strings.ToLower(name)
			if strings.Contains(lower, "api-key") || lower == "authorization" || lower == "x-careeros-dispatch" {
				workflowError(w, 400, "caller_credentials_forbidden")
				return
			}
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
		if err != nil || !accountstore.StrictJSON(raw) {
			draftRecoveryError(w, draftrecovery.ErrInvalid)
			return
		}
		var input struct {
			draftrecovery.Input
			JobContext         json.RawMessage `json:"jobContext"`
			QualificationProof string          `json:"qualificationProof"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			draftRecoveryError(w, draftrecovery.ErrInvalid)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		key := r.Header.Get("X-CareerOS-Action-Key")
		var generation draftrecovery.Generation
		var winning bool
		if retry := r.Header.Get("X-CareerOS-Retry-Key"); retry != "" {
			generation, winning, err = drafts.RetryManaged(r.Context(), p, key, retry, input.Input, input.JobContext, input.QualificationProof)
		} else {
			_, generation, winning, err = drafts.CreateManaged(r.Context(), p, key, input.Input, input.JobContext, input.QualificationProof)
		}
		if err != nil {
			draftRecoveryError(w, err)
			return
		}
		if winning {
			// A lost client reply is distinct from an authoritative cancellation.
			// Inference still has the database-anchored foreground bound.
			ctx, cancel := context.WithDeadline(context.WithoutCancel(r.Context()), deadline)
			defer cancel()
			publication, err := drafts.GenerateManaged(ctx, p, key, generation)
			if err == nil {
				err = drafts.Publish(ctx, p, key, publication)
			}
			if err != nil {
				finish, done := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
				defer done()
				_ = drafts.IncompleteManaged(finish, p, key, generation, err)
				draftRecoveryError(w, err)
				return
			}
		}
		writeDraftResult(drafts, w, r, key, winning)
	}
	protected.HandleFunc("POST /api/drafts", create)
	protected.HandleFunc("POST /api/application-draft", create)
	protected.HandleFunc("POST /.netlify/functions/application-draft", create)
	registerDraftReads(protected, drafts)
	draftHandler := service.Protect(pilotAccount(protected))
	base := newSyntheticQualification(service, profiles, h, drafts.QualificationProof)
	router := http.NewServeMux()
	for _, path := range []string{"/api/drafts", "/api/drafts/", "/api/application-draft", "/.netlify/functions/application-draft"} {
		router.Handle(path, draftHandler)
	}
	router.Handle("/.netlify/functions/account/", http.StripPrefix("/.netlify/functions/account", router))
	router.Handle("/", base)
	return router
}
