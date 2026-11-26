package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/hosteddraft"
	"professional-information-repo/internal/pilotruntime"
)

func main() {
	if len(os.Args) != 1 || run() != nil {
		log.Fatal("hosted Draft attestor unavailable")
	}
}

func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil || number == 0 || strconv.FormatUint(number, 10) != port {
		return errors.New("invalid attestor port")
	}
	token, err := pilotruntime.HostedDraftSecret(os.Getenv("CAREEROS_HOSTED_DRAFT_ATTESTOR_TOKEN"))
	if err != nil {
		return err
	}
	process, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(process, 5*time.Second)
	defer cancel()
	db, err := accountstore.OpenHostedDraftAttestor(startup, os.Getenv("CAREEROS_HOSTED_DRAFT_ATTESTOR_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	handler, err := hosteddraft.NewAttestorHandler(db, token)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              net.JoinHostPort("0.0.0.0", port),
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       10 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-process.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
		<-done
	}
	return nil
}
