package aidispatch_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/fieldvalidation"
	"professional-information-repo/internal/openaihttp"
)

type controllerFunc func(context.Context, string, *http.Request) (*http.Response, error)

func (f controllerFunc) Send(ctx context.Context, provider string, request *http.Request) (*http.Response, error) {
	return f(ctx, provider, request)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func response(status int) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}
}

func request(t *testing.T) *http.Request {
	t.Helper()
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader([]byte(`{"model":"synthetic","max_completion_tokens":1}`)))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer sk-synthetic-credential")
	r.Header.Set("X-Untrusted", "never forwarded")
	return r
}

func forbiddenClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("controlled path reached BYOK transport")
		return nil, errors.New("forbidden transport")
	})}
}

func TestManagedMissingOrDenyingControllerNeverFallsBack(t *testing.T) {
	denied := errors.New("synthetic denial")
	for _, ctx := range []context.Context{
		aidispatch.RequireManaged(context.Background()),
		aidispatch.WithController(context.Background(), nil),
		aidispatch.WithController(context.Background(), controllerFunc(func(context.Context, string, *http.Request) (*http.Response, error) { return nil, denied })),
	} {
		got, err := aidispatch.Send(ctx, forbiddenClient(t), "openai", request(t))
		if got != nil || err == nil {
			t.Fatal("missing/denying control admitted a send")
		}
	}
}

func TestA10_ControlledSendIsOneInvocationWithoutRedirectRetryOrCredential(t *testing.T) {
	for _, status := range []int{307, 308, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			ctx, finish := aidiagnostics.Start(context.Background(), "generation", "fixture-v1")
			defer finish("")
			ctx = aidispatch.WithCoordinate(ctx, "fixture/one")
			ctx = aidispatch.WithController(ctx, controllerFunc(func(ctx context.Context, provider string, r *http.Request) (*http.Response, error) {
				calls++
				if provider != "openai" || aidiagnostics.StageName(ctx) != "generation" || aidispatch.Coordinate(ctx) != "fixture/one" {
					t.Fatal("lost provider/stage binding")
				}
				if len(r.Header) != 1 || r.Header.Get("Content-Type") != "application/json" {
					t.Fatal("arbitrary headers or credentials reached controller")
				}
				resp := response(status)
				resp.Header.Set("Location", "https://unapproved.invalid/")
				return resp, nil
			}))
			resp, err := aidispatch.Send(ctx, forbiddenClient(t), "openai", request(t))
			if err != nil || resp.StatusCode != status || calls != 1 {
				t.Fatal("response was retried or followed")
			}
			resp.Body.Close()
		})
	}
}

func TestControlledRequestAllowlist(t *testing.T) {
	ctx := aidispatch.WithController(context.Background(), controllerFunc(func(context.Context, string, *http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached controller")
		return nil, nil
	}))
	for _, target := range []string{"http://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/chat/completions?extra=1", "https://api.openai.com/other", "https://user@api.openai.com/v1/chat/completions"} {
		r := request(t)
		r.URL, _ = r.URL.Parse(target)
		if _, err := aidispatch.Send(ctx, forbiddenClient(t), "openai", r); !errors.Is(err, aidispatch.ErrRequest) {
			t.Fatal("request allowlist did not deny")
		}
	}
	if _, err := aidispatch.Send(ctx, forbiddenClient(t), "other-provider", request(t)); !errors.Is(err, aidispatch.ErrRequest) {
		t.Fatal("unknown provider accepted")
	}
}

func TestExplicitBYOKClientIsPreservedOutsideManagedContext(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer sk-synthetic-credential" {
			t.Fatal("explicit BYOK behavior changed")
		}
		return response(200), nil
	})}
	resp, err := aidispatch.Send(context.Background(), client, "openai", request(t))
	if err != nil || resp.StatusCode != 200 || calls != 1 {
		t.Fatal("explicit client not used once")
	}
	resp.Body.Close()
}

func TestBothRealAdaptersUseControlledBoundary(t *testing.T) {
	var providers []string
	ctx := aidispatch.WithController(context.Background(), controllerFunc(func(ctx context.Context, provider string, r *http.Request) (*http.Response, error) {
		providers = append(providers, provider)
		if _, ok := ctx.Deadline(); !ok || r.Header.Get("Authorization") != "" {
			t.Fatal("adapter lost deadline or exposed credential")
		}
		return response(429), nil
	}))
	_, cancel, resp, err := openaihttp.Post(ctx, forbiddenClient(t), time.Second, "sk-synthetic", []byte(`{"max_completion_tokens":1}`))
	defer cancel()
	if err != nil || resp.StatusCode != 429 {
		t.Fatal("OpenAI did not traverse controlled path")
	}
	resp.Body.Close()
	decision := fieldvalidation.Classify(ctx, forbiddenClient(t), "synthetic-typesafe", fieldvalidation.ProfessionalInformation, "I use Java.")
	if decision.Outcome.Kind != "service_failure" || decision.Outcome.Reason != "rate_limit" || strings.Join(providers, ",") != "openai,typesafe" {
		t.Fatal("TypeSafe did not traverse controlled path exactly once")
	}
}
