package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"professional-information-repo/internal/accountstore"
)

func main() {
	schemaID := flag.Bool("schema-id", false, "print the embedded Account schema identity")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected hosted migration arguments")
	}
	if *schemaID {
		_, _ = os.Stdout.WriteString(accountstore.HostedPilotSchemaID() + "\n")
		return
	}
	if err := migrate(); err != nil {
		log.Fatal("hosted Account migration refused or failed")
	}
}

func migrate() error {
	process, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(process, 30*time.Second)
	defer cancel()
	return accountstore.MigrateHostedPilot(ctx, os.Getenv("CAREEROS_HOSTED_MIGRATOR_DATABASE_URL"))
}
