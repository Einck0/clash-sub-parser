package main

import (
	"context"
	"testing"
	"time"
)

// Startup must not implicitly purge nodes without a current subscription source.
func TestApplicationServicesStartupPreservesInventory(t *testing.T) {
	_, db := setupTestLifecycleDB(t)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("fixture SQL: %v", err)
		}
	}
	exec(`INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at) VALUES ('sub', 'Sub', 'https://example.com/sub', 'rev', ?, ?)`, now, now)
	exec(`INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid) VALUES ('fetch', 'sub', ?, 'success', 'digest', 1, 1)`, now)
	exec(`INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision) VALUES ('group', 'Group', 'select', ?, ?, 0, 1)`, now, now)
	for _, id := range []string{"orphan", "manual", "normal"} {
		exec(`INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at) VALUES (?, 'ss', ?, '1.1.1.1', 8388, '{}', 1, 1, ?, ?)`, id, id, now, now)
		exec(`INSERT INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, config_fingerprint, created_at) VALUES (?, 1, '{}', ?, ?)`, id, "fp-"+id, now)
		exec(`INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at) VALUES (?, 1, ?)`, id, now)
		exec(`INSERT INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, created_at, updated_at) VALUES (?, 'group', 'add', 'manual_node', ?, ?, ?, ?)`, "policy-"+id, map[string]int{"orphan": 0, "manual": 1, "normal": 2}[id], id, now, now)
	}
	exec(`INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('normal', 'sub', 'fetch')`)
	exec(`INSERT INTO node_source_history (id, node_logical_id, subscription_id, source_identity, source_label, connection_revision, relation_state, cause, first_observed_at, last_observed_at, evidence_kind, evidence_key, evidence_json, created_at) VALUES ('hist-orphan', 'orphan', 'sub', 'sub:1', 'Sub', 1, 'verified', 'refresh_removed', ?, ?, 'prune', 'key-orphan', '{}', ?)`, now, now, now)
	exec(`INSERT INTO node_source_history (id, node_logical_id, subscription_id, source_identity, source_label, connection_revision, relation_state, cause, first_observed_at, last_observed_at, evidence_kind, evidence_key, evidence_json, created_at) VALUES ('hist-manual', 'manual', NULL, 'manual:user', 'Manual', 1, 'verified', 'manual_confirmed', ?, ?, 'manual', 'key-manual', '{}', ?)`, now, now, now)

	queries := []string{
		`SELECT COUNT(*) FROM nodes WHERE logical_id IN ('orphan','manual','normal')`,
		`SELECT COUNT(*) FROM node_connection_versions WHERE node_logical_id IN ('orphan','manual','normal')`,
		`SELECT COUNT(*) FROM node_connection_heads WHERE logical_id IN ('orphan','manual','normal')`,
		`SELECT COUNT(*) FROM policy_group_sources WHERE id IN ('policy-orphan','policy-manual','policy-normal')`,
		`SELECT COUNT(*) FROM node_source_history WHERE id IN ('hist-orphan','hist-manual')`,
		`SELECT COUNT(*) FROM node_sources WHERE node_logical_id = 'normal' AND subscription_id = 'sub'`,
	}
	want := []int{3, 3, 3, 3, 2, 1}
	check := func(stage string) {
		t.Helper()
		for i, q := range queries {
			var got int
			if err := db.QueryRowContext(ctx, q).Scan(&got); err != nil || got != want[i] {
				t.Fatalf("%s query %d: got %d want %d, error %v", stage, i, got, want[i], err)
			}
		}
	}
	check("before startup")
	for i := 0; i < 2; i++ {
		services, err := makeApplicationServices(ctx, db, appServiceOptions{})
		if err != nil {
			t.Fatalf("startup %d: %v", i+1, err)
		}
		t.Cleanup(func() { _ = services.probeScheduler.Close() })
		check("after startup")
		drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = services.probeScheduler.Drain(drainCtx)
		cancel()
		if err != nil {
			t.Fatalf("startup %d scheduler drain: %v", i+1, err)
		}
	}
}
