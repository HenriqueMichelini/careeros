package accountapi_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Every database in this rehearsal is created by the test. No DSN,
// PostgreSQL service, or database credential is accepted from the operator.
type restorePostgres struct {
	tools, root, data, socket, dsn string
	running                        bool
}

func newRestorePostgres(t *testing.T, tools string) *restorePostgres {
	t.Helper()
	root, err := os.MkdirTemp("", "co293-")
	if err != nil {
		t.Fatal(err)
	}
	pg := &restorePostgres{tools: tools, root: root, data: filepath.Join(root, "data"), socket: filepath.Join(root, "sock")}
	t.Cleanup(func() {
		if pg.running {
			if err := pg.stop(); err != nil {
				t.Errorf("owned PostgreSQL stop failed; retained private fixture: %v", err)
				return
			}
		}
		if t.Failed() {
			log, err := os.ReadFile(filepath.Join(root, "postgres.log"))
			if err == nil {
				for _, line := range strings.Split(string(log), "\n") {
					if strings.Contains(line, "ERROR:") || strings.Contains(line, "FATAL:") {
						t.Log(line)
					}
				}
			}
		}
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	if err := os.Mkdir(pg.socket, 0700); err != nil {
		t.Fatal(err)
	}
	if len(pg.socket)+len("/.s.PGSQL.55493") >= 108 || strings.ContainsAny(pg.socket, " \t\n'\\") {
		t.Fatal("restore rehearsal needs a short private PostgreSQL temporary path")
	}
	q := url.Values{"host": {pg.socket}, "port": {"55493"}, "sslmode": {"disable"}, "connect_timeout": {"2"}}
	pg.dsn = (&url.URL{Scheme: "postgresql", User: url.User("restore_operator"), Path: "/careeros_restore_test", RawQuery: q.Encode()}).String()
	pg.must(t, "initdb", "-D", pg.data, "--username=restore_operator", "--no-locale", "--encoding=UTF8", "--auth-local=trust", "--auth-host=reject")
	pg.running = true
	pg.must(t, "pg_ctl", "-D", pg.data, "-l", filepath.Join(root, "postgres.log"),
		"-o", "-h '' -k "+pg.socket+" -p 55493", "-w", "start")
	admin, err := url.Parse(pg.dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin.Path = "/postgres"
	pg.must(t, "psql", "--no-psqlrc", "--set=ON_ERROR_STOP=on", "--dbname="+admin.String(), "--command=CREATE DATABASE careeros_restore_test")
	return pg
}

func (p *restorePostgres) command(program string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rtk", append([]string{"proxy", filepath.Join(p.tools, program)}, args...)...)
	// Client configuration cannot override the owned Unix socket or load a
	// caller's PostgreSQL service/passfile. Server subprocesses inherit this.
	for _, pair := range os.Environ() {
		name, _, _ := strings.Cut(pair, "=")
		if !strings.HasPrefix(name, "PG") {
			cmd.Env = append(cmd.Env, pair)
		}
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("private %s: %w: %s", program, err, output)
	}
	return nil
}

func (p *restorePostgres) must(t *testing.T, program string, args ...string) {
	t.Helper()
	if err := p.command(program, args...); err != nil {
		t.Fatal(err)
	}
}

func (p *restorePostgres) stop() error {
	if err := p.command("pg_ctl", "-D", p.data, "-m", "fast", "-w", "stop"); err != nil {
		return err
	}
	p.running = false
	return nil
}

func (p *restorePostgres) backup(t *testing.T, schema, path string) string {
	t.Helper()
	p.must(t, "pg_dump", "--format=custom", "--no-owner", "--dbname="+p.dsn, "--schema="+schema, "--file="+path)
	digest, err := restoreFileDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func restoreFileDigest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 8<<20 {
		return "", errors.New("local restore artifact is missing or exceeds fixture bounds")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func (p *restorePostgres) restore(path, expected string) error {
	if digest, err := restoreFileDigest(path); err != nil || digest != expected {
		return errors.New("local backup failed its pinned digest check")
	}
	return p.command("pg_restore", "--single-transaction", "--exit-on-error", "--no-owner", "--dbname="+p.dsn, path)
}

func restoreFixtureRoles(t *testing.T, source, target *sql.DB) {
	t.Helper()
	rows, err := source.Query(`SELECT rolname FROM pg_roles
		WHERE rolname ~ '^ai_fixture_(rt|review|support)_[0-9a-f]{16}$'
		AND rolcanlogin AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole
		AND NOT rolinherit AND NOT rolreplication AND NOT rolbypassrls ORDER BY rolname`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			t.Fatal(err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil || len(roles) != 3 {
		t.Fatal("unexpected restricted-role inventory in private backup", len(roles), err)
	}
	for _, role := range roles {
		// Local Unix-socket identities only. Reject broader/global roles and
		// never export their credentials or trust configuration.
		if _, err := target.Exec("CREATE ROLE " + pgx.Identifier{role}.Sanitize() +
			" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD NULL"); err != nil {
			t.Fatal(err)
		}
	}
}

func restoreSQL(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func restoreWrite(t *testing.T, path string, contents []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
}
