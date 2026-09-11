package web_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"clash-sub-parser/frontend"
	"clash-sub-parser/internal/server/web"
)

func createTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!DOCTYPE html><html><head><title>CSP</title></head><body><div id=\"app\"></div></body></html>"),
		},
		"assets/app.js": &fstest.MapFile{
			Data: []byte("console.log('csp test');"),
		},
		"assets/style.css": &fstest.MapFile{
			Data: []byte("body { background: #000; }"),
		},
		"assets/logo.svg": &fstest.MapFile{
			Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"),
		},
		"favicon.ico": &fstest.MapFile{
			Data: []byte("fake-ico-bytes"),
		},
	}
}

func TestSPAHandler_RootIndexHTML(t *testing.T) {
	fsys := createTestFS()
	handler := web.NewSPAHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got: %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got: %s", ct)
	}

	cc := res.Header.Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("expected Cache-Control to contain no-cache, got: %s", cc)
	}

	nosniff := res.Header.Get("X-Content-Type-Options")
	if nosniff != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got: %s", nosniff)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "<div id=\"app\"></div>") {
		t.Errorf("expected body to contain div#app, got: %s", string(body))
	}
}

func TestSPAHandler_SPAFallback(t *testing.T) {
	fsys := createTestFS()
	handler := web.NewSPAHandler(fsys)

	routes := []string{
		"/subscriptions",
		"/nodes",
		"/rules",
		"/settings",
		"/node-groups/1/detail",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Fatalf("route %s expected 200, got: %d", route, res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "text/html") {
				t.Errorf("route %s expected text/html, got: %s", route, ct)
			}

			body, _ := io.ReadAll(res.Body)
			if !strings.Contains(string(body), "<div id=\"app\"></div>") {
				t.Errorf("route %s expected fallback index.html, got: %s", route, string(body))
			}
		})
	}
}

func TestSPAHandler_StaticAssets(t *testing.T) {
	fsys := createTestFS()
	handler := web.NewSPAHandler(fsys)

	tests := []struct {
		path         string
		expectedType string
		checkCache   bool
	}{
		{path: "/assets/app.js", expectedType: "application/javascript", checkCache: true},
		{path: "/assets/style.css", expectedType: "text/css", checkCache: true},
		{path: "/assets/logo.svg", expectedType: "image/svg+xml", checkCache: true},
		{path: "/favicon.ico", expectedType: "image/x-icon", checkCache: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Fatalf("expected status 200, got: %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, tt.expectedType) {
				t.Errorf("expected Content-Type starting with %s, got: %s", tt.expectedType, ct)
			}

			if tt.checkCache {
				cc := res.Header.Get("Cache-Control")
				if !strings.Contains(cc, "max-age=31536000") || !strings.Contains(cc, "immutable") {
					t.Errorf("expected immutable cache control for %s, got: %s", tt.path, cc)
				}
			}
		})
	}
}

func TestSPAHandler_MissingAssetReturns404(t *testing.T) {
	fsys := createTestFS()
	handler := web.NewSPAHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/assets/not-found-chunk.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404 for missing asset, got: %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if strings.Contains(string(body), "<div id=\"app\">") {
		t.Errorf("missing asset must NOT fall back to index.html!")
	}
}

func TestSPAHandler_APIRoutesReturn404(t *testing.T) {
	fsys := createTestFS()
	handler := web.NewSPAHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/api/unknown-endpoint", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404 for /api/ unknown route, got: %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json error response, got: %s", ct)
	}

	var jsonErr map[string]any
	if err := json.NewDecoder(res.Body).Decode(&jsonErr); err != nil {
		t.Fatalf("failed to decode JSON error: %v", err)
	}
	if jsonErr["error"] != "not found" {
		t.Errorf("expected error 'not found', got: %v", jsonErr["error"])
	}
}

func TestSPAHandler_MethodNotAllowed(t *testing.T) {
	fsys := createTestFS()
	handler := web.NewSPAHandler(fsys)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, m := range methods {
		req := httptest.NewRequest(m, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		res := rec.Result()
		_ = res.Body.Close()
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("method %s expected 405, got: %d", m, res.StatusCode)
		}
	}
}

func TestSPAHandler_HEADRequest(t *testing.T) {
	fsys := createTestFS()
	handler := web.NewSPAHandler(fsys)

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("HEAD expected status 200, got: %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html for HEAD, got: %s", ct)
	}

	body, _ := io.ReadAll(res.Body)
	if len(body) > 0 {
		t.Errorf("HEAD request must return empty body, got %d bytes", len(body))
	}
}

func TestSPAHandler_RealEmbeddedFS(t *testing.T) {
	realFS, err := frontend.FS()
	if err != nil {
		t.Fatalf("failed to get real embedded frontend FS: %v", err)
	}

	handler := web.NewSPAHandler(realFS)

	// 1. Test root /
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("real embedded root expected 200, got: %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "<html") {
		t.Errorf("expected real embedded index.html content")
	}

	// 2. Test fallback /subscriptions
	req2 := httptest.NewRequest(http.MethodGet, "/subscriptions", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	res2 := rec2.Result()
	defer res2.Body.Close()

	if res2.StatusCode != http.StatusOK {
		t.Fatalf("real embedded fallback expected 200, got: %d", res2.StatusCode)
	}
}
