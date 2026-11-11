package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func retentionSetupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := setupTestDB(t)
	return db
}

func retentionCreateTestSubscription(t *testing.T, db *sql.DB, subID, name string) {
	t.Helper()
	now := domain.NowUTC().Format(time.RFC3339)
	const q = `
	INSERT INTO subscriptions (
		id, name, source_url_secret_ref, enabled, prefix,
		refresh_interval_seconds, refresh_user_agent_policy, refresh_fetch_proxy_ref,
		refresh_timeout_seconds, refresh_max_response_bytes,
		config_json, revision, is_primary, userinfo, created_at, updated_at
	) VALUES (?, ?, 'secret://sub', 1, '', 86400, '', '', 30, 10485760, '{}', 'rev-1', 0, '', ?, ?);`

	if _, err := db.Exec(q, subID, name, now, now); err != nil {
		t.Fatalf("failed to insert test subscription: %v", err)
	}
}

func retentionInsertTestFetch(t *testing.T, db *sql.DB, fetchID, subID string, startedAt time.Time, outcome domain.FetchOutcome) {
	t.Helper()
	if !outcome.IsValid() {
		t.Fatalf("invalid fetch outcome: %s", outcome)
	}
	nowStr := startedAt.Format(time.RFC3339)
	parsedCount := 1
	validCount := 1
	if outcome == domain.FetchOutcomeFailed {
		parsedCount = 0
		validCount = 0
	}
	const q = `
	INSERT INTO subscription_fetches (
		id, subscription_id, started_at, finished_at, outcome,
		content_digest, redacted_error, nodes_parsed, nodes_valid
	) VALUES (?, ?, ?, ?, ?, 'sha256:digest', '', ?, ?);`

	if _, err := db.Exec(q, fetchID, subID, nowStr, nowStr, string(outcome), parsedCount, validCount); err != nil {
		t.Fatalf("failed to insert test fetch: %v", err)
	}
}

func retentionInsertTestPayload(t *testing.T, db *sql.DB, payloadID, subID, fetchID string, createdAt time.Time, pinned bool) {
	t.Helper()
	nowStr := createdAt.Format(time.RFC3339)
	pinnedInt := 0
	if pinned {
		pinnedInt = 1
	}
	const q = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, 'sha256:digest', X'', 200, '{}', ?, ?);`

	if _, err := db.Exec(q, payloadID, subID, fetchID, pinnedInt, nowStr); err != nil {
		t.Fatalf("failed to insert test payload: %v", err)
	}
}

func retentionInsertTestEntry(t *testing.T, db *sql.DB, entryID, payloadID, subID, sourceKey, rawName string, kind domain.EntryKind, nodeID *string) {
	t.Helper()
	if !kind.IsValid() {
		t.Fatalf("invalid entry kind: %s", kind)
	}
	nowStr := domain.NowUTC().Format(time.RFC3339)

	var nextOrdinal int
	if err := db.QueryRow("SELECT COALESCE(MAX(ordinal) + 1, 0) FROM subscription_entries WHERE payload_id = ?;", payloadID).Scan(&nextOrdinal); err != nil {
		nextOrdinal = 0
	}

	const q = `
	INSERT INTO subscription_entries (
		id, payload_id, subscription_id, ordinal, source_key, raw_name,
		protocol, server, port, entry_kind, classification_reason,
		classification_version, source_provenance_json,
		parsed_config_json, parser_version, warnings_json, node_logical_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?, 'ss', '127.0.0.1', 8388, ?, 'test', 'v2-proven-combo', '{}', '{}', '1.0.0', '[]', ?, ?);`

	var nodeVal any = nil
	if nodeID != nil {
		nodeVal = *nodeID
	}
	if _, err := db.Exec(q, entryID, payloadID, subID, nextOrdinal, sourceKey, rawName, string(kind), nodeVal, nowStr); err != nil {
		t.Fatalf("failed to insert test entry: %v", err)
	}
}

func retentionGetLedgerOverride(t *testing.T, db *sql.DB, subID, anchor string) (domain.EntryKind, string, error) {
	t.Helper()
	var kind, reason string
	err := db.QueryRow("SELECT user_kind_override, override_reason FROM subscription_entry_overrides WHERE subscription_id = ? AND override_anchor = ?;", subID, anchor).Scan(&kind, &reason)
	if err != nil {
		return "", "", err
	}
	return domain.EntryKind(kind), reason, nil
}

// Legacy databases are never pruned implicitly; facts must survive the rejection.
func TestSubscriptionRetention_LegacyRequiresMaintenance(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	if err := sqlite.NewMigrationRunner(db, migrations.FS).RunUpTo(ctx, 29); err != nil {
		t.Fatal(err)
	}
	sub := "legacy-retention"
	retentionCreateTestSubscription(t, db, sub, "legacy")
	now := domain.NowUTC()
	for i := 0; i < 2; i++ {
		f, p := fmt.Sprintf("legacy-f%d", i), fmt.Sprintf("legacy-p%d", i)
		retentionInsertTestFetch(t, db, f, sub, now, domain.FetchOutcomeSuccess)
		retentionInsertTestPayload(t, db, p, sub, f, now, i == 0)
		retentionInsertTestEntry(t, db, fmt.Sprintf("legacy-e%d", i), p, sub, "key", "name", domain.EntryKindProxy, nil)
	}
	if _, err := db.Exec(`INSERT INTO publications(id,target,snapshot_digest,compiler_version,token_hash,state,created_at,content) VALUES('legacy-pub','mihomo','digest','1','legacy-token','active',?,X'01'); INSERT INTO publication_payload_refs VALUES('legacy-pub','legacy-p0')`, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	kind := domain.EntryKindNotice
	if err := sqlite.NewSubscriptionEntryRepository(db).SetUserOverride(ctx, "legacy-e0", &kind, "anchor", "reason", "admin", now); err != nil {
		t.Fatal(err)
	}
	_, err := sqlite.PruneSubscriptionRetention(ctx, db, sub, 20, nil)
	if err == nil || !strings.Contains(err.Error(), "maintenance_required") {
		t.Fatalf("expected maintenance_required, got %v", err)
	}
	for q, want := range map[string]int{
		`SELECT count(*) FROM subscription_payloads WHERE subscription_id='legacy-retention'`:                                                                                                  2,
		`SELECT count(*) FROM subscription_entries WHERE subscription_id='legacy-retention'`:                                                                                                   2,
		`SELECT count(*) FROM subscription_fetches WHERE subscription_id='legacy-retention'`:                                                                                                   2,
		`SELECT count(*) FROM subscription_payloads WHERE id='legacy-p0' AND pinned=1`:                                                                                                         1,
		`SELECT count(*) FROM publication_payload_refs WHERE publication_id='legacy-pub' AND payload_id='legacy-p0'`:                                                                           1,
		`SELECT count(*) FROM subscription_entry_overrides WHERE subscription_id='legacy-retention' AND override_anchor='anchor' AND user_kind_override='notice' AND override_reason='reason'`: 1,
	} {
		var got int
		if err := db.QueryRow(q).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d want %d: %v", q, got, want, err)
		}
	}
}

func retentionSealedDB(t *testing.T) *sql.DB {
	t.Helper()
	db := retentionSetupTestDB(t)
	if err := sqlite.CheckSubscriptionStorageReady(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSubscriptionRetention_OnePayloadIncludingPin(t *testing.T) {
	db := retentionSealedDB(t)
	sub := "single-sub"
	now := domain.NowUTC()
	retentionCreateTestSubscription(t, db, sub, "single")
	retentionInsertTestFetch(t, db, "single-f1", sub, now, domain.FetchOutcomeSuccess)
	retentionInsertTestFetch(t, db, "single-f2", sub, now, domain.FetchOutcomeSuccess)
	retentionInsertTestPayload(t, db, "single-p1", sub, "single-f1", now, true)
	_, err := db.Exec(`INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,http_status,headers_json,pinned,created_at) VALUES('single-p2',?,'single-f2','digest',X'',200,'{}',1,?)`, sub, now.Format(time.RFC3339))
	if err == nil {
		t.Fatal("second payload accepted despite pin")
	}
	if _, err := db.Exec(`UPDATE subscription_payloads SET body_blob=X'616263' WHERE id='single-p1'`); err == nil {
		t.Fatal("payload body accepted")
	}
	var count, bytes int
	if err := db.QueryRow(`SELECT count(*),sum(length(body_blob)) FROM subscription_payloads WHERE subscription_id=?`, sub).Scan(&count, &bytes); err != nil || count != 1 || bytes != 0 {
		t.Fatalf("payloads=%d body bytes=%d err=%v", count, bytes, err)
	}
}

func TestSubscriptionRetention_FetchUnionAndDiagnostics(t *testing.T) {
	ctx := context.Background()
	db := retentionSealedDB(t)
	sub := "fetch-union"
	now := domain.NowUTC()
	retentionCreateTestSubscription(t, db, sub, "union")
	for _, f := range []string{"applied", "raw", "attempt"} {
		retentionInsertTestFetch(t, db, f, sub, now, domain.FetchOutcomeSuccess)
	}
	retentionInsertTestPayload(t, db, "current", sub, "applied", now, false)
	retentionInsertTestEntry(t, db, "current-entry", "current", sub, "key", "name", domain.EntryKindProxy, nil)
	if _, err := db.Exec(`INSERT INTO subscription_original_configs(subscription_id,source_revision,content_digest,body_blob,fetch_id) VALUES(?,'rev-1','digest',X'61','raw'); INSERT INTO subscription_current_state(subscription_id,source_revision,applied_payload_id,applied_fetch_id,last_attempt_fetch_id,last_attempt_status) VALUES(?,'rev-1','current','applied','attempt','failed')`, sub, sub); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("failed-%02d", i)
		retentionInsertTestFetch(t, db, id, sub, now.Add(time.Duration(i)*time.Minute), domain.FetchOutcomeFailed)
		if _, err := db.Exec(`UPDATE subscription_fetches SET redacted_error='synthetic diagnostic' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE subscription_current_state SET last_attempt_fetch_id=?,last_attempt_status='failed' WHERE subscription_id=?`, id, sub); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlite.PruneSubscriptionRetention(ctx, db, sub, 20, nil); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var diagnostic string
	if err := db.QueryRow(`SELECT count(*) FROM subscription_fetches WHERE subscription_id=?`, sub).Scan(&count); err != nil || count != 3 {
		t.Fatalf("fetch count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT redacted_error FROM subscription_fetches WHERE id='failed-11'`).Scan(&diagnostic); err != nil || diagnostic != "synthetic diagnostic" {
		t.Fatalf("diagnostic=%q err=%v", diagnostic, err)
	}
	for _, id := range []string{"applied", "raw", "failed-11"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM subscription_fetches WHERE id=?`, id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("lost fetch %s: %v", id, err)
		}
	}
	var entries int
	if err := db.QueryRow(`SELECT count(*) FROM subscription_entries WHERE id='current-entry'`).Scan(&entries); err != nil || entries != 1 {
		t.Fatalf("lost current entry: %v", err)
	}
}

func TestSubscriptionRetention_ReplacementAtomicAndEvidence(t *testing.T) {
	ctx := context.Background()
	db := retentionSealedDB(t)
	sub := "atomic-sub"
	now := domain.NowUTC()
	retentionCreateTestSubscription(t, db, sub, "atomic")
	retentionInsertTestFetch(t, db, "atomic-f1", sub, now, domain.FetchOutcomeSuccess)
	retentionInsertTestFetch(t, db, "atomic-f2", sub, now, domain.FetchOutcomeSuccess)
	retentionInsertTestPayload(t, db, "atomic-p1", sub, "atomic-f1", now, true)
	retentionInsertTestEntry(t, db, "atomic-e1", "atomic-p1", sub, "key", "name", domain.EntryKindProxy, nil)
	replace := func(fail bool) error {
		return sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
			if err := sqlite.ReplaceCurrentPayloadTx(ctx, tx, sub); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO subscription_payloads(id,subscription_id,fetch_id,content_digest,body_blob,http_status,headers_json,pinned,created_at) VALUES('atomic-p2',?,?, 'new-digest',X'',200,'{}',0,?)`, sub, "atomic-f2", now.Format(time.RFC3339))
			if err != nil {
				return err
			}
			if fail {
				_, err = tx.ExecContext(ctx, `INSERT INTO subscription_entries(id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,server,port,entry_kind,created_at) VALUES('bad','missing',?,0,'k','n','ss','host',1,'proxy',?)`, sub, now.Format(time.RFC3339))
				return err
			}
			return nil
		})
	}
	if err := replace(true); err == nil {
		t.Fatal("invalid entry should roll back replacement")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM subscription_payloads p JOIN subscription_entries e ON e.payload_id=p.id WHERE p.id='atomic-p1' AND e.id='atomic-e1' AND p.pinned=1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("old generation not restored: %d %v", n, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM subscription_payload_evidence WHERE payload_id='atomic-p1'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rolled-back evidence persisted: %d %v", n, err)
	}
	if err := replace(false); err != nil {
		t.Fatal(err)
	}
	var digest string
	var pinned int
	if err := db.QueryRow(`SELECT content_digest,pinned FROM subscription_payload_evidence WHERE payload_id='atomic-p1'`).Scan(&digest, &pinned); err != nil || digest != "sha256:digest" || pinned != 1 {
		t.Fatalf("evidence digest=%s pin=%d err=%v", digest, pinned, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM subscription_payloads WHERE id='atomic-p1'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("old payload retained: %d %v", n, err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// 5. Behavior Fixture: Durable Override Ledger - Set, Clear, Absent Override, and Reappearance
func TestSubscriptionEntryOverride_DurableLedger_SetClearReappearance(t *testing.T) {
	ctx := context.Background()
	db := retentionSetupTestDB(t)

	subID := "sub-override-test"
	retentionCreateTestSubscription(t, db, subID, "Override Sub")

	now := domain.NowUTC()
	retentionInsertTestFetch(t, db, "fetch-ov-1", subID, now, domain.FetchOutcomeSuccess)
	retentionInsertTestPayload(t, db, "payload-ov-1", subID, "fetch-ov-1", now, false)
	retentionInsertTestEntry(t, db, "entry-ov-1", "payload-ov-1", subID, "source-key-alpha", "Node Alpha", domain.EntryKindProxy, nil)

	entryRepo := sqlite.NewSubscriptionEntryRepository(db)

	// Subtest A: Set override on entry
	noticeKind := domain.EntryKindNotice
	err := entryRepo.SetUserOverride(ctx, "entry-ov-1", &noticeKind, "anchor-alpha", "user marked notice", "admin", now)
	if err != nil {
		t.Fatalf("SetUserOverride failed: %v", err)
	}

	// Verify projection in subscription_entries
	entry, err := entryRepo.GetByID(ctx, "entry-ov-1")
	if err != nil || entry == nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if entry.UserKindOverride == nil || *entry.UserKindOverride != domain.EntryKindNotice {
		t.Fatalf("expected entry projection to be notice, got %v", entry.UserKindOverride)
	}

	// Verify durable ledger in subscription_entry_overrides via direct SQL helper
	ledgerKind, _, err := retentionGetLedgerOverride(t, db, subID, "anchor-alpha")
	if err != nil {
		t.Fatalf("retentionGetLedgerOverride failed: %v", err)
	}
	if ledgerKind != domain.EntryKindNotice {
		t.Fatalf("expected ledger entry kind notice, got %v", ledgerKind)
	}

	// Subtest B: Pruning old payload does NOT erase absent override
	// Delete payload-ov-1 and its entry
	_, err = db.ExecContext(ctx, "DELETE FROM subscription_entries WHERE id = 'entry-ov-1';")
	if err != nil {
		t.Fatalf("entry delete failed: %v", err)
	}
	_, err = db.ExecContext(ctx, "DELETE FROM subscription_payloads WHERE id = 'payload-ov-1';")
	if err != nil {
		t.Fatalf("payload delete failed: %v", err)
	}

	// Override in subscription_entry_overrides MUST STILL EXIST!
	survivingKind, _, err := retentionGetLedgerOverride(t, db, subID, "anchor-alpha")
	if err != nil {
		t.Fatalf("durable override was destroyed by entry deletion! %v", err)
	}
	if survivingKind != domain.EntryKindNotice {
		t.Fatalf("expected surviving override kind notice, got %v", survivingKind)
	}

	// Subtest C: Clear override removes durable intent and clears all same-anchor projections
	// Insert fresh entries with same anchor across payloads
	retentionInsertTestPayload(t, db, "payload-ov-2", subID, "fetch-ov-1", now, false)
	retentionInsertTestEntry(t, db, "entry-ov-2", "payload-ov-2", subID, "source-key-alpha", "Node Alpha", domain.EntryKindProxy, nil)
	retentionInsertTestEntry(t, db, "entry-ov-3", "payload-ov-2", subID, "source-key-alpha", "Node Alpha", domain.EntryKindProxy, nil)
	// Project the override
	_ = entryRepo.SetUserOverride(ctx, "entry-ov-2", &noticeKind, "anchor-alpha", "remark", "admin", now)

	// Now clear it
	err = entryRepo.SetUserOverride(ctx, "entry-ov-2", nil, "anchor-alpha", "", "", now)
	if err != nil {
		t.Fatalf("clear override failed: %v", err)
	}

	// Verify projection cleared on entry-ov-2
	clearedEntry, err := entryRepo.GetByID(ctx, "entry-ov-2")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if clearedEntry.UserKindOverride != nil {
		t.Fatalf("expected projection to be nil, got %v", clearedEntry.UserKindOverride)
	}

	// Verify projection also cleared on same-anchor entry-ov-3 to avoid resurrection
	clearedEntry3, err := entryRepo.GetByID(ctx, "entry-ov-3")
	if err != nil {
		t.Fatalf("GetByID failed on entry-ov-3: %v", err)
	}
	if clearedEntry3.UserKindOverride != nil {
		t.Fatalf("expected same-anchor projection to be nil, got %v", clearedEntry3.UserKindOverride)
	}

	// Verify ledger entry deleted
	_, _, err = retentionGetLedgerOverride(t, db, subID, "anchor-alpha")
	if err == nil {
		t.Fatalf("expected override to be deleted from ledger after clear, but it still exists!")
	}
}

// 7. Behavior Fixture: Migration Backfill Clear Ranking (Does not resurrect cleared overrides)
func TestSubscriptionRetention_MigrationBackfillClearRanking(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 27); err != nil {
		t.Fatalf("failed to apply migrations up to 27: %v", err)
	}

	subID := "sub-backfill-test"
	retentionCreateTestSubscription(t, db, subID, "Backfill Test Sub")

	t1 := "2026-01-01T10:00:00Z"
	t2 := "2026-01-01T11:00:00Z"

	// Case A: Older override 'notice', newer explicit CLEAR (user_kind_override NULL, override_anchor present)
	// Must NOT resurrect the cleared override!
	retentionInsertTestFetch(t, db, "fetch-bf-1", subID, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), domain.FetchOutcomeSuccess)
	retentionInsertTestPayload(t, db, "payload-bf-1", subID, "fetch-bf-1", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), false)

	// Old entry with active override
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscription_entries (
			id, payload_id, subscription_id, ordinal, source_key, raw_name,
			protocol, server, port, entry_kind, user_kind_override, override_anchor, override_at, created_at
		) VALUES ('entry-bf-1', 'payload-bf-1', ?, 0, 'key-alpha', 'Alpha', 'ss', '1.1.1.1', 8388, 'proxy', 'notice', 'anchor-alpha', ?, ?);
	`, subID, t1, t1)
	if err != nil {
		t.Fatalf("failed to insert entry-bf-1: %v", err)
	}

	// Newer entry with explicit clear (user_kind_override IS NULL, override_at / anchor recorded)
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_entries (
			id, payload_id, subscription_id, ordinal, source_key, raw_name,
			protocol, server, port, entry_kind, user_kind_override, override_anchor, override_at, created_at
		) VALUES ('entry-bf-2', 'payload-bf-1', ?, 1, 'key-alpha', 'Alpha', 'ss', '1.1.1.1', 8388, 'proxy', NULL, 'anchor-alpha', ?, ?);
	`, subID, t2, t2)
	if err != nil {
		t.Fatalf("failed to insert entry-bf-2: %v", err)
	}

	// Case B: Anchor beta has active override with no newer clear
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_entries (
			id, payload_id, subscription_id, ordinal, source_key, raw_name,
			protocol, server, port, entry_kind, user_kind_override, override_anchor, override_at, created_at
		) VALUES ('entry-bf-3', 'payload-bf-1', ?, 2, 'key-beta', 'Beta', 'ss', '1.1.1.2', 8388, 'proxy', 'notice', 'anchor-beta', ?, ?);
	`, subID, t1, t1)
	if err != nil {
		t.Fatalf("failed to insert entry-bf-3: %v", err)
	}

	// Case C: Ordinary history without override evidence must not create intent.
	retentionInsertTestEntry(t, db, "entry-bf-plain", "payload-bf-1", subID, "key-plain", "Plain", domain.EntryKindProxy, nil)

	// Case D: Equal timestamps must still honor the later rowid's explicit clear.
	// Case E: A clear with only override_at evidence must suppress the source-key fallback.
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_entries (
			id, payload_id, subscription_id, ordinal, source_key, raw_name,
			protocol, server, port, entry_kind, user_kind_override, override_anchor, override_at, created_at
		) VALUES
			('entry-bf-tie-old', 'payload-bf-1', ?, 4, 'key-tie', 'Tie', 'ss', '1.1.1.3', 8388, 'proxy', 'notice', 'anchor-tie', ?, ?),
			('entry-bf-tie-clear', 'payload-bf-1', ?, 5, 'key-tie', 'Tie', 'ss', '1.1.1.3', 8388, 'proxy', NULL, 'anchor-tie', ?, ?),
			('entry-bf-source-old', 'payload-bf-1', ?, 6, 'key-source', 'Source', 'ss', '1.1.1.4', 8388, 'proxy', 'notice', '', ?, ?),
			('entry-bf-source-clear', 'payload-bf-1', ?, 7, 'key-source', 'Source', 'ss', '1.1.1.4', 8388, 'proxy', NULL, '', ?, ?);
	`, subID, t1, t1, subID, t1, t1, subID, t1, t1, subID, t2, t2)
	if err != nil {
		t.Fatalf("failed to insert tie-break and timestamp-only clear evidence: %v", err)
	}

	// Run migration 28 via actual migration runner
	if err := runner.RunUpTo(ctx, 28); err != nil {
		t.Fatalf("failed to apply migration 28: %v", err)
	}

	// Verify anchor-alpha is NOT resurrected in subscription_entry_overrides
	var alphaCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_entry_overrides WHERE subscription_id = ? AND override_anchor = 'anchor-alpha';", subID).Scan(&alphaCount); err != nil {
		t.Fatalf("failed to query anchor-alpha count: %v", err)
	}
	if alphaCount != 0 {
		t.Fatalf("cleared override anchor-alpha was resurrected by backfill! count=%d", alphaCount)
	}

	// Verify anchor-beta IS backfilled
	var betaKind string
	err = db.QueryRowContext(ctx, "SELECT user_kind_override FROM subscription_entry_overrides WHERE subscription_id = ? AND override_anchor = 'anchor-beta';", subID).Scan(&betaKind)
	if err != nil || betaKind != "notice" {
		t.Fatalf("active override anchor-beta was not backfilled: %v, kind=%s", err, betaKind)
	}

	// Beta is the only surviving intent: no ordinary history, tied clear or
	// timestamp-only clear may introduce or resurrect another ledger record.
	var ledgerCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_entry_overrides WHERE subscription_id = ?", subID).Scan(&ledgerCount); err != nil || ledgerCount != 1 {
		t.Fatalf("expected only anchor-beta in backfilled ledger, count=%d err=%v", ledgerCount, err)
	}
}

func TestSubscriptionRetention_MigrationBackfillSequentialScanPlan(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 28); err != nil {
		t.Fatalf("failed to prepare migration 28 schema: %v", err)
	}

	// Explain the actual embedded INSERT, not a rewritten SELECT that could
	// drift from the migration. The ledger must exist for EXPLAIN to resolve it.
	migration, err := migrations.FS.ReadFile("000028_subscription_retention.sql")
	if err != nil {
		t.Fatal(err)
	}
	const insertStart = "INSERT OR IGNORE INTO subscription_entry_overrides ("
	_, backfill, found := strings.Cut(string(migration), insertStart)
	if !found {
		t.Fatal("migration 28 backfill INSERT not found")
	}
	backfill, _, found = strings.Cut(backfill, ";")
	if !found {
		t.Fatal("migration 28 backfill INSERT terminator not found")
	}

	// Keep the problematic historical index present: dropping it would hide
	// the regression rather than prove the migration chooses a table scan.
	var indexCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type='index' AND name='idx_sub_entries_source_key'`).Scan(&indexCount); err != nil || indexCount != 1 {
		t.Fatalf("source-key index missing: count=%d err=%v", indexCount, err)
	}
	rows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+insertStart+backfill)
	if err != nil {
		t.Fatalf("failed to explain migration 28 backfill: %v", err)
	}
	defer rows.Close()
	sequentialScan := false
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
		if strings.Contains(detail, "subscription_entries") {
			if !strings.Contains(detail, "SCAN subscription_entries") || strings.Contains(detail, "USING") {
				t.Fatalf("backfill must scan history sequentially, not via a non-covering index: %s", detail)
			}
			sequentialScan = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !sequentialScan {
		t.Fatalf("backfill table scan missing from plan: %v", plan)
	}
}
