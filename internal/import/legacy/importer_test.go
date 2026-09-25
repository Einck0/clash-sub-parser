package legacy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"

	_ "modernc.org/sqlite"
)

// createLegacyFixtureDatabase populates an 11-table legacy SQLite database with secrets and blobs.
func createLegacyFixtureDatabase(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite fixture: %v", err)
	}
	defer db.Close()

	schema := `
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

		CREATE TABLE rule_categories (
			id INTEGER NOT NULL PRIMARY KEY,
			name VARCHAR(80) NOT NULL UNIQUE,
			sort_order INTEGER NOT NULL
		);

		CREATE TABLE dns_config (
			id INTEGER NOT NULL PRIMARY KEY,
			raw_yaml TEXT NOT NULL,
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

		CREATE TABLE nodes (
			id INTEGER NOT NULL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			protocol VARCHAR(64) NOT NULL,
			server VARCHAR(255) NOT NULL,
			port INTEGER NOT NULL,
			password VARCHAR(255),
			uuid VARCHAR(255),
			private_key VARCHAR(255)
		);

		CREATE TABLE config_snapshots (
			id INTEGER NOT NULL PRIMARY KEY,
			label VARCHAR(200),
			description VARCHAR(500),
			snapshot_data TEXT NOT NULL,
			created_at DATETIME
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
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to execute fixture schema: %v", err)
	}

	data := `
		INSERT INTO security_settings VALUES (1, 1, 1, 1, 1, 'super_secret_token_hash_value_xyz789', 0, '');
		INSERT INTO subscriptions (id, name, url, update_interval, is_primary, enabled, filter_regex, filter_media_unlock, include_node_names, exclude_node_names, node_renames, source_nodes, manual_nodes, raw_nodes, fetch_failed_count, fetch_comments)
		VALUES (1, 'MainFeed', 'https://user_admin:secret_password_123@sub.example.invalid/path?token=secret_query_token_999&param=normal', 3600, 1, 1, '[]', '[]', '[]', '[]', '{}', '[]', '[]', '["legacy_blob_data"]', 0, '[]');
		INSERT INTO subscriptions (id, name, url, update_interval, is_primary, enabled, filter_regex, filter_media_unlock, include_node_names, exclude_node_names, node_renames, source_nodes, manual_nodes, raw_nodes, fetch_failed_count, fetch_comments)
		VALUES (2, 'BackupFeed', 'https://backup.example.invalid/feed', 7200, 0, 1, '[]', '[]', '[]', '[]', '{}', '[]', '[]', '[]', 0, '[]');
		INSERT INTO node_groups (id, name, kind, group_type, sort_order, regex_rules, filter_media_unlock, include_nodes, include_group_ids, include_group_nodes_ids, include_entries, add_fallback, exclude_nodes, exclude_group_ids, url_test_config, load_balance_config, fallback_config)
		VALUES (1, 'AutoSelect', 'url-test', 'url-test', 1, '[]', '[]', '[]', '[]', '[]', '[]', 0, '[]', '[]', '{}', '{}', '{}');
		INSERT INTO node_groups (id, name, kind, group_type, sort_order, regex_rules, filter_media_unlock, include_nodes, include_group_ids, include_group_nodes_ids, include_entries, add_fallback, exclude_nodes, exclude_group_ids, url_test_config, load_balance_config, fallback_config)
		VALUES (2, 'ProxyGroup', 'select', 'select', 2, '[]', '[]', '[]', '[1]', '[]', '[]', 0, '[]', '[]', '{}', '{}', '{}');
		INSERT INTO rules (id, name, category, type, value, proxy, options, sort_order, enabled)
		VALUES (1, 'GoogleRule', 'Direct', 'DOMAIN-SUFFIX', 'google.com', 'AutoSelect', '{}', 1, 1);
		INSERT INTO rules (id, name, category, type, value, proxy, options, sort_order, enabled)
		VALUES (2, 'OrphanRule', 'Direct', 'DOMAIN-SUFFIX', 'orphan.example.com', 'NonExistentGroup', '{}', 2, 1);
		INSERT INTO nodes (id, name, protocol, server, port, password, uuid, private_key)
		VALUES (1, 'HK-Node-01', 'vless', 'hk.example.invalid', 443, 'secret_node_pass', 'secret_node_uuid_456', 'secret_node_private_key_789');
		INSERT INTO nodes (id, name, protocol, server, port, password, uuid, private_key)
		VALUES (2, 'Invalid-Proto-Node', 'unsupported_proto', 'bad.example.invalid', 80, '', '', '');
		INSERT INTO nodes (id, name, protocol, server, port, password, uuid, private_key)
		VALUES (3, 'HK-Node-01-Duplicate', 'vless', 'hk.example.invalid', 443, 'secret_node_pass_dup', 'secret_node_uuid_dup', 'secret_node_private_key_dup');
		INSERT INTO config_snapshots VALUES (1, 'v1', 'snapshot note', '{"legacy": "snapshot_payload"}', '2026-01-01 00:00:00');
	`

	if _, err := db.Exec(data); err != nil {
		t.Fatalf("failed to insert fixture data: %v", err)
	}
}

func initTargetDatabase(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	cfg := sqlite.Config{
		Path:         dbPath,
		ForeignKeys:  true,
		WALMode:      false,
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 1,
	}
	db, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open target sqlite: %v", err)
	}

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations on target: %v", err)
	}
	return db
}

func TestImporterDryRunSourceReadOnlyAndNoTargetWrites(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "legacy_source.db")
	targetPath := filepath.Join(tempDir, "target.db")

	createLegacyFixtureDatabase(t, sourcePath)
	targetDB := initTargetDatabase(t, targetPath)
	defer targetDB.Close()

	beforeBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest := sha256.Sum256(beforeBytes)
	beforeStat, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	opts := Options{
		SourcePath: sourcePath,
		TargetPath: targetPath,
		DryRun:     true,
	}

	importer := NewImporter()
	report, err := importer.Import(context.Background(), opts)
	if err != nil {
		t.Fatalf("dry-run import failed: %v", err)
	}

	if !report.DryRun {
		t.Fatal("expected report.DryRun to be true")
	}
	if report.TargetRevisionID == "" {
		t.Fatal("expected non-empty TargetRevisionID")
	}
	if report.TargetRevisionState != string(domain.RevisionStateDraft) {
		t.Fatalf("expected target revision state %q, got %q", domain.RevisionStateDraft, report.TargetRevisionState)
	}

	// Verify source was not modified
	afterBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest := sha256.Sum256(afterBytes)
	if hex.EncodeToString(beforeDigest[:]) != hex.EncodeToString(afterDigest[:]) {
		t.Fatal("source database bytes changed during dry-run")
	}
	afterStat, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !beforeStat.ModTime().Equal(afterStat.ModTime()) {
		t.Fatal("source database mod time changed during dry-run")
	}

	// Verify target database was NOT written to during dry-run
	var subCount, groupCount, revCount int
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM subscriptions").Scan(&subCount); err != nil {
		t.Fatal(err)
	}
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM node_groups").Scan(&groupCount); err != nil {
		t.Fatal(err)
	}
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM configuration_revisions").Scan(&revCount); err != nil {
		t.Fatal(err)
	}

	if subCount != 0 || groupCount != 0 || revCount != 0 {
		t.Fatalf("dry-run wrote records to target DB: subs=%d, groups=%d, revs=%d", subCount, groupCount, revCount)
	}

	// Verify reports contain no secrets
	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	jsonStr := string(reportJSON)
	textStr := report.FormatText()

	secrets := []string{
		"super_secret_token_hash_value_xyz789",
		"secret_password_123",
		"secret_query_token_999",
		"secret_node_pass",
		"secret_node_uuid_456",
		"secret_node_private_key_789",
		"secret_node_pass_dup",
		"secret_node_uuid_dup",
		"secret_node_private_key_dup",
	}
	for _, secret := range secrets {
		if strings.Contains(jsonStr, secret) {
			t.Fatalf("secret leaked in dry-run JSON report: %s", secret)
		}
		if strings.Contains(textStr, secret) {
			t.Fatalf("secret leaked in dry-run text report: %s", secret)
		}
	}
}

func TestImporterActualImportWritesDraftRevisionAndZeroSecretLeaks(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "legacy_source.db")
	targetPath := filepath.Join(tempDir, "target.db")

	createLegacyFixtureDatabase(t, sourcePath)
	targetDB := initTargetDatabase(t, targetPath)
	defer targetDB.Close()

	beforeBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest := sha256.Sum256(beforeBytes)

	opts := Options{
		SourcePath: sourcePath,
		TargetPath: targetPath,
		DryRun:     false,
	}

	importer := NewImporter()
	report, err := importer.Import(context.Background(), opts)
	if err != nil {
		t.Fatalf("actual import failed: %v", err)
	}

	if report.DryRun {
		t.Fatal("expected report.DryRun to be false")
	}
	if report.TargetRevisionID == "" {
		t.Fatal("expected TargetRevisionID to be set")
	}
	if report.TargetRevisionState != "draft" {
		t.Fatalf("expected TargetRevisionState draft, got %s", report.TargetRevisionState)
	}

	// 1. Source database was untouched
	afterBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest := sha256.Sum256(afterBytes)
	if hex.EncodeToString(beforeDigest[:]) != hex.EncodeToString(afterDigest[:]) {
		t.Fatal("source database was modified during actual import")
	}

	// 2. Check target database records
	// Subscriptions
	var subCount int
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM subscriptions").Scan(&subCount); err != nil {
		t.Fatal(err)
	}
	if subCount != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", subCount)
	}

	var mainFeedURL string
	if err := targetDB.QueryRow("SELECT source_url_secret_ref FROM subscriptions WHERE name = 'MainFeed'").Scan(&mainFeedURL); err != nil {
		t.Fatal(err)
	}
	// Verify credentials and sensitive query tokens were stripped from MainFeed URL
	if strings.Contains(mainFeedURL, "secret_password_123") || strings.Contains(mainFeedURL, "secret_query_token_999") || strings.Contains(mainFeedURL, "user_admin") {
		t.Fatalf("MainFeed URL contains secrets or credentials: %s", mainFeedURL)
	}
	if !strings.Contains(mainFeedURL, "https://sub.example.invalid/path") {
		t.Fatalf("MainFeed URL unexpected format: %s", mainFeedURL)
	}

	// Groups
	var groupCount int
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM node_groups").Scan(&groupCount); err != nil {
		t.Fatal(err)
	}
	if groupCount != 2 {
		t.Fatalf("expected 2 node groups, got %d", groupCount)
	}

	// Group edges (AutoSelect included in ProxyGroup)
	var edgeCount int
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM group_edges").Scan(&edgeCount); err != nil {
		t.Fatal(err)
	}
	if edgeCount != 1 {
		t.Fatalf("expected 1 group edge, got %d", edgeCount)
	}

	// Policy rules (GoogleRule mapped, OrphanRule quarantined because target group doesn't exist)
	var ruleCount int
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM policy_rules").Scan(&ruleCount); err != nil {
		t.Fatal(err)
	}
	if ruleCount != 1 {
		t.Fatalf("expected 1 mapped policy rule, got %d", ruleCount)
	}

	// Nodes (HK-Node-01 imported with opaque secret ref, duplicate skipped, Invalid-Proto-Node quarantined)
	if report.Counts.NodesImported != 1 {
		t.Fatalf("expected 1 imported node in report, got %d", report.Counts.NodesImported)
	}
	var nodeCount int
	if err := targetDB.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&nodeCount); err != nil {
		t.Fatal(err)
	}
	if nodeCount != 1 {
		t.Fatalf("expected 1 valid node, got %d", nodeCount)
	}

	// Configuration revision MUST be in draft state
	var revState, revDigest string
	if err := targetDB.QueryRow("SELECT state, content_digest FROM configuration_revisions WHERE id = ?", report.TargetRevisionID).Scan(&revState, &revDigest); err != nil {
		t.Fatalf("failed to query configuration revision: %v", err)
	}
	if revState != string(domain.RevisionStateDraft) {
		t.Fatalf("expected configuration revision state %q, got %q", domain.RevisionStateDraft, revState)
	}
	if revDigest == "" {
		t.Fatal("expected non-empty revision content digest")
	}

	// Audit event recorded
	var auditCount int
	var auditAction, auditResult string
	if err := targetDB.QueryRow("SELECT COUNT(*), action, result FROM audit_events WHERE action = 'legacy.import'").Scan(&auditCount, &auditAction, &auditResult); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected 1 audit event for legacy.import, got %d", auditCount)
	}
	if auditResult != string(domain.AuditResultSuccess) {
		t.Fatalf("expected audit result success, got %s", auditResult)
	}

	// Comprehensive secret leak audit across whole target DB!
	secrets := []string{
		"super_secret_token_hash_value_xyz789",
		"secret_password_123",
		"secret_query_token_999",
		"secret_node_pass",
		"secret_node_uuid_456",
		"secret_node_private_key_789",
		"secret_node_pass_dup",
		"secret_node_uuid_dup",
		"secret_node_private_key_dup",
	}

	// Check all string columns across all tables in target DB
	tables := []string{
		"subscriptions", "nodes", "node_groups", "group_edges",
		"policy_rules", "admission_rules", "configuration_revisions",
		"audit_events", "settings",
	}
	for _, table := range tables {
		rows, err := targetDB.Query("SELECT * FROM " + table)
		if err != nil {
			continue
		}
		cols, _ := rows.Columns()
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		for rows.Next() {
			if err := rows.Scan(valuePtrs...); err != nil {
				continue
			}
			for i, colVal := range values {
				if colVal == nil {
					continue
				}
				strVal := ""
				switch v := colVal.(type) {
				case []byte:
					strVal = string(v)
				case string:
					strVal = v
				}
				for _, secret := range secrets {
					if strings.Contains(strVal, secret) {
						t.Fatalf("CRITICAL SECURITY DEFECT: secret %q leaked into target table %q, column %q: %s", secret, table, cols[i], strVal)
					}
				}
			}
		}
		rows.Close()
	}

	// Verify quarantine report captured OrphanRule and Invalid-Proto-Node and blobs
	var foundOrphanRule, foundInvalidNode, foundSecurityExcluded bool
	for _, q := range report.Quarantine {
		if q.FieldOrEntity == "OrphanRule" || strings.Contains(q.Reason, "NonExistentGroup") {
			foundOrphanRule = true
		}
		if q.FieldOrEntity == "Invalid-Proto-Node" || strings.Contains(q.Reason, "unsupported_proto") {
			foundInvalidNode = true
		}
	}
	for _, s := range report.SecretExclusions {
		if s.Field == "url_credentials" || s.Field == "token_hash" || s.Field == "password" {
			foundSecurityExcluded = true
		}
	}
	if !foundOrphanRule {
		t.Error("expected OrphanRule to be quarantined")
	}
	if !foundInvalidNode {
		t.Error("expected Invalid-Proto-Node to be quarantined")
	}
	if !foundSecurityExcluded {
		t.Error("expected secrets to be noted in secret exclusions")
	}
}

func TestImporterTransactionRollbackOnTargetError(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "legacy_source.db")
	targetPath := filepath.Join(tempDir, "target_broken.db")

	createLegacyFixtureDatabase(t, sourcePath)
	targetDB := initTargetDatabase(t, targetPath)

	// Inject a schema conflict into targetDB that breaks subscriptions insert
	_, err := targetDB.Exec("DROP TABLE subscriptions;")
	if err != nil {
		t.Fatal(err)
	}
	targetDB.Close()

	opts := Options{
		SourcePath: sourcePath,
		TargetPath: targetPath,
		DryRun:     false,
	}

	importer := NewImporter()
	_, err = importer.Import(context.Background(), opts)
	if err == nil {
		t.Fatal("expected import to fail due to missing subscriptions table")
	}

	// Reopen target DB and verify no partial configuration revision was written
	checkDB, err := sql.Open("sqlite", targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer checkDB.Close()

	var revCount int
	if err := checkDB.QueryRow("SELECT COUNT(*) FROM configuration_revisions").Scan(&revCount); err != nil {
		t.Fatal(err)
	}
	if revCount != 0 {
		t.Fatalf("expected 0 revisions after rollback, got %d", revCount)
	}
}

func TestImporterValidationAndErrors(t *testing.T) {
	imp := NewImporter()

	// 1. Missing source path
	_, err := imp.Import(context.Background(), Options{SourcePath: ""})
	if err == nil || !strings.Contains(err.Error(), "source path is required") {
		t.Fatalf("expected missing_source error, got %v", err)
	}

	// 2. Missing target path when not dry-run
	_, err = imp.Import(context.Background(), Options{SourcePath: "some.db", TargetPath: "", DryRun: false})
	if err == nil || !strings.Contains(err.Error(), "target path is required") {
		t.Fatalf("expected missing_target error, got %v", err)
	}

	// 3. Source path does not exist
	_, err = imp.Import(context.Background(), Options{SourcePath: "nonexistent_source.db", DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "cannot access legacy source database") {
		t.Fatalf("expected cannot access error, got %v", err)
	}

	// 4. Source path is a directory
	tempDir := t.TempDir()
	_, err = imp.Import(context.Background(), Options{SourcePath: tempDir, DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "source path is a directory") {
		t.Fatalf("expected directory error, got %v", err)
	}
}

func TestImporterReportFormatting(t *testing.T) {
	report := &ImportReport{
		DryRun:              true,
		SourcePath:          "/path/to/legacy.db",
		SourceDigest:        "digest12345",
		SchemaFingerprint:   "fingerprint67890",
		TargetRevisionID:    "018f0000-0000-7000-8000-000000000001",
		TargetRevisionState: "draft",
		Counts: ImportCounts{
			SubscriptionsImported: 2,
			NodeGroupsImported:    3,
			GroupEdgesImported:    1,
			PolicyRulesImported:   5,
			NodesImported:         10,
			QuarantineCount:       4,
			SecretExclusionCount:  2,
		},
		Quarantine: []QuarantineItem{
			{
				Table:         "subscriptions",
				RecordIDHash:  "hash001",
				FieldOrEntity: "raw_nodes",
				Category:      QuarantineCategoryLegacyBlob,
				Reason:        "blob quarantined",
			},
		},
		SecretExclusions: []SecretExclusionItem{
			{
				Table:        "subscriptions",
				RecordIDHash: "hash002",
				Field:        "url_credentials",
				Action:       "excluded_and_sanitized",
			},
		},
		Timestamp: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC),
	}

	text := report.FormatText()
	if !strings.Contains(text, "DRY-RUN PREVIEW (NO WRITES)") {
		t.Errorf("expected dry-run text in formatted text, got: %s", text)
	}
	if !strings.Contains(text, "018f0000-0000-7000-8000-000000000001") {
		t.Errorf("expected revision id in text, got: %s", text)
	}
	if !strings.Contains(text, "draft") {
		t.Errorf("expected draft state in text, got: %s", text)
	}
	if !strings.Contains(text, "url_credentials") {
		t.Errorf("expected url_credentials in text, got: %s", text)
	}

	jsonBytes, err := report.MarshalJSON()
	if err != nil {
		t.Fatalf("failed to marshal report JSON: %v", err)
	}
	var unmarshaled ImportReport
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal report JSON: %v", err)
	}
	if unmarshaled.Counts.SubscriptionsImported != 2 || unmarshaled.TargetRevisionState != "draft" {
		t.Fatalf("unexpected unmarshaled report: %#v", unmarshaled)
	}
}

func TestImporterContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "legacy_source.db")
	createLegacyFixtureDatabase(t, sourcePath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	imp := NewImporter()
	_, err := imp.Import(ctx, Options{
		SourcePath: sourcePath,
		DryRun:     true,
	})
	if err == nil {
		t.Fatal("expected error due to cancelled context")
	}
}
