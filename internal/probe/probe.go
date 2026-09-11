package probe

import (
	"context"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/pool"
)

// Options specifies execution settings for the probe engine.
type Options struct {
	Concurrency      int           `json:"concurrency"`
	Timeout          time.Duration `json:"timeout"`
	CheckMediaUnlock bool          `json:"check_media_unlock"`
	CheckAIUnlock    bool          `json:"check_ai_unlock"`
	MemoryLimitMB    int           `json:"memory_limit_mb"`
}

// DefaultOptions returns standard probe execution options.
func DefaultOptions() Options {
	return Options{
		Concurrency:      100,
		Timeout:          5 * time.Second,
		CheckMediaUnlock: true,
		CheckAIUnlock:    true,
		MemoryLimitMB:    150,
	}
}

// ToPoolOptions converts probe Options to pool.Options.
func (o Options) ToPoolOptions() pool.Options {
	memBytes := int64(o.MemoryLimitMB) * 1024 * 1024
	if memBytes <= 0 {
		memBytes = 150 * 1024 * 1024
	}
	pOpts := pool.DefaultOptions()
	if o.Concurrency > 0 {
		pOpts.Concurrency = o.Concurrency
	}
	if o.Timeout > 0 {
		pOpts.TaskTimeout = o.Timeout
	}
	pOpts.MemoryLimitBytes = memBytes
	return pOpts
}

// Engine defines the contract for running in-memory high-concurrency node probing.
type Engine interface {
	// Probe executes an isolated capability check against a single proxy node.
	Probe(ctx context.Context, node *domain.Node) (*domain.NodeProbeResult, error)

	// ProbeBatch dispatches concurrent probes for multiple nodes over a sliding window worker pool.
	ProbeBatch(ctx context.Context, nodes []*domain.Node, opts Options) (<-chan *domain.NodeProbeResult, error)
}
