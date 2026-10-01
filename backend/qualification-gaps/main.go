package qualificationgaps

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"professional-information-repo/internal/openaihttp"
	"professional-information-repo/internal/profilevalidation"
)

const (
	maxRequestBytes = 128 << 10
	maxTextBytes    = 12 << 10
	maxPostingBytes = 30 << 10
	gapTimeout      = 25 * time.Second
	model           = "gpt-6-luna"
)

type repository = profilevalidation.Profile
type experience = profilevalidation.Experience
type project = profilevalidation.Project
type gapRequest struct {
	Repository repository `json:"repository"`
	JobPosting string     `json:"jobPosting"`
}

// These allowlisted types are the only profile fields that can reach the provider.
type providerProfile struct {
	Skills       string               `json:"skills"`
	Competencies string               `json:"competencies"`
	Tools        string               `json:"tools"`
	Experience   []providerExperience `json:"experience"`
	Projects     []providerProject    `json:"projects"`
}
type providerExperience struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	Responsibilities string `json:"responsibilities"`
	Achievements     string `json:"achievements"`
	Duration         string `json:"duration,omitempty"`
	Recency          string `json:"recency,omitempty"`
}
type providerProject struct {
	Description  string `json:"description"`
	Technologies string `json:"technologies"`
	Highlights   string `json:"highlights"`
}
type gap struct {
	Kind        string `json:"kind"`
	Requirement string `json:"requirement"`
	Details     string `json:"details"`
}
type gapResult struct {
	Gaps []gap `json:"gaps"`
}
type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
type app struct{ client *http.Client }

func isTimeout(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func NewHandler() http.Handler { return (app{client: &http.Client{Timeout: gapTimeout}}).handler() }
func (a app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/qualification-gaps", a.check)
	return mux
}
func (a app) check(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	started := time.Now()
	status, outcome := http.StatusOK, "ok"
	defer func() {
		log.Printf("qualification_gaps status=%d outcome=%s duration_ms=%d", status, outcome, time.Since(started).Milliseconds())
	}()
	key := strings.TrimSpace(r.Header.Get("X-OpenAI-Api-Key"))
	if !strings.HasPrefix(key, "sk-") || strings.HasPrefix(key, "sk-ant-") || len(key) > 512 {
		status, outcome = http.StatusUnauthorized, "key"
		writeError(w, status, outcome)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil || !hasExactFields(raw, "repository", "jobPosting") {
		status, outcome = http.StatusBadRequest, "input"
		writeError(w, status, outcome)
		return
	}
	var input gapRequest
	encoded, _ := json.Marshal(raw)
	if json.Unmarshal(encoded, &input) != nil || !completeRepository(raw["repository"]) {
		status, outcome = http.StatusBadRequest, "input"
		writeError(w, status, outcome)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || !validRequest(input) {
		status, outcome = http.StatusBadRequest, "input"
		writeError(w, status, outcome)
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
			status, code = http.StatusBadGateway, "outage"
		}
		outcome = code
		writeError(w, status, code)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}
func validRequest(in gapRequest) bool {
	if strings.TrimSpace(in.JobPosting) == "" || len(in.JobPosting) > maxPostingBytes {
		return false
	}
	return profilevalidation.Valid(in.Repository, maxTextBytes)
}

func hasExactFields(value map[string]json.RawMessage, fields ...string) bool {
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
	var repo map[string]json.RawMessage
	if json.Unmarshal(raw, &repo) != nil || !hasExactFields(repo, "careerGoals", "skills", "competencies", "experience", "tools", "projects", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo") {
		return false
	}
	for _, key := range []string{"careerGoals", "skills", "competencies", "tools", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo"} {
		if !jsonString(repo[key]) {
			return false
		}
	}
	for _, pair := range []struct {
		key    string
		fields []string
	}{
		{"experience", []string{"id", "company", "title", "startDate", "endDate", "current", "location", "description", "responsibilities", "achievements"}},
		{"projects", []string{"id", "name", "description", "technologies", "url", "highlights"}},
	} {
		if !jsonArray(repo[pair.key]) {
			return false
		}
		var entries []json.RawMessage
		if json.Unmarshal(repo[pair.key], &entries) != nil {
			return false
		}
		for _, entry := range entries {
			var fields map[string]json.RawMessage
			if json.Unmarshal(entry, &fields) != nil || !hasExactFields(fields, pair.fields...) {
				return false
			}
			for name, value := range fields {
				if name == "current" {
					s := strings.TrimSpace(string(value))
					if s != "true" && s != "false" {
						return false
					}
				} else if !jsonString(value) {
					return false
				}
			}
		}
	}
	return true
}

func jsonString(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return len(value) > 0 && value[0] == '"'
}

func jsonArray(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return len(value) > 0 && value[0] == '['
}
func toProviderProfile(r repository) providerProfile {
	out := providerProfile{Skills: r.Skills, Competencies: r.Competencies, Tools: r.Tools}
	for _, e := range r.Experience {
		duration, recency := experienceTiming(e.StartDate, e.EndDate, e.Current, time.Now())
		out.Experience = append(out.Experience, providerExperience{e.Title, e.Description, e.Responsibilities, e.Achievements, duration, recency})
	}
	for _, p := range r.Projects {
		out.Projects = append(out.Projects, providerProject{p.Description, p.Technologies, p.Highlights})
	}
	return out
}
func experienceTiming(startText, endText string, current bool, now time.Time) (string, string) {
	start, ok := parseMonthYear(startText)
	if !ok {
		return "", ""
	}
	end := now
	if !current {
		parsed, ok := parseMonthYear(endText)
		if !ok {
			return "", ""
		}
		end = time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
	if months < 0 {
		return "", ""
	}
	duration := fmtMonths(months)
	if current {
		return duration, "current"
	}
	ago := (now.Year()-end.Year())*12 + int(now.Month()-end.Month())
	if ago < 0 {
		return "", ""
	}
	return duration, fmtMonths(ago) + " ago"
}

func parseMonthYear(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse("2006-01", value); err == nil {
		return parsed, true
	}
	parts := strings.Fields(strings.ToLower(value))
	if len(parts) != 2 {
		return time.Time{}, false
	}
	monthName := strings.TrimSuffix(parts[0], ".")
	year, err := strconv.Atoi(parts[1])
	if err != nil || year < 1 || year > 9999 {
		return time.Time{}, false
	}
	months := map[string]time.Month{
		"jan": time.January, "january": time.January, "janeiro": time.January,
		"feb": time.February, "fev": time.February, "february": time.February, "fevereiro": time.February,
		"mar": time.March, "march": time.March, "março": time.March, "marco": time.March,
		"apr": time.April, "abr": time.April, "april": time.April, "abril": time.April,
		"may": time.May, "mai": time.May, "maio": time.May,
		"jun": time.June, "june": time.June, "junho": time.June,
		"jul": time.July, "july": time.July, "julho": time.July,
		"aug": time.August, "ago": time.August, "august": time.August, "agosto": time.August,
		"sep": time.September, "set": time.September, "sept": time.September, "september": time.September, "setembro": time.September,
		"oct": time.October, "out": time.October, "october": time.October, "outubro": time.October,
		"nov": time.November, "november": time.November, "novembro": time.November,
		"dec": time.December, "dez": time.December, "december": time.December, "dezembro": time.December,
	}
	month, ok := months[monthName]
	if !ok {
		return time.Time{}, false
	}
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC), true
}
func fmtMonths(months int) string {
	if months < 1 {
		return "under one month"
	}
	years, remain := months/12, months%12
	if years == 0 {
		return strconv.Itoa(remain) + " months"
	}
	if remain == 0 {
		return strconv.Itoa(years) + " years"
	}
	return strconv.Itoa(years) + " years " + strconv.Itoa(remain) + " months"
}
func (a app) callProvider(parent context.Context, key string, input gapRequest) (gapResult, string, error) {
	var empty gapResult
	profileJSON, _ := json.Marshal(toProviderProfile(input.Repository))
	prompt := "You are checking whether a candidate's professional profile may omit qualifications they already have.\n\nJOB POSTING:\n" + input.JobPosting + "\n\nCANDIDATE QUALIFICATION PROFILE (JSON):\n" + string(profileJSON) + "\n\nFind at most 5 specific skills or types of experience explicitly required or preferred by the posting that are not stated or clearly supported in the profile. This is only a memory prompt for the candidate; do not decide whether they truly have the qualification. Return only concrete qualifications from the posting, not generic traits or duties. Do not list equivalent support or infer gaps from missing keywords. If the posting is only a URL, too vague, or there are no plausible omitted qualifications, return an empty list. Keep requirement concise and details to one short sentence grounded in the posting. Return only JSON: {\"gaps\":[{\"kind\":\"skill\",\"requirement\":\"short qualification name\",\"details\":\"what the posting asks for\"}]}"
	body, _ := json.Marshal(map[string]any{"model": model, "reasoning_effort": "none", "max_completion_tokens": 1200, "response_format": map[string]string{"type": "json_object"}, "messages": []any{map[string]string{"role": "user", "content": prompt}}})
	ctx, cancel, resp, err := openaihttp.Post(parent, a.client, gapTimeout, key, body)
	defer cancel()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return empty, "timeout", err
		}
		return empty, "outage", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 401, 403:
		return empty, "key", errors.New("provider key rejected")
	case 429:
		return empty, "rate_limit", errors.New("provider rate limited")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return empty, "outage", errors.New("provider unavailable")
	}
	var upstream openAIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&upstream); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || (isTimeout(err)) {
			return empty, "timeout", err
		}
		return empty, "invalid_output", errors.New("provider response invalid")
	}
	if len(upstream.Choices) != 1 {
		return empty, "invalid_output", errors.New("provider response invalid")
	}
	var rawResult map[string]json.RawMessage
	if json.Unmarshal([]byte(strings.TrimSpace(upstream.Choices[0].Message.Content)), &rawResult) != nil || !hasExactFields(rawResult, "gaps") || !jsonArray(rawResult["gaps"]) {
		return empty, "invalid_output", errors.New("provider result missing gaps array")
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(upstream.Choices[0].Message.Content)))
	decoder.DisallowUnknownFields()
	var result gapResult
	if err := decoder.Decode(&result); err != nil {
		return empty, "invalid_output", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || !validResult(result) {
		return empty, "invalid_output", errors.New("invalid gap result")
	}
	return result, "", nil
}
func validResult(r gapResult) bool {
	if len(r.Gaps) > 5 {
		return false
	}
	for _, g := range r.Gaps {
		if (g.Kind != "skill" && g.Kind != "experience") || strings.TrimSpace(g.Requirement) == "" || strings.TrimSpace(g.Details) == "" || len(g.Requirement) > 200 || len(g.Details) > 1000 {
			return false
		}
	}
	return true
}
func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
