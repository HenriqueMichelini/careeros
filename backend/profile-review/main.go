package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	maxRequestBytes = 64 << 10
	maxProfileText  = 12 << 10
	reviewTimeout   = 25 * time.Second
	model           = "gpt-6-luna"
)

type experience struct {
	ID               string `json:"id"`
	Company          string `json:"company"`
	Title            string `json:"title"`
	StartDate        string `json:"startDate"`
	EndDate          string `json:"endDate"`
	Current          bool   `json:"current"`
	Location         string `json:"location"`
	Description      string `json:"description"`
	Responsibilities string `json:"responsibilities"`
	Achievements     string `json:"achievements"`
}

type project struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Technologies string `json:"technologies"`
	URL          string `json:"url"`
	Highlights   string `json:"highlights"`
}

type repository struct {
	CareerGoals      string       `json:"careerGoals"`
	Skills           string       `json:"skills"`
	Competencies     string       `json:"competencies"`
	Experience       []experience `json:"experience"`
	Tools            string       `json:"tools"`
	Projects         []project    `json:"projects"`
	EmploymentStatus string       `json:"employmentStatus"`
	CurrentSalary    string       `json:"currentSalary"`
	DesiredSalary    string       `json:"desiredSalary"`
	AdditionalInfo   string       `json:"additionalInfo"`
}

type reviewRequest struct {
	Repository     repository `json:"repository"`
	ChangedSection string     `json:"changedSection"`
}

type reviewResult struct {
	UpdatedRepository repository `json:"updatedRepository"`
	Summary           string     `json:"summary"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type app struct{ client *http.Client }

func main() {
	port := os.Getenv("PROFILE_REVIEW_PORT")
	if port == "" {
		port = "8787"
	}
	server := &http.Server{Addr: ":" + port, Handler: app{client: &http.Client{Timeout: reviewTimeout}}.handler(), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second}
	log.Printf("profile-review listening port=%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (a app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/profile/review", a.review)
	return mux
}

func (a app) review(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	started := time.Now()
	status, outcome := http.StatusOK, "ok"
	defer func() {
		log.Printf("profile_review status=%d outcome=%s duration_ms=%d", status, outcome, time.Since(started).Milliseconds())
	}()
	if r.Method != http.MethodPost {
		status = http.StatusMethodNotAllowed
		outcome = "input"
		writeError(w, status, "input")
		return
	}
	key := strings.TrimSpace(r.Header.Get("X-OpenAI-Api-Key"))
	if !strings.HasPrefix(key, "sk-") || strings.HasPrefix(key, "sk-ant-") || len(key) > 512 {
		status = http.StatusUnauthorized
		outcome = "key"
		writeError(w, status, "key")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var rawInput map[string]json.RawMessage
	if err := decoder.Decode(&rawInput); err != nil || !hasFields(rawInput, "repository", "changedSection") {
		status = http.StatusBadRequest
		outcome = "input"
		writeError(w, status, "input")
		return
	}
	var input reviewRequest
	if err := json.Unmarshal(mustMarshal(rawInput), &input); err != nil || !completeRepository(rawInput["repository"]) {
		status = http.StatusBadRequest
		outcome = "input"
		writeError(w, status, "input")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		status = http.StatusBadRequest
		outcome = "input"
		writeError(w, status, "input")
		return
	}
	if !validRequest(input) {
		status = http.StatusBadRequest
		outcome = "input"
		writeError(w, status, "input")
		return
	}
	result, code, err := a.callProvider(r.Context(), key, input)
	if err != nil {
		switch code {
		case "key":
			status = http.StatusUnauthorized
		case "rate_limit":
			status = http.StatusTooManyRequests
		case "timeout":
			status = http.StatusGatewayTimeout
		case "invalid_output":
			status = http.StatusBadGateway
		default:
			status = http.StatusBadGateway
			code = "outage"
		}
		outcome = code
		writeError(w, status, code)
		return
	}
	if !validResult(result, input.Repository) {
		status = http.StatusBadGateway
		outcome = "invalid_output"
		writeError(w, status, outcome)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}

func validRequest(in reviewRequest) bool {
	if strings.TrimSpace(in.ChangedSection) == "" || len(in.ChangedSection) > 200 {
		return false
	}
	r := in.Repository
	values := []string{r.CareerGoals, r.Skills, r.Competencies, r.Tools, r.EmploymentStatus, r.CurrentSalary, r.DesiredSalary, r.AdditionalInfo}
	for _, s := range values {
		if len(s) > maxProfileText {
			return false
		}
	}
	if len(r.Experience) > 40 || len(r.Projects) > 40 {
		return false
	}
	for _, e := range r.Experience {
		if e.ID == "" || len(e.ID) > 100 || !bounded(e.Company, e.Title, e.StartDate, e.EndDate, e.Location, e.Description, e.Responsibilities, e.Achievements) {
			return false
		}
	}
	for _, p := range r.Projects {
		if p.ID == "" || len(p.ID) > 100 || !bounded(p.Name, p.Description, p.Technologies, p.URL, p.Highlights) {
			return false
		}
	}
	return true
}

func bounded(values ...string) bool {
	for _, value := range values {
		if len(value) > maxProfileText {
			return false
		}
	}
	return true
}

func (a app) callProvider(parent context.Context, key string, input reviewRequest) (reviewResult, string, error) {
	var empty reviewResult
	repoJSON, _ := json.Marshal(input.Repository)
	prompt := "You are an expert career coach reviewing a professional profile. The user updated the section: " + input.ChangedSection + ".\n\nProfile JSON:\n" + string(repoJSON) + "\n\nImprove clarity, grammar, and professional tone throughout; preserve all facts, numbers, names, dates, and structure. Never invent facts. Return only JSON with this exact shape: {\"updatedRepository\":<complete profile object with every original field>,\"summary\":\"brief description\"}. Include every field and every list entry."
	body, _ := json.Marshal(map[string]any{"model": model, "reasoning_effort": "none", "max_completion_tokens": 5000, "response_format": map[string]string{"type": "json_object"}, "messages": []any{map[string]string{"role": "user", "content": prompt}}})
	ctx, cancel := context.WithTimeout(parent, reviewTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return empty, "outage", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := a.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return empty, "timeout", err
		}
		return empty, "outage", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return empty, "key", errors.New("provider key rejected")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return empty, "rate_limit", errors.New("provider rate limited")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return empty, "outage", errors.New("provider unavailable")
	}
	var upstream openAIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&upstream); err != nil || len(upstream.Choices) != 1 || strings.TrimSpace(upstream.Choices[0].Message.Content) == "" {
		return empty, "invalid_output", errors.New("provider response invalid")
	}
	decoder := json.NewDecoder(strings.NewReader(upstream.Choices[0].Message.Content))
	decoder.DisallowUnknownFields()
	var result reviewResult
	var rawResult map[string]json.RawMessage
	if err := decoder.Decode(&rawResult); err != nil || !hasFields(rawResult, "updatedRepository", "summary") || !completeRepository(rawResult["updatedRepository"]) {
		return empty, "invalid_output", errors.New("incomplete provider result")
	}
	if err := json.Unmarshal(mustMarshal(rawResult), &result); err != nil {
		return empty, "invalid_output", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return empty, "invalid_output", errors.New("trailing response data")
	}
	return result, "", nil
}

func validResult(result reviewResult, original repository) bool {
	if strings.TrimSpace(result.Summary) == "" || len(result.Summary) > 1000 {
		return false
	}
	updated := result.UpdatedRepository
	if len(updated.Experience) != len(original.Experience) || len(updated.Projects) != len(original.Projects) {
		return false
	}
	for i, entry := range updated.Experience {
		if entry.ID != original.Experience[i].ID {
			return false
		}
	}
	for i, entry := range updated.Projects {
		if entry.ID != original.Projects[i].ID {
			return false
		}
	}
	return true
}

func hasFields(value map[string]json.RawMessage, fields ...string) bool {
	if len(value) != len(fields) {
		return false
	}
	for _, field := range fields {
		if _, ok := value[field]; !ok {
			return false
		}
	}
	return true
}

func completeRepository(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || !hasFields(value, "careerGoals", "skills", "competencies", "experience", "tools", "projects", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo") {
		return false
	}
	for _, field := range []string{"careerGoals", "skills", "competencies", "tools", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo"} {
		if !isJSONType(value[field], '"') {
			return false
		}
	}
	if !isJSONType(value["experience"], '[') || !isJSONType(value["projects"], '[') {
		return false
	}
	var entries []json.RawMessage
	if json.Unmarshal(value["experience"], &entries) != nil {
		return false
	}
	for _, entry := range entries {
		var fields map[string]json.RawMessage
		if json.Unmarshal(entry, &fields) != nil || !hasFields(fields, "id", "company", "title", "startDate", "endDate", "current", "location", "description", "responsibilities", "achievements") {
			return false
		}
		for _, field := range []string{"id", "company", "title", "startDate", "endDate", "location", "description", "responsibilities", "achievements"} {
			if !isJSONType(fields[field], '"') {
				return false
			}
		}
		current := strings.TrimSpace(string(fields["current"]))
		if current != "true" && current != "false" {
			return false
		}
	}
	entries = nil
	if json.Unmarshal(value["projects"], &entries) != nil {
		return false
	}
	for _, entry := range entries {
		var fields map[string]json.RawMessage
		if json.Unmarshal(entry, &fields) != nil || !hasFields(fields, "id", "name", "description", "technologies", "url", "highlights") {
			return false
		}
		for _, field := range []string{"id", "name", "description", "technologies", "url", "highlights"} {
			if !isJSONType(fields[field], '"') {
				return false
			}
		}
	}
	return true
}

func isJSONType(raw json.RawMessage, first byte) bool {
	value := strings.TrimSpace(string(raw))
	return len(value) > 0 && value[0] == first
}

func mustMarshal(value any) []byte { encoded, _ := json.Marshal(value); return encoded }

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
