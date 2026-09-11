package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"clash-sub-parser/frontend"
)

// DistFS returns the embedded frontend filesystem rooted at dist.
func DistFS() (fs.FS, error) {
	return frontend.FS()
}

// SPAHandler serves embedded frontend assets with SPA fallback routing.
type SPAHandler struct {
	fsys fs.FS
}

// NewSPAHandler constructs an http.Handler serving the given filesystem as an SPA.
func NewSPAHandler(fsys fs.FS) http.Handler {
	return &SPAHandler{fsys: fsys}
}

// ServeHTTP handles incoming HTTP requests for static assets and SPA routes.
func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := path.Clean(r.URL.Path)

	// API isolation: never serve SPA fallback for API routes
	if cleanPath == "/api" || strings.HasPrefix(cleanPath, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
		return
	}

	// SPA web assets only accept GET and HEAD
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	relPath := strings.TrimPrefix(cleanPath, "/")
	if relPath == "" {
		relPath = "index.html"
	}

	// Attempt to open the static file directly
	file, err := h.fsys.Open(relPath)
	if err == nil {
		defer file.Close()
		stat, err := file.Stat()
		if err == nil && !stat.IsDir() {
			h.serveFile(w, r, relPath, file, stat)
			return
		}
	}

	// If missing asset under /assets/, strictly return 404 instead of falling back to HTML
	if strings.HasPrefix(cleanPath, "/assets/") {
		http.NotFound(w, r)
		return
	}

	// Fallback to index.html for client-side routing
	h.serveIndex(w, r)
}

func (h *SPAHandler) serveFile(w http.ResponseWriter, r *http.Request, relPath string, file fs.File, stat fs.FileInfo) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Resolve MIME type
	ext := strings.ToLower(filepath.Ext(relPath))
	mimeType := detectMIMEType(ext)
	w.Header().Set("Content-Type", mimeType)

	// Set Cache-Control header
	if relPath == "index.html" {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	} else if strings.HasPrefix(relPath, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}

	if rs, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), rs)
		return
	}

	// Fallback if fs.File does not implement ReadSeeker
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read asset: %v", err), http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, r, stat.Name(), stat.ModTime(), bytes.NewReader(data))
}

func (h *SPAHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	indexFile, err := h.fsys.Open("index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer indexFile.Close()

	stat, err := indexFile.Stat()
	var modTime time.Time
	if err == nil {
		modTime = stat.ModTime()
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	if rs, ok := indexFile.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "index.html", modTime, rs)
		return
	}

	data, err := io.ReadAll(indexFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read index.html: %v", err), http.StatusInternalServerError)
		return
	}

	http.ServeContent(w, r, "index.html", modTime, bytes.NewReader(data))
}

func detectMIMEType(ext string) string {
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".map":
		return "application/json"
	default:
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
		return "application/octet-stream"
	}
}
