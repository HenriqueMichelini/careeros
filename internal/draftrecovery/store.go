package draftrecovery

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/qualificationmatching"
)

//go:embed schema.sql
var schema string

type Store struct {
	db      *sql.DB
	cfg     Config
	managed bool
}

func Open(ctx context.Context, dsn string, cfg Config) (*Store, error) {
	if dsn == "" || len(cfg.BindingKey) < 32 || cfg.Decisions == nil || cfg.Actions == nil || cfg.Actions.ManagedDraftEnabled() || cfg.Review == nil {
		return nil, ErrDenied
	}
	return open(ctx, dsn, cfg)
}

func open(ctx context.Context, dsn string, cfg Config) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, ErrDenied
	}
	db.SetMaxOpenConns(8)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, ErrDenied
	}
	cfg.BindingKey = append([]byte(nil), cfg.BindingKey...)
	return &Store{db: db, cfg: cfg}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrDenied
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(72481308)"); err != nil {
		return ErrDenied
	}
	if _, err = tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("Draft migration: %w", err)
	}
	return tx.Commit()
}
func (s *Store) digest(kind string, raw []byte) string {
	m := hmac.New(sha256.New, s.cfg.BindingKey)
	m.Write([]byte("draft-recovery-v1/" + kind + ":"))
	m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}

type record struct {
	Result
	binding                      Binding
	epoch                        uint64
	grantEnd                     time.Time
	inputDigest, publishedDigest string
	generationAttempt            int
}

const columns = `namespace,action_id,owner,creation_key,input_digest,source_digest,profile_id,profile_revision,language,generation_version,review_version,authority_epoch,admitted_at,deadline,grant_expires_at,state,artifact_version,published_digest,cleanup_queued,generation_attempt`

func scanRecord(row interface{ Scan(...any) error }) (record, error) {
	var r record
	err := row.Scan(&r.Action.Namespace, &r.Action.ID, &r.Action.Owner, &r.CreationKey, &r.inputDigest, &r.binding.SourceDigest, &r.binding.Profile.ID, &r.binding.Profile.Revision, &r.binding.Language, &r.binding.GenerationVersion, &r.binding.ReviewVersion, &r.epoch, &r.AdmittedAt, &r.Deadline, &r.grantEnd, &r.State, &r.Version, &r.publishedDigest, &r.CleanupQueued, &r.generationAttempt)
	r.Action.Workflow = "application_draft"
	r.binding.Action = r.Action
	r.binding.InputDigest = r.inputDigest
	return r, err
}
func (s *Store) begin(ctx context.Context, p auth.Principal) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ErrDenied
	}
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}
func (s *Store) decision(ctx context.Context, tx *sql.Tx, p auth.Principal, a Action) (Decision, time.Time, error) {
	if err := accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return Decision{}, time.Time{}, err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return Decision{}, now, ErrDenied
	}
	d, err := s.cfg.Decisions.Decide(ctx, Authority{tx, p, a, now})
	if err != nil {
		return d, now, err
	}
	// An adapter may block. Its initial trusted time is not a final read/send
	// clock; refresh the exact session and PostgreSQL time after it returns.
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return d, now, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return d, now, ErrDenied
	}
	return d, now, nil
}
func (s *Store) current(ctx context.Context, tx *sql.Tx, p auth.Principal, in Input) (Generation, error) {
	g := Generation{Input: in}
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1 FOR UPDATE`, p.AccountID).Scan(&raw); err != nil {
		return g, ErrConflict
	}
	doc, err := profiledocument.Decode(raw)
	if err != nil || doc.ID != in.Profile.ID || doc.Revision != in.Profile.Revision {
		return g, ErrConflict
	}
	g.Binding = Binding{Profile: in.Profile, SourceDigest: s.digest("profile", raw), Language: in.Language, GenerationVersion: GenerationVersion, ReviewVersion: ReviewVersion}
	approved := qualificationmatching.ApprovedProjection(doc)
	if s.managed {
		g.Binding.GenerationVersion = aicontrol.ManagedDraftPolicyVersion
		g.document = approved
	}
	selected := map[string]bool{}
	missing := map[string]bool{}
	for _, id := range in.SelectedFactIDs {
		selected[id] = true
		missing[id] = true
	}
	g.Facts = []Fact{}
	for _, f := range qualificationmatching.Select(approved, "", 0, 1<<30).Facts {
		if len(in.SelectedFactIDs) > 0 && !selected[f.ID] {
			continue
		}
		snapshot, _ := json.Marshal(f)
		g.Facts = append(g.Facts, Fact{Reference{"profile", doc.ID, f.ID, f.Revision}, snapshot})
		delete(missing, f.ID)
	}
	if len(missing) > 0 || len(g.Facts) == 0 {
		return g, ErrInvalid
	}
	for kind, answers := range map[string][]Answer{"answer": in.Answers, "confirmation": in.Confirmations} {
		for i, a := range answers {
			snapshot, _ := json.Marshal(a)
			g.Facts = append(g.Facts, Fact{Reference{kind, doc.ID, fmt.Sprintf("application-%s-%d", kind, i), 1}, snapshot})
		}
	}
	view := profiledocument.View(doc)
	str := func(key string) string { v, _ := view[key].(string); return v }
	g.Identity = Identity{str("fullName"), str("email"), str("phone"), str("location"), str("professionalLinks")}
	return g, nil
}
func (r record) sourceMatches(g Generation) bool {
	return r.binding.SourceDigest == g.Binding.SourceDigest && r.binding.Profile == g.Binding.Profile
}

// Create returns winning=true only for the one durable generation witness.
// A recovered shared Action uses its ORIGINAL admission clock, not this insert.
func (s *Store) Create(ctx context.Context, p auth.Principal, key string, input Input) (Result, Generation, bool, error) {
	if s.managed || input.Job.Qualification != nil {
		return Result{}, Generation{}, false, ErrDenied
	}
	return s.create(ctx, p, key, input, draftReuse{})
}

func (s *Store) create(ctx context.Context, p auth.Principal, key string, input Input, reuse draftReuse) (Result, Generation, bool, error) {
	var empty Generation
	in, raw, err := normalize(input, s.managed)
	if err != nil || !validToken(key) {
		return Result{}, empty, false, ErrInvalid
	}
	digest := s.digest("input", raw)
	tx, err := s.begin(ctx, p)
	if err != nil {
		return Result{}, empty, false, err
	}
	if err = s.authorizeDraftReuse(ctx, tx, p, in.Job.Posting, reuse); err != nil {
		tx.Rollback()
		return Result{}, empty, false, err
	}
	prior, err := scanRecord(tx.QueryRowContext(ctx, "SELECT "+columns+" FROM careeros_draft_actions WHERE owner=$1 AND creation_key=$2 FOR UPDATE", p.AccountID, key))
	if err == nil {
		tx.Rollback()
		if !hmac.Equal([]byte(prior.inputDigest), []byte(digest)) {
			return Result{}, empty, false, ErrConflict
		}
		r, e := s.Read(ctx, p, key)
		return r, empty, false, e
	}
	if !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return Result{}, empty, false, ErrDenied
	}
	d, now, err := s.decision(ctx, tx, p, Action{})
	if err != nil || !d.Generate || d.Deleted || d.Revoked || d.Cancelled || !now.Before(d.GrantExpiresAt) {
		tx.Rollback()
		return Result{}, empty, false, denied(err)
	}
	initial, err := s.current(ctx, tx, p, in)
	if err != nil {
		tx.Rollback()
		return Result{}, empty, false, err
	}
	guard, err := s.admission(ctx, tx, p, key, digest, in, initial, d)
	if err != nil {
		tx.Rollback()
		return Result{}, empty, false, err
	}
	if guardClosed(guard, d, now) {
		if err = s.closeAdmission(ctx, tx, p, key, decisionClosure(d)); err != nil {
			tx.Rollback()
			return Result{}, empty, false, ErrDenied
		}
		if err = tx.Commit(); err != nil {
			return Result{}, empty, false, ErrDenied
		}
		r, e := s.Read(ctx, p, key)
		return r, empty, false, e
	}
	if err = tx.Commit(); err != nil {
		return Result{}, empty, false, ErrDenied
	}
	shared, err := s.cfg.Actions.Open(ctx, p, key, "application_draft", raw)
	if err != nil {
		return Result{}, empty, false, err
	}
	ref := shared.ActionReference()
	action := Action{ref.Namespace, ref.ID, ref.Owner, ref.Workflow}
	if action.Owner != p.AccountID || action.Workflow != "application_draft" || action.ID == "" {
		return Result{}, empty, false, ErrDenied
	}
	if err = s.fault("after_shared_action_commit"); err != nil {
		return Result{}, empty, false, err
	}
	tx, err = s.begin(ctx, p)
	if err != nil {
		return Result{}, empty, false, err
	}
	defer tx.Rollback()
	d, now, err = s.decision(ctx, tx, p, action)
	if err != nil {
		return Result{}, empty, false, err
	}
	if err = s.authorizeDraftReuse(ctx, tx, p, in.Job.Posting, reuse); err != nil {
		return Result{}, empty, false, err
	}
	guard, err = s.admission(ctx, tx, p, key, digest, in, initial, d)
	if err != nil {
		return Result{}, empty, false, err
	}
	if guardClosed(guard, d, now) {
		if err = s.closeAdmission(ctx, tx, p, key, decisionClosure(d)); err != nil {
			return Result{}, empty, false, ErrDenied
		}
		if err = tx.Commit(); err != nil {
			return Result{}, empty, false, ErrDenied
		}
		r, e := s.Read(ctx, p, key)
		return r, empty, false, e
	}
	var admitted time.Time
	var owner, creation, workflow, sharedState string
	err = tx.QueryRowContext(ctx, `SELECT owner,creation_key,workflow,created_at,state FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, action.Namespace, action.ID).Scan(&owner, &creation, &workflow, &admitted, &sharedState)
	if err != nil || owner != p.AccountID || creation != key || workflow != "application_draft" {
		return Result{}, empty, false, ErrDenied
	}
	prior, err = scanRecord(tx.QueryRowContext(ctx, "SELECT "+columns+" FROM careeros_draft_actions WHERE owner=$1 AND creation_key=$2 FOR UPDATE", p.AccountID, key))
	if err == nil {
		tx.Rollback()
		if prior.Action != action || prior.inputDigest != digest {
			return Result{}, empty, false, ErrConflict
		}
		r, e := s.Read(ctx, p, key)
		return r, empty, false, e
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Result{}, empty, false, ErrDenied
	}
	if sharedClosure(sharedState) != "" {
		tx.Rollback()
		r, e := s.Read(ctx, p, key)
		return r, empty, false, e
	}
	if sharedState != "recoverable" {
		return Result{}, empty, false, ErrDenied
	}
	g, err := s.current(ctx, tx, p, in)
	if err != nil {
		return Result{}, empty, false, err
	}
	if !guard.sourceMatches(g) {
		return Result{}, empty, false, ErrConflict
	}
	deadline := admitted.Add(CustomerWindow)
	if guard.grantEnd.Before(deadline) {
		deadline = guard.grantEnd
	}
	if !now.Before(deadline) {
		return Result{}, empty, false, ErrDenied
	}
	g.Binding.Action = action
	g.Binding.InputDigest = digest
	if s.managed {
		g.session = shared
		g.jobContext = reuse.raw
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO careeros_draft_actions(namespace,action_id,owner,creation_key,input_digest,source_digest,profile_id,profile_revision,language,generation_version,review_version,authority_epoch,admitted_at,deadline,grant_expires_at,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'pending')`, action.Namespace, action.ID, p.AccountID, key, digest, g.Binding.SourceDigest, in.Profile.ID, in.Profile.Revision, in.Language, g.Binding.GenerationVersion, g.Binding.ReviewVersion, guard.epoch, admitted, deadline, guard.grantEnd)
	if err != nil {
		return Result{}, empty, false, ErrDenied
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_draft_content(namespace,action_id,input) VALUES($1,$2,$3)`, action.Namespace, action.ID, raw); err != nil {
		return Result{}, empty, false, ErrDenied
	}
	if err = tx.Commit(); err != nil {
		return Result{}, empty, false, ErrDenied
	}
	if err = s.fault("after_create_commit"); err != nil {
		return Result{}, empty, false, err
	}
	r := Result{Action: action, CreationKey: key, State: "pending", AdmittedAt: admitted, Deadline: deadline}
	return r, g, true, nil
}
func denied(err error) error {
	if err != nil {
		return err
	}
	return ErrDenied
}
func (s *Store) fault(stage string) error {
	if s.cfg.Fault != nil {
		return s.cfg.Fault(stage)
	}
	return nil
}

func (s *Store) load(ctx context.Context, tx *sql.Tx, p auth.Principal, key string) (record, Decision, time.Time, error) {
	r, err := scanRecord(tx.QueryRowContext(ctx, "SELECT "+columns+" FROM careeros_draft_actions WHERE owner=$1 AND creation_key=$2 FOR UPDATE", p.AccountID, key))
	if errors.Is(err, sql.ErrNoRows) {
		r, err = s.gapRecord(ctx, tx, p, key)
	}
	if err != nil {
		return r, Decision{}, time.Time{}, err
	}
	d, now, err := s.decision(ctx, tx, p, r.Action)
	if err != nil {
		return r, d, now, err
	}
	var sharedState string
	var attempt int
	if err = tx.QueryRowContext(ctx, `SELECT state,attempt FROM ai_fixture_action WHERE namespace=$1 AND id=$2 AND owner=$3 AND workflow='application_draft' FOR UPDATE`, r.Action.Namespace, r.Action.ID, p.AccountID).Scan(&sharedState, &attempt); err != nil {
		return r, d, now, ErrDenied
	}
	previous := r.State
	shared := sharedClosure(sharedState)
	staleUndelivered := false
	if s.managed && !d.Delivered && r.generationAttempt != 0 &&
		(r.State == "ready" || r.State == "pending" || r.State == "incomplete") {
		var source []byte
		e := tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1`, p.AccountID).Scan(&source)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return r, d, now, ErrDenied
		}
		staleUndelivered = e != nil || s.digest("profile", source) != r.binding.SourceDigest
	}
	switch {
	case d.Deleted || shared == "deleted":
		r.State = "deleted"
	case r.State == "deleted":
		// Retain the stronger terminal state.
	case shared == "closed",
		staleUndelivered,
		!d.Delivered && (d.Epoch != r.epoch || d.Revoked || d.Cancelled || !now.Before(r.Deadline) || !now.Before(d.GrantExpiresAt)),
		d.Delivered && !now.Before(r.grantEnd.Add(30*24*time.Hour)):
		r.State = "closed"
	}
	closed := r.State == "closed" || r.State == "deleted"
	if s.managed {
		r.Attempt = attempt
		r.RetryAllowed = !closed && r.State == "incomplete" && d.Generate && sharedState == "failed" && attempt == 1
	}
	if closed {
		if err = s.closeAdmission(ctx, tx, p, key, r.State); err != nil {
			return r, d, now, ErrDenied
		}
		if !r.CleanupQueued || previous != r.State {
			if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET state=$3,cleanup_queued=true WHERE namespace=$1 AND action_id=$2`, r.Action.Namespace, r.Action.ID, r.State); err != nil {
				return r, d, now, ErrDenied
			}
			r.CleanupQueued = true
		}
		// Propagate closure only from a nonterminal ledger state. Customer
		// cancellation and any existing delivery/exposure evidence stay intact.
		if shared == "" && !d.Delivered {
			if _, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state=$3 WHERE namespace=$1 AND id=$2 AND NOT customer_effect`, r.Action.Namespace, r.Action.ID, r.State); err != nil {
				return r, d, now, ErrDenied
			}
		}
	}
	return r, d, now, nil
}

func (s *Store) Publish(ctx context.Context, p auth.Principal, key string, publication Publication) error {
	raw := publication.raw
	if len(raw) > MaxBundleBytes || !accountstore.StrictJSON(raw) || !hmac.Equal([]byte(publication.seal), []byte(s.digest("publication", raw))) {
		return ErrInvalid
	}
	var b Bundle
	if json.Unmarshal(raw, &b) != nil || b.Support.Completion == nil {
		return ErrInvalid
	}
	digest := s.digest("bundle", raw)
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, d, now, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State == "closed" || r.State == "deleted" {
		_ = tx.Commit()
		return ErrDenied
	}
	if r.Version == 1 {
		if r.publishedDigest != digest {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !d.Publish || d.Deleted || d.Revoked || d.Cancelled || d.Epoch != r.epoch || !now.Before(r.Deadline) || !now.Before(d.GrantExpiresAt) || !reflectBinding(r.binding, b.Binding) {
		return ErrDenied
	}
	var inputRaw []byte
	if err = tx.QueryRowContext(ctx, `SELECT input FROM careeros_draft_content WHERE namespace=$1 AND action_id=$2 FOR UPDATE`, r.Action.Namespace, r.Action.ID).Scan(&inputRaw); err != nil {
		return ErrDenied
	}
	var in Input
	if json.Unmarshal(inputRaw, &in) != nil || s.digest("input", inputRaw) != r.inputDigest {
		return ErrDenied
	}
	current, err := s.current(ctx, tx, p, in)
	if err != nil {
		return err
	}
	if !r.sourceMatches(current) {
		return ErrConflict
	}
	current.Binding = r.binding
	if err = ValidateComplete(current, b); err != nil {
		return err
	}
	completion := *b.Support.Completion
	b.Support.Completion = nil
	candidateRaw, _ := json.Marshal(b)
	if err = validateCompletion(current, b, completion, s.digest("candidate", candidateRaw)); err != nil {
		return err
	}
	if err = s.fault("before_publish_commit"); err != nil {
		return err
	}
	// Recheck time, exact session, injected fences and Profile after any blocking
	// adapter/fault work. No transaction is held while the provider executes.
	d, now, err = s.decision(ctx, tx, p, r.Action)
	if err != nil || !d.Publish || d.Deleted || d.Revoked || d.Cancelled || d.Epoch != r.epoch || !now.Before(r.Deadline) || !now.Before(d.GrantExpiresAt) {
		return denied(err)
	}
	current, err = s.current(ctx, tx, p, in)
	if err != nil || !r.sourceMatches(current) {
		return ErrConflict
	}
	if s.managed {
		if publication.session == nil || publication.session.ID() != r.Action.ID || publication.session.DraftAttempt() != r.generationAttempt {
			return ErrDenied
		}
		if err = publication.session.AuthorizeDraftPublication(ctx, tx); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_content SET bundle=$3 WHERE namespace=$1 AND action_id=$2`, r.Action.Namespace, r.Action.ID, raw); err != nil {
		return ErrDenied
	}
	if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET state='ready',artifact_version=1,published_digest=$3 WHERE namespace=$1 AND action_id=$2`, r.Action.Namespace, r.Action.ID, digest); err != nil {
		return ErrDenied
	}
	updated, err := tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='candidate',result_ref=$3,execution_finished=true WHERE namespace=$1 AND id=$2 AND state NOT IN ('canceled','closed','deleted','delivered') AND NOT customer_effect`, r.Action.Namespace, r.Action.ID, BundleVersion+"/1")
	if err != nil {
		return ErrDenied
	}
	if count, err := updated.RowsAffected(); err != nil || count != 1 {
		return ErrDenied
	}
	if err = tx.Commit(); err != nil {
		return ErrDenied
	}
	return s.fault("after_publish_commit")
}
func reflectBinding(a, b Binding) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return equalJSON(x, y)
}

func (s *Store) Incomplete(ctx context.Context, p auth.Principal, key string) error {
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, _, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State == "pending" {
		if _, err = tx.ExecContext(ctx, `UPDATE careeros_draft_actions SET state='incomplete' WHERE namespace=$1 AND action_id=$2`, r.Action.Namespace, r.Action.ID); err != nil {
			return ErrDenied
		}
	}
	return tx.Commit()
}

// WithResult retains all account/session/decision/content locks THROUGH the
// caller's final HTTP writes; callers must not buffer then flush outside it.
func (s *Store) WithResult(ctx context.Context, p auth.Principal, key string, write func(Result) error) error {
	if !validToken(key) || write == nil {
		return ErrInvalid
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, d, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State == "closed" || r.State == "deleted" {
		if err = tx.Commit(); err != nil {
			return ErrDenied
		}
		return write(r.Result)
	}
	if !d.Read {
		return ErrDenied
	}
	if r.State == "ready" {
		var inputRaw, bundleRaw []byte
		if err = tx.QueryRowContext(ctx, `SELECT input,bundle FROM careeros_draft_content WHERE namespace=$1 AND action_id=$2 FOR SHARE`, r.Action.Namespace, r.Action.ID).Scan(&inputRaw, &bundleRaw); err != nil {
			return ErrDenied
		}
		var in Input
		var b Bundle
		if json.Unmarshal(inputRaw, &in) != nil || json.Unmarshal(bundleRaw, &b) != nil || s.digest("bundle", bundleRaw) != r.publishedDigest || !reflectBinding(r.binding, b.Binding) {
			return ErrDenied
		}
		r.Bundle = &b
		r.Job = &in.Job
		r.Context = &RetainedContext{in.Answers, in.Confirmations, in.SelectedFactIDs}
		var source []byte
		if tx.QueryRowContext(ctx, `SELECT document FROM careeros_profiles WHERE account_id=$1`, p.AccountID).Scan(&source) != nil || s.digest("profile", source) != r.binding.SourceDigest {
			r.Historical = true
		}
	}
	if err = s.fault("before_result_write"); err != nil {
		return err
	}
	d, _, err = s.decision(ctx, tx, p, r.Action)
	if err != nil || !d.Read || d.Deleted {
		return denied(err)
	}
	// Re-evaluate closure after the last potentially blocking authority check.
	r2, finalDecision, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r2.State == "closed" || r2.State == "deleted" {
		if err = tx.Commit(); err != nil {
			return ErrDenied
		}
		return write(r2.Result)
	}
	if !finalDecision.Read {
		return ErrDenied
	}
	if err = write(r.Result); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Read(ctx context.Context, p auth.Principal, key string) (Result, error) {
	var r Result
	err := s.WithResult(ctx, p, key, func(v Result) error { r = v; return nil })
	return r, err
}

// Cleanup removes only this queued purpose-content, retaining the immutable
// non-content Action/replay record and all unrelated Profile/ledger data.
func (s *Store) Cleanup(ctx context.Context, p auth.Principal, key string) error {
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, _, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if !r.CleanupQueued {
		return ErrDenied
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM careeros_draft_content WHERE namespace=$1 AND action_id=$2`, r.Action.Namespace, r.Action.ID); err != nil {
		return ErrDenied
	}
	return tx.Commit()
}
