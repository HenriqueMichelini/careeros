// deletion-local removes only already-fenced synthetic content in a disposable
// local database. It cannot start deletion, grant rights or contact a provider.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"professional-information-repo/internal/accountstore"
	"strings"
	"time"
)

func run() error {
	synthetic := flag.Bool("synthetic-only", false, "required: disposable local database only")
	owner := flag.String("account-id", "", "opaque owner of an existing deletion operation")
	operation := flag.String("operation-id", "", "existing fenced deletion operation")
	flag.Parse()
	dsn := os.Getenv("CAREEROS_DELETION_LOCAL_DATABASE_URL")
	u, err := url.Parse(dsn)
	if !*synthetic || err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || !strings.HasSuffix(u.Path, "_test") || *owner == "" || *operation == "" {
		return fmt.Errorf("requires --synthetic-only, an existing operation and disposable loopback *_test PostgreSQL")
	}
	for key := range u.Query() {
		if key != "sslmode" && key != "search_path" {
			return fmt.Errorf("unsupported local DSN option")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := accountstore.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	out, err := store.PurgeDeletion(ctx, *owner, *operation)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
