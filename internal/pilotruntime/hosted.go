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
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/draftrecovery"
)

type HostedConfig struct {
	Port           string
	Assets         fs.FS
	DatabaseURL    string
	GoogleSubjects string
	Auth           auth.Config
	Draft          *HostedDraftConfig
}

type HostedDraftConfig struct {
	LedgerURL   string
	ActionKey   [32]byte
	ArtifactKey [32]byte
	Respond     func(context.Context, aicontrol.HostedDraftCall) (*http.Response, error)
}

// NewHostedServer serves the closed HTTPS-origin candidate behind Cloud Run
// TLS. An explicit configuration can enable only the enrolled fake Draft.
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
	pilot := accountapi.NewPilot(service, store, nil)
	closed := service.Protect(auth.UnavailableHandler())
	handler := closedHostedRoutes(pilot, closed)
	ready := func(ctx context.Context) (string, error) {
		return "closed", store.HostedPilotClosed(ctx)
	}
	if cfg.Draft != nil {
		draft, check, closeDraft, err := openHostedDraft(startup, cfg.DatabaseURL, *cfg.Draft, service, store)
		if err != nil {
			ready = func(ctx context.Context) (string, error) {
				_, _, err := store.HostedPilotInvitationClosed(ctx)
				return scopedDraftUnavailable, err
			}
		} else {
			closeStore = func() {
				closeDraft()
				_ = store.Close()
			}
			ready = check
			handler = hostedDraftRoutes(handler, draft, closed)
		}
	}
	if _, err := ready(startup); err != nil {
		closeStore()
		return nil, nil, errors.New("hosted pilot authority unavailable")
	}
	site, err := newSiteWithReadiness(authority, cfg.Auth.PublicOrigin, cfg.Assets, handler, ready)
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

func openHostedDraft(ctx context.Context, accountURL string, cfg HostedDraftConfig, service *auth.Service, account *accountstore.Store) (
	http.Handler, func(context.Context) (string, error), func(), error,
) {
	if cfg.ArtifactKey == ([32]byte{}) || cfg.ArtifactKey == cfg.ActionKey {
		return nil, nil, nil, auth.ErrUnavailable
	}
	actionDB, err := accountstore.OpenHostedDraftLedger(ctx, accountURL, cfg.LedgerURL)
	if err != nil {
		return nil, nil, nil, err
	}
	action, err := aicontrol.OpenHostedDraft(ctx, actionDB, aicontrol.HostedDraftConfig{BindingKey: cfg.ActionKey, Respond: cfg.Respond})
	if err != nil {
		_ = actionDB.Close()
		return nil, nil, nil, err
	}
	artifactDB, err := accountstore.OpenHostedDraftLedger(ctx, accountURL, cfg.LedgerURL)
	if err != nil {
		_ = action.Close()
		return nil, nil, nil, err
	}
	drafts, err := draftrecovery.OpenHostedDraft(ctx, artifactDB, action, cfg.ArtifactKey[:])
	if err != nil {
		_ = artifactDB.Close()
		_ = action.Close()
		return nil, nil, nil, err
	}
	ready := func(ctx context.Context) (string, error) {
		stopped, available, err := account.HostedPilotInvitationClosed(ctx)
		if err != nil {
			return "", err
		}
		if stopped {
			return "closed", nil
		}
		if !available || action.HostedDraftReady(ctx) != nil || drafts.HostedDraftReady(ctx) != nil {
			return scopedDraftUnavailable, nil
		}
		return "scoped-fake", nil
	}
	closeDraft := func() {
		_ = drafts.Close()
		_ = action.Close()
	}
	return accountapi.NewHostedDraftOnly(service, action, drafts), ready, closeDraft, nil
}

func hostedDraftRoutes(account, draft, unavailable http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/api/application-draft" || path == "/api/draft-allowance" || inNamespace(path, "/api/drafts") {
			if r.Method == http.MethodPost && (path == "/api/application-draft" || path == "/api/drafts") &&
				r.Context().Value(siteInferenceKey{}) != "scoped-fake" {
				unavailable.ServeHTTP(w, r)
				return
			}
			draft.ServeHTTP(w, r)
			return
		}
		account.ServeHTTP(w, r)
	})
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
