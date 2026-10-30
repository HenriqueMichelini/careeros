// Package aicontrol is a synthetic-only, disposable PostgreSQL admission
// laboratory. Nothing in this package discovers credentials, opens provider
// accounts, installs production schemas or enables managed network egress.
package aicontrol

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

var (
	ErrDenied            = errors.New("synthetic admission denied")
	ErrConflict          = errors.New("synthetic immutable binding conflict")
	ErrUnavailable       = errors.New("synthetic authority unavailable")
	ErrOutputUnavailable = errors.New("synthetic output unavailable; no redispatch")
	ErrCapacity          = errors.New("synthetic capacity exhausted")
)

//go:embed schema.sql
var fixtureSchema string

// ReviewClock is deliberately independent of DB admission expiry. Unknown clock
// health denies time-based review. It cannot change session/grant expiry authority.
type ReviewClock interface{ Sample() (time.Time, bool) }
type systemClock struct{}

func (systemClock) Sample() (time.Time, bool) { return time.Now().UTC(), true }

// SyntheticConfig accepts only finite laboratory values, in synthetic integer
// SYN units. No price, currency, customer right or production policy is selected.
type SyntheticConfig struct {
	Namespace                                       string
	BindingKey                                      [32]byte
	Limit                                           int64
	Capacity, MaxDispatches, MaxStages, MaxAttempts int
	CustomerUnits                                   int64
	MaxInputBytes, MaxOutputTokens                  int64
	ExpiresAt                                       time.Time
	Gate                                            *SyntheticGate
	Clock                                           ReviewClock
}

func DefaultSyntheticConfig(namespace string, gate *SyntheticGate) SyntheticConfig {
	return SyntheticConfig{Namespace: namespace, BindingKey: sha256.Sum256([]byte("synthetic-only-key-not-a-credential")), Limit: 100, Capacity: 1, MaxDispatches: 32, MaxStages: 32, MaxAttempts: 3, CustomerUnits: 3, MaxInputBytes: 16 << 20, MaxOutputTokens: 1000000, ExpiresAt: time.Now().Add(time.Hour), Gate: gate, Clock: systemClock{}}
}

type Harness struct {
	db          *sql.DB
	reviewerDB  *sql.DB
	supportRole string
	cfg         SyntheticConfig
	mu          sync.Mutex
	responder   func(context.Context, string, *http.Request) (*http.Response, error)
	hooks       map[string]func() error
}

var tokenLabel = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var coordinateLabel = regexp.MustCompile(`^[A-Za-z0-9_:./-]{1,512}$`)
var workflows = map[string]bool{"profile_ingestion": true, "profile_review": true, "cv_generation": true, "qualification_gaps": true, "application_draft": true}
var stages = map[string]bool{"field_validation": true, "extraction": true, "reconciliation": true, "section_review": true, "section_support_check": true, "generation": true, "resume_support": true, "artifact_support": true, "qualification_matching": true, "requirement_evidence": true, "statement_support_assessment": true}

// NewSynthetic requires an explicitly named isolated schema on a Unix socket or
// literal loopback address. It never reads environment variables or account files.
// The caller must first install existing account tables in this disposable schema.
func NewSynthetic(ctx context.Context, dsn string, cfg SyntheticConfig) (*Harness, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgresql" && u.Scheme != "postgres") {
		return nil, ErrDenied
	}
	host := u.Query().Get("host")
	if host == "" {
		host = u.Hostname()
	}
	if !(strings.HasPrefix(host, "/") || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
		return nil, ErrDenied
	}
	schema := u.Query().Get("search_path")
	if !strings.HasPrefix(schema, "careeros_ai_test_") || !tokenLabel.MatchString(schema) || !tokenLabel.MatchString(cfg.Namespace) || cfg.Gate == nil || cfg.Clock == nil || cfg.BindingKey == ([32]byte{}) || cfg.Limit <= 0 || cfg.Limit > math.MaxInt64/4 || cfg.Capacity <= 0 || cfg.Capacity > 10000 || cfg.MaxDispatches <= 0 || cfg.MaxDispatches > 4096 || cfg.MaxStages <= 0 || cfg.MaxStages > 4096 || cfg.MaxAttempts <= 0 || cfg.MaxAttempts > 100 || cfg.CustomerUnits <= 0 || cfg.MaxInputBytes <= 0 || cfg.MaxInputBytes > 1<<40 || cfg.MaxOutputTokens <= 0 || cfg.MaxOutputTokens > 1<<40 || !cfg.ExpiresAt.After(time.Now()) {
		return nil, ErrDenied
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, ErrUnavailable
	}
	db.SetMaxOpenConns(8)
	fail := func() (*Harness, error) { db.Close(); return nil, ErrUnavailable }
	if db.PingContext(ctx) != nil {
		return fail()
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fail()
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT to_regclass('ai_fixture_namespace') IS NOT NULL").Scan(&exists); err != nil {
		return fail()
	}
	if !exists {
		if _, err = tx.ExecContext(ctx, fixtureSchema); err != nil {
			return fail()
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_namespace(id,epoch) VALUES($1,1) ON CONFLICT DO NOTHING`, cfg.Namespace)
	if err != nil {
		return fail()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_policy(namespace,expires_at,max_dispatches,max_attempts,max_stages,units,max_input_bytes,max_output_tokens) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, cfg.Namespace, cfg.ExpiresAt, cfg.MaxDispatches, cfg.MaxAttempts, cfg.MaxStages, cfg.CustomerUnits, cfg.MaxInputBytes, cfg.MaxOutputTokens)
	if err != nil {
		return fail()
	}
	for _, scope := range []string{"global", "provider:openai", "provider:typesafe"} {
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_fixture_scope(namespace,scope,limit_amount,capacity) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, cfg.Namespace, scope, cfg.Limit, cfg.Capacity); err != nil {
			return fail()
		}
	}
	if tx.Commit() != nil {
		return fail()
	}
	runtimeDB, reviewerDB, supportRole, err := syntheticRolePools(ctx, db, dsn)
	if err != nil {
		return fail()
	}
	db.Close()
	return &Harness{db: runtimeDB, reviewerDB: reviewerDB, supportRole: supportRole, cfg: cfg, hooks: map[string]func() error{}}, nil
}
func (h *Harness) Close() error {
	if h.reviewerDB != nil {
		h.reviewerDB.Close()
	}
	return h.db.Close()
}

// SetResponder installs an in-memory synthetic provider. Send never uses an HTTP
// client or network transport. The callback receives a clone without credentials.
func (h *Harness) SetResponder(fn func(context.Context, string, *http.Request) (*http.Response, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.responder = fn
}

// SetFault is an explicit local fault-injection seam; it cannot mint a permit.
func (h *Harness) SetFault(phase string, fn func() error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks[phase] = fn
}
func (h *Harness) fault(phase string) error {
	h.mu.Lock()
	f := h.hooks[phase]
	h.mu.Unlock()
	if f != nil {
		return f()
	}
	return nil
}
func (h *Harness) ready(ctx context.Context, tx *sql.Tx) error {
	var epoch int64
	var retired bool
	if tx.QueryRowContext(ctx, `SELECT epoch,retired FROM ai_fixture_namespace WHERE id=$1`, h.cfg.Namespace).Scan(&epoch, &retired) != nil {
		return ErrUnavailable
	}
	if retired || !h.cfg.Gate.permits(h.cfg.Namespace, epoch) {
		return ErrDenied
	}
	return nil
}
func (h *Harness) begin(ctx context.Context, p auth.Principal) (*sql.Tx, error) {
	tx, e := h.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, ErrUnavailable
	}
	if e = accountstore.AuthorizeTransaction(ctx, tx, p); e != nil {
		tx.Rollback()
		return nil, e
	}
	if e = h.ready(ctx, tx); e != nil {
		tx.Rollback()
		return nil, e
	}
	return tx, nil
}
func opaque() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
func (h *Harness) fingerprint(owner, purpose string, raw []byte) ([]byte, error) {
	if h.cfg.BindingKey == ([32]byte{}) || len(raw) > 1<<20 || !accountstore.StrictJSON(raw) {
		return nil, ErrDenied
	}
	var v any
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return nil, ErrDenied
	}
	// Credentials are transport material, never replay identity. Only remove
	// reserved transport fields at the envelope boundary; opaque career data is
	// not recursively rewritten. Valid workflow envelopes have no such fields.
	if fields, ok := v.(map[string]any); ok {
		for _, key := range []string{"apiKey", "typeSafeApiKey", "typesafeApiKey", "openaiApiKey", "openAIKey", "headers", "authorization", "csrfToken", "sessionToken"} {
			delete(fields, key)
		}
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return nil, ErrDenied
	}
	mac := hmac.New(sha256.New, h.cfg.BindingKey[:])
	for _, part := range [][]byte{[]byte("synthetic-ai-binding-v1"), []byte(h.cfg.Namespace), []byte(owner), []byte(purpose), canonical} {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(part)))
		mac.Write(n[:])
		mac.Write(part)
	}
	return mac.Sum(nil), nil
}
func safe(err error) error {
	if err == nil {
		return nil
	}
	return ErrUnavailable
}
func countMax(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func checkedSum(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, ErrDenied
	}
	return a + b, nil
}
func (h *Harness) String() string { return "aicontrol synthetic authority" }
func labelError(s string) error   { return fmt.Errorf("%w: %s", ErrDenied, s) }
