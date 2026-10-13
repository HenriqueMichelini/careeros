package qualificationgaps

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestApprovedQualificationCapturePreservesPrivateProfileProjection(t *testing.T) {
	raw, _ := json.Marshal(sampleRequest())
	hash := sha256.Sum256(raw)
	manifest, _ := json.Marshal(map[string]any{
		"version": "release-capture-manifest-v1", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"cases": []any{map[string]any{
			"id": "qualification-synthetic", "path": "/api/qualification-gaps", "origin": "https://release.example",
			"requestSHA256": hex.EncodeToString(hash[:]), "stages": []string{"field_validation", "qualification_matching"}, "dispatch": true,
		}},
	})
	t.Setenv("CAREEROS_RELEASE_CAPTURE", string(manifest))
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(providerOK()))}, nil
	})}
	req := httptest.NewRequest("POST", "/api/qualification-gaps", strings.NewReader(string(raw)))
	req.Header.Set("Origin", "https://release.example")
	req.Header.Set("X-CareerOS-Release-Capture", "qualification-synthetic")
	req.Header.Set("X-OpenAI-Api-Key", "sk-capture-openai-93f")
	req.Header.Set("X-TypeSafe-Api-Key", "capture-typesafe-c5e")
	w := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(w, req)
	var output struct {
		ReleaseCapture struct {
			Outcome   string
			Envelopes []struct{ Stage, RequestBody, ResponseBody string }
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &output) != nil || w.Code != 200 || output.ReleaseCapture.Outcome != "complete" || len(output.ReleaseCapture.Envelopes) != 2 {
		t.Fatalf("approved qualification request must return both exact provider exchanges: status=%d", w.Code)
	}
	for _, forbidden := range []string{"SENTINEL_CURRENT_SALARY", "SENTINEL_DESIRED_SALARY", "SENTINEL_CAREER_GOALS", "SENTINEL_COMPANY", "capture-openai-93f", "capture-typesafe-c5e", "Authorization"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("private or authentication field entered capture: %s", forbidden)
		}
	}
	if !strings.Contains(output.ReleaseCapture.Envelopes[0].RequestBody, sampleRequest().JobPosting) || output.ReleaseCapture.Envelopes[1].ResponseBody != providerOK() {
		t.Fatal("capture must retain the actual whole-original Jev input and qualification response")
	}
}

func TestQualificationInspectionStopsAtFirstEnvelopeWithoutDispatch(t *testing.T) {
	raw, _ := json.Marshal(sampleRequest())
	hash := sha256.Sum256(raw)
	manifest, _ := json.Marshal(map[string]any{
		"version": "release-capture-manifest-v1", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"cases": []any{map[string]any{
			"id": "qualification-inspection", "path": "/api/qualification-gaps", "origin": "https://release.example",
			"requestSHA256": hex.EncodeToString(hash[:]), "stages": []string{"field_validation", "qualification_matching"}, "dispatch": false,
		}},
	})
	t.Setenv("CAREEROS_RELEASE_CAPTURE", string(manifest))
	dispatches := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		dispatches++
		t.Error("inspection must stop before the provider transport")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(providerOK()))}, nil
	})}
	req := httptest.NewRequest("POST", "/api/qualification-gaps", bytes.NewReader(raw))
	req.Header.Set("Origin", "https://release.example")
	req.Header.Set("X-CareerOS-Release-Capture", "qualification-inspection")
	req.Header.Set("X-OpenAI-Api-Key", "sk-release-inspection-inert")
	req.Header.Set("X-TypeSafe-Api-Key", "release-inspection-inert")
	w := httptest.NewRecorder()
	NewHandlerWithClient(client).ServeHTTP(w, req)
	var output struct {
		ReleaseCapture struct {
			Outcome, CaseID, RequestSHA256 string
			Envelopes                      []struct {
				Stage, Outcome, RequestBody, RequestSHA256 string
				RequestBytes                               *int
				HTTPStatus, ResponseBytes                  *int
				DurationMS                                 *float64
			}
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	capture := output.ReleaseCapture
	if w.Code != http.StatusBadGateway || w.Header().Get("Cache-Control") != "no-store" || dispatches != 0 ||
		capture.Outcome != "inspection_only" || capture.CaseID != "qualification-inspection" ||
		capture.RequestSHA256 != hex.EncodeToString(hash[:]) || len(capture.Envelopes) != 1 {
		t.Fatalf("inspection contract: status=%d dispatches=%d capture=%+v", w.Code, dispatches, capture)
	}
	envelope := capture.Envelopes[0]
	var providerInput struct {
		State struct{ Field, Submission string }
	}
	if err := json.Unmarshal([]byte(envelope.RequestBody), &providerInput); err != nil {
		t.Fatal(err)
	}
	providerHash := sha256.Sum256([]byte(envelope.RequestBody))
	if envelope.Stage != "field_validation" || envelope.Outcome != "not_dispatched" ||
		providerInput.State.Field != "job_posting" || providerInput.State.Submission != sampleRequest().JobPosting ||
		envelope.RequestBytes == nil || *envelope.RequestBytes != len(envelope.RequestBody) ||
		envelope.RequestSHA256 != hex.EncodeToString(providerHash[:]) ||
		envelope.HTTPStatus != nil || envelope.ResponseBytes != nil || envelope.DurationMS != nil {
		t.Fatal("inspection must retain only the exact first envelope, with unavailable provider measurements")
	}
}

func TestPrivateQualificationCaptureIncludesResponseDependentRequirementEvidence(t *testing.T) {
	w := explainedRequest(t, clarification, nil, func(req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(raw))
		hash := sha256.Sum256(raw)
		manifest, _ := json.Marshal(map[string]any{
			"version": "release-capture-manifest-v1", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"cases": []any{map[string]any{
				"id": "explained-qualification", "path": "/api/qualification-gaps", "origin": "https://release.example",
				"requestSHA256": hex.EncodeToString(hash[:]), "stages": []string{"field_validation", "qualification_matching", "requirement_evidence"}, "dispatch": true,
			}},
		})
		t.Setenv("CAREEROS_RELEASE_CAPTURE", string(manifest))
		req.Header.Set("Origin", "https://release.example")
		req.Header.Set("X-CareerOS-Release-Capture", "explained-qualification")
		req.Header.Set("X-OpenAI-Api-Key", "sk-capture-openai-93f")
		req.Header.Set("X-TypeSafe-Api-Key", "capture-typesafe-c5e")
	})
	var result struct {
		ReleaseCapture struct {
			Outcome   string
			Envelopes []struct{ Stage, RequestBody, Admission string }
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || w.Code != 200 || result.ReleaseCapture.Outcome != "complete" || len(result.ReleaseCapture.Envelopes) != 3 {
		t.Fatal("explained qualification capture must include all three real serialized stages")
	}
	dependent := result.ReleaseCapture.Envelopes[2]
	if dependent.Stage != "requirement_evidence" || dependent.Admission != "release-egress-v1" || !strings.Contains(dependent.RequestBody, "Java required.") || !strings.Contains(dependent.RequestBody, "requirementIndex") {
		t.Fatal("requirement support must be inspected after job understanding, from the actual dependent envelope")
	}
}
