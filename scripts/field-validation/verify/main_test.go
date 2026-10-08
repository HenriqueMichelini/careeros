package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	draft "professional-information-repo/backend/application-draft"
	ingestion "professional-information-repo/backend/profile-ingestion"
	gaps "professional-information-repo/backend/qualification-gaps"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Public HTTP seams specified in #33. Classifications here are controlled:
// this proves enforcement, never the semantic accuracy of a real classifier.
func TestUnresolvedSubmissionsStayBlockedAcrossRepeatedAndConfirmationRequests(t *testing.T) {
	original := http.DefaultTransport
	var logs bytes.Buffer
	oldLog := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { http.DefaultTransport = original; log.SetOutput(oldLog) })
	raw, err := os.ReadFile("../../../docs/evaluations/field-validation-integration-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []sample
	if json.Unmarshal(raw, &cases) != nil {
		t.Fatal("invalid corpus")
	}
	for _, item := range cases {
		if item.Expected == "accept" {
			continue
		}
		paths := []string{"/api/profile/ingest"}
		if item.Field == "job_posting" {
			paths = []string{"/api/qualification-gaps", "/api/application-draft"}
		}
		for _, path := range paths {
			t.Run(item.ID+path, func(t *testing.T) {
				content, attack := "professional_fact", "none"
				if item.Field == "job_posting" {
					content = "job_with_context"
				}
				switch item.Category {
				case "attack":
					attack = "detected"
				case "uncertain":
					attack = "uncertain"
				case "insufficient":
					content = "relevant_but_insufficient"
				case "irrelevant", "unusable":
					content = item.Category
				}
				calls := 0
				http.DefaultTransport = transport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.String() != "https://api.typesafe.ai/v1/systemone" {
						t.Fatal("blocked input reached downstream processing")
					}
					deadline, ok := r.Context().Deadline()
					if !ok || time.Until(deadline) > 3*time.Second {
						t.Fatal("unbounded classifier")
					}
					var payload struct {
						State map[string]string `json:"state"`
					}
					json.NewDecoder(r.Body).Decode(&payload)
					if len(payload.State) != 2 || payload.State["submission"] != item.Text || payload.State["field"] != string(item.Field) {
						t.Fatal("outbound content was not minimized")
					}
					options := []string{"professional_fact", "relevant_but_insufficient", "irrelevant", "unusable"}
					if item.Field == "job_posting" {
						options = []string{"job_with_context", "job_title_only", "relevant_but_insufficient", "irrelevant", "unusable"}
					}
					choice := func(value string, options []string) any {
						p := map[string]float64{}
						for _, o := range options {
							p[o] = 0
						}
						p[value] = 1
						return map[string]any{"type": "choice", "choice": value, "confidence": 1, "probabilities": p}
					}
					encoded, _ := json.Marshal(map[string]any{"model": "jev-1.13.0", "answers": map[string]any{"content": choice(content, options), "attack": choice(attack, []string{"none", "detected", "uncertain"})}})
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(encoded)), Header: http.Header{}}, nil
				})
				profile := map[string]any{"careerGoals": "", "skills": "saved Kotlin fact", "competencies": "", "tools": "", "experience": []any{}, "projects": []any{}, "employmentStatus": "", "currentSalary": "", "desiredSalary": "", "additionalInfo": ""}
				body := map[string]any{"input": item.Text, "profile": profile}
				handler := ingestion.NewHandler()
				if item.Field == "job_posting" {
					body = map[string]any{"repository": profile, "jobPosting": item.Text}
					handler = gaps.NewHandler()
					if path == "/api/application-draft" {
						handler = draft.NewHandler()
						body["confirmedQualifications"] = []any{map[string]string{"kind": "skill", "requirement": "AWS", "userContext": "confirmed only for this draft"}}
					}
				}
				// A previous response and a confirmation cannot authorize this unchanged input.
				for attempt := 0; attempt < 2; attempt++ {
					encoded, _ := json.Marshal(body)
					req := httptest.NewRequest("POST", path, bytes.NewReader(encoded))
					req.Header.Set("X-OpenAI-Api-Key", "sk-synthetic-secret")
					req.Header.Set("X-TypeSafe-Api-Key", "synthetic-secret")
					req.Header.Set("X-Accepted", "true")
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, req)
					if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"kind":"`+item.Expected+`"`) {
						t.Fatalf("unexpected decision: %d %s", w.Code, w.Body.String())
					}
					var response map[string]any
					json.Unmarshal(w.Body.Bytes(), &response)
					if len(response) != 1 {
						t.Fatal("blocked response exposed downstream output")
					}
				}
				if calls != 2 {
					t.Fatalf("expected fresh classification each time, got %d calls", calls)
				}
			})
		}
	}
	for _, sensitive := range []string{"synthetic-secret", "saved Kotlin fact", "confirmed only for this draft"} {
		if strings.Contains(logs.String(), sensitive) {
			t.Fatal("log leaked credentials or Profile facts")
		}
	}
	for _, item := range cases {
		if strings.Contains(logs.String(), item.Text) {
			t.Fatal("log leaked submission")
		}
	}
}
