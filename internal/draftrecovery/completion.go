package draftrecovery

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"reflect"

	"professional-information-repo/internal/auth"
)

// Prepare performs the required independent completion check outside every DB
// transaction. The check sees the complete original job and exact candidate.
// A generator's labels or an optional caller index cannot substitute for it.
func (s *Store) Prepare(ctx context.Context, p auth.Principal, key string, b Bundle) (Publication, error) {
	var empty Publication
	if b.Support.Completion != nil {
		return empty, ErrInvalid
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	r, d, now, err := s.load(ctx, tx, p, key)
	if err != nil {
		return empty, err
	}
	if r.State == "closed" || r.State == "deleted" || !d.Publish || d.Deleted || d.Revoked || d.Cancelled || d.Epoch != r.epoch || !now.Before(r.Deadline) || !now.Before(d.GrantExpiresAt) || !reflectBinding(r.binding, b.Binding) {
		return empty, ErrDenied
	}
	var inputRaw []byte
	if err = tx.QueryRowContext(ctx, `SELECT input FROM careeros_draft_content WHERE namespace=$1 AND action_id=$2`, r.Action.Namespace, r.Action.ID).Scan(&inputRaw); err != nil {
		return empty, ErrDenied
	}
	var in Input
	if json.Unmarshal(inputRaw, &in) != nil || s.digest("input", inputRaw) != r.inputDigest {
		return empty, ErrDenied
	}
	g, err := s.current(ctx, tx, p, in)
	if err != nil || !r.sourceMatches(g) {
		return empty, ErrConflict
	}
	g.Binding = r.binding
	if err = ValidateComplete(g, b); err != nil {
		return empty, err
	}
	if err = tx.Commit(); err != nil {
		return empty, ErrDenied
	}
	// Freeze before invoking a callback: a callback cannot mutate the candidate
	// through shared slices and then publish different bytes under its check.
	raw, err := json.Marshal(b)
	if err != nil {
		return empty, ErrInvalid
	}
	digest := s.digest("candidate", raw)
	var candidate Bundle
	if json.Unmarshal(raw, &candidate) != nil {
		return empty, ErrInvalid
	}
	completion, err := s.cfg.Review(ctx, ReviewRequest{g, candidate, digest})
	if err != nil {
		return empty, err
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	if err = validateCompletion(g, b, completion, digest); err != nil {
		return empty, err
	}
	if json.Unmarshal(raw, &b) != nil {
		return empty, ErrInvalid
	}
	b.Support.Completion = &completion
	raw, err = json.Marshal(b)
	if err != nil || len(raw) > MaxBundleBytes {
		return empty, ErrInvalid
	}
	return Publication{raw, s.digest("publication", raw)}, nil
}

func validateCompletion(g Generation, b Bundle, c Completion, digest string) error {
	if c.Version != CompletionVersion || c.State != "complete" || !hmac.Equal([]byte(c.CandidateDigest), []byte(digest)) || c.InputDigest != g.Binding.InputDigest || c.Concerns == nil || len(c.Concerns) > 0 || c.AnswerRequests == nil || len(c.AnswerRequests) > 100 {
		return ErrInvalid
	}
	if len(c.AnswerRequests) == 0 {
		if b.Answers.State != "none_requested" || len(b.Answers.Requests) != 0 {
			return ErrInvalid
		}
		return nil
	}
	if b.Answers.State != "complete" || len(b.Answers.Requests) != len(c.AnswerRequests) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for i, r := range c.AnswerRequests {
		if !validToken(r.ID) || seen[r.ID] || !validRange(r.Source, g.Input.Job.Posting) || b.Answers.Requests[i].ID != r.ID || !reflect.DeepEqual(b.Answers.Requests[i].Source, r.Source) {
			return ErrInvalid
		}
		seen[r.ID] = true
	}
	return nil
}
