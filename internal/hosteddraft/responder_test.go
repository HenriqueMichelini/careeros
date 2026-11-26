package hosteddraft

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/profiledocument"
)

type fixtureController func(context.Context, string, *http.Request) (*http.Response, error)

func (c fixtureController) Send(ctx context.Context, provider string, req *http.Request) (*http.Response, error) {
	return c(ctx, provider, req)
}

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPinnedFakeDraftResponsesPassCanonicalReview(t *testing.T) {
	const origin = "https://attestor.fixture.test"
	token := sha256.Sum256([]byte("synthetic private attestor test token"))
	var stages []string
	handler, err := attestorHandler(token, func(_ context.Context, a attestation, bindings attestationBindings) (bool, error) {
		if !strings.Contains(attestationSQL, hex.EncodeToString(bindings.result[:])) {
			t.Fatalf("SQL fixture digest disagrees for %s", a.Stage)
		}
		stages = append(stages, a.Stage)
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := fixtureTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != origin+attestationPath || req.Header.Get("X-Serverless-Authorization") != "Bearer fixture-identity" {
			t.Fatal("fixture transport received unpinned or unauthenticated request")
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result.Result(), nil
	})
	client, err := NewAttestorClientWithHTTPClient(origin, token, func(context.Context) (string, error) { return "fixture-identity", nil }, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	respond := NewFakeResponder(client)
	controller := fixtureController(func(ctx context.Context, provider string, req *http.Request) (*http.Response, error) {
		// The managed controller explicitly installs the standard tier before
		// claiming durable intent. Reproduce that byte-level handoff here.
		if provider == "openai" {
			var fields map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&fields); err != nil {
				t.Fatal(err)
			}
			_ = req.Body.Close()
			fields["service_tier"] = json.RawMessage(`"standard"`)
			body, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		stage := aidiagnostics.StageName(ctx)
		res, err := respond(ctx, aicontrol.HostedDraftCall{
			Namespace: fakeNamespace, DispatchID: "dispatch_" + stage, ActionID: "fixture_action", Stage: stage,
			Provider: provider, RequestDigest: sha256.Sum256([]byte(stage)),
			HandoffNonce: sha256.Sum256([]byte("handoff:" + stage)), Request: req,
		})
		if err != nil {
			t.Errorf("fake %s: %v", stage, err)
		}
		return res, err
	})
	document := fixtureProfile(t)
	ctx := aidispatch.WithController(aidispatch.WithWorkflowRoot(t.Context(), "application_draft", []byte(`{}`)), controller)
	result, err := applicationdraft.GenerateManaged(ctx, applicationdraft.ManagedInput{
		Profile: document, Posting: fixturePosting, Language: "en", FullName: "Fixture Career",
		ResumeNotNeeded: func(context.Context) error { t.Fatal("resume review did not run"); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := result.CompleteSnapshot("en", "Fixture Career")
	if err != nil || snapshot.RoleSummary != fixturePosting || !strings.Contains(snapshot.Resume, "Java") || !strings.Contains(snapshot.CoverLetter, fixtureLetter) {
		t.Fatalf("pinned Draft snapshot incomplete: %v", err)
	}
	if !slices.Equal(stages, []string{"field_validation", "generation", "resume_support", "artifact_support"}) {
		t.Fatalf("attested stages = %v", stages)
	}
}

func TestAttestationRejectsTamperingAndUnpinnedResult(t *testing.T) {
	key := sha256.Sum256([]byte("synthetic request signature key"))
	handler, err := attestorHandler(key, func(context.Context, attestation, attestationBindings) (bool, error) {
		t.Fatal("invalid request reached the recorder")
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := fixtureResponse("generation")
	if err != nil {
		t.Fatal(err)
	}
	proof := dispatchAttestation(aicontrol.HostedDraftCall{
		Namespace: fakeNamespace, DispatchID: "known_dispatch", ActionID: "known_action", Stage: "generation",
		Provider: "openai", RequestDigest: sha256.Sum256([]byte("durable binding")), HandoffNonce: sha256.Sum256([]byte("live handoff")),
	}, sha256.Sum256(content))
	canonical, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	result := sha256.Sum256([]byte("substituted response"))
	proof.ResultDigest = hex.EncodeToString(result[:])
	modified, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		signed []byte
		status int
	}{
		{name: "tampered signed body", signed: canonical, status: http.StatusUnauthorized},
		{name: "signed unpinned digest", signed: modified, status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, attestationPath, bytes.NewReader(modified))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", attestationAuthorization(key, test.signed))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}

func fixtureProfile(t *testing.T) profiledocument.Document {
	t.Helper()
	raw, err := os.ReadFile("../profiledocument/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct{ Document json.RawMessage }
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("Profile fixtures unavailable: %v", err)
	}
	doc, err := profiledocument.Decode(fixtures[0].Document)
	if err != nil {
		t.Fatal(err)
	}
	doc.Entities = []profiledocument.Entity{}
	doc.Facts[0].Context = []profiledocument.Reference{}
	doc.Facts[0].Value = json.RawMessage(`"Java"`)
	doc.Facts[0].Assertion, doc.Facts[0].Intent, doc.Facts[0].Certainty = "affirmed", "actual", "certain"
	doc.Facts[0].Normalization = profiledocument.Normalization{Observed: "Java", Policy: "exact-alias-v1"}
	doc.Facts[0].Temporal = profiledocument.Temporal{Precision: "unknown"}
	doc.Evidence[0].Excerpt = "I use Java."
	doc.Links = doc.Links[:1]
	return doc
}
