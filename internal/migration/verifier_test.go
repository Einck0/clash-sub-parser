package migration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestReadOnlyEnforcement(t *testing.T) {
	// Create a temporary sqlite database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_ro.db")

	// Setup initial data using read-write mode
	rwDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open rw db: %v", err)
	}
	_, err = rwDB.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, val TEXT); INSERT INTO test (val) VALUES ('init');")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	rwDB.Close()

	// Now open using the Verifier in read-only mode
	v := NewVerifier(Options{
		DBPath: dbPath,
	})

	roDB, err := v.OpenReadOnlyDB()
	if err != nil {
		t.Fatalf("failed to open ro db: %v", err)
	}
	defer roDB.Close()

	// Attempt write operation - MUST FAIL
	_, err = roDB.Exec("INSERT INTO test (val) VALUES ('illegal write')")
	if err == nil {
		t.Fatalf("expected write operation to fail on read-only connection, but it succeeded")
	}
}

func TestVerifySyntheticData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_synthetic.db")
	reportPath := filepath.Join(tmpDir, "report.json")

	rwDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	initSQL := `
	CREATE TABLE subscriptions (
		id INTEGER PRIMARY KEY,
		name TEXT,
		url TEXT,
		update_interval INTEGER,
		is_primary INTEGER,
		node_prefix TEXT,
		filter_regex TEXT,
		raw_nodes TEXT,
		last_fetched_at TEXT,
		fetch_comments TEXT,
		last_fetch_error TEXT,
		fetch_failed_count INTEGER,
		subscription_userinfo TEXT,
		profile_update_interval TEXT,
		profile_web_page_url TEXT,
		include_node_names TEXT,
		exclude_node_names TEXT,
		source_nodes TEXT,
		manual_nodes TEXT,
		enabled INTEGER,
		node_renames TEXT,
		proxy_chain TEXT,
		node_proxy_chains TEXT,
		filter_min_speed_mbps REAL,
		filter_media_unlock TEXT
	);
	INSERT INTO subscriptions (id, name, url, enabled, is_primary) VALUES (1, 'Sub1', 'https://example.com/sub', 1, 1);

	CREATE TABLE nodes (
		id INTEGER PRIMARY KEY,
		logical_id TEXT,
		name TEXT,
		protocol TEXT,
		server TEXT,
		port INTEGER,
		normalized_payload TEXT,
		payload_fingerprint TEXT,
		lifecycle_state TEXT,
		created_at TEXT,
		updated_at TEXT
	);
	INSERT INTO nodes (id, logical_id, name, protocol, server, port, normalized_payload, payload_fingerprint, lifecycle_state)
	VALUES (1, 'node-1', 'Test Node', 'vless', 'example.com', 443, '{"uuid":"test-uuid","tls":true,"reality-opts":{"public-key":"test-key","short-id":"abc"}}', 'fp-1', 'active');

	CREATE TABLE node_groups (
		id INTEGER PRIMARY KEY,
		name TEXT,
		kind TEXT,
		group_type TEXT,
		sort_order INTEGER,
		regex_rules TEXT,
		include_nodes TEXT,
		include_group_ids TEXT,
		exclude_nodes TEXT,
		url_test_config TEXT,
		load_balance_config TEXT,
		fallback_config TEXT,
		include_group_nodes_ids TEXT,
		include_entries TEXT,
		add_fallback INTEGER,
		exclude_group_ids TEXT,
		filter_min_speed_mbps REAL,
		filter_media_unlock TEXT
	);
	INSERT INTO node_groups (id, name, kind, group_type, sort_order, include_entries)
	VALUES (1, 'Group1', 'select', 'select', 1, '[{"type":"node","value":"Test Node"}]');

	CREATE TABLE rules (
		id INTEGER PRIMARY KEY,
		name TEXT,
		category TEXT,
		type TEXT,
		value TEXT,
		proxy TEXT,
		options TEXT,
		sort_order INTEGER,
		enabled INTEGER
	);
	INSERT INTO rules (id, name, category, type, value, proxy, enabled)
	VALUES (1, 'Rule1', 'General', 'DOMAIN-SUFFIX', 'google.com', 'Proxy', 1);

	CREATE TABLE node_probe_results (
		id INTEGER PRIMARY KEY,
		node_key TEXT,
		name TEXT,
		server TEXT,
		port INTEGER,
		type TEXT,
		status TEXT,
		latency_ms INTEGER,
		speed_mbps REAL,
		ip TEXT,
		country TEXT,
		asn INTEGER,
		organization TEXT,
		media TEXT,
		error TEXT,
		checked_at INTEGER
	);
	INSERT INTO node_probe_results (id, node_key, name, server, port, type, status, latency_ms, media)
	VALUES (1, 'Test Node|vless|example.com:443', 'Test Node', 'example.com', 443, 'vless', 'ok', 100, '{"youtube":{"unlocked":true}}');
	`

	if _, err := rwDB.Exec(initSQL); err != nil {
		t.Fatalf("failed to insert synthetic data: %v", err)
	}
	rwDB.Close()

	v := NewVerifier(Options{
		DBPath:     dbPath,
		OutputPath: reportPath,
	})

	report, err := v.Verify(context.Background())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	for _, fc := range report.FieldChecks {
		if fc.Failed > 0 {
			t.Logf("FieldCheck failure: %+v", fc)
		}
	}
	t.Logf("Report validation errors: %d, warnings: %d", report.ValidationErrors, report.ValidationWarnings)

	if report.Verdict != "PASS" {
		t.Fatalf("expected PASS verdict, got %s", report.Verdict)
	}
	if report.Entities.Subscriptions.Count != 1 {
		t.Errorf("expected 1 subscription, got %d", report.Entities.Subscriptions.Count)
	}
	if report.Entities.Nodes.Count != 1 {
		t.Errorf("expected 1 node, got %d", report.Entities.Nodes.Count)
	}
	if report.Entities.NodeGroups.Count != 1 {
		t.Errorf("expected 1 group, got %d", report.Entities.NodeGroups.Count)
	}
	if report.Entities.Rules.Count != 1 {
		t.Errorf("expected 1 rule, got %d", report.Entities.Rules.Count)
	}
	if report.Entities.ProbeResults.Count != 1 {
		t.Errorf("expected 1 probe result, got %d", report.Entities.ProbeResults.Count)
	}

	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Fatalf("report file was not created at %s", reportPath)
	}
}

func TestVerifyLiveBackupDB(t *testing.T) {
	// Look for the backup database relative to repository root
	dbPath := filepath.Join("..", "..", "backups", "clash_sub_parser_pre_go_rewrite.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skipf("live backup db not found at %s, skipping live test", dbPath)
	}

	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "live_report.json")

	v := NewVerifier(Options{
		DBPath:     dbPath,
		OutputPath: reportPath,
	})

	report, err := v.Verify(context.Background())
	if err != nil {
		t.Fatalf("verify live db failed: %v", err)
	}

	for _, fc := range report.FieldChecks {
		if fc.Failed > 0 {
			t.Logf("Live DB FieldCheck failure: %+v", fc)
		}
	}
	t.Logf("Live DB validation errors: %d, warnings: %d", report.ValidationErrors, report.ValidationWarnings)

	// Invariant checks as mandated by Card 1.2
	if report.Entities.Nodes.Count != 7236 {
		t.Errorf("expected 7236 nodes, got %d", report.Entities.Nodes.Count)
	}
	if report.Entities.Subscriptions.Count != 8 {
		t.Errorf("expected 8 subscriptions, got %d", report.Entities.Subscriptions.Count)
	}
	if report.Entities.NodeGroups.Count != 29 {
		t.Errorf("expected 29 node groups, got %d", report.Entities.NodeGroups.Count)
	}
	if report.Entities.Rules.Count != 470 {
		t.Errorf("expected 470 rules, got %d", report.Entities.Rules.Count)
	}
	if report.Entities.ProbeResults.Count != 7297 {
		t.Errorf("expected 7297 probe results, got %d", report.Entities.ProbeResults.Count)
	}
	if report.ValidationErrors != 0 {
		t.Errorf("expected 0 validation errors, got %d", report.ValidationErrors)
	}
	if report.Verdict != "PASS" {
		t.Errorf("expected verdict PASS, got %s", report.Verdict)
	}
}

func TestMarkdownReportGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join("..", "..", "backups", "clash_sub_parser_pre_go_rewrite.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skipf("live backup db not found at %s, skipping markdown test", dbPath)
	}

	jsonPath := filepath.Join(tmpDir, "report.json")
	mdPath := filepath.Join(tmpDir, "report.md")

	v := NewVerifier(Options{
		DBPath:       dbPath,
		OutputPath:   jsonPath,
		MarkdownPath: mdPath,
	})

	report, err := v.Verify(context.Background())
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	if report.Verdict != "PASS" {
		t.Fatalf("expected PASS, got %s", report.Verdict)
	}

	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read md report failed: %v", err)
	}
	content := string(mdBytes)
	if !strings.Contains(content, "7,236") {
		t.Errorf("markdown report missing 7,236 node count")
	}
	if !strings.Contains(content, "modernc.org/sqlite") {
		t.Errorf("markdown report missing modernc.org/sqlite engine name")
	}
}

func TestInvalidDatabasePath(t *testing.T) {
	v := NewVerifier(Options{
		DBPath: "/nonexistent/path/to/missing.db",
	})
	_, err := v.Verify(context.Background())
	if err == nil {
		t.Fatalf("expected error for missing db, got nil")
	}
}

