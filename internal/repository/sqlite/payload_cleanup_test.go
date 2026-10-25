package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestInspectPayloadStorage_DryRun(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	now := domain.NowUTC()
	subID := "sub_inspect_" + domain.MustNewUUIDv7()
	subRepo := sqlite.NewSubscriptionRepository(db)
	if err := subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "Inspect Sub",
		SourceURLSecretRef: "secret://inspect",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	fetchID := "fetch_inspect_" + domain.MustNewUUIDv7()
	if err := fetchRepo.Create(ctx, &domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:inspectdigest",
	}); err != nil {
		t.Fatalf("failed to create fetch: %v", err)
	}

	const insertSQL = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, 200, '{}', ?, ?);`

	p1 := "payload_ins_1_" + domain.MustNewUUIDv7()
	p2 := "payload_ins_2_" + domain.MustNewUUIDv7()
	p3 := "payload_ins_3_" + domain.MustNewUUIDv7()

	body1 := []byte("body_120_bytes_padding_01234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789") // 120 bytes
	body2 := append(body1, body1...)                                                                                                                         // 240 bytes
	body3 := []byte{}                                                                                                                                        // 0 bytes

	if _, err := db.Exec(insertSQL, p1, subID, fetchID, "sha256:d1", body1, 1, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert p1: %v", err)
	}
	if _, err := db.Exec(insertSQL, p2, subID, fetchID, "sha256:d2", body2, 0, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert p2: %v", err)
	}
	if _, err := db.Exec(insertSQL, p3, subID, fetchID, "sha256:d3", body3, 0, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert p3: %v", err)
	}

	// Add entry, node, publication, and ref
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	entryID := "entry_ins_" + domain.MustNewUUIDv7()
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{{
		ID:                    entryID,
		PayloadID:             p1,
		SubscriptionID:        subID,
		Ordinal:               0,
		SourceKey:             "k1",
		RawName:               "Node-1",
		Protocol:              domain.ProtocolVMess,
		EntryKind:             domain.EntryKindProxy,
		ClassificationReason:  "Normal",
		ClassificationVersion: "v2-proven-combo",
		ParsedConfigJSON:      `{"server":"1.1.1.1","port":443}`,
		CreatedAt:             now,
	}}); err != nil {
		t.Fatalf("failed to insert entry: %v", err)
	}

	nodeRepo := sqlite.NewNodeRepository(db)
	nodeID := "node_ins_" + domain.MustNewUUIDv7()
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolVMess,
		DisplayName: "Node-1",
		Server:      "1.1.1.1",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{UUID: "11111111-1111-1111-1111-111111111111"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}}); err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}

	pubRepo := sqlite.NewPublicationRepository(db)
	pubID := "pub_ins_" + domain.MustNewUUIDv7()
	if err := pubRepo.Create(ctx, &domain.Publication{
		ID:              pubID,
		Target:          domain.TargetMihomo,
		SnapshotDigest:  "sha256:pubsnap",
		CompilerVersion: "1.0.0",
		TokenHash:       "token_ins_" + domain.MustNewUUIDv7(),
		State:           domain.PublicationStateActive,
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("failed to create pub: %v", err)
	}

	pubRefRepo := sqlite.NewPublicationPayloadRefRepository(db)
	if err := pubRefRepo.AddRefs(ctx, pubID, []string{p1}); err != nil {
		t.Fatalf("failed to add ref: %v", err)
	}

	// Execute InspectPayloadStorage (dry-run)
	rep, err := sqlite.InspectPayloadStorage(ctx, db)
	if err != nil {
		t.Fatalf("InspectPayloadStorage failed: %v", err)
	}

	if rep.PayloadCount != 3 {
		t.Errorf("expected PayloadCount=3, got %d", rep.PayloadCount)
	}
	if rep.NonemptyBodyCount != 2 {
		t.Errorf("expected NonemptyBodyCount=2, got %d", rep.NonemptyBodyCount)
	}
	expectedBytes := int64(len(body1) + len(body2))
	if rep.BodyBytes != expectedBytes {
		t.Errorf("expected BodyBytes=%d, got %d", expectedBytes, rep.BodyBytes)
	}
	if rep.ClearedBodyCount != 0 {
		t.Errorf("expected ClearedBodyCount=0 for inspect, got %d", rep.ClearedBodyCount)
	}
	if rep.ClearedBodyBytes != 0 {
		t.Errorf("expected ClearedBodyBytes=0 for inspect, got %d", rep.ClearedBodyBytes)
	}
	if rep.RetainedEntries != 1 {
		t.Errorf("expected RetainedEntries=1, got %d", rep.RetainedEntries)
	}
	if rep.RetainedNodes != 1 {
		t.Errorf("expected RetainedNodes=1, got %d", rep.RetainedNodes)
	}
	if rep.RetainedSubscriptions != 1 {
		t.Errorf("expected RetainedSubscriptions=1, got %d", rep.RetainedSubscriptions)
	}
	if rep.RetainedPublications != 1 {
		t.Errorf("expected RetainedPublications=1, got %d", rep.RetainedPublications)
	}
	if rep.RetainedRefs != 1 {
		t.Errorf("expected RetainedRefs=1, got %d", rep.RetainedRefs)
	}

	// Verify dry-run made zero changes to database
	var dbNonempty int64
	if err := db.QueryRow("SELECT COUNT(*) FROM subscription_payloads WHERE body_blob IS NOT NULL AND LENGTH(body_blob) > 0;").Scan(&dbNonempty); err != nil {
		t.Fatalf("failed to query db nonempty count: %v", err)
	}
	if dbNonempty != 2 {
		t.Errorf("expected 2 nonempty bodies to remain in DB after inspect, got %d", dbNonempty)
	}
}

func TestClearPayloadBodies_SuccessAndPreservation(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	now := domain.NowUTC()
	subID := "sub_clear_" + domain.MustNewUUIDv7()
	subRepo := sqlite.NewSubscriptionRepository(db)
	if err := subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "Clear Sub",
		SourceURLSecretRef: "secret://clear",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	fetchID := "fetch_clear_" + domain.MustNewUUIDv7()
	if err := fetchRepo.Create(ctx, &domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:cleardigest",
	}); err != nil {
		t.Fatalf("failed to create fetch: %v", err)
	}

	const insertSQL = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, 200, '{}', ?, ?);`

	pPinned := "payload_pinned_" + domain.MustNewUUIDv7()
	pUnpinned := "payload_unpinned_" + domain.MustNewUUIDv7()
	pRef := "payload_ref_" + domain.MustNewUUIDv7()

	body1 := []byte("pinned_payload_raw_subscription_bytes_12345678901234567890")      // 59 bytes
	body2 := []byte("unpinned_payload_raw_bytes_abcdefghijklmnopqrstuvwxyz0123456789") // 64 bytes
	body3 := []byte("referenced_payload_raw_bytes_XYZ9876543210")                      // 43 bytes
	totalExpectedBytes := int64(len(body1) + len(body2) + len(body3))

	if _, err := db.Exec(insertSQL, pPinned, subID, fetchID, "sha256:pinned", body1, 1, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert pinned payload: %v", err)
	}
	if _, err := db.Exec(insertSQL, pUnpinned, subID, fetchID, "sha256:unpinned", body2, 0, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert unpinned payload: %v", err)
	}
	if _, err := db.Exec(insertSQL, pRef, subID, fetchID, "sha256:ref", body3, 0, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert referenced payload: %v", err)
	}

	// Add entries with parsed_config_json to prove it is preserved
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	entryID := "entry_clear_" + domain.MustNewUUIDv7()
	const expectedParsedConfig = `{"server":"203.0.113.10","port":443,"credentials":{"uuid":"22222222-2222-2222-2222-222222222222"}}`
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{{
		ID:                    entryID,
		PayloadID:             pPinned,
		SubscriptionID:        subID,
		Ordinal:               0,
		SourceKey:             "k-clear",
		RawName:               "HK-Clear-01",
		Protocol:              domain.ProtocolVMess,
		EntryKind:             domain.EntryKindProxy,
		ClassificationReason:  "Normal proxy",
		ClassificationVersion: "v2-proven-combo",
		ParsedConfigJSON:      expectedParsedConfig,
		CreatedAt:             now,
	}}); err != nil {
		t.Fatalf("failed to insert entry: %v", err)
	}

	// Add node and connection version
	nodeRepo := sqlite.NewNodeRepository(db)
	nodeID := "node_clear_" + domain.MustNewUUIDv7()
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolVMess,
		DisplayName: "HK-Clear-01",
		Server:      "203.0.113.10",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{UUID: "22222222-2222-2222-2222-222222222222"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}}); err != nil {
		t.Fatalf("failed to upsert node: %v", err)
	}

	verRepo := sqlite.NewNodeConnectionVersionRepository(db)
	const expectedEffectiveConfig = `{"server":"203.0.113.10","port":443}`
	if err := verRepo.Save(ctx, &domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID,
		ConnectionRevision:  2,
		EffectiveConfigJSON: expectedEffectiveConfig,
		ConfigFingerprint:   "sha256:fingerprint_clear_v2",
		SourceEntryID:       &entryID,
		SchemaVersion:       1,
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("failed to save connection version: %v", err)
	}

	pubRepo := sqlite.NewPublicationRepository(db)
	pubID := "pub_clear_" + domain.MustNewUUIDv7()
	if err := pubRepo.Create(ctx, &domain.Publication{
		ID:              pubID,
		Target:          domain.TargetMihomo,
		SnapshotDigest:  "sha256:pubclearsnap",
		CompilerVersion: "1.0.0",
		TokenHash:       "token_clear_" + domain.MustNewUUIDv7(),
		State:           domain.PublicationStateActive,
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("failed to create publication: %v", err)
	}

	pubRefRepo := sqlite.NewPublicationPayloadRefRepository(db)
	if err := pubRefRepo.AddRefs(ctx, pubID, []string{pRef}); err != nil {
		t.Fatalf("failed to add publication payload ref: %v", err)
	}

	// Execute ClearPayloadBodies
	rep, err := sqlite.ClearPayloadBodies(ctx, db)
	if err != nil {
		t.Fatalf("ClearPayloadBodies failed: %v", err)
	}

	if rep.PayloadCount != 3 {
		t.Errorf("expected PayloadCount=3, got %d", rep.PayloadCount)
	}
	if rep.NonemptyBodyCount != 0 {
		t.Errorf("expected NonemptyBodyCount=0, got %d", rep.NonemptyBodyCount)
	}
	if rep.BodyBytes != 0 {
		t.Errorf("expected BodyBytes=0, got %d", rep.BodyBytes)
	}
	if rep.ClearedBodyCount != 3 {
		t.Errorf("expected ClearedBodyCount=3, got %d", rep.ClearedBodyCount)
	}
	if rep.ClearedBodyBytes != totalExpectedBytes {
		t.Errorf("expected ClearedBodyBytes=%d, got %d", totalExpectedBytes, rep.ClearedBodyBytes)
	}
	if rep.RetainedEntries != 1 {
		t.Errorf("expected RetainedEntries=1, got %d", rep.RetainedEntries)
	}
	if rep.RetainedNodes != 1 {
		t.Errorf("expected RetainedNodes=1, got %d", rep.RetainedNodes)
	}
	if rep.RetainedSubscriptions != 1 {
		t.Errorf("expected RetainedSubscriptions=1, got %d", rep.RetainedSubscriptions)
	}
	if rep.RetainedPublications != 1 {
		t.Errorf("expected RetainedPublications=1, got %d", rep.RetainedPublications)
	}
	if rep.RetainedRefs != 1 {
		t.Errorf("expected RetainedRefs=1, got %d", rep.RetainedRefs)
	}

	// Verify DB state:
	// 1. All payload bodies in DB are 0 bytes
	var nonemptyCount int64
	if err := db.QueryRow("SELECT COUNT(*) FROM subscription_payloads WHERE body_blob IS NOT NULL AND LENGTH(body_blob) > 0;").Scan(&nonemptyCount); err != nil {
		t.Fatalf("failed to count nonempty: %v", err)
	}
	if nonemptyCount != 0 {
		t.Errorf("expected 0 nonempty bodies in DB, got %d", nonemptyCount)
	}

	// 2. All 3 payloads still exist (no rows deleted)
	var totalPayloads int64
	if err := db.QueryRow("SELECT COUNT(*) FROM subscription_payloads;").Scan(&totalPayloads); err != nil {
		t.Fatalf("failed to count payloads: %v", err)
	}
	if totalPayloads != 3 {
		t.Errorf("expected total payloads to remain 3, got %d", totalPayloads)
	}

	// 3. Pinned flag preserved
	var isPinned int
	if err := db.QueryRow("SELECT pinned FROM subscription_payloads WHERE id = ?;", pPinned).Scan(&isPinned); err != nil {
		t.Fatalf("failed to query pinned status: %v", err)
	}
	if isPinned != 1 {
		t.Errorf("expected pinned payload to retain pinned=1, got %d", isPinned)
	}

	// 4. Publication payload refs preserved
	refs, err := pubRefRepo.ListPayloadIDsByPublication(ctx, pubID)
	if err != nil || len(refs) != 1 || refs[0] != pRef {
		t.Errorf("expected publication payload ref to be preserved, got %v (err: %v)", refs, err)
	}

	// 5. Subscription entry facts (parsed_config_json, raw_name, etc.) completely intact
	e, err := entryRepo.GetByID(ctx, entryID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	if e.ParsedConfigJSON != expectedParsedConfig {
		t.Errorf("entry ParsedConfigJSON altered! expected %s, got %s", expectedParsedConfig, e.ParsedConfigJSON)
	}
	if e.RawName != "HK-Clear-01" {
		t.Errorf("entry RawName altered! got %s", e.RawName)
	}

	// 6. Node connection version facts preserved
	ver, err := verRepo.GetByRevision(ctx, nodeID, 2)
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if ver.EffectiveConfigJSON != expectedEffectiveConfig {
		t.Errorf("node connection version EffectiveConfigJSON altered! expected %s, got %s", expectedEffectiveConfig, ver.EffectiveConfigJSON)
	}

	// 7. Foreign key check returns 0 violations
	var fkCount int
	fkRows, err := db.Query("PRAGMA foreign_key_check;")
	if err != nil {
		t.Fatalf("foreign_key_check failed: %v", err)
	}
	for fkRows.Next() {
		fkCount++
	}
	fkRows.Close()
	if fkCount != 0 {
		t.Errorf("expected 0 foreign key violations, got %d", fkCount)
	}
}

func TestClearPayloadBodies_Idempotence(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	now := domain.NowUTC()
	subID := "sub_idem_" + domain.MustNewUUIDv7()
	subRepo := sqlite.NewSubscriptionRepository(db)
	_ = subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "Idem Sub",
		SourceURLSecretRef: "secret://idem",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	fetchID := "fetch_idem_" + domain.MustNewUUIDv7()
	_ = fetchRepo.Create(ctx, &domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:idemdigest",
	})

	pID := "payload_idem_" + domain.MustNewUUIDv7()
	const insertSQL = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, 200, '{}', 0, ?);`
	_, _ = db.Exec(insertSQL, pID, subID, fetchID, "sha256:idemdigest", []byte("initial body bytes"), now.Format(time.RFC3339))

	// First execution clears the body
	rep1, err := sqlite.ClearPayloadBodies(ctx, db)
	if err != nil {
		t.Fatalf("first ClearPayloadBodies failed: %v", err)
	}
	if rep1.ClearedBodyCount != 1 || rep1.ClearedBodyBytes != int64(len("initial body bytes")) {
		t.Fatalf("expected 1 cleared body in first run, got %+v", rep1)
	}

	// Second execution should clear 0 bodies (idempotent)
	rep2, err := sqlite.ClearPayloadBodies(ctx, db)
	if err != nil {
		t.Fatalf("second ClearPayloadBodies failed: %v", err)
	}
	if rep2.PayloadCount != 1 {
		t.Errorf("expected PayloadCount=1, got %d", rep2.PayloadCount)
	}
	if rep2.NonemptyBodyCount != 0 {
		t.Errorf("expected NonemptyBodyCount=0, got %d", rep2.NonemptyBodyCount)
	}
	if rep2.BodyBytes != 0 {
		t.Errorf("expected BodyBytes=0, got %d", rep2.BodyBytes)
	}
	if rep2.ClearedBodyCount != 0 {
		t.Errorf("expected ClearedBodyCount=0 on second run, got %d", rep2.ClearedBodyCount)
	}
	if rep2.ClearedBodyBytes != 0 {
		t.Errorf("expected ClearedBodyBytes=0 on second run, got %d", rep2.ClearedBodyBytes)
	}
}

func TestInspectAndClear_FailClosedMissingTable(t *testing.T) {
	ctx := context.Background()
	emptyDB, _ := setupEmptyTestDB(t)

	// In an empty database without tables applied, both Inspect and Clear must fail closed
	if _, err := sqlite.InspectPayloadStorage(ctx, emptyDB); err == nil {
		t.Fatal("expected InspectPayloadStorage to fail closed on empty db without schema tables")
	}

	if _, err := sqlite.ClearPayloadBodies(ctx, emptyDB); err == nil {
		t.Fatal("expected ClearPayloadBodies to fail closed on empty db without schema tables")
	}
}

func TestClearPayloadBodies_RollbackOnFKViolation(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	now := domain.NowUTC()
	subID := "sub_fk_" + domain.MustNewUUIDv7()
	subRepo := sqlite.NewSubscriptionRepository(db)
	_ = subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "FK Sub",
		SourceURLSecretRef: "secret://fk",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	fetchID := "fetch_fk_" + domain.MustNewUUIDv7()
	_ = fetchRepo.Create(ctx, &domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:fkdigest",
	})

	pID := "payload_fk_" + domain.MustNewUUIDv7()
	initialBody := []byte("payload body that must be rolled back on fk error")
	const insertSQL = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, 200, '{}', 0, ?);`
	_, _ = db.Exec(insertSQL, pID, subID, fetchID, "sha256:fkdigest", initialBody, now.Format(time.RFC3339))

	// Inject a foreign key violation by disabling foreign keys temporarily
	if _, err := db.Exec("PRAGMA foreign_keys = OFF;"); err != nil {
		t.Fatalf("failed to disable foreign keys: %v", err)
	}
	if _, err := db.Exec("INSERT INTO publication_payload_refs (publication_id, payload_id) VALUES ('nonexistent_pub', 'nonexistent_payload');"); err != nil {
		t.Fatalf("failed to insert invalid fk row: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("failed to re-enable foreign keys: %v", err)
	}

	// Attempt ClearPayloadBodies: should fail due to foreign key integrity check violation
	_, err := sqlite.ClearPayloadBodies(ctx, db)
	if err == nil {
		t.Fatal("expected ClearPayloadBodies to fail due to foreign key violation")
	}

	// Verify rollback: initial body must still exist in database
	var storedBlobLen int
	if err := db.QueryRow("SELECT LENGTH(body_blob) FROM subscription_payloads WHERE id = ?;", pID).Scan(&storedBlobLen); err != nil {
		t.Fatalf("failed to query stored body_blob: %v", err)
	}
	if storedBlobLen != len(initialBody) {
		t.Errorf("expected body to NOT be cleared due to rollback, got length %d (expected %d)", storedBlobLen, len(initialBody))
	}
}

func TestClearPayloadBodies_RollbackOnWithoutRowIDFKViolation(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	now := domain.NowUTC()
	subID := "sub_worowid_" + domain.MustNewUUIDv7()
	subRepo := sqlite.NewSubscriptionRepository(db)
	_ = subRepo.Create(ctx, &domain.Subscription{
		ID:                 subID,
		Name:               "WORowID Sub",
		SourceURLSecretRef: "secret://worowid",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	fetchID := "fetch_worowid_" + domain.MustNewUUIDv7()
	_ = fetchRepo.Create(ctx, &domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:worowiddigest",
	})

	pID := "payload_worowid_" + domain.MustNewUUIDv7()
	initialBody := []byte("body that must not clear on without rowid fk error")
	const insertSQL = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, 200, '{}', 0, ?);`
	_, _ = db.Exec(insertSQL, pID, subID, fetchID, "sha256:worowiddigest", initialBody, now.Format(time.RFC3339))

	// Create a WITHOUT ROWID table with a foreign key referencing subscriptions
	const createWORowIDTable = `
	CREATE TABLE test_without_rowid (
		item_key TEXT PRIMARY KEY,
		sub_id TEXT,
		FOREIGN KEY (sub_id) REFERENCES subscriptions(id)
	) WITHOUT ROWID;`
	if _, err := db.Exec(createWORowIDTable); err != nil {
		t.Fatalf("failed to create without rowid table: %v", err)
	}

	// Insert invalid foreign key while FK checks are off
	if _, err := db.Exec("PRAGMA foreign_keys = OFF;"); err != nil {
		t.Fatalf("failed to disable foreign keys: %v", err)
	}
	if _, err := db.Exec("INSERT INTO test_without_rowid (item_key, sub_id) VALUES ('k1', 'nonexistent_subscription_id');"); err != nil {
		t.Fatalf("failed to insert invalid fk into without rowid table: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("failed to re-enable foreign keys: %v", err)
	}

	// Attempt ClearPayloadBodies: PRAGMA foreign_key_check will return rowid NULL.
	// Must detect violation, fail-closed, and rollback.
	_, err := sqlite.ClearPayloadBodies(ctx, db)
	if err == nil {
		t.Fatal("expected ClearPayloadBodies to fail due to WITHOUT ROWID foreign key violation")
	}

	// Verify rollback: body must remain untouched
	var storedBlobLen int
	if err := db.QueryRow("SELECT LENGTH(body_blob) FROM subscription_payloads WHERE id = ?;", pID).Scan(&storedBlobLen); err != nil {
		t.Fatalf("failed to query stored body_blob: %v", err)
	}
	if storedBlobLen != len(initialBody) {
		t.Errorf("expected body to NOT be cleared due to rollback, got length %d (expected %d)", storedBlobLen, len(initialBody))
	}
}
