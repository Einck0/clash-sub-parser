// Package webassets provides embedded static assets for the CSP control plane web interface.
package webassets

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// DistFS contains all embedded files under the dist directory.
//
//go:embed all:dist
var DistFS embed.FS

// FS returns an fs.FS rooted at the dist directory.
func FS() (fs.FS, error) {
	sub, err := fs.Sub(DistFS, "dist")
	if err != nil {
		return nil, fmt.Errorf("failed to derive sub-filesystem for dist: %w", err)
	}
	return sub, nil
}

// Handler returns an http.Handler that serves the embedded web application.
// Any existing static asset under dist/ is served directly.
// Any route that does not match a physical static file falls back to index.html (SPA routing).
func Handler() (http.Handler, error) {
	rootFS, err := FS()
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(rootFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := path.Clean(r.URL.Path)
		trimmed := strings.TrimPrefix(reqPath, "/")

		if trimmed == "" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if file exists in embedded fs and is a regular file
		f, err := rootFS.Open(trimmed)
		if err == nil {
			stat, statErr := f.Stat()
			if statErr == nil && !stat.IsDir() {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			_ = f.Close()
		}

		// Fallback to index.html for SPA client-side routes
		indexFile, err := rootFS.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer indexFile.Close()

		stat, err := indexFile.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if seeker, ok := indexFile.(io.ReadSeeker); ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", stat.ModTime(), seeker)
			return
		}

		data, err := io.ReadAll(indexFile)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}), nil
}
