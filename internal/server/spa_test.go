package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestServer_EmbeddedSPA_Routes(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	handler := srv.Router()

	// 1. Health check must work as before
	t.Run("GET /health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got: %d", rec.Code)
		}
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["status"] != "ok" {
			t.Errorf("expected status ok, got: %v", body["status"])
		}
	})

	// 2. Root URL GET / must return index.html
	t.Run("GET / (Root SPA)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got: %d", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "text/html") {
			t.Errorf("expected text/html, got: %s", ct)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "<html") {
			t.Errorf("expected HTML content, got: %s", body)
		}
	})

	// 3. Client-side routes must fallback to index.html
	clientRoutes := []string{
		"/subscriptions",
		"/nodes",
		"/rules",
		"/settings",
		"/dashboard/metrics",
	}
	for _, route := range clientRoutes {
		t.Run("Fallback "+route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for route %s, got: %d", route, rec.Code)
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, "text/html") {
				t.Errorf("expected text/html for %s, got: %s", route, ct)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "<html") {
				t.Errorf("expected index.html content for %s", route)
			}
		})
	}

	// 4. Embedded static assets under /assets/
	t.Run("GET /assets static file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/index-CetPrhTk.js", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for index JS, got: %d", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "application/javascript") {
			t.Errorf("expected javascript content-type, got: %s", ct)
		}
		cc := rec.Header().Get("Cache-Control")
		if !strings.Contains(cc, "immutable") {
			t.Errorf("expected immutable Cache-Control, got: %s", cc)
		}
	})

	// 5. Missing asset must return 404, not index.html
	t.Run("GET /assets missing file returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/does-not-exist.js", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing asset, got: %d", rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, "<html") {
			t.Errorf("missing asset must NOT return index.html")
		}
	})

	// 6. Unknown API route must return 404 JSON, not index.html
	t.Run("GET /api unknown route returns 404 JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/unknown-service-path", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown api, got: %d", rec.Code)
		}
		ct := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			t.Errorf("expected application/json, got: %s", ct)
		}
	})

	// 7. Client export endpoint must continue working (/clash)
	t.Run("GET /clash export route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/clash", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for /clash, got: %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "proxies:") {
			t.Errorf("expected clash config body, got: %s", body)
		}
	})
}

func TestServer_CustomMountFrontend(t *testing.T) {
	srv, _, _ := setupTestServer(t)

	customFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!DOCTYPE html><html><body>CUSTOM_SPA</body></html>"),
		},
	}

	srv.MountFrontend(customFS)
	handler := srv.Router()

	req := httptest.NewRequest(http.MethodGet, "/any-page", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got: %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "CUSTOM_SPA") {
		t.Errorf("expected CUSTOM_SPA content, got: %s", string(body))
	}
}
