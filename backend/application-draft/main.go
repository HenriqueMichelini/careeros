package applicationdraft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"professional-information-repo/internal/openaihttp"
	"professional-information-repo/internal/profilevalidation"
)

const (
	maxRequestBytes = 128 << 10
	timeout         = 25 * time.Second
	model           = "gpt-6-luna"
)

type profile = profilevalidation.Profile
type request struct {
	Profile        profile                           `json:"repository"`
	JobPosting     string                            `json:"jobPosting"`
	Confirmed      []qualification                   `json:"confirmedQualifications"`
	Qualifications *profilevalidation.Qualifications `json:"qualifications,omitempty"`
}
type qualification struct {
	Kind        string `json:"kind"`
	Requirement string `json:"requirement"`
	UserContext string `json:"userContext"`
}
type result struct {
	JobTitle           string      `json:"jobTitle"`
	Company            string      `json:"company"`
	JobSummary         string      `json:"jobSummary"`
	Resume             string      `json:"resume"`
	CoverLetter        coverLetter `json:"coverLetter"`
	ApplicationAnswers string      `json:"applicationAnswers"`
}
type coverLetter struct {
	Greeting string `json:"greeting"`
	Body     string `json:"body"`
	Closing  string `json:"closing"`
}

type upstream struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
type app struct{ client *http.Client }

func NewHandler() http.Handler { return (app{&http.Client{Timeout: timeout}}).handler() }
func (a app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/application-draft", a.generate)
	return mux
}
func (a app) generate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	started := time.Now()
	status, outcome := 200, "ok"
	defer func() {
		log.Printf("application_draft status=%d outcome=%s duration_ms=%d", status, outcome, time.Since(started).Milliseconds())
	}()
	key := strings.TrimSpace(r.Header.Get("X-OpenAI-Api-Key"))
	if !strings.HasPrefix(key, "sk-") || strings.HasPrefix(key, "sk-ant-") || len(key) > 512 {
		status, outcome = 401, "key"
		writeError(w, status, outcome)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	d := json.NewDecoder(r.Body)
	var raw map[string]json.RawMessage
	if d.Decode(&raw) != nil || !completeInputShape(raw) {
		status, outcome = 400, "input"
		writeError(w, status, outcome)
		return
	}
	encoded, _ := json.Marshal(raw)
	var in request
	if json.Unmarshal(encoded, &in) != nil {
		status, outcome = 400, "input"
		writeError(w, status, outcome)
		return
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF || strings.TrimSpace(in.JobPosting) == "" || len(in.JobPosting) > 30<<10 || !profilevalidation.Valid(in.Profile, 12<<10) || len(in.Confirmed) > 25 ||
		(in.Qualifications != nil && !profilevalidation.ValidQualifications(*in.Qualifications)) {
		status, outcome = 400, "input"
		writeError(w, status, outcome)
		return
	}
	for _, q := range in.Confirmed {
		if (q.Kind != "skill" && q.Kind != "experience") || strings.TrimSpace(q.Requirement) == "" || len(q.Requirement) > 2000 || len(q.UserContext) > 2000 {
			status, outcome = 400, "input"
			writeError(w, status, outcome)
			return
		}
	}
	out, code, err := a.call(r.Context(), key, in)
	if err != nil {
		status = 502
		switch code {
		case "key":
			status = 401
		case "rate_limit":
			status = 429
		case "timeout":
			status = 504
		}
		outcome = code
		writeError(w, status, code)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}
func (a app) call(parent context.Context, key string, in request) (result, string, error) {
	var empty result
	repo, _ := json.Marshal(in.Profile)
	repo = omitEmptyProfileFields(repo)
	quals, _ := json.Marshal(in.Confirmed)
	structured := []byte("{}")
	if in.Qualifications != nil {
		structured, _ = json.Marshal(qualificationFacts(*in.Qualifications))
	}
	prompt := "You are an expert career coach and professional writer. Generate highly personalized application materials from the candidate's full professional profile and this job posting. Draw specifically on real experience, skills, projects, education, certifications, and languages; tailor every sentence to the role; mirror the posting's tone; and never invent employers, dates, proficiency, duration, examples, outcomes, or other facts. The resume must be clean Markdown with these exact level-two headings in this order when supported by Profile facts: Professional Summary, Technical Skills, Professional Experience, Education, Certifications, Languages. Omit any unsupported section. Put tools and competencies under Technical Skills and relevant projects under Professional Experience; do not add separate project or tools sections. Use level-three headings for experience, project, and education entries and bullets for supporting details. Keep the resume concise enough for one A4 page, aiming for roughly 400 words or fewer; prioritize the most relevant verified evidence without inventing facts. Do not add a name or contact header because the client supplies it from saved Profile facts. Do not use sample values or placeholders. The cover letter must be specific and under 400 words including the signature the application will append. Return coverLetter as an object with exactly greeting, body, and closing. greeting is a single-line salutation to the hiring team. body contains only tailored prose paragraphs, each line ending in sentence punctuation. closing is exactly one of: Sincerely,; Kind regards,; Best regards,; Atenciosamente,; Cordialmente,. Do not include any candidate name, signature, identity placeholder, or contact detail in any part; the application appends the saved Profile name locally. Never include a closing or signature inside body. Provide 5-6 useful application answers in Markdown. Qualifications listed below were explicitly confirmed for this application only. Use them as relevant, but if no candidate context is supplied, mention only the qualification and do not imply a specific achievement or work history. Do not add confirmed qualifications to the saved profile. Return only JSON with exactly these fields: jobTitle, company, jobSummary, resume, applicationAnswers (non-empty strings), and coverLetter (the object described above).\nPROFILE:\n" + string(repo) + "\nSTRUCTURED PROFILE QUALIFICATIONS:\n" + string(structured) + "\nJOB POSTING:\n" + in.JobPosting + "\nUSER-CONFIRMED QUALIFICATIONS FOR THIS DRAFT ONLY:\n" + string(quals)
	body, _ := json.Marshal(map[string]any{"model": model, "reasoning_effort": "none", "max_completion_tokens": 8000, "response_format": map[string]string{"type": "json_object"}, "messages": []any{map[string]string{"role": "user", "content": prompt}}})
	ctx, cancel, resp, err := openaihttp.Post(parent, a.client, timeout, key, body)
	defer cancel()
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return empty, "timeout", err
		}
		return empty, "outage", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return empty, "key", errors.New("key")
	}
	if resp.StatusCode == 429 {
		return empty, "rate_limit", errors.New("rate limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return empty, "outage", errors.New("provider unavailable")
	}
	var u upstream
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&u); err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return empty, "timeout", err
		}
		return empty, "invalid_output", err
	}
	if len(u.Choices) != 1 {
		return empty, "invalid_output", errors.New("invalid response")
	}
	d := json.NewDecoder(strings.NewReader(u.Choices[0].Message.Content))
	d.DisallowUnknownFields()
	if d.Decode(&empty) != nil {
		return result{}, "invalid_output", errors.New("invalid output")
	}
	var tail any
	if d.Decode(&tail) != io.EOF || !valid(empty) {
		return result{}, "invalid_output", errors.New("invalid output")
	}
	return empty, "", nil
}
func qualificationFacts(q profilevalidation.Qualifications) map[string]any {
	education := make([]map[string]string, 0, len(q.Education))
	for _, e := range q.Education {
		if strings.TrimSpace(e.Degree) == "" && strings.TrimSpace(e.Institution) == "" {
			continue
		}
		education = append(education, map[string]string{"degree": e.Degree, "institution": e.Institution, "location": e.Location, "graduationDate": e.GraduationDate, "details": e.Details})
	}
	certifications := make([]map[string]string, 0, len(q.Certifications))
	for _, c := range q.Certifications {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		certifications = append(certifications, map[string]string{"name": c.Name, "issuer": c.Issuer, "date": c.Date, "credentialId": c.CredentialID, "url": c.URL})
	}
	languages := make([]map[string]string, 0, len(q.Languages))
	for _, l := range q.Languages {
		if strings.TrimSpace(l.Name) == "" {
			continue
		}
		languages = append(languages, map[string]string{"name": l.Name, "proficiency": l.Proficiency})
	}
	return map[string]any{"education": education, "certifications": certifications, "languages": languages}
}
func valid(r result) bool {
	return strings.TrimSpace(r.JobTitle) != "" && strings.TrimSpace(r.Company) != "" && strings.TrimSpace(r.JobSummary) != "" && strings.TrimSpace(r.Resume) != "" && validCoverLetter(r.CoverLetter) && strings.TrimSpace(r.ApplicationAnswers) != ""
}

var signatureLine = regexp.MustCompile(`^(?:(?:Dr|Dra|Mr|Ms|Mrs|Sr|Sra)\.?\s+)?\p{Lu}[\p{L}'’.-]*(?:\s+(?:\p{Lu}[\p{L}'’.-]*|da|de|do|dos|das|van|von|der))+(?:,\s*\p{Lu}[\p{L}.'’-]*(?:\s+\p{Lu}[\p{L}.'’-]*)*)?$`)

var introducedIdentity = regexp.MustCompile(`(?:My name is|Meu nome é|Me chamo)\s+`)

// Structured parts keep the model outside the application-owned signature slot.
func validCoverLetter(c coverLetter) bool {
	if strings.TrimSpace(c.Greeting) == "" || strings.ContainsAny(strings.TrimSpace(c.Greeting), "\r\n") || strings.TrimSpace(c.Body) == "" || introducedIdentity.MatchString(c.Body) {
		return false
	}
	switch strings.TrimSpace(c.Closing) {
	case "Sincerely,", "Kind regards,", "Best regards,", "Atenciosamente,", "Cordialmente,":
	default:
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(c.Body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && (!strings.ContainsAny(line[len(line)-1:], ".!?") || signatureLine.MatchString(line)) {
			return false
		}
	}
	return len(strings.Fields(c.Greeting+" "+c.Body+" "+c.Closing)) < 400
}

func exactFields(raw json.RawMessage, expected ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, ok := fields[key]; !ok {
			return false
		}
	}
	return true
}
func completeInputShape(raw map[string]json.RawMessage) bool {
	if len(raw) != 3 && len(raw) != 4 {
		return false
	}
	for _, key := range []string{"repository", "jobPosting", "confirmedQualifications"} {
		if _, ok := raw[key]; !ok {
			return false
		}
	}
	if len(raw) == 4 && (raw["qualifications"] == nil || !profilevalidation.CompleteQualifications(raw["qualifications"])) {
		return false
	}
	if !jsonString(raw["jobPosting"]) {
		return false
	}
	if !exactFields(raw["repository"], "careerGoals", "skills", "competencies", "experience", "tools", "projects", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo") {
		return false
	}
	var repo map[string]json.RawMessage
	_ = json.Unmarshal(raw["repository"], &repo)
	for _, key := range []string{"careerGoals", "skills", "competencies", "tools", "employmentStatus", "currentSalary", "desiredSalary", "additionalInfo"} {
		if !jsonString(repo[key]) {
			return false
		}
	}
	for _, group := range []struct {
		key    string
		fields []string
	}{{"experience", []string{"id", "company", "title", "startDate", "endDate", "current", "location", "description", "responsibilities", "achievements"}}, {"projects", []string{"id", "name", "description", "technologies", "url", "highlights"}}} {
		if !jsonArray(repo[group.key]) {
			return false
		}
		var items []json.RawMessage
		if json.Unmarshal(repo[group.key], &items) != nil {
			return false
		}
		for _, item := range items {
			if !exactFields(item, group.fields...) {
				return false
			}
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(item, &fields)
			for key, value := range fields {
				if group.key == "experience" && key == "current" {
					if !jsonBoolean(value) {
						return false
					}
				} else if !jsonString(value) {
					return false
				}
			}
		}
	}
	if !jsonArray(raw["confirmedQualifications"]) {
		return false
	}
	var quals []json.RawMessage
	if json.Unmarshal(raw["confirmedQualifications"], &quals) != nil {
		return false
	}
	for _, q := range quals {
		if !exactFields(q, "kind", "requirement", "userContext") {
			return false
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(q, &fields)
		for _, key := range []string{"kind", "requirement", "userContext"} {
			if !jsonString(fields[key]) {
				return false
			}
		}
	}
	return true
}

func jsonString(raw json.RawMessage) bool {
	rawValue := strings.TrimSpace(string(raw))
	if len(rawValue) == 0 || rawValue[0] != '"' {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) == nil
}

func jsonBoolean(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value == "true" || value == "false"
}

func jsonArray(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return len(value) > 0 && value[0] == '['
}

// Empty optional fields are omitted while all populated profile fields are retained.
func omitEmptyProfileFields(raw []byte) []byte {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return raw
	}
	for k, v := range fields {
		var s string
		if json.Unmarshal(v, &s) == nil && s == "" {
			delete(fields, k)
			continue
		}
		var a []json.RawMessage
		if json.Unmarshal(v, &a) == nil && len(a) == 0 {
			delete(fields, k)
		}
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return out
}
func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
