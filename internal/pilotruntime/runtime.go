package pilotruntime

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/netip"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
)

const (
	readHeaderLimit = 3 * time.Second
	readLimit       = 10 * time.Second
	writeLimit      = 15 * time.Second
	idleLimit       = 30 * time.Second
	maxHeaderBytes  = 16 << 10
)

type Config struct {
	Listen      string
	DatabaseURL string
	Assets      fs.FS
}

// NewServer builds a local rehearsal server. Its origin is derived from its
// sole literal loopback listener and cannot be supplied by forwarding headers.
func NewServer(ctx context.Context, cfg Config) (*http.Server, func(), error) {
	address, err := netip.ParseAddrPort(cfg.Listen)
	if err != nil || !address.Addr().IsLoopback() || address.Addr().Zone() != "" || address.Port() == 0 ||
		address.String() != cfg.Listen {
		return nil, nil, errors.New("pilot listener must be a canonical loopback address and port")
	}
	if cfg.Assets == nil {
		return nil, nil, errors.New("pilot frontend is required")
	}

	account, ready, closeStore := closedAccount(ctx, "http://"+cfg.Listen, cfg.DatabaseURL)
	handler, err := newSite(cfg.Listen, cfg.Assets, account, ready)
	if err != nil {
		closeStore()
		return nil, nil, err
	}
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderLimit,
		ReadTimeout:       readLimit,
		WriteTimeout:      writeLimit,
		IdleTimeout:       idleLimit,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	return server, closeStore, nil
}

func closedAccount(ctx context.Context, origin, databaseURL string) (http.Handler, func(context.Context) error, func()) {
	start, cancel := context.WithTimeout(ctx, readinessLimit)
	defer cancel()
	store, err := accountstore.OpenClosedPilotPackage(start, databaseURL)
	if err != nil {
		return nil, nil, func() {}
	}
	closeStore := func() { _ = store.Close() }
	provider := auth.ProviderConfig{
		ID: "google", ClientID: "closed-local", ClientSecret: "closed-local",
		RedirectURI: origin + "/api/auth/google/callback", Issuers: []string{origin},
		AuthorizationEndpoint: origin + "/api/auth/unconfigured",
		TokenEndpoint:         origin + "/api/auth/unconfigured",
		JWKSURL:               origin + "/api/auth/unconfigured",
		Scopes:                []string{"openid"},
	}
	registry, err := auth.NewRegistry(closedIdentity{})
	if err != nil {
		closeStore()
		return nil, nil, func() {}
	}
	config := auth.Config{
		PublicOrigin: origin, SessionTTL: 12 * time.Hour, TransactionTTL: 10 * time.Minute,
		Providers: []auth.ProviderConfig{provider},
	}
	service, err := auth.NewService(config, registry, store.AuthStores())
	if err != nil {
		closeStore()
		return nil, nil, func() {}
	}
	pilot := accountapi.NewPilot(service, store, nil)
	unavailable := auth.UnavailableHandler()
	guarded := service.Protect(unavailable)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/api/auth/google/start" || p == "/api/auth/google/callback":
			unavailable.ServeHTTP(w, r)
		case p == "/api/pilot/activate", inNamespace(p, "/api/ai"), inNamespace(p, "/api/drafts"):
			guarded.ServeHTTP(w, r)
		case !safeMethod(r.Method) && p != "/api/auth/logout" && p != "/api/auth/logout-all":
			// No profile, invitation, deletion or managed mutation is mounted
			// in the unactivated preparation package. Account redirects and
			// method changes cannot reach the development caller-key handlers.
			guarded.ServeHTTP(w, r)
		default:
			pilot.ServeHTTP(w, r)
		}
	})
	return handler, store.ClosedPilotPackageReady, closeStore
}

// closedIdentity has no HTTP client, issuer discovery, or exchange path.
type closedIdentity struct{}

func (closedIdentity) ID() string { return "google" }

func (closedIdentity) AuthorizationURL(_, _, _, _ string) string { return "" }

func (closedIdentity) Exchange(context.Context, string, string, string, string) (auth.Identity, error) {
	return auth.Identity{}, auth.ErrUnavailable
}
