package draftrecovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	applicationdraft "professional-information-repo/backend/application-draft"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/auth"
)

// OpenManaged selects the real local pilot grant adapter. Controlled recovery
// still uses its injected test contract, but cannot select this composition.
func OpenManaged(ctx context.Context, dsn string, actions *aicontrol.Harness, bindingKey []byte) (*Store, error) {
	u, err := url.Parse(dsn)
	if err != nil || !actions.ManagedDraftEnabled() || len(bindingKey) < 32 ||
		(u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.HasPrefix(u.Query().Get("search_path"), "careeros_ai_test_") ||
		!validToken(u.Query().Get("search_path")) {
		return nil, ErrDenied
	}
	host := u.Query().Get("host")
	if host == "" {
		host = u.Hostname()
	}
	if ip := net.ParseIP(host); !strings.HasPrefix(host, "/") && (ip == nil || !ip.IsLoopback()) {
		return nil, ErrDenied
	}
	store, err := open(ctx, dsn, Config{Actions: actions, BindingKey: bindingKey, Decisions: DecisionFunc(managedPilotDecision)})
	if err != nil {
		return nil, err
	}
	store.managed = true
	return store, nil
}

func (s *Store) UsesManagedAuthority(actions *aicontrol.Harness) bool {
	return s != nil && s.managed && s.cfg.Actions == actions && actions.ManagedDraftEnabled()
}

func (s *Store) CreateManaged(ctx context.Context, p auth.Principal, key string, in Input, rawContext json.RawMessage, proof string) (Result, Generation, bool, error) {
	if !s.managed {
		return Result{}, Generation{}, false, ErrDenied
	}
	reuse, err := s.prepareDraftReuse(&in, rawContext, proof)
	if err != nil {
		return Result{}, Generation{}, false, err
	}
	return s.create(ctx, p, key, in, reuse)
}

func managedPilotDecision(ctx context.Context, a Authority) (Decision, error) {
	var activated, expiry, accessEnd sql.NullTime
	var state string
	var eligible, held bool
	err := a.Transaction.QueryRowContext(ctx, `
		SELECT p.activated_at,p.expires_at,p.state,COALESCE(p.age_eligible,false) AND NOT p.invitation_revoked,p.access_ended_at,p.retrieval_held
		FROM careeros_pilot_access p JOIN careeros_identities i USING(provider,issuer,subject)
		WHERE p.account_id=$1 AND i.account_id=$1`, a.Principal.AccountID).Scan(&activated, &expiry, &state, &eligible, &accessEnd, &held)
	if errors.Is(err, sql.ErrNoRows) {
		return Decision{Revoked: true}, nil
	}
	if err != nil {
		return Decision{}, ErrDenied
	}
	if !activated.Valid || !expiry.Valid || activated.Time.UnixMicro() <= 0 {
		return Decision{Revoked: true}, nil
	}
	d := Decision{Epoch: uint64(activated.Time.UnixMicro()), GrantExpiresAt: expiry.Time, Revoked: state != "active" || !eligible || held}
	if a.Action.ID != "" {
		if err = a.Transaction.QueryRowContext(ctx, `SELECT customer_effect FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 AND workflow='application_draft'`,
			a.Action.Namespace, a.Action.ID, a.Principal.AccountID).Scan(&d.Delivered); err != nil {
			return Decision{}, ErrDenied
		}
	}
	d.Generate = !d.Revoked && a.Now.Before(d.GrantExpiresAt)
	d.Publish = d.Generate
	end := expiry.Time
	if accessEnd.Valid && accessEnd.Time.Before(end) {
		end = accessEnd.Time
	}
	retrievalState := state == "active" || (state == "revoked" && accessEnd.Valid)
	d.Read = eligible && !held && retrievalState && (d.Generate || (d.Delivered && a.Now.Before(end.Add(30*24*time.Hour))))
	return d, nil
}

// GenerateManaged consumes the one witness returned by Create and prepares
// an immutable publication from the existing managed graph's exact output.
// The callback used by controlled recovery is unavailable in this mode.
func (s *Store) GenerateManaged(ctx context.Context, p auth.Principal, key string, g Generation) (Publication, error) {
	if !s.managed || g.session == nil || g.Binding.Action.Owner != p.AccountID {
		return Publication{}, ErrDenied
	}
	_, raw, err := normalize(g.Input, true)
	if err != nil || g.Binding.Action.ID != g.session.ID() || s.digest("input", raw) != g.Binding.InputDigest {
		return Publication{}, ErrConflict
	}
	ctx, cancel, err := g.session.ManagedDraftContext(ctx)
	if err != nil {
		return Publication{}, err
	}
	defer cancel()
	dispatch := &draftDispatch{session: g.session}
	ctx = aidispatch.WithController(ctx, dispatch)
	in := applicationdraft.ManagedInput{
		Profile: g.document, Posting: g.Input.Job.Posting, Language: g.Input.Language,
		FullName:        g.Identity.FullName,
		JobContext:      g.jobContext,
		SelectedFactIDs: g.Input.SelectedFactIDs, ResumeNotNeeded: g.session.MarkDraftResumeNotNeeded,
	}
	for _, a := range g.Input.Answers {
		in.Answers = append(in.Answers, applicationdraft.ManagedAnswer{Requirement: a.Requirement, UserContext: a.UserContext})
	}
	for _, a := range g.Input.Confirmations {
		in.Confirmations = append(in.Confirmations, applicationdraft.ManagedAnswer{Requirement: a.Requirement, UserContext: a.UserContext, Kind: a.Kind})
	}
	result, err := applicationdraft.GenerateManaged(ctx, in)
	if ctx.Err() != nil {
		return Publication{}, ctx.Err()
	}
	if dispatch.err != nil {
		return Publication{}, dispatch.err
	}
	if err != nil {
		if dispatch.knownFailure {
			return Publication{}, &knownDraftFailure{err}
		}
		return Publication{}, err
	}
	snapshot, err := result.CompleteSnapshot(g.Input.Language, g.Identity.FullName)
	if err != nil {
		return Publication{}, err
	}
	bundle, requests, err := managedBundle(g, snapshot)
	if err != nil {
		return Publication{}, err
	}
	return s.prepare(ctx, p, key, bundle, func(_ context.Context, r ReviewRequest) (Completion, error) {
		return Completion{
			Version: CompletionVersion, State: "complete", CandidateDigest: r.CandidateDigest,
			InputDigest: g.Binding.InputDigest, AnswerRequests: requests, Concerns: []string{},
		}, nil
	}, g.session)
}

type knownDraftFailure struct{ error }

func (f *knownDraftFailure) Unwrap() error { return f.error }

type draftDispatch struct {
	session      *aicontrol.Session
	err          error
	knownFailure bool
}

func (d *draftDispatch) Send(ctx context.Context, provider string, req *http.Request) (*http.Response, error) {
	response, err := d.session.Send(ctx, provider, req)
	if err != nil {
		d.err = err
	}
	stage := aidiagnostics.StageName(ctx)
	d.knownFailure = err == nil && response != nil &&
		(stage == "field_validation" || stage == "generation") &&
		(response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 && response.StatusCode < 600)
	return response, err
}

// IncompleteManaged records an explicit local outcome while leaving every
// unsettled reservation, intent and expense in the shared ledger.
func (s *Store) IncompleteManaged(ctx context.Context, p auth.Principal, key string, g Generation, cause error) error {
	if !s.managed || g.session == nil {
		return ErrDenied
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, _, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State != "pending" || r.Action.ID != g.session.ID() || r.generationAttempt != g.session.DraftAttempt() {
		return tx.Commit()
	}
	var known *knownDraftFailure
	if err = g.session.FinishDraftFailure(ctx, tx, errors.As(cause, &known)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET state='incomplete' WHERE namespace=$1 AND action_id=$2 AND state='pending' AND generation_attempt=$3`, r.Action.Namespace, r.Action.ID, r.generationAttempt); err != nil {
		return ErrDenied
	}
	return tx.Commit()
}

func managedBundle(g Generation, snapshot applicationdraft.ManagedSnapshot) (Bundle, []AnswerRequirement, error) {
	b := Bundle{
		Schema: BundleVersion, Binding: g.Binding, Identity: g.Identity, JobTitle: snapshot.JobTitle, Company: snapshot.Company,
		RoleSummary: snapshot.RoleSummary, Resume: snapshot.Resume, CoverLetter: snapshot.CoverLetter,
		Answers: EmployerAnswers{State: "none_requested", Text: snapshot.AnswerText, Requests: []AnswerRequest{}},
		Support: Support{Version: ReviewVersion, Check: "complete", Concerns: []string{}, Facts: []Fact{}, Resume: []Claim{}, Artifacts: []Claim{}},
	}
	catalog := make(map[string]Fact, len(g.Facts))
	for _, fact := range g.Facts {
		catalog[fact.Reference.ID] = fact
	}
	retained := map[string]bool{}
	convert := func(claim applicationdraft.ManagedClaim) (Claim, error) {
		c := Claim{
			Field: claim.Field, Text: claim.Text, State: claim.State, Nonfactual: claim.Nonfactual,
			Sources: []Reference{}, JobSources: []JobRange{}, Concerns: append([]string{}, claim.Concerns...),
		}
		for _, src := range claim.Sources {
			fact, found := catalog[src.ID]
			if !found || fact.Reference.ProfileID != src.ProfileID || fact.Reference.Revision != src.Revision {
				return Claim{}, ErrInvalid
			}
			c.Sources = append(c.Sources, fact.Reference)
			if !retained[src.ID] {
				b.Support.Facts = append(b.Support.Facts, fact)
				retained[src.ID] = true
			}
		}
		for _, src := range claim.JobSources {
			c.JobSources = append(c.JobSources, JobRange{src.Start, src.End, src.Quote})
		}
		return c, nil
	}
	for _, claim := range snapshot.ResumeClaims {
		c, err := convert(claim)
		if err != nil {
			return Bundle{}, nil, err
		}
		b.Support.Resume = append(b.Support.Resume, c)
	}
	for _, claim := range snapshot.ArtifactClaims {
		c, err := convert(claim)
		if err != nil {
			return Bundle{}, nil, err
		}
		b.Support.Artifacts = append(b.Support.Artifacts, c)
	}
	requests := []AnswerRequirement{}
	if len(snapshot.AnswerRequests) > 0 {
		answers := strings.Split(snapshot.AnswerText, "\n\n")
		if len(answers) != len(snapshot.AnswerRequests) {
			return Bundle{}, nil, ErrInvalid
		}
		b.Answers.State = "complete"
		for i, src := range snapshot.AnswerRequests {
			id, source := fmt.Sprintf("request-%d", i+1), JobRange{src.Start, src.End, src.Quote}
			b.Answers.Requests = append(b.Answers.Requests, AnswerRequest{id, source, answers[i]})
			requests = append(requests, AnswerRequirement{id, source})
		}
	}
	return b, requests, nil
}
