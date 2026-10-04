package inventory_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func createTestManifestFile(t *testing.T, dir string, manifest inventory.RecoveryManifest) (string, string) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write manifest file: %v", err)
	}

	return path, hash
}

// createColdArchiveFixture creates a real SQLite cold archive database fixture.
func createColdArchiveFixture(t *testing.T, dir string) (string, string) {
	t.Helper()
	path := filepath.Join(dir, "cold_archive.db")
	db, err := sqlite.Open(sqlite.Config{Path: path, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to create cold archive fixture: %v", err)
	}
	defer db.Close()

	schema := `
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
	);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to initialize cold archive schema: %v", err)
	}

	// Insert test sources
	// Source 1: 7li (matches modern sub 7li)
	// Source 2: Eeox (matches modern sub Eeox)
	// Source 3: DeletedSource (no modern sub)
	_, _ = db.Exec(`INSERT INTO sources (id, logical_id, name, kind, url, created_at, updated_at) VALUES
		(1, 'src-1', '7li', 'subscription', 'https://z.7li7li.com/api/v1/client/subscribe?token=tok1', '2026-09-11 08:00:00', '2026-09-11 08:00:00'),
		(2, 'src-2', 'Eeox', 'subscription', 'https://api.eeox.net/api/v1/client/subscribe?token=tok2', '2026-09-11 08:00:00', '2026-09-11 08:00:00'),
		(3, 'src-3', 'OldDeleted', 'subscription', 'https://deleted.archive.net/sub', '2026-09-11 08:00:00', '2026-09-11 08:00:00');`)

	_, _ = db.Exec(`INSERT INTO source_revisions (id, source_id, revision_id, status, payload_hash, created_at, fetched_at) VALUES
		(10, 1, 'rev-10', 'active', 'hash10', '2026-09-11 08:26:06', '2026-09-11 08:26:06'),
		(20, 2, 'rev-20', 'active', 'hash20', '2026-09-13 16:13:16', '2026-09-13 16:13:16'),
		(30, 3, 'rev-30', 'active', 'hash30', '2026-09-14 00:00:00', '2026-09-14 00:00:00');`)

	// Node 100: VLESS orphan1 (primary)
	payload100 := `{"name":"[vless]Orphan1","type":"vless","server":"orphan1.example.com","port":8443,"uuid":"501e2af2-26f2-466c-9e91-a87c5a738ad8","tls":true,"servername":"itunes.apple.com","flow":"xtls-rprx-vision"}`
	// Node 101: VLESS orphan1 (secondary same canonical from Eeox)
	payload101 := `{"name":"[vless]Orphan1-Eeox","type":"vless","server":"orphan1.example.com","port":8443,"uuid":"501e2af2-26f2-466c-9e91-a87c5a738ad8","tls":true,"servername":"itunes.apple.com","flow":"xtls-rprx-vision"}`
	// Node 200: VMess orphan2 (primary)
	payload200 := `{"name":"[vmess]Orphan2","type":"vmess","server":"orphan2.example.com","port":443,"uuid":"22222222-2222-2222-2222-222222222222"}`
	// Node 201: VMess orphan2 with DIFFERENT UUID (conflicting credential candidate!)
	payload201 := `{"name":"[vmess]Orphan2-Conflict","type":"vmess","server":"orphan2.example.com","port":443,"uuid":"99999999-9999-9999-9999-999999999999"}`
	// Node 300: WireGuard orphan3 (from OldDeleted source)
	payload300 := `{"name":"[wg]Orphan3","type":"wireguard","server":"orphan3.example.com","port":51820,"ip":"10.0.0.2/32","private-key":"privkey333=","public-key":"pubkey333=","pre-shared-key":"psk333="}`

	_, _ = db.Exec(`INSERT INTO nodes (id, logical_id, name, protocol, server, port, normalized_payload, payload_fingerprint, created_at, updated_at) VALUES
		(100, 'arc-n-100', 'Orphan1', 'vless', 'orphan1.example.com', 8443, ?, 'fp100', '2026-09-11 08:26:06', '2026-09-11 08:26:06'),
		(101, 'arc-n-101', 'Orphan1-2', 'vless', 'orphan1.example.com', 8443, ?, 'fp101', '2026-09-11 08:26:06', '2026-09-11 08:26:06'),
		(200, 'arc-n-200', 'Orphan2', 'vmess', 'orphan2.example.com', 443, ?, 'fp200', '2026-09-13 16:13:16', '2026-09-13 16:13:16'),
		(201, 'arc-n-201', 'Orphan2-Conflict', 'vmess', 'orphan2.example.com', 443, ?, 'fp201', '2026-09-13 16:13:16', '2026-09-13 16:13:16'),
		(300, 'arc-n-300', 'Orphan3', 'wireguard', 'orphan3.example.com', 51820, ?, 'fp300', '2026-09-14 00:00:00', '2026-09-14 00:00:00');`,
		payload100, payload101, payload200, payload201, payload300)

	_, _ = db.Exec(`INSERT INTO node_source_links (id, source_revision_id, node_id, display_name, created_at) VALUES
		(1001, 10, 100, 'Orphan1 Link', '2026-09-11 08:26:06'),
		(1002, 20, 101, 'Orphan1 Link 2', '2026-09-11 08:26:06'),
		(2001, 20, 200, 'Orphan2 Link', '2026-09-13 16:13:16'),
		(2002, 20, 201, 'Orphan2 Conflict Link', '2026-09-13 16:13:16'),
		(3001, 30, 300, 'Orphan3 Link', '2026-09-14 00:00:00');`)

	data, _ := os.ReadFile(path)
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

// createV1BackupFixture creates a real SQLite v1 backup database fixture.
func createV1BackupFixture(t *testing.T, dir, filename string) (string, string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	db, err := sqlite.Open(sqlite.Config{Path: path, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to create v1 backup fixture: %v", err)
	}
	defer db.Close()

	schema := `
	CREATE TABLE nodes (
		logical_id TEXT PRIMARY KEY,
		protocol TEXT NOT NULL,
		display_name TEXT NOT NULL,
		active INTEGER NOT NULL DEFAULT 1,
		server TEXT NOT NULL,
		port INTEGER NOT NULL,
		config_json TEXT NOT NULL DEFAULT '{}',
		connection_revision INTEGER DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE subscriptions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		source_url_secret_ref TEXT NOT NULL
	);
	CREATE TABLE subscription_fetches (
		id TEXT PRIMARY KEY,
		subscription_id TEXT NOT NULL,
		fetched_at TEXT NOT NULL
	);
	CREATE TABLE node_sources (
		node_logical_id TEXT NOT NULL,
		subscription_id TEXT NOT NULL,
		last_seen_fetch_id TEXT NOT NULL
	);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to initialize v1 backup schema: %v", err)
	}

	subID := "01a0b9af-c116-7280-9629-f31353b85805"
	fetchID := "fetch-v1-001"
	v1NodeID := "node-v1-refresh-1"

	_, _ = db.Exec(`INSERT INTO subscriptions (id, name, source_url_secret_ref) VALUES (?, 'einck-qzz', 'https://234.qzz.io/fsllistyaml');`, subID)
	_, _ = db.Exec(`INSERT INTO subscription_fetches (id, subscription_id, fetched_at) VALUES (?, ?, '2026-09-29T00:52:20Z');`, fetchID, subID)
	_, _ = db.Exec(`INSERT INTO nodes (logical_id, protocol, display_name, active, server, port, config_json, connection_revision, created_at, updated_at) VALUES
		(?, 'hysteria2', 'V1 Orphan', 1, '156.229.160.253', 55000, '{"password":"pass-hy2-v1","transport":{"sni":"kr.test"}}', 1, '2026-09-29T00:52:21Z', '2026-09-29T00:52:21Z');`, v1NodeID)
	_, _ = db.Exec(`INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id) VALUES (?, ?, ?);`, v1NodeID, subID, fetchID)

	data, _ := os.ReadFile(path)
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:])
}

func setupTargetDBWithFixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()

	// Modern subscriptions:
	// Sub 1: 7li (matches cold archive src 1)
	// Sub 2: Eeox (matches cold archive src 2)
	// Sub 3: einck-qzz (matches v1 backup sub)
	sub1ID := "01a0b9af-c116-7204-b4a3-98a91733145d"
	sub2ID := "01a0b9af-c116-78d1-9ffd-fef9309e794f"
	sub3ID := "01a0b9af-c116-7280-9629-f31353b85805"

	_, _ = db.ExecContext(ctx, `INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at) VALUES
		(?, '7li', 'https://z.7li7li.com/api/v1/client/subscribe', 'rev1', '2026-09-11', '2026-09-11'),
		(?, 'Eeox', 'https://api.eeox.net/api/v1/client/subscribe', 'rev2', '2026-09-11', '2026-09-11'),
		(?, 'einck-qzz', 'https://234.qzz.io/fsllistyaml', 'rev3', '2026-09-29', '2026-09-29');`,
		sub1ID, sub2ID, sub3ID)

	// Active Node (Scope E = 1)
	activeNode := domain.Node{
		LogicalID:          "node-active-1",
		Protocol:           domain.ProtocolVLESS,
		DisplayName:        "Active Node 1",
		Server:             "active.example.com",
		Port:               443,
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	nodeRepo := sqlite.NewNodeRepository(db)
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{activeNode}); err != nil {
		t.Fatalf("failed to insert active node: %v", err)
	}

	sourceRepo := sqlite.NewNodeSourceRepository(db)
	if err := sourceRepo.Upsert(ctx, &domain.NodeSource{
		NodeLogicalID:   activeNode.LogicalID,
		SubscriptionID:  sub1ID,
		LastSeenFetchID: "fetch-act-1",
	}); err != nil {
		t.Fatalf("failed to insert active node source: %v", err)
	}

	// Orphan 1: VLESS (matches pk 100 in cold archive)
	orphan1 := domain.Node{
		LogicalID:          "node-orphan-1",
		Protocol:           domain.ProtocolVLESS,
		DisplayName:        "Orphan Node 1",
		Server:             "orphan1.example.com",
		Port:               8443,
		Active:             false,
		ConnectionRevision: 1,
		Credentials: domain.InboundProtocolCredential{
			UUID: "501e2af2-26f2-466c-9e91-a87c5a738ad8",
			Transport: map[string]string{
				"flow": "xtls-rprx-vision",
				"sni":  "itunes.apple.com",
				"tls":  "true",
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	// Orphan 2: VMess (matches pk 200 in cold archive)
	orphan2 := domain.Node{
		LogicalID:          "node-orphan-2",
		Protocol:           domain.ProtocolVMess,
		DisplayName:        "Orphan Node 2",
		Server:             "orphan2.example.com",
		Port:               443,
		Active:             false,
		ConnectionRevision: 1,
		Credentials: domain.InboundProtocolCredential{
			UUID: "22222222-2222-2222-2222-222222222222",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	// Orphan 3: WireGuard (matches pk 300, deleted source)
	orphan3 := domain.Node{
		LogicalID:          "node-orphan-3",
		Protocol:           domain.ProtocolWireGuard,
		DisplayName:        "Orphan Node 3",
		Server:             "orphan3.example.com",
		Port:               51820,
		Active:             false,
		ConnectionRevision: 1,
		Credentials: domain.InboundProtocolCredential{
			PrivateKey:   "privkey333=",
			PublicKey:    "pubkey333=",
			PresharedKey: "psk333=",
			LocalAddress: []string{"10.0.0.2/32"},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	// Orphan 4: Hysteria2 (matches v1 backup node)
	orphan4 := domain.Node{
		LogicalID:          "node-v1-refresh-1",
		Protocol:           domain.ProtocolHysteria2,
		DisplayName:        "V1 Orphan",
		Server:             "156.229.160.253",
		Port:               55000,
		Active:             false,
		ConnectionRevision: 1,
		Credentials: domain.InboundProtocolCredential{
			Password: "pass-hy2-v1",
			Transport: map[string]string{
				"sni": "kr.test",
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{orphan1, orphan2, orphan3, orphan4}); err != nil {
		t.Fatalf("failed to insert orphan nodes: %v", err)
	}
}

func TestRecoverSourceHistory_RealFixtures_DryRunAndApply(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)
	v1BackupPath, v1Hash := createV1BackupFixture(t, tempDir, "csp-v1-refresh.db")

	targetDBPath := filepath.Join(tempDir, "target.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Build recovery manifest with 4 nodes:
	// - orphan1: cold archive primary (pk 100) + secondary same canonical (pk 101)
	// - orphan2: cold archive primary (pk 200) + discarded conflicting candidate (pk 201)
	// - orphan3: cold archive primary (pk 300) from DeletedSource
	// - orphan4: v1 backup refresh pruned
	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID:       "run-fixture-1",
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
			V1BackupDigests: map[string]string{
				"csp-v1-refresh.db": v1Hash,
			},
		},
		Summary: inventory.ManifestSummary{
			TotalCoveredOrphans:                     4,
			VerifiedNodes:                           4,
			PrimaryVerifiedEdges:                    4,
			SecondarySameCanonicalEdges:             1,
			DiscardedConflictingCredentialCandidates: 1,
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				DisplayName:      "Orphan Node 1",
				ProvenanceOrigin: "legacy_cold_archive",
				NodeMatchType:    "fullcanonical_and_importmapping",
				Verdict:          "verified",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:       "primary_imported_source",
					LegacyNodePK:           100,
					LegacyLinkPK:           1001,
					LegacySourceID:         1,
					HistoricalSourceName:   "7li",
					TargetSubscriptionID:   "01a0b9af-c116-7204-b4a3-98a91733145d",
					TargetSubscriptionName: "7li",
					ObservedAt:             "2026-09-11 08:26:06",
					RelationState:          "verified",
				},
				SecondarySameCanonicalSources: []inventory.ManifestSourceEdge{
					{
						RelationshipKind:       "secondary_same_canonical_source",
						LegacyNodePK:           101,
						LegacyLinkPK:           1002,
						LegacySourceID:         2,
						HistoricalSourceName:   "Eeox",
						TargetSubscriptionID:   "01a0b9af-c116-78d1-9ffd-fef9309e794f",
						TargetSubscriptionName: "Eeox",
						ObservedAt:             "2026-09-11 08:26:06",
						RelationState:          "verified",
					},
				},
				Evidence: []inventory.ManifestEvidence{
					{ArchivePath: coldPath, SHA256: coldHash, TableKeys: "nodes(pk=100)"},
				},
			},
			{
				NodeLogicalID:    "node-orphan-2",
				Protocol:         "vmess",
				Server:           "orphan2.example.com",
				Port:             443,
				DisplayName:      "Orphan Node 2",
				ProvenanceOrigin: "legacy_cold_archive",
				NodeMatchType:    "selected_row_verified_with_collision_notes",
				Verdict:          "verified",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:       "primary_imported_source",
					LegacyNodePK:           200,
					LegacyLinkPK:           2001,
					LegacySourceID:         2,
					HistoricalSourceName:   "Eeox",
					TargetSubscriptionID:   "01a0b9af-c116-78d1-9ffd-fef9309e794f",
					TargetSubscriptionName: "Eeox",
					ObservedAt:             "2026-09-13 16:13:16",
					RelationState:          "verified",
				},
				DiscardedCollisionNotes: []inventory.ManifestSourceEdge{
					{
						RelationshipKind:       "discarded_conflicting_credential_candidate",
						LegacyNodePK:           201,
						LegacyLinkPK:           2002,
						LegacySourceID:         2,
						HistoricalSourceName:   "Eeox",
						TargetSubscriptionID:   "01a0b9af-c116-78d1-9ffd-fef9309e794f",
						TargetSubscriptionName: "Eeox",
						RelationState:          "discarded_conflict_not_attached",
					},
				},
				Evidence: []inventory.ManifestEvidence{
					{ArchivePath: coldPath, SHA256: coldHash, TableKeys: "nodes(pk=200)"},
				},
			},
			{
				NodeLogicalID:    "node-orphan-3",
				Protocol:         "wireguard",
				Server:           "orphan3.example.com",
				Port:             51820,
				DisplayName:      "Orphan Node 3",
				ProvenanceOrigin: "legacy_cold_archive",
				NodeMatchType:    "fullcanonical_and_importmapping",
				Verdict:          "verified",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         300,
					LegacyLinkPK:         3001,
					LegacySourceID:       3,
					HistoricalSourceName: "OldDeleted",
					RelationState:        "verified",
				},
				Evidence: []inventory.ManifestEvidence{
					{ArchivePath: coldPath, SHA256: coldHash, TableKeys: "nodes(pk=300)"},
				},
			},
			{
				NodeLogicalID:      "node-v1-refresh-1",
				Protocol:           "hysteria2",
				Server:             "156.229.160.253",
				Port:               55000,
				DisplayName:        "V1 Orphan",
				ConnectionRevision: 1,
				ProvenanceOrigin:   "v1_subscription_refresh",
				NodeMatchType:      "fullcanonical_and_historicalversion",
				Verdict:            "verified",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:       "v1_subscription_refresh_pruned",
					TargetSubscriptionID:   "01a0b9af-c116-7280-9629-f31353b85805",
					TargetSubscriptionName: "einck-qzz",
					HistoricalSourceName:   "einck-qzz",
					RelationState:          "verified",
				},
				Evidence: []inventory.ManifestEvidence{
					{
						BackupFile: filepath.Base(v1BackupPath),
						SHA256:     v1Hash,
						TableKeys:  "node_sources(node_logical_id=node-v1-refresh-1)",
					},
				},
			},
		},
	}

	manifestPath, manifestHash := createTestManifestFile(t, tempDir, manifest)

	// Step 1: Dry-Run Test
	dryRunRes, err := inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 true,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !dryRunRes.DryRun || dryRunRes.Status != "dry_run_success" {
		t.Fatalf("expected dry_run_success, got %#v", dryRunRes)
	}
	if dryRunRes.InsertedRecords != 0 {
		t.Errorf("dry run must have 0 inserted records, got %d", dryRunRes.InsertedRecords)
	}
	if dryRunRes.TotalEdgesToInsert != 5 { // 4 primary + 1 secondary
		t.Errorf("expected 5 edges to insert, got %d", dryRunRes.TotalEdgesToInsert)
	}
	if dryRunRes.DiscardedCandidates != 1 {
		t.Errorf("expected 1 discarded candidate, got %d", dryRunRes.DiscardedCandidates)
	}

	// Verify database file was NOT written to
	var histCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_source_history;").Scan(&histCount)
	if histCount != 0 {
		t.Fatalf("dry run must not write any records to node_source_history, found %d", histCount)
	}

	// Step 2: Apply Recovery
	applyRes, err := inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if applyRes.Status != "success" || applyRes.InsertedRecords != 5 {
		t.Fatalf("expected 5 inserted records, got: %#v", applyRes)
	}
	if applyRes.DiscardedCandidates != 1 {
		t.Errorf("expected 1 discarded candidate, got %d", applyRes.DiscardedCandidates)
	}

	// Check invariants
	if !applyRes.PostInvariants.BusinessAssetsUntouched {
		t.Errorf("expected BusinessAssetsUntouched to be true")
	}
	if applyRes.PostInvariants.ActiveNodes != 1 {
		t.Errorf("expected active nodes 1 (Scope E unchanged), got %d", applyRes.PostInvariants.ActiveNodes)
	}
	if applyRes.PostInvariants.NodeSources != 1 {
		t.Errorf("expected node_sources 1, got %d", applyRes.PostInvariants.NodeSources)
	}
	if applyRes.PostInvariants.TotalNodes != 5 {
		t.Errorf("expected total nodes 5, got %d", applyRes.PostInvariants.TotalNodes)
	}

	// Verify orphan 1: has 2 records (primary 7li + secondary Eeox)
	hRepo := sqlite.NewNodeSourceHistoryRepository(db)
	h1, err := hRepo.ListByNodeLogicalID(ctx, "node-orphan-1")
	if err != nil || len(h1) != 2 {
		t.Fatalf("expected 2 history records for orphan 1, got %d (err: %v)", len(h1), err)
	}
	for _, h := range h1 {
		if h.ConnectionRevision != nil {
			t.Errorf("legacy cold archive records must have connection_revision NULL, got %v", *h.ConnectionRevision)
		}
		if h.RelationState != domain.RelationStateVerified {
			t.Errorf("expected verified, got %s", h.RelationState)
		}
	}

	// Verify orphan 2: has EXACTLY 1 RECORD (primary)!
	// Discarded conflicting candidate must NEVER be attached to orphan 2!
	h2, err := hRepo.ListByNodeLogicalID(ctx, "node-orphan-2")
	if err != nil || len(h2) != 1 {
		t.Fatalf("expected 1 history record for orphan 2 (discarded isolated), got %d (err: %v)", len(h2), err)
	}

	// Verify orphan 3: Deleted source retains old label and namespace with subscription_id = NULL
	h3, err := hRepo.ListByNodeLogicalID(ctx, "node-orphan-3")
	if err != nil || len(h3) != 1 {
		t.Fatalf("expected 1 history record for orphan 3, got %d (err: %v)", len(h3), err)
	}
	if h3[0].SubscriptionID != nil {
		t.Errorf("deleted source record must have subscription_id NULL, got %v", *h3[0].SubscriptionID)
	}
	if h3[0].SourceLabel != "OldDeleted" {
		t.Errorf("expected source_label OldDeleted, got %s", h3[0].SourceLabel)
	}
	if h3[0].SourceIdentity != "legacy:src:3" {
		t.Errorf("expected source_identity legacy:src:3, got %s", h3[0].SourceIdentity)
	}

	// Verify orphan 4 (v1 refresh): has connection_revision = 1 and observed timestamp
	h4, err := hRepo.ListByNodeLogicalID(ctx, "node-v1-refresh-1")
	if err != nil || len(h4) != 1 {
		t.Fatalf("expected 1 history record for v1 orphan, got %d (err: %v)", len(h4), err)
	}
	if h4[0].ConnectionRevision == nil || *h4[0].ConnectionRevision != 1 {
		t.Errorf("expected v1 refresh connection_revision 1, got %v", h4[0].ConnectionRevision)
	}
	if h4[0].Cause != domain.CauseRefreshRemoved {
		t.Errorf("expected cause refresh_removed, got %s", h4[0].Cause)
	}
	if h4[0].FirstObservedAt == nil || h4[0].FirstObservedAt.Format(time.RFC3339) != "2026-09-29T00:52:20Z" {
		t.Errorf("expected factual fetched_at timestamp 2026-09-29T00:52:20Z, got %v", h4[0].FirstObservedAt)
	}

	// Step 3: Idempotent Re-apply
	applyRes2, err := inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("idempotent apply failed: %v", err)
	}
	if applyRes2.InsertedRecords != 0 || applyRes2.NewlyInserted != 0 || applyRes2.ExistingRecords != 5 {
		t.Errorf("expected 0 newly inserted and 5 existing on idempotent re-apply, got inserted=%d newly=%d existing=%d",
			applyRes2.InsertedRecords, applyRes2.NewlyInserted, applyRes2.ExistingRecords)
	}
	var totalRows int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_source_history;").Scan(&totalRows)
	if totalRows != 5 {
		t.Fatalf("idempotent apply must not create duplicate rows, expected 5, got %d", totalRows)
	}
}

func TestRecoverSourceHistory_TamperedCredentialsRejected(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "tamper_target.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Exploit Test: Tamper with orphan1's UUID in target database while keeping protocol, server, port identical
	_, err = db.ExecContext(ctx, `UPDATE nodes SET config_json = '{"uuid":"TAMPERED-UUID-EXPLOIT"}' WHERE logical_id = 'node-orphan-1';`)
	if err != nil {
		t.Fatalf("failed to tamper node: %v", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE node_connection_versions SET effective_config_json = '{"credentials":{"uuid":"TAMPERED-UUID-EXPLOIT"}}' WHERE node_logical_id = 'node-orphan-1';`)
	if err != nil {
		t.Fatalf("failed to tamper connection versions: %v", err)
	}

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "tamper-run",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	manifestPath, manifestHash := createTestManifestFile(t, tempDir, manifest)

	// Dry run MUST FAIL due to credential mismatch with archive!
	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 true,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected FAIL-CLOSED error when target node credentials differ from archive evidence, but dry run succeeded")
	}

	// Apply MUST ALSO FAIL closed
	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected FAIL-CLOSED error on apply when target node credentials differ from archive evidence")
	}

	// Verify 0 rows written
	var count int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_source_history;").Scan(&count)
	if count != 0 {
		t.Fatalf("fail-closed must write 0 rows to node_source_history, found %d", count)
	}
}

func TestRecoverSourceHistory_TamperedManifestOrOldPK(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "tamper_manifest.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Subtest 1: Sha256 mismatch
	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "sha-test",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	manifestPath, _ := createTestManifestFile(t, tempDir, manifest)

	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		DryRun:                 true,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected error on manifest sha256 mismatch")
	}

	// Subtest 2: Old PK mismatch (invalid link_pk)
	badPKManifest := manifest
	badPKManifest.Nodes[0].PrimaryHistoricalSource.LegacyLinkPK = 999999
	badPath, badHash := createTestManifestFile(t, tempDir, badPKManifest)

	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           badPath,
		ExpectedManifestSHA256: badHash,
		DryRun:                 true,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected error when legacy link pk does not exist in archive")
	}
}

func TestRecoverSourceHistory_Schema15_DryRunReadOnlyNoDDL(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	// Create database at Schema 15 (without Migration 16)
	schema15Path := filepath.Join(tempDir, "schema15.db")
	db15, err := sqlite.Open(sqlite.Config{Path: schema15Path, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open schema15 db: %v", err)
	}

	runner := sqlite.NewMigrationRunner(db15, migrations.FS)
	if err := runner.RunUpTo(ctx, 15); err != nil {
		t.Fatalf("failed to run migrations up to 15: %v", err)
	}

	// Verify schema is at 15
	var v int
	_ = db15.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations;").Scan(&v)
	if v != 15 {
		t.Fatalf("expected schema version 15, got %d", v)
	}
	var histTblExists int
	_ = db15.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='node_source_history';").Scan(&histTblExists)
	if histTblExists != 0 {
		t.Fatalf("node_source_history must NOT exist at schema 15")
	}

	setupTargetDBWithFixtures(t, db15)
	_ = db15.Close()

	// Hash schema 15 file before dry-run
	initialData, _ := os.ReadFile(schema15Path)
	initialHash := sha256.Sum256(initialData)

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "schema15-test",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	manifestPath, manifestHash := createTestManifestFile(t, tempDir, manifest)

	// Step 1: Open strictly read-only for dry-run
	roDB, err := sqlite.OpenReadOnly(schema15Path)
	if err != nil {
		t.Fatalf("failed to open read-only db: %v", err)
	}
	defer roDB.Close()

	dryRes, err := inventory.RecoverSourceHistory(ctx, roDB, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 true,
		TargetDBPath:           schema15Path,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("dry run on schema 15 failed: %v", err)
	}
	if dryRes.Status != "dry_run_success" {
		t.Errorf("expected dry_run_success, got %s", dryRes.Status)
	}
	if dryRes.PlannedSchema != "migration_16_required" {
		t.Errorf("expected PlannedSchema migration_16_required, got %s", dryRes.PlannedSchema)
	}
	if dryRes.InsertedRecords != 0 {
		t.Errorf("expected 0 inserted records in dry run, got %d", dryRes.InsertedRecords)
	}

	_ = roDB.Close()

	// Verify database file is 100% UNCHANGED (0 DDL writes, 0 bytes modified)
	postData, _ := os.ReadFile(schema15Path)
	postHash := sha256.Sum256(postData)
	if initialHash != postHash {
		t.Fatalf("CRITICAL: dry run modified database file on disk!")
	}

	// Step 2: Apply on Schema 15 MUST FAIL closed because table does not exist
	rwDB, err := sqlite.Open(sqlite.Config{Path: schema15Path, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open rw db: %v", err)
	}
	defer rwDB.Close()

	_, err = inventory.RecoverSourceHistory(ctx, rwDB, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 false,
		TargetDBPath:           schema15Path,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected apply on schema 15 to fail closed with missing migration 16 error")
	}
}

func TestRecoverSourceHistory_HistoricalConnectionVersionMatch(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "hist_ver_target.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Simulate node whose current connection revision is 2 (modified credentials),
	// but connection revision 1 in node_connection_versions matches the archive!
	// 1. Insert revision 1 into node_connection_versions
	rev1JSON := `{"credentials":{"uuid":"501e2af2-26f2-466c-9e91-a87c5a738ad8","transport":{"flow":"xtls-rprx-vision","sni":"itunes.apple.com","tls":"true"}}}`
	_, _ = db.ExecContext(ctx, `INSERT INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, recorded_at) VALUES
		('node-orphan-1', 1, ?, '2026-09-11 08:26:06');`, rev1JSON)

	// 2. Advance current node to revision 2 with different credentials
	_, _ = db.ExecContext(ctx, `UPDATE nodes SET connection_revision = 2, config_json = '{"uuid":"NEW-REV-2-UUID"}' WHERE logical_id = 'node-orphan-1';`)
	_, _ = db.ExecContext(ctx, `INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at) VALUES
		('node-orphan-1', 2, '2026-09-20 00:00:00');`)

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "hist-ver-run",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	manifestPath, manifestHash := createTestManifestFile(t, tempDir, manifest)

	// Since historical version 1 matches the archive, recovery must SUCCEED!
	res, err := inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           manifestPath,
		ExpectedManifestSHA256: manifestHash,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("expected recovery to match historical version in node_connection_versions, got error: %v", err)
	}
	if res.Status != "success" || res.InsertedRecords != 1 {
		t.Fatalf("expected 1 record inserted via historical revision match, got %#v", res)
	}
}

func TestRecoverSourceHistory_TamperedTransportOrWGKey(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "tamper_transport.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Case 1: Tamper SNI on orphan 1 (VLESS)
	_, _ = db.ExecContext(ctx, `UPDATE nodes SET config_json = '{"uuid":"501e2af2-26f2-466c-9e91-a87c5a738ad8","transport":{"sni":"tampered.sni.com","tls":"true"}}' WHERE logical_id = 'node-orphan-1';`)
	_, _ = db.ExecContext(ctx, `UPDATE node_connection_versions SET effective_config_json = '{"credentials":{"uuid":"501e2af2-26f2-466c-9e91-a87c5a738ad8","transport":{"sni":"tampered.sni.com","tls":"true"}}}' WHERE node_logical_id = 'node-orphan-1';`)

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "sni-tamper",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	mPath, mHash := createTestManifestFile(t, tempDir, manifest)

	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           mPath,
		ExpectedManifestSHA256: mHash,
		DryRun:                 true,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected fail-closed error when transport SNI differs from archive, got nil")
	}

	// Case 2: Tamper WireGuard public key on orphan 3
	_, _ = db.ExecContext(ctx, `UPDATE nodes SET config_json = '{"public_key":"TAMPERED-PUBKEY=","preshared_key":"psk333="}' WHERE logical_id = 'node-orphan-3';`)
	_, _ = db.ExecContext(ctx, `UPDATE node_connection_versions SET effective_config_json = '{"credentials":{"public_key":"TAMPERED-PUBKEY=","preshared_key":"psk333="}}' WHERE node_logical_id = 'node-orphan-3';`)

	manifestWG := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "wg-tamper",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-3",
				Protocol:         "wireguard",
				Server:           "orphan3.example.com",
				Port:             51820,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         300,
					LegacyLinkPK:         3001,
					LegacySourceID:       3,
					HistoricalSourceName: "OldDeleted",
					RelationState:        "verified",
				},
			},
		},
	}
	mWGPath, mWGHash := createTestManifestFile(t, tempDir, manifestWG)

	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           mWGPath,
		ExpectedManifestSHA256: mWGHash,
		DryRun:                 true,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected fail-closed error when WireGuard public key differs from archive, got nil")
	}
}

func TestRecoverSourceHistory_DynamicCounts_NoHardcodedCounts(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "dynamic_counts.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Add 3 new active nodes and 1 extra subscription to database
	nodeRepo := sqlite.NewNodeRepository(db)
	for i := 1; i <= 3; i++ {
		_ = nodeRepo.UpsertBatch(ctx, []domain.Node{
			{
				LogicalID:          fmt.Sprintf("node-new-extra-%d", i),
				Protocol:           domain.ProtocolVLESS,
				DisplayName:        fmt.Sprintf("Extra Node %d", i),
				Server:             fmt.Sprintf("extra%d.test", i),
				Port:               443,
				Active:             true,
				ConnectionRevision: 1,
				CreatedAt:          time.Now().UTC(),
				UpdatedAt:          time.Now().UTC(),
			},
		})
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO subscriptions (id, name, source_url_secret_ref, revision, created_at, updated_at) VALUES
		('sub-extra-1', 'ExtraSub', 'https://extra.sub/url', 'revX', '2026-10-01', '2026-10-01');`)

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "dyn-counts-test",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	mPath, mHash := createTestManifestFile(t, tempDir, manifest)

	res, err := inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           mPath,
		ExpectedManifestSHA256: mHash,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("recovery failed with extra assets: %v", err)
	}
	// Invariants should dynamically reflect the 3 extra nodes (total 8 nodes, 4 active)
	if res.PostInvariants.TotalNodes != 8 {
		t.Errorf("expected 8 total nodes dynamically, got %d", res.PostInvariants.TotalNodes)
	}
	if res.PostInvariants.ActiveNodes != 4 {
		t.Errorf("expected 4 active nodes dynamically, got %d", res.PostInvariants.ActiveNodes)
	}
}

func TestCanonicalSourceURL_NormalizationRules(t *testing.T) {
	// Rule 1: Same host & path but different query tokens must NEVER produce the same canonical URL
	url1 := "https://example.com/api/v1/client/subscribe?token=token_alpha"
	url2 := "https://example.com/api/v1/client/subscribe?token=token_beta"
	c1 := inventory.CanonicalSourceURL(url1)
	c2 := inventory.CanonicalSourceURL(url2)
	if c1 == c2 {
		t.Fatalf("different query tokens on same host/path must NOT produce same canonical URL: %s == %s", c1, c2)
	}

	// Rule 2: Case normalization on scheme and host
	urlUpper := "HTTPS://EXAMPLE.COM:443/Sub/Path/?token=abc#frag"
	urlLower := "https://example.com/Sub/Path/?token=abc"
	cUpper := inventory.CanonicalSourceURL(urlUpper)
	cLower := inventory.CanonicalSourceURL(urlLower)
	if cUpper != cLower {
		t.Fatalf("expected scheme/host case and default port 443 normalized to match: %s != %s", cUpper, cLower)
	}

	// Rule 3: Path case and trailing slashes are strictly preserved
	urlTrailing := "https://example.com/sub/"
	urlNoTrailing := "https://example.com/sub"
	if inventory.CanonicalSourceURL(urlTrailing) == inventory.CanonicalSourceURL(urlNoTrailing) {
		t.Fatalf("trailing slash must not be arbitrarily trimmed")
	}

	// Rule 4: Userinfo is preserved
	urlUser := "https://user:pass@example.com/sub"
	cUser := inventory.CanonicalSourceURL(urlUser)
	if !strings.Contains(cUser, "user:pass@") {
		t.Fatalf("userinfo must be preserved, got %s", cUser)
	}

	// Rule 5: Fragment is stripped
	urlFrag := "https://example.com/sub?token=abc#section-1"
	cFrag := inventory.CanonicalSourceURL(urlFrag)
	if strings.Contains(cFrag, "#") {
		t.Fatalf("fragment must be stripped, got %s", cFrag)
	}
}

func TestRecoverSourceHistory_TransportHeadersNegativeRejected(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "transport_header_test.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Update node-orphan-1 in target DB to have a conflicting transport header
	// Same server and port, same UUID, but different transport Host header
	conflictingJSON := `{"credentials":{"uuid":"501e2af2-26f2-466c-9e91-a87c5a738ad8","transport":{"type":"ws","path":"/vless-ws","host":"conflicting-host.evil.com","tls":"true"}}}`
	_, err = db.ExecContext(ctx, `UPDATE nodes SET config_json = ? WHERE logical_id = 'node-orphan-1';`, conflictingJSON)
	if err != nil {
		t.Fatalf("failed to update node config: %v", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE node_connection_versions SET effective_config_json = ? WHERE node_logical_id = 'node-orphan-1';`, conflictingJSON)
	if err != nil {
		t.Fatalf("failed to update node connection versions: %v", err)
	}

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "transport-test",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	mPath, mHash := createTestManifestFile(t, tempDir, manifest)

	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           mPath,
		ExpectedManifestSHA256: mHash,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected recovery to fail closed when transport headers conflict, but it succeeded")
	}
	if !strings.Contains(err.Error(), "configuration mismatch with historical archive evidence") {
		t.Fatalf("expected configuration mismatch error, got: %v", err)
	}
}

func TestRecoverSourceHistory_ImmutableEvidenceConflictRejected(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "immutable_evidence_test.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Pre-insert an existing history record for node-orphan-1 with the same evidence_key but different source_identity
	evidenceKey := "legacy_cold_archive:pk:100:link:1001"
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_source_history (
			id, node_logical_id, subscription_id, source_identity, source_label,
			relation_state, cause, first_observed_at, last_observed_at,
			evidence_kind, evidence_key, evidence_json, created_at
		) VALUES (
			'01925b74-1234-7000-8000-000000000001', 'node-orphan-1', NULL,
			'legacy:src:999_DIFFERENT', 'OldLabel', 'verified', 'legacy_import',
			'2026-09-11 08:26:06', '2026-09-11 08:26:06',
			'legacy_cold_archive', ?, '{"data":"original"}', '2026-10-04T00:00:00Z'
		);`, evidenceKey)
	if err != nil {
		t.Fatalf("failed to pre-insert history record: %v", err)
	}

	manifest := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "immutable-conflict-test",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1, // Conflicts with legacy:src:999_DIFFERENT!
					HistoricalSourceName: "7li",
					RelationState:        "verified",
				},
			},
		},
	}
	mPath, mHash := createTestManifestFile(t, tempDir, manifest)

	_, err = inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           mPath,
		ExpectedManifestSHA256: mHash,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err == nil {
		t.Fatalf("expected recovery to fail closed on immutable evidence conflict, but it succeeded")
	}
	if !strings.Contains(err.Error(), "historical evidence conflict") {
		t.Fatalf("expected historical evidence conflict error, got: %v", err)
	}
}

func TestService_SourceDeletedAndSourceUnmappedSemantics(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	dbPath := filepath.Join(tempDir, "source_deleted_unmapped_test.db")
	db, err := sqlite.Open(sqlite.Config{Path: dbPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	nodeRepo := sqlite.NewNodeRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	historyRepo := sqlite.NewNodeSourceHistoryRepository(db)
	svc := inventory.NewService(db, subRepo, nil, nodeRepo, nil, nil)

	testNodeID := "node-sem-test-1"
	_ = nodeRepo.UpsertBatch(ctx, []domain.Node{
		{
			LogicalID:          testNodeID,
			Protocol:           domain.ProtocolVLESS,
			DisplayName:        "Semantics Test Node",
			Server:             "test.example.com",
			Port:               443,
			Active:             false,
			ConnectionRevision: 1,
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		},
	})

	// Create 1 active subscription, 1 disabled subscription, and 1 deleted subscription
	activeSubID := "01a00000-0000-7000-8000-000000000001"
	disabledSubID := "01a00000-0000-7000-8000-000000000002"
	deletedSubID := "01a00000-0000-7000-8000-000000000099"

	nowStr := domain.NowUTC().Format(time.RFC3339)
	_, _ = db.ExecContext(ctx, `INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at) VALUES
		(?, 'Active Sub', 'https://act.example.com', 1, 'rev1', ?, ?),
		(?, 'Disabled Sub', 'https://dis.example.com', 0, 'rev2', ?, ?),
		(?, 'Will Delete Sub', 'https://del.example.com', 1, 'rev3', ?, ?);`,
		activeSubID, nowStr, nowStr, disabledSubID, nowStr, nowStr, deletedSubID, nowStr, nowStr)

	// Record 1: Unmapped historical import (subscription_id is NULL, cause is legacy_import)
	recUnmapped := domain.NodeSourceHistory{
		ID:             domain.MustNewUUIDv7(),
		NodeLogicalID:  testNodeID,
		SubscriptionID: nil,
		SourceIdentity: "legacy:src:10",
		SourceLabel:    "Historical Unmapped",
		RelationState:  domain.RelationStateVerified,
		Cause:          domain.CauseLegacyImport,
		EvidenceKind:   "legacy_cold_archive",
		EvidenceKey:    "test:unmapped:1",
		EvidenceJSON:   "{}",
		CreatedAt:      time.Now().UTC(),
	}

	// Record 2: Subscription deletion event (cause is subscription_deleted)
	recSubDeleted := domain.NodeSourceHistory{
		ID:             domain.MustNewUUIDv7(),
		NodeLogicalID:  testNodeID,
		SubscriptionID: nil,
		SourceIdentity: "runtime:del:1",
		SourceLabel:    "Deleted Source",
		RelationState:  domain.RelationStateVerified,
		Cause:          domain.CauseSubscriptionDeleted,
		EvidenceKind:   "subscription_deletion_snapshot",
		EvidenceKey:    "test:deleted:1",
		EvidenceJSON:   "{}",
		CreatedAt:      time.Now().UTC(),
	}

	// Record 3: Historical record pointing to deleted sub ID that is not in subscriptions table
	recTargetDeleted := domain.NodeSourceHistory{
		ID:             domain.MustNewUUIDv7(),
		NodeLogicalID:  testNodeID,
		SubscriptionID: &deletedSubID,
		SourceIdentity: "runtime:old:1",
		SourceLabel:    "Target Existed But Now Deleted",
		RelationState:  domain.RelationStateVerified,
		Cause:          domain.CauseRefreshRemoved,
		EvidenceKind:   "refresh_snapshot",
		EvidenceKey:    "test:deleted_target:1",
		EvidenceJSON:   "{}",
		CreatedAt:      time.Now().UTC(),
	}

	// Record 4: Disabled subscription (subscription_id is disabledSubID, subscription exists)
	recDisabled := domain.NodeSourceHistory{
		ID:             domain.MustNewUUIDv7(),
		NodeLogicalID:  testNodeID,
		SubscriptionID: &disabledSubID,
		SourceIdentity: "runtime:dis:1",
		SourceLabel:    "Disabled Sub",
		RelationState:  domain.RelationStateVerified,
		Cause:          domain.CauseRefreshRemoved,
		EvidenceKind:   "refresh_snapshot",
		EvidenceKey:    "test:disabled:1",
		EvidenceJSON:   "{}",
		CreatedAt:      time.Now().UTC(),
	}

	// Record 5: Active subscription (subscription_id is activeSubID, subscription exists)
	recActive := domain.NodeSourceHistory{
		ID:             domain.MustNewUUIDv7(),
		NodeLogicalID:  testNodeID,
		SubscriptionID: &activeSubID,
		SourceIdentity: "runtime:act:1",
		SourceLabel:    "Active Sub",
		RelationState:  domain.RelationStateVerified,
		Cause:          domain.CauseRefreshRemoved,
		EvidenceKind:   "refresh_snapshot",
		EvidenceKey:    "test:active:1",
		EvidenceJSON:   "{}",
		CreatedAt:      time.Now().UTC(),
	}

	if err := historyRepo.InsertBatch(ctx, []domain.NodeSourceHistory{
		recUnmapped, recSubDeleted, recTargetDeleted, recDisabled, recActive,
	}); err != nil {
		t.Fatalf("failed to insert history records: %v", err)
	}

	// Now delete deletedSubID with FK temporarily disabled so the reference remains to simulate historical dangling pointer
	_, _ = db.ExecContext(ctx, "PRAGMA foreign_keys = OFF;")
	_, _ = db.ExecContext(ctx, "DELETE FROM subscriptions WHERE id = ?;", deletedSubID)
	_, _ = db.ExecContext(ctx, "PRAGMA foreign_keys = ON;")

	resp, err := svc.GetNodeSourceHistory(ctx, testNodeID)
	if err != nil {
		t.Fatalf("GetNodeSourceHistory failed: %v", err)
	}

	findView := func(label string) inventory.NodeSourceHistoryItemView {
		for _, h := range resp.History {
			if h.SourceLabel == label {
				return h
			}
		}
		t.Fatalf("label %s not found in history response", label)
		return inventory.NodeSourceHistoryItemView{}
	}

	// 1. Unmapped: source_deleted = false, source_unmapped = true
	vUnmapped := findView("Historical Unmapped")
	if vUnmapped.SourceDeleted {
		t.Errorf("unmapped record must have source_deleted false, got true")
	}
	if !vUnmapped.SourceUnmapped {
		t.Errorf("unmapped record must have source_unmapped true, got false")
	}

	// 2. Cause subscription_deleted: source_deleted = true, source_unmapped = false
	vSubDel := findView("Deleted Source")
	if !vSubDel.SourceDeleted {
		t.Errorf("cause subscription_deleted must have source_deleted true, got false")
	}
	if vSubDel.SourceUnmapped {
		t.Errorf("cause subscription_deleted must have source_unmapped false, got true")
	}

	// 3. Target deleted: source_deleted = true, source_unmapped = false
	vTargetDel := findView("Target Existed But Now Deleted")
	if !vTargetDel.SourceDeleted {
		t.Errorf("target deleted record must have source_deleted true, got false")
	}
	if vTargetDel.SourceUnmapped {
		t.Errorf("target deleted record must have source_unmapped false, got true")
	}

	// 4. Disabled sub: source_deleted = false, source_unmapped = false
	vDisabled := findView("Disabled Sub")
	if vDisabled.SourceDeleted {
		t.Errorf("disabled subscription must have source_deleted false, got true")
	}
	if vDisabled.SourceUnmapped {
		t.Errorf("disabled subscription must have source_unmapped false, got true")
	}

	// 5. Active sub: source_deleted = false, source_unmapped = false
	vActive := findView("Active Sub")
	if vActive.SourceDeleted {
		t.Errorf("active subscription must have source_deleted false, got true")
	}
	if vActive.SourceUnmapped {
		t.Errorf("active subscription must have source_unmapped false, got true")
	}
}

func TestModernSubscriptionMapping_StrictScoping(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	coldPath, coldHash := createColdArchiveFixture(t, tempDir)

	targetDBPath := filepath.Join(tempDir, "strict_scoping_test.db")
	db, err := sqlite.Open(sqlite.Config{Path: targetDBPath, ForeignKeys: true})
	if err != nil {
		t.Fatalf("failed to open target db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	setupTargetDBWithFixtures(t, db)

	// Add two modern subscriptions with the exact same URL as cold archive source 1 (ambiguous multiple subs)
	subAmbiguousID1 := "01a00000-0000-7000-8000-000000000098"
	subAmbiguousID2 := "01a00000-0000-7000-8000-000000000099"
	nowStr := domain.NowUTC().Format(time.RFC3339)
	_, _ = db.ExecContext(ctx, `INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at) VALUES
		(?, 'Duplicate URL Sub 1', 'https://z.7li7li.com/api/v1/client/subscribe?token=tok1', 1, 'rev1', ?, ?),
		(?, 'Duplicate URL Sub 2', 'https://z.7li7li.com/api/v1/client/subscribe?token=tok1', 1, 'rev1', ?, ?);`,
		subAmbiguousID1, nowStr, nowStr, subAmbiguousID2, nowStr, nowStr)

	// Manifest without candidate target_subscription_id: must remain unmapped (NULL subscription_id), NOT arbitrarily pick one!
	manifestAmbiguous := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "ambiguous-test",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-1",
				Protocol:         "vless",
				Server:           "orphan1.example.com",
				Port:             8443,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         100,
					LegacyLinkPK:         1001,
					LegacySourceID:       1,
					HistoricalSourceName: "7li",
					TargetSubscriptionID: "", // No candidate ID proof
					RelationState:        "verified",
				},
			},
		},
	}
	mPath1, mHash1 := createTestManifestFile(t, tempDir, manifestAmbiguous)

	res1, err := inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           mPath1,
		ExpectedManifestSHA256: mHash1,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if res1.InsertedRecords != 1 {
		t.Fatalf("expected 1 record inserted, got %d", res1.InsertedRecords)
	}

	hRepo := sqlite.NewNodeSourceHistoryRepository(db)
	hList, err := hRepo.ListByNodeLogicalID(ctx, "node-orphan-1")
	if err != nil || len(hList) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(hList))
	}
	// Ambiguous multiple subscriptions without stable ID proof must be left unmapped (NULL)
	if hList[0].SubscriptionID != nil {
		t.Fatalf("ambiguous subscription URL without candidate ID proof must have NULL subscription_id, got %v", *hList[0].SubscriptionID)
	}

	// Scenario C: Modern subscription with matching name 'OldDeleted' but DIFFERENT URL
	// Must NOT link by name fallback! subscription_id must remain NULL (unmapped)!
	subDifferentURLID := "01a00000-0000-7000-8000-000000000077"
	_, _ = db.ExecContext(ctx, `INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at) VALUES
		(?, 'OldDeleted', 'https://completely-different-url.com/sub', 1, 'rev1', ?, ?);`, subDifferentURLID, nowStr, nowStr)

	manifestNoNameFallback := inventory.RecoveryManifest{
		SchemaVersion: "2.1.0",
		Metadata: inventory.ManifestMetadata{
			RunID: "no-name-fallback-test",
			ColdArchive: inventory.ManifestArchive{
				Path:   coldPath,
				SHA256: coldHash,
			},
		},
		Nodes: []inventory.ManifestNode{
			{
				NodeLogicalID:    "node-orphan-3",
				Protocol:         "wireguard",
				Server:           "orphan3.example.com",
				Port:             51820,
				ProvenanceOrigin: "legacy_cold_archive",
				PrimaryHistoricalSource: &inventory.ManifestSourceEdge{
					RelationshipKind:     "primary_imported_source",
					LegacyNodePK:         300,
					LegacyLinkPK:         3001,
					LegacySourceID:       3, // OldDeleted in archive
					HistoricalSourceName: "OldDeleted",
					RelationState:        "verified",
				},
			},
		},
	}
	mPath3, mHash3 := createTestManifestFile(t, tempDir, manifestNoNameFallback)
	res3, err3 := inventory.RecoverSourceHistory(ctx, db, inventory.RecoveryConfig{
		ManifestPath:           mPath3,
		ExpectedManifestSHA256: mHash3,
		DryRun:                 false,
		TargetDBPath:           targetDBPath,
		ArchiveDir:             tempDir,
	})
	if err3 != nil {
		t.Fatalf("recovery failed: %v", err3)
	}
	if res3.InsertedRecords != 1 {
		t.Fatalf("expected 1 record inserted, got %d", res3.InsertedRecords)
	}

	h3List, err := hRepo.ListByNodeLogicalID(ctx, "node-orphan-3")
	if err != nil || len(h3List) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(h3List))
	}
	// Name fallback must NOT happen even though name matches 'OldDeleted'!
	if h3List[0].SubscriptionID != nil {
		t.Fatalf("must NOT fallback to name matching when URL differs, got subscription_id: %v", *h3List[0].SubscriptionID)
	}
}
