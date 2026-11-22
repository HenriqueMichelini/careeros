// lifecycle-local runs one bounded synthetic batch, never a hosted scheduler or
// recipient transport. Real email and provider access remain unavailable.
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
	limit := flag.Int("limit", 10, "bounded account batch, 1 to 100")
	flag.Parse()
	dsn := os.Getenv("CAREEROS_LIFECYCLE_LOCAL_DATABASE_URL")
	u, err := url.Parse(dsn)
	if !*synthetic || err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || !strings.HasSuffix(u.Path, "_test") || *limit < 1 || *limit > 100 {
		return fmt.Errorf("requires --synthetic-only and disposable loopback *_test PostgreSQL")
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
	worker, err := accountstore.NewLifecycleWorker(store, nil)
	if err != nil {
		return err
	}
	out, err := worker.Run(ctx, *limit)
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
