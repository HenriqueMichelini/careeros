package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/pilotruntime"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8787", "literal loopback address and port")
	static := flag.String("static", "frontend", "packaged frontend directory")
	schemaID := flag.Bool("schema-id", false, "print the embedded account schema identity")
	flag.Parse()
	if *schemaID {
		_, _ = os.Stdout.WriteString(accountstore.PilotPackageSchemaID() + "\n")
		return
	}
	root, err := os.OpenRoot(*static)
	if err != nil {
		log.Fatal("pilot frontend unavailable")
	}
	defer root.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server, closeStore, err := pilotruntime.NewServer(ctx, pilotruntime.Config{
		Listen: *listen, Assets: root.FS(), DatabaseURL: os.Getenv("CAREEROS_PILOT_LOCAL_DATABASE_URL"),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer closeStore()
	done := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if server.Shutdown(shutdown) != nil {
				_ = server.Close()
			}
		case <-done:
		}
	}()
	log.Printf("closed local pilot listening on %s", server.Addr)
	err = server.ListenAndServe()
	close(done)
	<-drained
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("pilot listener unavailable")
	}
}
