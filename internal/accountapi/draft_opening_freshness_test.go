package accountapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func waitOpeningLock(t *testing.T, x *openingFixture, query string) {
	t.Helper()
	limit := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := x.f.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND state='active' AND wait_event_type='Lock' AND query LIKE $1)`, query).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		if time.Now().After(limit) {
			t.Fatal("opening request did not reach the intended database lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDraftOpeningAllowanceStatusRefreshesAfterSharedActionWait(t *testing.T) {
	x := newOpeningFixture(t)
	created := x.create(t, "status-shared-wait")
	tx, err := x.f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var admission, deadline time.Time
	if err = tx.QueryRow(`SELECT at-interval '30 minutes',at FROM (SELECT clock_timestamp()+interval '1500 milliseconds' AS at) origin`).Scan(&admission, &deadline); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE ai_fixture_action SET created_at=$3 WHERE namespace=$1 AND id=$2`,
		`UPDATE careeros_draft_actions SET admitted_at=$3::timestamptz,deadline=$4::timestamptz WHERE namespace=$1 AND action_id=$2`,
		`UPDATE careeros_draft_allowance SET admitted_at=$3::timestamptz,deadline=$4::timestamptz WHERE namespace=$1 AND action_id=$2`,
	} {
		args := []any{created.Action.Namespace, created.Action.ID, admission, deadline}
		if query == `UPDATE ai_fixture_action SET created_at=$3 WHERE namespace=$1 AND id=$2` {
			args = args[:3]
		}
		if _, err = tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	block, err := x.f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback()
	var id string
	if err = block.QueryRow(`SELECT id FROM ai_fixture_action WHERE namespace=$1 AND id=$2 FOR UPDATE`, created.Action.Namespace, created.Action.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- x.request(t, http.MethodGet, "/api/draft-allowance", "", nil) }()
	waitOpeningLock(t, x, `%SELECT state,attempt FROM ai_fixture_action%FOR UPDATE%`)
	for {
		var elapsed bool
		if err = x.f.DB.QueryRow(`SELECT clock_timestamp()>=$1`, deadline).Scan(&elapsed); err != nil {
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
		var status openingAllowance
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status != (openingAllowance{5, 0, 0, 5}) {
			t.Fatalf("allowance used pre-shared-lock time: status%d %s", w.Code, w.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("allowance remained blocked after the shared Action was released")
	}
	var released time.Time
	if err = x.f.DB.QueryRow(`SELECT released_at FROM careeros_draft_allowance WHERE namespace=$1 AND action_id=$2`, created.Action.Namespace, created.Action.ID).Scan(&released); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if x.allowance(t) != (openingAllowance{5, 0, 0, 5}) {
			t.Fatal("repeat status changed the released customer outcome")
		}
	}
	var again time.Time
	if err = x.f.DB.QueryRow(`SELECT released_at FROM careeros_draft_allowance WHERE namespace=$1 AND action_id=$2`, created.Action.Namespace, created.Action.ID).Scan(&again); err != nil || !again.Equal(released) || x.generation.Load() != 1 || x.check.Load() != 1 {
		t.Fatalf("status repeated settlement or inference: %v %v/%v callbacks%d/%d", err, released, again, x.generation.Load(), x.check.Load())
	}
}

func TestDraftOpeningNewWitnessRefreshesRestrictiveDecisionAfterOlderReservationWait(t *testing.T) {
	for _, fence := range []string{"cancel", "delete"} {
		t.Run(fence, func(t *testing.T) {
			x := newOpeningFixture(t)
			old := x.create(t, "older-ready")
			block, err := x.f.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer block.Rollback()
			var id string
			if err = block.QueryRow(`SELECT action_id FROM careeros_draft_actions WHERE namespace=$1 AND action_id=$2 FOR UPDATE`, old.Action.Namespace, old.Action.ID).Scan(&id); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- x.request(t, http.MethodPost, "/api/drafts", "new-after-fence", x.input) }()
			waitOpeningLock(t, x, `%FROM careeros_draft_actions WHERE owner=%FOR UPDATE%`)
			if fence == "cancel" {
				x.cancelled.Store(true)
			} else {
				x.deleted.Store(true)
			}
			if err = block.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case w := <-done:
				if w.Code < 400 || x.generation.Load() != 1 || x.check.Load() != 1 {
					t.Fatalf("winning restrictive decision allowed a new callback: status%d generation%d check%d", w.Code, x.generation.Load(), x.check.Load())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("new creation remained blocked after the older reservation was released")
			}
			var witnesses int
			if err = x.f.DB.QueryRow(`SELECT count(*) FROM careeros_draft_actions WHERE owner=$1 AND creation_key='new-after-fence'`, x.f.Principal.AccountID).Scan(&witnesses); err != nil || witnesses != 0 {
				t.Fatalf("winning terminal decision retained a new generation witness: %d %v", witnesses, err)
			}
			x.cancelled.Store(false)
			x.deleted.Store(false)
			for range 2 {
				w := x.request(t, http.MethodPost, "/api/drafts", "new-after-fence", x.input)
				if w.Code < 400 || x.generation.Load() != 1 || x.check.Load() != 1 {
					t.Fatalf("restored flags revived the already fenced Action: status%d callbacks%d/%d", w.Code, x.generation.Load(), x.check.Load())
				}
			}
		})
	}
}
