package accountapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"professional-information-repo/internal/aidiagnostics"
	"professional-information-repo/internal/cvsource"
	"professional-information-repo/internal/profiledocument"
)

func TestManagedCVRejectsStaleFullProfileWithIdenticalFacts(t *testing.T) {
	f, _, p, r := managedCVFixture(t, nil)
	var client profiledocument.Document
	_ = json.Unmarshal(f.Document, &client)
	client.Entities = []profiledocument.Entity{{ID: "education", Revision: 1, Kind: "education", LegacyID: "old-ed", Order: 0}}
	client.Facts[0].Field = "degree"
	client.Facts[0].Value = json.RawMessage(`"Computer science"`)
	client.Facts[0].Owner = profiledocument.Reference{ProfileID: client.ID, ID: "education", Revision: 1}
	client.Facts[0].Support = "unsupported"
	client.Links = []profiledocument.Link{}
	client.Evidence = []profiledocument.Evidence{}
	current := client
	current.Revision = 2
	encoded, _ := json.Marshal(current)
	if _, e := profiledocument.Decode(encoded); e != nil {
		t.Fatal(e)
	}
	_, e := f.DB.Exec(`UPDATE careeros_profiles SET document=$2 WHERE account_id=$1`, f.Principal.AccountID, encoded)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]any{"profileSource": cvsource.ProfileSource{ID: client.ID, Revision: client.Revision}, "facts": cvsource.Facts(client), "cvLanguage": "en", "density": "balanced", "statementSupport": true})
	w := qualificationRequest(t, f, r, "/api/cv/generate", "old-full-revision", "", body)
	if w.Code < 400 || len(p.calls) != 0 {
		t.Fatalf("client full revision1/current2 equal facts silently rebound: status%d sends%v", w.Code, p.calls)
	}
	var request map[string]any
	_ = json.Unmarshal(body, &request)
	delete(request, "profileSource")
	missing, _ := json.Marshal(request)
	w = qualificationRequest(t, f, r, "/api/cv/generate", "missing-full-source", "", missing)
	if w.Code < 400 || len(p.calls) != 0 {
		t.Fatal("missing full revision accepted")
	}
	request["profileSource"] = cvsource.ProfileSource{ID: "foreign-profile", Revision: current.Revision}
	foreign, _ := json.Marshal(request)
	w = qualificationRequest(t, f, r, "/api/cv/generate", "foreign-full-source", "", foreign)
	if w.Code < 400 || len(p.calls) != 0 {
		t.Fatal("foreign full source accepted with equal facts")
	}
	request["profileSource"] = cvsource.ProfileSource{ID: current.ID, Revision: current.Revision}
	fresh, _ := json.Marshal(request)
	w = qualificationRequest(t, f, r, "/api/cv/generate", "current-full-source", "", fresh)
	if w.Code != 200 || len(p.calls) != 2 {
		t.Fatal("current sparse full revision denied", w.Code, w.Body.String())
	}
}

func TestManagedCVPersistedDeadlineClampsActualInFlightContext(t *testing.T) {
	for _, leg := range []string{"generation", "statement_support_assessment"} {
		t.Run(leg, func(t *testing.T) {
			f, h, p, r := managedCVFixture(t, nil)
			normalized, _ := cvsource.Normalize(managedCVBody())
			s, e := h.Open(context.Background(), f.Principal, "aged-budget", "cv_generation", normalized)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.BeginStandaloneCV(context.Background()); e != nil {
				t.Fatal(e)
			}
			var expires time.Time
			if e = f.DB.QueryRow(`UPDATE ai_fixture_stage SET created_at=clock_timestamp()-interval '24 seconds' WHERE stage='managed_request_cv' RETURNING created_at+interval '25 seconds'`).Scan(&expires); e != nil {
				t.Fatal(e)
			}
			continued := false
			var latest time.Time
			reached := 0
			_ = h.SetQualificationResponder(func(ctx context.Context, provider string, request *http.Request) (*http.Response, bool, error) {
				reached++
				d, ok := ctx.Deadline()
				if !ok {
					t.Error("provider has no deadline")
				}
				if d.After(latest) {
					latest = d
				}
				if aidiagnostics.StageName(ctx) == leg {
					wait := time.Until(expires) + 200*time.Millisecond
					if wait < 0 {
						wait = 0
					}
					select {
					case <-ctx.Done():
						return nil, false, ctx.Err()
					case <-time.After(wait):
						continued = true
					}
				}
				return p.respond(ctx, provider, request)
			})
			w := qualificationRequest(t, f, r, "/api/cv/generate", "aged-budget", "", managedCVBody())
			t.Logf("leg=%s reached=%d deadline_delta=%s continued_past_expiry=%v", leg, reached, latest.Sub(expires), continued)
			if continued || latest.After(expires) {
				t.Fatalf("actual%s context reset beyond durable deadline: extra%s continued%v status%d", leg, latest.Sub(expires), continued, w.Code)
			}
			if leg == "generation" {
				if w.Code < 400 || reached != 1 || strings.Contains(w.Body.String(), `"schemaVersion":3`) {
					t.Fatalf("expired generation %d %s sends%d", w.Code, w.Body.String(), reached)
				}
			} else if w.Code != 200 || reached != 2 || !strings.Contains(w.Body.String(), `"assessment":"unavailable"`) {
				t.Fatalf("connected candidate discarded %d %s sends%d", w.Code, w.Body.String(), reached)
			}
			if w.Header().Get("X-CareerOS-Retry-Allowed") != "" {
				t.Fatal("unknown work acquired Retry")
			}
			scope, e := h.ScopeStatus(context.Background(), f.Principal, "global")
			if e != nil || scope.Occupied != 1 || scope.Counted <= 0 {
				t.Fatalf("unknown hold released: %+v %v", scope, e)
			}
		})
	}
}
