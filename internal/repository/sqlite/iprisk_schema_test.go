package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestIPRiskSchemaIsNormalizedAppendOnlyAndIndexed(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	for _, table := range []string{
		"ip_risk_provider_settings",
		"ip_risk_observations",
		"risk_policy_revisions",
		"risk_policy_providers",
		"risk_policy_score_bands",
		"risk_policy_trait_rules",
	} {
		var count int
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?;", table).Scan(&count); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	var observationSQL string
	if err := db.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE type='table' AND name='ip_risk_observations';").Scan(&observationSQL); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ip_address", "full_ip", "raw_payload", "api_key", "cookie", "url_query"} {
		if strings.Contains(strings.ToLower(observationSQL), forbidden) {
			t.Fatalf("observation schema contains forbidden field %q: %s", forbidden, observationSQL)
		}
	}

	var nodeID string
	if err := db.QueryRowContext(ctx, "SELECT logical_id FROM nodes LIMIT 1;").Scan(&nodeID); err == nil {
		t.Fatal("baseline unexpectedly contains a node")
	} else if err != sql.ErrNoRows {
		// The query is only used to ensure the table is available before the FK probe.
		t.Fatalf("query baseline nodes: %v", err)
	}

	var triggerCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='trigger' AND name IN ('ip_risk_observations_no_update', 'ip_risk_observations_no_delete');`).Scan(&triggerCount); err != nil {
		t.Fatal(err)
	}
	if triggerCount != 2 {
		t.Fatalf("expected append-only triggers, got %d", triggerCount)
	}

	for _, indexName := range []string{
		"idx_ip_risk_observations_node_observed",
		"idx_ip_risk_observations_cache_key",
		"idx_ip_risk_observations_provider_status",
		"idx_ip_risk_provider_settings_enabled",
		"idx_risk_policy_providers_revision",
		"idx_risk_policy_score_bands_revision",
	} {
		var count int
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?;", indexName).Scan(&count); err != nil {
			t.Fatalf("check index %s: %v", indexName, err)
		}
		if count != 1 {
			t.Fatalf("expected index %s to exist", indexName)
		}
	}
}

func TestIPRiskObservationSchemaRejectsInvalidForeignKeysAndUpdates(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()
	observationID := "0191e4a0-0000-7000-8000-000000000101"

	_, err := db.ExecContext(ctx, `
		INSERT INTO ip_risk_observations (
			id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
			observed_at, expires_at, status, score, confidence, network_class,
			anonymizer_traits, evidence_digest, redacted_summary
		) VALUES (?, 'node_missing', 'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'fixture', 'v1',
			'2026-09-16T00:00:00Z', '2026-09-16T01:00:00Z', 'available', 20, 90,
			'datacenter', '[]', 'sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 'safe summary');`, observationID)
	if err == nil {
		t.Fatal("observation with missing node must violate foreign key")
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES ('node_0123456789abcdef', 'ss', 'fixture', 'sec://node', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
	) VALUES ('fixture', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
			'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`)
	if err != nil {
		t.Fatalf("insert provider settings: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
		) VALUES ('invalid', 'v1', 'plaintext', 1, 10, 100, 5, 1024,
			'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`)
	if err == nil {
		t.Fatal("plaintext provider secret must violate schema constraint")
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO ip_risk_observations (
			id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
			observed_at, expires_at, status, score, confidence, network_class,
			anonymizer_traits, evidence_digest, redacted_summary
		) VALUES (?, 'node_0123456789abcdef', 'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'fixture', 'v1',
			'2026-09-16T00:00:00Z', '2026-09-16T01:00:00Z', 'available', 20, 90,
			'datacenter', '[]', 'sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 'safe summary');`, observationID)
	if err != nil {
		t.Fatalf("insert valid observation: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE ip_risk_observations SET status='error' WHERE id=?;", observationID); err == nil {
		t.Fatal("updating an observation must be rejected")
	}
	if _, err := db.ExecContext(ctx,
		"DELETE FROM ip_risk_observations WHERE id=?;", observationID); err == nil {
		t.Fatal("deleting an observation must be rejected")
	}
	if _, err := db.ExecContext(ctx,
		"DELETE FROM nodes WHERE logical_id=?;", "node_0123456789abcdef"); err == nil {
		t.Fatal("deleting a node with observations must be rejected")
	}
}

func TestIPRiskSchemaRejectsSensitiveIdentifiersAndRawJSON(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	// 1. ip_risk_provider_settings CHECK constraint on provider and schema_version
	for name, query := range map[string]string{
		"provider with query": `
			INSERT INTO ip_risk_provider_settings (
				provider, schema_version, secret_reference, max_concurrency,
				requests_per_minute, daily_request_budget, per_request_timeout_ms,
				max_response_bytes, created_at, updated_at
			) VALUES ('https://provider.invalid?token=1', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
				'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`,
		"provider with API key": `
			INSERT INTO ip_risk_provider_settings (
				provider, schema_version, secret_reference, max_concurrency,
				requests_per_minute, daily_request_budget, per_request_timeout_ms,
				max_response_bytes, created_at, updated_at
			) VALUES ('api_key=secret', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
				'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`,
		"provider with IP": `
			INSERT INTO ip_risk_provider_settings (
				provider, schema_version, secret_reference, max_concurrency,
				requests_per_minute, daily_request_budget, per_request_timeout_ms,
				max_response_bytes, created_at, updated_at
			) VALUES ('198.51.100.7', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
				'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`,
		"schema_version with query": `
			INSERT INTO ip_risk_provider_settings (
				provider, schema_version, secret_reference, max_concurrency,
				requests_per_minute, daily_request_budget, per_request_timeout_ms,
				max_response_bytes, created_at, updated_at
			) VALUES ('fixture', 'v1?key=val', 'secret://fixture', 1, 10, 100, 5, 1024,
				'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`,
		"schema_version with IP": `
			INSERT INTO ip_risk_provider_settings (
				provider, schema_version, secret_reference, max_concurrency,
				requests_per_minute, daily_request_budget, per_request_timeout_ms,
				max_response_bytes, created_at, updated_at
			) VALUES ('fixture', '2001:db8::7', 'secret://fixture', 1, 10, 100, 5, 1024,
				'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, query); err == nil {
				t.Fatalf("expected SQLite CHECK constraint error for %s", name)
			}
		})
	}

	// 2. ip_risk_observations CHECK constraint on redacted_summary
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, created_at, updated_at)
		VALUES ('node_0123456789abcdef', 'ss', 'fixture', 'secret://node', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO ip_risk_provider_settings (
			provider, schema_version, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms,
			max_response_bytes, created_at, updated_at
		) VALUES ('fixture', 'v1', 'secret://fixture', 1, 10, 100, 5, 1024,
			'2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');`)
	if err != nil {
		t.Fatalf("insert provider settings: %v", err)
	}

	for name, summary := range map[string]string{
		"raw JSON object":   `{"status":"ok","score":42}`,
		"raw JSON array":    `[{"risk":42}]`,
		"embedded raw JSON": `prefix {"ip":"1.2.3.4"}`,
		"overlong summary":  strings.Repeat("a", 513),
	} {
		t.Run("observation_"+name, func(t *testing.T) {
			obsID := domain.MustNewUUIDv7()
			_, err := db.ExecContext(ctx, `
				INSERT INTO ip_risk_observations (
					id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
					observed_at, expires_at, status, score, confidence, network_class,
					anonymizer_traits, evidence_digest, redacted_summary
				) VALUES (?, 'node_0123456789abcdef', 'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'fixture', 'v1',
					'2026-09-16T00:00:00Z', '2026-09-16T01:00:00Z', 'available', 20, 90,
					'datacenter', '[]', 'sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', ?);`, obsID, summary)
			if err == nil {
				t.Fatalf("expected SQLite CHECK constraint error on redacted_summary for %s", name)
			}
		})
	}
}
