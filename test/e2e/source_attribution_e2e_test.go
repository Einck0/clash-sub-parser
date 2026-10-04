package e2e_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestSourceAttributionE2E_FullLifecycle(t *testing.T) {
	h := setupTestHarness(t)
	ctx := context.Background()

	nodeRepo := sqlite.NewNodeRepository(h.DB)
	subRepo := sqlite.NewSubscriptionRepository(h.DB)
	sourceRepo := sqlite.NewNodeSourceRepository(h.DB)

	// 1. Create a primary subscription
	subID := "01a0b9af-c116-7204-b4a3-98a91733145d"
	sub := domain.Subscription{
		ID:                 subID,
		Name:               "7li Live Provider",
		SourceURLSecretRef: "secret-url-key",
		Enabled:            true,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	if err := subRepo.Create(ctx, &sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	// 2. Seed 31 active nodes in Scope E
	var liveNodes []domain.Node
	for i := 1; i <= 31; i++ {
		node := domain.Node{
			LogicalID:          fmt.Sprintf("node-scope-e-%02d", i),
			Protocol:           domain.ProtocolSS,
			DisplayName:        fmt.Sprintf("Scope-E-Node-%02d", i),
			Server:             fmt.Sprintf("198.51.100.%d", i),
			Port:               443,
			Active:             true,
			ConnectionRevision: 1,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pass"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		liveNodes = append(liveNodes, node)
	}
	if err := nodeRepo.UpsertBatch(ctx, liveNodes); err != nil {
		t.Fatalf("upsert live nodes: %v", err)
	}
	for _, n := range liveNodes {
		if err := sourceRepo.Upsert(ctx, &domain.NodeSource{
			NodeLogicalID:   n.LogicalID,
			SubscriptionID:  subID,
			LastSeenFetchID: "fetch-initial-31",
		}); err != nil {
			t.Fatalf("upsert node source: %v", err)
		}
	}

	tempDir := t.TempDir()
	coldArchivePath := filepath.Join(tempDir, "cold-archive.db")
	coldDB, err := sqlite.Open(sqlite.Config{Path: coldArchivePath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to create cold archive in e2e: %v", err)
	}

	_, err = coldDB.Exec(`
	CREATE TABLE nodes (
		id INTEGER PRIMARY KEY,
		logical_id VARCHAR(36) NOT NULL,
		name VARCHAR(255) NOT NULL,
		protocol VARCHAR(64) NOT NULL,
		server VARCHAR(255) NOT NULL,
		port INTEGER NOT NULL,
		normalized_payload JSON NOT NULL,
		payload_fingerprint VARCHAR(64) NOT NULL,
		lifecycle_state VARCHAR(32) DEFAULT 'active' NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE sources (
		id INTEGER PRIMARY KEY,
		logical_id VARCHAR(36) NOT NULL,
		name VARCHAR(120) NOT NULL,
		kind VARCHAR(32) NOT NULL,
		url TEXT,
		update_interval INTEGER,
		enabled BOOLEAN DEFAULT '1' NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE source_revisions (
		id INTEGER PRIMARY KEY,
		source_id INTEGER NOT NULL,
		revision_id VARCHAR(36) NOT NULL,
		status VARCHAR(32) NOT NULL,
		payload_hash VARCHAR(64) NOT NULL,
		node_count INTEGER DEFAULT 0 NOT NULL,
		error_summary TEXT,
		fetched_at DATETIME,
		created_at DATETIME NOT NULL
	);
	CREATE TABLE node_source_links (
		id INTEGER PRIMARY KEY,
		source_revision_id INTEGER NOT NULL,
		node_id INTEGER NOT NULL,
		display_name VARCHAR(255) NOT NULL,
		original_order INTEGER DEFAULT 0 NOT NULL,
		created_at DATETIME NOT NULL
	);`)
	if err != nil {
		t.Fatalf("create cold archive schema: %v", err)
	}

	_, _ = coldDB.Exec(`INSERT INTO sources (id, logical_id, name, kind, url, created_at, updated_at) VALUES
		(1, 'src-1', '7li Live Provider', 'subscription', 'https://7li.live.provider/sub', '2026-09-11 08:26:06', '2026-09-11 08:26:06');`)
	_, _ = coldDB.Exec(`INSERT INTO source_revisions (id, source_id, revision_id, status, payload_hash, created_at, fetched_at) VALUES
		(1, 1, 'rev-1', 'active', 'hash-1', '2026-09-11 08:26:06', '2026-09-11 08:26:06');`)

	// 3. Seed 5 orphan nodes (active = 0, no node_sources)
	var orphanNodes []domain.Node
	for i := 1; i <= 5; i++ {
		node := domain.Node{
			LogicalID:          fmt.Sprintf("node-orphan-legacy-%02d", i),
			Protocol:           domain.ProtocolSS,
			DisplayName:        fmt.Sprintf("Legacy-Orphan-%02d", i),
			Server:             fmt.Sprintf("203.0.113.%d", i),
			Port:               8388,
			Active:             false,
			ConnectionRevision: 1,
			Credentials: domain.InboundProtocolCredential{
				Password: "secret-password",
				Method:   "aes-256-gcm",
			},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		orphanNodes = append(orphanNodes, node)

		payload := fmt.Sprintf(`{"name":"Legacy-Orphan-%02d","type":"ss","server":"203.0.113.%d","port":8388,"password":"secret-password","cipher":"aes-256-gcm"}`, i, i)
		_, _ = coldDB.Exec(`INSERT INTO nodes (id, logical_id, name, protocol, server, port, normalized_payload, payload_fingerprint, created_at, updated_at) VALUES
			(?, ?, ?, 'ss', ?, 8388, ?, 'fp', '2026-09-11 08:26:06', '2026-09-11 08:26:06');`,
			7000+i-1, fmt.Sprintf("arc-node-%02d", i), fmt.Sprintf("Legacy-Orphan-%02d", i), fmt.Sprintf("203.0.113.%d", i), payload)
		_, _ = coldDB.Exec(`INSERT INTO node_source_links (id, source_revision_id, node_id, display_name, created_at) VALUES
			(?, 1, ?, ?, '2026-09-11 08:26:06');`, 10+i-1, 7000+i-1, fmt.Sprintf("Legacy-Orphan-%02d", i))
	}
	_ = coldDB.Close()

	coldBytes, _ := os.ReadFile(coldArchivePath)
	coldSum := sha256.Sum256(coldBytes)
	coldHash := hex.EncodeToString(coldSum[:])

	if err := nodeRepo.UpsertBatch(ctx, orphanNodes); err != nil {
		t.Fatalf("upsert orphan nodes: %v", err)
	}

	// Verify pre-condition: total nodes = 36, active nodes = 31, node_sources = 31 (Scope E = 31)
	var preTotal, preActive, preSources int
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes;").Scan(&preTotal)
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE active = 1;").Scan(&preActive)
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_sources;").Scan(&preSources)
	if preTotal != 36 || preActive != 31 || preSources != 31 {
		t.Fatalf("pre-condition mismatch: total=%d, active=%d, sources=%d (want 36, 31, 31)", preTotal, preActive, preSources)
	}

	// 4. Create recovery manifest covering 5 orphan nodes
	var manifestNodes []inventory.ManifestNode
	for i, n := range orphanNodes {
		manifestNodes = append(manifestNodes, inventory.ManifestNode{
			NodeLogicalID:      n.LogicalID,
			Protocol:           string(n.Protocol),
			Server:             n.Server,
			Port:               n.Port,
			DisplayName:        n.DisplayName,
			ConnectionRevision: 1,
			ProvenanceOrigin:   "legacy_cold_archive",
			NodeMatchType:      "fullcanonical_and_importmapping",
			Verdict:            "verified",
			PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
				RelationshipKind:       "primary_imported_source",
				LegacyNodePK:           int64(7000 + i),
				LegacyLinkPK:           int64(10 + i),
				LegacySourceID:         1,
				HistoricalSourceName:   "7li Live Provider",
				TargetSubscriptionID:   subID,
				TargetSubscriptionName: "7li Live Provider",
				ObservedAt:             "2026-09-11 08:26:06",
				RelationState:          "verified",
			},
			Evidence: []inventory.ManifestEvidence{
				{
					ArchivePath: coldArchivePath,
					SHA256:      coldHash,
					TableKeys:   fmt.Sprintf("nodes(pk=%d), node_source_links(pk=%d)", 7000+i, 10+i),
				},
			},
		})
	}

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID:       "e2e-run-test",
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			TargetDB:    "test.db",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldArchivePath,
				SHA256: coldHash,
			},
		},
		Summary: inventory.ManifestSummary{
			TotalCoveredOrphans:  5,
			VerifiedNodes:        5,
			PrimaryVerifiedEdges: 5,
		},
		Nodes: manifestNodes,
	}

	manifestBytes, _ := json.Marshal(manifest)
	sum := sha256.Sum256(manifestBytes)
	manifestHash := hex.EncodeToString(sum[:])
	manifestFile := filepath.Join(tempDir, "manifest.json")
	if err := os.WriteFile(manifestFile, manifestBytes, 0644); err != nil {
		t.Fatalf("failed to write test manifest: %v", err)
	}

	// 5. Test Dry-Run: 0 writes, Scope E strictly 31
	dryRes, err := inventory.RecoverSourceHistory(ctx, h.DB, inventory.RecoveryConfig{
		ManifestPath:           manifestFile,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 true,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if dryRes.InsertedRecords != 0 {
		t.Errorf("dry run must insert 0 records, got %d", dryRes.InsertedRecords)
	}

	var histCount int
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_source_history;").Scan(&histCount)
	if histCount != 0 {
		t.Fatalf("dry run must not write to DB, found %d rows", histCount)
	}

	// 6. Test Apply: recovers 5 records, Scope E strictly 31
	applyRes, err := inventory.RecoverSourceHistory(ctx, h.DB, inventory.RecoveryConfig{
		ManifestPath:           manifestFile,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 false,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if applyRes.InsertedRecords != 5 {
		t.Fatalf("expected 5 inserted records, got %d", applyRes.InsertedRecords)
	}

	var postTotal2, postActive2, postSources2 int
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes;").Scan(&postTotal2)
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE active = 1;").Scan(&postActive2)
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_sources;").Scan(&postSources2)
	if postActive2 != 31 || postSources2 != 31 || postTotal2 != 36 {
		t.Fatalf("Scope E must remain strictly 31, got total=%d active=%d sources=%d", postTotal2, postActive2, postSources2)
	}

	// 7. Test Idempotency: re-applying writes 0 duplicate rows
	applyRes2, err := inventory.RecoverSourceHistory(ctx, h.DB, inventory.RecoveryConfig{
		ManifestPath:           manifestFile,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 false,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("idempotent apply failed: %v", err)
	}
	if applyRes2.InsertedRecords != 0 || applyRes2.NewlyInserted != 0 || applyRes2.ExistingRecords != 5 {
		t.Errorf("expected 0 newly inserted and 5 existing on idempotent re-apply, got inserted=%d newly=%d existing=%d",
			applyRes2.InsertedRecords, applyRes2.NewlyInserted, applyRes2.ExistingRecords)
	}
	_ = h.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_source_history;").Scan(&histCount)
	if histCount != 5 {
		t.Fatalf("expected exactly 5 rows in node_source_history after idempotent re-run, got %d", histCount)
	}

	// 8. Test API endpoint GET /api/v1/nodes/{id}/source-history
	// Orphan node
	orphanID := orphanNodes[0].LogicalID
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+orphanID+"/source-history", nil)
	req.Header.Set("Authorization", "Bearer "+h.InitialAdminToken)
	rec := httptest.NewRecorder()
	h.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on orphan source-history, got %d: %s", rec.Code, rec.Body.String())
	}
	var orphanAPIResp struct {
		Data struct {
			CurrentSources    []any  `json:"current_sources"`
			History           []any  `json:"history"`
			AttributionStatus string `json:"attribution_status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &orphanAPIResp)
	if orphanAPIResp.Data.AttributionStatus != "historical_verified" {
		t.Errorf("expected attribution_status historical_verified, got %s", orphanAPIResp.Data.AttributionStatus)
	}
	if len(orphanAPIResp.Data.CurrentSources) != 0 {
		t.Errorf("expected 0 current sources for orphan node, got %d", len(orphanAPIResp.Data.CurrentSources))
	}
	if len(orphanAPIResp.Data.History) != 1 {
		t.Errorf("expected 1 history record, got %d", len(orphanAPIResp.Data.History))
	}

	// Active node
	activeID := liveNodes[0].LogicalID
	reqActive := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/"+activeID+"/source-history", nil)
	reqActive.Header.Set("Authorization", "Bearer "+h.InitialAdminToken)
	recActive := httptest.NewRecorder()
	h.Router.ServeHTTP(recActive, reqActive)
	if recActive.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on active source-history, got %d", recActive.Code)
	}
	var activeAPIResp struct {
		Data struct {
			CurrentSources []struct {
				SubscriptionID string `json:"subscription_id"`
				Name           string `json:"name"`
				Enabled        bool   `json:"enabled"`
			} `json:"current_sources"`
			AttributionStatus string `json:"attribution_status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(recActive.Body.Bytes(), &activeAPIResp)
	if activeAPIResp.Data.AttributionStatus != "current" {
		t.Errorf("expected attribution_status current, got %s", activeAPIResp.Data.AttributionStatus)
	}
	if len(activeAPIResp.Data.CurrentSources) != 1 {
		t.Errorf("expected 1 current source, got %d", len(activeAPIResp.Data.CurrentSources))
	}

	// 9. Test Subscription Refresh Pruning: 1 live node omitted from refresh
	// Let's configure memory fetcher with only 30 nodes (liveNodes[0] omitted)
	var newYAMLProxies string
	for _, n := range liveNodes[1:] {
		newYAMLProxies += fmt.Sprintf(`  - name: %q
    type: ss
    server: %s
    port: %d
    cipher: aes-128-gcm
    password: pass
`, n.DisplayName, n.Server, n.Port)
	}
	newYAML := fmt.Sprintf("proxies:\n%s", newYAMLProxies)

	h.Fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/yaml",
		ContentDigest: "digest-refresh-30",
		Body:          []byte(newYAML),
	})

	invService := inventory.NewService(h.DB, subRepo, sqlite.NewSubscriptionFetchRepository(h.DB), nodeRepo, sourceRepo, h.Fetcher)
	resRef, err := invService.ReconcileSubscription(ctx, subID)
	if err != nil || resRef.NodesValid != 30 {
		t.Fatalf("reconcile subscription failed: %v (res=%#v)", err, resRef)
	}

	// Verify liveNodes[0] was pruned from live node_sources, but active remains true
	omittedDetail, err := invService.GetNode(ctx, activeID)
	if err != nil || !omittedDetail.Active {
		t.Fatalf("omitted node active must remain true, got active=%v", omittedDetail.Active)
	}
	omittedSources, err := sourceRepo.ListByNode(ctx, activeID)
	if err != nil || len(omittedSources) != 0 {
		t.Fatalf("omitted node must be pruned from node_sources, got %d", len(omittedSources))
	}

	// Verify omitted node has refresh_removed captured in history
	omittedHistResp, err := invService.GetNodeSourceHistory(ctx, activeID)
	if err != nil {
		t.Fatalf("GetNodeSourceHistory for omitted node failed: %v", err)
	}
	if omittedHistResp.AttributionStatus != domain.AttributionStatusHistoricalVerified {
		t.Errorf("expected historical_verified status for omitted node, got %s", omittedHistResp.AttributionStatus)
	}
	if len(omittedHistResp.History) != 1 || omittedHistResp.History[0].Cause != domain.CauseRefreshRemoved {
		t.Errorf("expected 1 history record with cause refresh_removed, got: %#v", omittedHistResp.History)
	}

	// 10. Test Subscription Deletion Cascade: delete subscription
	if err := subRepo.Delete(ctx, subID); err != nil {
		t.Fatalf("failed to delete subscription: %v", err)
	}

	// All remaining 30 nodes are snapshotted with subscription_deleted, and subscription_id is set to NULL
	for _, n := range liveNodes[1:] {
		hList, err := sqlite.NewNodeSourceHistoryRepository(h.DB).ListByNodeLogicalID(ctx, n.LogicalID)
		if err != nil || len(hList) != 1 {
			t.Fatalf("expected 1 history record for node %s after sub delete, got %d", n.LogicalID, len(hList))
		}
		if hList[0].Cause != domain.CauseSubscriptionDeleted {
			t.Errorf("expected cause subscription_deleted for node %s, got %s", n.LogicalID, hList[0].Cause)
		}
		if hList[0].SubscriptionID != nil {
			t.Errorf("expected subscription_id to be NULL after cascade, got %v", *hList[0].SubscriptionID)
		}
		if hList[0].SourceLabel != "7li Live Provider" {
			t.Errorf("expected source_label to be preserved, got %s", hList[0].SourceLabel)
		}
	}
}
