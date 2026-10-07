package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func createTestDBFile(t *testing.T, dbPath string) {
	t.Helper()
	dbCfg := sqlite.DefaultConfig(dbPath)
	db, err := sqlite.Open(dbCfg)
	if err != nil {
		t.Fatalf("failed to open test db file: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	ctx := context.Background()
	nowStr := domain.NowUTC().Format(time.RFC3339)

	// Insert subscriptions
	subs := []struct {
		id   string
		name string
		url  string
	}{
		{"01a0b9af-c116-7204-b4a3-98a91733145d", "7li", "https://z.7li7li.com/api/v1/client/subscribe"},
		{"01a0b9af-c116-77cf-86e2-cba027607d17", "魔戒", "https://msub.xn--m7r52rosihxm.com/api/v1/client/subscribe"},
		{"01a0b9af-c116-7967-bc7f-197aa43a2e49", "Dogegg", "https://traffic.dogeggo.us.ci/sub/UiDmU_4Qup1"},
	}
	for _, s := range subs {
		_, err := db.ExecContext(ctx, `
			INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
			VALUES (?, ?, ?, 1, 'rev1', ?, ?);
		`, s.id, s.name, s.url, nowStr, nowStr)
		if err != nil {
			t.Fatalf("insert sub: %v", err)
		}
	}

	// Insert sample node
	nodeID := domain.MustNewUUIDv7()
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at)
		VALUES (?, 'vmess', 'Node1', '1.2.3.4', 443, '{}', 1, 1, ?, ?);
	`, nodeID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, config_fingerprint, created_at)
		VALUES (?, 1, '{"server":"1.2.3.4","port":443}', 'fp1', ?);
	`, nodeID, nowStr)
	if err != nil {
		t.Fatalf("insert node conn ver: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
		VALUES (?, 1, ?);
	`, nodeID, nowStr)
	if err != nil {
		t.Fatalf("insert node conn head: %v", err)
	}
}

func TestResetNodeInventoryCLI_Validation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	createTestDBFile(t, dbPath)

	t.Run("help flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"reset-node-inventory", "--help"}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("expected exit code 0, got %d", code)
		}
	})

	t.Run("neither dry-run nor apply specified", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"reset-node-inventory", "--db", dbPath}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "either --dry-run or --apply must be explicitly specified") {
			t.Errorf("expected fail-safe error message, got %q", stderr.String())
		}
	})

	t.Run("apply without backup confirmation rejected", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"reset-node-inventory", "--db", dbPath, "--apply"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1 without backup confirmation, got %d", code)
		}
		if !strings.Contains(stderr.String(), "apply requires backup confirmation") {
			t.Errorf("expected backup confirmation error, got %q", stderr.String())
		}
	})

	t.Run("both dry-run and apply specified", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"reset-node-inventory", "--db", dbPath, "--dry-run", "--apply"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("expected exit code 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "cannot specify both --dry-run and --apply") {
			t.Errorf("expected mutual exclusion error message, got %q", stderr.String())
		}
	})
}

func TestResetNodeInventoryCLI_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "dry_run.db")
	createTestDBFile(t, dbPath)

	var stdout, stderr bytes.Buffer
	code := run([]string{"reset-node-inventory", "--db", dbPath, "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. stderr: %s", code, stderr.String())
	}

	var rep domain.NodeInventoryResetReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v. Raw: %s", err, stdout.String())
	}

	if !rep.DryRun {
		t.Errorf("expected DryRun true")
	}
	if rep.PreCounts["nodes"] != 1 {
		t.Errorf("expected pre nodes 1, got %d", rep.PreCounts["nodes"])
	}

	// Verify DB file was not modified
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	var cnt int
	_ = db.QueryRow("SELECT COUNT(*) FROM nodes;").Scan(&cnt)
	if cnt != 1 {
		t.Errorf("expected nodes still 1 after dry run, got %d", cnt)
	}
}

func TestResetNodeInventoryCLI_Apply(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apply.db")
	createTestDBFile(t, dbPath)

	var stdout, stderr bytes.Buffer
	code := run([]string{"reset-node-inventory", "--db", dbPath, "--apply", "--confirm-backup", "--restore-source-tokens", "--archive-db", "/home/service/backups/csp-legacy-cold-archive-20260919.db"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. stderr: %s", code, stderr.String())
	}

	var rep domain.NodeInventoryResetReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v. Raw: %s", err, stdout.String())
	}

	if rep.DryRun {
		t.Errorf("expected DryRun false")
	}
	if rep.PostCounts["nodes"] != 0 {
		t.Errorf("expected post nodes 0, got %d", rep.PostCounts["nodes"])
	}
	if len(rep.FKViolations) > 0 {
		t.Errorf("expected 0 FK violations, got %v", rep.FKViolations)
	}
	if !rep.PreservedAssetsUntouched {
		t.Errorf("expected preserved assets untouched")
	}
	if !rep.SourceURLsRestored["7li"] || !rep.SourceURLsRestored["魔戒"] {
		t.Errorf("expected 7li and 魔戒 to be restored")
	}

	// Verify in DB directly
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var nodeCnt int
	_ = db.QueryRow("SELECT COUNT(*) FROM nodes;").Scan(&nodeCnt)
	if nodeCnt != 0 {
		t.Errorf("expected 0 nodes in DB, got %d", nodeCnt)
	}

	var url7li string
	_ = db.QueryRow("SELECT source_url_secret_ref FROM subscriptions WHERE name = '7li';").Scan(&url7li)
	if !strings.Contains(url7li, "token=") {
		t.Errorf("expected 7li to have token, got %s", url7li)
	}
}
