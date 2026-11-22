package accountapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
)

func deliveredReview(t *testing.T, r *recoveryFixture, key string) draftrecovery.Result {
	t.Helper()
	created := r.create(t, key)
	r.authority(t, `UPDATE controlled_draft_test_authority SET delivered_action=$1 WHERE owner=$2`, created.Action.ID, r.f.Principal.AccountID)
	return created
}

func humanCommand(r draftrecovery.Result, key, artifact, operation string, revision int64) draftrecovery.ReviewCommand {
	return draftrecovery.ReviewCommand{DecisionKey: key, ExpectedRevision: revision, BundleDigest: r.BundleDigest, Artifact: artifact, Operation: operation}
}

func TestDurableDraftReviewRejectsWrongAuthorityBindingAndLateDecisions(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	created := r.create(t, "authority-review")
	path := "/api/drafts/authority-review/versions/1/review"
	command := humanCommand(created, "review-summary", "summary", "review", 0)
	if w := r.request(t, "POST", path, "", command); w.Code != 403 {
		t.Fatalf("readiness became delivery: %d %s", w.Code, w.Body.String())
	}
	r.authority(t, `UPDATE controlled_draft_test_authority SET delivered_action=$1 WHERE owner=$2`, created.Action.ID, r.f.Principal.AccountID)
	before, err := r.f.Store.ReadProfile(context.Background(), r.f.Principal, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, path string
		command    draftrecovery.ReviewCommand
		status     int
	}{
		{"wrong-version", "/api/drafts/authority-review/versions/2/review", command, 404},
		{"wrong-digest", path, humanCommand(draftrecovery.Result{BundleDigest: fmt.Sprintf("%064d", 0)}, "wrong-digest", "summary", "review", 0), 409},
		{"wrong-artifact", path, humanCommand(created, "wrong-artifact", "email", "review", 0), 422},
		{"accept-before-review", path, humanCommand(created, "unreviewed", "summary", "accept", 0), 409},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := r.request(t, "POST", c.path, "", c.command)
			if w.Code != c.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	w := r.request(t, "POST", path, "", command)
	if w.Code != 200 {
		t.Fatalf("review %d %s", w.Code, w.Body.String())
	}
	var reviewed draftrecovery.Result
	json.Unmarshal(w.Body.Bytes(), &reviewed)
	if reviewed.Review == nil || reviewed.Review.Revision != 1 {
		t.Fatal("missing review", w.Body.String())
	}
	changed := command
	changed.Artifact = "resume"
	if w = r.request(t, "POST", path, "", changed); w.Code != 409 {
		t.Fatalf("changed key accepted: %d", w.Code)
	}
	if w = r.request(t, "POST", path, "", humanCommand(created, "late-other-tab", "resume", "review", 0)); w.Code != 409 {
		t.Fatalf("late revision accepted: %d", w.Code)
	}
	for _, denial := range []struct {
		name, query string
		status      int
	}{
		{"read-denied", `UPDATE controlled_draft_test_authority SET allow_read=false`, 403},
		{"session-expired", `UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second'`, 401},
	} {
		r.authority(t, denial.query)
		w = r.request(t, "POST", "/.netlify/functions/account"+path, "", command)
		if w.Code != denial.status {
			t.Fatalf("duplicate %s: %d %s", denial.name, w.Code, w.Body.String())
		}
		if w.Body.String() == "" {
			t.Fatal("denial has no truthful status")
		}
		r.authority(t, `UPDATE controlled_draft_test_authority SET allow_read=true`)
		r.authority(t, `UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '1 hour'`)
	}
	w = r.request(t, "GET", "/api/drafts/authority-review/versions/1", "", nil)
	var recovered draftrecovery.Result
	json.Unmarshal(w.Body.Bytes(), &recovered)
	if w.Code != 200 || !reflect.DeepEqual(reviewed.Bundle, recovered.Bundle) || recovered.Review.Revision != 1 {
		t.Fatalf("denial changed decision %d %s", w.Code, w.Body.String())
	}
	after, err := r.f.Store.ReadProfile(context.Background(), r.f.Principal, "")
	if err != nil || !reflect.DeepEqual(before.Document, after.Document) {
		t.Fatal("review saved Profile", err)
	}
	start, err := r.f.Service.BeginLogin(context.Background(), "google", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	foreign, err := r.f.Service.CompleteLogin(context.Background(), "google", u.Query().Get("state"), "foreign-review", start.BindingToken)
	if err != nil {
		t.Fatal(err)
	}
	r.f.Login = foreign
	if w = r.request(t, "POST", path, "", command); w.Code != 404 {
		t.Fatalf("foreign review exposed source: %d %s", w.Code, w.Body.String())
	}
	if r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatal("review inferred", r.sends.Load(), r.checks.Load())
	}
}

func TestDurableDraftReviewConcurrentVersionDecisionHasOneWinner(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	created := deliveredReview(t, r, "concurrent-review")
	path := "/api/drafts/concurrent-review/versions/1/review"
	var wait sync.WaitGroup
	codes := make(chan int, 2)
	for i, artifact := range []string{"summary", "resume"} {
		wait.Add(1)
		go func(i int, artifact string) {
			defer wait.Done()
			codes <- r.request(t, "POST", path, "", humanCommand(created, fmt.Sprintf("tab-%d", i), artifact, "review", 0)).Code
		}(i, artifact)
	}
	wait.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("revision race", counts)
	}
	var result draftrecovery.Result
	w := r.request(t, "GET", "/api/drafts/concurrent-review/versions/1", "", nil)
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result.Review == nil || result.Review.Revision != 1 {
		t.Fatal("race changed wrong version", w.Body.String())
	}
}

func TestDurableDraftReviewRechecksExpiryAfterCommitWait(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	created := deliveredReview(t, r, "expiry-review")
	r.authority(t, `UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '200 milliseconds'`)
	r.fault = func(stage string) error {
		if stage == "before_review_commit" {
			time.Sleep(250 * time.Millisecond)
		}
		return nil
	}
	w := r.request(t, "POST", "/api/drafts/expiry-review/versions/1/review", "", humanCommand(created, "expired-after-wait", "summary", "review", 0))
	if w.Code != 401 {
		t.Fatalf("expired commit %d %s", w.Code, w.Body.String())
	}
	r.fault = nil
	r.authority(t, `UPDATE careeros_sessions SET expires_at=clock_timestamp()+interval '1 hour'`)
	var result draftrecovery.Result
	w = r.request(t, "GET", "/api/drafts/expiry-review/versions/1", "", nil)
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result.Review.Revision != 0 {
		t.Fatal("expired choice committed", w.Body.String())
	}
}

func TestDurableDraftReviewHistoricalRightsAndPurposeDeletion(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	created := deliveredReview(t, r, "historical-review")
	scopeBefore, err := r.actions.ScopeStatus(context.Background(), r.f.Principal, "global")
	if err != nil {
		t.Fatal(err)
	}
	var doc profiledocument.Document
	json.Unmarshal(r.f.Document, &doc)
	expected := doc.Revision
	doc.Revision++
	for i := range doc.Facts {
		if doc.Facts[i].Owner.ID == doc.ID {
			doc.Facts[i].Owner.Revision = doc.Revision
		}
	}
	raw, _ := json.Marshal(doc)
	if _, err = r.f.Store.SaveProfile(context.Background(), r.f.Principal, "historical-profile-change", expected, raw); err != nil {
		t.Fatal(err)
	}
	path := "/api/drafts/historical-review/versions/1/review"
	for i, artifact := range []string{"summary", "resume", "cover", "answers"} {
		for j, operation := range []string{"review", "accept"} {
			w := r.request(t, "POST", path, "", humanCommand(created, artifact+"-"+operation, artifact, operation, int64(2*i+j)))
			if w.Code != 200 {
				t.Fatalf("historical decision %d %s", w.Code, w.Body.String())
			}
		}
	}
	r.authority(t, `UPDATE controlled_draft_test_authority SET allow_generate=false,allow_publish=false,grant_end=clock_timestamp()-interval '29 days'`)
	if _, err = r.f.DB.Exec(`UPDATE careeros_draft_actions SET grant_expires_at=clock_timestamp()-interval '29 days'`); err != nil {
		t.Fatal(err)
	}
	w := r.request(t, "GET", "/api/drafts/accepted", "", nil)
	var result draftrecovery.Result
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || !result.Historical || !result.Review.Accepted || !reflect.DeepEqual(result.Bundle, created.Bundle) {
		t.Fatalf("lost historical read rights %d %s", w.Code, w.Body.String())
	}
	r.authority(t, `UPDATE controlled_draft_test_authority SET deleted=true,epoch=epoch+1`)
	w = r.request(t, "GET", "/api/drafts/accepted", "", nil)
	var closed draftrecovery.Result
	json.Unmarshal(w.Body.Bytes(), &closed)
	if w.Code != 410 || closed.Bundle != nil || closed.Review != nil {
		t.Fatalf("deleted accepted content leaked: %d %s", w.Code, w.Body.String())
	}
	if w = r.request(t, "POST", path, "", humanCommand(created, "summary-review", "summary", "review", 0)); w.Code != 403 {
		t.Fatalf("deleted replay allowed %d", w.Code)
	}
	if err = r.store.Cleanup(context.Background(), r.f.Principal, "historical-review"); err != nil {
		t.Fatal(err)
	}
	r.authority(t, `UPDATE controlled_draft_test_authority SET deleted=false,allow_read=true,epoch=epoch+1`)
	w = r.request(t, "GET", "/api/drafts/historical-review/versions/1", "", nil)
	json.Unmarshal(w.Body.Bytes(), &closed)
	if w.Code != 410 || closed.Bundle != nil || closed.Review != nil {
		t.Fatalf("deleted state revived: %d %s", w.Code, w.Body.String())
	}
	scopeAfter, err := r.actions.ScopeStatus(context.Background(), r.f.Principal, "global")
	if err != nil || !reflect.DeepEqual(scopeBefore, scopeAfter) {
		t.Fatal("purpose deletion changed financial authority", err, scopeBefore, scopeAfter)
	}
	if r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatal("historical review inferred")
	}
}

func TestDurableDraftReviewAliasFinalReadFenceWithholdsCommittedDecision(t *testing.T) {
	var finalDenied atomic.Bool
	r := newRecoveryFixture(t, func(r *recoveryFixture) {
		base := r.config.Decisions
		r.config.Decisions = draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
			d, e := base.Decide(ctx, a)
			if finalDenied.Load() {
				d.Read = false
			}
			return d, e
		})
	})
	created := deliveredReview(t, r, "final-review")
	command := humanCommand(created, "summary-review", "summary", "review", 0)
	for _, duplicate := range []bool{false, true} {
		r.fault = func(stage string) error {
			if stage == "before_result_write" {
				finalDenied.Store(true)
			}
			return nil
		}
		w := r.request(t, "POST", "/.netlify/functions/account/api/drafts/final-review/versions/1/review", "", command)
		if w.Code != 403 || strings.Contains(w.Body.String(), "Java") || strings.Contains(w.Body.String(), "artifacts") {
			t.Fatalf("final read denial leaked duplicate=%v: %d %s", duplicate, w.Code, w.Body.String())
		}
		finalDenied.Store(false)
		r.fault = nil
		w = r.request(t, "GET", "/api/drafts/final-review/versions/1", "", nil)
		var result draftrecovery.Result
		json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != 200 || result.Review.Revision != 1 || !reflect.DeepEqual(result.Bundle, created.Bundle) {
			t.Fatalf("committed denied reply lost decision: %d %s", w.Code, w.Body.String())
		}
	}
	if r.sends.Load() != 1 || r.checks.Load() != 1 {
		t.Fatal("read fence regenerated")
	}
}

func TestDurableDraftReviewObservedTerminalFenceCannotRevive(t *testing.T) {
	for _, c := range []struct {
		name                      string
		duplicate, final, closure bool
	}{
		{"new-deletion", false, false, false},
		{"duplicate-deletion", true, false, false},
		{"new-final-deletion", false, true, false},
		{"duplicate-final-deletion", true, true, false},
		{"closure-before-opening", false, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var calls, trip atomic.Int32
			r := newRecoveryFixture(t, func(r *recoveryFixture) {
				base := r.config.Decisions
				r.config.Decisions = draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
					d, e := base.Decide(ctx, a)
					if n := trip.Load(); n != 0 && calls.Add(1) >= n {
						d.Deleted = true
						d.Epoch++
					}
					return d, e
				})
			})
			created := r.create(t, "terminal-review")
			if !c.closure {
				r.authority(t, `UPDATE controlled_draft_test_authority SET delivered_action=$1`, created.Action.ID)
			}
			command := humanCommand(created, "one-review", "summary", "review", 0)
			path := "/.netlify/functions/account/api/drafts/terminal-review/versions/1/review"
			if c.duplicate {
				if w := r.request(t, "POST", path, "", command); w.Code != 200 {
					t.Fatalf("first decision %d %s", w.Code, w.Body.String())
				}
			}
			if c.final {
				calls.Store(0)
				trip.Store(2)
			} else if c.closure {
				r.authority(t, `UPDATE controlled_draft_test_authority SET cancelled=true,epoch=epoch+1`)
			} else {
				r.authority(t, `UPDATE controlled_draft_test_authority SET deleted=true,epoch=epoch+1`)
			}
			w := r.request(t, "POST", path, "", command)
			if w.Code != 403 {
				t.Fatalf("terminal observation %d %s", w.Code, w.Body.String())
			}
			trip.Store(0)
			// Apparent restoration has a NEW epoch and cannot resurrect a terminal
			// Action, even if a later injected opening flag is now true.
			r.authority(t, `UPDATE controlled_draft_test_authority SET cancelled=false,deleted=false,allow_read=true,delivered_action=$1,epoch=epoch+2`, created.Action.ID)
			w = r.request(t, "GET", "/api/drafts/terminal-review/versions/1", "", nil)
			var result draftrecovery.Result
			json.Unmarshal(w.Body.Bytes(), &result)
			if w.Code != 410 || result.Bundle != nil || result.Review != nil {
				t.Fatalf("observed terminal fence revived: %d %s", w.Code, w.Body.String())
			}
			if r.sends.Load() != 1 || r.checks.Load() != 1 {
				t.Fatal("terminal recovery inferred")
			}
		})
	}
}
