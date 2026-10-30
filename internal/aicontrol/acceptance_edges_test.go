package aicontrol

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestA04_IndependentlyLimitedScopes(t *testing.T) {
	for _, scope := range []string{"global", "account", "provider:openai"} {
		for _, resource := range []string{"money", "capacity"} {
			t.Run(scope+"/"+resource, func(t *testing.T) {
				f := setup(t, func(c *SyntheticConfig) { c.Limit, c.Capacity = 1000, 10 })
				// Establish the account scope before the independent processes race.
				f.open(t, "scope_setup")
				key := scope
				if scope == "account" {
					key = "account:" + f.p.AccountID
				}
				query := `UPDATE ai_fixture_scope SET limit_amount=100 WHERE scope=$1`
				if resource == "capacity" {
					query = `UPDATE ai_fixture_scope SET capacity=1 WHERE scope=$1`
				}
				if _, err := f.db.Exec(query, key); err != nil {
					t.Fatal(err)
				}
				runPair(t, f.child("independent_one", "send"), f.child("independent_two", "send"))
				var calls, dispatches int
				if err := f.db.QueryRow(`SELECT count(*) FROM ai_test_invocations`).Scan(&calls); err != nil {
					t.Fatal(err)
				}
				if err := f.db.QueryRow(`SELECT count(*) FROM ai_fixture_dispatch`).Scan(&dispatches); err != nil {
					t.Fatal(err)
				}
				if calls != 1 || dispatches != 1 {
					t.Fatalf("independent %s/%s race admitted calls=%d dispatches=%d", scope, resource, calls, dispatches)
				}
				rows, err := f.db.Query(`SELECT counted,occupied FROM ai_fixture_scope WHERE scope IN ('global',$1,'provider:openai')`, "account:"+f.p.AccountID)
				if err != nil {
					t.Fatal(err)
				}
				defer rows.Close()
				for rows.Next() {
					var amount, occupied int
					if err := rows.Scan(&amount, &occupied); err != nil || amount != 60 || occupied != 1 {
						t.Fatalf("partially committed allocation: amount=%d occupied=%d err=%v", amount, occupied, err)
					}
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func availableCustomerUnits(t *testing.T, f *fixture) int64 {
	t.Helper()
	var units, used int64
	if err := f.db.QueryRow(`SELECT units FROM ai_fixture_policy WHERE namespace=$1`, f.cfg.Namespace).Scan(&units); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT count(*) FROM ai_fixture_action WHERE namespace=$1 AND owner=$2 AND (customer_reserved OR customer_effect)`, f.cfg.Namespace, f.p.AccountID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	return units - used
}

func TestA21_TimeoutCancelAndLateEventsPreserveThreeRights(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "customer_timeout")
	if got := availableCustomerUnits(t, f); got != 3 {
		t.Fatal(got)
	}
	if err := s.ReserveCustomer(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := availableCustomerUnits(t, f); got != 2 {
		t.Fatal(got)
	}
	f.h.SetResponder(func(context.Context, string, *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	resp, err := send(s)
	closeResp(resp)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("timeout fixture did not reach the provider", err)
	}
	reviewer := f.h.SyntheticReviewer()
	for i := 0; i < 2; i++ {
		if err := reviewer.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "cancel"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []CustomerEvent{
		{Kind: "candidate", ResultRef: "late", Complete: true, Recoverable: true},
		{Kind: "open", ResultRef: "late", Complete: true, Recoverable: true},
		{Kind: "accept"}, {Kind: "export"},
	} {
		_ = reviewer.Customer(context.Background(), f.p, s.ID(), event)
	}
	status := f.status(t, s)
	if got := availableCustomerUnits(t, f); got != 3 || status.CustomerReserved || status.CustomerEffect || status.State != "canceled" {
		t.Fatalf("late event revived/debited cancellation: units=%d status=%+v", got, status)
	}
	if scope := f.scope(t); scope.Counted != 60 || scope.Occupied != 1 {
		t.Fatal("customer release freed supplier obligation", scope)
	}
}

func TestA22_CancelAndDeliveryRaceWithLostAcknowledgment(t *testing.T) {
	for _, first := range []string{"cancel", "open"} {
		t.Run(first+"_wins", func(t *testing.T) {
			f := setup(t, nil)
			s := f.open(t, "ordered_delivery")
			if err := s.ReserveCustomer(context.Background()); err != nil {
				t.Fatal(err)
			}
			r := f.h.SyntheticReviewer()
			candidate := CustomerEvent{Kind: "candidate", ResultRef: "recoverable_fake", Complete: true, Recoverable: true}
			if err := r.Customer(context.Background(), f.p, s.ID(), candidate); err != nil {
				t.Fatal(err)
			}
			locked, err := f.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer locked.Rollback()
			if _, err = locked.Exec(`SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, f.p.AccountID); err != nil {
				t.Fatal(err)
			}
			// Queue both real SQL commands behind the same owner lock. The
			// first waiter must commit before the second can observe its state.
			var wg sync.WaitGroup
			start := func(kind string) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					event := CustomerEvent{Kind: kind, ResultRef: candidate.ResultRef, Complete: true, Recoverable: true}
					_ = r.Customer(context.Background(), f.p, s.ID(), event)
				}()
			}
			start(first)
			waitForBlockedCustomer(t, f, 1)
			second := "cancel"
			if first == "cancel" {
				second = "open"
			}
			start(second)
			waitForBlockedCustomer(t, f, 2)
			if err := locked.Commit(); err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			// The successful delivery acknowledgment is deliberately discarded;
			// status retrieval and duplicate confirmation must not regenerate.
			for _, kind := range []string{"open", "open", "accept", "export", "cancel"} {
				_ = r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: kind, ResultRef: candidate.ResultRef, Complete: true, Recoverable: true})
			}
			want, effect := int64(3), false
			if first == "open" {
				want, effect = 2, true
			}
			status := f.status(t, s)
			if got := availableCustomerUnits(t, f); got != want || status.CustomerEffect != effect || status.CustomerReserved || f.calls.Load() != 0 {
				t.Fatalf("wrong ordered single effect: units=%d want=%d status=%+v", got, want, status)
			}
		})
	}
}

func waitForBlockedCustomer(t *testing.T, f *fixture, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var blocked int
		if err := f.db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%careeros_accounts%FOR UPDATE%'`).Scan(&blocked); err == nil && blocked >= count {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("customer command did not wait at the actual PostgreSQL owner lock")
}

func TestA24_ClosedUnconfirmedActionCannotAcquireLateCharge(t *testing.T) {
	f := setup(t, nil)
	s := f.open(t, "terminal_unconfirmed")
	if err := s.ReserveCustomer(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := f.h.SyntheticReviewer()
	deadline := time.Now().Add(-time.Minute) // independent fixture customer clock
	if err := r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: "close", Deadline: &deadline}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"close", "candidate", "open", "accept", "export"} {
		_ = r.Customer(context.Background(), f.p, s.ID(), CustomerEvent{Kind: kind, Deadline: &deadline, ResultRef: "late_fake", Complete: true, Recoverable: true})
	}
	status := f.status(t, s)
	if got := availableCustomerUnits(t, f); got != 3 || status.State != "closed" || status.CustomerEffect || status.CustomerReserved {
		t.Fatalf("terminal unconfirmed closure charged late: units=%d status=%+v", got, status)
	}
}
