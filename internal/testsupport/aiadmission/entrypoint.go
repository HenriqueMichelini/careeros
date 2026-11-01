package aiadmission

import (
	"context"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidispatch"
)

func Workflow(path string) string {
	switch path {
	case "/api/profile/ingest":
		return "profile_ingestion"
	case "/api/profile/review":
		return "profile_review"
	case "/api/cv/generate":
		return "cv_generation"
	case "/api/qualification-gaps":
		return "qualification_gaps"
	case "/api/application-draft":
		return "application_draft"
	}
	return ""
}

// NewHarness installs only the disposable local schema. These deliberately
// roomy finite values let multi-leg fixtures reach support checks; they are not
// proposed operating limits or an authorization to spend.
func NewHarness(t *testing.T, f *Fixture, namespace string) *aicontrol.Harness {
	t.Helper()
	gate := aicontrol.NewSyntheticGate()
	cfg := aicontrol.DefaultSyntheticConfig(namespace, gate)
	cfg.Limit = 6000
	cfg.Capacity = 64
	h, err := aicontrol.NewSynthetic(context.Background(), f.DSN, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	gate.Reconcile(namespace, 1)
	if err := h.SyntheticReviewer().SeedGrant(context.Background(), f.Principal, cfg.ExpiresAt, cfg.CustomerUnits); err != nil {
		t.Fatal(err)
	}
	return h
}

// AssertFunction exercises the supplied actual Function handler using both its
// public alias and direct URL. The controller can only arrive through the Go
// context, never a caller header or environment-enabled managed mode.
func AssertFunction(t *testing.T, fn func(context.Context, events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error), route, alias string) {
	t.Helper()
	f := NewFixture(t)
	f.ConfigureEnv(t)
	h := NewHarness(t, f, "function_fixture")
	for _, flag := range []string{"", "0"} {
		t.Run("auth_flag_"+flag, func(t *testing.T) {
			t.Setenv("CAREEROS_REQUIRE_AUTH", flag)
			for index, path := range []string{route, alias} {
				t.Run(path, func(t *testing.T) {
					body := f.Body(t, route)
					fake := &Provider{T: t, Workflow: Workflow(route)}
					h.SetResponder(fake.Respond)
					session, err := h.Open(context.Background(), f.Principal, "action_"+flag+"_"+string(rune('a'+index)), Workflow(route), EffectiveRoot(t, route, body))
					if err != nil {
						t.Fatal(err)
					}
					ctx := aidispatch.WithController(context.Background(), session)
					event := f.Event(t, path, route, body)
					anonymous := event
					anonymous.Headers = map[string]string{"Origin": Origin, "X-CareerOS-Account": f.Login.Account.ID, "X-OpenAI-Api-Key": "sk-synthetic", "X-TypeSafe-Api-Key": "synthetic"}
					denied, err := fn(ctx, anonymous)
					if err != nil || denied.StatusCode != 401 || len(fake.Calls) != 0 {
						t.Fatalf("anonymous Function reached dispatch: %v %+v calls=%v", err, denied, fake.Calls)
					}
					missing, err := fn(aidispatch.RequireManaged(context.Background()), event)
					if err != nil || missing.StatusCode < 400 || len(fake.Calls) != 0 {
						t.Fatalf("missing control reached provider: %v %+v calls=%v", err, missing, fake.Calls)
					}
					response, err := fn(ctx, event)
					if err != nil || response.StatusCode != http.StatusOK || len(fake.Calls) == 0 {
						t.Fatalf("valid Function control failed: %v %+v calls=%v", err, response, fake.Calls)
					}
					status, err := h.Status(context.Background(), f.Principal, session.ID())
					if err != nil || len(status.Dispatches) != len(fake.Calls) || status.CustomerEffect {
						t.Fatalf("Function sends not accounted independently of delivery: %v %+v calls=%v", err, status, fake.Calls)
					}
				})
			}
		})
	}
	t.Run("configured_session_missing_policy", func(t *testing.T) {
		body := f.Body(t, route)
		fake := &Provider{T: t, Workflow: Workflow(route)}
		h.SetResponder(fake.Respond)
		session, err := h.Open(context.Background(), f.Principal, "missing_policy", Workflow(route), EffectiveRoot(t, route, body))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.DB.Exec(`DELETE FROM ai_fixture_policy WHERE namespace='function_fixture'`); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{route, alias} {
			response, err := fn(aidispatch.WithController(context.Background(), session), f.Event(t, path, route, body))
			if err != nil || response.StatusCode < 400 || len(fake.Calls) != 0 {
				t.Fatalf("missing SQL policy did not fail closed: %v %+v calls=%v", err, response, fake.Calls)
			}
		}
	})
}
