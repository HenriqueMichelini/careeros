package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	profilereview "professional-information-repo/backend/profile-review"
)

func main() {
	port := os.Getenv("PROFILE_REVIEW_PORT")
	if port == "" {
		port = "8787"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           profilereview.NewHandler(),
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("profile-review listening port=%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
