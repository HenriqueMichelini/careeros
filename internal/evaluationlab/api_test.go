package evaluationlab_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAPIProtectsCatalogReadsAndMutations(t *testing.T) {
	p := semanticPlanner(t)
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := lab.NewRunner(p, lab.RunnerOptions{Retain: s.Save, BeforeDispatch: s.Health})
	api, err := lab.NewAPI(p, r, s, lab.APIOptions{Origin: "http://127.0.0.1:8123", Context: context.Background()})
	if err != nil {
		t.Fatal(err)
	}
	session := api.Session()
	for _, tc := range []struct {
		name, host, origin, auth, csrf, method, path string
		want                                         int
	}{
		{name: "unauthorized read", host: "127.0.0.1:8123", method: "GET", path: "/lab/api/suites", want: 403},
		{name: "allowed read", host: "127.0.0.1:8123", auth: session.Authorization, method: "GET", path: "/lab/api/suites", want: 200},
		{name: "rebound host", host: "attacker.test:8123", auth: session.Authorization, method: "GET", path: "/lab/api/suites", want: 403},
		{name: "cross origin", host: "127.0.0.1:8123", origin: "http://attacker.test", auth: session.Authorization, method: "GET", path: "/lab/api/suites", want: 403},
		{name: "no csrf", host: "127.0.0.1:8123", origin: "http://127.0.0.1:8123", auth: session.Authorization, method: "POST", path: "/lab/api/plans", want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Host = tc.host
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Authorization", tc.auth)
			req.Header.Set("X-Lab-CSRF", tc.csrf)
			out := httptest.NewRecorder()
			api.ServeHTTP(out, req)
			if out.Code != tc.want {
				t.Fatalf("status=%d want%d", out.Code, tc.want)
			}
			if out.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable sensitive response")
			}
			if out.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("unexpected CORS")
			}
		})
	}
	if _, err = lab.NewAPI(p, r, s, lab.APIOptions{Origin: "http://0.0.0.0:8123"}); err == nil {
		t.Fatal("non-loopback origin accepted")
	}
}

func TestAPIAdmissionIsIdempotentAndCancellationStopsDispatch(t *testing.T) {
	p := semanticPlanner(t)
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	entered := make(chan struct{})
	var once sync.Once
	r := lab.NewRunner(p, lab.RunnerOptions{Retain: s.Save, BeforeDispatch: func(ctx context.Context) error { once.Do(func() { close(entered) }); <-ctx.Done(); return ctx.Err() }})
	api, err := lab.NewAPI(p, r, s, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	call := func(path string, input any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(input)
		request := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		request.Host = "127.0.0.1:8123"
		request.Header.Set("Origin", "http://127.0.0.1:8123")
		request.Header.Set("Authorization", api.Session().Authorization)
		request.Header.Set("X-Lab-CSRF", api.Session().CSRF)
		request.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		api.ServeHTTP(out, request)
		return out
	}
	out := call("/lab/api/plans", map[string]any{"selection": lab.Selection{Suite: "semantic-quality", Mode: "controlled", Configuration: "current", CaseIDs: []string{"en-fact"}}, "caps": factPlan(t, p).Caps})
	if out.Code != 200 {
		t.Fatalf("plan failed: %d", out.Code)
	}
	var plan lab.Plan
	if err = json.Unmarshal(out.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	ids := make(chan string, 8)
	statuses := make(chan int, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := call("/lab/api/runs", map[string]any{"plan": plan, "idempotencyToken": "same-token-0000000000000000"})
			var result struct {
				ID string `json:"id"`
			}
			json.Unmarshal(response.Body.Bytes(), &result)
			ids <- result.ID
			statuses <- response.Code
		}()
	}
	wg.Wait()
	close(ids)
	close(statuses)
	id := ""
	created := 0
	for current := range ids {
		if current == "" {
			t.Fatal("missing id")
		}
		if id != "" && id != current {
			t.Fatal("token redispatched")
		}
		id = current
	}
	for status := range statuses {
		if status == 201 {
			created++
		} else if status != 200 {
			t.Fatalf("race status%d", status)
		}
	}
	if created != 1 {
		t.Fatalf("admitted%d", created)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("execution did not start")
	}
	if out = call("/lab/api/runs", map[string]any{"plan": plan, "idempotencyToken": "unrelated-token-000000000000"}); out.Code != 409 {
		t.Fatal("busy admission missing")
	}

	otherPlan, err := p.Plan(plan.Selection, plan.Caps)
	if err != nil {
		t.Fatal(err)
	}
	if out = call("/lab/api/runs", map[string]any{"plan": otherPlan, "idempotencyToken": "same-token-0000000000000000"}); out.Code != 409 {
		t.Fatal("token reused for another frozen plan")
	}
	altered := plan
	altered.Caps.MaxProviderCalls++
	if out = call("/lab/api/runs", map[string]any{"plan": altered, "idempotencyToken": "same-token-0000000000000000"}); out.Code != 400 {
		t.Fatal("changed plan accepted")
	}
	for i := 0; i < 2; i++ {
		if out = call("/lab/api/runs/"+id+"/cancel", struct{}{}); out.Code != 200 {
			t.Fatal("cancel not idempotent")
		}
	}
	record := awaitRun(t, r, id)
	if record.EngineState != "canceled" || len(record.Dispatches) != 0 {
		t.Fatal("cancel dispatched")
	}
	saved, err := s.Load(id)
	if err != nil || saved.EngineState != "canceled" {
		t.Fatalf("terminal cancellation not durable: %v", err)
	}
}

func TestAPIHistoryImportExportAndBounds(t *testing.T) {
	p := semanticPlanner(t)
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := lab.NewRunner(p, lab.RunnerOptions{Retain: s.Save, BeforeDispatch: s.Health, MaxEvents: 3})
	api, err := lab.NewAPI(p, r, s, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, raw []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Host = "127.0.0.1:8123"
		req.Header.Set("Authorization", api.Session().Authorization)
		req.Header.Set("Origin", "http://127.0.0.1:8123")
		req.Header.Set("X-Lab-CSRF", api.Session().CSRF)
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		api.ServeHTTP(out, req)
		return out
	}
	id, err := r.Start(context.Background(), factPlan(t, p))
	if err != nil {
		t.Fatal(err)
	}
	record := awaitRun(t, r, id)
	for _, path := range []string{"/lab/api/runs?limit=1", "/lab/api/runs/" + id, "/lab/api/runs/" + id + "/cases/" + record.Cases[0].Identity, "/lab/api/runs/" + id + "/exports?format=diagnostics", "/lab/api/runs/" + id + "/exports?format=json"} {
		if out := request("GET", path, nil); out.Code != 200 {
			t.Fatalf("read %s failed%d", path, out.Code)
		}
	}
	events := request("GET", "/lab/api/runs/"+id+"/events?after=0&limit=3", nil)
	var page struct {
		Gap   bool   `json:"gap"`
		Epoch string `json:"epoch"`
	}
	json.Unmarshal(events.Body.Bytes(), &page)
	if events.Code != 200 || !page.Gap || page.Epoch == "" {
		t.Fatal("event ring gap not explicit")
	}
	if out := request("GET", "/lab/api/runs/"+id+"/events?after=0&epoch=old-session", nil); out.Code != 409 {
		t.Fatal("stale event cursor accepted")
	}
	raw, err := os.ReadFile("../../docs/evaluations/semantic-quality/controlled.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"originalBytes": raw})
	out := request("POST", "/lab/api/imports", input)
	if out.Code != 201 {
		t.Fatalf("import failed%d", out.Code)
	}
	var imported struct {
		ID string `json:"id"`
	}
	json.Unmarshal(out.Body.Bytes(), &imported)
	if request("GET", "/lab/api/runs/"+imported.ID+"/exports?format=original", nil).Code != 200 {
		t.Fatal("retained export failed")
	}
	for _, payload := range []string{`{"selection":{"suite":"semantic-quality","mode":"live","configuration":"current"},"caps":{}}`, `{"path":"/etc/passwd"}`, `{"providerHost":"https://evil.test","credentials":"secret"}`} {
		if out := request("POST", "/lab/api/plans", []byte(payload)); out.Code != 400 {
			t.Fatal("unsupported execution input accepted")
		}
	}
	if out := request("POST", "/lab/api/imports", bytes.Repeat([]byte("x"), 17<<20)); out.Code != 400 {
		t.Fatal("oversized upload accepted")
	}

	comparison, _ := json.Marshal(map[string]any{"leftId": id, "rightId": id, "changedDimension": "configuration"})
	if out := request("POST", "/lab/api/comparisons", comparison); out.Code != 200 {
		t.Fatal("comparison contract unavailable")
	}
	unknown, _ := json.Marshal(map[string]any{"originalBytes": []byte(`{"unknown":true}`)})
	quarantinedResponse := request("POST", "/lab/api/imports", unknown)
	var quarantined struct {
		ID string `json:"id"`
	}
	json.Unmarshal(quarantinedResponse.Body.Bytes(), &quarantined)
	comparison, _ = json.Marshal(map[string]any{"leftId": id, "rightId": quarantined.ID, "changedDimension": "configuration"})
	if out := request("POST", "/lab/api/comparisons", comparison); out.Code != 409 {
		t.Fatal("quarantine entered trusted comparison")
	}

	nested := []byte(`{"originalBytes":` + strings.Repeat("[", 40) + `0` + strings.Repeat("]", 40) + `}`)
	if out := request("POST", "/lab/api/imports", nested); out.Code != 400 {
		t.Fatal("deep JSON accepted")
	}
	if out := request("GET", "/lab/api/runs/"+id+"/events?after=999999", nil); out.Code != 400 {
		t.Fatal("future event cursor accepted")
	}
	if out := request("GET", "/lab/api/runs?path=/etc/passwd", nil); out.Code != 400 {
		t.Fatal("arbitrary query accepted")
	}
}
