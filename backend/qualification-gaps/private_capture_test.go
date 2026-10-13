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
