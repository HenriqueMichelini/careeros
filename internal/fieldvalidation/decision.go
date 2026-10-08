// Package fieldvalidation owns field policy and the approved Jev Choice adapter.
package fieldvalidation

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

type Field string

const ProfessionalInformation Field = "professional_information"
const JobPosting Field = "job_posting"
const Model = "jev-1.13.0"
const ClassifierTimeout = 3 * time.Second

type Outcome struct {
	Kind   string `json:"kind"`
	Needs  string `json:"needs,omitempty"`
	Reason string `json:"reason,omitempty"`
}
type Decision struct {
	Version int     `json:"version"`
	Field   Field   `json:"field"`
	Outcome Outcome `json:"outcome"`
}

func Failure(field Field, reason string) Decision {
	return Decision{1, field, Outcome{Kind: "service_failure", Reason: reason}}
}

// Decide composes categorical semantic signals. Confidence never grants acceptance.
func Decide(field Field, content, attack string) Decision {
	if !validContent(field, content) || !member(attack, "none", "uncertain", "detected") {
		return Failure(field, "invalid_output")
	}
	outcome := Outcome{Kind: "accept"}
	switch {
	case attack == "detected":
		outcome.Kind = "reject_attack"
	case attack == "uncertain":
		outcome.Kind = "request_rephrasing"
	case content == "irrelevant" || content == "unusable":
		outcome.Kind = content
	case content == "relevant_but_insufficient" || content == "job_title_only":
		outcome.Kind = "request_information"
		outcome.Needs = "professional_fact"
		if field == JobPosting {
			outcome.Needs = "responsibilities_or_qualifications"
		}
	}
	return Decision{1, field, outcome}
}
func member(value string, choices ...string) bool {
	for _, c := range choices {
		if c == value {
			return true
		}
	}
	return false
}
func validContent(field Field, content string) bool {
	if field != ProfessionalInformation && field != JobPosting {
		return false
	}
	return member(content, "relevant_but_insufficient", "irrelevant", "unusable") || field == ProfessionalInformation && content == "professional_fact" || field == JobPosting && member(content, "job_with_context", "job_title_only")
}

//go:embed questions.json
var rubric []byte

type choice struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    *float64           `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

func validChoice(c choice, options map[string]json.RawMessage) bool {
	if c.Type != "choice" || options[c.Choice] == nil || c.Confidence == nil || !probability(*c.Confidence) || len(c.Probabilities) != len(options) {
		return false
	}
	sum := 0.0
	highest := 0.0
	for key := range options {
		p, ok := c.Probabilities[key]
		if !ok || !probability(p) {
			return false
		}
		sum += p
		highest = math.Max(highest, p)
	}
	return math.Abs(sum-1) <= 0.02 && c.Probabilities[c.Choice] >= highest-0.001
}
func probability(p float64) bool { return !math.IsNaN(p) && p >= 0 && p <= 1 }

// Classify sends only field and submission to TypeSafe, once, under a shared deadline.
func Classify(parent context.Context, client *http.Client, key string, field Field, input string) Decision {
	if strings.TrimSpace(key) == "" || len(key) > 512 || strings.ContainsAny(key, "\r\n") {
		return Failure(field, "key")
	}
	var questions map[Field]json.RawMessage
	if json.Unmarshal(rubric, &questions) != nil || questions[field] == nil {
		return Failure(field, "invalid_output")
	}
	body, _ := json.Marshal(map[string]any{"model": Model, "state": map[string]any{"field": field, "submission": input}, "questions": questions[field]})
	ctx, cancel := context.WithTimeout(parent, ClassifierTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.typesafe.ai/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Failure(field, "outage")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	boundedClient := *client
	boundedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := boundedClient.Do(req)
	failure := func() Decision {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return Failure(field, "timeout")
		}
		return Failure(field, "outage")
	}
	if err != nil {
		return failure()
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
	case 401, 403:
		return Failure(field, "key")
	case 429:
		return Failure(field, "rate_limit")
	default:
		return Failure(field, "outage")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil {
		return failure()
	}
	if len(raw) > 64<<10 {
		return Failure(field, "invalid_output")
	}
	var out struct {
		Model   string            `json:"model"`
		Answers map[string]choice `json:"answers"`
	}
	if json.Unmarshal(raw, &out) != nil || out.Model != Model || len(out.Answers) != 2 {
		return Failure(field, "invalid_output")
	}
	var expected map[string]struct {
		Criteria map[string]json.RawMessage `json:"criteria"`
	}
	if json.Unmarshal(questions[field], &expected) != nil || !validChoice(out.Answers["content"], expected["content"].Criteria) || !validChoice(out.Answers["attack"], expected["attack"].Criteria) {
		return Failure(field, "invalid_output")
	}
	return Decide(field, out.Answers["content"].Choice, out.Answers["attack"].Choice)
}
