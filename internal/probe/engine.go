package probe

import (
	"context"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/dialer"
	"clash-sub-parser/internal/probe/pipeline"
	"clash-sub-parser/internal/probe/pool"
)

// ErrNilNode is returned when attempting to probe a nil node.
var ErrNilNode = errors.New("node cannot be nil")

type engine struct {
	pipe *pipeline.Pipeline
}

// NewEngine creates a new in-memory high-concurrency probe engine.
func NewEngine(pipe *pipeline.Pipeline) Engine {
	if pipe == nil {
		pipe = pipeline.New(pipeline.DefaultConfig())
	}
	return &engine{
		pipe: pipe,
	}
}

// Probe executes an isolated capability check against a single proxy node.
func (e *engine) Probe(ctx context.Context, node *domain.Node) (*domain.NodeProbeResult, error) {
	if node == nil {
		return nil, ErrNilNode
	}

	d, err := dialer.NewDialer(ctx, node)
	if err != nil {
		nodeKey := node.LogicalID
		if nodeKey == "" {
			nodeKey = node.Name
		}
		return &domain.NodeProbeResult{
			NodeKey:   nodeKey,
			Name:      node.Name,
			Server:    node.Server,
			Port:      node.Port,
			Type:      string(node.Protocol),
			Status:    domain.ProbeStatusError,
			Error:     fmt.Sprintf("failed to initialize sing-box dialer: %v", err),
			CheckedAt: time.Now().Unix(),
		}, nil
	}
	defer func() {
		_ = d.Close()
	}()

	return e.pipe.Execute(ctx, node, d)
}

// ProbeBatch dispatches concurrent probes for multiple nodes over a sliding window worker pool.
func (e *engine) ProbeBatch(ctx context.Context, nodes []*domain.Node, opts Options) (<-chan *domain.NodeProbeResult, error) {
	pOpts := opts.ToPoolOptions()
	workerPool := pool.New[*domain.NodeProbeResult](pOpts)

	bufSize := len(nodes)
	if bufSize <= 0 {
		bufSize = 1
	}
	outChan := make(chan *domain.NodeProbeResult, bufSize)

	// Background collector goroutine reading pool results and streaming to outChan
	go func() {
		defer close(outChan)
		for res := range workerPool.Results() {
			if res.Value != nil {
				outChan <- res.Value
			} else if res.Error != nil {
				// Create an error observation so every node has a corresponding observation
				outChan <- &domain.NodeProbeResult{
					NodeKey:   res.ID,
					Status:    domain.ProbeStatusError,
					Error:     res.Error.Error(),
					CheckedAt: time.Now().Unix(),
				}
			}
		}
	}()

	// Dispatch tasks to the worker pool
	go func() {
		for i, n := range nodes {
			node := n
			taskID := fmt.Sprintf("node-%d", i+1)
			if node.LogicalID != "" {
				taskID = node.LogicalID
			}

			task := pool.Task[*domain.NodeProbeResult]{
				ID:      taskID,
				Timeout: opts.Timeout,
				Fn: func(taskCtx context.Context) (*domain.NodeProbeResult, error) {
					return e.Probe(taskCtx, node)
				},
			}

			if err := workerPool.Submit(ctx, task); err != nil {
				break
			}
		}
		workerPool.Stop()
	}()

	return outChan, nil
}
