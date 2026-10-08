// Replay exact live-provider responses through production handlers. Missing
// responses export the next exact payload for inspection, without network I/O.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	draft "professional-information-repo/backend/application-draft"
	ingestion "professional-information-repo/backend/profile-ingestion"
)

type sample struct {
	ID   string          `json:"id"`
	Path string          `json:"path"`
	Body json.RawMessage `json:"body"`
}
type replay struct {
	dir                            string
	count, classifiers, generators int
	pending                        bool
}

func (t *replay) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.String() != "https://api.typesafe.ai/v1/systemone" && r.URL.String() != "https://api.openai.com/v1/chat/completions" {
		return nil, fmt.Errorf("unexpected recipient")
	}
	if r.URL.Host == "api.typesafe.ai" {
		t.classifiers++
	} else {
		t.generators++
	}
	if t.classifiers > 8 || t.generators > 14 {
		return nil, fmt.Errorf("budget exceeded")
	}
	t.count++
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(t.dir, fmt.Sprintf("call-%02d", t.count))
	prior, err := os.ReadFile(base + ".payload.json")
	if err == nil && !bytes.Equal(prior, raw) {
		panic("payload changed during replay")
	}
	if err = os.WriteFile(base+".payload.json", raw, 0600); err != nil {
		panic(err)
	}
	response, err := os.ReadFile(base + ".response.json")
	if os.IsNotExist(err) {
		metadata, _ := json.Marshal(map[string]any{"call": t.count, "url": r.URL.String(), "payload": base + ".payload.json"})
		os.WriteFile(filepath.Join(t.dir, "pending.json"), metadata, 0600)
		t.pending = true
		return nil, fmt.Errorf("awaiting exact payload inspection")
	}
	if err != nil {
		panic(err)
	}
	var stored struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if json.Unmarshal(response, &stored) != nil {
		panic("invalid private response")
	}
	return &http.Response{StatusCode: stored.Status, Body: io.NopCloser(bytes.NewReader(stored.Body)), Header: http.Header{}}, nil
}
func main() {
	if len(os.Args) != 2 {
		panic("private replay directory required")
	}
	dir := os.Args[1]
	raw, err := os.ReadFile("docs/evaluations/field-validation-remediation-cases.json")
	if err != nil {
		panic(err)
	}
	var cases []sample
	if json.Unmarshal(raw, &cases) != nil || len(cases) != 8 {
		panic("invalid approved fixture set")
	}
	transport := &replay{dir: dir}
	http.DefaultTransport = transport
	for _, item := range cases {
		var h http.Handler
		if item.Path == "/api/profile/ingest" {
			h = ingestion.NewHandler()
		} else if item.Path == "/api/application-draft" {
			h = draft.NewHandler()
		} else {
			panic("unapproved workflow")
		}
		r := httptest.NewRequest("POST", item.Path, bytes.NewReader(item.Body))
		r.Header.Set("X-OpenAI-Api-Key", "sk-offline-replay")
		r.Header.Set("X-TypeSafe-Api-Key", "offline-replay")
		w := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(w, r)
		if transport.pending {
			os.Exit(10)
		}
		os.WriteFile(filepath.Join(dir, item.ID+".json"), w.Body.Bytes(), 0600)
		fmt.Printf("RESULT %s status=%d replay_ms=%d no_store=%t calls=%d\n", item.ID, w.Code, time.Since(start).Milliseconds(), w.Header().Get("Cache-Control") == "no-store", transport.count)
		if w.Code != 200 {
			fmt.Println("STOP operational failure")
			os.Exit(1)
		}
		if item.Path == "/api/profile/ingest" {
			var result struct {
				Operations []struct{ Target, Value string }
				Unresolved []string `json:"unresolvedClaimIds"`
				Unverified int      `json:"unverifiedClaimCount"`
				Decision   struct{ Outcome struct{ Kind string } }
			}
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Decision.Outcome.Kind != "accept" {
				fmt.Println("STOP decision failure")
				os.Exit(1)
			}
			java := false
			for _, op := range result.Operations {
				if op.Target == "skills" && strings.EqualFold(op.Value, "Java") {
					java = true
				}
			}
			if (item.ID == "en-duplicate" && len(result.Operations) != 0) || (item.ID != "en-duplicate" && (!java || len(result.Unresolved) > 0 || result.Unverified > 0)) {
				fmt.Println("STOP proposal failure")
				os.Exit(1)
			}
		}
	}
	fmt.Printf("COMPLETE classifier_calls=%d generation_calls=%d\n", transport.classifiers, transport.generators)
}
