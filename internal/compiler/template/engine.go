package template

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/flosch/pongo2/v6"
)

//go:embed templates/*
var defaultTemplateFS embed.FS

// Engine wraps pongo2.TemplateSet with custom filters and embedded templates.
type Engine struct {
	templateSet *pongo2.TemplateSet
}

// NewEngine initializes an Engine with the embedded templates.
func NewEngine() (*Engine, error) {
	sub, err := fs.Sub(defaultTemplateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("failed to open embedded templates sub-filesystem: %w", err)
	}
	return NewEngineWithFS(sub)
}

// NewEngineWithFS initializes an Engine with a custom fs.FS template source.
func NewEngineWithFS(fsys fs.FS) (*Engine, error) {
	RegisterCustomFilters()

	loader := NewFSLoader(fsys)
	set := pongo2.NewSet("template-engine", loader)
	return &Engine{templateSet: set}, nil
}

// Render compiles and renders a template file by name.
func (e *Engine) Render(name string, ctx map[string]any) (string, error) {
	pCtx := pongo2.Context{}
	for k, v := range ctx {
		pCtx[k] = v
	}

	tpl, err := e.templateSet.FromFile(name)
	if err != nil {
		return "", fmt.Errorf("failed to load template %q: %w", name, err)
	}
	out, err := tpl.Execute(pCtx)
	if err != nil {
		return "", fmt.Errorf("failed to execute template %q: %w", name, err)
	}
	return out, nil
}

// RenderString compiles and renders an inline template string.
func (e *Engine) RenderString(content string, ctx map[string]any) (string, error) {
	pCtx := pongo2.Context{}
	for k, v := range ctx {
		pCtx[k] = v
	}

	tpl, err := e.templateSet.FromString(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse template string: %w", err)
	}
	out, err := tpl.Execute(pCtx)
	if err != nil {
		return "", fmt.Errorf("failed to execute template string: %w", err)
	}
	return out, nil
}
