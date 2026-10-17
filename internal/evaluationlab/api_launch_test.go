package evaluationlab_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
	"testing/fstest"
)

func TestLoopbackLaunchShellNeverAuthorizesCrossSiteRequests(t *testing.T) {
	p := semanticPlanner(t)
	s, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := lab.NewRunner(p, lab.RunnerOptions{})
	api, err := lab.NewAPI(p, r, s, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	for _, site := range []string{"none", "same-origin", "cross-site"} {
		req := httptest.NewRequest(http.MethodGet, "/lab/", nil)
		req.Host = "127.0.0.1:8123"
		req.Header.Set("Sec-Fetch-Site", site)
		out := httptest.NewRecorder()
		api.ServeHTTP(out, req)
		if site == "cross-site" {
			if out.Code != 403 {
				t.Fatal("cross-site shell issued session")
			}
			continue
		}
		if out.Code != 200 || out.Header().Get("Content-Security-Policy") == "" || !bytes.Contains(out.Body.Bytes(), []byte("__LAB_SESSION__")) {
			t.Fatal("same-origin launch unavailable")
		}
	}
	old := api.Session()
	next, err := lab.NewAPI(p, r, s, lab.APIOptions{Origin: "http://127.0.0.1:8123"})
	if err != nil {
		t.Fatal(err)
	}
	if old.Authorization == next.Session().Authorization || old.CSRF == next.Session().CSRF {
		t.Fatal("session reused across launches")
	}
}

func TestAssetRoutesUseOnlyFixedCodeFiles(t *testing.T) {
	p := semanticPlanner(t)
	store, err := lab.OpenFileStore(filepath.Join(t.TempDir(), "history"), lab.StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runner := lab.NewRunner(p, lab.RunnerOptions{})
	assets := fstest.MapFS{"lab.js": {Data: []byte("safe-code")}, "lab.css": {Data: []byte("safe-style")}, "private.txt": {Data: []byte("must-not-be-served")}}
	api, err := lab.NewAPI(p, runner, store, lab.APIOptions{Origin: "http://127.0.0.1:8123", Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/lab/assets/lab.js", "/lab/assets/lab.css", "/lab/assets/private.txt", "/lab/assets/../private.txt", "/lab/assets/%2e%2e/private.txt"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Host = "127.0.0.1:8123"
		response := httptest.NewRecorder()
		api.ServeHTTP(response, req)
		allowed := path == "/lab/assets/lab.js" || path == "/lab/assets/lab.css"
		if allowed && response.Code != 200 {
			t.Fatal("fixed code asset unavailable")
		}
		if !allowed && response.Code != 404 {
			t.Fatal("unlisted asset route accepted")
		}
		if bytes.Contains(response.Body.Bytes(), []byte("must-not-be-served")) {
			t.Fatal("non-code file exposed")
		}
	}
}
