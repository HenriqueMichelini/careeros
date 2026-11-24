package pilotruntime

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

type HostedConfig struct {
	Port           string
	Assets         fs.FS
	DatabaseURL    string
	GoogleSubjects string
	Auth           auth.Config
}

// NewHostedServer is the closed HTTPS-origin candidate behind Cloud Run TLS.
// Its process has no invitation, grant issuance, or AI dispatch capability.
func NewHostedServer(ctx context.Context, cfg HostedConfig) (*http.Server, func(), error) {
	authority, listen, err := hostedAddress(cfg.Auth.PublicOrigin, cfg.Port)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Assets == nil {
		return nil, nil, errors.New("hosted pilot frontend is required")
	}
	startup, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	store, err := accountstore.OpenHostedPilot(startup, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, errors.New("hosted pilot database unavailable")
	}
	closeStore := func() { _ = store.Close() }
	service, err := auth.NewHostedGoogleService(cfg.Auth, store.AuthStores(), cfg.GoogleSubjects)
	if err != nil {
		closeStore()
		return nil, nil, errors.New("hosted Google authentication configuration invalid")
	}
	if err := store.HostedPilotClosed(startup); err != nil {
		closeStore()
		return nil, nil, errors.New("hosted pilot closed authority unavailable")
	}
	pilot := accountapi.NewPilot(service, store, nil)
	closed := service.Protect(auth.UnavailableHandler())
	site, err := newSiteAtOrigin(authority, cfg.Auth.PublicOrigin, cfg.Assets, closedHostedRoutes(pilot, closed), store.HostedPilotClosed)
	if err != nil {
		closeStore()
		return nil, nil, err
	}
	return &http.Server{
		Addr: listen,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Referrer-Policy", "no-referrer")
			site.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: readHeaderLimit,
		ReadTimeout:       readLimit,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       idleLimit,
		MaxHeaderBytes:    maxHeaderBytes,
	}, closeStore, nil
}

func closedHostedRoutes(pilot, closed http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/api/pilot/activate" || p == "/api/draft-allowance" ||
			inNamespace(p, "/api/drafts") || inNamespace(p, "/api/ai") {
			closed.ServeHTTP(w, r)
			return
		}
		pilot.ServeHTTP(w, r)
	})
}

func hostedAddress(origin, port string) (string, string, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" ||
		u.Host != u.Hostname() || u.Host != strings.ToLower(u.Host) ||
		!strings.Contains(u.Host, ".") || strings.HasSuffix(u.Host, ".") ||
		net.ParseIP(u.Host) != nil || origin != "https://"+u.Host {
		return "", "", errors.New("hosted pilot requires one canonical HTTPS origin")
	}
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 || strconv.Itoa(n) != port {
		return "", "", errors.New("hosted pilot requires a valid PORT")
	}
	return u.Host, net.JoinHostPort("0.0.0.0", port), nil
}
