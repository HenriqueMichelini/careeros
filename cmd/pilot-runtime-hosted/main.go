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
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/pilotruntime"
)

func main() {
	assets := flag.String("assets", "/app/dist", "packaged frontend directory")
	schemaID := flag.Bool("schema-id", false, "print the embedded Account schema identity")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected hosted runtime arguments")
	}
	if *schemaID {
		_, _ = os.Stdout.WriteString(accountstore.HostedPilotSchemaID() + "\n")
		return
	}
	if err := run(*assets); err != nil {
		log.Fatal(err)
	}
}

func run(assets string) error {
	config, err := auth.ConfigFromEnv()
	if err != nil {
		return errors.New("hosted pilot authentication configuration invalid")
	}
	root, err := os.OpenRoot(assets)
	if err != nil {
		return errors.New("hosted pilot frontend unavailable")
	}
	defer root.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server, closeStore, err := pilotruntime.NewHostedServer(ctx, pilotruntime.HostedConfig{
		Port:           os.Getenv("PORT"),
		Assets:         root.FS(),
		DatabaseURL:    os.Getenv("CAREEROS_DATABASE_URL"),
		GoogleSubjects: os.Getenv("CAREEROS_HOSTED_GOOGLE_SUBJECTS"),
		Auth:           config,
	})
	if err != nil {
		return err
	}
	defer closeStore()

	serving := make(chan error, 1)
	go func() { serving <- server.ListenAndServe() }()
	select {
	case err := <-serving:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("hosted pilot listener unavailable")
		}
	case <-ctx.Done():
		// Leave time to exit within Cloud Run's platform grace period.
		shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
		<-serving
	}
	return nil
}
