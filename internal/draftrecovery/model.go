// Package draftrecovery owns restricted-purpose durable Application Draft
// publication, including its local managed generation handoff. Delivery is separate.
package draftrecovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
)

const (
	BundleVersion     = "application-draft-bundle-v1"
	GenerationVersion = "controlled-complete-draft-v1"
	ReviewVersion     = "application-draft-review-snapshot-v1"
	CompletionVersion = "complete-draft-eligibility-v1"
	CustomerWindow    = 30 * time.Minute
	MaxBundleBytes    = accountstore.MaxDocumentBytes
)

var (
	ErrInvalid  = errors.New("invalid complete Draft")
	ErrConflict = errors.New("Draft binding changed")
	ErrDenied   = errors.New("Draft authority unavailable")
	ErrNotFound = errors.New("Draft not found")
	ErrPending  = errors.New("Draft result unavailable")
)

type Source struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}
type JobRange struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Quote string `json:"quote"`
}
type Analysis struct {
	Version        string     `json:"version"`
	Requirements   []JobRange `json:"requirements"`
	AnswerRequests []JobRange `json:"answerRequests"`
}
type Job struct {
	Posting       string               `json:"posting"`
	Analysis      *Analysis            `json:"analysis,omitempty"`
	Qualification *QualificationSource `json:"qualification,omitempty"`
}
type QualificationSource struct {
	ActionID      string `json:"actionId"`
	ContextDigest string `json:"contextDigest"`
}
type Answer struct {
	Requirement string `json:"requirement"`
	UserContext string `json:"userContext"`
	Kind        string `json:"kind,omitempty"`
}
type Input struct {
	Profile         Source   `json:"profileSource"`
	Job             Job      `json:"job"`
	Answers         []Answer `json:"answers"`
	Confirmations   []Answer `json:"confirmations"`
	SelectedFactIDs []string `json:"selectedFactIds"`
	Language        string   `json:"language"`
}
type Action struct {
	Namespace string `json:"namespace"`
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Workflow  string `json:"workflow"`
}
type Binding struct {
	Action            Action `json:"action"`
	Profile           Source `json:"profileSource"`
	SourceDigest      string `json:"sourceDigest"`
	InputDigest       string `json:"inputDigest"`
	Language          string `json:"language"`
	GenerationVersion string `json:"generationVersion"`
	ReviewVersion     string `json:"reviewVersion"`
}
type Identity struct {
	FullName          string `json:"fullName"`
	Email             string `json:"email"`
	Phone             string `json:"phone"`
	Location          string `json:"location"`
	ProfessionalLinks string `json:"professionalLinks"`
}
type Reference struct {
	Kind      string `json:"kind"`
	ProfileID string `json:"profileId"`
	ID        string `json:"id"`
	Revision  int64  `json:"revision"`
}
type Fact struct {
	Reference Reference       `json:"reference"`
	Snapshot  json.RawMessage `json:"snapshot"`
}
type Claim struct {
	Field      string      `json:"field,omitempty"`
	Text       string      `json:"text"`
	State      string      `json:"state"`
	Nonfactual bool        `json:"nonfactual"`
	Sources    []Reference `json:"sources"`
	JobSources []JobRange  `json:"jobSources"`
	Concerns   []string    `json:"concerns"`
}
type Support struct {
	Version    string      `json:"version"`
	Check      string      `json:"check"`
	Facts      []Fact      `json:"facts"`
	Resume     []Claim     `json:"resume"`
	Artifacts  []Claim     `json:"artifacts"`
	Concerns   []string    `json:"concerns"`
	Completion *Completion `json:"completion,omitempty"`
}
type AnswerRequirement struct {
	ID     string   `json:"id"`
	Source JobRange `json:"source"`
}
type Completion struct {
	Version         string              `json:"version"`
	State           string              `json:"state"`
	CandidateDigest string              `json:"candidateDigest"`
	InputDigest     string              `json:"inputDigest"`
	AnswerRequests  []AnswerRequirement `json:"answerRequests"`
	Concerns        []string            `json:"concerns"`
}
type ReviewRequest struct {
	Generation      Generation
	Candidate       Bundle
	CandidateDigest string
}
type ControlledReview func(context.Context, ReviewRequest) (Completion, error)

// Publication can only be prepared by the repository after its independent
// controlled review. Its fields are private; HTTP has no publication endpoint.
type Publication struct {
	raw     []byte
	seal    string
	session *aicontrol.Session
}
type AnswerRequest struct {
	ID     string   `json:"id"`
	Source JobRange `json:"source"`
	Answer string   `json:"answer"`
}
type EmployerAnswers struct {
	State    string          `json:"state"`
	Text     string          `json:"text"`
	Requests []AnswerRequest `json:"requests"`
}
type Bundle struct {
	Schema      string          `json:"schema"`
	Binding     Binding         `json:"binding"`
	JobTitle    *string         `json:"jobTitle"`
	Company     *string         `json:"company"`
	RoleSummary string          `json:"roleSummary"`
	Resume      string          `json:"resume"`
	CoverLetter string          `json:"coverLetter"`
	Identity    Identity        `json:"identity"`
	Answers     EmployerAnswers `json:"employerAnswers"`
	Support     Support         `json:"support"`
}
type Result struct {
	Action        Action           `json:"action"`
	CreationKey   string           `json:"creationKey"`
	State         string           `json:"state"`
	Version       int              `json:"artifactVersion"`
	AdmittedAt    time.Time        `json:"admittedAt"`
	Deadline      time.Time        `json:"deadline"`
	Historical    bool             `json:"historical"`
	CleanupQueued bool             `json:"cleanupQueued"`
	Attempt       int              `json:"attempt,omitempty"`
	RetryAllowed  bool             `json:"retryAllowed,omitempty"`
	Bundle        *Bundle          `json:"bundle,omitempty"`
	BundleDigest  string           `json:"bundleDigest,omitempty"`
	Delivered     bool             `json:"delivered,omitempty"`
	Opening       *OpeningStatus   `json:"opening,omitempty"`
	Review        *HumanReview     `json:"review,omitempty"`
	Job           *Job             `json:"job,omitempty"`
	Context       *RetainedContext `json:"context,omitempty"`
}
type RetainedContext struct {
	Answers         []Answer `json:"answers"`
	Confirmations   []Answer `json:"confirmations"`
	SelectedFactIDs []string `json:"selectedFactIds"`
}

// Authority is supplied to trusted server adapters only, never decoded from HTTP.
// Mutations of injected grant/deletion authority must take the same account lock.
type Authority struct {
	Transaction *sql.Tx
	Principal   auth.Principal
	Action      Action
	Now         time.Time
}
type Decision struct {
	Generate, Publish, Read, Delivered, Deleted, Revoked, Cancelled bool
	GrantExpiresAt                                                  time.Time
	Epoch                                                           uint64
}
type Decisions interface {
	Decide(context.Context, Authority) (Decision, error)
}
type DecisionFunc func(context.Context, Authority) (Decision, error)

func (f DecisionFunc) Decide(ctx context.Context, a Authority) (Decision, error) { return f(ctx, a) }

type Config struct {
	BindingKey []byte
	Decisions  Decisions
	Actions    *aicontrol.Harness
	Review     ControlledReview
	Fault      func(string) error
}

// Generation's source facts/identity exist only in memory during one controlled
// invocation. Only facts actually used by complete support snapshots are retained.
type Generation struct {
	Input      Input
	Binding    Binding
	Identity   Identity
	Facts      []Fact
	document   profiledocument.Document
	session    *aicontrol.Session
	jobContext json.RawMessage
}
type ControlledProvider func(context.Context, Generation) (Bundle, error)

func validToken(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func validRange(r JobRange, source string) bool {
	return r.Start >= 0 && r.End > r.Start && r.End <= len(source) && utf8.ValidString(source[:r.Start]) && utf8.ValidString(source[r.Start:r.End]) && source[r.Start:r.End] == r.Quote
}
func normalize(in Input, managed bool) (Input, []byte, error) {
	if in.Profile.ID == "" || in.Profile.Revision < 1 || in.Profile.Revision > 9007199254740991 || (in.Language != "en" && in.Language != "pt-BR") || !profiledocument.HasText(in.Job.Posting) || len(in.Job.Posting) > 30<<10 || !utf8.ValidString(in.Job.Posting) || len(in.Answers) > 100 || len(in.Confirmations) > 100 || len(in.SelectedFactIDs) > 500 {
		return in, nil, ErrInvalid
	}
	if in.Answers == nil {
		in.Answers = []Answer{}
	}
	if in.Confirmations == nil {
		in.Confirmations = []Answer{}
	}
	if in.SelectedFactIDs == nil {
		in.SelectedFactIDs = []string{}
	}
	for _, a := range in.Answers {
		if !validAnswer(a) || !profiledocument.HasText(a.UserContext) || a.Kind != "" {
			return in, nil, ErrInvalid
		}
	}
	for _, a := range in.Confirmations {
		if !validAnswer(a) || (!managed && (!profiledocument.HasText(a.UserContext) || a.Kind != "")) ||
			(managed && a.Kind != "" && a.Kind != "skill" && a.Kind != "experience") {
			return in, nil, ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, id := range in.SelectedFactIDs {
		if id == "" || seen[id] {
			return in, nil, ErrInvalid
		}
		seen[id] = true
	}
	if a := in.Job.Analysis; a != nil {
		if a.Version != "controlled-job-analysis-v1" || a.Requirements == nil || a.AnswerRequests == nil || len(a.Requirements) > 100 || len(a.AnswerRequests) > 100 {
			return in, nil, ErrInvalid
		}
		for _, r := range append(append([]JobRange{}, a.Requirements...), a.AnswerRequests...) {
			if !validRange(r, in.Job.Posting) {
				return in, nil, ErrInvalid
			}
		}
	}
	raw, err := json.Marshal(in)
	if err != nil || !accountstore.StrictJSON(raw) {
		return in, nil, ErrInvalid
	}
	return in, raw, nil
}

func validAnswer(a Answer) bool {
	return profiledocument.HasText(a.Requirement) && len(a.Requirement) <= 4000 && len(a.UserContext) <= 2000
}

func equalJSON(a, b []byte) bool {
	var x, y any
	return accountstore.StrictJSON(a) && accountstore.StrictJSON(b) && json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}
func refKey(r Reference) string { b, _ := json.Marshal(r); return string(b) }

// ValidateComplete checks actual correspondence, not a provider's complete flag.
// Semantic/model and rendered-layout quality remain independently tested with
// controlled fixtures; this does not certify a live provider or arbitrary prose.
func ValidateComplete(g Generation, b Bundle) error {
	if b.Schema != BundleVersion || !reflect.DeepEqual(b.Binding, g.Binding) || !reflect.DeepEqual(b.Identity, g.Identity) || b.Support.Version != ReviewVersion || b.Support.Check != "complete" || b.Support.Concerns == nil || len(b.Support.Concerns) != 0 || b.Support.Facts == nil || b.Support.Resume == nil || b.Support.Artifacts == nil || len(b.Support.Resume) == 0 || len(b.Support.Resume) > 80 || len(b.Support.Artifacts) > 100 || len(b.Support.Facts) > 500 {
		return ErrInvalid
	}
	for _, v := range []string{b.RoleSummary, b.Resume, b.CoverLetter} {
		if !profiledocument.HasText(v) || len(v) > 20000 {
			return ErrInvalid
		}
	}
	if profiledocument.WhitespaceSplitCount(profiledocument.TrimText(b.Resume)) > 400 {
		return ErrInvalid
	}
	if !applicationdraft.ValidCompletedCoverLetter(b.CoverLetter, g.Identity.FullName) {
		return ErrInvalid
	}
	for _, v := range []*string{b.JobTitle, b.Company} {
		if v != nil && (!profiledocument.HasText(*v) || len(*v) > 1000) {
			return ErrInvalid
		}
	}
	allowed := map[string]Fact{}
	for _, f := range g.Facts {
		allowed[refKey(f.Reference)] = f
	}
	retained := map[string]bool{}
	for _, f := range b.Support.Facts {
		k := refKey(f.Reference)
		before, ok := allowed[k]
		if !ok || retained[k] || !equalJSON(before.Snapshot, f.Snapshot) {
			return ErrInvalid
		}
		retained[k] = true
	}
	used := map[string]bool{}
	validate := func(c Claim) bool {
		if !profiledocument.HasText(c.Text) || len(c.Text) > 20000 || c.State != "supported" || c.Concerns == nil || len(c.Concerns) != 0 || c.Sources == nil || c.JobSources == nil || len(c.Sources) > 16 || len(c.JobSources) > 16 {
			return false
		}
		if !c.Nonfactual && len(c.Sources)+len(c.JobSources) == 0 {
			return false
		}
		if (c.Field == "jobTitle" || c.Field == "company") && (c.Nonfactual || len(c.JobSources) == 0) {
			return false
		}
		seen := map[string]bool{}
		for _, r := range c.Sources {
			k := refKey(r)
			if !retained[k] || seen[k] {
				return false
			}
			seen[k] = true
			used[k] = true
		}
		for _, r := range c.JobSources {
			if !validRange(r, g.Input.Job.Posting) {
				return false
			}
		}
		return true
	}
	var resume []string
	for _, c := range b.Support.Resume {
		if c.Field != "" || !validate(c) {
			return ErrInvalid
		}
		if c.Nonfactual {
			if !structuralHeading(c.Text, g.Input.Language) || len(c.Sources) > 0 || len(c.JobSources) > 0 {
				return ErrInvalid
			}
		} else if len(c.Sources) == 0 {
			return ErrInvalid
		}
		resume = append(resume, c.Text)
	}
	if strings.Join(resume, "\n\n") != b.Resume {
		return ErrInvalid
	}
	expected := map[string]string{"roleSummary": b.RoleSummary, "coverLetter": b.CoverLetter, "applicationAnswers": b.Answers.Text}
	if b.JobTitle != nil {
		expected["jobTitle"] = *b.JobTitle
	}
	if b.Company != nil {
		expected["company"] = *b.Company
	}
	fields := map[string][]string{}
	for _, c := range b.Support.Artifacts {
		if _, ok := expected[c.Field]; !ok || !validate(c) {
			return ErrInvalid
		}
		fields[c.Field] = append(fields[c.Field], c.Text)
	}
	for k, v := range expected {
		if strings.Join(fields[k], "\n\n") != v {
			return ErrInvalid
		}
	}
	for k := range retained {
		if !used[k] {
			return ErrInvalid
		}
	}
	if b.Answers.Requests == nil {
		return ErrInvalid
	}
	switch b.Answers.State {
	case "none_requested":
		if len(b.Answers.Requests) != 0 || b.Answers.Text != "" {
			return ErrInvalid
		}
		if g.Input.Job.Analysis != nil && len(g.Input.Job.Analysis.AnswerRequests) > 0 {
			return ErrInvalid
		}
	case "complete":
		if len(b.Answers.Requests) == 0 || len(b.Answers.Requests) > 100 || !profiledocument.HasText(b.Answers.Text) {
			return ErrInvalid
		}
		ids := map[string]bool{}
		var texts []string
		for _, r := range b.Answers.Requests {
			if !validToken(r.ID) || ids[r.ID] || !validRange(r.Source, g.Input.Job.Posting) || !profiledocument.HasText(r.Answer) {
				return ErrInvalid
			}
			ids[r.ID] = true
			texts = append(texts, r.Answer)
		}
		if strings.Join(texts, "\n\n") != b.Answers.Text {
			return ErrInvalid
		}
		if a := g.Input.Job.Analysis; a != nil {
			if len(a.AnswerRequests) != len(b.Answers.Requests) {
				return ErrInvalid
			}
			for i, r := range a.AnswerRequests {
				if !reflect.DeepEqual(r, b.Answers.Requests[i].Source) {
					return ErrInvalid
				}
			}
		}
	default:
		return ErrInvalid
	}
	raw, err := json.Marshal(b)
	if err != nil || len(raw) > MaxBundleBytes || !accountstore.StrictJSON(raw) {
		return ErrInvalid
	}
	return nil
}

func structuralHeading(text, language string) bool {
	headings := []string{"Professional Summary", "Technical Skills", "Professional Experience", "Education", "Certifications", "Languages"}
	if language == "pt-BR" {
		headings = []string{"Resumo Profissional", "Competências Técnicas", "Experiência Profissional", "Educação", "Certificações", "Idiomas"}
	}
	for _, h := range headings {
		if text == "## "+h {
			return true
		}
	}
	return false
}
