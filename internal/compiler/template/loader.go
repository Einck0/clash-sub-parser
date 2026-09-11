package template

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// FSLoader implements pongo2.TemplateLoader on top of any io/fs.FS (such as embed.FS).
type FSLoader struct {
	fsys fs.FS
}

// NewFSLoader creates a new FSLoader for the provided filesystem.
func NewFSLoader(fsys fs.FS) *FSLoader {
	return &FSLoader{fsys: fsys}
}

// Abs calculates the path to a given template within the virtual filesystem.
func (l *FSLoader) Abs(base, name string) string {
	cleanName := path.Clean(strings.TrimPrefix(name, "/"))
	if base == "" {
		return cleanName
	}
	cleanBase := path.Clean(strings.TrimPrefix(base, "/"))
	dir := path.Dir(cleanBase)
	if dir == "." || dir == "/" {
		return cleanName
	}
	return path.Clean(path.Join(dir, cleanName))
}

// Get returns an io.Reader containing the template's content.
func (l *FSLoader) Get(templatePath string) (io.Reader, error) {
	cleanPath := path.Clean(strings.TrimPrefix(templatePath, "/"))
	data, err := fs.ReadFile(l.fsys, cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %q: %w", cleanPath, err)
	}
	return bytes.NewReader(data), nil
}
