package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func setupRetentionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := setupTestDB(t)
	return db
}

func createTestSubscription(t *testing.T, db *sql.DB, subID, name string) {
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

func insertTestFetch(t *testing.T, db *sql.DB, fetchID, subID string, startedAt time.Time, outcome domain.FetchOutcome) {
	t.Helper()
	if !outcome.IsValid() {
		t.Fatalf("invalid fetch outcome: %s", outcome)
	}
	nowStr := startedAt.Format(time.RFC3339)
	const q = `
	INSERT INTO subscription_fetches (
		id, subscription_id, started_at, finished_at, outcome,
		content_digest, redacted_error, nodes_parsed, nodes_valid
	) VALUES (?, ?, ?, ?, ?, 'sha256:digest', '', 1, 1);`

	if _, err := db.Exec(q, fetchID, subID, nowStr, nowStr, string(outcome)); err != nil {
		t.Fatalf("failed to insert test fetch: %v", err)
	}
}

func insertTestPayload(t *testing.T, db *sql.DB, payloadID, subID, fetchID string, createdAt time.Time, pinned bool) {
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

func insertTestEntry(t *testing.T, db *sql.DB, entryID, payloadID, subID, sourceKey, rawName string, kind domain.EntryKind, nodeID *string) {
	t.Helper()
	if !kind.IsValid() {
		t.Fatalf("invalid entry kind: %s", kind)
	}
	nowStr := domain.NowUTC().Format(time.RFC3339)
	const q = `
	INSERT INTO subscription_entries (
		id, payload_id, subscription_id, ordinal, source_key, raw_name,
		protocol, server, port, entry_kind, classification_reason,
		classification_version, source_provenance_json,
		parsed_config_json, parser_version, warnings_json, node_logical_id, created_at
	) VALUES (?, ?, ?, 0, ?, ?, 'ss', '127.0.0.1', 8388, ?, 'test', 'v2-proven-combo', '{}', '{}', '1.0.0', '[]', ?, ?);`

	var nodeVal any = nil
	if nodeID != nil {
		nodeVal = *nodeID
	}
	if _, err := db.Exec(q, entryID, payloadID, subID, sourceKey, rawName, string(kind), nodeVal, nowStr); err != nil {
		t.Fatalf("failed to insert test entry: %v", err)
	}
}

func getLedgerOverride(t *testing.T, db *sql.DB, subID, anchor string) (domain.EntryKind, string, error) {
	t.Helper()
	var kind, reason string
	err := db.QueryRow("SELECT user_kind_override, override_reason FROM subscription_entry_overrides WHERE subscription_id = ? AND override_anchor = ?;", subID, anchor).Scan(&kind, &reason)
	if err != nil {
		return "", "", err
	}
	return domain.EntryKind(kind), reason, nil
}

// 1. Behavior Fixture: Retention Repeat and Bounded Legacy Drain (Max 20 per refresh)
func TestSubscriptionRetention_RepeatAndBoundedDrain(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-retention-repeat"
	createTestSubscription(t, db, subID, "Repeat Sub")

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create 25 successful fetches and payloads
	for i := 1; i <= 25; i++ {
		tAt := baseTime.Add(time.Duration(i) * time.Hour)
		fID := fmt.Sprintf("fetch-%02d", i)
		pID := fmt.Sprintf("payload-%02d", i)
		insertTestFetch(t, db, fID, subID, tAt, domain.FetchOutcomeSuccess)
		insertTestPayload(t, db, pID, subID, fID, tAt, false)
		insertTestEntry(t, db, fmt.Sprintf("entry-%02d", i), pID, subID, fmt.Sprintf("key-%02d", i), "Node", domain.EntryKindProxy, nil)
	}

	// First retention run: max 20 deletions
	res1, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("first retention prune failed: %v", err)
	}
	if res1.DeletedPayloads != 20 {
		t.Fatalf("expected exactly 20 deleted payloads in batch, got %d", res1.DeletedPayloads)
	}
	if res1.ProtectedPayloads != 2 {
		t.Fatalf("expected 2 protected payloads (newest 2 success), got %d", res1.ProtectedPayloads)
	}

	// 5 payloads should remain in DB
	var countAfter1 int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id = ?;", subID).Scan(&countAfter1)
	if countAfter1 != 5 {
		t.Fatalf("expected 5 payloads remaining after first batch, got %d", countAfter1)
	}

	// Second retention run: drains remaining 3 unprotected payloads, preserving newest 2
	res2, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("second retention prune failed: %v", err)
	}
	if res2.DeletedPayloads != 3 {
		t.Fatalf("expected 3 deleted payloads in second batch, got %d", res2.DeletedPayloads)
	}
	if res2.ProtectedPayloads != 2 {
		t.Fatalf("expected 2 protected payloads, got %d", res2.ProtectedPayloads)
	}

	// Third retention run: idempotent, 0 deleted, 2 protected
	res3, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("third retention prune failed: %v", err)
	}
	if res3.DeletedPayloads != 0 {
		t.Fatalf("expected 0 deleted payloads on idempotent run, got %d", res3.DeletedPayloads)
	}
	if res3.ProtectedPayloads != 2 {
		t.Fatalf("expected 2 protected payloads, got %d", res3.ProtectedPayloads)
	}

	// Verify exact surviving payloads are payload-24 and payload-25 (newest two)
	var survivingIDs []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM subscription_payloads WHERE subscription_id = ? ORDER BY created_at ASC;", subID)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		survivingIDs = append(survivingIDs, id)
	}
	if len(survivingIDs) != 2 || survivingIDs[0] != "payload-24" || survivingIDs[1] != "payload-25" {
		t.Fatalf("expected payload-24 and payload-25 to survive, got %v", survivingIDs)
	}
}

// 2. Behavior Fixture: Failure and Zero-Node Refreshes Preserve Last Usable Inventory
func TestSubscriptionRetention_FailureAndZeroNodePreserved(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-failure-preserve"
	createTestSubscription(t, db, subID, "Failure Preserve Sub")

	t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Two usable successful payloads
	insertTestFetch(t, db, "fetch-succ-1", subID, t1, domain.FetchOutcomeSuccess)
	insertTestPayload(t, db, "payload-succ-1", subID, "fetch-succ-1", t1, false)
	insertTestEntry(t, db, "entry-succ-1", "payload-succ-1", subID, "key-1", "Node-1", domain.EntryKindProxy, nil)

	insertTestFetch(t, db, "fetch-succ-2", subID, t2, domain.FetchOutcomeSuccess)
	insertTestPayload(t, db, "payload-succ-2", subID, "fetch-succ-2", t2, false)
	insertTestEntry(t, db, "entry-succ-2", "payload-succ-2", subID, "key-2", "Node-2", domain.EntryKindProxy, nil)

	// One failed fetch with payload (e.g. HTTP error or zero-node)
	insertTestFetch(t, db, "fetch-fail-3", subID, t3, domain.FetchOutcomeFailed)
	insertTestPayload(t, db, "payload-fail-3", subID, "fetch-fail-3", t3, false)

	// Run retention:
	// - payload-fail-3 is protected because it is newest payload of ANY outcome
	// - payload-succ-2 and payload-succ-1 are protected because they are the newest TWO success payloads
	res, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("retention prune failed: %v", err)
	}
	if res.DeletedPayloads != 0 {
		t.Fatalf("expected 0 payloads deleted, got %d", res.DeletedPayloads)
	}
	if res.ProtectedPayloads != 3 {
		t.Fatalf("expected 3 protected payloads, got %d", res.ProtectedPayloads)
	}
}

// 3. Behavior Fixture: Comprehensive Protection Matrix
// Pins, Publication Refs, Connection-Version Source Entries, Node Source Fetches, Orphan Provenance
func TestSubscriptionRetention_ProtectionMatrix(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-matrix"
	createTestSubscription(t, db, subID, "Matrix Sub")

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create 10 payloads (P01..P10)
	for i := 1; i <= 10; i++ {
		tAt := baseTime.Add(time.Duration(i) * time.Hour)
		fID := fmt.Sprintf("fetch-%02d", i)
		pID := fmt.Sprintf("payload-%02d", i)
		insertTestFetch(t, db, fID, subID, tAt, domain.FetchOutcomeSuccess)
		insertTestPayload(t, db, pID, subID, fID, tAt, false)
		insertTestEntry(t, db, fmt.Sprintf("entry-%02d", i), pID, subID, fmt.Sprintf("key-%02d", i), "Node", domain.EntryKindProxy, nil)
	}

	// Protection A: Pin P01 explicitly
	payloadRepo := sqlite.NewSubscriptionPayloadRepository(db)
	if err := payloadRepo.Pin(ctx, "payload-01", true); err != nil {
		t.Fatalf("failed to pin payload-01: %v", err)
	}

	// Protection B: Publication ref on P02
	pubID := "pub-01"
	nowStr := domain.NowUTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		INSERT INTO publications (id, token_hash, state, content_bytes, created_at, updated_at)
		VALUES (?, 'hash-pub-01', 'active', X'deadbeef', ?, ?);
	`, pubID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert test publication: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO publication_payload_refs (publication_id, payload_id)
		VALUES (?, 'payload-02');
	`, pubID)
	if err != nil {
		t.Fatalf("failed to insert publication payload ref: %v", err)
	}

	// Protection C: Connection version referencing entry-03 on P03
	nodeID := "node-conn-01"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, 'ss', 'Conn Node', 'ref', 1, ?, ?);
	`, nodeID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_connection_versions (
			node_logical_id, connection_revision, effective_config_json, config_fingerprint, source_entry_id, schema_version, created_at
		) VALUES (?, 1, '{}', 'sha256:fp', 'entry-03', 1, ?);
	`, nodeID, nowStr)
	if err != nil {
		t.Fatalf("failed to insert connection version: %v", err)
	}

	// Protection D: Current node sources referencing fetch-04 (P04)
	nodeSourceNodeID := "node-source-01"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, 'ss', 'Source Node', 'ref', 1, ?, ?);
	`, nodeSourceNodeID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, 'fetch-04');
	`, nodeSourceNodeID, subID)
	if err != nil {
		t.Fatalf("failed to insert node source: %v", err)
	}

	// Protection E: Orphan node provenance protection on P05
	// Node has entry on P05, but no node_sources and no node_source_history
	orphanNodeID := "node-orphan-01"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, 'ss', 'Orphan Node', 'ref', 1, ?, ?);
	`, orphanNodeID, nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert orphan node: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE subscription_entries SET node_logical_id = ? WHERE id = 'entry-05';
	`, orphanNodeID)
	if err != nil {
		t.Fatalf("failed to associate entry-05 with orphan node: %v", err)
	}

	// Run retention:
	// P01: protected by Pin
	// P02: protected by Publication ref
	// P03: protected by Connection version source entry
	// P04: protected by Current node sources fetch
	// P05: protected by Orphan node provenance
	// P09, P10: protected by Newest 2 success payloads
	// Candidates to delete: P06, P07, P08 (3 payloads)
	res, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if res.DeletedPayloads != 3 {
		t.Fatalf("expected 3 deleted payloads (P06, P07, P08), got %d", res.DeletedPayloads)
	}
	if res.ProtectedPayloads != 7 {
		t.Fatalf("expected 7 protected payloads, got %d", res.ProtectedPayloads)
	}

	// Verify exact surviving payload IDs
	var surviving []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM subscription_payloads WHERE subscription_id = ? ORDER BY id ASC;", subID)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		surviving = append(surviving, id)
	}

	expectedSurviving := map[string]bool{
		"payload-01": true,
		"payload-02": true,
		"payload-03": true,
		"payload-04": true,
		"payload-05": true,
		"payload-09": true,
		"payload-10": true,
	}
	if len(surviving) != len(expectedSurviving) {
		t.Fatalf("expected %d surviving payloads, got %d: %v", len(expectedSurviving), len(surviving), surviving)
	}
	for _, id := range surviving {
		if !expectedSurviving[id] {
			t.Fatalf("unexpected surviving payload: %s", id)
		}
	}
}

// 4. Behavior Fixture: Deterministic RowID Tie-Breaking on Equal Timestamps
func TestSubscriptionRetention_DeterministicEqualTimestamps(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-tie-breaker"
	createTestSubscription(t, db, subID, "Tie Breaker Sub")

	sameTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Create 5 payloads with IDENTICAL created_at
	for i := 1; i <= 5; i++ {
		fID := fmt.Sprintf("fetch-%d", i)
		pID := fmt.Sprintf("payload-%d", i)
		insertTestFetch(t, db, fID, subID, sameTime, domain.FetchOutcomeSuccess)
		insertTestPayload(t, db, pID, subID, fID, sameTime, false)
		insertTestEntry(t, db, fmt.Sprintf("entry-%d", i), pID, subID, fmt.Sprintf("key-%d", i), "Node", domain.EntryKindProxy, nil)
	}

	// Run retention: newest 2 by rowid DESC must be kept (payload-4 and payload-5)
	res, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("retention prune failed: %v", err)
	}
	if res.DeletedPayloads != 3 {
		t.Fatalf("expected 3 deleted payloads, got %d", res.DeletedPayloads)
	}
	if res.ProtectedPayloads != 2 {
		t.Fatalf("expected 2 protected payloads, got %d", res.ProtectedPayloads)
	}

	// Verify payload-4 and payload-5 survived
	var surviving []string
	rows, err := db.QueryContext(ctx, "SELECT id FROM subscription_payloads WHERE subscription_id = ? ORDER BY id ASC;", subID)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		surviving = append(surviving, id)
	}
	if len(surviving) != 2 || surviving[0] != "payload-4" || surviving[1] != "payload-5" {
		t.Fatalf("expected payload-4 and payload-5 to survive deterministically, got %v", surviving)
	}
}

// 5. Behavior Fixture: Durable Override Ledger - Set, Clear, Absent Override, and Reappearance
func TestSubscriptionEntryOverride_DurableLedger_SetClearReappearance(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-override-test"
	createTestSubscription(t, db, subID, "Override Sub")

	now := domain.NowUTC()
	insertTestFetch(t, db, "fetch-ov-1", subID, now, domain.FetchOutcomeSuccess)
	insertTestPayload(t, db, "payload-ov-1", subID, "fetch-ov-1", now, false)
	insertTestEntry(t, db, "entry-ov-1", "payload-ov-1", subID, "source-key-alpha", "Node Alpha", domain.EntryKindProxy, nil)

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
	ledgerKind, _, err := getLedgerOverride(t, db, subID, "anchor-alpha")
	if err != nil {
		t.Fatalf("getLedgerOverride failed: %v", err)
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
	survivingKind, _, err := getLedgerOverride(t, db, subID, "anchor-alpha")
	if err != nil {
		t.Fatalf("durable override was destroyed by entry deletion! %v", err)
	}
	if survivingKind != domain.EntryKindNotice {
		t.Fatalf("expected surviving override kind notice, got %v", survivingKind)
	}

	// Subtest C: Clear override removes durable intent
	// Insert fresh entry with same anchor
	insertTestPayload(t, db, "payload-ov-2", subID, "fetch-ov-1", now, false)
	insertTestEntry(t, db, "entry-ov-2", "payload-ov-2", subID, "source-key-alpha", "Node Alpha", domain.EntryKindProxy, nil)
	// Project the override
	_ = entryRepo.SetUserOverride(ctx, "entry-ov-2", &noticeKind, "anchor-alpha", "remark", "admin", now)

	// Now clear it
	err = entryRepo.SetUserOverride(ctx, "entry-ov-2", nil, "anchor-alpha", "", "", now)
	if err != nil {
		t.Fatalf("clear override failed: %v", err)
	}

	// Verify projection cleared
	clearedEntry, err := entryRepo.GetByID(ctx, "entry-ov-2")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if clearedEntry.UserKindOverride != nil {
		t.Fatalf("expected projection to be nil, got %v", clearedEntry.UserKindOverride)
	}

	// Verify ledger entry deleted
	_, _, err = getLedgerOverride(t, db, subID, "anchor-alpha")
	if err == nil {
		t.Fatalf("expected override to be deleted from ledger after clear, but it still exists!")
	}
}

// 6. Behavior Fixture: Foreign Key Integrity and Rollback Safety
func TestSubscriptionRetention_RollbackAndFKIntegrity(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-fk-test"
	createTestSubscription(t, db, subID, "FK Sub")

	now := domain.NowUTC()
	for i := 1; i <= 5; i++ {
		fID := fmt.Sprintf("fetch-fk-%d", i)
		pID := fmt.Sprintf("payload-fk-%d", i)
		insertTestFetch(t, db, fID, subID, now.Add(time.Duration(i)*time.Minute), domain.FetchOutcomeSuccess)
		insertTestPayload(t, db, pID, subID, fID, now.Add(time.Duration(i)*time.Minute), false)
		insertTestEntry(t, db, fmt.Sprintf("entry-fk-%d", i), pID, subID, fmt.Sprintf("key-%d", i), "Node", domain.EntryKindProxy, nil)
	}

	// Run retention
	res, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("retention failed: %v", err)
	}
	if res.DeletedPayloads != 3 {
		t.Fatalf("expected 3 deleted, got %d", res.DeletedPayloads)
	}

	// Check foreign key integrity
	var fkViolations []string
	fkRows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		t.Fatalf("foreign_key_check failed: %v", err)
	}
	defer fkRows.Close()
	for fkRows.Next() {
		var table, parent string
		var rowid int64
		var fkid int
		_ = fkRows.Scan(&table, &rowid, &parent, &fkid)
		fkViolations = append(fkViolations, fmt.Sprintf("table=%s rowid=%d parent=%s", table, rowid, parent))
	}
	if len(fkViolations) > 0 {
		t.Fatalf("foreign key violations found after retention: %v", fkViolations)
	}

	// Verify no orphan entries remain
	var orphanEntries int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_entries WHERE payload_id NOT IN (SELECT id FROM subscription_payloads);").Scan(&orphanEntries)
	if orphanEntries != 0 {
		t.Fatalf("expected 0 orphan subscription_entries, found %d", orphanEntries)
	}
}

// 7. Behavior Fixture: Migration Backfill Clear Ranking (Does not resurrect cleared overrides)
func TestSubscriptionRetention_MigrationBackfillClearRanking(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-backfill-test"
	createTestSubscription(t, db, subID, "Backfill Test Sub")

	t1 := "2026-01-01T10:00:00Z"
	t2 := "2026-01-01T11:00:00Z"

	// Case A: Older override 'notice', newer explicit CLEAR (user_kind_override NULL, override_anchor present)
	// Must NOT resurrect the cleared override!
	insertTestFetch(t, db, "fetch-bf-1", subID, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), domain.FetchOutcomeSuccess)
	insertTestPayload(t, db, "payload-bf-1", subID, "fetch-bf-1", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), false)

	// Old entry with active override
	_, err = db.ExecContext(ctx, `
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

	// Clear table to test migration backfill query directly
	_, _ = db.ExecContext(ctx, "DELETE FROM subscription_entry_overrides;")

	// Execute migration backfill query
	backfillSQL := `
	INSERT OR IGNORE INTO subscription_entry_overrides (
		subscription_id, override_anchor, source_key, raw_name,
		user_kind_override, override_reason, override_at, actor_ref
	)
	SELECT
		subscription_id,
		effective_anchor,
		source_key,
		raw_name,
		user_kind_override,
		override_reason,
		COALESCE(NULLIF(override_at, ''), created_at) AS override_at,
		actor_ref
	FROM (
		SELECT
			subscription_id,
			COALESCE(NULLIF(override_anchor, ''), NULLIF(source_key, ''), protocol || '|' || server || '|' || port || '|' || raw_name) AS effective_anchor,
			source_key,
			raw_name,
			user_kind_override,
			override_reason,
			override_at,
			created_at,
			actor_ref,
			ROW_NUMBER() OVER (
				PARTITION BY subscription_id, COALESCE(NULLIF(override_anchor, ''), NULLIF(source_key, ''), protocol || '|' || server || '|' || port || '|' || raw_name)
				ORDER BY COALESCE(NULLIF(override_at, ''), created_at) DESC, created_at DESC, rowid DESC
			) AS rn
		FROM subscription_entries
		WHERE (user_kind_override IS NOT NULL AND user_kind_override != '')
		   OR (override_at IS NOT NULL AND override_at != '')
		   OR (override_anchor != '')
	) ranked
	WHERE ranked.rn = 1
	  AND ranked.effective_anchor != ''
	  AND ranked.user_kind_override IS NOT NULL
	  AND ranked.user_kind_override != '';`

	if _, err := db.ExecContext(ctx, backfillSQL); err != nil {
		t.Fatalf("backfill query execution failed: %v", err)
	}

	// Verify anchor-alpha is NOT resurrected in subscription_entry_overrides
	var alphaCount int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_entry_overrides WHERE subscription_id = ? AND override_anchor = 'anchor-alpha';", subID).Scan(&alphaCount)
	if alphaCount != 0 {
		t.Fatalf("cleared override anchor-alpha was resurrected by backfill! count=%d", alphaCount)
	}

	// Verify anchor-beta IS backfilled
	var betaKind string
	err = db.QueryRowContext(ctx, "SELECT user_kind_override FROM subscription_entry_overrides WHERE subscription_id = ? AND override_anchor = 'anchor-beta';", subID).Scan(&betaKind)
	if err != nil || betaKind != "notice" {
		t.Fatalf("active override anchor-beta was not backfilled: %v, kind=%s", err, betaKind)
	}
}

// 8. Behavior Fixture: Successive Zero-Node Refreshes Do Not Age Out Usable Data
func TestSubscriptionRetention_ZeroNodePayloadDoesNotAgeOutUsableData(t *testing.T) {
	ctx := context.Background()
	db := setupRetentionTestDB(t)

	subID := "sub-zero-node-age"
	createTestSubscription(t, db, subID, "Zero Node Sub")

	baseTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	// 1. Initial usable payload with 2 valid nodes
	insertTestFetch(t, db, "fetch-usable", subID, baseTime, domain.FetchOutcomeSuccess)
	insertTestPayload(t, db, "payload-usable", subID, "fetch-usable", baseTime, false)
	insertTestEntry(t, db, "entry-usable-1", "payload-usable", subID, "k1", "Node 1", domain.EntryKindProxy, nil)
	insertTestEntry(t, db, "entry-usable-2", "payload-usable", subID, "k2", "Node 2", domain.EntryKindProxy, nil)

	// 2. Three successive empty/zero-node fetches (nodes_valid = 0, no entries)
	for i := 1; i <= 3; i++ {
		tAt := baseTime.Add(time.Duration(i) * time.Hour)
		fID := fmt.Sprintf("fetch-empty-%d", i)
		pID := fmt.Sprintf("payload-empty-%d", i)
		// Insert fetch with nodes_valid = 0
		const insFetch = `
		INSERT INTO subscription_fetches (
			id, subscription_id, started_at, finished_at, outcome,
			content_digest, redacted_error, nodes_parsed, nodes_valid
		) VALUES (?, ?, ?, ?, 'success', 'sha256:empty', '', 0, 0);`
		tStr := tAt.Format(time.RFC3339)
		if _, err := db.ExecContext(ctx, insFetch, fID, subID, tStr, tStr); err != nil {
			t.Fatalf("insert empty fetch failed: %v", err)
		}
		insertTestPayload(t, db, pID, subID, fID, tAt, false)
	}

	// Run retention
	res, err := sqlite.PruneSubscriptionRetention(ctx, db, subID, 20, nil)
	if err != nil {
		t.Fatalf("retention failed: %v", err)
	}

	// The usable payload MUST survive because empty payloads do not count towards the 2 useful success payloads!
	var usableSurvives int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE id = 'payload-usable';").Scan(&usableSurvives)
	if usableSurvives != 1 {
		t.Fatalf("usable payload was aged out by successive empty payloads! deleted=%d, protected=%d", res.DeletedPayloads, res.ProtectedPayloads)
	}
}
