package applicationdraft

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/profiledocument"
)

type ManagedAnswer struct {
	Requirement, UserContext, Kind string
}

// ManagedInput is constructed only from the frozen server admission. The
// existing handler pipeline uses that Profile, never a caller repository.
type ManagedInput struct {
	Profile         profiledocument.Document
	Posting         string
	Language        string
	JobContext      json.RawMessage
	SelectedFactIDs []string
	Answers         []ManagedAnswer
	Confirmations   []ManagedAnswer
	ResumeNotNeeded func(context.Context) error
}

type ManagedFailure struct {
	Code string
}

func (e *ManagedFailure) Error() string { return e.Code }

// The output can be constructed only by the existing pipeline. Recovery maps
// it to the restricted bundle after it has verified the managed Action.
type ManagedResult struct{ result }

type ManagedCitation struct {
	Start, End int
	Quote      string
}

type ManagedClaim struct {
	Field, Text, State string
	Nonfactual         bool
	Sources            []profiledocument.Reference
	JobSources         []ManagedCitation
	Concerns           []string
}

type ManagedSnapshot struct {
	JobTitle, Company                *string
	RoleSummary, Resume, CoverLetter string
	AnswerText                       string
	ResumeClaims, ArtifactClaims     []ManagedClaim
	AnswerRequests                   []ManagedCitation
}

// GenerateManaged runs the real gate and generation/review stages. The nil
// network client and required controller keep this entry point egress-free.
func GenerateManaged(ctx context.Context, input ManagedInput) (ManagedResult, error) {
	workflow, root := aidispatch.WorkflowRoot(ctx)
	if workflow != "application_draft" || len(root) == 0 || input.ResumeNotNeeded == nil {
		return ManagedResult{}, &ManagedFailure{Code: "authority"}
	}
	ctx = aidispatch.RequireManaged(ctx)
	doc := input.Profile
	if len(input.SelectedFactIDs) > 0 {
		doc.Facts = slices.DeleteFunc(slices.Clone(doc.Facts), func(f profiledocument.Fact) bool {
			return !slices.Contains(input.SelectedFactIDs, f.ID)
		})
	}
	in := request{
		ReviewArtifacts: true, ReviewResume: true, managed: true, resumeNotNeeded: input.ResumeNotNeeded,
		SelectedFactIDs: input.SelectedFactIDs, ProfileEvidence: &doc, JobPosting: input.Posting,
		JobContext: input.JobContext, CvLanguage: input.Language, Confirmed: []qualification{},
	}
	for _, answer := range input.Answers {
		in.Answers = append(in.Answers, qualificationAnswer{answer.Requirement, answer.UserContext})
	}
	for _, answer := range input.Confirmations {
		kind := answer.Kind
		if kind == "" {
			kind = "skill"
		}
		in.Confirmed = append(in.Confirmed, qualification{kind, answer.Requirement, answer.UserContext})
	}
	// The legacy adapters require a nonempty local reference. Session.Send
	// removes all authorization headers before invoking the fake responder.
	const syntheticReference = "controlled-provider-reference"
	decision := fieldvalidation.Classify(ctx, nil, syntheticReference, fieldvalidation.JobPosting, in.JobPosting)
	if decision.Outcome.Kind != "accept" {
		if decision.Outcome.Kind == "service_failure" {
			return ManagedResult{}, &ManagedFailure{Code: decision.Outcome.Reason}
		}
		return ManagedResult{}, &ManagedFailure{Code: "job_rejected"}
	}
	out, code, err := (app{}).call(ctx, syntheticReference, in)
	if err != nil {
		return ManagedResult{}, &ManagedFailure{Code: code}
	}
	return ManagedResult{out}, nil
}

// CompleteSnapshot covers the exact original output and the checker's answer
// catalog. A successful HTTP envelope or a missing judgment is insufficient.
func (r ManagedResult) CompleteSnapshot(language, fullName string) (ManagedSnapshot, error) {
	if r.ResumeReview == nil || r.ResumeReview.Check != "complete" || r.ArtifactReview == nil ||
		r.ArtifactReview.Check != "complete" || r.ArtifactReview.answers == nil || len(r.ArtifactReview.concerns) != 0 {
		return ManagedSnapshot{}, &ManagedFailure{Code: "review_incomplete"}
	}
	snapshot := ManagedSnapshot{
		JobTitle: r.JobTitle, Company: r.Company, RoleSummary: r.JobSummary, Resume: r.Resume,
		AnswerText: r.ApplicationAnswers, AnswerRequests: managedCitations(r.ArtifactReview.answers),
	}
	claims := make([]resumeClaim, len(r.ResumeReview.Claims))
	for i, claim := range r.ResumeReview.Claims {
		claims[i] = claim.resumeClaim
	}
	for _, line := range orderedResumeLines(claims, language) {
		claim := r.ResumeReview.Claims[line.index]
		snapshot.ResumeClaims = append(snapshot.ResumeClaims, ManagedClaim{Text: line.text, State: claim.State, Sources: claim.Sources, Concerns: claim.Concerns})
	}
	for _, claim := range r.ArtifactReview.Claims {
		field := claim.Field
		switch field {
		case "jobSummary":
			field = "roleSummary"
		case "greeting", "body":
			field = "coverLetter"
		}
		snapshot.ArtifactClaims = append(snapshot.ArtifactClaims, ManagedClaim{
			Field: field, Text: claim.Text, State: claim.State, Nonfactual: claim.Nonfactual,
			Sources: claim.Sources, JobSources: managedCitations(claim.JobSources), Concerns: claim.Concerns,
		})
	}
	closing := r.CoverLetter.Closing
	if name := profiledocument.TrimText(fullName); name != "" {
		closing += "\n" + name
	}
	snapshot.CoverLetter = strings.Join([]string{r.CoverLetter.Greeting, r.CoverLetter.Body, closing}, "\n\n")
	snapshot.ArtifactClaims = append(snapshot.ArtifactClaims, ManagedClaim{Field: "coverLetter", Text: closing, State: "supported", Nonfactual: true})
	if !ValidCompletedCoverLetter(snapshot.CoverLetter, fullName) {
		return ManagedSnapshot{}, &ManagedFailure{Code: "review_incomplete"}
	}
	return snapshot, nil
}

func managedCitations(sources []jobSupport) []ManagedCitation {
	out := make([]ManagedCitation, len(sources))
	for i, source := range sources {
		out[i] = ManagedCitation{source.Start, source.End, source.Quote}
	}
	return out
}

func validAnswerCatalog(posting string, answers []jobSupport) bool {
	if answers == nil || len(answers) > 100 {
		return false
	}
	previousEnd := 0
	for _, answer := range answers {
		if answer.Start < previousEnd || answer.End <= answer.Start || answer.End > len(posting) ||
			!utf8.ValidString(posting[:answer.Start]) || !utf8.ValidString(posting[:answer.End]) ||
			answer.Quote != posting[answer.Start:answer.End] || !profiledocument.HasText(answer.Quote) {
			return false
		}
		previousEnd = answer.End
	}
	return true
}
