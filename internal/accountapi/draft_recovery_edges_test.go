package accountapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

type recoveryFixture struct {
	f        *aiadmission.Fixture
	store    *draftrecovery.Store
	actions  *aicontrol.Harness
	router   http.Handler
	sends    atomic.Int32
	checks   atomic.Int32
	provider draftrecovery.ControlledProvider
	review   draftrecovery.ControlledReview
	fault    func(string) error
	config   draftrecovery.Config
}

func newRecoveryFixture(t *testing.T, beforeMigration func(*recoveryFixture)) *recoveryFixture {
	t.Helper()
	r := &recoveryFixture{f: aiadmission.NewFixture(t)}
	ctx := context.Background()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("controlled-draft", 1)
	var err error
	r.actions, err = aicontrol.NewSynthetic(ctx, r.f.DSN, aicontrol.DefaultSyntheticConfig("controlled-draft", gate))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.actions.Close() })
	if err = r.actions.SyntheticReviewer().SeedGrant(ctx, r.f.Principal, time.Now().Add(time.Hour), 3); err != nil {
		t.Fatal(err)
	}
	_, err = r.f.DB.Exec(`CREATE TABLE controlled_draft_test_authority(owner text PRIMARY KEY,allow_generate boolean,allow_publish boolean,allow_read boolean,grant_end timestamptz,epoch bigint,deleted boolean DEFAULT false,revoked boolean DEFAULT false,cancelled boolean DEFAULT false,delivered_action text DEFAULT '')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.f.DB.Exec(`INSERT INTO controlled_draft_test_authority(owner,allow_generate,allow_publish,allow_read,grant_end,epoch) VALUES($1,true,true,true,clock_timestamp()+interval '1 hour',1)`, r.f.Principal.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	r.config = draftrecovery.Config{Actions: r.actions, Review: func(ctx context.Context, q draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
		r.checks.Add(1)
		if r.review != nil {
			return r.review(ctx, q)
		}
		return controlledRecoveryReview(ctx, q)
	}, BindingKey: []byte("draft-test-binding-key-32-bytes-long"), Decisions: draftrecovery.DecisionFunc(func(ctx context.Context, a draftrecovery.Authority) (draftrecovery.Decision, error) {
		var d draftrecovery.Decision
		var delivered string
		err := a.Transaction.QueryRowContext(ctx, `SELECT allow_generate,allow_publish,allow_read,grant_end,epoch,deleted,revoked,cancelled,delivered_action FROM controlled_draft_test_authority WHERE owner=$1 FOR UPDATE`, a.Principal.AccountID).Scan(&d.Generate, &d.Publish, &d.Read, &d.GrantExpiresAt, &d.Epoch, &d.Deleted, &d.Revoked, &d.Cancelled, &delivered)
		d.Delivered = a.Action.ID != "" && delivered == a.Action.ID
		return d, err
	}), Fault: func(stage string) error {
		if r.fault != nil {
			return r.fault(stage)
		}
		return nil
	}}
	if beforeMigration != nil {
		beforeMigration(r)
	}
	r.store, err = draftrecovery.Open(ctx, r.f.DSN, r.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.store.Close() })
	if err = r.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r.router = accountapi.NewControlledDraftRecovery(r.f.Service, r.store, func(ctx context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		r.sends.Add(1)
		if r.provider != nil {
			return r.provider(ctx, g)
		}
		return completeRecoveryBundle(g), nil
	})
	return r
}
func (r *recoveryFixture) input(t *testing.T) draftrecovery.Input {
	t.Helper()
	var doc profiledocument.Document
	if err := json.Unmarshal(r.f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	return draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision}, Job: draftrecovery.Job{Posting: "Java required."}, Language: "en", Answers: []draftrecovery.Answer{}, Confirmations: []draftrecovery.Answer{}, SelectedFactIDs: []string{}}
}
func (r *recoveryFixture) request(t *testing.T, method, path, key string, input any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(input)
	req := r.f.Request(t, context.Background(), path, body)
	req.Method = method
	req.Header.Del("X-OpenAI-Api-Key")
	req.Header.Del("X-TypeSafe-Api-Key")
	req.Header.Set("X-CareerOS-Action-Key", key)
	w := httptest.NewRecorder()
	r.router.ServeHTTP(w, req)
	return w
}
func (r *recoveryFixture) create(t *testing.T, key string) draftrecovery.Result {
	t.Helper()
	w := r.request(t, "POST", "/api/drafts", key, r.input(t))
	var out draftrecovery.Result
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	return out
}
func (r *recoveryFixture) authority(t *testing.T, query string, args ...any) {
	t.Helper()
	tx, err := r.f.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM careeros_accounts WHERE id=$1 FOR UPDATE`, r.f.Principal.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestControlledDraftRejectsIncompleteOrMismatchedWholeBundle(t *testing.T) {
	cases := []struct {
		name   string
		change func(*draftrecovery.Bundle)
	}{
		{"cv-only", func(b *draftrecovery.Bundle) { b.RoleSummary = ""; b.CoverLetter = "" }},
		{"unicode-empty", func(b *draftrecovery.Bundle) { b.RoleSummary = "\ufeff"; b.Support.Artifacts[0].Text = "\ufeff" }},
		{"uncertain-resume", func(b *draftrecovery.Bundle) { b.Support.Resume[0].State = "uncertain" }},
		{"unsupported-artifact", func(b *draftrecovery.Bundle) { b.Support.Artifacts[0].State = "unsupported" }},
		{"unavailable-review", func(b *draftrecovery.Bundle) { b.Support.Check = "unavailable" }},
		{"unresolved-concern", func(b *draftrecovery.Bundle) {
			b.Support.Artifacts[0].Concerns = []string{"missing required employer answer"}
		}},
		{"invalid-job-range", func(b *draftrecovery.Bundle) { b.Support.Artifacts[0].JobSources[0].Quote = "Other job" }},
		{"field-claim-mismatch", func(b *draftrecovery.Bundle) { b.CoverLetter += "\nChanged" }},
		{"mixed-action", func(b *draftrecovery.Bundle) { b.Binding.Action.ID = "old-action" }},
		{"false-none-requested", func(b *draftrecovery.Bundle) {
			b.Answers.Requests = []draftrecovery.AnswerRequest{{ID: "missing", Source: draftrecovery.JobRange{Start: 0, End: 14, Quote: "Java required."}, Answer: "Java"}}
		}},
		{"career-as-nonfactual", func(b *draftrecovery.Bundle) {
			b.Support.Resume[0].Nonfactual = true
			b.Support.Resume[0].Sources = []draftrecovery.Reference{}
			b.Support.Resume[0].JobSources = []draftrecovery.JobRange{}
		}},
		{"career-with-job-only", func(b *draftrecovery.Bundle) {
			b.Support.Resume[0].Sources = []draftrecovery.Reference{}
			b.Support.Resume[0].JobSources = []draftrecovery.JobRange{{Start: 0, End: 14, Quote: "Java required."}}
		}},
		{"closing-only-letter", func(b *draftrecovery.Bundle) {
			b.CoverLetter = "Sincerely,"
			b.Support.Artifacts[1].Text = b.CoverLetter
		}},
		{"missing-letter-body", func(b *draftrecovery.Bundle) {
			b.CoverLetter = "Dear hiring team,\n\nSincerely,"
			b.Support.Artifacts[1].Text = b.CoverLetter
		}},
		{"unapproved-letter-closing", func(b *draftrecovery.Bundle) {
			b.CoverLetter = "Dear hiring team,\n\nI use Java.\n\nCheers,"
			b.Support.Artifacts[1].Text = b.CoverLetter
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRecoveryFixture(t, nil)
			r.provider = func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
				b := completeRecoveryBundle(g)
				tc.change(&b)
				return b, nil
			}
			w := r.request(t, "POST", "/api/drafts", "invalid", r.input(t))
			if w.Code < 400 || r.sends.Load() != 1 {
				t.Fatalf("incomplete published: %d %s", w.Code, w.Body.String())
			}
			status := r.request(t, "GET", "/api/drafts/invalid", "", nil)
			var result draftrecovery.Result
			if status.Code != 202 || json.Unmarshal(status.Body.Bytes(), &result) != nil || result.State != "incomplete" || result.Bundle != nil || result.Version != 0 {
				t.Fatalf("partial became ready: %d %s", status.Code, status.Body.String())
			}
			_ = r.request(t, "POST", "/api/drafts", "invalid", r.input(t))
			if r.sends.Load() != 1 {
				t.Fatal("incomplete replay regenerated")
			}
		})
	}
}

func TestControlledDraftContentExcludesInheritedLedgerDefaultPrivileges(t *testing.T) {
	var runtimeRole, parent string
	r := newRecoveryFixture(t, func(r *recoveryFixture) {
		if err := r.f.DB.QueryRow(`SELECT rolname FROM pg_roles WHERE rolname LIKE 'ai_fixture_rt_%' AND has_schema_privilege(rolname,current_schema(),'USAGE')`).Scan(&runtimeRole); err != nil {
			t.Fatal(err)
		}
		parent = fmt.Sprintf("draft_default_%d", time.Now().UnixNano())
		if _, err := r.f.DB.Exec("CREATE ROLE " + parent + " NOLOGIN"); err != nil {
			t.Fatal(err)
		}
		if _, err := r.f.DB.Exec("ALTER ROLE " + runtimeRole + " INHERIT"); err != nil {
			t.Fatal(err)
		}
		if _, err := r.f.DB.Exec("GRANT " + parent + " TO " + runtimeRole); err != nil {
			t.Fatal(err)
		}
		var inherits bool
		if err := r.f.DB.QueryRow("SELECT pg_has_role($1,$2,'USAGE')", runtimeRole, parent).Scan(&inherits); err != nil || !inherits {
			t.Fatalf("fixture did not create inherited membership: %v %v", inherits, err)
		}
		var schema string
		if err := r.f.DB.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
			t.Fatal(err)
		}
		if _, err := r.f.DB.Exec("ALTER DEFAULT PRIVILEGES IN SCHEMA " + schema + " GRANT SELECT ON TABLES TO " + parent); err != nil {
			t.Fatal(err)
		}
	})
	r.create(t, "private-content")
	var allowed bool
	if err := r.f.DB.QueryRow(`SELECT has_table_privilege($1,'careeros_draft_content','SELECT')`, runtimeRole).Scan(&allowed); err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("retained career content inherited by AI-ledger runtime role through default privileges")
	}
	for _, table := range []string{"careeros_draft_actions", "careeros_draft_admissions", "careeros_draft_reviews", "careeros_draft_review_commands"} {
		if err := r.f.DB.QueryRow(`SELECT has_table_privilege($1,$2,'SELECT')`, runtimeRole, table).Scan(&allowed); err != nil || allowed {
			t.Fatalf("ledger role inherited restricted recovery metadata: %s allowed=%v err=%v", table, allowed, err)
		}
	}

	u, _ := url.Parse(r.f.DSN)
	u.User = url.User(runtimeRole)
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var current string
	if err = db.QueryRow("SELECT current_user").Scan(&current); err != nil || current != runtimeRole {
		t.Fatalf("not an independent role login: %s %v", current, err)
	}
	if rows, err := db.Query(`SELECT input,bundle FROM careeros_draft_content`); err == nil {
		rows.Close()
		t.Fatal("ledger role read retained content")
	} else if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("unexpected role failure: %v", err)
	}
}

func TestControlledDraftFreezesIdentityAndSelectedSourceOnly(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	var doc profiledocument.Document
	if err := json.Unmarshal(r.f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	doc.Revision++
	for i := range doc.Facts {
		if doc.Facts[i].Owner.ID == doc.ID {
			doc.Facts[i].Owner.Revision = doc.Revision
		}
	}
	name := doc.Facts[0]
	name.ID = "frozen-name"
	name.Field = "fullName"
	name.Value = json.RawMessage(`"Original Person"`)
	name.Normalization.Observed = "Original Person"
	name.Order = 2
	name.Support = "unsupported"
	name.Origin = profiledocument.Origin{Kind: "manual_edit", Original: "user"}
	other := name
	other.ID = "not-selected"
	other.Field = "tools"
	other.Value = json.RawMessage(`"Unselected Kubernetes"`)
	other.Normalization.Observed = "Unselected Kubernetes"
	other.Order = 3
	doc.Facts = append(doc.Facts, name, other)
	raw, _ := json.Marshal(doc)
	if _, err := r.f.Store.SaveProfile(context.Background(), r.f.Principal, "named-profile", 1, raw); err != nil {
		t.Fatal(err)
	}
	r.f.Document = raw
	var selected []string
	r.provider = func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		for _, f := range g.Facts {
			selected = append(selected, f.Reference.ID)
		}
		return completeRecoveryBundle(g), nil
	}
	in := r.input(t)
	in.SelectedFactIDs = []string{doc.Facts[0].ID}
	w := r.request(t, "POST", "/api/drafts", "frozen-identity", in)
	var original draftrecovery.Result
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &original) != nil {
		t.Fatalf("named result: %d %s", w.Code, w.Body.String())
	}
	if original.Bundle.Identity.FullName != "Original Person" || !strings.HasSuffix(original.Bundle.CoverLetter, "\nOriginal Person") || !reflect.DeepEqual(selected, []string{doc.Facts[0].ID}) {
		t.Fatalf("identity/selection changed: name=%q letter=%q facts=%v", original.Bundle.Identity.FullName, original.Bundle.CoverLetter, selected)
	}
	doc.Revision++
	for i := range doc.Facts {
		if doc.Facts[i].Owner.ID == doc.ID {
			doc.Facts[i].Owner.Revision = doc.Revision
		}
		if doc.Facts[i].ID == "frozen-name" {
			doc.Facts[i].Value = json.RawMessage(`"Changed Person"`)
			doc.Facts[i].Revision++
			doc.Facts[i].Normalization.Observed = "Changed Person"
		}
	}
	raw, _ = json.Marshal(doc)
	if _, err := r.f.Store.SaveProfile(context.Background(), r.f.Principal, "changed-name", 2, raw); err != nil {
		t.Fatal(err)
	}
	r.authority(t, `UPDATE controlled_draft_test_authority SET allow_generate=false,allow_publish=false WHERE owner=$1`, r.f.Principal.AccountID)
	w = r.request(t, "GET", "/api/drafts/frozen-identity/versions/1", "", nil)
	var historical draftrecovery.Result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &historical) != nil || !historical.Historical || !reflect.DeepEqual(original.Bundle, historical.Bundle) || r.sends.Load() != 1 {
		t.Fatalf("historical changed/re-generated: %d %s sends=%d", w.Code, w.Body.String(), r.sends.Load())
	}
}

func TestControlledDraftMissingEmployerAnswerCannotMeanNoneWhenAnalysisAbsent(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	in := r.input(t)
	in.Job.Posting = "Java required. Explain your Java experience."
	in.Job.Analysis = nil
	w := r.request(t, "POST", "/api/drafts", "missing-required-answer", in)
	if w.Code < 400 {
		t.Fatalf("actual employer request omitted under explicit none-requested label: %d %s", w.Code, w.Body.String())
	}
	status := r.request(t, "GET", "/api/drafts/missing-required-answer", "", nil)
	var result draftrecovery.Result
	if json.Unmarshal(status.Body.Bytes(), &result) != nil || result.Bundle != nil || result.Version != 0 || r.sends.Load() != 1 {
		t.Fatalf("omission became recoverable ready content: %s", status.Body.String())
	}
}

func TestControlledDraftCompleteEmployerAnswersWithoutOptionalAnalysis(t *testing.T) {
	r := newRecoveryFixture(t, nil)
	in := r.input(t)
	in.Job.Posting = "Java required. Explain your Java experience."
	in.Answers = []draftrecovery.Answer{{Requirement: "Explain your Java experience.", UserContext: "I use Java."}}
	w := r.request(t, "POST", "/api/drafts", "complete-answers", in)
	var result draftrecovery.Result
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Bundle == nil || result.Bundle.Answers.State != "complete" || len(result.Bundle.Answers.Requests) != 1 || result.Job.Analysis != nil {
		t.Fatalf("explicit answers required optional analysis or failed complete publication: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/drafts/complete-answers", "/.netlify/functions/account/api/drafts/complete-answers/versions/1"} {
		read := r.request(t, "GET", path, "", nil)
		var recovered draftrecovery.Result
		if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &recovered) != nil || !reflect.DeepEqual(result, recovered) || r.sends.Load() != 1 || r.checks.Load() != 1 {
			t.Fatalf("complete answers changed/rechecked on recovery: %d %s", read.Code, read.Body.String())
		}
	}
}
