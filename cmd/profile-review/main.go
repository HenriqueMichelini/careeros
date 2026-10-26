package main

import (
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

// newHandler composes one process-local auth service with the existing
// workflows. The in-memory store is a reference for local/single-process use;
// a shared persistence backend remains unselected, so the Netlify functions
// are not wired to this guard yet.
func newHandler() http.Handler {
	mux := http.NewServeMux()
	service := configuredAuthService()

	mux.Handle("/api/cv/generate", protect(cvgeneration.NewHandler(), service))
	mux.Handle("/api/profile/ingest", protect(profileingestion.NewHandler(), service))
	mux.Handle("/api/application-draft", protect(applicationdraft.NewHandler(), service))
	mux.Handle("/api/qualification-gaps", protect(qualificationgaps.NewHandler(), service))
	mux.Handle("/api/profile/review", protect(profilereview.NewHandler(), service))
	mux.Handle("/api/auth/", auth.NewHandler(service))
	return mux
}

func configuredAuthService() *auth.Service {
	config, err := auth.ConfigFromEnv()
	if err != nil {
		log.Printf("auth unavailable: %v", err)
		return nil
	}
	providerConfig := config.Providers[0]
	provider, err := auth.NewGoogleProvider(providerConfig, config.HTTPClient, config.Now)
	if err != nil {
		log.Printf("auth unavailable: %v", err)
		return nil
	}
	registry, err := auth.NewRegistry(provider)
	if err != nil {
		log.Printf("auth unavailable: %v", err)
		return nil
	}
	service, err := auth.NewService(config, registry, auth.NewMemoryStores())
	if err != nil {
		log.Printf("auth unavailable: %v", err)
		return nil
	}
	return service
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
