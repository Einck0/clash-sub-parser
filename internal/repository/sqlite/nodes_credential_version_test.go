package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestNodeCredentialVersionMigrationAndOperations(t *testing.T) {
	ctx := context.Background()

	// 1. Set up an empty DB and apply only migrations 1 through 5 (legacy DB before 000006)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	// Read and apply migrations 1 through 5 manually to simulate pre-000006 database state
	legacyFiles := []string{
		"000001_initial_schema.sql",
		"000002_ip_risk_schema.sql",
		"000003_subscription_config.sql",
		"000004_admin_token.sql",
		"000005_node_credentials.sql",
	}

	// Ensure schema_migrations table
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("failed to create schema_migrations: %v", err)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	for i, f := range legacyFiles {
		content, err := migrations.FS.ReadFile(f)
		if err != nil {
			t.Fatalf("failed to read embedded migration %s: %v", f, err)
		}
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			t.Fatalf("failed to execute legacy migration %s: %v", f, err)
		}
		ver := i + 1
		if _, err := db.ExecContext(ctx, "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?);", ver, f, nowStr); err != nil {
			t.Fatalf("failed to record migration %s: %v", f, err)
		}
	}

	// Insert legacy node row in the old nodes table (which had no credential_version column)
	legacyNodeID := "legacy-node-001"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?);
	`, legacyNodeID, "ss", "Legacy Node 1", "ref-legacy", 1, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert legacy node: %v", err)
	}

	// 2. Now run the full migration runner (should apply 000006_node_credential_version.sql)
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to run migrations including 000006: %v", err)
	}

	// 3. Verify reading legacy node via NodeRepository yields CredentialVersion == 0
	nodeRepo := sqlite.NewNodeRepository(db)
	node, err := nodeRepo.GetByLogicalID(ctx, legacyNodeID)
	if err != nil {
		t.Fatalf("failed to query legacy node via repository: %v", err)
	}
	if node.CredentialVersion != 0 {
		t.Fatalf("expected legacy node CredentialVersion to be 0 (fail-closed), got %d", node.CredentialVersion)
	}

	// Verify List also returns CredentialVersion == 0
	nodes, total, err := nodeRepo.List(ctx, domain.NodeFilter{})
	if err != nil {
		t.Fatalf("failed to list nodes: %v", err)
	}
	if total != 1 || len(nodes) != 1 {
		t.Fatalf("expected 1 node, got total=%d len=%d", total, len(nodes))
	}
	if nodes[0].CredentialVersion != 0 {
		t.Fatalf("expected listed legacy node CredentialVersion == 0, got %d", nodes[0].CredentialVersion)
	}

	// 4. Test UpsertBatch with a specific non-zero CredentialVersion
	newNodeID := "versioned-node-002"
	newNode := domain.Node{
		LogicalID:                 newNodeID,
		Protocol:                  domain.ProtocolVMess,
		DisplayName:               "Versioned Node 2",
		NormalizedConfigSecretRef: "ref-v2",
		CredentialVersion:         3,
		Active:                    true,
		CreatedAt:                 domain.NowUTC(),
		UpdatedAt:                 domain.NowUTC(),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{newNode}); err != nil {
		t.Fatalf("failed to upsert new versioned node: %v", err)
	}

	fetchedNew, err := nodeRepo.GetByLogicalID(ctx, newNodeID)
	if err != nil {
		t.Fatalf("failed to get new node: %v", err)
	}
	if fetchedNew.CredentialVersion != 3 {
		t.Fatalf("expected CredentialVersion 3, got %d", fetchedNew.CredentialVersion)
	}

	// 5. Test updating existing node's CredentialVersion via UpsertBatch
	fetchedNew.CredentialVersion = 4
	fetchedNew.DisplayName = "Versioned Node 2 Updated"
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{*fetchedNew}); err != nil {
		t.Fatalf("failed to update node credential version: %v", err)
	}

	updatedNode, err := nodeRepo.GetByLogicalID(ctx, newNodeID)
	if err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}
	if updatedNode.CredentialVersion != 4 {
		t.Fatalf("expected updated CredentialVersion 4, got %d", updatedNode.CredentialVersion)
	}
	if updatedNode.DisplayName != "Versioned Node 2 Updated" {
		t.Fatalf("expected updated DisplayName, got %s", updatedNode.DisplayName)
	}
}
