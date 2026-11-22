package accountapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
	"professional-information-repo/internal/draftrecovery"
)

func TestDraftOpeningCustomerOutcomeNeverReleasesUnknownProviderState(t *testing.T) {
	for _, outcome := range []string{"delivered", "terminal"} {
		t.Run(outcome, func(t *testing.T) {
			x := newOpeningFixture(t)
			const key = "separate-provider-hold"
			ctx := context.Background()
			var before aicontrol.ActionStatus
			scopes := map[string]aicontrol.ScopeStatus{}
			calls := 0
			x.actions.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("synthetic unknown provider outcome")
			})
			x.router = accountapi.NewControlledDraftRecovery(x.f.Service, x.store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
				x.generation.Add(1)
				root, _ := json.Marshal(g.Input)
				session, err := x.actions.Open(ctx, x.f.Principal, key, "application_draft", root)
				if err != nil {
					t.Fatal(err)
				}
				stage, _ := aidiagnostics.Start(aidispatch.WithWorkflowRoot(ctx, "application_draft", root), "field_validation", "jev-rubric-v1/policy-v1")
				request, _ := http.NewRequest(http.MethodPost, "https://api.typesafe.ai/v1/systemone", strings.NewReader(`{"model":"jev-1.13.0","state":{"field":"job_posting","submission":"synthetic"}}`))
				if _, err = session.Send(stage, "typesafe", request); err == nil {
					t.Fatal("fixture did not reach unknown provider state")
				}
				before, err = x.actions.Status(ctx, x.f.Principal, session.ID())
				if err != nil || len(before.Dispatches) != 1 || before.Dispatches[0].Remote != "possibly_active" {
					t.Fatalf("unknown fixture status: %+v %v", before, err)
				}
				for _, scope := range []string{"global", "provider:typesafe", "account:" + x.f.Principal.AccountID} {
					v, err := x.actions.ScopeStatus(ctx, x.f.Principal, scope)
					if err != nil || v.Counted != 60 || v.Occupied != 1 {
						t.Fatalf("original SYN obligation %s: %+v %v", scope, v, err)
					}
					scopes[scope] = v
				}
				return completeRecoveryBundle(g), nil
			})
			created := x.create(t, key)
			receipt := x.receipt(t, key)
			want := openingAllowance{5, 0, 1, 4}
			if outcome == "terminal" {
				x.cancelled.Store(true)
				want = openingAllowance{5, 0, 0, 5}
			}
			w := x.confirm(t, key, receipt)
			if outcome == "delivered" && w.Code != 200 || outcome == "terminal" && w.Code < 400 {
				t.Fatalf("customer outcome: %d %s", w.Code, w.Body.String())
			}
			if outcome == "delivered" {
				x.deleted.Store(true)
				x.request(t, http.MethodGet, "/api/drafts/"+key+"/versions/1", "", nil)
			}
			if err := x.store.Cleanup(ctx, x.f.Principal, key); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				x.confirm(t, key, receipt)
			}
			if x.allowance(t) != want || calls != 1 || x.generation.Load() != 1 || x.check.Load() != 1 {
				t.Fatalf("customer replay changed calls/rights: %+v calls%d generation%d checks%d", x.allowance(t), calls, x.generation.Load(), x.check.Load())
			}
			after, err := x.actions.Status(ctx, x.f.Principal, created.Action.ID)
			if err != nil || !reflect.DeepEqual(before.Dispatches, after.Dispatches) {
				t.Fatalf("customer outcome changed provider dispatch: before%+v after%+v err%v", before.Dispatches, after.Dispatches, err)
			}
			for scope, original := range scopes {
				v, err := x.actions.ScopeStatus(ctx, x.f.Principal, scope)
				if err != nil || v != original {
					t.Fatalf("customer outcome released %s: before%+v after%+v err%v", scope, original, v, err)
				}
			}
		})
	}
}

func TestDraftOpeningLedgerRolesCannotForgeCustomerSettlementOrReadReceipts(t *testing.T) {
	x := newOpeningFixture(t)
	created := x.create(t, "role-bound-opening")
	var runtimeRole, reviewerRole string
	for _, row := range []struct {
		pattern string
		target  *string
	}{{"ai_fixture_rt_%", &runtimeRole}, {"ai_fixture_review_%", &reviewerRole}} {
		if err := x.f.DB.QueryRow(`SELECT rolname FROM pg_roles WHERE rolname LIKE $1 AND has_table_privilege(rolname,'ai_fixture_action','SELECT')`, row.pattern).Scan(row.target); err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{runtimeRole, reviewerRole} {
		u, _ := url.Parse(x.f.DSN)
		u.User = url.User(role)
		db, err := sql.Open("pgx", u.String())
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var current string
		if db.QueryRow(`SELECT current_user`).Scan(&current) != nil || current != role {
			t.Fatal("not an independently authenticated restricted role")
		}
		for _, query := range []string{
			`SELECT state FROM careeros_draft_allowance`,
			`SELECT receipt_id FROM careeros_draft_opening_receipts`,
			`UPDATE careeros_draft_allowance SET state='delivered',confirmed_at=clock_timestamp()`,
			`UPDATE careeros_draft_allowance SET state='released',released_at=clock_timestamp()`,
			`DELETE FROM careeros_draft_opening_receipts`,
		} {
			if _, err = db.Exec(query); err == nil {
				t.Fatalf("restricted role acquired opening authority: role%s query%s", role, query)
			}
		}
		if role == runtimeRole {
			for _, field := range []string{"customer_effect=true", "customer_reserved=false", "state='delivered'"} {
				if _, err = db.Exec(`UPDATE ai_fixture_action SET `+field+` WHERE namespace=$1 AND id=$2`, created.Action.Namespace, created.Action.ID); err == nil {
					t.Fatalf("runtime bypassed raw SQL settlement fence: %s", field)
				}
			}
		}
	}
	if x.allowance(t) != (openingAllowance{5, 1, 0, 4}) {
		t.Fatal("failed raw writes changed customer rights")
	}
}
