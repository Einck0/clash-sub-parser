package sqlite_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestDataflowRepositories(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)

	// Repositories
	subRepo := sqlite.NewSubscriptionRepository(db)
	payloadRepo := sqlite.NewSubscriptionPayloadRepository(db)
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	verRepo := sqlite.NewNodeConnectionVersionRepository(db)
	overrideRepo := sqlite.NewNodeOverrideRepository(db)
	pubRefRepo := sqlite.NewPublicationPayloadRefRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)

	now := domain.NowUTC()

	// 1. Create a Subscription
	subID := "sub_test_" + domain.MustNewUUIDv7()
	sub := domain.Subscription{
		ID:                 subID,
		Name:               "Test Sub",
		SourceURLSecretRef: "secret://sub-key",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := subRepo.Create(ctx, &sub); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}

	// Create a SubscriptionFetch
	fetchID := "fetch_" + domain.MustNewUUIDv7()
	fetch := domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:digest12345",
	}
	if err := fetchRepo.Create(ctx, &fetch); err != nil {
		t.Fatalf("failed to create fetch: %v", err)
	}

	// 2. Test Payload Save and Query
	payloadID := "payload_" + domain.MustNewUUIDv7()
	p := domain.SubscriptionPayload{
		ID:             payloadID,
		SubscriptionID: subID,
		FetchID:        fetchID,
		ContentDigest:  "sha256:digest12345",
		BodyBlob:       []byte("raw subscription body bytes"),
		HTTPStatus:     200,
		HeadersJSON:    `{"content-type":"text/yaml"}`,
		Pinned:         false,
		CreatedAt:      now,
	}
	if err := payloadRepo.Save(ctx, &p); err != nil {
		t.Fatalf("failed to save payload: %v", err)
	}

	gotP, err := payloadRepo.GetByID(ctx, payloadID)
	if err != nil {
		t.Fatalf("failed to get payload by ID: %v", err)
	}
	if gotP.ContentDigest != "sha256:digest12345" || len(gotP.BodyBlob) != 0 {
		t.Fatalf("payload content mismatch: %+v (expected empty BodyBlob per FR-RAW-02, got len=%d)", gotP, len(gotP.BodyBlob))
	}

	// Direct DB check shows body_blob was saved as empty blob X''
	var storedBlobLen int
	if err := db.QueryRow("SELECT LENGTH(body_blob) FROM subscription_payloads WHERE id = ?;", payloadID).Scan(&storedBlobLen); err != nil {
		t.Fatalf("failed to query stored body_blob length: %v", err)
	}
	if storedBlobLen != 0 {
		t.Fatalf("expected stored body_blob length 0, got %d", storedBlobLen)
	}

	// Historical non-empty body injection test (FR-RAW-02):
	// Even if database has historical non-empty body bytes, GetByID, GetLatestBySubscription,
	// and GetByDigest must return empty body and never load historical bytes into memory.
	histFetchID := "fetch_hist_" + domain.MustNewUUIDv7()
	histFetch := domain.SubscriptionFetch{
		ID:             histFetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:histdigest",
	}
	if err := fetchRepo.Create(ctx, &histFetch); err != nil {
		t.Fatalf("failed to create hist fetch: %v", err)
	}

	histPayloadID := "payload_hist_" + domain.MustNewUUIDv7()
	const insertHistSQL = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, 200, '{}', 0, ?);`
	if _, err := db.Exec(insertHistSQL, histPayloadID, subID, histFetchID, "sha256:histdigest", []byte("historical raw subscription body"), now.Add(time.Second).Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to inject historical payload: %v", err)
	}

	// In SQLite table directly, length is > 0
	var histRawLen int
	if err := db.QueryRow("SELECT LENGTH(body_blob) FROM subscription_payloads WHERE id = ?;", histPayloadID).Scan(&histRawLen); err != nil {
		t.Fatalf("failed to query hist raw len: %v", err)
	}
	if histRawLen == 0 {
		t.Fatalf("expected injected historical body to have non-zero length in DB")
	}

	// GetByID returns empty BodyBlob
	gotHistByID, err := payloadRepo.GetByID(ctx, histPayloadID)
	if err != nil {
		t.Fatalf("GetByID failed on historical payload: %v", err)
	}
	if len(gotHistByID.BodyBlob) != 0 {
		t.Fatalf("expected GetByID to return empty BodyBlob, got len=%d", len(gotHistByID.BodyBlob))
	}

	// GetLatestBySubscription returns empty BodyBlob
	gotHistLatest, err := payloadRepo.GetLatestBySubscription(ctx, subID)
	if err != nil {
		t.Fatalf("GetLatestBySubscription failed: %v", err)
	}
	if gotHistLatest.ID != histPayloadID || len(gotHistLatest.BodyBlob) != 0 {
		t.Fatalf("expected latest payload to be histPayloadID with empty BodyBlob, got id=%s len=%d", gotHistLatest.ID, len(gotHistLatest.BodyBlob))
	}

	// GetByDigest returns empty BodyBlob
	gotHistDigest, err := payloadRepo.GetByDigest(ctx, "sha256:histdigest")
	if err != nil {
		t.Fatalf("GetByDigest failed: %v", err)
	}
	if gotHistDigest.ID != histPayloadID || len(gotHistDigest.BodyBlob) != 0 {
		t.Fatalf("expected GetByDigest to return empty BodyBlob, got len=%d", len(gotHistDigest.BodyBlob))
	}

	gotLatest, err := payloadRepo.GetLatestBySubscription(ctx, subID)
	if err != nil || gotLatest.ID != payloadID {
		t.Fatalf("failed to get latest payload: %v", err)
	}

	gotByDigest, err := payloadRepo.GetByDigest(ctx, "sha256:digest12345")
	if err != nil || gotByDigest.ID != payloadID {
		t.Fatalf("failed to get payload by digest: %v", err)
	}

	// Pin payload
	if err := payloadRepo.Pin(ctx, payloadID, true); err != nil {
		t.Fatalf("failed to pin payload: %v", err)
	}
	gotP, _ = payloadRepo.GetByID(ctx, payloadID)
	if !gotP.Pinned {
		t.Fatalf("expected payload to be pinned")
	}

	// 3. Test Subscription Entries
	entryID := "entry_" + domain.MustNewUUIDv7()
	entry := domain.SubscriptionEntry{
		ID:                    entryID,
		PayloadID:             payloadID,
		SubscriptionID:        subID,
		Ordinal:               0,
		SourceKey:             "raw-entry-0",
		RawName:               "HK-Proxy-01",
		Protocol:              domain.ProtocolVMess,
		Server:                "198.51.100.1",
		Port:                  443,
		EntryKind:             domain.EntryKindProxy,
		ClassificationReason:  "Normal proxy",
		ClassificationVersion: "v2-proven-combo",
		SourceProvenanceJSON:  `{"index":0}`,
		ParsedConfigJSON:      `{"server":"198.51.100.1","port":443}`,
		ParserVersion:         "1.0.0",
		WarningsJSON:          "[]",
		CreatedAt:             now,
	}
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{entry}); err != nil {
		t.Fatalf("failed to save entry batch: %v", err)
	}

	entries, err := entryRepo.ListByPayload(ctx, payloadID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 entry for payload, got %d (err: %v)", len(entries), err)
	}

	// Set user override on entry
	overrideKind := domain.EntryKindNotice
	if err := entryRepo.SetUserOverride(ctx, entryID, &overrideKind, "anchor-1", "user flagged", "admin", now); err != nil {
		t.Fatalf("failed to set user override: %v", err)
	}
	updatedEntry, err := entryRepo.GetByID(ctx, entryID)
	if err != nil {
		t.Fatalf("failed to get entry by ID: %v", err)
	}
	if updatedEntry.UserKindOverride == nil || *updatedEntry.UserKindOverride != domain.EntryKindNotice {
		t.Fatalf("expected user override to be notice")
	}

	// 4. Test Node Connection Versions and Heads
	nodeID := "node_" + domain.MustNewUUIDv7()
	testNode := domain.Node{
		LogicalID:   nodeID,
		Protocol:    domain.ProtocolVMess,
		DisplayName: "Test Node",
		Server:      "198.51.100.1",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{UUID: "11111111-1111-1111-1111-111111111111"},
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{testNode}); err != nil {
		t.Fatalf("failed to upsert node: %v", err)
	}

	// Check that node_connection_versions and node_connection_heads were created
	headVer, err := verRepo.GetHead(ctx, nodeID)
	if err != nil {
		t.Fatalf("failed to get node head: %v", err)
	}
	if headVer.ConnectionRevision != 1 {
		t.Fatalf("expected head revision 1, got %d", headVer.ConnectionRevision)
	}

	// Add revision 2
	ver2 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID,
		ConnectionRevision:  2,
		EffectiveConfigJSON: `{"server":"198.51.100.2","port":8443,"credentials":{"uuid":"11111111-1111-1111-1111-111111111111"}}`,
		ConfigFingerprint:   "sha256:fingerprint_v2",
		SchemaVersion:       1,
		CreatedAt:           now,
	}
	if err := verRepo.Save(ctx, &ver2); err != nil {
		t.Fatalf("failed to save version 2: %v", err)
	}
	if err := verRepo.SetHead(ctx, nodeID, 2, now); err != nil {
		t.Fatalf("failed to set head to revision 2: %v", err)
	}

	// Verify NodeRepository.GetByLogicalID now reads revision 2 server/port from head
	loadedNode, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetByLogicalID failed: %v", err)
	}
	if loadedNode.ConnectionRevision != 2 || loadedNode.Server != "198.51.100.2" || loadedNode.Port != 8443 {
		t.Fatalf("expected node to reflect head rev 2 (198.51.100.2:8443), got %+v", loadedNode)
	}

	// 5. Test Node Overrides
	override := domain.NodeOverride{
		NodeLogicalID:     nodeID,
		FieldPath:         "transport.sni",
		OverrideValueJSON: `"custom.sni.example.com"`,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := overrideRepo.Save(ctx, &override); err != nil {
		t.Fatalf("failed to save node override: %v", err)
	}
	overrides, err := overrideRepo.GetByNode(ctx, nodeID)
	if err != nil || len(overrides) != 1 {
		t.Fatalf("expected 1 override, got %d (err: %v)", len(overrides), err)
	}

	// 6. Test Publication Payload Refs & Deletion Protection
	pubID := "pub_" + domain.MustNewUUIDv7()
	pub := domain.Publication{
		ID:              pubID,
		Target:          domain.TargetMihomo,
		SnapshotDigest:  "sha256:snap123",
		CompilerVersion: "1.0.0",
		TokenHash:       "token_hash_" + domain.MustNewUUIDv7(),
		State:           domain.PublicationStateActive,
		CreatedAt:       now,
	}
	if err := pubRepo.Create(ctx, &pub); err != nil {
		t.Fatalf("failed to create publication: %v", err)
	}

	if err := pubRefRepo.AddRefs(ctx, pubID, []string{payloadID}); err != nil {
		t.Fatalf("failed to add publication payload ref: %v", err)
	}

	refs, err := pubRefRepo.ListPayloadIDsByPublication(ctx, pubID)
	if err != nil || len(refs) != 1 || refs[0] != payloadID {
		t.Fatalf("expected payload %s referenced, got %v", payloadID, refs)
	}

	// Attempting to delete the referenced payload via PruneUnreferenced must not delete it
	// unpin it first
	_ = payloadRepo.Pin(ctx, payloadID, false)
	pruned, err := payloadRepo.PruneUnreferenced(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if pruned != 0 {
		t.Fatalf("expected 0 pruned because payload is referenced by publication, got %d", pruned)
	}
	// Verify payload still exists
	if _, err := payloadRepo.GetByID(ctx, payloadID); err != nil {
		t.Fatalf("payload was incorrectly deleted despite publication ref!")
	}
}
