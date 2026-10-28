package accountapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"professional-information-repo/internal/auth"
	"professional-information-repo/internal/netlifyproxy"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/qualificationmatching"
)

func usefulDocument(t *testing.T, id, text string) []byte {
	t.Helper()
	var doc profiledocument.Document
	if err := json.Unmarshal(fixture(t, 1), &doc); err != nil {
		t.Fatal(err)
	}
	old := doc.ID
	doc.Facts[0].Field = "skills"
	doc.Facts[0].Value, _ = json.Marshal(text)
	raw, _ := json.Marshal(doc)
	return bytes.ReplaceAll(raw, []byte(`"`+old+`"`), []byte(`"`+id+`"`))
}

func workflowBody(t *testing.T, raw []byte) []byte {
	t.Helper()
	doc, err := profiledocument.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	repo, qualifications := workflowProfile(doc)
	body, _ := json.Marshal(map[string]any{
		"repository": repo, "qualifications": qualifications,
		"profileEvidence": qualificationmatching.ApprovedProjection(doc),
		"jobPosting":      "Synthetic Java developer.", "confirmedQualifications": []any{},
	})
	return body
}

func TestPostgresWorkflowOwnershipAndResultFencing(t *testing.T) {
	dsn, dbconn := database(t)
	store := open(t, dsn, "journey-workflows")
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := service(t, store, fakeOIDC{})
	a, b, empty := login(t, svc, "alpha"), login(t, svc, "beta"), login(t, svc, "empty")
	insufficient := login(t, svc, "insufficient")
	if _, err := store.SaveProfile(context.Background(), insufficient.Principal, "first", 0, usefulDocument(t, "insufficient-profile", "")); err != nil {
		t.Fatal(err)
	}
	da, db := usefulDocument(t, "alpha-profile", "Java alpha"), usefulDocument(t, "beta-profile", "Go beta")
	for _, v := range []struct {
		who *auth.LoginResult
		doc []byte
	}{{a, da}, {b, db}} {
		if _, err := store.SaveProfile(context.Background(), v.who.Principal, "first", 0, v.doc); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/api/application-draft", "/api/qualification-gaps"} {
		t.Run(path, func(t *testing.T) {
			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("bad downstream body")
				}
				if path == "/api/application-draft" && (string(body["reviewResume"]) != "true" || string(body["reviewArtifacts"]) != "true") {
					t.Fatal("review bypass")
				}
				w.Write([]byte(`{"synthetic":"result"}`))
			})
			handler := ProtectWorkflow(svc, store, path, next)
			for _, tc := range []struct {
				name   string
				who    *auth.LoginResult
				body   []byte
				status int
			}{
				{"anonymous", nil, workflowBody(t, da), 401},
				{"missing Profile", empty, workflowBody(t, da), 422},
				{"insufficient saved Profile", insufficient, workflowBody(t, da), 422},
				{"foreign snapshot", a, workflowBody(t, db), 409},
				{"alpha positive", a, workflowBody(t, da), 200},
				{"beta positive", b, workflowBody(t, db), 200},
			} {
				t.Run(tc.name, func(t *testing.T) {
					result := api(t, handler, tc.who, "POST", path, tc.body)
					if result.Code != tc.status {
						t.Fatalf("got %d: %s", result.Code, result.Body.String())
					}
				})
			}
			if calls != 2 {
				t.Fatalf("denied request reached generation: %d", calls)
			}
			// A's queued command cannot execute under B's new cookie, even with B data.
			switched := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Set("X-CareerOS-Account", a.Account.ID)
				handler.ServeHTTP(w, r)
			})
			if result := api(t, switched, b, "POST", path, workflowBody(t, db)); result.Code != 409 {
				t.Fatal(result.Code)
			}
			noCSRF := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Header.Del("X-CSRF-Token"); handler.ServeHTTP(w, r) })
			if result := api(t, noCSRF, a, "POST", path, workflowBody(t, da)); result.Code != 403 {
				t.Fatal(result.Code)
			}
			tampered := bytes.Replace(workflowBody(t, da), []byte("Java alpha"), []byte("Invented qualification"), 1)
			if result := api(t, handler, a, "POST", path, tampered); result.Code != 409 {
				t.Fatal(result.Code)
			}
			// Exercise the same adapter used by direct Functions and public aliases.
			for _, eventPath := range []string{path, "/.netlify/functions/" + strings.TrimPrefix(path, "/api/")} {
				event := events.APIGatewayProxyRequest{Path: eventPath, HTTPMethod: "POST", Body: string(workflowBody(t, da)), Headers: map[string]string{
					"Origin": origin, "Cookie": "__Host-careeros_session=" + a.SessionToken + "; __Host-careeros_csrf=" + a.CSRFToken,
					"X-CSRF-Token": a.CSRFToken, "X-CareerOS-Account": a.Account.ID,
				}}
				result, err := netlifyproxy.Handle(handler, path, context.Background(), event)
				if err != nil || result.StatusCode != 200 {
					t.Fatalf("alias %s: %v %+v", eventPath, err, result)
				}
				delete(event.Headers, "Cookie")
				result, _ = netlifyproxy.Handle(handler, path, context.Background(), event)
				if result.StatusCode != 401 {
					t.Fatal("direct guest bypass")
				}
			}
		})
	}
	t.Run("Profile changed while generating", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			changed := bytes.ReplaceAll(db, []byte(`"revision":1`), []byte(`"revision":2`))
			if _, err := store.SaveProfile(r.Context(), b.Principal, "changed", 1, changed); err != nil {
				t.Fatal(err)
			}
			w.Write([]byte("STALE_RESULT"))
		})
		result := api(t, ProtectWorkflow(svc, store, "/api/application-draft", next), b, "POST", "/api/application-draft", workflowBody(t, db))
		if result.Code != 409 || strings.Contains(result.Body.String(), "STALE_RESULT") {
			t.Fatal(result.Code, result.Body.String())
		}
	})
	t.Run("cancelled response is not published", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cancel(); w.Write([]byte("CANCELLED_RESULT")) })
		guarded := ProtectWorkflow(svc, store, "/api/application-draft", next)
		withContext := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { guarded.ServeHTTP(w, r.WithContext(ctx)) })
		result := api(t, withContext, a, "POST", "/api/application-draft", workflowBody(t, da))
		if result.Body.Len() != 0 {
			t.Fatal(result.Body.String())
		}
	})
	t.Run("late result after revocation", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := svc.Logout(r.Context(), a.SessionToken); err != nil {
				t.Fatal(err)
			}
			w.Write([]byte("PRIVATE_RESULT"))
		})
		result := api(t, ProtectWorkflow(svc, store, "/api/application-draft", next), a, "POST", "/api/application-draft", workflowBody(t, da))
		if result.Code != 401 || strings.Contains(result.Body.String(), "PRIVATE_RESULT") {
			t.Fatal(result.Code, result.Body.String())
		}
	})
	t.Run("expired and suspended", func(t *testing.T) {
		for _, subject := range []string{"expired", "suspended"} {
			who := login(t, svc, subject)
			var err error
			if subject == "expired" {
				_, err = dbconn.Exec("UPDATE careeros_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", who.Principal.SessionID)
			} else {
				_, err = dbconn.Exec("UPDATE careeros_accounts SET status='suspended' WHERE id=$1", who.Account.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid session dispatched") })
			result := api(t, ProtectWorkflow(svc, store, "/api/application-draft", next), who, "POST", "/api/application-draft", workflowBody(t, db))
			if result.Code != 401 && result.Code != 403 {
				t.Fatal(result.Code, result.Body.String())
			}
		}
	})
}

func TestWorkflowAlwaysFailsClosedWithoutRuntime(t *testing.T) {
	for _, path := range []string{"/api/application-draft", "/api/qualification-gaps", "/api/profile/ingest", "/api/profile/review", "/api/cv/generate"} {
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unconfigured workflow dispatched") })
		result := httptest.NewRecorder()
		ProtectWorkflow(nil, nil, path, next).ServeHTTP(result, httptest.NewRequest("POST", path, nil))
		if result.Code != 503 {
			t.Fatal(result.Code)
		}
	}
}
