package probe_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe"
	"clash-sub-parser/internal/probe/pipeline"
)

func TestProbe_OptionsConversion(t *testing.T) {
	opts := probe.DefaultOptions()
	if opts.Concurrency != 100 {
		t.Fatalf("expected concurrency 100, got %d", opts.Concurrency)
	}
	if opts.MemoryLimitMB != 150 {
		t.Fatalf("expected memory limit 150, got %d", opts.MemoryLimitMB)
	}

	pOpts := opts.ToPoolOptions()
	if pOpts.Concurrency != 100 {
		t.Fatalf("expected pool concurrency 100, got %d", pOpts.Concurrency)
	}
	if pOpts.MemoryLimitBytes != 150*1024*1024 {
		t.Fatalf("expected 150MB, got %d", pOpts.MemoryLimitBytes)
	}
	if pOpts.TaskTimeout != 5*time.Second {
		t.Fatalf("expected task timeout 5s, got %v", pOpts.TaskTimeout)
	}
}

func TestEngine_NilNode(t *testing.T) {
	eng := probe.NewEngine(nil)
	res, err := eng.Probe(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error for nil node, got res: %v", res)
	}
}

func TestEngine_UnsupportedProtocolNode(t *testing.T) {
	pipe := pipeline.New(pipeline.DefaultConfig())
	eng := probe.NewEngine(pipe)

	node := &domain.Node{
		ID:        99,
		LogicalID: "unsupported-proto-node",
		Name:      "Unsupported",
		Protocol:  domain.ProtocolType("invalid-proto"),
		Server:    "1.2.3.4",
		Port:      1080,
	}

	res, err := eng.Probe(context.Background(), node)
	if err != nil {
		t.Fatalf("unexpected probe error: %v", err)
	}
	if res.Status != domain.ProbeStatusError {
		t.Errorf("expected status error, got %s", res.Status)
	}
	if res.Error == "" {
		t.Errorf("expected error message describing dialer failure")
	}
}

func TestEngine_ProbeBatch(t *testing.T) {
	pipe := pipeline.New(pipeline.DefaultConfig())
	eng := probe.NewEngine(pipe)

	const nodeCount = 10
	nodes := make([]*domain.Node, nodeCount)
	for i := 0; i < nodeCount; i++ {
		nodes[i] = &domain.Node{
			ID:        int64(i + 1),
			LogicalID: fmt.Sprintf("batch-node-%d", i+1),
			Name:      fmt.Sprintf("Batch-Node-%d", i+1),
			Protocol:  domain.ProtocolType("unsupported-proto"),
			Server:    "127.0.0.1",
			Port:      8000 + i,
		}
	}

	opts := probe.Options{
		Concurrency:   5,
		Timeout:       1 * time.Second,
		MemoryLimitMB: 50,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := eng.ProbeBatch(ctx, nodes, opts)
	if err != nil {
		t.Fatalf("ProbeBatch failed: %v", err)
	}

	receivedCount := 0
	for res := range ch {
		if res == nil {
			t.Errorf("received nil probe result")
			continue
		}
		receivedCount++
		if res.Status != domain.ProbeStatusError {
			t.Errorf("expected error status for unsupported proto, got %s", res.Status)
		}
	}

	if receivedCount != nodeCount {
		t.Errorf("expected %d results, got %d", nodeCount, receivedCount)
	}
}
