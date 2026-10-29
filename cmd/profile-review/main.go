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

// newHandler composes shared account/Profile authority and protected AI workflows.
func newHandler() http.Handler {
	mux := http.NewServeMux()
	runtime, _ := accountapi.FromEnv(context.Background())
	var accountHandler http.Handler = accountapi.New(nil, nil)
	if runtime != nil {
		accountHandler = runtime.Handler
	}

	protect := func(path string, next http.Handler) http.Handler {
		if runtime == nil {
			return accountapi.ProtectWorkflow(nil, nil, path, next)
		}
		return accountapi.ProtectWorkflow(runtime.Service, runtime.Store, path, next)
	}
	mux.Handle("/api/cv/generate", protect("/api/cv/generate", cvgeneration.NewHandler()))
	mux.Handle("/api/profile/ingest", protect("/api/profile/ingest", profileingestion.NewHandler()))
	mux.Handle("/api/application-draft", protect("/api/application-draft", applicationdraft.NewHandler()))
	mux.Handle("/api/qualification-gaps", protect("/api/qualification-gaps", qualificationgaps.NewHandler()))
	mux.Handle("/api/profile/review", protect("/api/profile/review", profilereview.NewHandler()))
	mux.Handle("/api/auth/", accountHandler)
	mux.Handle("/api/profile/current", accountHandler)
	mux.Handle("/api/profile/operations/", accountHandler)
	return mux
}
