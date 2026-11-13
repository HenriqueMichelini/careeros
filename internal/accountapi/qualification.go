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

	qualificationgaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

// NewSyntheticQualification is an explicit local-only composition. The Harness
// invokes only its in-memory fake responder; no environment or production router
// selects this function. References below satisfy the existing adapter contract
// and never represent actual credentials or reach a network client.
func NewSyntheticQualification(service *auth.Service, store *accountstore.Store, h *aicontrol.Harness) http.Handler {
	return newSyntheticQualification(service, store, h, nil)
}

type qualificationReceipt func(context.Context, auth.Principal, string, []byte, []byte) (string, error)

func newSyntheticQualification(service *auth.Service, store *accountstore.Store, h *aicontrol.Harness, receipt qualificationReceipt) http.Handler {
	if service == nil || store == nil || !h.QualificationEnabled() {
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
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
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
		session, err := h.Open(r.Context(), p, key, "qualification_gaps", raw)
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
		status, err := h.QualificationStatus(r.Context(), p, key)
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
		urlCopy.Path = "/api/qualification-gaps"
		request.URL = &urlCopy
		request.Header = r.Header.Clone()
		request.Header.Set("X-OpenAI-Api-Key", "sk-local-managed-reference")
		request.Header.Set("X-TypeSafe-Api-Key", "local-managed-reference")
		request.Body = io.NopCloser(bytes.NewReader(raw))
		request.ContentLength = int64(len(raw))
		result := httptest.NewRecorder()
		qualificationgaps.NewHandler().ServeHTTP(result, request)
		finishErr := session.FinishQualification(r.Context(), result.Code >= 400)
		if authority.failure != nil {
			qualificationError(w, authority.failure)
			return
		}
		if state, e := h.QualificationStatus(r.Context(), p, key); e == nil && state.RetryAllowed {
			w.Header().Set("X-CareerOS-Retry-Allowed", "true")
		}
		for k, v := range result.Header() {
			w.Header()[k] = v
		}
		if receipt != nil && finishErr == nil && result.Code == http.StatusOK && fields["jobContext"] == nil {
			if proof, err := receipt(r.Context(), p, session.ID(), raw, result.Body.Bytes()); err == nil {
				w.Header().Set("X-CareerOS-Draft-Context-Proof", proof)
			}
		}
		w.Header().Set("X-CareerOS-Action", session.ID())
		w.WriteHeader(result.Code)
		_, _ = io.Copy(w, result.Body)
	})
	base := NewPilot(service, store, map[string]http.Handler{"/api/qualification-gaps": qualified})
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
		result, err := h.QualificationStatus(r.Context(), p, r.PathValue("key"))
		if err != nil {
			qualificationError(w, err)
			return
		}
		respond(w, result, nil)
	})))
	mux := http.NewServeMux()
	mux.Handle("/api/ai/qualification/actions/{key}", status)
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

type qualificationAuthorityKey struct{}
type qualificationAuthority struct {
	session *aicontrol.Session
	failure error
}

func (a *qualificationAuthority) BindStage(ctx context.Context, coordinate string, input []byte) (aidispatch.StageReceipt, error) {
	if a.session == nil {
		return aidispatch.StageReceipt{}, aicontrol.ErrDenied
	}
	return a.session.BindStage(ctx, coordinate, input)
}
func (a *qualificationAuthority) ClarificationRevision() string {
	if a.session == nil {
		return ""
	}
	return a.session.ClarificationRevision()
}

func (a *qualificationAuthority) Send(ctx context.Context, provider string, r *http.Request) (*http.Response, error) {
	if a.session == nil {
		return nil, aicontrol.ErrDenied
	}
	response, err := a.session.Send(ctx, provider, r)
	if errors.Is(err, aicontrol.ErrCapacity) || errors.Is(err, aicontrol.ErrConflict) || errors.Is(err, aicontrol.ErrDenied) {
		a.failure = err
	}
	return response, err
}
func (a *qualificationAuthority) BeginResult(ctx context.Context) (context.Context, func(), error) {
	if a.session == nil {
		return ctx, func() {}, nil
	}
	return a.session.BeginResult(ctx)
}
func (a *qualificationAuthority) ResultAllowed(ctx context.Context) error {
	if a.session == nil {
		return nil
	}
	return a.session.ResultAllowed(ctx)
}

func qualificationError(w http.ResponseWriter, err error) {
	code, status := "managed_unavailable", http.StatusServiceUnavailable
	if errors.Is(err, aicontrol.ErrCapacity) {
		code, status = "qualification_paused", http.StatusTooManyRequests
	}
	if errors.Is(err, aicontrol.ErrConflict) {
		code, status = "qualification_changed", http.StatusConflict
	}
	if errors.Is(err, aicontrol.ErrDenied) {
		code, status = "qualification_denied", http.StatusForbidden
	}
	workflowError(w, status, code)
}
