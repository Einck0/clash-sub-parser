package frontend_test

import (
	"io"
	"io/fs"
	"strings"
	"testing"

	"clash-sub-parser/frontend"
)

func TestEmbeddedFrontend_FilesExist(t *testing.T) {
	distFS, err := frontend.FS()
	if err != nil {
		t.Fatalf("failed to obtain dist filesystem: %v", err)
	}

	// 1. Verify index.html exists and is non-empty
	indexFile, err := distFS.Open("index.html")
	if err != nil {
		t.Fatalf("failed to open embedded index.html: %v", err)
	}
	defer indexFile.Close()

	indexBytes, err := io.ReadAll(indexFile)
	if err != nil {
		t.Fatalf("failed to read embedded index.html: %v", err)
	}
	if len(indexBytes) == 0 {
		t.Fatalf("embedded index.html is empty")
	}

	content := string(indexBytes)
	if !strings.Contains(content, "<html") && !strings.Contains(content, "<!DOCTYPE") {
		t.Errorf("expected HTML doctype/tag in index.html, got: %s", content)
	}

	// 2. Verify assets directory exists and contains files
	entries, err := fs.ReadDir(distFS, "assets")
	if err != nil {
		t.Fatalf("failed to read embedded assets directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("embedded assets directory is empty")
	}

	var foundJS, foundCSS bool
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".js") {
			foundJS = true
		}
		if strings.HasSuffix(entry.Name(), ".css") {
			foundCSS = true
		}
	}

	if !foundJS {
		t.Errorf("expected at least one embedded JS asset in assets directory")
	}
	if !foundCSS {
		t.Errorf("expected at least one embedded CSS asset in assets directory")
	}
}
