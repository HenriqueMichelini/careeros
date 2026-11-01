// Package aidispatch is the shared send boundary for CareerOS AI adapters.
// A controller can only be installed by Go composition, never by request data.
package aidispatch

import (
	"context"
	"errors"
	"net/http"
)

// Controller authorizes and invokes a single provider request. The local
// synthetic controller uses an in-memory responder and never the BYOK client.
// Implementations must not retry or follow redirects. No provider credential is
// passed to this interface.
type Controller interface {
	Send(context.Context, string, *http.Request) (*http.Response, error)
}

type contextKey uint8

const (
	controllerKey contextKey = iota
	managedKey
	coordinateKey
	stageInputKey
	workflowRootKey
)

var ErrUnavailable = errors.New("managed AI dispatch unavailable")
var ErrRequest = errors.New("invalid AI dispatch request")

// WithController selects the code-installed controlled path. There is no
// fallback to a supplied client, including when the controller denies a send.
func WithController(ctx context.Context, controller Controller) context.Context {
	ctx = RequireManaged(ctx)
	if controller == nil {
		return ctx
	}
	return context.WithValue(ctx, controllerKey, controller)
}

// RequireManaged makes the absence of a controller fail closed. It is an
// internal composition option, not an environment variable or HTTP setting.
func RequireManaged(ctx context.Context) context.Context {
	return context.WithValue(ctx, managedKey, true)
}

// WithCoordinate carries the server-owned workflow/protocol stage coordinate.
// It is restricted binding data and must never be added to ordinary diagnostics.
// The controller still validates it and binds the exact effective request.
func WithCoordinate(ctx context.Context, coordinate string) context.Context {
	return context.WithValue(ctx, coordinateKey, coordinate)
}

func Coordinate(ctx context.Context) string {
	coordinate, _ := ctx.Value(coordinateKey).(string)
	return coordinate
}

// WithStageInput binds a validated protocol envelope, including authenticated
// handoffs and child inputs, before its first provider leg. The content is
// request-local only: the controller persists only its keyed fingerprint.
func WithStageInput(ctx context.Context, input []byte) context.Context {
	return context.WithValue(ctx, stageInputKey, append([]byte(nil), input...))
}

func StageInput(ctx context.Context) []byte {
	input, _ := ctx.Value(stageInputKey).([]byte)
	return append([]byte(nil), input...)
}

type workflowRoot struct {
	workflow string
	input    []byte
}

// WithWorkflowRoot is installed at the validated workflow boundary. The
// controller must compare it with the admitted immutable Action before sending.
// Provider credentials are headers and never part of this root envelope.
func WithWorkflowRoot(ctx context.Context, workflow string, rootJSON []byte) context.Context {
	return context.WithValue(ctx, workflowRootKey, workflowRoot{workflow, append([]byte(nil), rootJSON...)})
}

func WorkflowRoot(ctx context.Context) (string, []byte) {
	root, _ := ctx.Value(workflowRootKey).(workflowRoot)
	return root.workflow, append([]byte(nil), root.input...)
}

// Send preserves the explicit caller-key client when no managed context is
// present. Controlled requests bypass that client completely: hidden transport
// retries, redirects, cookies and network fallback cannot occur on this path.
func Send(ctx context.Context, client *http.Client, provider string, request *http.Request) (*http.Response, error) {
	controller, _ := ctx.Value(controllerKey).(Controller)
	managed, _ := ctx.Value(managedKey).(bool)
	if !managed {
		if client == nil {
			return nil, ErrUnavailable
		}
		return client.Do(request)
	}
	if controller == nil {
		return nil, ErrUnavailable
	}
	if !validRequest(provider, request) {
		return nil, ErrRequest
	}
	controlled := request.Clone(ctx)
	// Credential values and arbitrary caller headers have no role in synthetic
	// admission, request binding or the fake responder. Keep only the media type.
	controlled.Header = http.Header{"Content-Type": {"application/json"}}
	response, err := controller.Send(ctx, provider, controlled)
	if err == nil && (response == nil || response.Body == nil) {
		return nil, ErrUnavailable
	}
	return response, err
}

func validRequest(provider string, request *http.Request) bool {
	if request == nil || request.Method != http.MethodPost || request.URL == nil || request.URL.User != nil || request.URL.RawQuery != "" || request.URL.Fragment != "" || request.GetBody == nil || request.RequestURI != "" || request.Host != request.URL.Host {
		return false
	}
	switch provider {
	case "openai":
		return request.URL.String() == "https://api.openai.com/v1/chat/completions"
	case "typesafe":
		return request.URL.String() == "https://api.typesafe.ai/v1/systemone"
	default:
		return false
	}
}
