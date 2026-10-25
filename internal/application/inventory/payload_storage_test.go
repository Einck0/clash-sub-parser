package inventory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

// TestReconcile_PayloadBodyNotPersisted_RepeatedSuccess verifies FR-RAW-01 and FR-RAW-03:
// ReconcileSubscription persists an empty BLOB (0 bytes) for subscription_payloads.body_blob,
// while preserving structured node facts, parsed_config_json, source links, userinfo, and primary status.
// Repeated successful refreshes also persist only zero-length bodies.
func TestReconcile_PayloadBodyNotPersisted_RepeatedSuccess(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub-payload-zero"
	sub := createTestSub(t, subRepo, subID, "Payload Zero Sub", url, true)

	// Set is_primary on the subscription to verify primary status is preserved
	if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET is_primary = 1 WHERE id = ?", sub.ID); err != nil {
		t.Fatalf("failed to set is_primary: %v", err)
	}

	const subYAML = `
proxies:
  - name: "Node Alpha"
    type: ss
    server: 198.51.100.10
    port: 8388
    cipher: aes-128-gcm
    password: pass-alpha
  - name: "Node Beta"
    type: trojan
    server: 198.51.100.11
    port: 443
    password: pass-beta
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(subYAML),
		ContentDigest: "digest-payload-1",
		ContentType:   "application/x-yaml",
		ETag:          `"etag-1"`,
		UserInfo:      "upload=1024; download=2048; total=10737418240",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	// 1. First Reconcile
	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomeSuccess || res.NodesValid != 2 {
		t.Fatalf("expected 2 valid nodes, got outcome %s with %d valid nodes", res.Outcome, res.NodesValid)
	}

	// Verify subscription_payloads has body_blob with length 0
	var payloadCount int
	var bodyLenSum int
	var bodyBlob []byte
	var contentDigest string
	var headersJSON string
	var httpStatus int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(LENGTH(body_blob)), 0), body_blob, content_digest, headers_json, http_status
		FROM subscription_payloads WHERE subscription_id = ?;`, subID).Scan(
		&payloadCount, &bodyLenSum, &bodyBlob, &contentDigest, &headersJSON, &httpStatus,
	)
	if err != nil {
		t.Fatalf("failed to query subscription_payloads: %v", err)
	}
	if payloadCount != 1 {
		t.Fatalf("expected 1 payload row, got %d", payloadCount)
	}
	if bodyLenSum != 0 || len(bodyBlob) != 0 {
		t.Fatalf("expected body_blob length 0, got len=%d (sum=%d)", len(bodyBlob), bodyLenSum)
	}
	if contentDigest != "digest-payload-1" {
		t.Fatalf("expected content_digest 'digest-payload-1', got %s", contentDigest)
	}
	if httpStatus != 200 {
		t.Fatalf("expected http_status 200, got %d", httpStatus)
	}

	// Verify structured nodes are preserved with connection details
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("expected 2 active nodes, got total=%d, err=%v", total, err)
	}
	for _, n := range nodes {
		if n.Server == "" || n.Port == 0 {
			t.Fatalf("node %s has missing connection parameters: server=%s, port=%d", n.LogicalID, n.Server, n.Port)
		}
	}

	// Verify subscription_entries have whitelisted parsed_config_json without raw body
	rows, err := db.QueryContext(ctx, `
		SELECT raw_name, protocol, server, port, parsed_config_json
		FROM subscription_entries WHERE subscription_id = ?;`, subID)
	if err != nil {
		t.Fatalf("failed to query subscription_entries: %v", err)
	}
	defer rows.Close()

	entryCount := 0
	for rows.Next() {
		entryCount++
		var rawName, proto, server, cfgJSON string
		var port int
		if err := rows.Scan(&rawName, &proto, &server, &port, &cfgJSON); err != nil {
			t.Fatalf("failed to scan entry: %v", err)
		}
		var parsedMap map[string]any
		if err := json.Unmarshal([]byte(cfgJSON), &parsedMap); err != nil {
			t.Fatalf("entry %s has invalid parsed_config_json: %v", rawName, err)
		}
		if _, ok := parsedMap["protocol"]; !ok {
			t.Fatalf("entry %s parsed_config_json missing protocol", rawName)
		}
		if _, ok := parsedMap["server"]; !ok {
			t.Fatalf("entry %s parsed_config_json missing server", rawName)
		}
		if _, ok := parsedMap["port"]; !ok {
			t.Fatalf("entry %s parsed_config_json missing port", rawName)
		}
		if _, ok := parsedMap["credentials"]; !ok {
			t.Fatalf("entry %s parsed_config_json missing credentials", rawName)
		}
	}
	if entryCount != 2 {
		t.Fatalf("expected 2 subscription_entries, got %d", entryCount)
	}

	// Verify userinfo and is_primary on subscription are preserved
	var userInfo string
	var isPrimary int
	err = db.QueryRowContext(ctx, "SELECT userinfo, is_primary FROM subscriptions WHERE id = ?;", subID).Scan(&userInfo, &isPrimary)
	if err != nil {
		t.Fatalf("failed to query subscription userinfo/is_primary: %v", err)
	}
	if userInfo != "upload=1024; download=2048; total=10737418240" {
		t.Fatalf("expected updated userinfo, got %s", userInfo)
	}
	if isPrimary != 1 {
		t.Fatalf("expected is_primary == 1, got %d", isPrimary)
	}

	// 2. Second Reconcile (Repeated Success)
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(subYAML),
		ContentDigest: "digest-payload-2",
		ContentType:   "application/x-yaml",
		ETag:          `"etag-2"`,
		UserInfo:      "upload=2048; download=4096; total=10737418240",
	})

	res2, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if res2.Outcome != domain.FetchOutcomeSuccess || res2.NodesValid != 2 {
		t.Fatalf("second reconcile expected 2 valid nodes, got %d", res2.NodesValid)
	}

	// Verify all subscription_payloads rows for this subscription have length 0
	var totalPayloads int
	var nonemptyPayloads int
	var totalBodyBytes int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN LENGTH(body_blob) > 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(LENGTH(body_blob)), 0)
		FROM subscription_payloads WHERE subscription_id = ?;`, subID).Scan(
		&totalPayloads, &nonemptyPayloads, &totalBodyBytes,
	)
	if err != nil {
		t.Fatalf("failed to query repeated subscription_payloads: %v", err)
	}
	if totalPayloads != 2 {
		t.Fatalf("expected 2 total payload records, got %d", totalPayloads)
	}
	if nonemptyPayloads != 0 {
		t.Fatalf("expected 0 nonempty payloads, got %d", nonemptyPayloads)
	}
	if totalBodyBytes != 0 {
		t.Fatalf("expected 0 total body bytes, got %d", totalBodyBytes)
	}

	// Nodes and source links remain intact
	afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || afterTotal != 2 || len(afterNodes) != 2 {
		t.Fatalf("expected 2 active nodes after second reconcile, got total=%d", afterTotal)
	}
}

// TestReconcile_PayloadBodyNotPersisted_PartialSuccess verifies that partial subscription
// ingestion also stores an empty BLOB and preserves valid nodes and entry provenance.
func TestReconcile_PayloadBodyNotPersisted_PartialSuccess(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub-payload-partial"
	createTestSub(t, subRepo, subID, "Partial Payload Sub", url, true)

	// One valid proxy and one invalid entry (unsupported / broken proxy)
	const partialYAML = `
proxies:
  - name: "Valid Node"
    type: ss
    server: 198.51.100.20
    port: 8388
    cipher: aes-128-gcm
    password: pass-valid
  - name: "Broken Node"
    type: unknown_unsupported_protocol
    server: 198.51.100.21
    port: 9999
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(partialYAML),
		ContentDigest: "digest-partial-1",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	res, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil {
		t.Fatalf("reconcile partial failed with unexpected error: %v", err)
	}
	if res.Outcome != domain.FetchOutcomePartial {
		t.Fatalf("expected partial outcome, got %s", res.Outcome)
	}
	if res.NodesValid != 1 {
		t.Fatalf("expected 1 valid node, got %d", res.NodesValid)
	}

	// Verify payload row body_blob has length 0
	var bodyLenSum int
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(LENGTH(body_blob)), 0)
		FROM subscription_payloads WHERE subscription_id = ?;`, subID).Scan(&bodyLenSum)
	if err != nil {
		t.Fatalf("failed to query subscription_payloads: %v", err)
	}
	if bodyLenSum != 0 {
		t.Fatalf("expected body_blob length 0, got %d", bodyLenSum)
	}

	// Verify valid node was retained
	nodes, total, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || total != 1 || len(nodes) != 1 {
		t.Fatalf("expected 1 active node, got total=%d", total)
	}
	if nodes[0].DisplayName != "Valid Node" {
		t.Fatalf("expected 'Valid Node', got %s", nodes[0].DisplayName)
	}
}

// TestReconcile_PayloadBodyNotPersisted_FailuresPreserveOldAssets verifies that on fetch/parse
// failure or zero nodes, existing active assets are preserved and no raw body is leaked into storage.
func TestReconcile_PayloadBodyNotPersisted_FailuresPreserveOldAssets(t *testing.T) {
	ctx := context.Background()
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	fetcher := newMockFetcher()

	subID := domain.MustNewUUIDv7()
	url := "https://provider.example.com/sub-payload-failures"
	createTestSub(t, subRepo, subID, "Failure Preserve Sub", url, true)

	const initialYAML = `
proxies:
  - name: "Initial Node 1"
    type: ss
    server: 198.51.100.31
    port: 8388
    cipher: aes-128-gcm
    password: pass-initial-1
  - name: "Initial Node 2"
    type: trojan
    server: 198.51.100.32
    port: 443
    password: pass-initial-2
`
	fetcher.setResponse(url, &fetch.Response{
		StatusCode:    200,
		Body:          []byte(initialYAML),
		ContentDigest: "digest-initial",
	})

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)

	initRes, err := svc.ReconcileSubscription(ctx, subID)
	if err != nil || initRes.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Subtest 1: Network / HTTP failure preserves old nodes and creates no nonempty payload
	t.Run("network_failure_preserves_old_nodes", func(t *testing.T) {
		fetcher.setError(url, fmt.Errorf("502 Bad Gateway"))

		rFail, err := svc.ReconcileSubscription(ctx, subID)
		if err == nil {
			t.Fatalf("expected error on network failure, got nil")
		}
		if rFail == nil || rFail.Outcome != domain.FetchOutcomeFailed {
			t.Fatalf("expected failed outcome, got: %#v", rFail)
		}

		// Verify existing nodes still active
		afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil || afterTotal != 2 || len(afterNodes) != 2 {
			t.Fatalf("nodes mutated after network failure: got total=%d, want 2", afterTotal)
		}

		// Verify no nonempty body in subscription_payloads
		var nonemptyCount int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_payloads WHERE LENGTH(body_blob) > 0;`).Scan(&nonemptyCount)
		if nonemptyCount != 0 {
			t.Fatalf("expected 0 nonempty payloads, got %d", nonemptyCount)
		}
	})

	// Subtest 2: Corrupted parse failure preserves old nodes and creates no nonempty payload
	t.Run("parse_failure_preserves_old_nodes", func(t *testing.T) {
		fetcher.setResponse(url, &fetch.Response{
			StatusCode:    200,
			Body:          []byte("<<<!!!INVALID YAML CORRUPTED GARBAGE@@@>>>"),
			ContentDigest: "digest-corrupt",
		})

		rFail, err := svc.ReconcileSubscription(ctx, subID)
		if err == nil {
			t.Fatalf("expected error on corrupted parse failure, got nil")
		}
		if rFail == nil || rFail.Outcome != domain.FetchOutcomeFailed {
			t.Fatalf("expected failed outcome, got: %#v", rFail)
		}

		afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil || afterTotal != 2 || len(afterNodes) != 2 {
			t.Fatalf("nodes mutated after parse failure: got total=%d, want 2", afterTotal)
		}

		var nonemptyCount int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_payloads WHERE LENGTH(body_blob) > 0;`).Scan(&nonemptyCount)
		if nonemptyCount != 0 {
			t.Fatalf("expected 0 nonempty payloads, got %d", nonemptyCount)
		}
	})

	// Subtest 3: Zero nodes extracted preserves old nodes and creates no nonempty payload
	t.Run("zero_nodes_preserves_old_nodes", func(t *testing.T) {
		fetcher.setResponse(url, &fetch.Response{
			StatusCode:    200,
			Body:          []byte("proxies: []\n"),
			ContentDigest: "digest-empty",
		})

		rFail, err := svc.ReconcileSubscription(ctx, subID)
		if err == nil {
			t.Fatalf("expected error on zero nodes extracted, got nil")
		}
		if rFail == nil || rFail.Outcome != domain.FetchOutcomeFailed {
			t.Fatalf("expected failed outcome, got: %#v", rFail)
		}

		afterNodes, afterTotal, err := svc.ListNodes(ctx, domain.NodeFilter{ActiveOnly: true})
		if err != nil || afterTotal != 2 || len(afterNodes) != 2 {
			t.Fatalf("nodes mutated after zero nodes: got total=%d, want 2", afterTotal)
		}

		var nonemptyCount int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_payloads WHERE LENGTH(body_blob) > 0;`).Scan(&nonemptyCount)
		if nonemptyCount != 0 {
			t.Fatalf("expected 0 nonempty payloads, got %d", nonemptyCount)
		}
	})
}
