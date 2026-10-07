package inventory

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func setupInventoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	return db
}

func populateTestSubscriptions(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	nowStr := domain.NowUTC().Format(time.RFC3339)

	subs := []struct {
		id      string
		name    string
		url     string
		enabled int
	}{
		{"01a0b9af-c116-7204-b4a3-98a91733145d", "7li", "https://z.7li7li.com/api/v1/client/subscribe", 1},
		{"01a0b9af-c116-77cf-86e2-cba027607d17", "魔戒", "https://msub.xn--m7r52rosihxm.com/api/v1/client/subscribe", 1},
		{"01a0b9af-c116-7967-bc7f-197aa43a2e49", "Dogegg", "https://traffic.dogeggo.us.ci/sub/UiDmU_4Qup1", 1},
		{"01a0b9af-c116-763e-bf33-7a791621410a", "7li7li-便宜", "https://z.7li7li.com/api/v1/client/subscribe", 0},
		{"01a0b9af-c116-71da-822b-b8fe04752661", "WARP", "manual://nodes", 0},
		{"01a0b9af-c116-78d1-9ffd-fef9309e794f", "Eeox", "https://api.eeox.net/api/v1/client/subscribe", 0},
		{"01a0b9af-c116-7db2-8376-82f0cfbb21fd", "公开节点", "https://substore.einck.top/substore/api/file/", 0},
		{"01a0b9af-c116-7280-9629-f31353b85805", "einck-qzz", "https://234.qzz.io/fsllistyaml", 1},
		{"01a0b9af-c116-7789-a497-818f64a03bf2", "Githubusercontent", "https://raw.githubusercontent.com/Au1rxx/free", 0},
	}

	for _, s := range subs {
		_, err := db.ExecContext(ctx, `
			INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'rev1', ?, ?);
		`, s.id, s.name, s.url, s.enabled, nowStr, nowStr)
		if err != nil {
			t.Fatalf("insert subscription %s: %v", s.name, err)
		}
	}
}

func TestRedactTokenURL(t *testing.T) {
	raw := "https://example.com/api?token=secret123&foo=bar"
	redacted := RedactTokenURL(raw)
	if strings.Contains(redacted, "secret123") {
		t.Errorf("token leaked in redacted URL: %s", redacted)
	}
	if !strings.Contains(redacted, "token=%5BREDACTED%5D") && !strings.Contains(redacted, "token=[REDACTED]") {
		t.Errorf("expected masked token, got %s", redacted)
	}

	// Test path token masking
	rawPath := "https://traffic.dogeggo.us.ci/sub/UiDmU_4Qup1cQ3h62KTiB6L7FjOh9H8jpJ9ewcOLO1A/clash"
	redactedPath := RedactTokenURL(rawPath)
	if strings.Contains(redactedPath, "UiDmU_4Qup1cQ3h62KTiB6L7FjOh9H8jpJ9ewcOLO1A") {
		t.Errorf("path token leaked: %s", redactedPath)
	}
	if !strings.Contains(redactedPath, "REDACTED_PATH_TOKEN") {
		t.Errorf("expected REDACTED_PATH_TOKEN in masked path: %s", redactedPath)
	}
}

func TestRestoreSourceTokensFromArchive(t *testing.T) {
	db := setupInventoryTestDB(t)
	defer db.Close()

	populateTestSubscriptions(t, db)
	ctx := context.Background()
	archivePath := "/home/service/backups/csp-legacy-cold-archive-20260919.db"

	res, err := RestoreSourceTokensFromArchive(ctx, db, archivePath)
	if err != nil {
		t.Fatalf("RestoreSourceTokensFromArchive failed: %v", err)
	}

	if res.RestoredCount != 2 {
		t.Fatalf("expected 2 restored sources, got %d", res.RestoredCount)
	}

	// Verify 7li and 魔戒 have token
	var url7li, urlMj string
	_ = db.QueryRowContext(ctx, "SELECT source_url_secret_ref FROM subscriptions WHERE name = '7li';").Scan(&url7li)
	_ = db.QueryRowContext(ctx, "SELECT source_url_secret_ref FROM subscriptions WHERE name = '魔戒';").Scan(&urlMj)

	if !strings.Contains(url7li, "token=") {
		t.Errorf("7li URL missing token parameter")
	}
	if !strings.Contains(urlMj, "token=") {
		t.Errorf("魔戒 URL missing token parameter")
	}

	// Verify other 7 subscriptions are untouched
	var dogeggURL string
	var dogeggEnabled int
	_ = db.QueryRowContext(ctx, "SELECT source_url_secret_ref, enabled FROM subscriptions WHERE name = 'Dogegg';").Scan(&dogeggURL, &dogeggEnabled)
	if dogeggURL != "https://traffic.dogeggo.us.ci/sub/UiDmU_4Qup1" || dogeggEnabled != 1 {
		t.Errorf("Dogegg modified unexpectedly: url=%s, enabled=%d", dogeggURL, dogeggEnabled)
	}

	var warpEnabled int
	_ = db.QueryRowContext(ctx, "SELECT enabled FROM subscriptions WHERE name = 'WARP';").Scan(&warpEnabled)
	if warpEnabled != 0 {
		t.Errorf("WARP enabled status modified unexpectedly")
	}

	// Verify audit events
	var auditCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE action = 'subscription.source_url.token_restored';").Scan(&auditCount)
	if auditCount != 2 {
		t.Errorf("expected 2 audit events, got %d", auditCount)
	}

	// Test idempotency: second run does not bump revision or insert audit events
	var rev7liBefore string
	_ = db.QueryRowContext(ctx, "SELECT revision FROM subscriptions WHERE name = '7li';").Scan(&rev7liBefore)

	res2, err := RestoreSourceTokensFromArchive(ctx, db, archivePath)
	if err != nil {
		t.Fatalf("second restore failed: %v", err)
	}
	if res2.RestoredCount != 0 {
		t.Errorf("expected 0 restored on second run, got %d", res2.RestoredCount)
	}

	var rev7liAfter string
	_ = db.QueryRowContext(ctx, "SELECT revision FROM subscriptions WHERE name = '7li';").Scan(&rev7liAfter)
	if rev7liBefore != rev7liAfter {
		t.Errorf("revision bumped on idempotent re-run: %s != %s", rev7liBefore, rev7liAfter)
	}

	var auditCountAfter int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE action = 'subscription.source_url.token_restored';").Scan(&auditCountAfter)
	if auditCountAfter != 2 {
		t.Errorf("audit events duplicated on idempotent re-run: %d != 2", auditCountAfter)
	}
	rows, err := db.QueryContext(ctx, "SELECT redacted_summary FROM audit_events WHERE action = 'subscription.source_url.token_restored';")
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var summary string
		if err := rows.Scan(&summary); err == nil {
			if strings.Contains(summary, "token=") || strings.Contains(summary, "secret") {
				t.Errorf("token leaked in audit summary: %s", summary)
			}
		}
	}
}

func TestRunResetNodeInventory_WithTokenRestore(t *testing.T) {
	db := setupInventoryTestDB(t)
	defer db.Close()

	populateTestSubscriptions(t, db)
	ctx := context.Background()

	// Insert sample node
	nowStr := domain.NowUTC().Format(time.RFC3339)
	nodeID := domain.MustNewUUIDv7()
	_, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, connection_revision, created_at, updated_at)
		VALUES (?, 'vmess', 'TestNode', '1.2.3.4', 443, '{}', 1, 1, ?, ?);
	`, nodeID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}

	opts := ResetOptions{
		DryRun:              false,
		RestoreSourceTokens: true,
		ArchiveDBPath:       "/home/service/backups/csp-legacy-cold-archive-20260919.db",
	}

	report, err := RunResetNodeInventory(ctx, db, opts)
	if err != nil {
		t.Fatalf("RunResetNodeInventory: %v", err)
	}

	if report.PostCounts["nodes"] != 0 {
		t.Errorf("expected 0 nodes after reset, got %d", report.PostCounts["nodes"])
	}
	if !report.SourceURLsRestored["7li"] || !report.SourceURLsRestored["魔戒"] {
		t.Errorf("expected 7li and 魔戒 in SourceURLsRestored")
	}

	// Verify tokens restored
	var url7li string
	_ = db.QueryRowContext(ctx, "SELECT source_url_secret_ref FROM subscriptions WHERE name = '7li';").Scan(&url7li)
	if !strings.Contains(url7li, "token=") {
		t.Errorf("7li URL missing token")
	}
}
