package accountstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/profiledocument"
)

const MaxDocumentBytes = 4 * 1024 * 1024

var (
	ErrConflict   = errors.New("profile conflict")
	ErrValidation = errors.New("invalid profile save")
	ErrNotFound   = errors.New("operation not found")
)

type Outcome struct {
	OperationID       string `json:"operationId"`
	ProfileID         string `json:"profileId"`
	CommittedRevision int64  `json:"committedRevision"`
}
type ProfileResult struct {
	AccountID         string          `json:"accountId"`
	Document          json.RawMessage `json:"document"`
	Outcome           *Outcome        `json:"outcome,omitempty"`
	DeletedProfileIDs []string        `json:"deletedProfileIds,omitempty"`
}

// authorized locks the account before its session. Every auth mutation that
// can revoke this authority takes that same account lock. The DB clock is
// checked after lock acquisition, including when a request waited at expiry.
func authorized(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	if err := authorizedAccount(ctx, tx, p); err != nil {
		return err
	}
	// A stale restored Profile cannot override its independently surviving
	// deletion marker. Keep ordinary career access closed until reconciliation.
	var restoredDeleted bool
	if err := tx.QueryRowContext(ctx, `SELECT careeros_deleted_profile_restored($1)`, p.AccountID).Scan(&restoredDeleted); err != nil {
		return unavailable(err)
	}
	if restoredDeleted {
		return auth.ErrForbidden
	}
	return nil
}

func authorizedAccount(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	if err := authorizedSession(ctx, tx, p); err != nil {
		return err
	}
	var closed bool
	if err := tx.QueryRowContext(ctx, `SELECT careeros_account_closed($1)`, p.AccountID).Scan(&closed); err != nil {
		return unavailable(err)
	}
	if closed {
		return auth.ErrAuthenticationRequired
	}
	var compromised bool
	if err := tx.QueryRowContext(ctx, `SELECT careeros_control_account_denied($1)`, p.AccountID).Scan(&compromised); err != nil {
		return unavailable(err)
	}
	if compromised {
		return auth.ErrForbidden
	}
	return nil
}
func authorizedSession(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	var status auth.AccountStatus
	var generation uint64
	err := tx.QueryRowContext(ctx, "SELECT status,generation FROM careeros_accounts WHERE id=$1 FOR UPDATE", p.AccountID).Scan(&status, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrAuthenticationRequired
	}
	if err != nil {
		return unavailable(err)
	}
	if status != auth.AccountActive || generation != p.Generation {
		return auth.ErrAuthenticationRequired
	}
	v, err := scanSession(tx.QueryRowContext(ctx, "SELECT "+sessionColumns+" FROM careeros_sessions WHERE id=$1 AND account_id=$2 AND expires_at>clock_timestamp() FOR UPDATE", p.SessionID, p.AccountID))
	if errors.Is(err, sql.ErrNoRows) {
		return auth.ErrAuthenticationRequired
	}
	if err != nil {
		return unavailable(err)
	}
	if !p.MatchesSession(v) {
		return auth.ErrAuthenticationRequired
	}
	return nil
}
func currentProfile(ctx context.Context, tx *sql.Tx, owner string) (ProfileResult, error) {
	result := ProfileResult{AccountID: owner, Document: json.RawMessage("null")}
	err := tx.QueryRowContext(ctx, "SELECT document FROM careeros_profiles WHERE account_id=$1", owner).Scan(&result.Document)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return result, unavailable(err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT profile_id FROM careeros_deleted_profiles WHERE account_id=$1 ORDER BY profile_id`, owner)
	if err != nil {
		return result, unavailable(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id []byte
		if err = rows.Scan(&id); err != nil {
			return result, unavailable(err)
		}
		result.DeletedProfileIDs = append(result.DeletedProfileIDs, string(id))
	}
	return result, unavailable(rows.Err())
}
func (s *Store) ReadProfile(ctx context.Context, p auth.Principal, operation string) (ProfileResult, error) {
	var out ProfileResult
	err := s.WithProfileResult(ctx, p, operation, func(v ProfileResult) error { out = v; return nil })
	return out, err
}

// WithProfileResult keeps current authority through the actual outer write.
// A Save result also passes here after its commit: deletion can replace its
// response with current empty authority, never release a stale career copy.
func (s *Store) WithProfileResult(ctx context.Context, p auth.Principal, operation string, write func(ProfileResult) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if s.pilot {
		if err = lockPilot(ctx, tx); err != nil {
			return err
		}
	}
	if err = authorized(ctx, tx, p); err != nil {
		return err
	}
	if s.pilot {
		if err = requirePilot(ctx, tx, p.AccountID, PilotRetrieve); err != nil {
			return err
		}
	}
	result, err := currentProfile(ctx, tx, p.AccountID)
	if err != nil {
		return err
	}
	if operation != "" {
		var id []byte
		out := &Outcome{OperationID: operation}
		err = tx.QueryRowContext(ctx, "SELECT profile_id,committed_revision FROM careeros_profile_operations WHERE account_id=$1 AND operation_id=$2", p.AccountID, operation).Scan(&id, &out.CommittedRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return unavailable(err)
		}
		out.ProfileID = string(id)
		result.Outcome = out
	}
	if err = authorized(ctx, tx, p); err != nil {
		return err
	}
	if s.pilot {
		if err = requirePilot(ctx, tx, p.AccountID, PilotRetrieve); err != nil {
			return err
		}
	}
	if err = write(result); err != nil {
		return err
	}
	return unavailable(tx.Commit())
}

func validOperation(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// StrictJSON rejects encodings that encoding/json would silently replace.
// JSON bytes are stored verbatim; U+0000 and finite opaque numbers are supported.
func StrictJSON(raw []byte) bool {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		end, valid := jsonStringEnd(raw, i+1)
		if !valid {
			return false
		}
		i = end
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return uniqueJSONKeys(decoder, 0) == nil
}

// json.Valid has already established JSON syntax and escape bounds. These
// helpers additionally reject lone surrogates instead of replacing their text.
func jsonStringEnd(raw []byte, i int) (int, bool) {
	for i < len(raw) && raw[i] != '"' {
		if raw[i] != '\\' {
			i++
			continue
		}
		if raw[i+1] != 'u' {
			i += 2
			continue
		}
		var valid bool
		i, valid = unicodeEscapeEnd(raw, i)
		if !valid {
			return i, false
		}
	}
	return i, true
}

func unicodeEscapeEnd(raw []byte, i int) (int, bool) {
	n, _ := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
	i += 6
	if n >= 0xdc00 && n <= 0xdfff {
		return i, false
	}
	if n < 0xd800 || n > 0xdbff {
		return i, true
	}
	if i+6 > len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
		return i, false
	}
	low, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
	return i + 6, err == nil && low >= 0xdc00 && low <= 0xdfff
}

// JSON objects are maps in the shared domain. Reject duplicate keys rather
// than choosing a different winner in Go, JavaScript, hashing or later reads.
func uniqueJSONKeys(decoder *json.Decoder, depth int) error {
	if depth > 128 {
		return ErrValidation
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return ErrValidation
			}
			keys[name] = true
		}
		if err := uniqueJSONKeys(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

// SaveProfile publishes the document and a content-free operation receipt in
// the SAME transaction as current authorization and ownership validation.
// Replay recognition deliberately precedes stale-base validation.
func (s *Store) SaveProfile(ctx context.Context, p auth.Principal, operation string, expected int64, raw []byte) (ProfileResult, error) {
	if !validOperation(operation) || expected < 0 || expected >= 9007199254740991 || len(raw) > MaxDocumentBytes || !StrictJSON(raw) {
		return ProfileResult{}, ErrValidation
	}
	doc, err := profiledocument.Decode(raw)
	if err != nil || (expected > 0 && doc.Revision != expected+1) {
		return ProfileResult{}, ErrValidation
	}
	hash := sha256.Sum256(append([]byte(fmt.Sprintf("profile-save-v1:%d:", expected)), raw...))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProfileResult{}, unavailable(err)
	}
	defer tx.Rollback()
	if s.pilot {
		if err = lockPilot(ctx, tx); err != nil {
			return ProfileResult{}, err
		}
	}
	if err = authorized(ctx, tx, p); err != nil {
		return ProfileResult{}, err
	}
	if s.pilot {
		if err = requirePilot(ctx, tx, p.AccountID, PilotStandalone); err != nil {
			return ProfileResult{}, err
		}
	}
	var deleted bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_deleted_profiles WHERE account_id=$1 AND profile_id=$2)`, p.AccountID, []byte(doc.ID)).Scan(&deleted); err != nil {
		return ProfileResult{}, unavailable(err)
	}
	if deleted {
		return ProfileResult{}, ErrConflict
	}
	if result, completed, err := replayProfile(ctx, tx, p.AccountID, operation, hash[:]); completed || err != nil {
		return result, err
	}
	var pruned bool
	ids := make([]string, 0, len(doc.Evidence))
	for _, e := range doc.Evidence {
		ids = append(ids, e.ID)
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_profile_pruning WHERE account_id=$1 AND profile_id=$2 AND (evidence_id=ANY($3) OR new_revision>=$4 OR NOT EXISTS(SELECT 1 FROM careeros_profiles WHERE account_id=$1)))`, p.AccountID, []byte(doc.ID), ids, doc.Revision).Scan(&pruned); err != nil {
		return ProfileResult{}, unavailable(err)
	}
	if pruned {
		return ProfileResult{}, ErrConflict
	}
	current, err := currentProfile(ctx, tx, p.AccountID)
	if err != nil {
		return current, err
	}
	if err := validateProfileBase(current.Document, doc, expected); err != nil {
		return current, err
	}
	if !retainedEvidenceAllowed(profiledocument.Document{}, doc) {
		return current, ErrValidation
	}
	// The scoped hosted runtime uses a private PostgreSQL constraint trigger
	// to write pruning fences when the Profile and receipt transaction commits.
	if !s.hostedProfilePruning {
		if err = recordProfilePruning(ctx, tx, p.AccountID, current.Document, raw); err != nil {
			return current, err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO careeros_profiles(account_id,profile_id,revision,document) VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET profile_id=EXCLUDED.profile_id,revision=EXCLUDED.revision,document=EXCLUDED.document", p.AccountID, []byte(doc.ID), doc.Revision, raw)
	if err != nil {
		return current, unavailable(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO careeros_profile_operations
(account_id,operation_id,request_hash,profile_id,committed_revision) VALUES($1,$2,$3,$4,$5)`, p.AccountID, operation, hash[:], []byte(doc.ID), doc.Revision)
	if err != nil {
		return current, unavailable(err)
	}
	result := ProfileResult{AccountID: p.AccountID, Document: append(json.RawMessage(nil), raw...), Outcome: &Outcome{operation, doc.ID, doc.Revision}}
	return result, unavailable(tx.Commit())
}

// A completed operation is recognized before stale-base checks. It returns its
// historical receipt alongside current authority and never publishes again.
func replayProfile(ctx context.Context, tx *sql.Tx, owner, operation string, hash []byte) (ProfileResult, bool, error) {
	var priorHash, priorID []byte
	var revision int64
	err := tx.QueryRowContext(ctx, "SELECT request_hash,profile_id,committed_revision FROM careeros_profile_operations WHERE account_id=$1 AND operation_id=$2", owner, operation).Scan(&priorHash, &priorID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return ProfileResult{}, false, nil
	}
	if err != nil {
		return ProfileResult{}, false, unavailable(err)
	}
	if !bytes.Equal(priorHash, hash) {
		return ProfileResult{}, true, ErrConflict
	}
	result, err := currentProfile(ctx, tx, owner)
	if err != nil {
		return result, true, err
	}
	result.Outcome = &Outcome{operation, string(priorID), revision}
	return result, true, unavailable(tx.Commit())
}

func validateProfileBase(raw []byte, doc profiledocument.Document, expected int64) error {
	if bytes.Equal(raw, []byte("null")) {
		if expected != 0 {
			return ErrConflict
		}
		return nil
	}
	before, err := profiledocument.Decode(raw)
	if err != nil {
		return auth.ErrUnavailable
	}
	if before.ID != doc.ID || before.Revision != expected {
		return ErrConflict
	}
	if !retainedEvidenceAllowed(before, doc) {
		return ErrValidation
	}
	return nil
}

// #258 D02-D04: approved excerpts must reference remaining facts; deleting a
// fact removes its affected shared excerpt. Reject violations; never strip
// content in the storage adapter. Historical invalidated links may remain.
func retainedEvidenceAllowed(before, after profiledocument.Document) bool {
	facts := map[string]bool{}
	for _, f := range after.Facts {
		facts[f.ID] = true
	}
	forbidden := removedFactEvidence(before, facts)
	referenced := map[string]bool{}
	for _, l := range after.Links {
		if l.Kind == "supports" && l.State == "active" && facts[l.From.ID] {
			referenced[l.To.ID] = true
		}
	}
	for _, e := range after.Evidence {
		if e.Approval != "approved" || !referenced[e.ID] || forbidden[e.ID] {
			return false
		}
	}
	return true
}

func removedFactEvidence(before profiledocument.Document, remaining map[string]bool) map[string]bool {
	removed := map[string]bool{}
	for _, f := range before.Facts {
		if !remaining[f.ID] {
			removed[f.ID] = true
		}
	}
	forbidden := map[string]bool{}
	for _, l := range before.Links {
		if l.Kind == "supports" && removed[l.From.ID] {
			forbidden[l.To.ID] = true
		}
	}
	return forbidden
}
