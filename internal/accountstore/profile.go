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
	AccountID string          `json:"accountId"`
	Document  json.RawMessage `json:"document"`
	Outcome   *Outcome        `json:"outcome,omitempty"`
}

// authorized locks the account before its session. Every auth mutation that
// can revoke this authority takes that same account lock. The DB clock is
// checked after lock acquisition, including when a request waited at expiry.
func authorized(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
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
		return result, nil
	}
	return result, unavailable(err)
}
func (s *Store) ReadProfile(ctx context.Context, p auth.Principal, operation string) (ProfileResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProfileResult{}, unavailable(err)
	}
	defer tx.Rollback()
	if err = authorized(ctx, tx, p); err != nil {
		return ProfileResult{}, err
	}
	result, err := currentProfile(ctx, tx, p.AccountID)
	if err != nil {
		return result, err
	}
	if operation != "" {
		var id []byte
		out := &Outcome{OperationID: operation}
		err = tx.QueryRowContext(ctx, "SELECT profile_id,committed_revision FROM careeros_profile_operations WHERE account_id=$1 AND operation_id=$2", p.AccountID, operation).Scan(&id, &out.CommittedRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return result, ErrNotFound
		}
		if err != nil {
			return result, unavailable(err)
		}
		out.ProfileID = string(id)
		result.Outcome = out
	}
	return result, unavailable(tx.Commit())
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
		i++
		for i < len(raw) && raw[i] != '"' {
			if raw[i] != '\\' {
				i++
				continue
			}
			i++
			if raw[i] != 'u' {
				i++
				continue
			}
			n, _ := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			i += 5
			if n >= 0xdc00 && n <= 0xdfff {
				return false
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 > len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
					return false
				}
				low, e := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
				if e != nil || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return uniqueJSONKeys(decoder, 0) == nil
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
	if err = authorized(ctx, tx, p); err != nil {
		return ProfileResult{}, err
	}
	var priorHash, priorID []byte
	var revision int64
	err = tx.QueryRowContext(ctx, "SELECT request_hash,profile_id,committed_revision FROM careeros_profile_operations WHERE account_id=$1 AND operation_id=$2", p.AccountID, operation).Scan(&priorHash, &priorID, &revision)
	if err == nil {
		if !bytes.Equal(priorHash, hash[:]) {
			return ProfileResult{}, ErrConflict
		}
		result, e := currentProfile(ctx, tx, p.AccountID)
		if e != nil {
			return result, e
		}
		result.Outcome = &Outcome{operation, string(priorID), revision}
		return result, unavailable(tx.Commit())
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ProfileResult{}, unavailable(err)
	}
	current, err := currentProfile(ctx, tx, p.AccountID)
	if err != nil {
		return current, err
	}
	if bytes.Equal(current.Document, []byte("null")) {
		if expected != 0 {
			return current, ErrConflict
		}
	} else {
		before, e := profiledocument.Decode(current.Document)
		if e != nil {
			return current, auth.ErrUnavailable
		}
		if before.ID != doc.ID || before.Revision != expected {
			return current, ErrConflict
		}
		if !retainedEvidenceAllowed(before, doc) {
			return current, ErrValidation
		}
	}
	if !retainedEvidenceAllowed(profiledocument.Document{}, doc) {
		return current, ErrValidation
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO careeros_profiles(account_id,profile_id,revision,document) VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET profile_id=EXCLUDED.profile_id,revision=EXCLUDED.revision,document=EXCLUDED.document", p.AccountID, []byte(doc.ID), doc.Revision, raw)
	if err != nil {
		return current, unavailable(err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO careeros_profile_operations VALUES($1,$2,$3,$4,$5)", p.AccountID, operation, hash[:], []byte(doc.ID), doc.Revision)
	if err != nil {
		return current, unavailable(err)
	}
	result := ProfileResult{AccountID: p.AccountID, Document: append(json.RawMessage(nil), raw...), Outcome: &Outcome{operation, doc.ID, doc.Revision}}
	return result, unavailable(tx.Commit())
}

// #258 D02-D04: approved excerpts must reference remaining facts; deleting a
// fact removes its affected shared excerpt. Reject violations; never strip
// content in the storage adapter. Historical invalidated links may remain.
func retainedEvidenceAllowed(before, after profiledocument.Document) bool {
	facts := map[string]bool{}
	for _, f := range after.Facts {
		facts[f.ID] = true
	}
	removed := map[string]bool{}
	for _, f := range before.Facts {
		if !facts[f.ID] {
			removed[f.ID] = true
		}
	}
	forbidden := map[string]bool{}
	for _, l := range before.Links {
		if l.Kind == "supports" && removed[l.From.ID] {
			forbidden[l.To.ID] = true
		}
	}
	referenced := map[string]bool{}
	for _, l := range after.Links {
		if l.Kind == "supports" && facts[l.From.ID] {
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
