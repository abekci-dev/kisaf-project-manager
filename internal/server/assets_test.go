package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/abekci-dev/kisaf-project-manager/internal/config"
	"github.com/abekci-dev/kisaf-project-manager/internal/store"
)

func testServer(t *testing.T, files fstest.MapFS) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Path = "test-config.json"
	return New(cfg, st, files, func(string, ...any) {})
}

func get(s *Server, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "localhost"
	req.RemoteAddr = "127.0.0.1:5555"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// TestAssetsRevalidateInsteadOfExpiring is the regression guard for the upgrade
// bug: assets used to be served with max-age=300 and no validator, so replacing
// the binary left the browser running the previous UI for up to five minutes.
func TestAssetsRevalidateInsteadOfExpiring(t *testing.T) {
	s := testServer(t, fstest.MapFS{
		"index.html": {Data: []byte("<h1>hello</h1>")},
		"app.js":     {Data: []byte("console.log(1)")},
	})

	for _, path := range []string{"/", "/index.html", "/app.js"} {
		rec := get(s, path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, rec.Code)
			continue
		}
		cc := rec.Header().Get("Cache-Control")
		if strings.Contains(cc, "max-age") && !strings.Contains(cc, "max-age=0") {
			t.Errorf("%s: time based caching (%q) — goes stale across an upgrade", path, cc)
		}
		if rec.Header().Get("ETag") == "" {
			t.Errorf("%s: no ETag, the browser cannot revalidate", path)
		}
	}
}

func TestAssetETagChangesWithContent(t *testing.T) {
	a := testServer(t, fstest.MapFS{"app.js": {Data: []byte("old")}})
	b := testServer(t, fstest.MapFS{"app.js": {Data: []byte("new")}})

	first := get(a, "/app.js", nil).Header().Get("ETag")
	second := get(b, "/app.js", nil).Header().Get("ETag")

	if first == "" || second == "" {
		t.Fatal("no ETag was produced")
	}
	if first == second {
		t.Error("the content changed but the ETag did not — the browser keeps the old file")
	}
}

func TestAssetReturns304WhenUnchanged(t *testing.T) {
	s := testServer(t, fstest.MapFS{"app.js": {Data: []byte("console.log(1)")}})

	etag := get(s, "/app.js", nil).Header().Get("ETag")
	rec := get(s, "/app.js", map[string]string{"If-None-Match": etag})

	if rec.Code != http.StatusNotModified {
		t.Errorf("status %d, wanted 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("the 304 response carries a %d byte body", rec.Body.Len())
	}
}

// TestServiceWorkerCarriesVersion: the cache name must change between builds,
// otherwise an installed PWA keeps serving the previous shell.
func TestServiceWorkerCarriesVersion(t *testing.T) {
	old := Version
	Version = "9.9.9"
	defer func() { Version = old }()

	s := testServer(t, fstest.MapFS{
		"sw.js": {Data: []byte("const CACHE = 'kisaf-shell-__KISAF_VERSION__';")},
	})

	body := get(s, "/sw.js", nil).Body.String()
	if !strings.Contains(body, "kisaf-shell-9.9.9") {
		t.Errorf("the version was not embedded: %q", body)
	}
	if strings.Contains(body, "__KISAF_VERSION__") {
		t.Error("the placeholder was left in place")
	}
}

func TestUnknownAssetIs404(t *testing.T) {
	s := testServer(t, fstest.MapFS{"index.html": {Data: []byte("x")}})
	if rec := get(s, "/missing.js", nil); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, wanted 404", rec.Code)
	}
}

func TestIconsAreServedAndValidated(t *testing.T) {
	s := testServer(t, fstest.MapFS{"index.html": {Data: []byte("x")}})

	rec := get(s, "/icons/icon-192.png", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content type %q", ct)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("the icon was served without an ETag")
	}
	if get(s, "/icons/missing.png", nil).Code != http.StatusNotFound {
		t.Error("an unknown icon did not return 404")
	}
}

// ---------------------------------------------------------------------------
// Takeover endpoint
// ---------------------------------------------------------------------------

func postQuit(s *Server, remote string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/quit", nil)
	req.Host = "localhost"
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestQuitRequiresHeader(t *testing.T) {
	s := testServer(t, fstest.MapFS{"index.html": {Data: []byte("x")}})
	called := false
	s.SetOnQuit(func() { called = true })

	if rec := postQuit(s, "127.0.0.1:5555", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a request without the header returned %d, wanted 400", rec.Code)
	}
	if called {
		t.Error("shutdown was triggered without the header")
	}
}

func TestQuitRejectsRemoteCallers(t *testing.T) {
	s := testServer(t, fstest.MapFS{"index.html": {Data: []byte("x")}})
	called := false
	s.SetOnQuit(func() { called = true })

	// A remote caller is already stopped by auth; anything but 200 is fine here.
	rec := postQuit(s, "192.168.1.50:6000", map[string]string{"X-Kisaf-Quit": "1"})
	if rec.Code == http.StatusOK {
		t.Errorf("a remote caller was allowed to shut the server down (status %d)", rec.Code)
	}
	if called {
		t.Error("a remote request triggered shutdown")
	}
}

func TestQuitRejectsCrossSitePost(t *testing.T) {
	s := testServer(t, fstest.MapFS{"index.html": {Data: []byte("x")}})
	called := false
	s.SetOnQuit(func() { called = true })

	rec := postQuit(s, "127.0.0.1:5555", map[string]string{
		"X-Kisaf-Quit": "1",
		"Origin":       "http://evil-site.example",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("the cross origin request returned %d, wanted 403", rec.Code)
	}
	if called {
		t.Error("a request from another site shut the app down")
	}
}

func TestQuitAcceptsLocalUpgradeRequest(t *testing.T) {
	s := testServer(t, fstest.MapFS{"index.html": {Data: []byte("x")}})
	done := make(chan struct{})
	s.SetOnQuit(func() { close(done) })

	rec := postQuit(s, "127.0.0.1:5555", map[string]string{"X-Kisaf-Quit": "1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, wanted 200", rec.Code)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("shutdown was never called")
	}
}
