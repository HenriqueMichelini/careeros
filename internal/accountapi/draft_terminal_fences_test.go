package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
)

func TestManagedTerminalEveryFenceCompetesWithOpeningWithoutChangingProviderExpense(t *testing.T) {
	for _, fence := range []string{"original_deadline", "grant_expiry", "revocation", "deletion", "source"} {
		for _, openingFirst := range []bool{false, true} {
			name := "fence_first"
			if openingFirst {
				name = "opening_first"
			}
			t.Run(fence+"/"+name, func(t *testing.T) {
				var deleted atomic.Bool
				cfg := draftrecovery.Config{Decisions: draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
					var expiry time.Time
					var epoch uint64
					err := a.Transaction.QueryRowContext(ctx, `SELECT expires_at,(extract(epoch FROM activated_at)*1000000)::bigint FROM careeros_pilot_access WHERE account_id=$1`, a.Principal.AccountID).Scan(&expiry, &epoch)
					return draftrecovery.Decision{Generate: true, Publish: true, Read: true, Deleted: deleted.Load(), GrantExpiresAt: expiry, Epoch: epoch}, err
				})}
				f, h, provider, _, router, input := configuredTerminalDraftFixture(t, cfg)
				body, _ := json.Marshal(input)
				const key = "managed-terminal-fence"
				created := qualificationRequest(t, f, router, "/api/drafts", key, "", body)
				var ready draftrecovery.Result
				if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &ready) != nil {
					t.Fatalf("creation %d %s", created.Code, created.Body.String())
				}
				issued := openingRequest(t, f, router, "POST", "/api/drafts/"+key+"/versions/1/opening", struct{}{})
				var receipt draftrecovery.OpeningReceipt
				if issued.Code != 200 || json.Unmarshal(issued.Body.Bytes(), &receipt) != nil {
					t.Fatalf("receipt %d %s", issued.Code, issued.Body.String())
				}
				confirm := func() *httptest.ResponseRecorder {
					return openingRequest(t, f, router, "POST", "/api/drafts/"+key+"/versions/1/opening/confirm", draftrecovery.OpeningConfirmation{Receipt: receipt.Receipt, BundleDigest: receipt.BundleDigest, Manifest: receipt.Manifest})
				}
				if openingFirst {
					if w := confirm(); w.Code != 200 {
						t.Fatalf("opening first %d %s", w.Code, w.Body.String())
					}
				}
				switch fence {
				case "original_deadline":
					tx, err := f.DB.Begin()
					if err != nil {
						t.Fatal(err)
					}
					defer tx.Rollback()
					var original time.Time
					if err = tx.QueryRow(`SELECT clock_timestamp()-interval '31 minutes'`).Scan(&original); err != nil {
						t.Fatal(err)
					}
					for _, q := range []string{`UPDATE ai_fixture_action SET created_at=$3 WHERE namespace=$1 AND id=$2`, `UPDATE careeros_draft_actions SET admitted_at=$3::timestamptz,deadline=$3::timestamptz+interval '30 minutes' WHERE namespace=$1 AND action_id=$2`, `UPDATE careeros_draft_allowance SET admitted_at=$3::timestamptz,deadline=$3::timestamptz+interval '30 minutes' WHERE namespace=$1 AND action_id=$2`} {
						if _, err = tx.Exec(q, ready.Action.Namespace, ready.Action.ID, original); err != nil {
							t.Fatal(err)
						}
					}
					if err = tx.Commit(); err != nil {
						t.Fatal(err)
					}
				case "grant_expiry":
					if _, err := f.DB.Exec(`UPDATE careeros_pilot_access SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id=$1`, f.Principal.AccountID); err != nil {
						t.Fatal(err)
					}
				case "revocation":
					// Exact real pilot revocation state, independently of the injected adapter.
					if _, err := f.DB.Exec(`UPDATE careeros_pilot_access SET invitation_revoked=true WHERE account_id=$1`, f.Principal.AccountID); err != nil {
						t.Fatal(err)
					}
				case "deletion":
					deleted.Store(true)
				case "source":
					var doc profiledocument.Document
					if json.Unmarshal(f.Document, &doc) != nil {
						t.Fatal("fixture Profile")
					}
					old := doc.Revision
					doc.Revision++
					for i := range doc.Facts {
						if doc.Facts[i].Owner.ID == doc.ID {
							doc.Facts[i].Owner.Revision = doc.Revision
						}
					}
					raw, _ := json.Marshal(doc)
					if _, err := f.Store.SaveProfile(context.Background(), f.Principal, "terminal-source", old, raw); err != nil {
						t.Fatal(err)
					}
				}
				w := confirm()
				if !openingFirst && w.Code < 400 {
					t.Fatalf("fence lost %d %s", w.Code, w.Body.String())
				}
				want := 0
				if openingFirst {
					want = 1
				}
				for range 2 {
					confirm()
					status := openingRequest(t, f, router, "GET", "/api/drafts/"+key, nil)
					if !openingFirst {
						var closed draftrecovery.Result
						if status.Code != 410 || json.Unmarshal(status.Body.Bytes(), &closed) != nil || closed.Bundle != nil || closed.Delivered {
							t.Fatalf("terminal recovery %d %s", status.Code, status.Body.String())
						}
					}
					var reserved, consumed int
					if err := f.DB.QueryRow(`SELECT count(*) FILTER(WHERE customer_reserved),count(*) FILTER(WHERE customer_effect) FROM ai_fixture_action WHERE owner=$1`, f.Principal.AccountID).Scan(&reserved, &consumed); err != nil || reserved != 0 || consumed != want {
						t.Fatalf("effect reserved%d consumed%d want%d err%v", reserved, consumed, want, err)
					}
				}
				money, err := h.ScopeStatus(context.Background(), f.Principal, "global")
				if err != nil || money.Counted != 35344 || money.Occupied != 0 || len(provider.Calls) != 4 {
					t.Fatalf("customer fence changed provider accounting %+v %v sends%v", money, err, provider.Calls)
				}
			})
		}
	}
}

func TestManagedTerminalCancelRejectsForgedIdentityAndUncomposedAliases(t *testing.T) {
	f, _, provider, _, router, input := terminalDraftFixture(t)
	body, _ := json.Marshal(input)
	created := qualificationRequest(t, f, router, "/api/drafts", "terminal-exact-owner", "", body)
	var ready draftrecovery.Result
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &ready) != nil {
		t.Fatal(created.Body.String())
	}
	for _, kind := range []string{"owner", "id", "namespace", "workflow", "extra"} {
		t.Run(kind, func(t *testing.T) {
			bad := ready.Action
			switch kind {
			case "owner":
				bad.Owner = "foreign"
			case "id":
				bad.ID = "other"
			case "namespace":
				bad.Namespace = "other"
			case "workflow":
				bad.Workflow = "qualification_gaps"
			}
			command := map[string]any{"action": bad}
			if kind == "extra" {
				command["complete"] = true
			}
			w := openingRequest(t, f, router, "POST", "/api/drafts/terminal-exact-owner/cancel", command)
			if w.Code < 400 {
				t.Fatalf("forged %d %s", w.Code, w.Body.String())
			}
		})
	}
	foreign := qualificationParticipant(t, f, "foreign-terminal-owner")
	if w := openingRequest(t, foreign, router, "POST", "/api/drafts/terminal-exact-owner/cancel", map[string]any{"action": ready.Action}); w.Code < 400 {
		t.Fatal("foreign canceled owner")
	}
	w := openingRequest(t, f, router, "POST", "/.netlify/functions/account/api/drafts/terminal-exact-owner/cancel", map[string]any{"action": ready.Action})
	if w.Code != 200 || len(provider.Calls) != 4 {
		t.Fatalf("exact account alias %d %s sends%v", w.Code, w.Body.String(), provider.Calls)
	}
	_, _, _, _, uncomposed, _ := managedDraftFixture(t)
	if w := openingRequest(t, f, uncomposed, "POST", "/api/drafts/terminal-exact-owner/cancel", map[string]any{"action": ready.Action}); w.Code < 400 {
		t.Fatal("ordinary managed constructor exposed terminal protocol")
	}
}

func TestManagedTerminalOlderPurposeRowsCannotReviveCanceledSharedAction(t *testing.T) {
	f, h, provider, _, router, input := terminalDraftFixture(t)
	raw, _ := json.Marshal(input)
	const key = "terminal-purpose-restore"
	w := qualificationRequest(t, f, router, "/api/drafts", key, "", raw)
	var ready draftrecovery.Result
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &ready) != nil {
		t.Fatal(w.Body.String())
	}
	conn, err := f.DB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, table := range []string{"careeros_draft_actions", "careeros_draft_allowance"} {
		if _, err = conn.ExecContext(context.Background(), `CREATE TEMP TABLE terminal_before_`+table+` AS TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	canceled := openingRequest(t, f, router, "POST", "/api/drafts/"+key+"/cancel", map[string]any{"action": ready.Action})
	if canceled.Code != 200 {
		t.Fatal(canceled.Body.String())
	}
	// Only purpose rows are restored. The independently retained shared terminal
	// Action and admission guard remain current; this is not hosted restore proof.
	for _, q := range []string{`UPDATE careeros_draft_actions a SET state=b.state,cleanup_queued=b.cleanup_queued FROM terminal_before_careeros_draft_actions b WHERE a.namespace=b.namespace AND a.action_id=b.action_id`, `UPDATE careeros_draft_allowance a SET state=b.state,confirmed_at=b.confirmed_at,released_at=b.released_at FROM terminal_before_careeros_draft_allowance b WHERE a.namespace=b.namespace AND a.action_id=b.action_id`} {
		if _, err = conn.ExecContext(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		status := openingRequest(t, f, router, "GET", "/api/drafts/"+key+"/versions/1", nil)
		var closed draftrecovery.Result
		if status.Code != 410 || json.Unmarshal(status.Body.Bytes(), &closed) != nil || closed.State != "closed" || closed.Delivered || closed.Bundle != nil || closed.Opening == nil || closed.Opening.State != "released" {
			t.Fatalf("older rows revived %d %s", status.Code, status.Body.String())
		}
		late := qualificationRequest(t, f, router, "/api/drafts", key, "late-after-restore", raw)
		if late.Code != 403 {
			t.Fatalf("older-row Retry %d %s", late.Code, late.Body.String())
		}
	}
	money, err := h.ScopeStatus(context.Background(), f.Principal, "global")
	if err != nil || money.Counted != 35344 || money.Occupied != 0 || len(provider.Calls) != 4 {
		t.Fatalf("restore changed provider obligation %+v %v", money, err)
	}
	allowance := openingRequest(t, f, router, "GET", "/api/draft-allowance", nil)
	var rights openingAllowance
	if allowance.Code != 200 || json.Unmarshal(allowance.Body.Bytes(), &rights) != nil || rights != (openingAllowance{5, 0, 0, 5}) {
		t.Fatalf("restore customer outcome %d %s", allowance.Code, allowance.Body.String())
	}
}
