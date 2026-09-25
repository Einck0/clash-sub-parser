package inspect

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestInspectReportsSchemaAndCountsWithoutReadingValues(t *testing.T) {
	source := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE subscriptions (id INTEGER PRIMARY KEY, name TEXT, url TEXT, password TEXT, private_key TEXT);
		CREATE TABLE nodes (id INTEGER PRIMARY KEY, name TEXT, protocol TEXT, server TEXT, port INTEGER, uuid TEXT);
		INSERT INTO subscriptions (name, url, password, private_key) VALUES ('safe-name', 'https://example.invalid/feed', 'fixture-password', 'fixture-private-key');
		INSERT INTO nodes (name, protocol, server, port, uuid) VALUES ('node-name', 'vless', 'node.example.invalid', 443, 'fixture-uuid');
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest := sha256.Sum256(before)
	beforeInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}

	report, err := Inspect(source)
	if err != nil {
		t.Fatal(err)
	}

	if report.SchemaFingerprint == "" {
		t.Fatal("expected a schema fingerprint")
	}
	if report.TotalTables != 2 {
		t.Fatalf("expected 2 tables, got %d", report.TotalTables)
	}
	if report.TotalRows != 2 {
		t.Fatalf("expected 2 total rows, got %d", report.TotalRows)
	}
	if report.Tables["subscriptions"].RowCount != 1 || report.Tables["nodes"].RowCount != 1 {
		t.Fatalf("unexpected row counts: %#v", report.Tables)
	}
	if got := report.Tables["subscriptions"].Fields["password"]; got != FieldSecretExcluded {
		t.Fatalf("password category = %q", got)
	}
	if got := report.Tables["subscriptions"].Fields["private_key"]; got != FieldSecretExcluded {
		t.Fatalf("private_key category = %q", got)
	}
	if got := report.Tables["nodes"].Fields["protocol"]; got != FieldProtocol {
		t.Fatalf("protocol category = %q", got)
	}
	if got := report.Tables["nodes"].Fields["uuid"]; got != FieldSecretExcluded {
		t.Fatalf("uuid category = %q", got)
	}

	encoded, err := Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	textOutput := FormatText(report)

	for _, secret := range []string{"fixture-password", "fixture-private-key", "fixture-uuid", "safe-name", "node-name", "node.example.invalid"} {
		if strings.Contains(output, secret) {
			t.Fatalf("JSON report contains source value %q: %s", secret, output)
		}
		if strings.Contains(textOutput, secret) {
			t.Fatalf("Text report contains source value %q: %s", secret, textOutput)
		}
	}

	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest := sha256.Sum256(after)
	if hex.EncodeToString(beforeDigest[:]) != hex.EncodeToString(afterDigest[:]) {
		t.Fatal("source database bytes changed")
	}
	afterInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("source database modification time changed")
	}
}

func TestOpenReadOnlyStrictWriteRejection(t *testing.T) {
	source := filepath.Join(t.TempDir(), "readonly_test.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE test_table (id INTEGER PRIMARY KEY, value TEXT); INSERT INTO test_table VALUES (1, 'initial');")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	roDB, err := OpenReadOnly(source)
	if err != nil {
		t.Fatalf("failed to open in read-only mode: %v", err)
	}
	defer roDB.Close()

	// 1. SELECT should succeed
	var val string
	if err := roDB.QueryRow("SELECT value FROM test_table WHERE id = 1;").Scan(&val); err != nil {
		t.Fatalf("read query failed on read-only connection: %v", err)
	}
	if val != "initial" {
		t.Fatalf("expected 'initial', got %q", val)
	}

	// 2. INSERT must fail
	if _, err := roDB.Exec("INSERT INTO test_table VALUES (2, 'blocked');"); err == nil {
		t.Fatal("expected INSERT to fail on read-only database, but it succeeded")
	} else if !strings.Contains(err.Error(), "readonly") && !strings.Contains(err.Error(), "query_only") {
		t.Logf("expected readonly error message, got: %v", err)
	}

	// 3. UPDATE must fail
	if _, err := roDB.Exec("UPDATE test_table SET value = 'mutated' WHERE id = 1;"); err == nil {
		t.Fatal("expected UPDATE to fail on read-only database, but it succeeded")
	}

	// 4. DELETE must fail
	if _, err := roDB.Exec("DELETE FROM test_table WHERE id = 1;"); err == nil {
		t.Fatal("expected DELETE to fail on read-only database, but it succeeded")
	}

	// 5. DDL CREATE TABLE must fail
	if _, err := roDB.Exec("CREATE TABLE hacker (id INT);"); err == nil {
		t.Fatal("expected CREATE TABLE to fail on read-only database, but it succeeded")
	}
}

func TestLegacyFull11TablesInspection(t *testing.T) {
	source := filepath.Join(t.TempDir(), "legacy_full.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}

	legacyDDL := `
		CREATE TABLE config_snapshots (
			id INTEGER NOT NULL PRIMARY KEY,
			label VARCHAR(200),
			description VARCHAR(500),
			snapshot_data TEXT NOT NULL,
			created_at DATETIME
		);
		CREATE INDEX ix_config_snapshots_id ON config_snapshots (id);

		CREATE TABLE dns_config (
			id INTEGER NOT NULL PRIMARY KEY,
			raw_yaml TEXT NOT NULL,
			enabled BOOLEAN NOT NULL
		);

		CREATE TABLE generate_config (
			id INTEGER NOT NULL PRIMARY KEY,
			enabled BOOLEAN NOT NULL,
			subscriptions BOOLEAN NOT NULL,
			node_groups BOOLEAN NOT NULL,
			rules BOOLEAN NOT NULL,
			dns BOOLEAN NOT NULL,
			exclude_node_proxies BOOLEAN NOT NULL
		);

		CREATE TABLE node_groups (
			id INTEGER NOT NULL PRIMARY KEY,
			name VARCHAR(120) NOT NULL UNIQUE,
			kind VARCHAR(20) NOT NULL,
			group_type VARCHAR(20) NOT NULL,
			sort_order INTEGER NOT NULL,
			regex_rules JSON NOT NULL,
			filter_min_speed_mbps FLOAT,
			filter_media_unlock JSON NOT NULL,
			include_nodes JSON NOT NULL,
			include_group_ids JSON NOT NULL,
			include_group_nodes_ids JSON NOT NULL,
			include_entries JSON NOT NULL,
			add_fallback BOOLEAN NOT NULL,
			exclude_nodes JSON NOT NULL,
			exclude_group_ids JSON NOT NULL,
			url_test_config JSON NOT NULL,
			load_balance_config JSON NOT NULL,
			fallback_config JSON NOT NULL
		);
		CREATE INDEX ix_node_groups_id ON node_groups (id);

		CREATE TABLE node_probe_results (
			id INTEGER NOT NULL PRIMARY KEY,
			node_key VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			server VARCHAR(255) NOT NULL,
			port INTEGER,
			type VARCHAR(64) NOT NULL,
			status VARCHAR(32) NOT NULL,
			latency_ms INTEGER,
			speed_mbps FLOAT,
			ip VARCHAR(128),
			country VARCHAR(32),
			asn INTEGER,
			organization VARCHAR(255),
			media JSON NOT NULL,
			error TEXT,
			checked_at INTEGER NOT NULL
		);
		CREATE INDEX ix_node_probe_results_checked_at ON node_probe_results (checked_at);
		CREATE INDEX ix_node_probe_results_name ON node_probe_results (name);
		CREATE UNIQUE INDEX ix_node_probe_results_node_key ON node_probe_results (node_key);
		CREATE INDEX ix_node_probe_results_server ON node_probe_results (server);

		CREATE TABLE probe_config (
			id INTEGER NOT NULL PRIMARY KEY,
			probe_enabled BOOLEAN NOT NULL,
			probe_interval_minutes INTEGER NOT NULL,
			speedtest_enabled BOOLEAN NOT NULL,
			speedtest_url VARCHAR(512) NOT NULL,
			speedtest_timeout_s INTEGER NOT NULL,
			speedtest_max_bytes INTEGER NOT NULL,
			speedtest_min_speed_mbps FLOAT NOT NULL,
			media_check_enabled BOOLEAN NOT NULL,
			media_platforms JSON NOT NULL,
			media_timeout_s INTEGER NOT NULL,
			probe_concurrency INTEGER NOT NULL,
			probe_timeout_ms INTEGER NOT NULL
		);

		CREATE TABLE proxy_chain_bindings (
			id INTEGER NOT NULL PRIMARY KEY,
			target_type VARCHAR(20) NOT NULL,
			target_id INTEGER,
			target_name VARCHAR(255),
			dialer_type VARCHAR(20) NOT NULL,
			dialer_ref VARCHAR(255) NOT NULL,
			enabled BOOLEAN NOT NULL,
			sort_order INTEGER NOT NULL,
			note TEXT
		);
		CREATE INDEX ix_proxy_chain_bindings_id ON proxy_chain_bindings (id);
		CREATE INDEX ix_proxy_chain_bindings_target_id ON proxy_chain_bindings (target_id);
		CREATE INDEX ix_proxy_chain_bindings_target_type ON proxy_chain_bindings (target_type);

		CREATE TABLE rule_categories (
			id INTEGER NOT NULL PRIMARY KEY,
			name VARCHAR(80) NOT NULL UNIQUE,
			sort_order INTEGER NOT NULL
		);
		CREATE INDEX ix_rule_categories_id ON rule_categories (id);

		CREATE TABLE rules (
			id INTEGER NOT NULL PRIMARY KEY,
			name VARCHAR(160) NOT NULL,
			category VARCHAR(80) NOT NULL,
			type VARCHAR(40) NOT NULL,
			value VARCHAR(512) NOT NULL,
			proxy VARCHAR(160) NOT NULL,
			options JSON NOT NULL,
			sort_order INTEGER NOT NULL,
			enabled BOOLEAN NOT NULL
		);

		CREATE TABLE security_settings (
			id INTEGER NOT NULL PRIMARY KEY,
			auth_enabled BOOLEAN NOT NULL,
			protect_frontend BOOLEAN NOT NULL,
			protect_api BOOLEAN NOT NULL,
			protect_exports BOOLEAN NOT NULL,
			token_hash VARCHAR(128) NOT NULL,
			fetch_proxy_enabled BOOLEAN NOT NULL,
			fetch_proxy_url VARCHAR(512) NOT NULL
		);

		CREATE TABLE subscriptions (
			id INTEGER NOT NULL PRIMARY KEY,
			name VARCHAR(120) NOT NULL UNIQUE,
			url TEXT NOT NULL,
			update_interval INTEGER,
			is_primary BOOLEAN NOT NULL,
			enabled BOOLEAN NOT NULL,
			node_prefix VARCHAR(120),
			filter_regex JSON NOT NULL,
			filter_min_speed_mbps FLOAT,
			filter_media_unlock JSON NOT NULL,
			include_node_names JSON NOT NULL,
			exclude_node_names JSON NOT NULL,
			node_renames JSON NOT NULL,
			proxy_chain JSON NOT NULL DEFAULT '[]',
			node_proxy_chains JSON NOT NULL DEFAULT '{}',
			source_nodes JSON NOT NULL,
			manual_nodes JSON NOT NULL,
			raw_nodes JSON NOT NULL,
			last_fetched_at DATETIME,
			last_fetch_error TEXT,
			fetch_failed_count INTEGER NOT NULL,
			fetch_comments JSON NOT NULL,
			subscription_userinfo TEXT,
			profile_update_interval VARCHAR(40),
			profile_web_page_url TEXT
		);
		CREATE INDEX ix_subscriptions_id ON subscriptions (id);

		-- Populate sample rows with confidential tokens and values
		INSERT INTO security_settings VALUES (1, 1, 1, 1, 1, 'super_confidential_token_hash_abc123', 0, '');
		INSERT INTO subscriptions (id, name, url, is_primary, enabled, filter_regex, filter_media_unlock, include_node_names, exclude_node_names, node_renames, source_nodes, manual_nodes, raw_nodes, fetch_failed_count, fetch_comments)
			VALUES (1, 'MainFeed', 'https://user:password123@sub.example.invalid/path?token=secrettoken999', 1, 1, '[]', '[]', '[]', '[]', '{}', '[]', '[]', '[]', 0, '[]');
		INSERT INTO dns_config VALUES (1, 'nameserver: [8.8.8.8]', 1);
		INSERT INTO rule_categories VALUES (1, 'AdBlock', 0);
	`

	if _, err := db.Exec(legacyDDL); err != nil {
		db.Close()
		t.Fatalf("failed to initialize legacy fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	beforeBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest := sha256.Sum256(beforeBytes)
	beforeModTime := time.Now()

	report, err := Inspect(source)
	if err != nil {
		t.Fatalf("inspect failed on full legacy db: %v", err)
	}

	if report.TotalTables != 11 {
		t.Fatalf("expected 11 legacy tables, got %d", report.TotalTables)
	}
	if report.TotalRows != 4 {
		t.Fatalf("expected 4 total rows, got %d", report.TotalRows)
	}
	if report.SchemaFingerprint == "" {
		t.Fatal("expected non-empty schema fingerprint")
	}

	// Verify token_hash is categorized as secret_excluded
	secTbl := report.Tables["security_settings"]
	if secTbl.Fields["token_hash"] != FieldSecretExcluded {
		t.Fatalf("expected token_hash to be secret_excluded, got %v", secTbl.Fields["token_hash"])
	}

	// Verify subscriptions raw_nodes is quarantine_blob
	subTbl := report.Tables["subscriptions"]
	if subTbl.Fields["raw_nodes"] != FieldQuarantineBlob {
		t.Fatalf("expected raw_nodes to be quarantine_blob, got %v", subTbl.Fields["raw_nodes"])
	}

	// Verify JSON output
	jsonBytes, err := Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	jsonStr := string(jsonBytes)
	textStr := FormatText(report)

	// Stringent verification: NO row values or secrets present in output
	leakedStrings := []string{
		"super_confidential_token_hash_abc123",
		"password123",
		"secrettoken999",
		"https://user:password123",
		"MainFeed",
		"nameserver: [8.8.8.8]",
		"AdBlock",
	}
	for _, leak := range leakedStrings {
		if strings.Contains(jsonStr, leak) {
			t.Fatalf("CRITICAL SECURITY VIOLATION: JSON output leaked data value: %q", leak)
		}
		if strings.Contains(textStr, leak) {
			t.Fatalf("CRITICAL SECURITY VIOLATION: Text output leaked data value: %q", leak)
		}
	}

	// Verify source database was completely untouched
	afterBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest := sha256.Sum256(afterBytes)
	if beforeDigest != afterDigest {
		t.Fatal("source database bytes were modified during inspect!")
	}
	_ = beforeModTime
}

func TestInspectRejectsMissingSource(t *testing.T) {
	if _, err := Inspect(filepath.Join(t.TempDir(), "missing.db")); err == nil {
		t.Fatal("expected missing source error")
	}
}

func TestInspectRejectsDirectory(t *testing.T) {
	if _, err := Inspect(t.TempDir()); err == nil {
		t.Fatal("expected error when inspecting directory, got nil")
	}
}

func TestInspectEmptyDatabase(t *testing.T) {
	source := filepath.Join(t.TempDir(), "empty.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	report, err := Inspect(source)
	if err != nil {
		t.Fatalf("inspect empty db failed: %v", err)
	}
	if report.TotalTables != 0 || report.TotalRows != 0 {
		t.Fatalf("expected empty report, got tables=%d rows=%d", report.TotalTables, report.TotalRows)
	}
	if report.SchemaFingerprint == "" {
		t.Fatal("expected valid schema fingerprint for empty database")
	}
}

func TestInspectContextCancellation(t *testing.T) {
	source := filepath.Join(t.TempDir(), "ctx_test.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec("CREATE TABLE t (id INT);")
	_ = db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = InspectContext(ctx, source)
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
}
