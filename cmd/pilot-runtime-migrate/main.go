// pilot-runtime-migrate runs the existing account migration on the closed
// package's exact disposable PostgreSQL fixture.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"professional-information-repo/internal/accountstore"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := accountstore.MigrateClosedPilotPackage(ctx, os.Getenv("CAREEROS_PILOT_LOCAL_MIGRATOR_DATABASE_URL")); err != nil {
		log.Fatal("closed pilot migration requires its dedicated local database and migrator")
	}
	log.Print("closed pilot account schema applied")
}
