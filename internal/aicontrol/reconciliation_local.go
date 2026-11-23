package aicontrol

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"

	"professional-information-repo/internal/auth"
)

// LocalReconciliationDSN rejects hosted routing, alternate connection options,
// and unnamed/non-disposable schemas before opening any connection.
func LocalReconciliationDSN(dsn string) bool {
	u, e := url.Parse(dsn)
	if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || !strings.HasSuffix(u.Path, "_test") {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	schema := u.Query().Get("search_path")
	if !strings.HasPrefix(schema, "careeros_ai_test_") || !tokenLabel.MatchString(schema) {
		return false
	}
	for k := range u.Query() {
		if k != "search_path" && k != "sslmode" {
			return false
		}
	}
	return true
}

// OpenLocalReconciliation opens an already installed, restricted reconciliation
// role. It never migrates a schema, assumes an admin identity, or opens admission.
func OpenLocalReconciliation(ctx context.Context, dsn, namespace string) (*Reconciliation, error) {
	if !LocalReconciliationDSN(dsn) || !tokenLabel.MatchString(namespace) {
		return nil, ErrDenied
	}
	u, _ := url.Parse(dsn)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, ErrUnavailable
	}
	var database, role string
	if db.QueryRowContext(ctx, `SELECT current_database(),current_user`).Scan(&database, &role) != nil {
		db.Close()
		return nil, ErrUnavailable
	}
	digest := sha256.Sum256([]byte(database + "/" + u.Query().Get("search_path")))
	if role != "ai_fixture_review_"+fmt.Sprintf("%x", digest[:8]) {
		db.Close()
		return nil, ErrDenied
	}
	cfg := DefaultSyntheticConfig(namespace, nil)
	cfg.BindingKey = [32]byte{}
	cfg.BindingKeyVersion = ""
	cfg.BindingVerificationKeys = nil
	return &Reconciliation{h: &Harness{db: db, reviewerDB: db, cfg: cfg, hooks: map[string]func() error{}}}, nil
}
func (r *Reconciliation) Close() error { return r.h.reviewerDB.Close() }

// ApplyLocal uses the CLI's authenticated session to attest a critical
// transition when the finance process has no enrolled operations observer.
func (r *Reconciliation) ApplyLocal(ctx context.Context, p auth.Principal, command ReconciliationCommand, sessionToken string) (ReconciliationPreview, error) {
	if len(sessionToken) < 1 || len(sessionToken) > 128 {
		return ReconciliationPreview{}, ErrDenied
	}
	return r.apply(ctx, p, command, sessionToken)
}
