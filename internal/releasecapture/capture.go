// Package releasecapture returns bounded synthetic release evidence to the
// requesting browser. It never logs or persists payloads or authentication.
package releasecapture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	optInHeader     = "X-CareerOS-Release-Capture"
	manifestEnv     = "CAREEROS_RELEASE_CAPTURE"
	maxInputBytes   = 512 << 10
	maxCaptureBytes = 1 << 20
	maxResultBytes  = 512 << 10
)

type approvedCase struct {
	ID            string   `json:"id"`
	Path          string   `json:"path"`
	Origin        string   `json:"origin"`
	RequestSHA256 string   `json:"requestSHA256"`
	Stages        []string `json:"stages"`
	expiresAt     time.Time
}

type manifest struct {
	Version   string         `json:"version"`
	ExpiresAt time.Time      `json:"expiresAt"`
	Cases     []approvedCase `json:"cases"`
}

type evidence struct {
	Version       string     `json:"version"`
	CaseID        string     `json:"caseId"`
	RequestSHA256 string     `json:"requestSHA256"`
	Outcome       string     `json:"outcome"`
	Envelopes     []envelope `json:"envelopes"`
}

type collector struct {
	evidence
	approval      approvedCase
	used          int
	stopped       bool
	secrets       []string
	posting       string
	understandJob bool
}

type contextKey struct{}

func digest(raw []byte) string {
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// Handler leaves ordinary requests unchanged. An opt-in request is rejected
// before provider work unless its exact bytes, origin and route were approved.
func Handler(path string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Header.Values(optInHeader)) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		approval, raw, ok := admit(r, path)
		if !ok {
			writeFailure(w, http.StatusForbidden)
			return
		}
		c := &collector{approval: approval, evidence: evidence{Version: "release-capture-v1", CaseID: approval.ID, RequestSHA256: approval.RequestSHA256, Envelopes: []envelope{}}}
		c.secrets = []string{strings.TrimSpace(r.Header.Get("X-OpenAI-Api-Key")), strings.TrimSpace(r.Header.Get("X-TypeSafe-Api-Key"))}
		var input struct {
			JobPosting    string
			UnderstandJob bool
		}
		_ = json.Unmarshal(raw, &input)
		c.posting, c.understandJob = input.JobPosting, input.UnderstandJob
		r.Body = io.NopCloser(bytes.NewReader(raw))
		ctx := context.WithValue(r.Context(), contextKey{}, c)
		ctx, cancel := context.WithDeadline(ctx, approval.expiresAt)
		defer cancel()
		result := &boundedResult{headers: make(http.Header)}
		next.ServeHTTP(result, r.WithContext(ctx))
		c.Outcome = "complete"
		if c.stopped || len(c.Envelopes) != len(c.approval.Stages) {
			c.Outcome = "incomplete"
		}
		returnEvidence(w, result, c.evidence)
	})
}

func admit(r *http.Request, path string) (approvedCase, []byte, bool) {
	var empty approvedCase
	config := os.Getenv(manifestEnv)
	if len(config) == 0 || len(config) > 32<<10 || r.Method != http.MethodPost || r.URL.Path != path || len(r.Header.Values(optInHeader)) != 1 || len(r.Header.Values("Origin")) != 1 {
		return empty, nil, false
	}
	d := json.NewDecoder(bytes.NewBufferString(config))
	d.DisallowUnknownFields()
	var m manifest
	if d.Decode(&m) != nil || !validManifest(m) {
		return empty, nil, false
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return empty, nil, false
	}
	for _, candidate := range m.Cases {
		if candidate.ID != r.Header.Get(optInHeader) || candidate.Path != path || candidate.Origin == "" || candidate.Origin != r.Header.Get("Origin") || len(candidate.Stages) == 0 || len(candidate.Stages) > 4 {
			continue
		}
		defer r.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(r.Body, maxInputBytes+1))
		if err != nil || len(raw) > maxInputBytes || digest(raw) != candidate.RequestSHA256 {
			return empty, nil, false
		}
		candidate.expiresAt = m.ExpiresAt
		return candidate, raw, true
	}
	return empty, nil, false
}

var caseID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

func validManifest(m manifest) bool {
	if m.Version != "release-capture-manifest-v1" || !m.ExpiresAt.After(time.Now()) || m.ExpiresAt.After(time.Now().Add(24*time.Hour)) || len(m.Cases) == 0 || len(m.Cases) > 8 {
		return false
	}
	seen := map[string]bool{}
	for _, candidate := range m.Cases {
		hash, err := hex.DecodeString(candidate.RequestSHA256)
		if !caseID.MatchString(candidate.ID) || seen[candidate.ID] || err != nil || len(hash) != sha256.Size || strings.ToLower(candidate.RequestSHA256) != candidate.RequestSHA256 || !validOrigin(candidate.Origin) || !validStages(candidate) {
			return false
		}
		seen[candidate.ID] = true
	}
	return true
}

func validOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http" && slices.Contains([]string{"127.0.0.1", "localhost", "::1"}, u.Hostname())
}

func validStages(candidate approvedCase) bool {
	if candidate.Path == "/api/application-draft" {
		return slices.Equal(candidate.Stages, []string{"field_validation", "generation", "resume_support", "artifact_support"})
	}
	if candidate.Path == "/api/qualification-gaps" {
		return slices.Equal(candidate.Stages, []string{"field_validation", "qualification_matching", "requirement_evidence"}) ||
			slices.Equal(candidate.Stages, []string{"field_validation", "qualification_matching"}) ||
			slices.Equal(candidate.Stages, []string{"field_validation", "requirement_evidence"})
	}
	return false
}

type boundedResult struct {
	headers  http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (w *boundedResult) Header() http.Header { return w.headers }
func (w *boundedResult) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *boundedResult) Write(raw []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(raw) > maxResultBytes {
		w.overflow = true
		return len(raw), nil
	}
	return w.body.Write(raw)
}

func returnEvidence(w http.ResponseWriter, result *boundedResult, capture evidence) {
	var output map[string]json.RawMessage
	if result.overflow || json.Unmarshal(result.body.Bytes(), &output) != nil || output == nil {
		writeFailure(w, http.StatusBadGateway)
		return
	}
	raw, err := json.Marshal(capture)
	if err != nil {
		writeFailure(w, http.StatusBadGateway)
		return
	}
	output["releaseCapture"] = raw
	body, err := json.Marshal(output)
	if err != nil || len(body) > 3<<20 {
		writeFailure(w, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	status := result.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeFailure(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `{"error":"release_capture_unavailable"}`)
}
