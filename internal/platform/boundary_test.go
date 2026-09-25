package platform

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findRepoRoot locates the repository root by walking up until go.mod is found.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("failed to find repository root containing go.mod")
		}
		dir = parent
	}
}

func TestNoLegacyPackagesOrFiles(t *testing.T) {
	root := findRepoRoot(t)

	legacyPaths := []string{
		"backend/app",
		"backend/alembic",
		"backend/requirements.txt",
		"backend/alembic.ini",
		"internal/server",
		"internal/service",
		"internal/migration",
		"internal/compiler/template",
		"frontend/src/App.vue",
		"frontend/vite.config.js",
		"pytest.ini",
	}

	for _, relPath := range legacyPaths {
		fullPath := filepath.Join(root, relPath)
		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Errorf("legacy path must not exist in clean-slate repo: %s", relPath)
		}
	}
}

func TestGoModCleanSlateRequirements(t *testing.T) {
	root := findRepoRoot(t)
	goModPath := filepath.Join(root, "go.mod")

	data, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "module clash-sub-parser") {
		t.Errorf("go.mod must declare module 'clash-sub-parser', got:\n%s", content)
	}

	if !strings.Contains(content, "go 1.22") && !strings.Contains(content, "go 1.27") {
		t.Errorf("go.mod must specify Go 1.22+, got:\n%s", content)
	}
}

func TestDomainLayerPurity(t *testing.T) {
	root := findRepoRoot(t)
	domainDir := filepath.Join(root, "internal", "domain")

	if _, err := os.Stat(domainDir); os.IsNotExist(err) {
		t.Fatalf("domain directory does not exist: %s", domainDir)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, domainDir, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("failed to parse domain packages: %v", err)
	}

	forbiddenImports := []string{
		"net/http",
		"database/sql",
		"modernc.org/sqlite",
		"github.com/go-chi/chi/v5",
		"github.com/sagernet/sing-box",
		"clash-sub-parser/internal/transport",
		"clash-sub-parser/internal/repository",
		"clash-sub-parser/internal/application",
	}

	for pkgName, pkg := range pkgs {
		for fileName, file := range pkg.Files {
			// Skip test files when evaluating domain layer source code purity.
			if strings.HasSuffix(fileName, "_test.go") {
				continue
			}
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				for _, forbidden := range forbiddenImports {
					if strings.HasPrefix(importPath, forbidden) {
						t.Errorf("package %s file %s has forbidden domain import: %s", pkgName, fileName, importPath)
					}
				}
			}
		}
	}
}
