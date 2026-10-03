package publication_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
)

type dummyAuditRepo struct{}

func (d *dummyAuditRepo) Record(ctx context.Context, event *domain.AuditEvent) error {
	return nil
}
func (d *dummyAuditRepo) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, int, error) {
	return nil, 0, nil
}

func newCleanSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg := sqlite.Config{
		Path:            fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()),
		BusyTimeout:     time.Second,
		ForeignKeys:     true,
		WALMode:         false,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 0,
	}
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPublicationPayloadRefs_DeduplicationAndPruneProtection(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)

	// Set up schema and repos
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	payloadRepo := sqlite.NewSubscriptionPayloadRepository(db)
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	verRepo := sqlite.NewNodeConnectionVersionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	pubRefRepo := sqlite.NewPublicationPayloadRefRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)

	now := domain.NowUTC()

	// 1. Create a subscription & fetch
	subID := "sub_" + domain.MustNewUUIDv7()
	sub := domain.Subscription{
		ID:                 subID,
		Name:               "Test Sub",
		SourceURLSecretRef: "secret://ref",
		Enabled:            true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := subRepo.Create(ctx, &sub); err != nil {
		t.Fatal(err)
	}

	fetchID := "fetch_" + domain.MustNewUUIDv7()
	fetch := domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:sub_payload_123",
	}
	if err := fetchRepo.Create(ctx, &fetch); err != nil {
		t.Fatal(err)
	}

	// 2. Create raw payload
	payloadID := "payload_" + domain.MustNewUUIDv7()
	p := domain.SubscriptionPayload{
		ID:             payloadID,
		SubscriptionID: subID,
		FetchID:        fetchID,
		ContentDigest:  "sha256:sub_payload_123",
		BodyBlob:       []byte("proxies: []"),
		HTTPStatus:     200,
		HeadersJSON:    "{}",
		Pinned:         false,
		CreatedAt:      now,
	}
	if err := payloadRepo.Save(ctx, &p); err != nil {
		t.Fatal(err)
	}

	// 3. Create 2 nodes sharing the same payload, plus 1 legacy node
	nodeID1 := "node_test_01"
	nodeID2 := "node_test_02"
	legacyNodeID := "node_legacy_03"

	for _, n := range []struct {
		id   string
		port int
	}{
		{nodeID1, 8388},
		{nodeID2, 8389},
		{legacyNodeID, 8390},
	} {
		node := domain.Node{
			LogicalID:   n.id,
			Protocol:    domain.ProtocolSS,
			DisplayName: n.id,
			Server:      "1.1.1.1",
			Port:        n.port,
			Credentials: domain.InboundProtocolCredential{Password: "secret", Method: "aes-256-gcm"},
			Active:      true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
			t.Fatal(err)
		}
	}

	entryID1 := "entry_" + domain.MustNewUUIDv7()
	entryID2 := "entry_" + domain.MustNewUUIDv7()

	entry1 := domain.SubscriptionEntry{
		ID:                    entryID1,
		PayloadID:             payloadID,
		SubscriptionID:        subID,
		Ordinal:               0,
		SourceKey:             "k1",
		RawName:               "Node 1",
		Protocol:              domain.ProtocolSS,
		Server:                "1.1.1.1",
		Port:                  8388,
		EntryKind:             domain.EntryKindProxy,
		ClassificationReason:  "Normal proxy",
		ClassificationVersion: "v2-proven-combo",
		NodeLogicalID:         &nodeID1,
		CreatedAt:             now,
	}
	entry2 := domain.SubscriptionEntry{
		ID:                    entryID2,
		PayloadID:             payloadID,
		SubscriptionID:        subID,
		Ordinal:               1,
		SourceKey:             "k2",
		RawName:               "Node 2",
		Protocol:              domain.ProtocolSS,
		Server:                "1.1.1.2",
		Port:                  8388,
		EntryKind:             domain.EntryKindProxy,
		ClassificationReason:  "Normal proxy",
		ClassificationVersion: "v2-proven-combo",
		NodeLogicalID:         &nodeID2,
		CreatedAt:             now,
	}
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{entry1, entry2}); err != nil {
		t.Fatal(err)
	}

	// Link node1 and node2 versions to entry1 and entry2
	v1 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID1,
		ConnectionRevision:  1,
		EffectiveConfigJSON: `{"server":"1.1.1.1","port":8388}`,
		ConfigFingerprint:   "fp1",
		SourceEntryID:       &entryID1,
		SchemaVersion:       1,
		CreatedAt:           now,
	}
	v2 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID2,
		ConnectionRevision:  1,
		EffectiveConfigJSON: `{"server":"1.1.1.2","port":8388}`,
		ConfigFingerprint:   "fp2",
		SourceEntryID:       &entryID2,
		SchemaVersion:       1,
		CreatedAt:           now,
	}
	_ = verRepo.Save(ctx, &v1)
	_ = verRepo.Save(ctx, &v2)

	// Create policy and revision
	rev := domain.ConfigurationRevision{
		ID:        "rev_" + domain.MustNewUUIDv7(),
		CreatedAt: now,
	}
	if err := revRepo.Create(ctx, &rev); err != nil {
		t.Fatal(err)
	}
	if err := revRepo.SetActive(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}

	// Set up service with pubRefRepo
	svc := publication.NewService(pubRepo, &dummyAuditRepo{},
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithPayloadRefRepository(pubRefRepo),
	)

	// 4. Test ResolvePayloadIDsForNodes with Deduplication and Legacy NULL handling
	included := []domain.ManifestIncludedNode{
		{NodeID: nodeID1, ConnectionRevision: 1},
		{NodeID: nodeID2, ConnectionRevision: 1},      // shares same payloadID
		{NodeID: legacyNodeID, ConnectionRevision: 1}, // legacy node without raw payload
	}
	resolved, err := pubRefRepo.ResolvePayloadIDsForNodes(ctx, included)
	if err != nil {
		t.Fatalf("resolve payload IDs failed: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != payloadID {
		t.Fatalf("expected exactly 1 deduplicated payload ID %s, got %v", payloadID, resolved)
	}

	// 5. Test Preview generates draft and writes refs to DB
	snap := &resolver.ResolvedPolicySnapshot{
		RevisionID:     rev.ID,
		SnapshotDigest: "sha256:snap_test_digest",
		Nodes: []resolver.ResolvedNode{
			{LogicalID: nodeID1, DisplayName: "N1", Protocol: domain.ProtocolSS, Server: "1.1.1.1", Port: 8388, Credentials: domain.InboundProtocolCredential{Password: "p", Method: "aes-256-gcm"}, Active: true, ConnectionRevision: 1},
			{LogicalID: nodeID2, DisplayName: "N2", Protocol: domain.ProtocolSS, Server: "1.1.1.2", Port: 8388, Credentials: domain.InboundProtocolCredential{Password: "p", Method: "aes-256-gcm"}, Active: true, ConnectionRevision: 1},
		},
	}
	prevRes, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:   domain.TargetMihomo,
		Snapshot: snap,
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if len(prevRes.Manifest.PayloadIDs) != 1 || prevRes.Manifest.PayloadIDs[0] != payloadID {
		t.Fatalf("expected manifest payload_ids to contain %s, got %v", payloadID, prevRes.Manifest.PayloadIDs)
	}

	// Verify publication_payload_refs row exists in DB
	refRows, err := pubRefRepo.ListPayloadIDsByPublication(ctx, prevRes.SnapshotID)
	if err != nil || len(refRows) != 1 || refRows[0] != payloadID {
		t.Fatalf("expected publication_payload_refs to have %s, got %v", payloadID, refRows)
	}

	// 6. Test PruneUnreferenced cannot delete the referenced payload
	pruned, err := payloadRepo.PruneUnreferenced(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if pruned != 0 {
		t.Fatalf("expected 0 pruned because payload is referenced by publication draft, got %d", pruned)
	}

	// 7. Verify Publish activates draft and maintains payload protection
	pubRes, err := svc.Publish(ctx, publication.PublishCommand{
		Target:     domain.TargetMihomo,
		SnapshotID: prevRes.SnapshotID,
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if pubRes.Publication.State != domain.PublicationStateActive {
		t.Fatalf("expected publication state active, got %s", pubRes.Publication.State)
	}
	pruned, err = payloadRepo.PruneUnreferenced(ctx, now.Add(time.Hour))
	if err != nil || pruned != 0 {
		t.Fatalf("expected 0 pruned after publication activation, got %d (err: %v)", pruned, err)
	}
}

func TestPublicationPayloadRefs_SnapshotSourceVersion_UpdateSource_PrunePreservation(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)

	pubRefRepo := sqlite.NewPublicationPayloadRefRepository(db)
	payloadRepo := sqlite.NewSubscriptionPayloadRepository(db)
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	versionRepo := sqlite.NewNodeConnectionVersionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)

	now := domain.NowUTC()

	// 1. Setup subscription
	sub := domain.Subscription{
		ID:                 "sub-test-preservation",
		Name:               "Preservation Test",
		SourceURLSecretRef: "secret://test-sub",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := subRepo.Create(ctx, &sub); err != nil {
		t.Fatal(err)
	}

	fetch1 := domain.SubscriptionFetch{
		ID:             "fetch-1",
		SubscriptionID: sub.ID,
		StartedAt:      now.Add(-2 * time.Hour),
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:digest-v1",
	}
	if err := fetchRepo.Create(ctx, &fetch1); err != nil {
		t.Fatal(err)
	}

	fetch2 := domain.SubscriptionFetch{
		ID:             "fetch-2",
		SubscriptionID: sub.ID,
		StartedAt:      now.Add(-30 * time.Minute),
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:digest-v2",
	}
	if err := fetchRepo.Create(ctx, &fetch2); err != nil {
		t.Fatal(err)
	}

	// 2. Fetch 1 -> Payload 1 -> Entry 1 -> Node Version 1
	payload1ID := "payload-v1-" + domain.MustNewUUIDv7()
	p1 := domain.SubscriptionPayload{
		ID:             payload1ID,
		SubscriptionID: sub.ID,
		FetchID:        "fetch-1",
		ContentDigest:  "sha256:digest-v1",
		BodyBlob:       []byte("raw content v1"),
		HTTPStatus:     200,
		CreatedAt:      now.Add(-2 * time.Hour),
	}
	if err := payloadRepo.Save(ctx, &p1); err != nil {
		t.Fatal(err)
	}

	entry1ID := "entry-v1-" + domain.MustNewUUIDv7()
	e1 := domain.SubscriptionEntry{
		ID:             entry1ID,
		PayloadID:      payload1ID,
		SubscriptionID: sub.ID,
		Ordinal:        0,
		SourceKey:      "ss-node-1",
		RawName:        "Node 1",
		Protocol:       domain.ProtocolSS,
		Server:         "1.1.1.1",
		Port:           8388,
		EntryKind:      domain.EntryKindProxy,
		CreatedAt:      now.Add(-2 * time.Hour),
	}
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{e1}); err != nil {
		t.Fatal(err)
	}

	nodeID := "node-preservation-1"
	node := domain.Node{
		LogicalID:          nodeID,
		Protocol:           domain.ProtocolSS,
		DisplayName:        "Node 1",
		Server:             "1.1.1.1",
		Port:               8388,
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          now.Add(-2 * time.Hour),
		UpdatedAt:          now.Add(-2 * time.Hour),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}

	v1 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID,
		ConnectionRevision:  1,
		EffectiveConfigJSON: `{"server":"1.1.1.1","port":8388}`,
		ConfigFingerprint:   "sha256:fp1",
		SourceEntryID:       &entry1ID,
		CreatedAt:           now.Add(-2 * time.Hour),
	}
	if err := versionRepo.Save(ctx, &v1); err != nil {
		t.Fatal(err)
	}

	// 3. Create Publication 1 with Revision 1 -> pins Payload 1
	pub1ID := "pub-v1-" + domain.MustNewUUIDv7()
	pub1 := domain.Publication{
		ID:              pub1ID,
		Target:          domain.TargetMihomo,
		SnapshotDigest:  "digest-pub1",
		ContentDigest:   "sha256:pub1",
		Content:         []byte("proxies: []"),
		CompilerVersion: "1.0.0",
		TokenHash:       "token-pub1",
		State:           domain.PublicationStateActive,
		CreatedAt:       now.Add(-90 * time.Minute),
	}
	if err := pubRepo.Create(ctx, &pub1); err != nil {
		t.Fatal(err)
	}
	if err := pubRefRepo.AddRefs(ctx, pub1ID, []string{payload1ID}); err != nil {
		t.Fatal(err)
	}

	// 4. Update Source: Fetch 2 -> Payload 2 -> Entry 2 -> Node Version 2 (server changes to 2.2.2.2)
	payload2ID := "payload-v2-" + domain.MustNewUUIDv7()
	p2 := domain.SubscriptionPayload{
		ID:             payload2ID,
		SubscriptionID: sub.ID,
		FetchID:        "fetch-2",
		ContentDigest:  "sha256:digest-v2",
		BodyBlob:       []byte("raw content v2"),
		HTTPStatus:     200,
		CreatedAt:      now.Add(-30 * time.Minute),
	}
	if err := payloadRepo.Save(ctx, &p2); err != nil {
		t.Fatal(err)
	}

	entry2ID := "entry-v2-" + domain.MustNewUUIDv7()
	e2 := domain.SubscriptionEntry{
		ID:             entry2ID,
		PayloadID:      payload2ID,
		SubscriptionID: sub.ID,
		Ordinal:        0,
		SourceKey:      "ss-node-1",
		RawName:        "Node 1 Updated",
		Protocol:       domain.ProtocolSS,
		Server:         "2.2.2.2",
		Port:           8388,
		EntryKind:      domain.EntryKindProxy,
		CreatedAt:      now.Add(-30 * time.Minute),
	}
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{e2}); err != nil {
		t.Fatal(err)
	}

	node.Server = "2.2.2.2"
	node.ConnectionRevision = 2
	node.UpdatedAt = now.Add(-30 * time.Minute)
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}

	v2 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID,
		ConnectionRevision:  2,
		EffectiveConfigJSON: `{"server":"2.2.2.2","port":8388}`,
		ConfigFingerprint:   "sha256:fp2",
		SourceEntryID:       &entry2ID,
		CreatedAt:           now.Add(-30 * time.Minute),
	}
	if err := versionRepo.Save(ctx, &v2); err != nil {
		t.Fatal(err)
	}

	// 5. Verify ResolvePayloadIDsForNodes for Old Publication (Revision 1) STILL resolves to Payload 1!
	oldInc := []domain.ManifestIncludedNode{
		{NodeID: nodeID, ConnectionRevision: 1},
	}
	resolvedOld, err := pubRefRepo.ResolvePayloadIDsForNodes(ctx, oldInc)
	if err != nil {
		t.Fatalf("ResolvePayloadIDsForNodes error: %v", err)
	}
	if len(resolvedOld) != 1 || resolvedOld[0] != payload1ID {
		t.Fatalf("expected old publication to resolve to payload1 (%s), got: %v", payload1ID, resolvedOld)
	}

	// Verify ResolvePayloadIDsForNodes for New Revision (Revision 2) resolves to Payload 2!
	newInc := []domain.ManifestIncludedNode{
		{NodeID: nodeID, ConnectionRevision: 2},
	}
	resolvedNew, err := pubRefRepo.ResolvePayloadIDsForNodes(ctx, newInc)
	if err != nil {
		t.Fatalf("ResolvePayloadIDsForNodes error: %v", err)
	}
	if len(resolvedNew) != 1 || resolvedNew[0] != payload2ID {
		t.Fatalf("expected new revision to resolve to payload2 (%s), got: %v", payload2ID, resolvedNew)
	}

	// 6. Execute PruneUnreferenced: Payload 1 MUST be preserved because Publication 1 still pins it!
	pruned, err := payloadRepo.PruneUnreferenced(ctx, now)
	if err != nil {
		t.Fatalf("PruneUnreferenced failed: %v", err)
	}
	// Payload 1 is pinned by pub1 ref. Payload 2 is unpinned in publications, but let's check payload 1 is NOT pruned.
	storedP1, err := payloadRepo.GetByID(ctx, payload1ID)
	if err != nil || storedP1 == nil {
		t.Fatalf("payload1 was unexpectedly pruned despite being pinned by old publication! (err: %v, pruned count: %d)", err, pruned)
	}

	// 7. Verify Publication 1's pinned refs still list Payload 1
	pinnedRefs, err := pubRefRepo.ListPayloadIDsByPublication(ctx, pub1ID)
	if err != nil || len(pinnedRefs) != 1 || pinnedRefs[0] != payload1ID {
		t.Fatalf("expected publication1 to pin payload1, got %v", pinnedRefs)
	}
}

func TestPublicationPayloadRefs_SameConnectionRefresh_PreservesImmutableProvenanceAndProtectedPayload(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)

	pubRefRepo := sqlite.NewPublicationPayloadRefRepository(db)
	payloadRepo := sqlite.NewSubscriptionPayloadRepository(db)
	entryRepo := sqlite.NewSubscriptionEntryRepository(db)
	versionRepo := sqlite.NewNodeConnectionVersionRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)

	now := domain.NowUTC()

	// 1. Setup subscription
	sub := domain.Subscription{
		ID:                 "sub-immutable-test",
		Name:               "Immutable Provenance Test",
		SourceURLSecretRef: "secret://test-sub",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := subRepo.Create(ctx, &sub); err != nil {
		t.Fatal(err)
	}

	fetch1 := domain.SubscriptionFetch{
		ID:             "fetch-same-1",
		SubscriptionID: sub.ID,
		StartedAt:      now.Add(-2 * time.Hour),
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:same-digest-1",
	}
	if err := fetchRepo.Create(ctx, &fetch1); err != nil {
		t.Fatal(err)
	}

	// Initial Payload 1 -> Entry 1
	payload1ID := "payload-same-1"
	p1 := domain.SubscriptionPayload{
		ID:             payload1ID,
		SubscriptionID: sub.ID,
		FetchID:        fetch1.ID,
		ContentDigest:  "sha256:same-digest-1",
		BodyBlob:       []byte("raw content v1"),
		HTTPStatus:     200,
		CreatedAt:      now.Add(-2 * time.Hour),
	}
	if err := payloadRepo.Save(ctx, &p1); err != nil {
		t.Fatal(err)
	}

	entry1ID := "entry-same-1"
	e1 := domain.SubscriptionEntry{
		ID:             entry1ID,
		PayloadID:      payload1ID,
		SubscriptionID: sub.ID,
		Ordinal:        0,
		SourceKey:      "same-key-1",
		RawName:        "Node Same",
		Protocol:       domain.ProtocolSS,
		Server:         "1.1.1.1",
		Port:           8388,
		EntryKind:      domain.EntryKindProxy,
		CreatedAt:      now.Add(-2 * time.Hour),
	}
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{e1}); err != nil {
		t.Fatal(err)
	}

	nodeID := "node-same-conn-1"
	node := domain.Node{
		LogicalID:          nodeID,
		Protocol:           domain.ProtocolSS,
		DisplayName:        "Node Same",
		Server:             "1.1.1.1",
		Port:               8388,
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          now.Add(-2 * time.Hour),
		UpdatedAt:          now.Add(-2 * time.Hour),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}

	// Immutable Version 1 initially formed with source_entry_id = entry1ID
	v1 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID,
		ConnectionRevision:  1,
		EffectiveConfigJSON: `{"server":"1.1.1.1","port":8388}`,
		ConfigFingerprint:   "sha256:fpsame",
		SourceEntryID:       &entry1ID,
		CreatedAt:           now.Add(-2 * time.Hour),
	}
	if err := versionRepo.Save(ctx, &v1); err != nil {
		t.Fatal(err)
	}

	// Legacy node without source_entry_id (NULL legacy)
	legacyNodeID := "node-legacy-null-1"
	legacyNode := domain.Node{
		LogicalID:          legacyNodeID,
		Protocol:           domain.ProtocolSS,
		DisplayName:        "Node Legacy",
		Server:             "1.1.1.2",
		Port:               8388,
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          now.Add(-2 * time.Hour),
		UpdatedAt:          now.Add(-2 * time.Hour),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{legacyNode}); err != nil {
		t.Fatal(err)
	}
	vLegacy := domain.NodeConnectionVersion{
		NodeLogicalID:       legacyNodeID,
		ConnectionRevision:  1,
		EffectiveConfigJSON: `{"server":"1.1.1.2","port":8388}`,
		ConfigFingerprint:   "sha256:fplegacy",
		SourceEntryID:       nil, // NULL legacy
		CreatedAt:           now.Add(-2 * time.Hour),
	}
	if err := versionRepo.Save(ctx, &vLegacy); err != nil {
		t.Fatal(err)
	}

	// Publication 1 references Version 1 -> pins Payload 1
	pub1ID := "pub-same-" + domain.MustNewUUIDv7()
	pub1 := domain.Publication{
		ID:              pub1ID,
		Target:          domain.TargetMihomo,
		SnapshotDigest:  "digest-pubsame1",
		ContentDigest:   "sha256:pubsame1",
		Content:         []byte("proxies: []"),
		CompilerVersion: "1.0.0",
		TokenHash:       "token-pubsame1",
		State:           domain.PublicationStateActive,
		CreatedAt:       now.Add(-90 * time.Minute),
	}
	if err := pubRepo.Create(ctx, &pub1); err != nil {
		t.Fatal(err)
	}
	if err := pubRefRepo.AddRefs(ctx, pub1ID, []string{payload1ID}); err != nil {
		t.Fatal(err)
	}

	// 2. Simulate same connection refresh (connEqual == true)
	// New fetch 2 -> Payload 2 -> Entry 2
	fetch2 := domain.SubscriptionFetch{
		ID:             "fetch-same-2",
		SubscriptionID: sub.ID,
		StartedAt:      now.Add(-10 * time.Minute),
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:same-digest-2",
	}
	if err := fetchRepo.Create(ctx, &fetch2); err != nil {
		t.Fatal(err)
	}

	payload2ID := "payload-same-2"
	p2 := domain.SubscriptionPayload{
		ID:             payload2ID,
		SubscriptionID: sub.ID,
		FetchID:        fetch2.ID,
		ContentDigest:  "sha256:same-digest-2",
		BodyBlob:       []byte("raw content v2"),
		HTTPStatus:     200,
		CreatedAt:      now.Add(-10 * time.Minute),
	}
	if err := payloadRepo.Save(ctx, &p2); err != nil {
		t.Fatal(err)
	}

	entry2ID := "entry-same-2"
	e2 := domain.SubscriptionEntry{
		ID:             entry2ID,
		PayloadID:      payload2ID,
		SubscriptionID: sub.ID,
		Ordinal:        0,
		SourceKey:      "same-key-1",
		RawName:        "Node Same Refreshed",
		Protocol:       domain.ProtocolSS,
		Server:         "1.1.1.1",
		Port:           8388,
		EntryKind:      domain.EntryKindProxy,
		CreatedAt:      now.Add(-10 * time.Minute),
	}
	legacyEntryID := "entry-legacy-backfill"
	eLegacy := domain.SubscriptionEntry{
		ID:             legacyEntryID,
		PayloadID:      payload2ID,
		SubscriptionID: sub.ID,
		Ordinal:        1,
		SourceKey:      "same-key-legacy",
		RawName:        "Node Legacy Refreshed",
		Protocol:       domain.ProtocolSS,
		Server:         "1.1.1.2",
		Port:           8388,
		EntryKind:      domain.EntryKindProxy,
		CreatedAt:      now.Add(-10 * time.Minute),
	}
	if err := entryRepo.SaveBatch(ctx, []domain.SubscriptionEntry{e2, eLegacy}); err != nil {
		t.Fatal(err)
	}

	// Try to upsert version 1 with entry2ID (simulating connEqual execution on same revision)
	refreshV1 := domain.NodeConnectionVersion{
		NodeLogicalID:       nodeID,
		ConnectionRevision:  1,
		EffectiveConfigJSON: `{"server":"1.1.1.1","port":8388}`,
		ConfigFingerprint:   "sha256:fpsame",
		SourceEntryID:       &entry2ID,
		CreatedAt:           now.Add(-10 * time.Minute),
	}
	if err := versionRepo.Save(ctx, &refreshV1); err != nil {
		t.Fatal(err)
	}

	// Try to upsert legacy version with legacyEntryID (simulating backfilling previously NULL version)
	refreshVLegacy := domain.NodeConnectionVersion{
		NodeLogicalID:       legacyNodeID,
		ConnectionRevision:  1,
		EffectiveConfigJSON: `{"server":"1.1.1.2","port":8388}`,
		ConfigFingerprint:   "sha256:fplegacy",
		SourceEntryID:       &legacyEntryID,
		CreatedAt:           now.Add(-10 * time.Minute),
	}
	if err := versionRepo.Save(ctx, &refreshVLegacy); err != nil {
		t.Fatal(err)
	}

	// 3. Verify Immutable Provenance:
	// - v1 MUST preserve entry1ID (did NOT mutate to entry2ID!)
	loadedV1, err := versionRepo.GetByRevision(ctx, nodeID, 1)
	if err != nil {
		t.Fatalf("failed to load v1: %v", err)
	}
	if loadedV1.SourceEntryID == nil || *loadedV1.SourceEntryID != entry1ID {
		t.Fatalf("immutable provenance VIOLATED: v1 source_entry_id was overwritten! got %v, want %s",
			loadedV1.SourceEntryID, entry1ID)
	}

	// - legacy vLegacy MUST be backfilled with legacyEntryID (NULL legacy was successfully explainable-backfilled)
	loadedVLegacy, err := versionRepo.GetByRevision(ctx, legacyNodeID, 1)
	if err != nil {
		t.Fatalf("failed to load legacy version: %v", err)
	}
	if loadedVLegacy.SourceEntryID == nil || *loadedVLegacy.SourceEntryID != legacyEntryID {
		t.Fatalf("legacy backfill failed: expected %s, got %v", legacyEntryID, loadedVLegacy.SourceEntryID)
	}

	// 4. Verify Publication 1 STILL resolves to Payload 1 (not Payload 2)
	resolved, err := pubRefRepo.ResolvePayloadIDsForNodes(ctx, []domain.ManifestIncludedNode{
		{NodeID: nodeID, ConnectionRevision: 1},
	})
	if err != nil {
		t.Fatalf("ResolvePayloadIDsForNodes failed: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != payload1ID {
		t.Fatalf("expected old publication to resolve to payload1 (%s), got: %v", payload1ID, resolved)
	}

	// 5. PruneUnreferenced MUST NOT delete Payload 1
	pruned, err := payloadRepo.PruneUnreferenced(ctx, now)
	if err != nil {
		t.Fatalf("PruneUnreferenced failed: %v", err)
	}
	storedP1, err := payloadRepo.GetByID(ctx, payload1ID)
	if err != nil || storedP1 == nil {
		t.Fatalf("payload1 was unexpectedly pruned! (err: %v, pruned count: %d)", err, pruned)
	}
}
