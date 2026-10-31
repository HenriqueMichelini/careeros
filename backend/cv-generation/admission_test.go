package cvgeneration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/testsupport/aiadmission"
)

// These outcomes pass through the real shared send adapter and CV completion
// parser. Provider success and parsable prose cannot establish a usable result,
// financial proof, a customer debit, or permission to resend a durable dispatch.
func TestA09_ActualHandlerFailureOutcomesKeepReachedObligations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{"malformed_envelope", 200, `{`, nil},
		{"malformed_content", 200, `{"choices":[{"finish_reason":"stop","message":{"content":"{"}}]}`, nil},
		{"refused", 200, `{"choices":[{"finish_reason":"stop","message":{"refusal":"synthetic refusal","content":"{}"}}]}`, nil},
		{"truncated", 200, `{"choices":[{"finish_reason":"length","message":{"content":"{\"summary\":[],\"selected\":[]}"}}]}`, nil},
		{"rate_limited", 429, `{}`, nil},
		{"provider_outage", 503, `{}`, nil},
		{"provider_timeout", 0, "", context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := aiadmission.NewFixture(t)
			h := aiadmission.NewHarness(t, f, "failure_outcomes")
			path := "/api/cv/generate"
			body := f.Body(t, path)
			s, err := h.Open(context.Background(), f.Principal, tc.name, "cv_generation", body)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			h.SetResponder(func(_ context.Context, provider string, request *http.Request) (*http.Response, error) {
				calls++
				if provider != "openai" || request.Header.Get("Authorization") != "" {
					t.Fatal("unexpected provider or leaked credential")
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			handler := accountapi.ProtectWorkflow(f.Service, f.Store, path, NewHandler())
			invoke := func() *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, f.Request(t, aidispatch.WithController(context.Background(), s), path, body))
				return w
			}
			if w := invoke(); w.Code < 400 || calls != 1 {
				t.Fatalf("failed provider output became success or retried: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
			}
			if w := invoke(); w.Code < 400 || calls != 1 {
				t.Fatalf("durable failed send was repeated: status=%d calls=%d", w.Code, calls)
			}
			status, err := h.Status(context.Background(), f.Principal, s.ID())
			if err != nil || len(status.Dispatches) != 1 || status.CustomerReserved || status.CustomerEffect {
				t.Fatalf("failure changed a customer right: %v %+v", err, status)
			}
			d := status.Dispatches[0]
			if d.IntentAt == nil || d.Counted != 60 || d.Maximum != 60 || d.Remote != "possibly_active" {
				t.Fatalf("uncertain provider outcome released an obligation: %+v", d)
			}
			scope, err := h.ScopeStatus(context.Background(), f.Principal, "global")
			if err != nil || scope.Counted != 60 || scope.Occupied != 1 {
				t.Fatalf("uncertain provider exposure changed: %v %+v", err, scope)
			}
		})
	}
}
