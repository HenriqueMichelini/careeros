package profileingestion

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagedHandlerWithoutAuthorityCannotUseLegacyClient(t *testing.T) {
	// The exported injected source path is closed even when called directly,
	// without its account composition. A provider-free plan needs a managed
	// binder too; a caller-supplied key can never select the legacy transport.
	in := request{Input: "I use Java.", Profile: profile(), Stage: &stageRequest{SubmissionStageVersion, SubmissionStagePlan}}
	r := httptest.NewRequest("POST", "/api/profile/ingest", strings.NewReader(string(mustJSON(in))))
	r.Header.Set("X-OpenAI-Api-Key", "sk-synthetic-reference")
	w := httptest.NewRecorder()
	NewManagedHandler([]byte("synthetic-managed-key-at-least-32bytes")).ServeHTTP(w, r)
	if w.Code < 400 {
		t.Fatalf("missing authority accepted: %d %s", w.Code, w.Body.String())
	}
}
