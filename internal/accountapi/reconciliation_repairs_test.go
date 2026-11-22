package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"professional-information-repo/internal/aicontrol"
)

func TestReconciliationCoverageFailureCannotSuppressObservedBreach(t *testing.T) {
	for _, mode := range []string{"nil", "accepting", "rejecting", "erroring", "missing_proof"} {
		for _, closed := range []bool{false, true} {
			authority := "active"
			if closed {
				authority = "customer_closed"
			}
			t.Run(mode+"/"+authority, func(t *testing.T) {
				f, h, provider, router := qualificationFixture(t)
				if mode != "nil" {
					h.SetSyntheticUsageCoverage(func(aicontrol.BillingContract, aicontrol.ProviderObservation) (aicontrol.UnaccountedBound, error) {
						switch mode {
						case "accepting":
							return aicontrol.UnaccountedBound{Amount: 0, Reference: "complete_fake_gross_coverage"}, nil
						case "rejecting":
							return aicontrol.UnaccountedBound{}, aicontrol.ErrDenied
						case "erroring":
							return aicontrol.UnaccountedBound{}, errors.New("synthetic coverage unavailable")
						default:
							return aicontrol.UnaccountedBound{}, nil
						}
					})
				}
				if err := h.SetQualificationResponder(func(ctx context.Context, name string, request *http.Request) (*http.Response, bool, error) {
					response, err := provider.Respond(ctx, name, request)
					if err != nil {
						return response, false, err
					}
					if name == "typesafe" {
						raw, err := io.ReadAll(response.Body)
						response.Body.Close()
						if err != nil {
							return nil, false, err
						}
						var body map[string]any
						if json.Unmarshal(raw, &body) != nil {
							t.Fatal("invalid fake response")
						}
						body["model"] = "jev-1.13.0"
						body["service_tier"] = "standard"
						body["usage"] = map[string]int64{"input_tokens": 64000, "output_tokens": 0}
						raw, _ = json.Marshal(body)
						response.Body = io.NopCloser(bytes.NewReader(raw))
						response.Header.Set("X-Request-ID", "coverage_bound_request")
						if closed {
							if _, err = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.Principal.SessionID); err != nil {
								t.Fatal(err)
							}
							if _, err = f.DB.Exec(`UPDATE careeros_pilot_access SET state='revoked' WHERE account_id=$1`, f.Principal.AccountID); err != nil {
								t.Fatal(err)
							}
						}
					}
					return response, true, nil // explicit independent code-owned fake terminal proof
				}); err != nil {
					t.Fatal(err)
				}
				got := qualificationRequest(t, f, router, "/api/qualification-gaps", "coverage_breach", "", f.Body(t, "/api/qualification-gaps"))
				if got.Code == 200 || len(provider.Calls) != 1 {
					t.Fatalf("coverage result suppressed containment: status%d calls%v", got.Code, provider.Calls)
				}
				operator := f.Principal
				if closed {
					start, err := f.Service.BeginLogin(context.Background(), "google", "/", nil)
					if err != nil {
						t.Fatal(err)
					}
					u, _ := url.Parse(start.URL)
					fresh, err := f.Service.CompleteLogin(context.Background(), "google", u.Query().Get("state"), "fixture-account", start.BindingToken)
					if err != nil {
						t.Fatal(err)
					}
					operator = fresh.Principal
				}
				if err := aicontrol.InstallLocalReconciliation(context.Background(), f.DB, "qualification", operator.AccountID, time.Now().Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
				events, err := h.Reconciliation().Events(context.Background(), operator)
				if err != nil || len(events) != 1 || events[0].Obligation != 2688 || events[0].Reserved != 1344 {
					t.Fatal("missing exact above-bound evidence", events, err)
				}
				state, err := h.Reconciliation().Status(context.Background(), operator, events[0].Dispatch)
				if err != nil || state.Counted != 2688 || !state.AdmissionStopped {
					t.Fatal(state, err)
				}
				if mode == "accepting" {
					if state.Category != "supported_estimate" || state.Expense == nil || *state.Expense != 2688 {
						t.Fatal("valid settlement lost", state)
					}
				} else if state.Category != "unknown_maximum" || state.Expense != nil {
					t.Fatal("unverified D released or relabeled hold", state)
				}
				wantRemote := "terminal_proven"
				wantOccupied := 0
				if closed {
					wantRemote = "possibly_active"
					wantOccupied = 1
				}
				if state.Remote != wantRemote {
					t.Fatal("financial evidence changed independent capacity", state)
				}
				for _, scope := range state.Scopes {
					if scope.Counted != 2688 || scope.Occupied != wantOccupied {
						t.Fatal(scope)
					}
				}
				access, err := f.Store.PilotStatus(context.Background(), operator)
				if err != nil || access.DraftAllowance != 5 {
					t.Fatal("financial observation changed Draft allowance", access, err)
				}
			})
		}
	}
}

func TestReconciliationCoverageStillSettlesUPlusDWithoutReleasingRemoteHold(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	h.SetSyntheticUsageCoverage(func(aicontrol.BillingContract, aicontrol.ProviderObservation) (aicontrol.UnaccountedBound, error) {
		return aicontrol.UnaccountedBound{Amount: 7, Reference: "verified_remaining_gross_bound"}, nil
	})
	if err := h.SetQualificationResponder(func(ctx context.Context, name string, request *http.Request) (*http.Response, bool, error) {
		response, err := provider.Respond(ctx, name, request)
		if err != nil {
			return response, false, err
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return nil, false, err
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil {
			t.Fatal("invalid fake response")
		}
		body["model"] = "jev-1.13.0"
		body["service_tier"] = "standard"
		body["usage"] = map[string]int64{"input_tokens": 3095, "output_tokens": 0}
		raw, _ = json.Marshal(body)
		response.Body = io.NopCloser(bytes.NewReader(raw))
		response.Header.Set("X-Request-ID", "normal_coverage_request")
		return response, false, nil // financial evidence supplies no terminal proof
	}); err != nil {
		t.Fatal(err)
	}
	got := qualificationRequest(t, f, router, "/api/qualification-gaps", "coverage_normal", "", f.Body(t, "/api/qualification-gaps"))
	if got.Code == 200 || len(provider.Calls) != 1 {
		t.Fatal("unknown capacity unexpectedly released", got.Code, provider.Calls)
	}
	if err := aicontrol.InstallLocalReconciliation(context.Background(), f.DB, "qualification", f.Principal.AccountID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var dispatch string
	if err := f.DB.QueryRow(`SELECT id FROM ai_fixture_dispatch WHERE namespace='qualification'`).Scan(&dispatch); err != nil {
		t.Fatal(err)
	}
	state, err := h.Reconciliation().Status(context.Background(), f.Principal, dispatch)
	if err != nil || state.Counted != 137 || state.Expense == nil || *state.Expense != 130 || state.Category != "supported_estimate" || state.AdmissionStopped || state.Remote != "possibly_active" {
		t.Fatal(state, err)
	}
	for _, scope := range state.Scopes {
		if scope.Counted != 137 || scope.Occupied != 1 {
			t.Fatal(scope)
		}
	}
}
