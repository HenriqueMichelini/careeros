package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	cvgeneration "professional-information-repo/backend/cv-generation"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/cvsource"
)

// NewSyntheticStandaloneCV is an explicit local-only composition. The Harness
// invokes only its in-memory fake responder; no environment or production router
// selects this function. References below satisfy the existing adapter contract
// and never represent actual credentials or reach a network client.
func NewSyntheticStandaloneCV(service *auth.Service, store *accountstore.Store, h *aicontrol.Harness, signingKey []byte) http.Handler {
	if service == nil || store == nil || !h.StandaloneCVEnabled() || len(signingKey) < 32 {
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
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
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
		var assessed bool
		if json.Unmarshal(fields["statementSupport"], &assessed) != nil || !assessed {
			workflowError(w, 400, "assessment_required")
			return
		}
		var normalized bool
		raw, normalized = cvsource.Normalize(raw)
		if !normalized {
			workflowError(w, 400, "input")
			return
		}
		key := r.Header.Get("X-CareerOS-Action-Key")
		session, err := h.Open(r.Context(), p, key, "cv_generation", raw)
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
		status, err := h.StandaloneCVStatus(r.Context(), p, key)
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
		if err = session.BeginStandaloneCV(r.Context()); err != nil {
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
		urlCopy.Path = "/api/cv/generate"
		request.URL = &urlCopy
		request.Header = r.Header.Clone()
		request.Header.Set("X-OpenAI-Api-Key", "sk-local-managed-reference")
		request.Header.Set("X-TypeSafe-Api-Key", "local-managed-reference")
		request.Body = io.NopCloser(bytes.NewReader(raw))
		request.ContentLength = int64(len(raw))
		result := httptest.NewRecorder()
		cvgeneration.NewHandler().ServeHTTP(result, request)
		var outcome struct {
			SchemaVersion int `json:"schemaVersion"`
			Support       struct {
				Assessment string `json:"assessment"`
				Statements []struct {
					Status string `json:"status"`
				} `json:"statements"`
			} `json:"support"`
		}
		candidate := result.Code == 200 && json.Unmarshal(result.Body.Bytes(), &outcome) == nil && outcome.SchemaVersion == 3
		unresolved := candidate && outcome.Support.Assessment != "complete"
		for _, statement := range outcome.Support.Statements {
			unresolved = unresolved || statement.Status != "supported"
		}
		if e := session.FinishStandaloneCV(r.Context(), candidate, unresolved, result.Code >= 400); e != nil && !(candidate && errors.Is(e, aicontrol.ErrOutputUnavailable)) {
			qualificationError(w, e)
			return
		}
		if authority.failure != nil && !candidate {
			qualificationError(w, authority.failure)
			return
		}
		if state, e := h.StandaloneCVStatus(r.Context(), p, key); e == nil && state.RetryAllowed {
			w.Header().Set("X-CareerOS-Retry-Allowed", "true")
		}
		for k, v := range result.Header() {
			w.Header()[k] = v
		}
		w.Header().Set("X-CareerOS-Action", session.ID())
		w.WriteHeader(result.Code)
		_, _ = io.Copy(w, result.Body)
	})
	protected := protectPilotWorkflow(service, store, accountstore.PilotStandalone, ProtectWorkflow(service, store.PilotProfiles(), "/api/cv/generate", qualified))
	base := NewSyntheticProfileReview(service, store, h, signingKey)
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
		result, err := h.StandaloneCVStatus(r.Context(), p, r.PathValue("key"))
		if err != nil {
			qualificationError(w, err)
			return
		}
		respond(w, result, nil)
	})))
	mux := http.NewServeMux()
	mux.Handle("/api/cv/generate", protected)
	mux.Handle("/.netlify/functions/cv-generation", protected)
	mux.Handle("/api/ai/cv/actions/{key}", status)
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
