package accountapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	ingestion "professional-information-repo/backend/profile-ingestion"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

// NewSyntheticIngestion composes ingestion and qualification on the SAME
// Harness and shared scope rows. Only the injected fake responder can execute;
// no FromEnv/hosted path selects this constructor or enables actual egress.
func NewSyntheticIngestion(service *auth.Service, store *accountstore.Store, h *aicontrol.Harness, signingKey []byte) http.Handler {
	if service == nil || store == nil || !h.IngestionEnabled() || len(signingKey) < 32 {
		return auth.UnavailableHandler()
	}
	managed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil {
			workflowError(w, 400, "input")
			return
		}
		// These are envelope controls, never career-text matches. Managed
		// credentials, attempts and dispatch identities are server-owned.
		for _, key := range []string{"apiKey", "typeSafeApiKey", "typesafeApiKey", "openaiApiKey", "openAIKey", "headers", "authorization", "attempt", "dispatchId"} {
			if _, present := envelope[key]; present {
				workflowError(w, 400, "caller_credentials_forbidden")
				return
			}
		}
		root, coordinate, kind, ok := ingestion.ManagedEnvelope(raw)
		if !ok {
			workflowError(w, 400, "managed_ingestion_stage_closed")
			return
		}
		p, _ := auth.PrincipalFromRequest(r)
		session, err := h.Open(r.Context(), p, r.Header.Get("X-CareerOS-Action-Key"), "profile_ingestion", root)
		if err != nil {
			qualificationError(w, err)
			return
		}
		if retry := r.Header.Get("X-CareerOS-Retry-Key"); retry != "" {
			session, err = session.RetryIngestionStage(r.Context(), retry, coordinate, kind, raw)
			if err != nil {
				qualificationError(w, err)
				return
			}
		}
		stage, err := session.BeginIngestionStage(r.Context(), coordinate, kind, raw)
		if err != nil {
			qualificationError(w, err)
			return
		}
		if stage.State != "ready" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			_ = json.NewEncoder(w).Encode(aicontrol.QualificationStatus{ActionID: session.ID(), State: stage.State, RetryAllowed: stage.RetryAllowed})
			return
		}
		authority, _ := r.Context().Value(qualificationAuthorityKey{}).(*qualificationAuthority)
		if authority == nil {
			qualificationError(w, aicontrol.ErrDenied)
			return
		}
		authority.session = stage.Session
		request := r.Clone(aidispatch.WithController(r.Context(), authority))
		u := *r.URL
		u.Path = "/api/profile/ingest"
		request.URL = &u
		request.Header = r.Header.Clone()
		request.Header.Set("X-OpenAI-Api-Key", "sk-local-managed-reference")
		request.Header.Set("X-TypeSafe-Api-Key", "local-managed-reference")
		request.Body = io.NopCloser(bytes.NewReader(raw))
		request.ContentLength = int64(len(raw))
		// Receipt authority is purpose-separated and bound to this verified
		// account and immutable Action, including across its permitted retry.
		mac := hmac.New(sha256.New, signingKey)
		_, _ = mac.Write([]byte("managed-ingestion-receipts-v1\x00" + p.AccountID + "\x00" + session.ID()))
		workflow := ingestion.NewManagedHandler(mac.Sum(nil))
		result := httptest.NewRecorder()
		workflow.ServeHTTP(result, request)
		_ = stage.Session.FinishIngestionStage(r.Context(), result.Code >= 400)
		if authority.failure != nil {
			qualificationError(w, authority.failure)
			return
		}
		if status, e := h.IngestionStatus(r.Context(), p, r.Header.Get("X-CareerOS-Action-Key")); e == nil && status.RetryAllowed {
			w.Header().Set("X-CareerOS-Retry-Allowed", "true")
		}
		for k, v := range result.Header() {
			w.Header()[k] = v
		}
		w.Header().Set("X-CareerOS-Action", session.ID())
		w.WriteHeader(result.Code)
		_, _ = io.Copy(w, result.Body)
	})
	protected := protectPilotWorkflow(service, store, accountstore.PilotStandalone, ProtectWorkflow(service, store.PilotProfiles(), "/api/profile/ingest", managed))
	base := NewSyntheticQualification(service, store, h)
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
		v, e := h.IngestionStatus(r.Context(), p, r.PathValue("key"))
		if e != nil {
			qualificationError(w, e)
			return
		}
		respond(w, v, nil)
	})))
	mux := http.NewServeMux()
	mux.Handle("/api/profile/ingest", protected)
	mux.Handle("/.netlify/functions/profile-ingestion", protected)
	mux.Handle("/api/ai/ingestion/actions/{key}", status)
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
		if r.URL.Path == "/api/profile/ingest" || r.URL.Path == "/.netlify/functions/profile-ingestion" {
			ctx, cancel := context.WithTimeout(r.Context(), 27*time.Second)
			defer cancel()
			if r.Body != nil {
				stop := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
				defer stop()
			}
			r = r.WithContext(ctx)
		}
		mux.ServeHTTP(w, r)
	})
}
