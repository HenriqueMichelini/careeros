package evaluationlab_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	lab "professional-information-repo/internal/evaluationlab"
	"testing"
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
