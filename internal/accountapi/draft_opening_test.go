package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type openingAllowance struct {
	Allowance int `json:"allowance"`
	Reserved  int `json:"reserved"`
	Consumed  int `json:"consumed"`
	Remaining int `json:"remaining"`
}

func openingRequest(t *testing.T, f *aiadmission.Fixture, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := f.Request(t, context.Background(), path, raw)
	r.Method = method
	r.Header.Del("X-OpenAI-Api-Key")
	r.Header.Del("X-TypeSafe-Api-Key")
	if method == http.MethodPost && path == "/api/drafts" {
		r.Header.Set("X-CareerOS-Action-Key", "complete-opening")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestDraftOpeningCompleteReceiptCommitsOnePilotRight(t *testing.T) {
	f, _, _, _ := qualificationFixture(t) // Actual finite #276 activation, not SYN units.
	ctx := context.Background()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("opening-tracer", 1)
	actions, err := aicontrol.NewSynthetic(ctx, f.DSN, aicontrol.DefaultSyntheticConfig("opening-tracer", gate))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { actions.Close() })
	if err = actions.SyntheticReviewer().SeedGrant(ctx, f.Principal, time.Now().Add(time.Hour), 99); err != nil {
		t.Fatal(err)
	}
	var checks, generations atomic.Int32
	store, err := draftrecovery.OpenLocalOpening(ctx, f.DSN, draftrecovery.Config{
		Actions: actions, BindingKey: []byte("synthetic-opening-receipt-binding-key"),
		Review: func(ctx context.Context, r draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
			checks.Add(1)
			return controlledRecoveryReview(ctx, r)
		},
		Decisions: draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
			var expiry time.Time
			var epoch uint64
			err := a.Transaction.QueryRowContext(ctx, `SELECT expires_at,(extract(epoch FROM activated_at)*1000000)::bigint FROM careeros_pilot_access WHERE account_id=$1 AND state='active'`, a.Principal.AccountID).Scan(&expiry, &epoch)
			return draftrecovery.Decision{Generate: true, Publish: true, Read: true, GrantExpiresAt: expiry, Epoch: epoch}, err
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var router http.Handler
	router = accountapi.NewControlledDraftRecovery(f.Service, store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		generations.Add(1)
		w := openingRequest(t, f, router, http.MethodGet, "/api/draft-allowance", nil)
		var status openingAllowance
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status != (openingAllowance{5, 1, 0, 4}) {
			t.Errorf("before generation allowance = %d %s; want actual five with one reserved", w.Code, w.Body.String())
		}
		return completeRecoveryBundle(g), nil
	})
	var doc profiledocument.Document
	if err = json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	in := draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision}, Job: draftrecovery.Job{Posting: "Java required."}, Language: "en"}
	created := openingRequest(t, f, router, http.MethodPost, "/api/drafts", in)
	var ready draftrecovery.Result
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &ready) != nil || ready.State != "ready" || ready.Delivered || ready.Bundle == nil {
		t.Fatalf("ready is not delivery: %d %s", created.Code, created.Body.String())
	}
	receiptReply := openingRequest(t, f, router, http.MethodPost, "/api/drafts/complete-opening/versions/1/opening", struct{}{})
	var receipt struct {
		Receipt      string          `json:"receipt"`
		BundleDigest string          `json:"bundleDigest"`
		Manifest     json.RawMessage `json:"manifest"`
	}
	if receiptReply.Code != 200 || json.Unmarshal(receiptReply.Body.Bytes(), &receipt) != nil || receipt.Receipt == "" || receipt.BundleDigest != ready.BundleDigest || len(receipt.Manifest) == 0 {
		t.Fatalf("issued exact complete receipt: %d %s", receiptReply.Code, receiptReply.Body.String())
	}
	command := map[string]any{"receipt": receipt.Receipt, "bundleDigest": receipt.BundleDigest, "manifest": receipt.Manifest}
	confirmed := openingRequest(t, f, router, http.MethodPost, "/api/drafts/complete-opening/versions/1/opening/confirm", command)
	var delivered draftrecovery.Result
	if confirmed.Code != 200 || json.Unmarshal(confirmed.Body.Bytes(), &delivered) != nil || !delivered.Delivered || !reflect.DeepEqual(delivered.Bundle, ready.Bundle) || delivered.Action != ready.Action {
		t.Fatalf("one confirmed complete delivery: %d %s", confirmed.Code, confirmed.Body.String())
	}
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		path, body := "/api/drafts/complete-opening/versions/1", any(nil)
		if method == http.MethodPost {
			path += "/opening/confirm"
			body = command
		}
		w := openingRequest(t, f, router, method, path, body)
		var recovered draftrecovery.Result
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &recovered) != nil || !reflect.DeepEqual(delivered, recovered) {
			t.Fatalf("same settlement recovery: %d %s", w.Code, w.Body.String())
		}
	}
	statusReply := openingRequest(t, f, router, http.MethodGet, "/api/draft-allowance", nil)
	var status openingAllowance
	if statusReply.Code != 200 || json.Unmarshal(statusReply.Body.Bytes(), &status) != nil || status != (openingAllowance{5, 0, 1, 4}) || generations.Load() != 1 || checks.Load() != 1 {
		t.Fatalf("one effect without extra callbacks: %d %s generation=%d check=%d", statusReply.Code, statusReply.Body.String(), generations.Load(), checks.Load())
	}
}
