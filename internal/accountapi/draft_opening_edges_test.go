package accountapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type openingFixture struct {
	f                 *aiadmission.Fixture
	actions           *aicontrol.Harness
	store             *draftrecovery.Store
	router            http.Handler
	input             draftrecovery.Input
	config            draftrecovery.Config
	generation, check atomic.Int32
	fault             func(string) error
	deleted           atomic.Bool
	cancelled         atomic.Bool
}

func newOpeningFixture(t *testing.T) *openingFixture {
	t.Helper()
	f, _, _, _ := qualificationFixture(t)
	x := &openingFixture{f: f}
	ctx := context.Background()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("opening-edge", 1)
	var err error
	x.actions, err = aicontrol.NewSynthetic(ctx, f.DSN, aicontrol.DefaultSyntheticConfig("opening-edge", gate))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { x.actions.Close() })
	if err = x.actions.SyntheticReviewer().SeedGrant(ctx, f.Principal, time.Now().Add(time.Hour), 99); err != nil {
		t.Fatal(err)
	}
	x.config = draftrecovery.Config{
		Actions: x.actions, BindingKey: []byte("synthetic-opening-edge-binding-key"),
		Review: func(ctx context.Context, r draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
			x.check.Add(1)
			return controlledRecoveryReview(ctx, r)
		},
		Decisions: draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
			var expiry time.Time
			var epoch uint64
			var active bool
			err := a.Transaction.QueryRowContext(ctx, `SELECT expires_at,(extract(epoch FROM activated_at)*1000000)::bigint,state='active' AND age_eligible AND NOT invitation_revoked FROM careeros_pilot_access WHERE account_id=$1`, a.Principal.AccountID).Scan(&expiry, &epoch, &active)
			return draftrecovery.Decision{Generate: active, Publish: active, Read: active, Revoked: !active, Deleted: x.deleted.Load(), Cancelled: x.cancelled.Load(), GrantExpiresAt: expiry, Epoch: epoch}, err
		}),
		Fault: func(stage string) error {
			if x.fault != nil {
				return x.fault(stage)
			}
			return nil
		},
	}
	x.store, err = draftrecovery.OpenLocalOpening(ctx, f.DSN, x.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { x.store.Close() })
	if err = x.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	x.router = accountapi.NewControlledDraftRecovery(f.Service, x.store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		x.generation.Add(1)
		return completeRecoveryBundle(g), nil
	})
	var doc profiledocument.Document
	if err = json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	x.input = draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision}, Job: draftrecovery.Job{Posting: "Java required."}, Language: "en"}
	return x
}

func (x *openingFixture) request(t *testing.T, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := x.f.Request(t, context.Background(), path, raw)
	r.Method = method
	r.Header.Del("X-OpenAI-Api-Key")
	r.Header.Del("X-TypeSafe-Api-Key")
	if key != "" {
		r.Header.Set("X-CareerOS-Action-Key", key)
	}
	w := httptest.NewRecorder()
	x.router.ServeHTTP(w, r)
	return w
}

func (x *openingFixture) create(t *testing.T, key string) draftrecovery.Result {
	t.Helper()
	w := x.request(t, http.MethodPost, "/api/drafts", key, x.input)
	var result draftrecovery.Result
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Bundle == nil || result.Delivered {
		t.Fatalf("complete undelivered creation: %d %s", w.Code, w.Body.String())
	}
	return result
}

func (x *openingFixture) receipt(t *testing.T, key string) draftrecovery.OpeningReceipt {
	t.Helper()
	w := x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening", "", struct{}{})
	var receipt draftrecovery.OpeningReceipt
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &receipt) != nil || receipt.Receipt == "" {
		t.Fatalf("receipt: %d %s", w.Code, w.Body.String())
	}
	return receipt
}

func (x *openingFixture) confirm(t *testing.T, key string, r draftrecovery.OpeningReceipt) *httptest.ResponseRecorder {
	t.Helper()
	return x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm", "", draftrecovery.OpeningConfirmation{Receipt: r.Receipt, BundleDigest: r.BundleDigest, Manifest: r.Manifest})
}

func (x *openingFixture) allowance(t *testing.T) openingAllowance {
	t.Helper()
	w := x.request(t, http.MethodGet, "/api/draft-allowance", "", nil)
	var v openingAllowance
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil {
		t.Fatalf("allowance: %d %s", w.Code, w.Body.String())
	}
	return v
}

func TestDraftOpeningDeadlineUsesTimeAfterEverySettlementLock(t *testing.T) {
	for _, point := range []string{"pilot", "reservation"} {
		t.Run(point, func(t *testing.T) {
			x := newOpeningFixture(t)
			if _, err := x.f.DB.Exec(`UPDATE careeros_pilot_access SET expires_at=clock_timestamp()+interval '1800 milliseconds' WHERE account_id=$1`, x.f.Principal.AccountID); err != nil {
				t.Fatal(err)
			}
			const key = "deadline-wait"
			created := x.create(t, key)
			r := x.receipt(t, key)
			block, err := x.f.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer block.Rollback()
			query := `SELECT account_id FROM careeros_pilot_access WHERE account_id=$1 FOR UPDATE`
			if point == "reservation" {
				query = `SELECT owner FROM careeros_draft_allowance WHERE owner=$1 FOR UPDATE`
			}
			var owner string
			if err = block.QueryRow(query, x.f.Principal.AccountID).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				done <- x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm", "", draftrecovery.OpeningConfirmation{Receipt: r.Receipt, BundleDigest: r.BundleDigest, Manifest: r.Manifest})
			}()
			// Observe a real PostgreSQL lock wait, not a guessed scheduling delay.
			limit := time.Now().Add(5 * time.Second)
			for {
				var waiting bool
				err = x.f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND state='active' AND wait_event_type='Lock' AND (query LIKE '%FOR UPDATE OF p%' OR query LIKE '%FROM careeros_draft_allowance%FOR UPDATE%' OR query LIKE 'UPDATE careeros_draft_allowance%'))`).Scan(&waiting)
				if err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				if time.Now().After(limit) {
					t.Fatal("confirmation never reached the settlement row lock")
				}
				time.Sleep(5 * time.Millisecond)
			}
			for {
				var elapsed bool
				if err = x.f.DB.QueryRow(`SELECT clock_timestamp()>=$1`, created.Deadline).Scan(&elapsed); err != nil {
					t.Fatal(err)
				}
				if elapsed {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if err = block.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case w := <-done:
				if w.Code < 400 {
					t.Fatalf("stale pre-lock time confirmed delivery: %d %s", w.Code, w.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("confirmation did not finish after releasing its lock")
			}
			status := x.request(t, http.MethodGet, "/api/draft-allowance", "", nil)
			var allowance openingAllowance
			if status.Code != 200 || json.Unmarshal(status.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 0, 0, 5}) {
				t.Fatalf("deadline must release once without consumption: %d %s", status.Code, status.Body.String())
			}
		})
	}
}

func TestDraftOpeningInvalidEvidenceAndNewSessionCannotCreateDelivery(t *testing.T) {
	x := newOpeningFixture(t)
	const key = "evidence-bound"
	created := x.create(t, key)
	r := x.receipt(t, key)
	command := draftrecovery.OpeningConfirmation{Receipt: r.Receipt, BundleDigest: r.BundleDigest, Manifest: r.Manifest}
	for _, kind := range []string{"forged", "digest", "partial", "mixed", "missing"} {
		t.Run(kind, func(t *testing.T) {
			bad := command
			bad.Manifest.RequiredParts = append([]string{}, command.Manifest.RequiredParts...)
			switch kind {
			case "forged":
				bad.Receipt += "x"
			case "digest":
				bad.BundleDigest = "0000000000000000000000000000000000000000000000000000000000000000"
			case "partial":
				bad.Manifest.RequiredParts = bad.Manifest.RequiredParts[:1]
			case "mixed":
				bad.Manifest.Version = 2
			case "missing":
				bad.Receipt = ""
			}
			w := x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm", "", bad)
			if w.Code < 400 {
				t.Fatalf("invalid evidence delivered: %d %s", w.Code, w.Body.String())
			}
		})
	}
	for _, path := range []string{"/api/drafts/" + key + "/versions/2/opening/confirm", "/api/drafts/wrong-action/versions/1/opening/confirm"} {
		if w := x.request(t, http.MethodPost, path, "", command); w.Code < 400 {
			t.Fatalf("wrong bound endpoint succeeded: %d", w.Code)
		}
	}
	foreign := qualificationParticipant(t, x.f, "foreign-opening")
	if w := openingRequest(t, foreign, x.router, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm", command); w.Code < 400 {
		t.Fatalf("foreign owner acknowledged another result: %d", w.Code)
	}
	// A fresh authenticated session for the same owner cannot reuse the old
	// session's receipt. It recovers the existing Action and obtains its own.
	start, err := x.f.Service.BeginLogin(context.Background(), "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	login, err := x.f.Service.CompleteLogin(context.Background(), "google", u.Query().Get("state"), "fixture-account", start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	x.f.Login, x.f.Principal = login, login.Principal
	if w := x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm", "", command); w.Code < 400 {
		t.Fatalf("receipt crossed sessions: %d", w.Code)
	}
	status := x.request(t, http.MethodGet, "/api/draft-allowance", "", nil)
	var allowance openingAllowance
	if status.Code != 200 || json.Unmarshal(status.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 1, 0, 4}) {
		t.Fatalf("invalid/partial evidence changed allowance: %d %s", status.Code, status.Body.String())
	}
	fresh := x.receipt(t, key)
	if fresh.Receipt == r.Receipt {
		t.Fatal("new session received old session receipt")
	}
	confirmed := x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening/confirm", "", draftrecovery.OpeningConfirmation{Receipt: fresh.Receipt, BundleDigest: fresh.BundleDigest, Manifest: fresh.Manifest})
	var result draftrecovery.Result
	if confirmed.Code != 200 || json.Unmarshal(confirmed.Body.Bytes(), &result) != nil || !result.Delivered || result.Action != created.Action || x.generation.Load() != 1 || x.check.Load() != 1 {
		t.Fatalf("fresh-session same result recovery failed: %d %s", confirmed.Code, confirmed.Body.String())
	}
}

func TestDraftOpeningRealPilotRevocationCannotBeOverriddenByInjectedFlags(t *testing.T) {
	x := newOpeningFixture(t)
	// The controlled policy adapter is an extra fence. It is not a substitute
	// for the actual finite pilot grant and cannot grant rights with booleans.
	x.config.Decisions = draftrecovery.DecisionFunc(func(context.Context, draftrecovery.Authority) (draftrecovery.Decision, error) {
		var epoch uint64
		var expiry time.Time
		if err := x.f.DB.QueryRow(`SELECT (extract(epoch FROM activated_at)*1000000)::bigint,expires_at FROM careeros_pilot_access WHERE account_id=$1`, x.f.Principal.AccountID).Scan(&epoch, &expiry); err != nil {
			return draftrecovery.Decision{}, err
		}
		return draftrecovery.Decision{Generate: true, Publish: true, Read: true, Delivered: true, GrantExpiresAt: expiry, Epoch: epoch}, nil
	})
	store, err := draftrecovery.OpenLocalOpening(context.Background(), x.f.DSN, x.config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	x.router = accountapi.NewControlledDraftRecovery(x.f.Service, store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		x.generation.Add(1)
		return completeRecoveryBundle(g), nil
	})
	const key = "real-pilot-fence"
	x.create(t, key)
	if _, err = x.f.DB.Exec(`UPDATE careeros_pilot_access SET state='revoked' WHERE account_id=$1`, x.f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path, body := "/api/drafts/"+key+"/versions/1", any(nil)
		if method == http.MethodPost {
			path += "/opening"
			body = struct{}{}
		}
		w := x.request(t, method, path, "", body)
		if w.Code < 400 {
			t.Fatalf("injected flags bypassed actual pilot revocation: %d %s", w.Code, w.Body.String())
		}
	}
	status := x.request(t, http.MethodGet, "/api/draft-allowance", "", nil)
	var allowance openingAllowance
	if status.Code != 200 || json.Unmarshal(status.Body.Bytes(), &allowance) != nil || allowance != (openingAllowance{5, 0, 0, 5}) {
		t.Fatalf("revocation did not release without consumption: %d %s", status.Code, status.Body.String())
	}
}

func TestDraftOpeningLastPilotRightRacesAcrossNamespacesAndTabs(t *testing.T) {
	x := newOpeningFixture(t)
	for _, key := range []string{"first", "second", "third", "fourth"} {
		x.create(t, key)
		if w := x.confirm(t, key, x.receipt(t, key)); w.Code != 200 {
			t.Fatalf("prior confirmed right: %d %s", w.Code, w.Body.String())
		}
	}
	if v := x.allowance(t); v != (openingAllowance{5, 0, 4, 1}) {
		t.Fatalf("one remaining finite right: %+v", v)
	}
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("opening-second-namespace", 1)
	otherActions, err := aicontrol.NewSynthetic(context.Background(), x.f.DSN, aicontrol.DefaultSyntheticConfig("opening-second-namespace", gate))
	if err != nil {
		t.Fatal(err)
	}
	defer otherActions.Close()
	if err = otherActions.SyntheticReviewer().SeedGrant(context.Background(), x.f.Principal, time.Now().Add(time.Hour), 99); err != nil {
		t.Fatal(err)
	}
	otherConfig := x.config
	otherConfig.Actions = otherActions
	otherStore, err := draftrecovery.OpenLocalOpening(context.Background(), x.f.DSN, otherConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer otherStore.Close()
	otherRouter := accountapi.NewControlledDraftRecovery(x.f.Service, otherStore, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		x.generation.Add(1)
		return completeRecoveryBundle(g), nil
	})
	type outcome struct {
		key    string
		router http.Handler
		code   int
	}
	done := make(chan outcome, 2)
	start := make(chan struct{})
	for i, router := range []http.Handler{x.router, otherRouter} {
		key := []string{"last-main", "last-other"}[i]
		go func(key string, router http.Handler) {
			<-start
			raw, _ := json.Marshal(x.input)
			r := x.f.Request(t, context.Background(), "/api/drafts", raw)
			r.Header.Del("X-OpenAI-Api-Key")
			r.Header.Del("X-TypeSafe-Api-Key")
			r.Header.Set("X-CareerOS-Action-Key", key)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			done <- outcome{key, router, w.Code}
		}(key, router)
	}
	close(start)
	var winner outcome
	wins, denied := 0, 0
	for range 2 {
		result := <-done
		if result.code == 201 {
			wins++
			winner = result
		} else if result.code == 429 {
			denied++
		} else {
			t.Fatalf("unexpected last-right outcome: %+v", result)
		}
	}
	if wins != 1 || denied != 1 || x.generation.Load() != 5 || x.allowance(t) != (openingAllowance{5, 1, 4, 0}) {
		t.Fatalf("last right was duplicated across namespaces: wins=%d denied=%d generation=%d allowance=%+v", wins, denied, x.generation.Load(), x.allowance(t))
	}
	x.router = winner.router
	receipt := x.receipt(t, winner.key)
	acknowledged := make(chan int, 8)
	for range 8 {
		go func() { acknowledged <- x.confirm(t, winner.key, receipt).Code }()
	}
	for range 8 {
		if code := <-acknowledged; code != 200 {
			t.Fatalf("cross-tab duplicate ACK: %d", code)
		}
	}
	if x.allowance(t) != (openingAllowance{5, 0, 5, 0}) || x.generation.Load() != 5 {
		t.Fatalf("duplicate ACK changed finite rights or generated: %+v %d", x.allowance(t), x.generation.Load())
	}
	if w := x.request(t, http.MethodPost, "/api/drafts", "no-sixth-delivery", x.input); w.Code != 429 || x.generation.Load() != 5 {
		t.Fatalf("sixth right was admitted: %d generation=%d", w.Code, x.generation.Load())
	}
}

func bumpOpeningProfile(t *testing.T, x *openingFixture) {
	t.Helper()
	var doc profiledocument.Document
	if err := json.Unmarshal(x.f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	expected := doc.Revision
	doc.Revision++
	for i := range doc.Facts {
		if doc.Facts[i].Owner.ID == doc.ID {
			doc.Facts[i].Owner.Revision = doc.Revision
		}
	}
	raw, _ := json.Marshal(doc)
	if _, err := x.f.Store.SaveProfile(context.Background(), x.f.Principal, "opening-source-change", expected, raw); err != nil {
		t.Fatal(err)
	}
	x.f.Document = raw
}

func TestDraftOpeningTerminalOrderPreservesOneCustomerOutcome(t *testing.T) {
	for _, fence := range []string{"cancel", "delete", "profile"} {
		for _, order := range []string{"fence_first", "opening_first"} {
			t.Run(fence+"/"+order, func(t *testing.T) {
				x := newOpeningFixture(t)
				const key = "terminal-order"
				x.create(t, key)
				receipt := x.receipt(t, key)
				if order == "opening_first" {
					if w := x.confirm(t, key, receipt); w.Code != 200 {
						t.Fatalf("eligible opening: %d %s", w.Code, w.Body.String())
					}
				}
				switch fence {
				case "cancel":
					x.cancelled.Store(true)
				case "delete":
					x.deleted.Store(true)
				case "profile":
					bumpOpeningProfile(t, x)
				}
				w := x.confirm(t, key, receipt)
				want := openingAllowance{5, 0, 0, 5}
				if order == "opening_first" {
					want = openingAllowance{5, 0, 1, 4}
					if fence != "delete" && w.Code != 200 {
						t.Fatalf("prior delivery lost its single effect: %d %s", w.Code, w.Body.String())
					}
				} else if w.Code < 400 {
					t.Fatalf("terminal fence lost: %d %s", w.Code, w.Body.String())
				}
				if x.allowance(t) != want {
					t.Fatalf("wrong customer outcome: got%+v want%+v", x.allowance(t), want)
				}
				if fence == "delete" {
					if err := x.store.Cleanup(context.Background(), x.f.Principal, key); err != nil {
						t.Fatal(err)
					}
				}
				// Apparent later rights restoration never revives a winning
				// terminal no-consumption outcome or refunds a delivered one.
				x.cancelled.Store(false)
				x.deleted.Store(false)
				for range 2 {
					x.confirm(t, key, receipt)
				}
				if x.allowance(t) != want || x.generation.Load() != 1 || x.check.Load() != 1 {
					t.Fatalf("terminal replay changed rights/callbacks: %+v generation%d checks%d", x.allowance(t), x.generation.Load(), x.check.Load())
				}
			})
		}
	}
}

func TestDraftOpeningLostReceiptAndConfirmationRepliesRecoverSameEffect(t *testing.T) {
	x := newOpeningFixture(t)
	const key = "lost-opening-reply"
	original := x.create(t, key)
	var loseReceipt, loseConfirmation atomic.Bool
	loseReceipt.Store(true)
	loseConfirmation.Store(true)
	x.fault = func(stage string) error {
		if stage == "after_opening_receipt_commit" && loseReceipt.Swap(false) || stage == "after_opening_commit" && loseConfirmation.Swap(false) {
			return errors.New("synthetic committed acknowledgment loss")
		}
		return nil
	}
	if w := x.request(t, http.MethodPost, "/api/drafts/"+key+"/versions/1/opening", "", struct{}{}); w.Code != 503 {
		t.Fatalf("lost receipt reply not exercised: %d", w.Code)
	}
	receipt, again := x.receipt(t, key), x.receipt(t, key)
	if receipt.Receipt != again.Receipt || x.allowance(t) != (openingAllowance{5, 1, 0, 4}) {
		t.Fatal("lost receipt issuance created delivery or changed issued identity")
	}
	if w := x.confirm(t, key, receipt); w.Code != 503 {
		t.Fatalf("lost confirmation reply not exercised: %d", w.Code)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		var w *httptest.ResponseRecorder
		if method == http.MethodPost {
			w = x.confirm(t, key, receipt)
		} else {
			w = x.request(t, method, "/api/drafts/"+key+"/versions/1", "", nil)
		}
		var result draftrecovery.Result
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.Delivered || result.Action != original.Action || result.Version != original.Version || result.AdmittedAt != original.AdmittedAt || result.Deadline != original.Deadline {
			t.Fatalf("lost outcome did not recover same immutable delivery: %d %s", w.Code, w.Body.String())
		}
	}
	if x.allowance(t) != (openingAllowance{5, 0, 1, 4}) || x.generation.Load() != 1 || x.check.Load() != 1 {
		t.Fatalf("lost ACK regenerated or debited again: %+v %d/%d", x.allowance(t), x.generation.Load(), x.check.Load())
	}
}

func TestDraftOpeningAllowanceStatusResolvesExpiredOriginalAdmissionOnce(t *testing.T) {
	x := newOpeningFixture(t)
	var first draftrecovery.Result
	for i, key := range []string{"expires-first", "pending-second", "pending-third", "pending-fourth", "pending-fifth"} {
		created := x.create(t, key)
		if i == 0 {
			first = created
		}
	}
	if x.allowance(t) != (openingAllowance{5, 5, 0, 0}) {
		t.Fatal("five ready but unopened bundles must reserve, not consume")
	}
	// This is an older durable snapshot, with all original admission anchors
	// moved together. Status must use elapsed DB time, not refresh admission.
	tx, err := x.f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var original time.Time
	if err = tx.QueryRow(`SELECT clock_timestamp()-interval '31 minutes'`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE ai_fixture_action SET created_at=$3 WHERE namespace=$1 AND id=$2`,
		`UPDATE careeros_draft_actions SET admitted_at=$3::timestamptz,deadline=$3::timestamptz+interval '30 minutes' WHERE namespace=$1 AND action_id=$2`,
		`UPDATE careeros_draft_allowance SET admitted_at=$3::timestamptz,deadline=$3::timestamptz+interval '30 minutes' WHERE namespace=$1 AND action_id=$2`,
	} {
		if _, err = tx.Exec(query, first.Action.Namespace, first.Action.ID, original); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if v := x.allowance(t); v != (openingAllowance{5, 4, 0, 1}) {
			t.Fatalf("status did not resolve the original elapsed deadline once: %+v", v)
		}
	}
	closed := x.request(t, http.MethodGet, "/api/drafts/expires-first/versions/1", "", nil)
	var result draftrecovery.Result
	if closed.Code != 410 || json.Unmarshal(closed.Body.Bytes(), &result) != nil || result.State != "closed" || !result.AdmittedAt.Equal(original) || !result.Deadline.Equal(original.Add(30*time.Minute)) || result.Bundle != nil {
		t.Fatalf("status refreshed or exposed terminal work: %d %s", closed.Code, closed.Body.String())
	}
	x.create(t, "explicit-new-after-terminal")
	if x.allowance(t) != (openingAllowance{5, 5, 0, 0}) || x.generation.Load() != 6 || x.check.Load() != 6 {
		t.Fatalf("new explicit action did not reuse only released right: %+v %d/%d", x.allowance(t), x.generation.Load(), x.check.Load())
	}
}

func TestDraftOpeningReservationCannotGenerateAfterOlderOutcomeLockCrossesExpiry(t *testing.T) {
	x := newOpeningFixture(t)
	first := x.create(t, "older-unopened")
	var expiry time.Time
	if err := x.f.DB.QueryRow(`UPDATE careeros_pilot_access SET expires_at=clock_timestamp()+interval '1800 milliseconds' WHERE account_id=$1 RETURNING expires_at`, x.f.Principal.AccountID).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	block, err := x.f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback()
	var id string
	if err = block.QueryRow(`SELECT action_id FROM careeros_draft_actions WHERE namespace=$1 AND action_id=$2 FOR UPDATE`, first.Action.Namespace, first.Action.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- x.request(t, http.MethodPost, "/api/drafts", "new-after-wait", x.input) }()
	limit := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err = x.f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND state='active' AND wait_event_type='Lock' AND query LIKE '%FROM careeros_draft_actions WHERE owner=%FOR UPDATE%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(limit) {
			t.Fatal("new reservation did not reach the existing outcome lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for {
		var elapsed bool
		if err = x.f.DB.QueryRow(`SELECT clock_timestamp()>=$1`, expiry).Scan(&elapsed); err != nil {
			t.Fatal(err)
		}
		if elapsed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err = block.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		if w.Code < 400 || x.generation.Load() != 1 || x.check.Load() != 1 {
			t.Fatalf("expired reservation reached generation: status%d generation%d check%d", w.Code, x.generation.Load(), x.check.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("new reservation remained stuck after releasing the old row")
	}
	if v := x.allowance(t); v != (openingAllowance{5, 0, 0, 5}) {
		t.Fatalf("expiry created or consumed a new right: %+v", v)
	}
}

func TestDraftOpeningAllowanceStatusDoesNotShadowOpaqueCreationIdentity(t *testing.T) {
	x := newOpeningFixture(t)
	created := x.create(t, "allowance")
	w := x.request(t, http.MethodGet, "/api/drafts/allowance", "", nil)
	var restored draftrecovery.Result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &restored) != nil || restored.Action != created.Action || restored.BundleDigest != created.BundleDigest || restored.Bundle == nil {
		t.Fatalf("status route shadows a successfully admitted opaque creation key: %d %s", w.Code, w.Body.String())
	}
}
