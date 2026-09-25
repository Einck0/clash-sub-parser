package webassets_test

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clash-sub-parser/internal/webassets"
)

func TestFS(t *testing.T) {
	subFS, err := webassets.FS()
	if err != nil {
		t.Fatalf("FS() returned unexpected error: %v", err)
	}
	if subFS == nil {
		t.Fatal("FS() returned nil filesystem")
	}

	f, err := subFS.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open index.html from embedded FS: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}

	if !strings.Contains(string(content), "<div id=\"app\">") {
		t.Errorf("index.html missing <div id=\"app\">, got: %s", string(content))
	}
}

func TestHandler_Index(t *testing.T) {
	handler, err := webassets.Handler()
	if err != nil {
		t.Fatalf("Handler() returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "CSP Control Plane") && !strings.Contains(body, "csp") {
		t.Errorf("expected body to contain title, got: %s", body)
	}
}

func TestHandler_SPAFallback(t *testing.T) {
	handler, err := webassets.Handler()
	if err != nil {
		t.Fatalf("Handler() returned error: %v", err)
	}

	routes := []string{
		"/nodes",
		"/nodes/detail/node-123",
		"/subscriptions",
		"/policy",
		"/settings/theme",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200 for %s, got %d", route, rec.Code)
			}

			contentType := rec.Header().Get("Content-Type")
			if !strings.Contains(contentType, "text/html") {
				t.Errorf("expected text/html for %s, got %q", route, contentType)
			}

			body := rec.Body.String()
			if !strings.Contains(body, "<div id=\"app\">") {
				t.Errorf("expected SPA index fallback for %s", route)
			}
		})
	}
}

func TestHandler_StaticAsset(t *testing.T) {
	subFS, err := webassets.FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}

	// Check if there are any static assets in dist (e.g. favicon or svg or assets)
	var foundAsset string
	_ = fs.WalkDir(subFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && path != "index.html" {
			foundAsset = "/" + path
			return fs.SkipAll
		}
		return nil
	})

	if foundAsset != "" {
		handler, err := webassets.Handler()
		if err != nil {
			t.Fatalf("Handler() error: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, foundAsset, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for static asset %s, got %d", foundAsset, rec.Code)
		}
	}
}
