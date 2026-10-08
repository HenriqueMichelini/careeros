// Opt-in live verification through the production classifier and public handlers.
// No retries or fallback. Only synthetic corpus inputs; generated content stays
// in a private local file for human factual inspection, never in the report.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	draft "professional-information-repo/backend/application-draft"
	ingestion "professional-information-repo/backend/profile-ingestion"
	gaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/profilevalidation"
)

type sample struct {
	ID       string                `json:"id"`
	Language string                `json:"language"`
	Field    fieldvalidation.Field `json:"field"`
	Category string                `json:"category"`
	Text     string                `json:"text"`
	Expected string                `json:"expected"`
}
type record struct {
	ID         string                   `json:"id"`
	Decision   fieldvalidation.Decision `json:"decision"`
	DurationMS int64                    `json:"durationMs"`
}
type workflow struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"durationMs"`
	NoStore    bool   `json:"noStore"`
}

func main() {
	live := flag.Bool("live", false, "authorize one bounded synthetic run with environment-provided user keys")
	output := flag.String("output", "", "new metadata report file")
	private := flag.String("private-output", "", "new private generated-output file for local inspection")
	workflowLimit := flag.Int("workflow-limit", 6, "maximum representative workflow requests (0 through 6)")
	flag.Parse()
	if *workflowLimit < 0 || *workflowLimit > 6 {
		panic("Workflow limit must be 0 through 6")
	}
	if !*live || *output == "" || *private == "" || os.Getenv("TYPESAFE_API_KEY") == "" || os.Getenv("OPENAI_API_KEY") == "" {
		panic("Explicit --live, output paths and both user keys required")
	}
	reportFile, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic("Report destination must be new and writable")
	}
	defer reportFile.Close()
	privateFile, err := os.OpenFile(*private, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic("Private destination must be new and writable")
	}
	defer privateFile.Close()
	raw, err := os.ReadFile("docs/evaluations/field-validation-integration-cases.json")
	if err != nil {
		panic("Cannot read synthetic corpus")
	}
	var cases []sample
	if json.Unmarshal(raw, &cases) != nil || len(cases) != 36 {
		panic("Invalid frozen corpus")
	}
	digest := sha256.Sum256(raw)
	report := struct {
		Scope      string     `json:"scope"`
		Started    string     `json:"started"`
		CorpusHash string     `json:"corpusHash"`
		Model      string     `json:"model"`
		Records    []record   `json:"records"`
		Workflows  []workflow `json:"workflows"`
	}{Scope: "local production handlers / live providers; not deployed or browser evidence", Started: time.Now().UTC().Format(time.RFC3339), CorpusHash: hex.EncodeToString(digest[:]), Model: fieldvalidation.Model}
	checkpoint := func() {
		reportFile.Seek(0, 0)
		reportFile.Truncate(0)
		if json.NewEncoder(reportFile).Encode(report) != nil {
			panic("Cannot save metadata checkpoint")
		}
	}
	for _, item := range cases {
		start := time.Now()
		decision := fieldvalidation.Classify(context.Background(), &http.Client{Timeout: 3 * time.Second}, os.Getenv("TYPESAFE_API_KEY"), item.Field, item.Text)
		report.Records = append(report.Records, record{item.ID, decision, time.Since(start).Milliseconds()})
		checkpoint()
		if decision.Outcome.Kind == "service_failure" {
			fmt.Println("Stopped on classifier service failure; inspect metadata")
			return
		}
	}
	// Representative EN/PT proposals, qualification extraction and short drafts.
	// These are new gated requests, not a reuse of earlier classification acceptance.
	for _, item := range cases {
		if item.Category != "short" {
			continue
		}
		p := profilevalidation.Profile{Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
		paths := []string{"/api/profile/ingest"}
		if item.Field == fieldvalidation.JobPosting {
			paths = []string{"/api/qualification-gaps", "/api/application-draft"}
			p.Skills = "Java"
		}
		for _, path := range paths {
			if len(report.Workflows) >= *workflowLimit {
				fmt.Println("Live run complete within workflow limit; inspect private output")
				return
			}
			var handler http.Handler
			body := map[string]any{}
			switch path {
			case "/api/profile/ingest":
				handler = ingestion.NewHandler()
				body = map[string]any{"input": item.Text, "profile": p}
			case "/api/qualification-gaps":
				handler = gaps.NewHandler()
				body = map[string]any{"jobPosting": item.Text, "repository": p}
			default:
				handler = draft.NewHandler()
				locale := "en"
				if item.Language == "pt" {
					locale = "pt-BR"
				}
				body = map[string]any{"jobPosting": item.Text, "repository": p, "confirmedQualifications": []any{}, "cvLanguage": locale}
			}
			encoded, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", path, bytes.NewReader(encoded))
			req.Header.Set("X-TypeSafe-Api-Key", os.Getenv("TYPESAFE_API_KEY"))
			req.Header.Set("X-OpenAI-Api-Key", os.Getenv("OPENAI_API_KEY"))
			rec := httptest.NewRecorder()
			start := time.Now()
			handler.ServeHTTP(rec, req)
			report.Workflows = append(report.Workflows, workflow{item.ID, path, rec.Code, time.Since(start).Milliseconds(), rec.Header().Get("Cache-Control") == "no-store"})
			checkpoint()
			json.NewEncoder(privateFile).Encode(map[string]any{"id": item.ID, "path": path, "response": json.RawMessage(rec.Body.Bytes())})
			if rec.Code != 200 {
				fmt.Println("Stopped on workflow failure; inspect metadata")
				return
			}
		}
	}
	fmt.Println("Live run complete; inspect private output before reporting factual fidelity")
}
