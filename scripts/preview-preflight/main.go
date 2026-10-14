package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	gaps "professional-information-repo/backend/qualification-gaps"
)

type closedTransport struct{ calls int }

func (c *closedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls++
	if c.calls == 1 && r.URL.String() == "https://api.typesafe.ai/v1/systemone" {
		choice := func(selected string, options ...string) any {
			p := map[string]float64{}
			for _, o := range options {
				p[o] = 0
			}
			p[selected] = 1
			return map[string]any{"type": "choice", "choice": selected, "confidence": 1, "probabilities": p}
		}
		raw, _ := json.Marshal(map[string]any{"model": "jev-1.13.0", "answers": map[string]any{"content": choice("job_with_context", "job_with_context", "job_title_only", "relevant_but_insufficient", "irrelevant", "unusable"), "attack": choice("none", "none", "uncertain", "detected")}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}}, nil
	}
	return nil, errors.New("closed controlled transport; never dials")
}
func hash(raw []byte) string { x := sha256.Sum256(raw); return hex.EncodeToString(x[:]) }
func main() {
	if len(os.Args) != 2 {
		panic("private artifact directory required")
	}
	base := os.Args[1]
	origin := "https://deploy-preview-114--beamish-bavarois-333d03.netlify.app"
	for _, id := range []string{"p3-postgresql-en", "p3-typescript-pt"} {
		raw, err := os.ReadFile(filepath.Join(base, id+".qualification-request.json"))
		if err != nil {
			panic(err)
		}
		manifest, _ := json.Marshal(map[string]any{"version": "release-capture-manifest-v1", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "cases": []any{map[string]any{"id": id, "path": "/api/qualification-gaps", "origin": origin, "requestSHA256": hash(raw), "dispatch": true, "stages": []string{"field_validation", "qualification_matching", "requirement_evidence"}}}})
		if err = os.Setenv("CAREEROS_RELEASE_CAPTURE", string(manifest)); err != nil {
			panic(err)
		}
		transport := &closedTransport{}
		req := httptest.NewRequest("POST", "/api/qualification-gaps", bytes.NewReader(raw))
		req.Header.Set("Origin", origin)
		req.Header.Set("X-CareerOS-Release-Capture", id)
		req.Header.Set("X-OpenAI-Api-Key", "sk-release-inspection-inert")
		req.Header.Set("X-TypeSafe-Api-Key", "release-inspection-inert")
		w := httptest.NewRecorder()
		gaps.NewHandlerWithClient(&http.Client{Transport: transport}).ServeHTTP(w, req)
		var output struct {
			ReleaseCapture struct {
				Outcome   string
				Envelopes []struct {
					Stage, RequestBody, RequestSHA256, Endpoint string
					RequestBytes                                int
				}
			}
		}
		if err = json.Unmarshal(w.Body.Bytes(), &output); err != nil {
			panic(err)
		}
		if w.Code != 502 || transport.calls != 2 || output.ReleaseCapture.Outcome != "incomplete" || len(output.ReleaseCapture.Envelopes) != 2 {
			panic(fmt.Sprintf("controlled preflight blocked for %s status=%d transportCalls=%d envelopes=%d", id, w.Code, transport.calls, len(output.ReleaseCapture.Envelopes)))
		}
		if strings.Contains(w.Body.String(), "sk-release-inspection-inert") {
			panic("credential content")
		}
		if err = os.WriteFile(filepath.Join(base, id+".preflight.private.json"), w.Body.Bytes(), 0600); err != nil {
			panic(err)
		}
		envelopes := []map[string]any{}
		for _, e := range output.ReleaseCapture.Envelopes {
			if hash([]byte(e.RequestBody)) != e.RequestSHA256 || len(e.RequestBody) != e.RequestBytes {
				panic("envelope identity")
			}
			envelopes = append(envelopes, map[string]any{"stage": e.Stage, "endpoint": e.Endpoint, "requestBytes": e.RequestBytes, "requestSHA256": e.RequestSHA256})
		}
		summary := map[string]any{"id": id, "requestBytes": len(raw), "requestSHA256": hash(raw), "evidence": "controlled actual handler serialization with a fabricated Jev acceptance and a closed transport; no external provider traffic or quality evidence", "externalProviderDispatches": 0, "envelopes": envelopes}
		encoded, _ := json.MarshalIndent(summary, "", "  ")
		if err = os.WriteFile(filepath.Join(base, id+".preflight-summary.json"), encoded, 0600); err != nil {
			panic(err)
		}
		fmt.Println(string(encoded))
	}
}
