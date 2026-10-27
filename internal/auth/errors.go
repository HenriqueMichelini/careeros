// Package auth implements CareerOS's owner-selected third-party sign-in policy:
// Google-only OpenID Connect, application-owned opaque sessions and a
// deny-by-default ownership guard.
//
// It deliberately contains no password, account-linking or lost-access
// recovery flow. A provider assertion establishes a provider subject, never
// CareerOS ownership by itself; the service maps the provider subject to an
// account and issues its own session.
package auth

import "errors"

var (
	// ErrUnavailable means the auth service is not configured or cannot reach
	// a dependency. Callers must fail closed.
	ErrUnavailable = errors.New("auth unavailable")
	// ErrProviderNotAllowed means the requested provider is not on the
	// owner-selected allowlist.
	ErrProviderNotAllowed = errors.New("provider not allowed")
	// ErrInvalidTransaction covers missing, expired, replayed, mismatched or
	// tampered sign-in transactions.
	ErrInvalidTransaction = errors.New("invalid or expired sign-in transaction")
	// ErrInvalidCallback covers provider responses that cannot be trusted.
	ErrInvalidCallback = errors.New("invalid sign-in callback")
	// ErrAuthenticationRequired means there is no usable session.
	ErrAuthenticationRequired = errors.New("authentication required")
	// ErrForbidden means an authenticated account does not own the resource or
	// is no longer allowed to act.
	ErrForbidden = errors.New("forbidden")
	// ErrCapacity means a bounded store refused more entries.
	ErrCapacity = errors.New("auth store capacity reached")
)
