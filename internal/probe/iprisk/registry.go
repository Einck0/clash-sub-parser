package iprisk

import (
	"fmt"
	"strings"
	"sync"

	"clash-sub-parser/internal/domain"
)

// Registry manages thread-safe registration and resolution of IP risk provider adapters.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func providerKey(name, schemaVersion string) string {
	return fmt.Sprintf("%s:%s", strings.ToLower(strings.TrimSpace(name)), strings.ToLower(strings.TrimSpace(schemaVersion)))
}

// Register registers a provider. Duplicate registrations return an error.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return domain.NewValidationError("nil_provider", "provider cannot be nil")
	}
	name := strings.TrimSpace(p.Name())
	version := strings.TrimSpace(p.SchemaVersion())
	if name == "" || version == "" {
		return domain.NewValidationError("invalid_provider_key", "provider name and schema version cannot be empty")
	}

	key := providerKey(name, version)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[key]; exists {
		return domain.NewConflictError("provider_already_registered",
			fmt.Sprintf("provider %s with schema version %s already registered", name, version))
	}

	r.providers[key] = p
	return nil
}

// Get retrieves a provider by name and schema version.
func (r *Registry) Get(name, schemaVersion string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[providerKey(name, schemaVersion)]
	return p, ok
}

// All returns a slice of all registered providers.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		res = append(res, p)
	}
	return res
}
