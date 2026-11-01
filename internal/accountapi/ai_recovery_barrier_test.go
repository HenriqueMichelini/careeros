package accountapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestA32ProtectedResultHandoffRejectsPreviouslyParsedResultAfterGateCloses(t *testing.T) {
	f, gate, _, session, body := resultFixture(t)
	// The downstream handler has already parsed a transient result. Closing the
	// independent recovery authority before protected handoff withholds it even
	// though the caller's real session/Profile remain current.
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"PRIVATE_PARSED_SYNTHETIC_RESULT"}`))
		gate.Close("result_handoff")
	})
	handler := accountapi.ProtectWorkflow(f.Service, f.Store, "/api/cv/generate", downstream)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, f.Request(t, aidispatch.WithController(context.Background(), session), "/api/cv/generate", body))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "PRIVATE_PARSED_SYNTHETIC_RESULT") || !strings.Contains(response.Body.String(), "managed_result_unavailable") {
		t.Fatalf("closed authority released protected content: %d %s", response.Code, response.Body.String())
	}
	gate.Reconcile("result_handoff", 1)
	if session.ResultAllowed(context.Background()) == nil {
		t.Fatal("same epoch reopening revived parsed result capability")
	}
}

func resultFixture(t *testing.T) (*aiadmission.Fixture, *aicontrol.SyntheticGate, *aicontrol.Harness, *aicontrol.Session, []byte) {
	t.Helper()
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
	return f, gate, h, session, body
}

type pausedResultWriter struct {
	*httptest.ResponseRecorder
	phase            string
	entered, release chan struct{}
}

func (w *pausedResultWriter) WriteHeader(status int) {
	if w.phase == "header" {
		close(w.entered)
		<-w.release
	}
	w.ResponseRecorder.WriteHeader(status)
}
func (w *pausedResultWriter) Write(raw []byte) (int, error) {
	if w.phase == "body" {
		close(w.entered)
		<-w.release
	}
	return w.ResponseRecorder.Write(raw)
}

func TestA32ProtectedResultWriteMustFinishBeforeSuccessfulDrain(t *testing.T) {
	for _, phase := range []string{"header", "body"} {
		t.Run(phase, func(t *testing.T) {
			f, gate, h, session, body := resultFixture(t)
			const payload = "PRIVATE_PENDING_SYNTHETIC_RESULT"
			downstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(payload)) })
			handler := accountapi.ProtectWorkflow(f.Service, f.Store, "/api/cv/generate", downstream)
			writer := &pausedResultWriter{httptest.NewRecorder(), phase, make(chan struct{}), make(chan struct{})}
			defer func() {
				select {
				case <-writer.release:
				default:
					close(writer.release)
				}
			}()
			done := make(chan struct{})
			go func() {
				defer close(done)
				handler.ServeHTTP(writer, f.Request(t, aidispatch.WithController(context.Background(), session), "/api/cv/generate", body))
			}()
			select {
			case <-writer.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("protected result did not reach the controlled writer")
			}
			// Cancel cannot recall bytes already handed to an underlying writer.
			// It must instead leave that writer visible until it actually exits.
			gate.Close("result_handoff")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			err := gate.Drain(ctx, "result_handoff")
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) || writer.Body.Len() != 0 {
				t.Fatalf("pending output was declared drained: %v", err)
			}
			gate.Reconcile("result_handoff", 1)
			if _, err := h.Open(context.Background(), f.Principal, "pending_reopen", "cv_generation", body); err == nil {
				t.Fatal("pending result permitted recovery reopening")
			}
			reviewer := h.SyntheticReviewer()
			if err := reviewer.RetireNamespace(context.Background(), f.Principal); err != nil {
				t.Fatal(err)
			}
			proof := aicontrol.SyntheticRetirementProof{Namespace: "result_handoff", Epoch: 1, CapabilitiesExpired: true, RestoreHorizonExpired: true, SendersExcluded: true, AllEntrypointsClosed: true}
			if err := reviewer.PruneNamespaceMarkers(context.Background(), f.Principal, proof); err == nil {
				t.Fatal("pending result permitted replay-marker pruning")
			}
			close(writer.release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("released writer did not finish")
			}
			ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := gate.Drain(ctx, "result_handoff"); err != nil {
				t.Fatal("completed result remained active", err)
			}
			if phase == "header" && writer.Body.Len() != 0 {
				t.Fatal("cancellation before body write released content")
			}
			if phase == "body" && writer.Body.String() != payload {
				t.Fatal("test did not exercise an already-entered body write")
			}
			// No callback or writer remains after the successful drain; this
			// local proof does not release remote capacity or imply delivery.
			var effects, sends int
			if err := f.DB.QueryRow(`SELECT (SELECT count(*) FROM ai_fixture_action WHERE customer_effect),(SELECT count(*) FROM ai_fixture_dispatch)`).Scan(&effects, &sends); err != nil || effects != 0 || sends != 0 {
				t.Fatal("result handoff invented customer/provider effects", effects, sends, err)
			}
		})
	}
}
