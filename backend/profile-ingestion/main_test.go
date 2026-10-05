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
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}
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
		if bytes.Contains(raw, []byte("SECRET SALARY")) || bytes.Contains(raw, []byte("PRIVATE NOTES")) || bytes.Contains(raw, []byte("SECRET CITY")) || bytes.Contains(raw, []byte("https://secret.example")) {
			t.Fatal("unrelated sensitive field sent")
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
func TestInvalidProviderResultNeverReturnsOperations(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return completion(`{"claims":[{"id":"c1","source":"React","text":"React","targets":["skills"],"question":""}]}`), nil
		}
		return completion(`{"operations":[{"claimId":"c1","target":"currentSalary","entryId":"","field":"currentSalary","action":"update","value":"$999","finding":"addition"}]}`), nil
	})}
	w := send(t, (app{client: client}).handler(), request{Input: "React", Profile: profile()})
	if w.Code != 502 || !strings.Contains(w.Body.String(), "invalid_output") {
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
