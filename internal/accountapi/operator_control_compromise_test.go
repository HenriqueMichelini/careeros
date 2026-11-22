package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func controlFreshIdentity(t *testing.T, service *auth.Service, subject string) auth.LoginResult {
	t.Helper()
	start, err := service.BeginLogin(context.Background(), "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	result, err := service.CompleteLogin(context.Background(), "google", u.Query().Get("state"), subject, start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOperatorControlCompromiseRequiresFreshRestrictedRights(t *testing.T) {
	f, h, provider, _ := qualificationFixture(t)
	ctx := context.Background()
	if err := f.Store.WithRestrictedRightsStatus(ctx, f.Principal, func(accountstore.RestrictedRightsStatus) error { return nil }); err != auth.ErrForbidden {
		t.Fatalf("ordinary session cannot become restricted authority: %v", err)
	}
	body := f.Body(t, "/api/qualification-gaps")
	if _, err := h.Open(ctx, f.Principal, "unconfirmed-before-compromise", "qualification_gaps", body); err != nil {
		t.Fatal(err)
	}
	var beforeActivation, beforeExpiry time.Time
	if err := f.DB.QueryRow(`SELECT activated_at,expires_at FROM careeros_pilot_access WHERE account_id=$1`, f.Principal.AccountID).Scan(&beforeActivation, &beforeExpiry); err != nil {
		t.Fatal(err)
	}
	command := accountstore.ControlCommand{Control: "compromise", Stop: true, ExpectedVersion: 1, AccountID: f.Principal.AccountID}
	if _, err := f.Store.LocalControls(nil).Apply(ctx, f.ControlOperator.Principal, "stop-compromised-account", command); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Service.Authenticate(ctx, f.Login.SessionToken); err == nil {
		t.Fatal("old ordinary session survived compromised-access stop")
	}
	router := accountapi.NewPilot(f.Service, f.Store, nil)
	for _, path := range []string{"/api/profile/current", "/api/profile/export", "/.netlify/functions/account/api/profile/current"} {
		r := f.Request(t, ctx, path, nil)
		r.Method = http.MethodGet
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		if response.Code != 401 && response.Code != 403 {
			t.Fatalf("old ordinary route %s: %d", path, response.Code)
		}
	}
	var grantState string
	var held, noEarlyEnd bool
	var afterActivation, afterExpiry time.Time
	if err := f.DB.QueryRow(`SELECT state,(retrieval_held OR control_compromise_held),access_ended_at IS NULL,activated_at,expires_at FROM careeros_pilot_access WHERE account_id=$1`, f.Principal.AccountID).Scan(&grantState, &held, &noEarlyEnd, &afterActivation, &afterExpiry); err != nil {
		t.Fatal(err)
	}
	if grantState != "suspended" || !held || !noEarlyEnd || !beforeActivation.Equal(afterActivation) || !beforeExpiry.Equal(afterExpiry) {
		t.Fatal("compromise changed original grant/access-end clocks")
	}
	var actionState string
	if err := f.DB.QueryRow(`SELECT state FROM ai_fixture_action WHERE owner=$1 AND creation_key='unconfirmed-before-compromise'`, f.Principal.AccountID).Scan(&actionState); err != nil || actionState != "canceled" {
		t.Fatalf("unconfirmed Action must remain terminal: %q %v", actionState, err)
	}
	fresh := controlFreshIdentity(t, f.Service, "fixture-account")
	restricted := *f
	restricted.Login, restricted.Principal = fresh, fresh.Principal
	for _, path := range []string{"/api/profile/current", "/api/profile/export"} {
		r := restricted.Request(t, ctx, path, nil)
		r.Method = http.MethodGet
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		if response.Code != 401 && response.Code != 403 {
			t.Fatalf("fresh restricted identity gained ordinary access %s: %d", path, response.Code)
		}
	}
	r := restricted.Request(t, ctx, "/api/auth/session", nil)
	r.Method = http.MethodGet
	response := httptest.NewRecorder()
	router.ServeHTTP(response, r)
	var session struct {
		Authenticated    bool `json:"authenticated"`
		RestrictedRights bool `json:"restrictedRights"`
	}
	if json.Unmarshal(response.Body.Bytes(), &session) != nil || session.Authenticated || !session.RestrictedRights {
		t.Fatalf("fresh restricted session scope: %d %s", response.Code, response.Body.String())
	}
	r = restricted.Request(t, ctx, "/api/rights/status", nil)
	r.Method = http.MethodGet
	response = httptest.NewRecorder()
	router.ServeHTTP(response, r)
	if response.Code != 200 {
		t.Fatalf("fresh restricted rights metadata: %d %s", response.Code, response.Body.String())
	}
	other := controlFreshIdentity(t, f.Service, "independent-control-account")
	if _, err := f.Store.SaveProfile(ctx, other.Principal, "independent-save", 0, f.Document); err != nil {
		t.Fatalf("independent account was stopped: %v", err)
	}
	closeBody, _ := json.Marshal(accountstore.DeletionRequest{OperationID: "fresh-restricted-deletion", Kind: "account", Confirm: true})
	r = restricted.Request(t, ctx, "/api/rights/deletion/operations", closeBody)
	r.Header.Set("Origin", aiadmission.Origin)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, r)
	if response.Code != 202 || len(provider.Calls) != 0 {
		t.Fatalf("restricted explicit deletion: %d calls=%d %s", response.Code, len(provider.Calls), response.Body.String())
	}
}
