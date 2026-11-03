package main

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplicationServices_ProxyChainWiring(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	ctx := context.Background()

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}

	if appServices.chainRepo == nil {
		t.Fatal("expected non-nil chainRepo in applicationServices")
	}
	if appServices.chainService == nil {
		t.Fatal("expected non-nil chainService in applicationServices")
	}

	cfg, err := appServices.chainService.Get(ctx)
	if err != nil {
		t.Fatalf("chainService.Get failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil ProxyChainConfig")
	}
	if len(cfg.Bindings) != 0 {
		t.Fatalf("expected 0 initial bindings, got %d", len(cfg.Bindings))
	}
	if cfg.Version == "" {
		t.Fatal("expected non-empty deterministic initial version")
	}

	opts, err := appServices.chainService.Options(ctx)
	if err != nil {
		t.Fatalf("chainService.Options failed: %v", err)
	}
	if opts == nil || opts.Nodes == nil || opts.Groups == nil || opts.Subscriptions == nil {
		t.Fatal("expected non-nil OptionsResult and slices")
	}
}
