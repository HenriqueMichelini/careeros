package profilereview

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"professional-information-repo/internal/openaihttp"
	"professional-information-repo/internal/profilevalidation"
)

const (
	maxRequestBytes = 64 << 10
	maxProfileText  = 12 << 10
	reviewTimeout   = 25 * time.Second
	model           = "gpt-6-luna"
)

type experience = profilevalidation.Experience
type project = profilevalidation.Project
type repository = profilevalidation.Profile

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

// NewHandler returns the stateless Profile review HTTP API shared by local preview and Netlify.
func NewHandler() http.Handler {
	a := app{client: &http.Client{Timeout: reviewTimeout}}
	return a.handler()
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
	if err := json.Unmarshal(mustMarshal(rawInput), &input); err != nil || !profilevalidation.CompleteJSON(rawInput["repository"]) {
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
	result.UpdatedRepository = scopedRepository(input.Repository, result.UpdatedRepository, reviewFields(input.ChangedSection))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}

func reviewFields(section string) []string {
	switch strings.ToLower(strings.TrimSpace(section)) {
	case "goals", "career goals", "objetivos de carreira":
		return []string{"careerGoals"}
	case "skills", "skills, tools & tech", "habilidades e tecnologias":
		return []string{"skills", "competencies", "tools"}
	case "experience", "experiência":
		return []string{"experience"}
	case "projects", "projetos":
		return []string{"projects"}
	case "compensation", "remuneração":
		return []string{"employmentStatus", "currentSalary", "desiredSalary"}
	case "other", "outros":
		return []string{"additionalInfo"}
	default:
		return nil
	}
}

func validRequest(in reviewRequest) bool {
	return len(reviewFields(in.ChangedSection)) > 0 && profilevalidation.Valid(in.Repository, maxProfileText)
}

// Preserve every field outside the section the user chose, even if the model edits it.
func scopedRepository(original, updated repository, fields []string) repository {
	for _, field := range fields {
		switch field {
		case "careerGoals":
			original.CareerGoals = updated.CareerGoals
		case "skills":
			original.Skills = updated.Skills
		case "competencies":
			original.Competencies = updated.Competencies
		case "tools":
			original.Tools = updated.Tools
		case "experience":
			original.Experience = updated.Experience
		case "projects":
			original.Projects = updated.Projects
		case "employmentStatus":
			original.EmploymentStatus = updated.EmploymentStatus
		case "currentSalary":
			original.CurrentSalary = updated.CurrentSalary
		case "desiredSalary":
			original.DesiredSalary = updated.DesiredSalary
		case "additionalInfo":
			original.AdditionalInfo = updated.AdditionalInfo
		}
	}
	return original
}

func (a app) callProvider(parent context.Context, key string, input reviewRequest) (reviewResult, string, error) {
	var empty reviewResult
	repoJSON, _ := json.Marshal(input.Repository)
	prompt := "You are an expert career coach reviewing a professional profile. The user updated the section: " + input.ChangedSection + ".\n\nProfile JSON:\n" + string(repoJSON) + "\n\nImprove clarity, grammar, and professional tone only in these fields: " + strings.Join(reviewFields(input.ChangedSection), ", ") + ". Leave every other field unchanged; preserve all facts, numbers, names, dates, and structure. Never invent facts. Return only JSON with this exact shape: {\"updatedRepository\":<complete profile object with every original field>,\"summary\":\"brief description\"}. Include every field and every list entry."
	body, _ := json.Marshal(map[string]any{"model": model, "reasoning_effort": "none", "max_completion_tokens": 5000, "response_format": map[string]string{"type": "json_object"}, "messages": []any{map[string]string{"role": "user", "content": prompt}}})
	ctx, cancel, resp, err := openaihttp.Post(parent, a.client, reviewTimeout, key, body)
	defer cancel()
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
	if err := decoder.Decode(&rawResult); err != nil || !hasFields(rawResult, "updatedRepository", "summary") || !profilevalidation.CompleteJSON(rawResult["updatedRepository"]) {
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

func mustMarshal(value any) []byte { encoded, _ := json.Marshal(value); return encoded }

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
