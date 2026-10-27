package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	applicationdraft "professional-information-repo/backend/application-draft"
	cvgeneration "professional-information-repo/backend/cv-generation"
	profileingestion "professional-information-repo/backend/profile-ingestion"
	profilereview "professional-information-repo/backend/profile-review"
	qualificationgaps "professional-information-repo/backend/qualification-gaps"
	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/auth"
)

func main() {
	port := os.Getenv("PROFILE_REVIEW_PORT")
	if port == "" {
		port = "8787"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           newHandler(),
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      55 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("profile-review listening port=%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// newHandler composes shared account/Profile authority and legacy AI workflows.
func newHandler() http.Handler {
	mux := http.NewServeMux()
	runtime, _ := accountapi.FromEnv(context.Background())
	var service *auth.Service
	var accountHandler http.Handler = accountapi.New(nil, nil)
	if runtime != nil {
		service = runtime.Service
		accountHandler = runtime.Handler
	}

	mux.Handle("/api/cv/generate", protect(cvgeneration.NewHandler(), service))
	mux.Handle("/api/profile/ingest", protect(profileingestion.NewHandler(), service))
	mux.Handle("/api/application-draft", protect(applicationdraft.NewHandler(), service))
	mux.Handle("/api/qualification-gaps", protect(qualificationgaps.NewHandler(), service))
	mux.Handle("/api/profile/review", protect(profilereview.NewHandler(), service))
	mux.Handle("/api/auth/", accountHandler)
	mux.Handle("/api/profile/current", accountHandler)
	mux.Handle("/api/profile/operations/", accountHandler)
	return mux
}

// protect keeps the legacy guest/BYOK behavior unless an account-owned
// deployment explicitly sets CAREEROS_REQUIRE_AUTH=1. When enabled, missing
// configuration fails closed instead of silently serving guest traffic.
func protect(next http.Handler, service *auth.Service) http.Handler {
	if os.Getenv("CAREEROS_REQUIRE_AUTH") != "1" {
		return next
	}
	if service == nil {
		return auth.UnavailableHandler()
	}
	return service.Protect(next)
}
