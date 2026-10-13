package applicationdraft_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	applicationdraft "professional-information-repo/backend/application-draft"
	"strings"
	"testing"
	"time"
)

const privateCaptureOrigin = "https://release.example"

type captureProviderTransport func(*http.Request) (*http.Response, error)

func (f captureProviderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func privateCaptureRequest(t *testing.T) (*http.Request, *releaseCaptureFixture) {
	t.Helper()
	fixture := &releaseCaptureFixture{t: t, variant: "literal", posting: "Desenvolvedor Ruby.\nMantenha ferramentas CLI internas usando Ruby.\nRuby é obrigatório.\nExplique como você usa Ruby em português."}
	req := fixture.request()
	req.Header.Set("Origin", privateCaptureOrigin)
	req.Header.Set("X-CareerOS-Release-Capture", "ruby-pt-draft")
	return req, fixture
}

func approvePrivateCapture(t *testing.T, req *http.Request) string {
	t.Helper()
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	manifest, _ := json.Marshal(map[string]any{
		"version": "release-capture-manifest-v1", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"cases": []any{map[string]any{
			"id": "ruby-pt-draft", "path": "/api/application-draft", "origin": privateCaptureOrigin,
			"requestSHA256": hash, "stages": []string{"field_validation", "generation", "resume_support", "artifact_support"},
		}},
	})
	t.Setenv("CAREEROS_RELEASE_CAPTURE", string(manifest))
	return hash
}

func TestApprovedSyntheticCaptureReturnsExactProviderBodiesOnlyToRequester(t *testing.T) {
	req, fixture := privateCaptureRequest(t)
	hash := approvePrivateCapture(t, req)
	var requests, responses []string
	client := &http.Client{Transport: captureProviderTransport(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		requests = append(requests, string(raw))
		response, err := fixture.RoundTrip(r)
		if response != nil {
			raw, _ = io.ReadAll(response.Body)
			response.Body = io.NopCloser(bytes.NewReader(raw))
			responses = append(responses, string(raw))
		}
		return response, err
	})}
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	var result struct {
		ReleaseCapture struct {
			Version, CaseID, RequestSHA256, Outcome string
			Envelopes                               []struct {
				Stage, RequestBody, ResponseBody        string
				RequestSHA256, ResponseSHA256           string
				RequestBytes, ResponseBytes, HTTPStatus int
			}
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	capture := result.ReleaseCapture
	if w.Code != 200 || capture.Version != "release-capture-v1" || capture.CaseID != "ruby-pt-draft" || capture.RequestSHA256 != hash || capture.Outcome != "complete" || len(capture.Envelopes) != 4 {
		t.Fatalf("approved synthetic requester must receive four exact envelopes: status=%d capture=%+v", w.Code, capture)
	}
	for i, envelope := range capture.Envelopes {
		if envelope.RequestBody != requests[i] || envelope.ResponseBody != responses[i] || envelope.HTTPStatus != 200 {
			t.Fatalf("envelope %d must contain the actual serialized provider exchange", i)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("capture response must not be cached or exposed with CORS")
	}
	for _, forbidden := range []string{"sk-capture-only", "Bearer ", "Authorization", "capture-only"} {
		if strings.Contains(w.Body.String(), forbidden) || strings.Contains(logs.String(), forbidden) {
			t.Fatalf("authentication material must not enter capture or logs: %s", forbidden)
		}
	}
	if strings.Contains(logs.String(), fixture.posting) || strings.Contains(logs.String(), "manutenção de ferramentas") {
		t.Fatal("synthetic payload contents must not enter normal logs")
	}
}

func TestPrivateCaptureOmitsCredentialEchoesFromEitherProvider(t *testing.T) {
	req, _ := privateCaptureRequest(t)
	req.Header.Set("X-OpenAI-Api-Key", "sk-private-openai-93f")
	req.Header.Set("X-TypeSafe-Api-Key", "private-typesafe-c5e")
	approvePrivateCapture(t, req)
	calls := 0
	client := &http.Client{Transport: captureProviderTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		// A provider may echo credentials using JSON escapes. Neither provider's
		// credential may enter evidence, even during the other provider's stage.
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"error":"sk\u002dprivate\u002dopenai\u002d93f"}`))}, nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	var output struct {
		ReleaseCapture struct {
			Envelopes []struct{ Outcome, ResponseBody string }
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &output) != nil || len(output.ReleaseCapture.Envelopes) != 1 {
		t.Fatal("credential-bearing response must retain only safe capture metadata")
	}
	envelope := output.ReleaseCapture.Envelopes[0]
	if calls != 1 || envelope.ResponseBody != "" || envelope.Outcome != "credential_content" {
		t.Fatalf("credential echo must be withheld without downstream dispatch: calls=%d envelope=%+v", calls, envelope)
	}
}

func TestPrivateCaptureWithholdsAuthenticationFailureBodies(t *testing.T) {
	req, _ := privateCaptureRequest(t)
	approvePrivateCapture(t, req)
	client := &http.Client{Transport: captureProviderTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"error":"Invalid API key: capt...only"}`))}, nil
	})}
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
	var output struct {
		ReleaseCapture struct {
			Envelopes []struct {
				Outcome, ResponseBody string
				ResponseBytes         *int
			}
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &output) != nil || w.Code != 401 || len(output.ReleaseCapture.Envelopes) != 1 {
		t.Fatal("authentication status must preserve the production key-failure response")
	}
	envelope := output.ReleaseCapture.Envelopes[0]
	if envelope.ResponseBody != "" || envelope.ResponseBytes != nil || envelope.Outcome != "authentication_failure" {
		t.Fatal("authentication-failure bodies may contain masked credentials and must be withheld")
	}
}

func TestPrivateCaptureRejectsUnapprovedOrAmbiguousRequestsBeforeProviderWork(t *testing.T) {
	for _, name := range []string{"disabled", "changed body", "wrong origin", "duplicate origin", "unknown case", "expired", "malformed manifest", "duplicate case", "opaque origin", "duplicate opt-in"} {
		t.Run(name, func(t *testing.T) {
			req, fixture := privateCaptureRequest(t)
			approvePrivateCapture(t, req)
			var manifest map[string]any
			if err := json.Unmarshal([]byte(os.Getenv("CAREEROS_RELEASE_CAPTURE")), &manifest); err != nil {
				t.Fatal(err)
			}
			cases := manifest["cases"].([]any)
			switch name {
			case "disabled":
				t.Setenv("CAREEROS_RELEASE_CAPTURE", "")
			case "changed body":
				req.Body = io.NopCloser(strings.NewReader(`{"jobPosting":"Unapproved private material"}`))
			case "wrong origin":
				req.Header.Set("Origin", "https://other.example")
			case "duplicate origin":
				req.Header.Add("Origin", "https://other.example")
			case "unknown case":
				req.Header.Set("X-CareerOS-Release-Capture", "unapproved")
			case "expired":
				manifest["expiresAt"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
			case "malformed manifest":
				t.Setenv("CAREEROS_RELEASE_CAPTURE", os.Getenv("CAREEROS_RELEASE_CAPTURE")+` {}`)
			case "duplicate case":
				manifest["cases"] = append(cases, cases[0])
			case "opaque origin":
				cases[0].(map[string]any)["origin"] = "null"
				req.Header.Set("Origin", "null")
			case "duplicate opt-in":
				req.Header.Add("X-CareerOS-Release-Capture", "ruby-pt-draft")
			}
			if name == "expired" || name == "duplicate case" || name == "opaque origin" {
				raw, _ := json.Marshal(manifest)
				t.Setenv("CAREEROS_RELEASE_CAPTURE", string(raw))
			}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(&http.Client{Transport: fixture}).ServeHTTP(w, req)
			if w.Code != 403 || len(fixture.hashes) != 0 || strings.Contains(w.Body.String(), "releaseCapture") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unapproved capture must stop before provider work: status=%d dispatches=%d", w.Code, len(fixture.hashes))
			}
		})
	}
}

func TestPrivateCaptureNeverDispatchesAfterCallerCancellation(t *testing.T) {
	req, fixture := privateCaptureRequest(t)
	approvePrivateCapture(t, req)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	w := httptest.NewRecorder()
	applicationdraft.NewHandlerWithClient(&http.Client{Transport: fixture}).ServeHTTP(w, req.WithContext(ctx))
	if len(fixture.hashes) != 0 {
		t.Fatal("canceled synthetic capture must not dispatch to any provider")
	}
}

func TestPrivateCaptureRetainsFailuresWithoutTruncationOrRetry(t *testing.T) {
	for _, tc := range []struct {
		name              string
		raw               []byte
		failure           error
		encoding, outcome string
	}{
		{"malformed", []byte("not JSON"), nil, "utf8", "captured"},
		{"binary", []byte{0xff, 0x00, 0x80}, nil, "base64", "captured"},
		{"oversized", bytes.Repeat([]byte("x"), (1<<20)+1), nil, "", "capture_unavailable"},
		{"transport", nil, errors.New("untrusted provider failure details"), "", "transport_failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := privateCaptureRequest(t)
			approvePrivateCapture(t, req)
			calls := 0
			client := &http.Client{Transport: captureProviderTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if tc.failure != nil {
					return nil, tc.failure
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(tc.raw))}, nil
			})}
			w := httptest.NewRecorder()
			applicationdraft.NewHandlerWithClient(client).ServeHTTP(w, req)
			var output struct {
				ReleaseCapture struct {
					Outcome   string
					Envelopes []struct {
						Outcome, ResponseBody, ResponseEncoding string
						ResponseBytes, HTTPStatus               *int
					}
				}
			}
			if json.Unmarshal(w.Body.Bytes(), &output) != nil || len(output.ReleaseCapture.Envelopes) != 1 || calls != 1 || output.ReleaseCapture.Outcome != "incomplete" {
				t.Fatal("failed capture must stop once and preserve its incomplete outcome")
			}
			envelope := output.ReleaseCapture.Envelopes[0]
			if envelope.Outcome != tc.outcome || envelope.ResponseEncoding != tc.encoding {
				t.Fatalf("unexpected failure evidence: %+v", envelope)
			}
			if tc.encoding == "utf8" && envelope.ResponseBody != string(tc.raw) {
				t.Fatal("malformed response bytes were lost")
			}
			if tc.encoding == "base64" && envelope.ResponseBody != "/wCA" {
				t.Fatal("non-UTF8 response bytes must remain exactly recoverable")
			}
			if tc.encoding == "" && envelope.ResponseBody != "" {
				t.Fatal("unavailable capture must not return truncated content")
			}
			if tc.encoding == "" && envelope.ResponseBytes != nil {
				t.Fatal("unavailable byte counts must remain null")
			}
			if tc.failure != nil && envelope.HTTPStatus != nil {
				t.Fatal("unobserved HTTP status must remain null")
			}
			if strings.Contains(w.Body.String(), "untrusted provider failure details") {
				t.Fatal("transport error details must not enter evidence")
			}
		})
	}
}

func TestOrdinaryRequestCannotInheritPriorPrivateCapture(t *testing.T) {
	req, fixture := privateCaptureRequest(t)
	approvePrivateCapture(t, req)
	client := &http.Client{Transport: captureProviderTransport(func(r *http.Request) (*http.Response, error) { return fixture.RoundTrip(r) })}
	handler := applicationdraft.NewHandlerWithClient(client)
	captured := httptest.NewRecorder()
	handler.ServeHTTP(captured, req)
	if !strings.Contains(captured.Body.String(), `"releaseCapture"`) {
		t.Fatal("fixture must first exercise a captured request")
	}
	normal, nextFixture := privateCaptureRequest(t)
	fixture = nextFixture
	normal.Header.Del("X-CareerOS-Release-Capture")
	t.Setenv("CAREEROS_RELEASE_CAPTURE", "invalid configuration does not affect ordinary requests")
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, normal)
	if result.Code != 200 || len(fixture.hashes) != 4 || strings.Contains(result.Body.String(), "releaseCapture") || !strings.Contains(result.Body.String(), "Desenvolvedor Ruby") {
		t.Fatal("ordinary requests must keep their production response without previous capture state")
	}
}
