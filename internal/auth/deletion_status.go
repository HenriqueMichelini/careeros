package auth

import (
	"context"
	"net/http"
	"time"
)

// ClosedStatusSessionStore is a separate capability: its records are never
// returned by the ordinary SessionStore lookup. Only a fresh completed provider
// sign-in can create one, for an already closed account with a deletion record.
type ClosedStatusSessionStore interface {
	CreateClosedStatus(context.Context, Session) error
	FindClosedStatusByTokenHash(context.Context, [32]byte, time.Time) (Session, bool, error)
	RevokeClosedStatus(context.Context, string) error
}

func (s *Service) AuthenticateDeletionStatus(ctx context.Context, token string) (Principal, error) {
	if p, err := s.Authenticate(ctx, token); err == nil {
		return p, nil
	}
	if len(token) == 0 || len(token) > 128 {
		return Principal{}, ErrAuthenticationRequired
	}
	sessions, ok := s.sessions.(ClosedStatusSessionStore)
	if !ok {
		return Principal{}, ErrAuthenticationRequired
	}
	v, found, err := sessions.FindClosedStatusByTokenHash(ctx, hashSecret(token), s.now().UTC())
	if err != nil {
		return Principal{}, err
	}
	if !found {
		return Principal{}, ErrAuthenticationRequired
	}
	account, found, err := s.accounts.Get(ctx, v.AccountID)
	if err != nil {
		return Principal{}, err
	}
	if !found || (account.Status != AccountDeleted && account.Status != AccountCompromised) || account.Generation != v.Generation {
		return Principal{}, ErrAuthenticationRequired
	}
	return Principal{AccountID: account.ID, SessionID: v.ID, Generation: v.Generation, ExpiresAt: v.ExpiresAt, tokenHash: v.TokenHash, csrfHash: v.CSRFHash}, nil
}

// ProtectRestrictedRights is only for the separately scoped compromised-account
// rights process. It never authenticates a workspace/read/export/inference path.
func (s *Service) ProtectRestrictedRights(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if unsafeMethod(r.Method) && !s.SameOrigin(r) {
			writeAuthError(w, ErrForbidden)
			return
		}
		token, ok := sessionTokenFromRequest(r)
		if !ok {
			writeAuthError(w, ErrAuthenticationRequired)
			return
		}
		p, err := s.AuthenticateDeletionStatus(r.Context(), token)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		account, found, err := s.accounts.Get(r.Context(), p.AccountID)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		if !found || account.Status != AccountCompromised && !(account.Status == AccountDeleted && r.Method == http.MethodGet) {
			writeAuthError(w, ErrForbidden)
			return
		}
		if unsafeMethod(r.Method) && !validRequestCSRF(s, r, p) {
			writeAuthError(w, ErrForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, p)))
	})
}

// ProtectDeletionStatus permits only metadata GETs. It is never the middleware
// for a workspace route, mutation, grant, cleanup or ordinary career read.
func (s *Service) ProtectDeletionStatus(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			writeAuthError(w, ErrForbidden)
			return
		}
		token, ok := sessionTokenFromRequest(r)
		if !ok {
			writeAuthError(w, ErrAuthenticationRequired)
			return
		}
		p, err := s.AuthenticateDeletionStatus(r.Context(), token)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, p)))
	})
}
