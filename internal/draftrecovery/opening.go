package draftrecovery

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
)

const OpeningPolicyVersion = "complete-draft-opening-local-20261122-v1"

type OpeningStatus struct {
	Policy      string     `json:"policy"`
	State       string     `json:"state"`
	ConfirmedAt *time.Time `json:"confirmedAt,omitempty"`
}

type AllowanceStatus struct {
	Allowance int `json:"allowance"`
	Reserved  int `json:"reserved"`
	Consumed  int `json:"consumed"`
	Remaining int `json:"remaining"`
}

type OpeningManifest struct {
	Schema        string   `json:"schema"`
	BundleDigest  string   `json:"bundleDigest"`
	Version       int      `json:"artifactVersion"`
	RequiredParts []string `json:"requiredParts"`
}

type OpeningReceipt struct {
	Receipt      string          `json:"receipt"`
	BundleDigest string          `json:"bundleDigest"`
	Manifest     OpeningManifest `json:"manifest"`
}

type OpeningConfirmation struct {
	Receipt      string          `json:"receipt"`
	BundleDigest string          `json:"bundleDigest"`
	Manifest     OpeningManifest `json:"manifest"`
}

// OpenLocalOpening exposes the selected protocol only in an explicit disposable
// controlled-provider composition. Existing managed/default constructors remain
// unchanged until the separate terminal-integration slice is complete.
func OpenLocalOpening(ctx context.Context, dsn string, cfg Config) (*Store, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		!strings.HasPrefix(u.Query().Get("search_path"), "careeros_ai_test_") || !validToken(u.Query().Get("search_path")) {
		return nil, ErrDenied
	}
	host := u.Query().Get("host")
	if host == "" {
		host = u.Hostname()
	}
	if ip := net.ParseIP(host); !strings.HasPrefix(host, "/") && (ip == nil || !ip.IsLoopback()) {
		return nil, ErrDenied
	}
	if cfg.Decisions == nil {
		return nil, ErrDenied
	}
	additional := cfg.Decisions
	cfg.Decisions = DecisionFunc(func(ctx context.Context, a Authority) (Decision, error) {
		base, err := managedPilotDecision(ctx, a)
		if err != nil {
			return base, err
		}
		extra, err := additional.Decide(ctx, a)
		if err != nil {
			return base, err
		}
		// Controlled deletion/cancellation policy can further restrict the
		// real grant. A supplied flag cannot grant access or invent delivery.
		base.Generate = base.Generate && extra.Generate
		base.Publish = base.Publish && extra.Publish
		base.Read = base.Read && extra.Read
		base.Revoked = base.Revoked || extra.Revoked
		base.Deleted, base.Cancelled = extra.Deleted, extra.Cancelled
		if extra.Epoch != 0 {
			base.Epoch = extra.Epoch
		}
		if extra.GrantExpiresAt.Before(base.GrantExpiresAt) {
			base.GrantExpiresAt = extra.GrantExpiresAt
		}
		return base, nil
	})
	s, err := Open(ctx, dsn, cfg)
	if err != nil {
		return nil, err
	}
	s.opening = true
	return s, nil
}

func (s *Store) OpeningEnabled() bool { return s != nil && s.opening }

type openingGrant struct {
	activated, expires time.Time
	active             bool
}

func (s *Store) openingGrant(ctx context.Context, tx *sql.Tx, p auth.Principal) (openingGrant, error) {
	var g openingGrant
	var activated, expires sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT p.activated_at,p.expires_at,
 p.state='active' AND p.age_eligible AND NOT p.invitation_revoked AND p.draft_allowance=5
 AND p.participant_slot BETWEEN 1 AND 5
 FROM careeros_pilot_access p JOIN careeros_identities i USING(provider,issuer,subject)
 WHERE p.account_id=$1 AND i.account_id=$1 FOR UPDATE OF p`, p.AccountID).Scan(&activated, &expires, &g.active)
	if err != nil || !activated.Valid || !expires.Valid || activated.Time.UnixMicro() <= 0 {
		return g, ErrDenied
	}
	g.activated, g.expires = activated.Time, expires.Time
	return g, nil
}

func openingCounts(ctx context.Context, tx *sql.Tx, owner string) (AllowanceStatus, error) {
	v := AllowanceStatus{Allowance: 5}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE state='reserved'),count(*) FILTER(WHERE state='delivered') FROM careeros_draft_allowance WHERE owner=$1`, owner).Scan(&v.Reserved, &v.Consumed); err != nil {
		return v, ErrDenied
	}
	v.Remaining = 5 - v.Reserved - v.Consumed
	if v.Remaining < 0 {
		return v, ErrDenied
	}
	return v, nil
}

func (s *Store) Allowance(ctx context.Context, p auth.Principal) (AllowanceStatus, error) {
	if !s.OpeningEnabled() {
		return AllowanceStatus{}, ErrDenied
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return AllowanceStatus{}, err
	}
	defer tx.Rollback()
	if _, err = s.openingGrant(ctx, tx, p); err != nil {
		return AllowanceStatus{}, err
	}
	if err = s.resolveOpeningReservations(ctx, tx, p); err != nil {
		return AllowanceStatus{}, err
	}
	v, err := openingCounts(ctx, tx, p.AccountID)
	if err != nil {
		return v, err
	}
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return v, err
	}
	return v, tx.Commit()
}

func (s *Store) reserveOpening(ctx context.Context, tx *sql.Tx, p auth.Principal, a Action, admitted time.Time) error {
	g, err := s.openingGrant(ctx, tx, p)
	if err != nil {
		return err
	}
	var now time.Time
	if tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !g.active || !now.Before(g.expires) || !now.Before(admitted.Add(CustomerWindow)) {
		return ErrDenied
	}
	// Create has already resolved older reservations and rechecked their
	// restrictive authority before inserting the new generation witness.
	if err = accountstore.AuthorizeTransaction(ctx, tx, p); err != nil {
		return err
	}
	if tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil || !now.Before(g.expires) || !now.Before(admitted.Add(CustomerWindow)) {
		return ErrDenied
	}
	v, err := openingCounts(ctx, tx, p.AccountID)
	if err != nil {
		return err
	}
	if v.Remaining == 0 {
		return aicontrol.ErrCapacity
	}
	deadline := admitted.Add(CustomerWindow)
	if g.expires.Before(deadline) {
		deadline = g.expires
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO careeros_draft_allowance(namespace,action_id,owner,grant_activated_at,grant_expires_at,admitted_at,deadline,state) VALUES($1,$2,$3,$4,$5,$6,$7,'reserved')`, a.Namespace, a.ID, p.AccountID, g.activated, g.expires, admitted, deadline); err != nil {
		return ErrDenied
	}
	updated, err := tx.ExecContext(ctx, `UPDATE ai_fixture_action SET customer_reserved=true WHERE namespace=$1 AND id=$2 AND owner=$3 AND workflow='application_draft' AND state='recoverable' AND NOT customer_reserved AND NOT customer_effect`, a.Namespace, a.ID, p.AccountID)
	if err != nil {
		return ErrDenied
	}
	if n, e := updated.RowsAffected(); e != nil || n != 1 {
		return ErrDenied
	}
	return nil
}

// A status/admission decision can observe an elapsed original deadline even
// when nobody requests that old result. Resolving it is provider-free, ordered
// by the already held owner lock, and touches only this finite grant's holds.
func (s *Store) resolveOpeningReservations(ctx context.Context, tx *sql.Tx, p auth.Principal) error {
	rows, err := tx.QueryContext(ctx, `SELECT a.creation_key FROM careeros_draft_allowance o JOIN careeros_draft_actions a ON a.namespace=o.namespace AND a.action_id=o.action_id WHERE o.owner=$1 AND o.state='reserved' ORDER BY a.creation_key`, p.AccountID)
	if err != nil {
		return ErrDenied
	}
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return ErrDenied
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ErrDenied
	}
	for _, key := range keys {
		if _, _, _, err = s.load(ctx, tx, p, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) releaseOpening(ctx context.Context, tx *sql.Tx, r record) error {
	if _, err := tx.ExecContext(ctx, `UPDATE careeros_draft_allowance SET state='released',released_at=clock_timestamp() WHERE namespace=$1 AND action_id=$2 AND state='reserved'`, r.Action.Namespace, r.Action.ID); err != nil {
		return ErrDenied
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ai_fixture_action SET customer_reserved=false WHERE namespace=$1 AND id=$2 AND NOT customer_effect`, r.Action.Namespace, r.Action.ID); err != nil {
		return ErrDenied
	}
	return nil
}

func (s *Store) openingStatus(ctx context.Context, tx *sql.Tx, r record) (*OpeningStatus, error) {
	v := &OpeningStatus{Policy: OpeningPolicyVersion}
	var at sql.NullTime
	var reserved, effect bool
	err := tx.QueryRowContext(ctx, `SELECT o.state,o.confirmed_at,a.customer_reserved,a.customer_effect FROM careeros_draft_allowance o JOIN ai_fixture_action a ON a.namespace=o.namespace AND a.id=o.action_id WHERE o.namespace=$1 AND o.action_id=$2 AND o.owner=$3`, r.Action.Namespace, r.Action.ID, r.Action.Owner).Scan(&v.State, &at, &reserved, &effect)
	if errors.Is(err, sql.ErrNoRows) && r.Version == 0 {
		return nil, nil
	}
	if err != nil || reserved != (v.State == "reserved") || effect != (v.State == "delivered") || at.Valid != effect {
		return nil, ErrDenied
	}
	if at.Valid {
		v.ConfirmedAt = &at.Time
	}
	return v, nil
}

func openingManifest(r record) OpeningManifest {
	return OpeningManifest{OpeningPolicyVersion, r.publishedDigest, 1, []string{"role_summary", "resume", "cover_letter", "employer_answers", "support"}}
}

func (s *Store) openingBundle(ctx context.Context, tx *sql.Tx, r record) error {
	var raw []byte
	if tx.QueryRowContext(ctx, `SELECT bundle FROM careeros_draft_content WHERE namespace=$1 AND action_id=$2 FOR SHARE`, r.Action.Namespace, r.Action.ID).Scan(&raw) != nil {
		return ErrDenied
	}
	var b Bundle
	if json.Unmarshal(raw, &b) != nil || s.digest("bundle", raw) != r.publishedDigest || !reflectBinding(r.binding, b.Binding) || b.Schema != BundleVersion || b.Support.Completion == nil || b.Support.Completion.State != "complete" {
		return ErrDenied
	}
	return nil
}

func (s *Store) receiptToken(r record, id, session, manifest string) string {
	raw, _ := json.Marshal([]string{OpeningPolicyVersion, r.Action.Namespace, r.Action.ID, r.Action.Owner, id, session, manifest})
	return id + "." + s.digest("opening-receipt", raw)
}

func denyOpening(tx *sql.Tx, r record) error {
	if r.State == "closed" || r.State == "deleted" {
		if err := tx.Commit(); err != nil {
			return ErrDenied
		}
	}
	return ErrDenied
}

func (s *Store) IssueOpening(ctx context.Context, p auth.Principal, key string) (OpeningReceipt, error) {
	var out OpeningReceipt
	if !s.OpeningEnabled() || !validToken(key) {
		return out, ErrDenied
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	r, d, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return out, err
	}
	if r.State != "ready" || r.Version != 1 || !d.Read || d.Deleted || r.Opening == nil || r.Opening.State == "released" {
		return out, denyOpening(tx, r)
	}
	if err = s.openingBundle(ctx, tx, r); err != nil {
		return out, err
	}
	out.BundleDigest, out.Manifest = r.publishedDigest, openingManifest(r)
	raw, _ := json.Marshal(out.Manifest)
	manifest, session := s.digest("opening-manifest", raw), s.digest("opening-session", []byte(p.SessionID))
	var id string
	err = tx.QueryRowContext(ctx, `SELECT receipt_id FROM careeros_draft_opening_receipts WHERE namespace=$1 AND action_id=$2 AND session_binding=$3 AND manifest_digest=$4`, r.Action.Namespace, r.Action.ID, session, manifest).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return out, ErrDenied
		}
		id = hex.EncodeToString(nonce[:])
		_, err = tx.ExecContext(ctx, `INSERT INTO careeros_draft_opening_receipts(namespace,action_id,receipt_id,session_binding,artifact_version,bundle_digest,manifest_digest,policy_version,issued_at) VALUES($1,$2,$3,$4,1,$5,$6,$7,clock_timestamp())`, r.Action.Namespace, r.Action.ID, id, session, r.publishedDigest, manifest, OpeningPolicyVersion)
	}
	if err != nil {
		return out, ErrDenied
	}
	latest, final, _, err := s.load(ctx, tx, p, key)
	if err != nil {
		return out, err
	}
	if latest.State != "ready" || !final.Read || final.Deleted {
		return out, denyOpening(tx, latest)
	}
	out.Receipt = s.receiptToken(r, id, session, manifest)
	if err = tx.Commit(); err != nil {
		return OpeningReceipt{}, ErrDenied
	}
	return out, s.fault("after_opening_receipt_commit")
}

func (s *Store) ConfirmOpening(ctx context.Context, p auth.Principal, key string, c OpeningConfirmation) error {
	parts := strings.Split(c.Receipt, ".")
	if !s.OpeningEnabled() || !validToken(key) || len(parts) != 2 || len(parts[0]) != 32 || len(parts[1]) != 64 {
		return ErrInvalid
	}
	tx, err := s.begin(ctx, p)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Acquire every mutable settlement row before the authoritative clock.
	// Neither the grant lock nor a reservation UPDATE may wait after that clock.
	if _, err = s.openingGrant(ctx, tx, p); err != nil {
		return err
	}
	r, d, now, err := s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State != "ready" || !d.Read || d.Deleted || r.Opening == nil || r.Opening.State == "released" {
		return denyOpening(tx, r)
	}
	var settlement string
	if err = tx.QueryRowContext(ctx, `SELECT state FROM careeros_draft_allowance WHERE namespace=$1 AND action_id=$2 AND owner=$3 FOR UPDATE`, r.Action.Namespace, r.Action.ID, p.AccountID).Scan(&settlement); err != nil {
		return ErrDenied
	}
	manifestRaw, _ := json.Marshal(openingManifest(r))
	provided, _ := json.Marshal(c.Manifest)
	manifest, session := s.digest("opening-manifest", manifestRaw), s.digest("opening-session", []byte(p.SessionID))
	if c.BundleDigest != r.publishedDigest || !hmac.Equal(provided, manifestRaw) || !hmac.Equal([]byte(c.Receipt), []byte(s.receiptToken(r, parts[0], session, manifest))) {
		return ErrInvalid
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_draft_opening_receipts WHERE namespace=$1 AND action_id=$2 AND receipt_id=$3 AND session_binding=$4 AND artifact_version=1 AND bundle_digest=$5 AND manifest_digest=$6 AND policy_version=$7 AND issued_at<=$8)`, r.Action.Namespace, r.Action.ID, parts[0], session, r.publishedDigest, manifest, OpeningPolicyVersion, now).Scan(&valid); err != nil || !valid {
		return ErrInvalid
	}
	if err = s.openingBundle(ctx, tx, r); err != nil {
		return err
	}
	if err = s.fault("before_opening_commit"); err != nil {
		return err
	}
	r, d, now, err = s.load(ctx, tx, p, key)
	if err != nil {
		return err
	}
	if r.State != "ready" || !d.Read || d.Deleted || r.Opening == nil || r.Opening.State == "released" {
		return denyOpening(tx, r)
	}
	if r.Opening.State == "delivered" {
		return tx.Commit()
	}
	g, err := s.openingGrant(ctx, tx, p)
	if err != nil || !g.active || !now.Before(g.expires) || !now.Before(r.Deadline) || d.Revoked || d.Cancelled || d.Epoch != r.epoch {
		return denied(err)
	}
	updated, err := tx.ExecContext(ctx, `UPDATE careeros_draft_allowance SET state='delivered',confirmed_at=$4 WHERE namespace=$1 AND action_id=$2 AND owner=$3 AND state='reserved' AND grant_activated_at=$5 AND $4<deadline AND $4<grant_expires_at`, r.Action.Namespace, r.Action.ID, p.AccountID, now, g.activated)
	if err != nil {
		return ErrDenied
	}
	if n, e := updated.RowsAffected(); e != nil || n != 1 {
		return ErrDenied
	}
	updated, err = tx.ExecContext(ctx, `UPDATE ai_fixture_action SET state='delivered',customer_reserved=false,customer_effect=true WHERE namespace=$1 AND id=$2 AND owner=$3 AND state='candidate' AND customer_reserved AND NOT customer_effect`, r.Action.Namespace, r.Action.ID, p.AccountID)
	if err != nil {
		return ErrDenied
	}
	if n, e := updated.RowsAffected(); e != nil || n != 1 {
		return ErrDenied
	}
	if err = tx.Commit(); err != nil {
		return ErrDenied
	}
	return s.fault("after_opening_commit")
}
