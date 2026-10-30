package aicontrol

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/aidispatch"
)

func TestA34_RestrictedSQLRolesAndSafeRelease(t *testing.T) {
	f := setup(t, func(c *SyntheticConfig) { c.Limit, c.Capacity = 1000, 10 })
	s := f.open(t, "restricted")
	d := f.sent(t, s)
	var runtimeRole, reviewerRole string
	if err := f.h.db.QueryRow("SELECT current_user").Scan(&runtimeRole); err != nil {
		t.Fatal("runtime role unavailable")
	}
	if err := f.h.reviewerDB.QueryRow("SELECT current_user").Scan(&reviewerRole); err != nil {
		t.Fatal("reviewer role unavailable")
	}
	if runtimeRole == reviewerRole || !strings.HasPrefix(runtimeRole, "ai_fixture_rt_") || !strings.HasPrefix(reviewerRole, "ai_fixture_review_") {
		t.Fatal("fixture runtime and reviewer are not separate SQL roles")
	}
	conn, err := f.h.db.Conn(context.Background())
	if err != nil {
		t.Fatal("runtime connection unavailable")
	}
	if _, err = conn.ExecContext(context.Background(), "RESET ROLE"); err != nil {
		t.Fatal("runtime RESET ROLE failed")
	}
	var current, session string
	if err = conn.QueryRowContext(context.Background(), "SELECT current_user,session_user").Scan(&current, &session); err != nil || current != runtimeRole || session != runtimeRole {
		t.Fatal("runtime role can escape to the administrative login")
	}
	if _, err = conn.ExecContext(context.Background(), "SET ROLE "+reviewerRole); err == nil {
		t.Fatal("runtime can assume the reviewer role")
	}
	conn.Close()
	for _, query := range []string{
		`SELECT document FROM careeros_profiles`,
		`SELECT * FROM careeros_profile_operations`,
		`SELECT email,display_name FROM careeros_accounts`,
	} {
		_, err := f.h.reviewerDB.Exec(query)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "42501" {
			t.Fatal("reviewer unexpectedly has access to career content or personal account fields")
		}
	}
	for _, query := range []string{
		`UPDATE ai_fixture_dispatch SET floor_amount=0,counted=0`,
		`UPDATE ai_fixture_dispatch SET expense=0`,
		`DELETE FROM ai_fixture_dispatch`,
		`UPDATE ai_fixture_scope SET counted=0,occupied=0`,
		`UPDATE ai_fixture_action SET customer_effect=true`,
		`INSERT INTO ai_fixture_action(namespace,id,owner,creation_key,workflow,binding,grant_version,customer_effect) SELECT namespace,'forged_effect',owner,'forged_effect',workflow,binding,grant_version,true FROM ai_fixture_action LIMIT 1`,
		`INSERT INTO ai_fixture_dispatch(namespace,id,action,coordinate,stage,leg,attempt,provider,binding,maximum,floor_amount,counted,input_bytes,output_tokens,category) SELECT namespace,'forged_bill',action,coordinate,stage,leg,attempt,provider,binding,maximum,0,0,input_bytes,output_tokens,'confirmed_bill' FROM ai_fixture_dispatch LIMIT 1`,
		`UPDATE ai_fixture_policy SET expires_at=clock_timestamp()+interval '100 years'`,
		`UPDATE ai_fixture_dispatch SET state='reserved'`,
		`UPDATE ai_fixture_dispatch SET remote_state='terminal_proven'`,
		`UPDATE careeros_accounts SET created_at=clock_timestamp()`,
		`UPDATE careeros_sessions SET created_at=clock_timestamp()`,
	} {
		if _, err := f.h.db.Exec(query); err == nil {
			t.Fatal("runtime SQL unexpectedly allowed forbidden mutation", query)
		}
	}
	// A routine-support role cannot read or mutate the restricted metadata,
	// including by naming the schema explicitly rather than relying on search_path.
	u, err := url.Parse(f.dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	schema := q.Get("search_path")
	u.User = url.User(f.h.supportRole)
	q.Del("role")
	q.Del("user")
	q.Del("password")
	q.Del("options")
	u.RawQuery = q.Encode()
	support, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal("support fixture connection unavailable")
	}
	defer support.Close()
	for _, query := range []string{
		"SELECT * FROM " + schema + ".ai_fixture_dispatch",
		"SELECT * FROM " + schema + ".ai_fixture_action",
		"UPDATE " + schema + ".ai_fixture_dispatch SET counted=0",
	} {
		_, err := support.Exec(query)
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "42501" {
			t.Fatal("support did not receive an SQL insufficient-privilege denial")
		}
	}
	// Neither arbitrary SQL nor the only runtime release command can clear a
	// possibly-sent obligation. Actual approved evidence uses the other pool.
	if _, err := f.h.db.Exec("SELECT ai_fixture_abandon($1,$2)", f.cfg.Namespace, d.ID); err == nil {
		t.Fatal("runtime release command accepted an observed dispatch")
	}
	if scope := f.scope(t); scope.Counted != 60 || scope.Occupied != 1 {
		t.Fatal("denied SQL changed exposure", scope)
	}
	if err := f.h.SyntheticReviewer().ApplyEvidence(context.Background(), f.p, Evidence{
		ID: "restricted_bill", Dispatch: d.ID, Provider: d.Provider, Kind: "bill", Amount: pointer(40), ReleaseFloor: true,
	}); err != nil {
		t.Fatal("authorized synthetic reviewer could not reconcile", err)
	}
	if scope := f.scope(t); scope.Counted != 40 || scope.Occupied != 1 {
		t.Fatal("financial evidence changed remote capacity", scope)
	}
	// A still-reserved dispatch can be atomically aborted by the bounded
	// command; repeating it cannot release the already-counted expense twice.
	unsent := f.open(t, "unsent_restricted")
	reserved, err := unsent.Reserve(context.Background(), Leg{
		Provider: "openai", Coordinate: "root", Stage: "generation", Number: 1, Body: fixtureBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := unsent.Abandon(context.Background(), reserved.ID); err != nil {
			t.Fatal("safe unsent release denied", err)
		}
	}
	if scope := f.scope(t); scope.Counted != 40 || scope.Occupied != 1 {
		t.Fatal("safe release lost the prior obligation", scope)
	}
}

func TestA34RestrictedMetadataDoesNotEnterOrdinaryDiagnostics(t *testing.T) {
	f := setup(t, nil)
	const source = "synthetic-career-content-private-sentinel"
	const credential = "synthetic-credential-private-sentinel"
	const providerID = "synthetic-provider-request-private-sentinel"
	root := []byte(`{"synthetic":"` + source + `"}`)
	s, err := f.h.Open(context.Background(), f.p, "private_metadata", "cv_generation", root)
	if err != nil {
		t.Fatal(err)
	}
	f.h.SetResponder(func(_ context.Context, _ string, r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("credential sent to synthetic callback")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{providerID}}, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"` + source + `"}}]}`))}, nil
	})
	var ordinary []byte
	ctx := aidiagnostics.WithSink(context.Background(), func(attempt aidiagnostics.Attempt) { ordinary, _ = json.Marshal(attempt) })
	handler := aidiagnostics.Workflow("cv_generation", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, finish := aidiagnostics.Start(aidispatch.WithWorkflowRoot(r.Context(), "cv_generation", root), "generation", "synthetic-v1")
		defer finish("completed")
		request := req()
		request.Header.Set("Authorization", "Bearer "+credential)
		response, e := s.Send(ctx, "openai", request)
		if e != nil {
			t.Fatal(e)
		}
		closeResp(response)
		w.WriteHeader(200)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/synthetic", nil).WithContext(ctx))
	status := f.status(t, s)
	if len(ordinary) == 0 || len(status.Dispatches) != 1 {
		t.Fatal("ordinary diagnostic was not exercised")
	}
	d := status.Dispatches[0]
	for _, secret := range []string{source, credential, providerID, f.p.AccountID, s.ID(), d.ID, fmt.Sprintf("%x", s.root)} {
		if strings.Contains(string(ordinary), secret) {
			t.Fatal("restricted content or joinable metadata leaked into diagnostic export")
		}
	}
	// This reads the restricted fixture ledger with the test administrator. It
	// verifies content minimization separately from SQL access-control tests.
	for _, table := range []string{"ai_fixture_action", "ai_fixture_stage", "ai_fixture_dispatch", "ai_fixture_evidence", "ai_fixture_observation"} {
		rows, e := f.db.Query("SELECT row_to_json(t)::text FROM " + table + " t")
		if e != nil {
			t.Fatal("restricted fixture table unavailable", table)
		}
		for rows.Next() {
			var raw string
			if rows.Scan(&raw) != nil {
				t.Fatal("fixture row unavailable")
			}
			for _, secret := range []string{source, credential, "synthetic-must-not-reach-fixture"} {
				if strings.Contains(raw, secret) {
					t.Fatal("career content or credential persisted in non-content ledger", table)
				}
			}
		}
		if rows.Err() != nil {
			t.Fatal("fixture export failed")
		}
		rows.Close()
	}
	if err := f.h.SyntheticReviewer().ApplyEvidence(context.Background(), f.p, Evidence{ID: "invalid_private", Dispatch: d.ID, Provider: providerID, Kind: "bill", Amount: pointer(1)}); err == nil || strings.Contains(err.Error(), source) || strings.Contains(err.Error(), providerID) || strings.Contains(err.Error(), s.ID()) {
		t.Fatal("admission error disclosed restricted identifiers")
	}
}
