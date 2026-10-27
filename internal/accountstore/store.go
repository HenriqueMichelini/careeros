// Package accountstore composes auth and Profile authority in PostgreSQL.
package accountstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed" // Embed the schema used by the explicit migration command.
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Register the PostgreSQL database/sql driver.
	"professional-information-repo/internal/auth"
)

//go:embed schema.sql
var schema string

type Store struct{ db *sql.DB }

func Open(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		return nil, auth.ErrUnavailable
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, auth.ErrUnavailable
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, auth.ErrUnavailable
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

// Migrate is an explicit local/operator command, never run by request handlers.
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(72481301)"); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, schema); err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}
func unavailable(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: storage operation failed", auth.ErrUnavailable)
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

type accounts struct{ *Store }
type sessions struct{ *Store }
type transactions struct{ *Store }

func (s *Store) AuthStores() auth.Stores {
	return auth.Stores{Accounts: accounts{s}, Sessions: sessions{s}, Transactions: transactions{s}}
}

type scanner interface{ Scan(...any) error }

func scanAccount(row scanner) (auth.Account, error) {
	var a auth.Account
	var email, name []byte
	err := row.Scan(&a.ID, &a.Status, &a.Generation, &a.CreatedAt, &email, &name)
	a.Email = string(email)
	a.DisplayName = string(name)
	return a, err
}

const accountColumns = "id,status,generation,created_at,email,display_name"

func (s accounts) Get(ctx context.Context, id string) (auth.Account, bool, error) {
	a, err := scanAccount(s.db.QueryRowContext(ctx, "SELECT "+accountColumns+" FROM careeros_accounts WHERE id=$1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Account{}, false, nil
	}
	if err != nil {
		return a, false, unavailable(err)
	}
	rows, err := s.db.QueryContext(ctx, "SELECT provider,issuer,subject,created_at FROM careeros_identities WHERE account_id=$1", id)
	if err != nil {
		return a, false, unavailable(err)
	}
	defer rows.Close()
	for rows.Next() {
		var i auth.IdentityRef
		var p, iss, sub []byte
		if err = rows.Scan(&p, &iss, &sub, &i.CreatedAt); err != nil {
			return a, false, unavailable(err)
		}
		i.Provider = string(p)
		i.Issuer = string(iss)
		i.Subject = string(sub)
		a.Identities = append(a.Identities, i)
	}
	return a, true, unavailable(rows.Err())
}
func (s accounts) FindOrCreateBySubject(ctx context.Context, i auth.IdentityRef, email, name string, now time.Time) (auth.Account, bool, error) {
	if i.Provider == "" || i.Issuer == "" || i.Subject == "" {
		return auth.Account{}, false, auth.ErrInvalidCallback
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.Account{}, false, unavailable(err)
	}
	defer tx.Rollback()
	// One small creation lock prevents duplicate subjects without orphan accounts.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(72481302)"); err != nil {
		return auth.Account{}, false, unavailable(err)
	}
	a, err := scanAccount(tx.QueryRowContext(ctx, "SELECT a.id,a.status,a.generation,a.created_at,a.email,a.display_name FROM careeros_accounts a JOIN careeros_identities i ON i.account_id=a.id WHERE i.provider=$1 AND i.issuer=$2 AND i.subject=$3", []byte(i.Provider), []byte(i.Issuer), []byte(i.Subject)))
	created := false
	if errors.Is(err, sql.ErrNoRows) {
		id, e := newID()
		if e != nil {
			return a, false, e
		}
		a = auth.Account{ID: id, Status: auth.AccountActive, Generation: 1, CreatedAt: now.UTC(), Email: email, DisplayName: name}
		created = true
		_, err = tx.ExecContext(ctx, "INSERT INTO careeros_accounts VALUES($1,$2,$3,$4,$5,$6)", a.ID, a.Status, a.Generation, a.CreatedAt, []byte(email), []byte(name))
		if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO careeros_identities VALUES($1,$2,$3,$4,$5)", []byte(i.Provider), []byte(i.Issuer), []byte(i.Subject), a.ID, now.UTC())
		}
	}
	if err != nil {
		return a, false, unavailable(err)
	}
	i.CreatedAt = a.CreatedAt
	a.Identities = []auth.IdentityRef{i}
	return a, created, unavailable(tx.Commit())
}
func (s accounts) BumpGeneration(ctx context.Context, id string, _ time.Time) (auth.Account, error) {
	a, err := scanAccount(s.db.QueryRowContext(ctx, "UPDATE careeros_accounts SET generation=generation+1 WHERE id=$1 AND generation<18446744073709551615 RETURNING "+accountColumns, id))
	return a, unavailable(err)
}

func scanSession(row scanner) (auth.Session, error) {
	var s auth.Session
	var token, csrf []byte
	err := row.Scan(&s.ID, &s.AccountID, &token, &csrf, &s.Generation, &s.CreatedAt, &s.ExpiresAt)
	copy(s.TokenHash[:], token)
	copy(s.CSRFHash[:], csrf)
	return s, err
}

const sessionColumns = "id,account_id,token_hash,csrf_hash,generation,created_at,expires_at"

func (s sessions) FindByTokenHash(ctx context.Context, hash [32]byte, now time.Time) (auth.Session, bool, error) {
	v, err := scanSession(s.db.QueryRowContext(ctx, "SELECT "+sessionColumns+" FROM careeros_sessions WHERE token_hash=$1 AND expires_at>$2", hash[:], now))
	if errors.Is(err, sql.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, unavailable(err)
}
func (s sessions) Create(ctx context.Context, v auth.Session, previousID string, _ time.Time) error {
	if v.ID == "" || v.AccountID == "" {
		return auth.ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	// Account locks always precede session locks, in sorted order even when a
	// Google sign-in replaces a session belonging to a different account.
	previousOwner := ""
	if previousID != "" {
		if err = tx.QueryRowContext(ctx, "SELECT account_id FROM careeros_sessions WHERE id=$1", previousID).Scan(&previousOwner); errors.Is(err, sql.ErrNoRows) {
			return auth.ErrInvalidTransaction
		} else if err != nil {
			return unavailable(err)
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT id FROM careeros_accounts WHERE id=$1 OR id=$2 ORDER BY id FOR UPDATE", v.AccountID, previousOwner)
	if err != nil {
		return unavailable(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return unavailable(err)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return unavailable(err)
	}
	var status auth.AccountStatus
	var generation uint64
	if err = tx.QueryRowContext(ctx, "SELECT status,generation FROM careeros_accounts WHERE id=$1", v.AccountID).Scan(&status, &generation); err != nil {
		return unavailable(err)
	}
	if status != auth.AccountActive || generation != v.Generation {
		return auth.ErrInvalidTransaction
	}
	if previousID != "" {
		var valid bool
		// Unlike transaction-start now(), clock_timestamp() advances while this
		// transaction waits for account locks. The caller's clock is not authority.
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM careeros_sessions s JOIN careeros_accounts a ON a.id=s.account_id WHERE s.id=$1 AND s.expires_at>clock_timestamp() AND a.status='active' AND a.generation=s.generation)`, previousID).Scan(&valid)
		if err != nil {
			return unavailable(err)
		}
		if !valid {
			return auth.ErrInvalidTransaction
		}
		result, e := tx.ExecContext(ctx, "DELETE FROM careeros_sessions WHERE id=$1 AND expires_at>clock_timestamp()", previousID)
		if e != nil {
			return unavailable(e)
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return auth.ErrInvalidTransaction
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO careeros_sessions VALUES($1,$2,$3,$4,$5,$6,$7)", v.ID, v.AccountID, v.TokenHash[:], v.CSRFHash[:], v.Generation, v.CreatedAt, v.ExpiresAt)
	if err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}
func (s sessions) Revoke(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	// A row removed/replaced after this lookup is harmless. The common account
	// lock orders ordinary logout against Profile publication and replacement.
	var owner string
	err = tx.QueryRowContext(ctx, "SELECT account_id FROM careeros_sessions WHERE id=$1", id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, "SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE", owner); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM careeros_sessions WHERE id=$1", id); err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}
func (s sessions) RevokeAllForAccount(ctx context.Context, id string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, unavailable(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE", id); err != nil {
		return 0, unavailable(err)
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM careeros_sessions WHERE account_id=$1", id)
	if err != nil {
		return 0, unavailable(err)
	}
	n, _ := result.RowsAffected()
	return int(n), unavailable(tx.Commit())
}

func (s transactions) Create(ctx context.Context, v auth.Transaction, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(72481303)"); err != nil {
		return unavailable(err)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM careeros_oidc_transactions WHERE expires_at<=$1", now); err != nil {
		return unavailable(err)
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM careeros_oidc_transactions").Scan(&count); err != nil {
		return unavailable(err)
	}
	if count >= 10000 {
		return auth.ErrCapacity
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO careeros_oidc_transactions VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)", v.StateHash[:], v.BindingHash[:], v.ProviderID, v.Nonce, v.CodeVerifier, v.RedirectURI, v.ReturnTo, v.AccountID, v.Generation, v.PreviousSessionID, v.ExpiresAt)
	if err != nil {
		return unavailable(err)
	}
	return unavailable(tx.Commit())
}
func (s transactions) Consume(ctx context.Context, hash [32]byte, now time.Time) (auth.Transaction, bool, error) {
	var v auth.Transaction
	var state, binding []byte
	err := s.db.QueryRowContext(ctx, "DELETE FROM careeros_oidc_transactions WHERE state_hash=$1 RETURNING state_hash,binding_hash,provider,nonce,verifier,redirect_uri,return_to,account_id,generation,previous_session_id,expires_at", hash[:]).Scan(&state, &binding, &v.ProviderID, &v.Nonce, &v.CodeVerifier, &v.RedirectURI, &v.ReturnTo, &v.AccountID, &v.Generation, &v.PreviousSessionID, &v.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, false, nil
	}
	if err != nil {
		return v, false, unavailable(err)
	}
	copy(v.StateHash[:], state)
	copy(v.BindingHash[:], binding)
	return v, now.Before(v.ExpiresAt), nil
}
