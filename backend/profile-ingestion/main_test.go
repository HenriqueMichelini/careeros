package profileingestion

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"professional-information-repo/internal/profilevalidation"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func completion(content string) *http.Response {
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}
}

func TestLargePasteUsesStrictSchemasAndEnoughOutputBudget(t *testing.T) {
	input := strings.Repeat("Worked with Java and PostgreSQL.\n", 180)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body struct {
			MaxCompletionTokens int `json:"max_completion_tokens"`
			ResponseFormat      struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Strict bool `json:"strict"`
					Schema struct {
						Required             []string `json:"required"`
						AdditionalProperties bool     `json:"additionalProperties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.ResponseFormat.Type != "json_schema" || !body.ResponseFormat.JSONSchema.Strict || body.ResponseFormat.JSONSchema.Schema.AdditionalProperties || body.MaxCompletionTokens < 6000 {
			t.Fatalf("call %d must request bounded strict output with sufficient budget", calls)
		}
		if calls == 1 {
			if !contains(body.ResponseFormat.JSONSchema.Schema.Required, "claims") {
				t.Fatal("missing claims requirement")
			}
			return completion(`{"claims":[{"id":"c1","source":"Worked with Java and PostgreSQL.","text":"Used Java and PostgreSQL","targets":["skills"],"question":""}]}`), nil
		}
		if !contains(body.ResponseFormat.JSONSchema.Schema.Required, "operations") {
			t.Fatal("missing operations requirement")
		}
		return completion(`{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"add","value":"Java and PostgreSQL","finding":"addition"}]}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: input, Profile: profile()})
	if w.Code != 200 || calls != 2 {
		t.Fatalf("status %d calls %d", w.Code, calls)
	}
}

func TestTruncatedProviderOutputIsRejectedBeforeParsing(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"choices":[{"finish_reason":"length","message":{"content":"{\"claims\":[]}"}}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "Used Java", Profile: profile()})
	if w.Code != 502 || !strings.Contains(w.Body.String(), `"error":"truncated"`) {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestWrappedSourceIsMappedToExactInputExcerpt(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return completion(`{"claims":[{"id":"c1","source":"Worked with Java and PostgreSQL.","text":"Used Java and PostgreSQL","targets":["skills"],"question":""}]}`), nil
	})}
	claims, skipped, code := (app{client: client}).extract(httptest.NewRequest("POST", "/", nil).Context(), "sk-test", "Worked with Java\nand PostgreSQL.")
	if code != "" || skipped != 0 || len(claims) != 1 || claims[0].Source != "Worked with Java\nand PostgreSQL." {
		t.Fatalf("code %s claims %#v", code, claims)
	}
}

func TestExtractionRejectionLogsOnlyReason(t *testing.T) {
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return completion(`{"claims":[{"id":"c1","source":"INVENTED SECRET","text":"SECRET FACT","targets":["skills"],"question":""}]}`), nil
	})}
	_, skipped, code := (app{client: client}).extract(httptest.NewRequest("POST", "/", nil).Context(), "sk-test", "Used Java")
	if code != "" || skipped != 1 || !strings.Contains(logs.String(), "reason=source") || strings.Contains(logs.String(), "INVENTED SECRET") || strings.Contains(logs.String(), "sk-test") {
		t.Fatal("extraction rejection must identify a safe reason without content or key")
	}
}

func TestSelfEmploymentProposalUsesPersistedStatus(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"Self-employed since 2024","text":"Self-employed since 2024","targets":["employmentStatus"],"question":""}]}`), nil
		}
		return completion(`{"operations":[{"claimId":"c1","target":"employmentStatus","entryId":"","field":"employmentStatus","action":"add","value":"Self-employed","finding":"addition"}]}`), nil
	})}
	p := profilevalidation.Profile{Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
	w := send(t, (app{client: client}).handler(), request{Input: "Self-employed since 2024", Profile: p})
	if w.Code != 200 || calls != 2 {
		t.Fatalf("status %d calls %d", w.Code, calls)
	}
	var result result
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Value != "freelance" {
		t.Fatalf("status proposal %#v", result.Operations)
	}
}

func TestUnsafeEntryWithholdsOnlyItsClaimGroup(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"Used Go","text":"Used Go","targets":["skills"],"question":""},{"id":"c2","source":"Built a project","text":"Built a project","targets":["projects"],"question":""}]}`), nil
		}
		return completion(`{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"add","value":"Go","finding":"addition"},{"claimId":"c2","target":"projects","entryId":"p1","field":"description","action":"add","value":"Built a project","finding":"in_place"},{"claimId":"c2","target":"projects","entryId":"unknown","field":"name","action":"add","value":"Project","finding":"addition"}]}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "Used Go\nBuilt a project", Profile: profile()})
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got struct {
		Operations         []operation `json:"operations"`
		UnresolvedClaimIds []string    `json:"unresolvedClaimIds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Operations) != 1 || got.Operations[0].ClaimID != "c1" || len(got.UnresolvedClaimIds) != 1 || got.UnresolvedClaimIds[0] != "c2" {
		t.Fatalf("unexpected result %#v", got)
	}
}

func TestRelatedClaimsFillOneNewExperienceAndProject(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"Engineer at Aster Labs","text":"Engineer at Aster Labs","targets":["experience"],"question":""},{"id":"c2","source":"Built payment APIs","text":"Built payment APIs","targets":["experience"],"question":""},{"id":"c3","source":"Cut validation to 20 seconds","text":"Cut validation to 20 seconds","targets":["experience"],"question":""},{"id":"c4","source":"Harbor inventory project","text":"Harbor inventory project","targets":["projects"],"question":""},{"id":"c5","source":"Added audit trails with Go","text":"Added audit trails with Go","targets":["projects"],"question":""}]}`), nil
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte("one Experience entry")) || !bytes.Contains(body, []byte("Responsibilities: distinct actions")) || !bytes.Contains(body, []byte("existing Profile field repeats")) || !bytes.Contains(body, []byte("anchor's identity plus a linked claim")) {
			t.Fatal("comparison prompt must request grouped, detailed, concise fields and reviewed cleanup")
		}
		return completion(`{"operations":[{"claimId":"c1","target":"experience","entryId":"new:c1","field":"company","action":"add","value":"Aster Labs","finding":"addition"},{"claimId":"c1","target":"experience","entryId":"new:c1","field":"title","action":"add","value":"Engineer","finding":"addition"},{"claimId":"c2","target":"experience","entryId":"new:c1","field":"responsibilities","action":"add","value":"Built payment APIs","finding":"addition"},{"claimId":"c3","target":"experience","entryId":"new:c1","field":"achievements","action":"add","value":"Cut validation to 20 seconds","finding":"addition"},{"claimId":"c4","target":"projects","entryId":"new:c4","field":"name","action":"add","value":"Harbor","finding":"addition"},{"claimId":"c4","target":"projects","entryId":"new:c4","field":"description","action":"add","value":"Inventory project","finding":"addition"},{"claimId":"c5","target":"projects","entryId":"new:c4","field":"highlights","action":"add","value":"Added audit trails with Go","finding":"addition"}]}`), nil
	})}
	p := profilevalidation.Profile{Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
	w := send(t, (app{client: client}).handler(), request{Input: "Engineer at Aster Labs\nBuilt payment APIs\nCut validation to 20 seconds\nHarbor inventory project\nAdded audit trails with Go", Profile: p})
	if w.Code != 200 || calls != 2 {
		t.Fatalf("status %d calls %d", w.Code, calls)
	}
	var got result
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Operations) != 7 || len(got.UnresolvedClaimIds) != 0 {
		t.Fatalf("operations %d unresolved %v", len(got.Operations), got.UnresolvedClaimIds)
	}
}

func TestUnsafeNewEntryWithholdsAllLinkedClaims(t *testing.T) {
	claims := []claim{{ID: "c1", Targets: []string{"experience"}}, {ID: "c2", Targets: []string{"experience"}}}
	valid := []operation{{ClaimID: "c1", Target: "experience", EntryID: "new:c1", Field: "company", Action: "add", Value: "Aster"}, {ClaimID: "c1", Target: "experience", EntryID: "new:c1", Field: "title", Action: "add", Value: "Engineer"}}
	raw := append(append([]operation{}, valid...), operation{ClaimID: "c2", Target: "experience", EntryID: "new:c1", Field: "responsibilities", Action: "add", Value: "Built APIs"})
	filtered, unresolved := filterUnsafeNewGroups(valid, raw, claims, map[string]bool{"c2": true}, nil)
	if len(filtered) != 0 || len(unresolved) != 2 {
		t.Fatalf("filtered %#v unresolved %#v", filtered, unresolved)
	}
}

func TestUnknownClaimWithholdsItsNewEntryGroup(t *testing.T) {
	claims := []claim{{ID: "c1", Targets: []string{"projects"}}}
	valid := []operation{{ClaimID: "c1", Target: "projects", EntryID: "new:c1", Field: "name", Action: "add", Value: "Harbor"}}
	raw := append(append([]operation{}, valid...), operation{ClaimID: "unknown", Target: "projects", EntryID: "new:c1", Field: "highlights", Action: "add", Value: "Invented result"})
	filtered, unresolved := filterUnsafeNewGroups(valid, raw, claims, map[string]bool{}, map[string]bool{"projects/new:c1": true})
	if len(filtered) != 0 || len(unresolved) != 1 || unresolved[0] != "c1" {
		t.Fatalf("filtered %#v unresolved %#v", filtered, unresolved)
	}
}

func TestRepeatedFactProducesOneSuggestion(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"Built payment APIs","text":"Built payment APIs","targets":["experience"],"question":""},{"id":"c2","source":"Built payment APIs.","text":"Built payment APIs","targets":["experience"],"question":""}]}`), nil
		}
		return completion(`{"operations":[{"claimId":"c1","target":"experience","entryId":"e1","field":"responsibilities","action":"add","value":"Built payment APIs","finding":"in_place"},{"claimId":"c2","target":"experience","entryId":"e1","field":"responsibilities","action":"add","value":"Built payment APIs.","finding":"in_place"}]}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "Built payment APIs\nBuilt payment APIs.", Profile: profile()})
	var got result
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(got.Operations) != 1 || len(got.UnresolvedClaimIds) != 0 {
		t.Fatalf("status %d operations %#v unresolved %#v", w.Code, got.Operations, got.UnresolvedClaimIds)
	}
}

func TestRepeatedCapabilityAcrossSectionsIsSuggestedOnlyOnce(t *testing.T) {
	p := profilevalidation.Profile{Skills: "React\nDocker"}
	ops := []operation{
		{Target: "skills", Field: "skills", Action: "add", Value: "Go"},
		{Target: "tools", Field: "tools", Action: "add", Value: "Go."},
		{Target: "tools", Field: "tools", Action: "add", Value: "React"},
		{Target: "projects", Field: "technologies", Action: "add", Value: "Go, React"},
	}
	got := suppressRepeatedCapabilityFacts(ops, p)
	if len(got) != 2 || got[0].Target != "skills" || got[1].Target != "projects" {
		t.Fatalf("unexpected suggestions %#v", got)
	}
}

func TestMovingCapabilityKeepsDestinationSuggestion(t *testing.T) {
	p := profilevalidation.Profile{Skills: "Go"}
	ops := []operation{
		{Target: "skills", Field: "skills", Action: "remove"},
		{Target: "tools", Field: "tools", Action: "add", Value: "Go"},
	}
	got := suppressRepeatedCapabilityFacts(ops, p)
	if len(got) != 2 {
		t.Fatalf("moving a fact between sections needs both reviewable changes: %#v", got)
	}
}

func TestUnverifiableSourceDoesNotHideOtherClaims(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return completion(`{"claims":[{"id":"c1","source":"Used Go","text":"Used Go","targets":["skills"],"question":""},{"id":"c2","source":"Invented source","text":"Invented","targets":["skills"],"question":""}]}`), nil
	})}
	claims, skipped, code := (app{client: client}).extract(httptest.NewRequest("POST", "/", nil).Context(), "sk-test", "Used Go")
	if code != "" || len(claims) != 1 || claims[0].ID != "c1" || skipped != 1 {
		t.Fatalf("claims %#v skipped %d code %s", claims, skipped, code)
	}
}
func profile() profilevalidation.Profile {
	return profilevalidation.Profile{EmploymentStatus: "", Skills: "React", CurrentSalary: "SECRET SALARY", AdditionalInfo: "PRIVATE NOTES", Experience: []profilevalidation.Experience{{ID: "e1", Company: "Acme", Title: "Engineer", Location: "SECRET CITY", Achievements: "Built a search tool"}}, Projects: []profilevalidation.Project{{ID: "p1", Name: "Side project", URL: "https://secret.example"}}}
}
func send(t *testing.T, h http.Handler, in request) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(in)
	r := httptest.NewRequest("POST", "/api/profile/ingest", bytes.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestProjectionSendsOnlyClaimDestinations(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if calls == 1 {
			if bytes.Contains(raw, []byte("SECRET SALARY")) {
				t.Fatal("profile sent during extraction")
			}
			return completion(`{"claims":[{"id":"c1","source":"I used React at Acme","text":"Used React at Acme","targets":["skills","experience"],"question":""}]}`), nil
		}
		if bytes.Contains(raw, []byte("SECRET SALARY")) || bytes.Contains(raw, []byte("PRIVATE NOTES")) || bytes.Contains(raw, []byte("SECRET CITY")) || bytes.Contains(raw, []byte("https://secret.example")) || bytes.Contains(raw, []byte(`"p1"`)) {
			t.Fatal("unrelated sensitive field sent")
		}
		if !bytes.Contains(raw, []byte(`"enum":["","new:c1","e1"]`)) {
			t.Fatal("comparison response must be constrained to relevant exact entry IDs")
		}
		if !bytes.Contains(raw, []byte("Built a search tool")) || !bytes.Contains(raw, []byte("Acme")) {
			t.Fatal("relevant experience omitted")
		}
		return completion(`{"operations":[{"claimId":"c1","target":"skills","entryId":"","field":"skills","action":"add","value":"React at Acme","finding":"overlap"},{"claimId":"c1","target":"experience","entryId":"e1","field":"description","action":"add","value":"Used React","finding":"in_place"}]}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "I used React at Acme", Profile: profile()})
	if w.Code != 200 || calls != 2 {
		t.Fatalf("status %d calls %d: %s", w.Code, calls, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("response must not be cached")
	}
	var result result
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Operations) != 2 {
		t.Fatal(w.Body.String())
	}
}
func TestUnsafeOperationNeverReturnsItsClaimGroup(t *testing.T) {
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"React","text":"React","targets":["skills"],"question":""}]}`), nil
		}
		return completion(`{"operations":[{"claimId":"c1","target":"currentSalary","entryId":"","field":"currentSalary","action":"update","value":"$999","finding":"addition"}]}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "React", Profile: profile()})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"unresolvedClaimIds":["c1"]`) || !strings.Contains(w.Body.String(), `"operations":[]`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), "reason=claim_target") || strings.Contains(logs.String(), "$999") || strings.Contains(logs.String(), "sk-test") {
		t.Fatal("comparison rejection must identify a safe reason without content or key")
	}
}
func TestMalformedProviderResultNeverReturnsOperations(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"React","text":"React","targets":["skills"],"question":""}]}`), nil
		}
		return completion(`{"operations":[],"profile":{"skills":"unauthorized"}}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "React", Profile: profile()})
	if w.Code != 502 || !strings.Contains(w.Body.String(), "invalid_output") || strings.Contains(w.Body.String(), "operations") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestInputAndErrorsAreNoStore(t *testing.T) {
	h := NewHandler()
	w := send(t, h, request{Input: "", Profile: profile()})
	if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestProjectionMatchesAcrossSectionsWithoutLeakingDates(t *testing.T) {
	p := profile()
	p.Tools = "React"
	p.Experience[0].StartDate = "SECRET START DATE"
	p.Experience[0].EndDate = "SECRET END DATE"
	projected := projection([]claim{{ID: "c1", Source: "Improved conversion by 20% with React", Targets: []string{"experience", "skills"}}}, p)
	encoded := string(mustJSON(projected))
	if !strings.Contains(encoded, "related_tools") || strings.Contains(encoded, "SECRET START DATE") || strings.Contains(encoded, "SECRET SALARY") || strings.Contains(encoded, "SECRET CITY") {
		t.Fatal(encoded)
	}
	if mentionsDate("Won a 2023 award") {
		t.Fatal("award year is not an employment date")
	}
	if mentionsDate("Won an award in March 2023") || mentionsDate("Led a campaign 2021–2023") {
		t.Fatal("achievement dates must not disclose employment dates")
	}
	if !mentionsDate("Worked from 2021 to 2023") {
		t.Fatal("employment years should allow date comparison")
	}
	p.Tools = "JS"
	semantic := projection([]claim{{ID: "c2", Source: "JavaScript", Targets: []string{"skills"}}}, p)
	if semantic["related_tools"] != "JS" {
		t.Fatal("cross-section synonym context missing")
	}
	p.EmploymentStatus = "Employed"
	if !validInput(request{Input: "React", Profile: p}) {
		t.Fatal("legacy saved status should remain usable")
	}
}

func TestOnboardingAndCompensationProjection(t *testing.T) {
	empty := profilevalidation.Profile{Experience: []profilevalidation.Experience{}, Projects: []profilevalidation.Project{}}
	if !validInput(request{Input: "I learned Go", Profile: empty}) {
		t.Fatal("empty Profile should be accepted")
	}
	p := profile()
	p.DesiredSalary = "SECRET DESIRED"
	out := projection([]claim{{ID: "c1", Source: "My current salary is $80k", Targets: []string{"currentSalary"}}}, p)
	encoded := string(mustJSON(out))
	if !strings.Contains(encoded, "SECRET SALARY") || strings.Contains(encoded, "SECRET DESIRED") || strings.Contains(encoded, "PRIVATE NOTES") {
		t.Fatal(encoded)
	}
}

func TestMetadataLogOmitsContentAndKey(t *testing.T) {
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	h := NewHandler()
	body := `{"input":"SECRET INPUT","profile":{}}`
	r := httptest.NewRequest("POST", "/api/profile/ingest", strings.NewReader(body))
	r.Header.Set("X-OpenAI-Api-Key", "sk-SECRETKEY")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 || strings.Contains(logs.String(), "SECRET INPUT") || strings.Contains(logs.String(), "SECRETKEY") || !strings.Contains(logs.String(), "status=400") {
		t.Fatal(logs.String())
	}
}
