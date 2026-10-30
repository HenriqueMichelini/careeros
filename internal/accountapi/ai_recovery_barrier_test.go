package accountapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA32ProtectedResultHandoffRejectsPreviouslyParsedResultAfterGateCloses(t *testing.T) {
	f := aiadmission.NewFixture(t)
	gate := aicontrol.NewSyntheticGate()
	cfg := aicontrol.DefaultSyntheticConfig("result_handoff", gate)
	h, err := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	gate.Reconcile(cfg.Namespace, 1)
	if err = h.SyntheticReviewer().SeedGrant(context.Background(), f.Principal, cfg.ExpiresAt, cfg.CustomerUnits); err != nil {
		t.Fatal(err)
	}
	body := f.Body(t, "/api/cv/generate")
	session, err := h.Open(context.Background(), f.Principal, "parsed_result", "cv_generation", body)
	if err != nil {
		t.Fatal(err)
	}
	// The downstream handler has already parsed a transient result. Closing the
	// independent recovery authority before protected handoff withholds it even
	// though the caller's real session/Profile remain current.
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"PRIVATE_PARSED_SYNTHETIC_RESULT"}`))
		gate.Close(cfg.Namespace)
	})
	handler := accountapi.ProtectWorkflow(f.Service, f.Store, "/api/cv/generate", downstream)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, f.Request(t, aidispatch.WithController(context.Background(), session), "/api/cv/generate", body))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "PRIVATE_PARSED_SYNTHETIC_RESULT") || !strings.Contains(response.Body.String(), "managed_result_unavailable") {
		t.Fatalf("closed authority released protected content: %d %s", response.Code, response.Body.String())
	}
	gate.Reconcile(cfg.Namespace, 1)
	if session.ResultAllowed(context.Background()) == nil {
		t.Fatal("same epoch reopening revived parsed result capability")
	}
}
