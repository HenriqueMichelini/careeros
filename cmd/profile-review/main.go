package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	applicationdraft "professional-information-repo/backend/application-draft"
	profilereview "professional-information-repo/backend/profile-review"
	qualificationgaps "professional-information-repo/backend/qualification-gaps"
	profileingestion "professional-information-repo/backend/profile-ingestion"
)

func main() {
	port := os.Getenv("PROFILE_REVIEW_PORT")
	if port == "" {
		port = "8787"
	}
	server := &http.Server{
		Addr: ":" + port,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/profile/ingest" {
				profileingestion.NewHandler().ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/api/application-draft" {
				applicationdraft.NewHandler().ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/api/qualification-gaps" {
				qualificationgaps.NewHandler().ServeHTTP(w, r)
				return
			}
			profilereview.NewHandler().ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      55 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("profile-review listening port=%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
