package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestPurgeOrphanNodes_CascadesAndProtectsManual(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	subID := "sub-orphan-test"
	nowStr := time.Now().UTC().Format(time.RFC3339)

	// 1. Create a subscription
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at)
		VALUES (?, 'Test Sub', 'https://example.com/sub', 'rev-1', ?, ?);
	`, subID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert subscription: %v", err)
	}

	// 2. Insert two normal nodes and one manual confirmed node
	nodeActive := "node-active"
	nodeOrphan := "node-orphan"
	nodeManual := "node-manual"

	for _, id := range []string{nodeActive, nodeOrphan, nodeManual} {
		_, err := db.ExecContext(ctx, `
			INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at)
			VALUES (?, 'ss', ?, '1.1.1.1', 8388, '{}', 1, 1, ?, ?);
		`, id, id, nowStr, nowStr)
		if err != nil {
			t.Fatalf("failed to insert node %s: %v", id, err)
		}
		// Insert head and version
		_, _ = db.ExecContext(ctx, `
			INSERT INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, config_fingerprint, created_at)
			VALUES (?, 1, '{}', ?, ?);
		`, id, "fp-"+id, nowStr)
		_, _ = db.ExecContext(ctx, `
			INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
			VALUES (?, 1, ?);
		`, id, nowStr)
	}

	// Associate nodeActive with subscription
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, 'fetch-1');
	`, nodeActive, subID)
	if err != nil {
		t.Fatalf("failed to insert node_source: %v", err)
	}

	// Mark nodeOrphan as previously seen in subID via node_source_history
	_, _ = db.ExecContext(ctx, `
		INSERT INTO node_source_history (
			id, node_logical_id, subscription_id, source_identity, source_label,
			connection_revision, relation_state, cause, first_observed_at,
			last_observed_at, evidence_kind, evidence_key, evidence_json, created_at
		) VALUES (?, ?, ?, 'sub:1', 'Test Sub', 1, 'verified', 'refresh_removed', ?, ?, 'prune', ?, '{}', ?);
	`, domain.MustNewUUIDv7(), nodeOrphan, subID, nowStr, nowStr, "key-"+nodeOrphan, nowStr)

	// Mark nodeManual as manual_confirmed
	_, _ = db.ExecContext(ctx, `
		INSERT INTO node_source_history (
			id, node_logical_id, subscription_id, source_identity, source_label,
			connection_revision, relation_state, cause, first_observed_at,
			last_observed_at, evidence_kind, evidence_key, evidence_json, created_at
		) VALUES (?, ?, NULL, 'manual:user', 'Manual', 1, 'verified', 'manual_confirmed', ?, ?, 'manual', ?, '{}', ?);
	`, domain.MustNewUUIDv7(), nodeManual, nowStr, nowStr, "key-"+nodeManual, nowStr)

	// Insert override, observation, and policy_group_source for nodeOrphan
	_, _ = db.ExecContext(ctx, `INSERT INTO node_overrides (node_logical_id, field_path, override_value_json, created_at, updated_at) VALUES (?, 'display_name', '"Custom"', ?, ?);`, nodeOrphan, nowStr, nowStr)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO probe_runs (id, idempotency_key, state, deadline_at, created_at, updated_at)
		VALUES ('pr-1', 'run-1', 'completed', ?, ?, ?);
	`, nowStr, nowStr, nowStr)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO probe_observations (id, probe_run_id, node_logical_id, kind, verdict, observed_at)
		VALUES ('obs-1', 'pr-1', ?, 'baseline', 'available', ?);
	`, nodeOrphan, nowStr)

	// Purge orphans for subID
	err = WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		purged, pErr := PurgeOrphanNodesForSubscription(ctx, tx, subID)
		if pErr != nil {
			return pErr
		}
		if purged != 1 {
			return fmt.Errorf("expected 1 purged node, got %d", purged)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("PurgeOrphanNodesForSubscription failed: %v", err)
	}

	// nodeActive must still exist
	var countActive int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE logical_id = ?;", nodeActive).Scan(&countActive)
	if countActive != 1 {
		t.Errorf("nodeActive should be retained in nodes table")
	}

	// nodeManual must still exist (protected)
	var countManual int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE logical_id = ?;", nodeManual).Scan(&countManual)
	if countManual != 1 {
		t.Errorf("nodeManual should be protected and retained in nodes table")
	}

	// nodeOrphan must be completely removed from nodes and its heads, versions, overrides, observations
	var countOrphan int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE logical_id = ?;", nodeOrphan).Scan(&countOrphan)
	if countOrphan != 0 {
		t.Errorf("nodeOrphan must be purged from nodes table, got count %d", countOrphan)
	}
	var countHead int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_connection_heads WHERE logical_id = ?;", nodeOrphan).Scan(&countHead)
	if countHead != 0 {
		t.Errorf("nodeOrphan connection head must be purged")
	}
	var countObs int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM probe_observations WHERE node_logical_id = ?;", nodeOrphan).Scan(&countObs)
	if countObs != 0 {
		t.Errorf("nodeOrphan probe observation must be purged")
	}
}
