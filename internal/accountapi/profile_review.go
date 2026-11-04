package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	profilereview "professional-information-repo/backend/profile-review"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

// NewSyntheticProfileReview is an explicit local-only composition. The Harness
// invokes only its in-memory fake responder; no environment or production router
// selects this function. References below satisfy the existing adapter contract
// and never represent actual credentials or reach a network client.
func NewSyntheticProfileReview(service *auth.Service, store *accountstore.Store, h *aicontrol.Harness, signingKey []byte) http.Handler {
	if service == nil || store == nil || !h.ProfileReviewEnabled() || len(signingKey) < 32 {
		return auth.UnavailableHandler()
	}
	qualified := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromRequest(r)
		for name := range r.Header {
			lower := strings.ToLower(name)
			if strings.Contains(lower, "api-key") || lower == "authorization" || lower == "x-careeros-dispatch" {
				workflowError(w, 400, "caller_credentials_forbidden")
				return
			}
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil || !accountstore.StrictJSON(raw) {
			workflowError(w, 400, "input")
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			workflowError(w, 400, "input")
			return
		}
		for _, key := range []string{"apiKey", "typeSafeApiKey", "typesafeApiKey", "openaiApiKey", "openAIKey", "headers", "authorization", "attempt", "dispatchId"} {
			if _, ok := fields[key]; ok {
				workflowError(w, 400, "caller_credentials_forbidden")
				return
			}
		}
		key := r.Header.Get("X-CareerOS-Action-Key")
		session, err := h.Open(r.Context(), p, key, "profile_review", raw)
		if err != nil {
			qualificationError(w, err)
			return
		}
		if retry := r.Header.Get("X-CareerOS-Retry-Key"); retry != "" {
			session, err = session.RetryQualification(r.Context(), retry)
			if err != nil {
				qualificationError(w, err)
				return
			}
		}
		status, err := h.ProfileReviewStatus(r.Context(), p, key)
		if err != nil {
			qualificationError(w, err)
			return
		}
		if status.State != "ready" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(status)
			return
		}
		if err = session.BeginProfileReview(r.Context()); err != nil {
			qualificationError(w, err)
			return
		}
		ctx := aidispatch.WithController(r.Context(), session)
		authority, _ := r.Context().Value(qualificationAuthorityKey{}).(*qualificationAuthority)
		if authority == nil {
			qualificationError(w, aicontrol.ErrDenied)
			return
		}
		authority.session = session
		ctx = aidispatch.WithController(r.Context(), authority)
		request := r.Clone(ctx)
		urlCopy := *r.URL
		urlCopy.Path = "/api/profile/review"
		request.URL = &urlCopy
		request.Header = r.Header.Clone()
		request.Header.Set("X-OpenAI-Api-Key", "sk-local-managed-reference")
		request.Header.Set("X-TypeSafe-Api-Key", "local-managed-reference")
		request.Body = io.NopCloser(bytes.NewReader(raw))
		request.ContentLength = int64(len(raw))
		result := httptest.NewRecorder()
		profilereview.NewHandler().ServeHTTP(result, request)
		_ = session.FinishQualification(r.Context(), result.Code >= 400)
		if authority.failure != nil {
			qualificationError(w, authority.failure)
			return
		}
		if state, e := h.ProfileReviewStatus(r.Context(), p, key); e == nil && state.RetryAllowed {
			w.Header().Set("X-CareerOS-Retry-Allowed", "true")
		}
		for k, v := range result.Header() {
			w.Header()[k] = v
		}
		w.Header().Set("X-CareerOS-Action", session.ID())
		w.WriteHeader(result.Code)
		_, _ = io.Copy(w, result.Body)
	})
	protected := protectPilotWorkflow(service, store, accountstore.PilotStandalone, ProtectWorkflow(service, store.PilotProfiles(), "/api/profile/review", qualified))
	base := NewSyntheticIngestion(service, store, h, signingKey)
	status := service.Protect(pilotAccount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		if err := store.AuthorizePilot(r.Context(), p, accountstore.PilotStandalone); err != nil {
			respond(w, nil, err)
			return
		}
		result, err := h.ProfileReviewStatus(r.Context(), p, r.PathValue("key"))
		if err != nil {
			qualificationError(w, err)
			return
		}
		respond(w, result, nil)
	})))
	mux := http.NewServeMux()
	mux.Handle("/api/profile/review", protected)
	mux.Handle("/.netlify/functions/profile-review", protected)
	mux.Handle("/api/ai/profile-review/actions/{key}", status)
	mux.Handle("/", base)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authority := &qualificationAuthority{}
		r = r.WithContext(aidispatch.WithController(context.WithValue(r.Context(), qualificationAuthorityKey{}, authority), authority))
		if strings.HasPrefix(r.URL.Path, "/.netlify/functions/account/") {
			clone := r.Clone(r.Context())
			u := *r.URL
			u.Path = strings.TrimPrefix(u.Path, "/.netlify/functions/account")
			clone.URL = &u
			r = clone
		}
		mux.ServeHTTP(w, r)
	})
}
