package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"professional-information-repo/internal/accountstore"
	"professional-information-repo/internal/draftrecovery"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestOperatorControlInferenceStopAllowsIntendedDraftPublication(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	var calls atomic.Int32
	h.SetFault("after_controlled_callback", func() error {
		if calls.Add(1) == 4 {
			_, err := f.Store.LocalControls(nil).Apply(context.Background(), f.ControlOperator.Principal, "stop-after-full-graph", accountstore.ControlCommand{Control: "inference", Stop: true, ExpectedVersion: 1})
			return err
		}
		return nil
	})
	body, _ := json.Marshal(input)
	response := qualificationRequest(t, f, router, "/api/drafts", "intended-complete-after-stop", "", body)
	var ready draftrecovery.Result
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &ready) != nil || ready.Bundle == nil || len(provider.Calls) != 4 {
		t.Fatalf("intended complete publication %d %s calls%v", response.Code, response.Body.String(), provider.Calls)
	}
	fetched := openingRequest(t, f, router, "GET", "/api/drafts/intended-complete-after-stop", nil)
	var recovered draftrecovery.Result
	if fetched.Code != 200 || json.Unmarshal(fetched.Body.Bytes(), &recovered) != nil || !reflect.DeepEqual(ready, recovered) {
		t.Fatalf("authorized existing read %d %s", fetched.Code, fetched.Body.String())
	}
	issued := openingRequest(t, f, router, "POST", "/api/drafts/intended-complete-after-stop/versions/1/opening", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
		t.Fatal(issued.Code, issued.Body.String())
	}
	confirmed := openingRequest(t, f, router, "POST", "/api/drafts/intended-complete-after-stop/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
	if confirmed.Code != 200 || len(provider.Calls) != 4 {
		t.Fatalf("inference stop over-denied opening %d %s", confirmed.Code, confirmed.Body.String())
	}
}

func TestOperatorControlCompromisePreservesDeliveredAndUnknownObligations(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	body, _ := json.Marshal(input)
	const prior = "delivered-before-compromise"
	created := qualificationRequest(t, f, router, "/api/drafts", prior, "", body)
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	issued := openingRequest(t, f, router, "POST", "/api/drafts/"+prior+"/versions/1/opening", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
		t.Fatal(issued.Code, issued.Body.String())
	}
	confirmed := openingRequest(t, f, router, "POST", "/api/drafts/"+prior+"/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
	if confirmed.Code != 200 {
		t.Fatal(confirmed.Code, confirmed.Body.String())
	}
	var delivered draftrecovery.Result
	if json.Unmarshal(confirmed.Body.Bytes(), &delivered) != nil || !delivered.Delivered {
		t.Fatal("expected first delivery")
	}
	// The next first leg is attributable but has no independent terminal proof.
	var reached atomic.Int32
	if err := h.SetQualificationResponder(func(ctx context.Context, p string, r *http.Request) (*http.Response, bool, error) {
		reached.Add(1)
		response, err := provider.Respond(ctx, p, r)
		return response, false, err
	}); err != nil {
		t.Fatal(err)
	}
	h.SetFault("after_controlled_callback", func() error {
		if reached.Load() == 1 {
			_, err := f.Store.LocalControls(nil).Apply(context.Background(), f.ControlOperator.Principal, "compromise-after-first-leg", accountstore.ControlCommand{Control: "compromise", Stop: true, ExpectedVersion: 1, AccountID: f.Principal.AccountID})
			return err
		}
		return nil
	})
	failed := qualificationRequest(t, f, router, "/api/drafts", "unconfirmed-on-compromise", "", body)
	if failed.Code < 400 || reached.Load() != 1 || len(provider.Calls) != 5 {
		t.Fatalf("compromise should deny publication/later leg %d sends%d", failed.Code, reached.Load())
	}
	var reserved, effects, unknown int
	var total int64
	if err := f.DB.QueryRow(`SELECT count(*) FILTER(WHERE customer_reserved),count(*) FILTER(WHERE customer_effect) FROM ai_fixture_action WHERE owner=$1`, f.Principal.AccountID).Scan(&reserved, &effects); err != nil {
		t.Fatal(err)
	}
	if err := f.DB.QueryRow(`SELECT COALESCE(sum(counted),0),count(*) FILTER(WHERE remote_state<>'terminal_proven') FROM ai_fixture_dispatch`).Scan(&total, &unknown); err != nil {
		t.Fatal(err)
	}
	if reserved != 0 || effects != 1 || total != 36_688 || unknown != 1 {
		t.Fatalf("customer/provider separation reserved%d effects%d total%d unknown%d", reserved, effects, total, unknown)
	}
	if denied := openingRequest(t, f, router, "GET", "/api/drafts/"+prior, nil); denied.Code != 401 && denied.Code != 403 {
		t.Fatal("old delivered cache remained readable", denied.Code)
	}

}
