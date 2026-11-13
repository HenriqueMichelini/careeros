package draftrecovery

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/jobcontext"
)

type draftReuse struct {
	actionID string
	mac      string
	raw      json.RawMessage
}

// QualificationProof is invoked only from the explicit managed qualification
// composition after a fresh connected result and terminal kernel completion.
// The proof travels with the browser's transient context. No qualification
// output, receipt row, or reusable provider authority is persisted here.
func (s *Store) QualificationProof(ctx context.Context, p auth.Principal, actionID string, request, response []byte) (string, error) {
	if !s.managed {
		return "", ErrDenied
	}
	var input struct {
		Posting string          `json:"jobPosting"`
		Context json.RawMessage `json:"jobContext"`
	}
	var output struct {
		Context json.RawMessage `json:"jobContext"`
	}
	if json.Unmarshal(request, &input) != nil || input.Posting == "" || input.Context != nil ||
		json.Unmarshal(response, &output) != nil {
		return "", ErrInvalid
	}
	raw, err := canonicalDraftContext(output.Context)
	if err != nil {
		return "", err
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	binding, err := s.cfg.Actions.CompletedDraftQualification(ctx, tx, p, actionID)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", ErrDenied
	}
	return actionID + "." + s.qualificationMAC(binding, input.Posting, raw), nil
}

func (s *Store) qualificationMAC(b aicontrol.DraftQualificationBinding, posting string, raw json.RawMessage) string {
	payload, _ := json.Marshal(struct {
		Binding aicontrol.DraftQualificationBinding
		Policy  string
		Posting string
		Context json.RawMessage
	}{b, aicontrol.ManagedDraftPolicyVersion, posting, raw})
	return s.digest("qualification-proof", payload)
}

func canonicalDraftContext(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > 256<<10 || !accountstore.StrictJSON(raw) {
		return nil, ErrInvalid
	}
	parsed, err := jobcontext.DecodeContext(raw)
	if err != nil {
		return nil, ErrInvalid
	}
	return json.Marshal(parsed)
}

func (s *Store) prepareDraftReuse(in *Input, raw json.RawMessage, proof string) (draftReuse, error) {
	if in.Job.Analysis != nil || in.Job.Qualification != nil {
		return draftReuse{}, ErrInvalid
	}
	if len(raw) == 0 && proof == "" {
		if len(in.Confirmations) > 25 {
			return draftReuse{}, ErrInvalid
		}
		for _, c := range in.Confirmations {
			if len(c.Requirement) > 2000 {
				return draftReuse{}, ErrInvalid
			}
		}
		return draftReuse{}, nil
	}
	id, mac, ok := strings.Cut(proof, ".")
	if !ok || !validToken(id) || len(mac) != 64 {
		return draftReuse{}, ErrDenied
	}
	if _, err := hex.DecodeString(mac); err != nil {
		return draftReuse{}, ErrDenied
	}
	canonical, err := canonicalDraftContext(raw)
	if err != nil {
		return draftReuse{}, err
	}
	in.Job.Qualification = &QualificationSource{ActionID: id, ContextDigest: s.digest("qualification-context", canonical)}
	return draftReuse{actionID: id, mac: mac, raw: canonical}, nil
}

func (s *Store) authorizeDraftReuse(ctx context.Context, tx *sql.Tx, p auth.Principal, posting string, reuse draftReuse) error {
	if reuse.actionID == "" {
		return nil
	}
	binding, err := s.cfg.Actions.CompletedDraftQualification(ctx, tx, p, reuse.actionID)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(reuse.mac), []byte(s.qualificationMAC(binding, posting, reuse.raw))) {
		return ErrDenied
	}
	return nil
}
