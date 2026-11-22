package draftrecovery

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"professional-information-repo/internal/auth"
)

const HumanReviewVersion = "application-draft-human-review-v1"

type ArtifactDecision struct {
	Artifact string `json:"artifact"`
	State    string `json:"state"`
}

type HumanReview struct {
	Schema     string             `json:"schema"`
	Revision   int64              `json:"revision"`
	Artifacts  []ArtifactDecision `json:"artifacts"`
	Accepted   bool               `json:"accepted"`
	AcceptedAt *time.Time         `json:"acceptedAt,omitempty"`
}

// ReviewCommand names an immutable candidate and a human choice, never delivery
// authority, a new artifact body, a provider result or a Profile mutation.
type ReviewCommand struct {
	DecisionKey      string `json:"decisionKey"`
	ExpectedRevision int64  `json:"expectedRevision"`
	BundleDigest     string `json:"bundleDigest"`
	Artifact         string `json:"artifact"`
	Operation        string `json:"operation"`
}

func initialReview() HumanReview {
	return HumanReview{Schema: HumanReviewVersion, Artifacts: []ArtifactDecision{{"summary", "unreviewed"}, {"resume", "unreviewed"}, {"cover", "unreviewed"}, {"answers", "unreviewed"}}}
}

func (s *Store) humanReview(ctx context.Context, tx *sql.Tx, r record) (HumanReview, error) {
	v := initialReview()
	var raw []byte
	var digest string
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT decisions,published_digest,revision FROM careeros_draft_reviews WHERE namespace=$1 AND action_id=$2 FOR UPDATE`, r.Action.Namespace, r.Action.ID).Scan(&raw, &digest, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil || digest != r.publishedDigest || json.Unmarshal(raw, &v) != nil || v.Schema != HumanReviewVersion || v.Revision != revision || len(v.Artifacts) != 4 {
		return HumanReview{}, ErrDenied
	}
	for i, want := range initialReview().Artifacts {
		got := v.Artifacts[i]
		if got.Artifact != want.Artifact || (got.State != "unreviewed" && got.State != "reviewed" && got.State != "accepted") {
			return HumanReview{}, ErrDenied
		}
	}
	accepted := true
	for _, a := range v.Artifacts {
		accepted = accepted && a.State == "accepted"
	}
	if v.Accepted != accepted || v.Accepted != (v.AcceptedAt != nil) {
		return HumanReview{}, ErrDenied
	}
	return v, nil
}

func (s *Store) ApplyReview(ctx context.Context, p auth.Principal, key string, command ReviewCommand) error {
	if !validToken(key) || !validToken(command.DecisionKey) || command.ExpectedRevision < 0 || command.ExpectedRevision > 9007199254740991 || len(command.BundleDigest) != 64 || (command.Operation != "review" && command.Operation != "accept") {
		return ErrInvalid
	}
	index := -1
	for i, a := range initialReview().Artifacts {
		if command.Artifact == a.Artifact {
			index = i
		}
	}
	if index < 0 {
		return ErrInvalid
	}
	raw, _ := json.Marshal(command)
	binding := s.digest("human-decision", raw)
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, d, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State != "ready" || r.Version != 1 || !d.Read || !d.Delivered || d.Deleted {
		return denyReview(tx, r)
	}
	if !hmac.Equal([]byte(command.BundleDigest), []byte(r.publishedDigest)) {
		return ErrConflict
	}
	var prior string
	err = tx.QueryRowContext(ctx, `SELECT binding FROM careeros_draft_review_commands WHERE namespace=$1 AND action_id=$2 AND decision_key=$3`, r.Action.Namespace, r.Action.ID, command.DecisionKey).Scan(&prior)
	if err == nil {
		if !hmac.Equal([]byte(prior), []byte(binding)) {
			return ErrConflict
		}
		// Duplicate writes recover the existing effect under the same current
		// authority fence; a matching opaque key cannot bypass expired rights.
		latest, final, _, err := s.load(ctx, tx, p, key)
		if err != nil {
			return err
		}
		if latest.State != "ready" || !final.Read || !final.Delivered || final.Deleted {
			return denyReview(tx, latest)
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	v, err := s.humanReview(ctx, tx, r)
	if err != nil {
		return err
	}
	if v.Revision != command.ExpectedRevision {
		return ErrConflict
	}
	state := &v.Artifacts[index].State
	switch command.Operation {
	case "review":
		if *state != "unreviewed" {
			return ErrConflict
		}
		*state = "reviewed"
	case "accept":
		if *state != "reviewed" {
			return ErrConflict
		}
		*state = "accepted"
	}
	v.Revision++
	v.Accepted = true
	for _, a := range v.Artifacts {
		v.Accepted = v.Accepted && a.State == "accepted"
	}
	if err = s.fault("before_review_commit"); err != nil {
		return err
	}
	r2, final, now, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r2.State != "ready" || !final.Read || !final.Delivered || final.Deleted || r2.publishedDigest != r.publishedDigest {
		return denyReview(tx, r2)
	}
	if v.Accepted {
		v.AcceptedAt = &now
	}
	decisions, _ := json.Marshal(v)
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_draft_reviews(namespace,action_id,artifact_version,published_digest,revision,decisions,accepted_at) VALUES($1,$2,1,$3,$4,$5,$6) ON CONFLICT(namespace,action_id) DO UPDATE SET revision=EXCLUDED.revision,decisions=EXCLUDED.decisions,accepted_at=EXCLUDED.accepted_at`, r.Action.Namespace, r.Action.ID, r.publishedDigest, v.Revision, decisions, v.AcceptedAt); err != nil {
		return ErrDenied
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_draft_review_commands(namespace,action_id,decision_key,binding) VALUES($1,$2,$3,$4)`, r.Action.Namespace, r.Action.ID, command.DecisionKey, binding); err != nil {
		return ErrDenied
	}
	if err = tx.Commit(); err != nil {
		return ErrDenied
	}
	return s.fault("after_review_commit")
}

// load may observe a winning terminal fence before a rejected decision. Keep
// that closure just as Read/Publish do; no review write has happened yet. A
// temporary read/session denial on a nonterminal Action still rolls back.
func denyReview(tx *sql.Tx, r record) error {
	if r.State == "closed" || r.State == "deleted" {
		_ = tx.Commit()
	}
	return ErrDenied
}

// WithAccepted selects a committed accepted identity, then uses the ordinary
// exact-result read fence through the caller's final write. A newer acceptance
// cannot turn this selected immutable identity into a different bundle.
func (s *Store) WithAccepted(ctx context.Context, p auth.Principal, write func(Result) error) error {
	if write == nil {
		return ErrInvalid
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	var key string
	err = tx.QueryRowContext(ctx, `SELECT a.creation_key FROM careeros_draft_reviews r JOIN careeros_draft_actions a USING(namespace,action_id) WHERE a.owner=$1 AND r.accepted_at IS NOT NULL ORDER BY r.accepted_at DESC,a.action_id DESC LIMIT 1`, p.AccountID).Scan(&key)
	tx.Rollback()
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return ErrDenied
	}
	return s.WithResult(ctx, p, key, func(result Result) error {
		if result.State == "ready" && (result.Review == nil || !result.Review.Accepted) {
			return ErrDenied
		}
		return write(result)
	})
}
