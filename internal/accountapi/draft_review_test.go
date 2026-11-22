package accountapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"professional-information-repo/internal/accountapi"
	"professional-information-repo/internal/aicontrol"
	"professional-information-repo/internal/draftrecovery"
	"professional-information-repo/internal/profiledocument"
	"professional-information-repo/internal/testsupport/aiadmission"
)

func TestDurableDraftAcceptedSnapshotRecoversLostDecisionAndSurvivesReplacement(t *testing.T) {
	f := aiadmission.NewFixture(t)
	ctx := context.Background()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("accepted-review", 1)
	actions, err := aicontrol.NewSynthetic(ctx, f.DSN, aicontrol.DefaultSyntheticConfig("accepted-review", gate))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { actions.Close() })
	if err = actions.SyntheticReviewer().SeedGrant(ctx, f.Principal, time.Now().Add(time.Hour), 3); err != nil {
		t.Fatal(err)
	}
	var loseAck atomic.Bool
	var sends atomic.Int32
	store, err := draftrecovery.Open(ctx, f.DSN, draftrecovery.Config{Actions: actions, BindingKey: []byte("synthetic-draft-binding-key-32-bytes"), Review: controlledRecoveryReview,
		Decisions: draftrecovery.DecisionFunc(func(context.Context, draftrecovery.Authority) (draftrecovery.Decision, error) {
			return draftrecovery.Decision{Generate: true, Publish: true, Read: true, Delivered: true, GrantExpiresAt: time.Now().Add(time.Hour)}, nil
		}),
		Fault: func(stage string) error {
			if stage == "after_review_commit" && loseAck.Swap(false) {
				return errors.New("lost review acknowledgment")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	router := accountapi.NewControlledDraftRecovery(f.Service, store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		sends.Add(1)
		return completeRecoveryBundle(g), nil
	})
	call := func(method, path string, body any, key string) (int, map[string]any) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := f.Request(t, ctx, path, raw)
		r.Method = method
		r.Header.Del("X-OpenAI-Api-Key")
		r.Header.Del("X-TypeSafe-Api-Key")
		if key != "" {
			r.Header.Set("X-CareerOS-Action-Key", key)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatalf("invalid response %d %s", w.Code, w.Body.String())
		}
		return w.Code, result
	}
	var doc profiledocument.Document
	json.Unmarshal(f.Document, &doc)
	input := draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision}, Job: draftrecovery.Job{Posting: "Java required."}, Language: "en"}
	code, original := call("POST", "/api/drafts", input, "accepted-first")
	if code != 201 {
		t.Fatal(code, original)
	}
	path := "/api/drafts/accepted-first/versions/1/review"
	revision := 0
	for _, artifact := range []string{"summary", "resume", "cover", "answers"} {
		for _, operation := range []string{"review", "accept"} {
			command := map[string]any{"decisionKey": artifact + "-" + operation, "expectedRevision": revision, "bundleDigest": original["bundleDigest"], "artifact": artifact, "operation": operation}
			if artifact == "summary" && operation == "review" {
				loseAck.Store(true)
				code, _ = call("POST", path, command, "")
				if code != 503 {
					t.Fatalf("lost commit ack: %d", code)
				}
			}
			code, result := call("POST", path, command, "")
			if code != 200 {
				t.Fatal(code, result)
			}
			revision++
			if result["review"].(map[string]any)["revision"] != float64(revision) {
				t.Fatal("duplicate effect", result)
			}
		}
	}
	code, accepted := call("GET", "/api/drafts/accepted", nil, "")
	if code != 200 {
		t.Fatal("accepted retrieval", code, accepted)
	}
	if accepted["creationKey"] != "accepted-first" || !accepted["review"].(map[string]any)["accepted"].(bool) {
		t.Fatal("wrong accepted identity", accepted)
	}
	code, replacement := call("POST", "/api/drafts", input, "replacement")
	if code != 201 {
		t.Fatal(code, replacement)
	}
	code, stillAccepted := call("GET", "/api/drafts/accepted", nil, "")
	if code != 200 || stillAccepted["creationKey"] != "accepted-first" {
		t.Fatal("replacement overwrote accepted", code, stillAccepted)
	}
	if sends.Load() != 2 {
		t.Fatal("review or retrieval inferred", sends.Load())
	}
}

func TestDurableDraftReviewPersistsExactArtifactWithoutInference(t *testing.T) {
	f := aiadmission.NewFixture(t)
	ctx := context.Background()
	gate := aicontrol.NewSyntheticGate()
	gate.Reconcile("durable-review", 1)
	actions, err := aicontrol.NewSynthetic(ctx, f.DSN, aicontrol.DefaultSyntheticConfig("durable-review", gate))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { actions.Close() })
	if err = actions.SyntheticReviewer().SeedGrant(ctx, f.Principal, time.Now().Add(time.Hour), 3); err != nil {
		t.Fatal(err)
	}
	var delivered atomic.Bool
	var generations, checks atomic.Int32
	store, err := draftrecovery.Open(ctx, f.DSN, draftrecovery.Config{
		Actions: actions, BindingKey: []byte("synthetic-draft-binding-key-32-bytes"),
		Review: func(ctx context.Context, r draftrecovery.ReviewRequest) (draftrecovery.Completion, error) {
			checks.Add(1)
			return controlledRecoveryReview(ctx, r)
		},
		Decisions: draftrecovery.DecisionFunc(func(context.Context, draftrecovery.Authority) (draftrecovery.Decision, error) {
			return draftrecovery.Decision{Generate: true, Publish: true, Read: true, Delivered: delivered.Load(), GrantExpiresAt: time.Now().Add(time.Hour)}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	router := accountapi.NewControlledDraftRecovery(f.Service, store, func(_ context.Context, g draftrecovery.Generation) (draftrecovery.Bundle, error) {
		generations.Add(1)
		return completeRecoveryBundle(g), nil
	})
	var doc profiledocument.Document
	if err = json.Unmarshal(f.Document, &doc); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(draftrecovery.Input{Profile: draftrecovery.Source{ID: doc.ID, Revision: doc.Revision}, Job: draftrecovery.Job{Posting: "Java required."}, Language: "en"})
	request := f.Request(t, ctx, "/api/drafts", input)
	request.Header.Del("X-OpenAI-Api-Key")
	request.Header.Del("X-TypeSafe-Api-Key")
	request.Header.Set("X-CareerOS-Action-Key", "review-exact")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	delivered.Store(true)
	command, _ := json.Marshal(map[string]any{"decisionKey": "review-summary", "expectedRevision": 0, "bundleDigest": created["bundleDigest"], "artifact": "summary", "operation": "review"})
	request = f.Request(t, ctx, "/api/drafts/review-exact/versions/1/review", command)
	request.Header.Del("X-OpenAI-Api-Key")
	request.Header.Del("X-TypeSafe-Api-Key")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Review struct {
			Revision  int                                `json:"revision"`
			Artifacts []struct{ Artifact, State string } `json:"artifacts"`
		} `json:"review"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Review.Revision != 1 || len(result.Review.Artifacts) != 4 || result.Review.Artifacts[0].Artifact != "summary" || result.Review.Artifacts[0].State != "reviewed" {
		t.Fatalf("review decision not persisted: %s", w.Body.String())
	}
	if generations.Load() != 1 || checks.Load() != 1 {
		t.Fatalf("decision ran inference: %d/%d", generations.Load(), checks.Load())
	}
}
