package accountapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"professional-information-repo/internal/aicontrol"
)

func TestRestrictedReconciliationSelectedUSDRouterHistory(t *testing.T) {
	f, h, provider, router := qualificationFixture(t)
	body := f.Body(t, "/api/qualification-gaps")
	if got := qualificationRequest(t, f, router, "/api/qualification-gaps", "usd_history", "", body); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	session, err := h.Open(context.Background(), f.Principal, "usd_history", "qualification_gaps", body)
	if err != nil {
		t.Fatal(err)
	}
	before, err := h.Status(context.Background(), f.Principal, session.ID())
	if err != nil {
		t.Fatal(err)
	}
	var dispatch aicontrol.Dispatch
	for _, d := range before.Dispatches {
		if d.Provider == "typesafe" {
			dispatch = d
		}
	}
	if dispatch.Maximum != 1344 {
		t.Fatal(dispatch)
	}
	if err = aicontrol.InstallLocalReconciliation(context.Background(), f.DB, "qualification", f.Principal.AccountID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	p := func(n int64) *int64 { return &n }
	command := aicontrol.ReconciliationCommand{ID: "selected_usage", Kind: "usage", Dispatch: dispatch.ID, Evidence: aicontrol.ReconciliationEvidence{Source: "verified_usage", Revision: 1, Reason: "complete_gross_coverage", Provider: "typesafe", Model: "jev-1.13.0", Tier: "standard", Rate: aicontrol.QualificationRateID, Currency: "USD", BillingNamespace: "synthetic", RequestID: "verified_request", Complete: true, Usage: &aicontrol.Usage{Input: p(3095), Output: p(0)}, Difference: p(7)}}
	control := h.Reconciliation()
	initial, err := control.Status(context.Background(), f.Principal, dispatch.ID)
	if err != nil {
		t.Fatal(err)
	}
	command.ExpectedVersion = initial.Version
	preview, err := control.Preview(context.Background(), f.Principal, command)
	// 3095 * .042 microUSD rounds up to130, plus the independently justified7.
	if err != nil || preview.Amount == nil || *preview.Amount != 130 || preview.After != 137 || preview.Before != 1344 {
		t.Fatal(preview, err)
	}
	command.Preview = preview.Token
	if _, err = control.Apply(context.Background(), f.Principal, command); err != nil {
		t.Fatal(err)
	}
	state, err := control.Status(context.Background(), f.Principal, dispatch.ID)
	if err != nil || state.Counted != 137 || state.Remote != "terminal_proven" {
		t.Fatal(state, err)
	}
	global, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || global.Counted != 24137 || global.Occupied != 0 {
		t.Fatal(global, err)
	}
	allowance, err := f.Store.PilotStatus(context.Background(), f.Principal)
	if err != nil || allowance.DraftAllowance != 5 {
		t.Fatal(allowance, err)
	}
	replay := qualificationRequest(t, f, router, "/api/qualification-gaps", "usd_history", "", body)
	if replay.Code != 202 || len(provider.Calls) != 3 {
		t.Fatal(replay.Code, provider.Calls)
	}
	command.ID = "bad_currency"
	command.ExpectedVersion = initial.Version + 1
	command.Evidence.Source = "bad_currency"
	command.Evidence.Currency = "EUR"
	if _, err = control.Preview(context.Background(), f.Principal, command); err == nil {
		t.Fatal("unsupported conversion repriced original dispatch")
	}
	after, err := h.Status(context.Background(), f.Principal, session.ID())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range after.Dispatches {
		if d.ID == dispatch.ID && (d.Expense == nil || *d.Expense != 130 || d.Counted != 137) {
			raw, _ := json.Marshal(d)
			t.Fatal(string(raw))
		}
	}
}

func TestRestrictedReconciliationDefaultRuntimeBreachContains(t *testing.T) {
	for _, closed := range []bool{false, true} {
		name := "active"
		if closed {
			name = "customer_closed"
		}
		t.Run(name, func(t *testing.T) {
			f, h, provider, router := qualificationFixture(t)
			if err := h.SetQualificationResponder(func(ctx context.Context, name string, request *http.Request) (*http.Response, bool, error) {
				response, err := provider.Respond(ctx, name, request)
				if err != nil {
					return response, false, err
				}
				if name == "typesafe" {
					raw, readErr := io.ReadAll(response.Body)
					response.Body.Close()
					if readErr != nil {
						return nil, false, readErr
					}
					var object map[string]any
					if json.Unmarshal(raw, &object) != nil {
						t.Fatal("invalid fixture response")
					}
					object["model"] = "jev-1.13.0"
					object["service_tier"] = "standard"
					object["usage"] = map[string]int64{"input_tokens": 64000, "output_tokens": 0}
					changed, _ := json.Marshal(object)
					response.Body = io.NopCloser(bytes.NewReader(changed))
					response.Header.Set("X-Request-ID", "runtime_over_bound")
					if closed {
						if _, err = f.DB.Exec(`UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, f.Principal.SessionID); err != nil {
							t.Fatal(err)
						}
						if _, err = f.DB.Exec(`UPDATE careeros_pilot_access SET state='revoked' WHERE account_id=$1`, f.Principal.AccountID); err != nil {
							t.Fatal(err)
						}
					}
				}
				return response, true, nil
			}); err != nil {
				t.Fatal(err)
			}
			got := qualificationRequest(t, f, router, "/api/qualification-gaps", "runtime_breach", "", f.Body(t, "/api/qualification-gaps"))
			if got.Code == 200 || len(provider.Calls) != 1 {
				t.Fatalf("uncovered runtime observation admitted another leg: status%d calls%v", got.Code, provider.Calls)
			}
			operator := f.Principal
			if closed {
				start, err := f.Service.BeginLogin(context.Background(), "google", "/", nil)
				if err != nil {
					t.Fatal(err)
				}
				u, _ := url.Parse(start.URL)
				login, err := f.Service.CompleteLogin(context.Background(), "google", u.Query().Get("state"), "fixture-account", start.BindingToken)
				if err != nil {
					t.Fatal(err)
				}
				operator = login.Principal
			}
			if err := aicontrol.InstallLocalReconciliation(context.Background(), f.DB, "qualification", operator.AccountID, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			events, err := h.Reconciliation().Events(context.Background(), operator)
			if err != nil || len(events) != 1 || events[0].Obligation != 2688 || events[0].Reserved != 1344 {
				t.Fatal(events, err)
			}
			state, err := h.Reconciliation().Status(context.Background(), operator, events[0].Dispatch)
			if err != nil || state.Expense != nil || state.Counted != 2688 || !state.AdmissionStopped || state.Category != "unknown_maximum" {
				t.Fatal("observation relabeled as settlement", state, err)
			}
		})
	}
}
