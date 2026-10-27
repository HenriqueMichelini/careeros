// account-migrate applies the reviewed additive schema to an explicitly supplied
// database. It provisions no service and is not invoked by application startup.
package main

import (
	"context"
	"log"
	"os"
	"professional-information-repo/internal/accountstore"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := accountstore.Open(ctx, os.Getenv("CAREEROS_DATABASE_URL"))
	if err != nil {
		log.Fatal("account storage unavailable")
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		log.Fatal("account schema migration failed")
	}
	log.Print("account schema v1 applied")
}
