package publication_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

func TestPreview_TargetDefaultsAndModeValidation(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setupService()

	validSSID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	validSS := resolver.ResolvedNode{
		LogicalID:   validSSID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "Valid SS",
		Server:      "1.1.1.1",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "password123",
		},
		Active: true,
	}

	snap := &resolver.ResolvedPolicySnapshot{
		RevisionID:      "rev-test-modes",
		SnapshotDigest:  "sha256:snap-test-modes",
		CompilerVersion: "1.0.0",
		Nodes:           []resolver.ResolvedNode{validSS},
		NodeLogicalIDs:  []string{validSS.LogicalID},
		Groups: []resolver.ResolvedGroup{
			{
				ID:        "grp-test",
				Name:      "Proxy",
				GroupType: domain.GroupTypeSelect,
				Members: []resolver.ResolvedGroupMember{
					{Kind: resolver.MemberKindNode, TargetID: validSS.LogicalID, DisplayName: validSS.DisplayName},
				},
				AllNodeLogicalIDs: []string{validSS.LogicalID},
			},
		},
		Rules: []resolver.ResolvedRule{
			{ID: "r1", TargetGroupName: "Proxy", Expression: "MATCH"},
		},
	}

	t.Run("DefaultModesPerTarget", func(t *testing.T) {
		for _, target := range []domain.CompilerTarget{domain.TargetMihomo, domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX} {
			res, err := svc.Preview(ctx, publication.PreviewQuery{
				Target:   target,
				Snapshot: snap,
			})
			if err != nil {
				t.Fatalf("expected default preview for %s to succeed, got: %v", target, err)
			}
			if len(res.Content) == 0 {
				t.Fatalf("expected non-empty preview content for %s", target)
			}
			if len(res.Manifest.Included) != 1 {
				t.Fatalf("expected 1 included node in manifest for %s, got %d", target, len(res.Manifest.Included))
			}
		}
	})

	t.Run("RejectInvalidMode", func(t *testing.T) {
		for _, badMode := range []string{"invalid", "fuzzy", "lenient", "UNKNOWN", "strict_v2"} {
			_, err := svc.Preview(ctx, publication.PreviewQuery{
				Target:     domain.TargetSingBox,
				Snapshot:   snap,
				CompatMode: badMode,
			})
			if err == nil {
				t.Fatalf("expected invalid mode %q to be rejected, got nil error", badMode)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "invalid_mode" {
				t.Fatalf("expected invalid_mode error for mode %q, got: %v", badMode, err)
			}
		}
	})

	t.Run("ExplicitStrict_RejectsIncompatibleOnNonMihomo", func(t *testing.T) {
		// Mixed snapshot: SS + VLESS (unsupported on Surge / QX)
		vlessNodeID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "2.2.2.2", 443, nil)
		vlessNode := resolver.ResolvedNode{
			LogicalID:   vlessNodeID,
			Protocol:    domain.ProtocolVLESS,
			DisplayName: "VLESS Node",
			Server:      "2.2.2.2",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID: "01944aa0-0000-7000-8000-000000000001",
			},
			Active: true,
		}
		mixedSnap := &resolver.ResolvedPolicySnapshot{
			RevisionID:      "rev-mixed",
			SnapshotDigest:  "sha256:snap-mixed",
			CompilerVersion: "1.0.0",
			Nodes:           []resolver.ResolvedNode{validSS, vlessNode},
			NodeLogicalIDs:  []string{validSS.LogicalID, vlessNode.LogicalID},
		}

		// Surge explicit strict should reject with StrictCapabilityError
		_, err := svc.Preview(ctx, publication.PreviewQuery{
			Target:     domain.TargetSurge,
			Snapshot:   mixedSnap,
			CompatMode: "strict",
		})
		if err == nil {
			t.Fatal("expected Surge explicit strict to fail for VLESS, but got nil")
		}
		var strictErr *publication.StrictCapabilityError
		if !errors.As(err, &strictErr) {
			t.Fatalf("expected *publication.StrictCapabilityError, got %T: %v", err, err)
		}
		if !strings.Contains(strictErr.Error(), "target capability validation failed in strict mode") {
			t.Fatalf("expected strict capability failure message, got: %v", strictErr)
		}
		if !strings.HasPrefix(strictErr.SnapshotID, "snapshot_") {
			t.Fatalf("expected valid draft snapshot ID on strict capability error, got: %s", strictErr.SnapshotID)
		}
	})

	t.Run("ExplicitCompatible_AcceptsMixedOnNonMihomo", func(t *testing.T) {
		vlessNodeID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "2.2.2.2", 443, nil)
		vlessNode := resolver.ResolvedNode{
			LogicalID:   vlessNodeID,
			Protocol:    domain.ProtocolVLESS,
			DisplayName: "VLESS Node",
			Server:      "2.2.2.2",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID: "01944aa0-0000-7000-8000-000000000001",
			},
			Active: true,
		}
		mixedSnap := &resolver.ResolvedPolicySnapshot{
			RevisionID:      "rev-mixed",
			SnapshotDigest:  "sha256:snap-mixed",
			CompilerVersion: "1.0.0",
			Nodes:           []resolver.ResolvedNode{validSS, vlessNode},
			NodeLogicalIDs:  []string{validSS.LogicalID, vlessNode.LogicalID},
		}

		// Surge default and explicit compatible should succeed and exclude VLESS
		res, err := svc.Preview(ctx, publication.PreviewQuery{
			Target:     domain.TargetSurge,
			Snapshot:   mixedSnap,
			CompatMode: "compatible",
		})
		if err != nil {
			t.Fatalf("expected compatible Surge preview to succeed, got: %v", err)
		}
		if res.Manifest.NodeCount != 1 {
			t.Fatalf("expected 1 compatible node in manifest, got %d", res.Manifest.NodeCount)
		}
		if res.Manifest.ExcludedCount != 1 {
			t.Fatalf("expected 1 excluded node in manifest, got %d", res.Manifest.ExcludedCount)
		}
		if len(res.Manifest.Excluded) != 1 || res.Manifest.Excluded[0].NodeID != vlessNode.LogicalID {
			t.Fatalf("expected VLESS node to be excluded, got: %+v", res.Manifest.Excluded)
		}
	})
}

func TestPreview_FactsPath_RealDraftPersistenceAndPublishAfterVersionGC(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)

	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	payloadRepo := sqlite.NewSubscriptionPayloadRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	pubRefRepo := sqlite.NewPublicationPayloadRefRepository(db)
	revRepo := sqlite.NewRevisionRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	factsLoader := sqlite.NewConsistentFactsLoader(db)

	now := domain.NowUTC()

	// 1. Subscription & fetch
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
		ContentDigest:  "sha256:sub_payload_lifecycle",
	}
	if err := fetchRepo.Create(ctx, &fetch); err != nil {
		t.Fatal(err)
	}

	// 2. Payload
	payloadID := "payload_" + domain.MustNewUUIDv7()
	p := domain.SubscriptionPayload{
		ID:             payloadID,
		SubscriptionID: subID,
		FetchID:        fetchID,
		ContentDigest:  "sha256:sub_payload_lifecycle",
		BodyBlob:       []byte("proxies: []"),
		HTTPStatus:     200,
		HeadersJSON:    "{}",
		Pinned:         false,
		CreatedAt:      now,
	}
	if err := payloadRepo.Save(ctx, &p); err != nil {
		t.Fatal(err)
	}

	// 3. Node & source
	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.2.3.4", 8388, nil)
	node := domain.Node{
		LogicalID:   nodeID,
		DisplayName: "Lifecycle SS Node",
		Protocol:    domain.ProtocolSS,
		Server:      "1.2.3.4",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "secretpassword",
		},
		Active:             true,
		ConnectionRevision: 1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, nodeID, subID, fetchID); err != nil {
		t.Fatal(err)
	}

	// Bind the known original payload to the version created by UpsertBatch.
	entryID := "entry-lifecycle-1"
	if _, err := db.ExecContext(ctx, `INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,server,port,entry_kind,classification_reason,node_logical_id,created_at) VALUES (?,?,?,0,'lifecycle','Lifecycle SS Node','ss','1.2.3.4',8388,'proxy','fixture',?,?)`, entryID, payloadID, subID, nodeID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE node_connection_versions SET source_entry_id=? WHERE node_logical_id=? AND connection_revision=1`, entryID, nodeID); err != nil {
		t.Fatal(err)
	}

	// 4. Revision
	revID := "rev-lifecycle-1"
	rev := domain.ConfigurationRevision{
		ID:            revID,
		ContentDigest: "sha256:rev-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     now,
	}
	if err := revRepo.Create(ctx, &rev); err != nil {
		t.Fatal(err)
	}
	if err := revRepo.SetActive(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}

	svc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithPayloadRefRepository(pubRefRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)

	// 5. Execute Preview via facts path (Snapshot is nil)
	prevRes, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetSingBox,
		RevisionID: revID,
	})
	if err != nil {
		t.Fatalf("preview via facts path failed: %v", err)
	}

	// Verify SnapshotID is a real persisted draft ID, not digest pseudo-ID
	if !strings.HasPrefix(prevRes.SnapshotID, "snapshot_") {
		t.Fatalf("expected SnapshotID to start with 'snapshot_', got: %s", prevRes.SnapshotID)
	}
	if prevRes.SnapshotID == prevRes.SnapshotDigest {
		t.Fatalf("SnapshotID must not be a digest pseudo-ID: %s", prevRes.SnapshotID)
	}

	// Verify draft exists in pubRepo
	draftPub, err := pubRepo.GetByID(ctx, prevRes.SnapshotID)
	if err != nil {
		t.Fatalf("failed to retrieve draft from repository: %v", err)
	}
	if draftPub.State != domain.PublicationStateDraft {
		t.Fatalf("expected draft state, got: %s", draftPub.State)
	}
	if string(draftPub.Content) != string(prevRes.Content) {
		t.Fatal("persisted draft content does not match preview content")
	}
	if draftPub.ContentDigest != prevRes.ContentDigest {
		t.Fatalf("persisted draft content digest %q != preview digest %q", draftPub.ContentDigest, prevRes.ContentDigest)
	}

	// Verify payload refs are persisted
	refRows, err := pubRefRepo.ListPayloadIDsByPublication(ctx, prevRes.SnapshotID)
	if err != nil || len(refRows) != 1 || refRows[0] != payloadID {
		t.Fatalf("expected publication_payload_refs to have payload %s, got: %v (err: %v)", payloadID, refRows, err)
	}

	// Verify manifest
	if prevRes.Manifest.NodeCount != 1 || len(prevRes.Manifest.Included) != 1 || prevRes.Manifest.Included[0].NodeID != nodeID {
		t.Fatalf("unexpected manifest included nodes: %+v", prevRes.Manifest)
	}
	if prevRes.Manifest.ExcludedCount != 0 {
		t.Fatalf("expected 0 excluded nodes, got: %d", prevRes.Manifest.ExcludedCount)
	}
	if prevRes.Manifest.Included[0].ConnectionRevision != 1 || len(prevRes.Manifest.PayloadIDs) != 1 || prevRes.Manifest.PayloadIDs[0] != payloadID {
		t.Fatalf("preview did not freeze revision 1 and its origin: %+v", prevRes.Manifest)
	}
	if !strings.Contains(string(prevRes.Content), node.Server) {
		t.Fatal("preview content did not contain the original connection")
	}

	assertFrozenOrigin := func(stage string) {
		t.Helper()
		refs, err := pubRefRepo.ListPayloadIDsByPublication(ctx, prevRes.SnapshotID)
		if err != nil || len(refs) != 1 || refs[0] != payloadID {
			t.Fatalf("%s: frozen payload refs changed: %v (err: %v)", stage, refs, err)
		}
		var source, originFetch, digest string
		err = db.QueryRowContext(ctx, `SELECT e.source_identity,e.fetch_id,e.content_digest
			FROM publication_payload_refs r JOIN subscription_payload_evidence e ON e.payload_id=r.payload_id
			WHERE r.publication_id=? AND r.payload_id=?`, prevRes.SnapshotID, payloadID).Scan(&source, &originFetch, &digest)
		if err != nil || source != "sub:"+subID || originFetch != fetchID || digest != p.ContentDigest {
			t.Fatalf("%s: frozen origin changed: source=%q fetch=%q digest=%q err=%v", stage, source, originFetch, digest, err)
		}
	}
	assertFrozenOrigin("after preview")

	// Advance the current subscription generation and head before publishing the
	// draft. Fresh SQLite storage has consumer closure sealed, so the unleased
	// old full connection version is reclaimed by UpsertBatch.
	fetch2 := fetch
	fetch2.ID = "fetch_" + domain.MustNewUUIDv7()
	fetch2.ContentDigest = "sha256:sub_payload_lifecycle_v2"
	if err := fetchRepo.Create(ctx, &fetch2); err != nil {
		t.Fatal(err)
	}
	replaceFixtureGeneration(t, ctx, db, subID)
	p2 := p
	p2.ID = "payload_" + domain.MustNewUUIDv7()
	p2.FetchID = fetch2.ID
	p2.ContentDigest = fetch2.ContentDigest
	p2.BodyBlob = []byte("proxies: [new generation]")
	if err := payloadRepo.Save(ctx, &p2); err != nil {
		t.Fatal(err)
	}
	entry2ID := "entry-lifecycle-2"
	if _, err := db.ExecContext(ctx, `INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,server,port,entry_kind,classification_reason,node_logical_id,created_at) VALUES (?,?,?,0,'lifecycle','Lifecycle SS Node','ss','1.2.3.5',8388,'proxy','fixture',?,?)`, entry2ID, p2.ID, subID, nodeID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	node.Server = "1.2.3.5"
	node.Credentials.Password = "new-synthetic-password"
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatal(err)
	}
	attachFixtureOrigin(t, ctx, db, nodeID, 2, entry2ID)
	if _, err := db.ExecContext(ctx, `UPDATE node_sources SET last_seen_fetch_id=? WHERE node_logical_id=? AND subscription_id=?`, fetch2.ID, nodeID, subID); err != nil {
		t.Fatal(err)
	}
	versions := sqlite.NewNodeConnectionVersionRepository(db)
	head, err := versions.GetHead(ctx, nodeID)
	if err != nil || head.ConnectionRevision != 2 {
		t.Fatalf("expected advanced head revision 2: %+v err=%v", head, err)
	}
	var oldVersionCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions WHERE node_logical_id=? AND connection_revision=1`, nodeID).Scan(&oldVersionCount); err != nil || oldVersionCount != 0 {
		t.Fatalf("old connection version was not reclaimed: count=%d err=%v", oldVersionCount, err)
	}
	assertRetiredPayloadEvidence(t, ctx, db, payloadRepo, payloadID, entryID, p.ContentDigest)
	assertFrozenOrigin("after refresh and GC")

	newPreview, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSingBox, RevisionID: revID})
	if err != nil {
		t.Fatalf("preview of refreshed head failed: %v", err)
	}
	if !strings.Contains(string(newPreview.Content), node.Server) || string(newPreview.Content) == string(prevRes.Content) {
		t.Fatal("refreshed preview did not use the new connection")
	}
	if len(newPreview.Manifest.Included) != 1 || newPreview.Manifest.Included[0].ConnectionRevision != 2 || len(newPreview.Manifest.PayloadIDs) != 1 || newPreview.Manifest.PayloadIDs[0] != p2.ID {
		t.Fatalf("refreshed preview did not use new revision and origin: %+v", newPreview.Manifest)
	}

	// 6. Real Publish using the original persisted draft SnapshotID
	pubRes, err := svc.Publish(ctx, publication.PublishCommand{
		Target:     domain.TargetSingBox,
		SnapshotID: prevRes.SnapshotID,
		ActorKind:  domain.ActorKindAdmin,
		RequestID:  "req-publish-lifecycle-test",
	})
	if err != nil {
		t.Fatalf("publish with preview draft ID failed: %v", err)
	}
	if pubRes.Publication.State != domain.PublicationStateActive {
		t.Fatalf("expected publication state active, got: %s", pubRes.Publication.State)
	}
	if pubRes.ContentDigest != prevRes.ContentDigest {
		t.Fatalf("published content digest %q != preview content digest %q", pubRes.ContentDigest, prevRes.ContentDigest)
	}
	assertFrozenOrigin("after publish")

	// 7. Download and verify byte equality
	artifact, err := svc.ResolveAndServeAuthorized(ctx, pubRes.Publication.ID, func(tokenHash string) bool { return true })
	if err != nil {
		t.Fatalf("ResolveAndServeAuthorized failed: %v", err)
	}
	if string(artifact.Content) != string(prevRes.Content) {
		t.Fatal("downloaded publication bytes do not match preview bytes")
	}
	if artifact.ContentDigest != prevRes.ContentDigest {
		t.Fatalf("downloaded content digest %s != preview digest %s", artifact.ContentDigest, prevRes.ContentDigest)
	}
}

func TestPreview_FactsPath_NoSilentFallbackOnFactsFailure(t *testing.T) {
	ctx := context.Background()

	// Facts loader returns error
	failingLoader := &memoryFactsLoader{
		err: fmt.Errorf("database transaction failed: read replica unavailable"),
	}

	svc := publication.NewService(
		nil,
		&dummyAuditRepo{},
		publication.WithConsistentFactsLoader(failingLoader),
	)

	_, err := svc.Preview(ctx, publication.PreviewQuery{
		Target: domain.TargetSingBox,
	})
	if err == nil {
		t.Fatal("expected preview to fail when facts loader fails, but got nil error")
	}
	if !strings.Contains(err.Error(), "database transaction failed") {
		t.Fatalf("expected facts loader error to be propagated, got: %v", err)
	}
}

func TestBuildCurrentArtifact_NonMihomoCompatibleProjection(t *testing.T) {
	ctx := context.Background()
	svc := publication.NewService(nil, nil)

	validSSID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	validSS := domain.Node{
		LogicalID:   validSSID,
		DisplayName: "Valid Shadowsocks",
		Protocol:    domain.ProtocolSS,
		Server:      "1.1.1.1",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "secretpassword",
		},
		Active:             true,
		ConnectionRevision: 1,
	}

	vlessNodeID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "2.2.2.2", 443, nil)
	vlessNode := domain.Node{
		LogicalID:   vlessNodeID,
		DisplayName: "VLESS Unsupported on Surge",
		Protocol:    domain.ProtocolVLESS,
		Server:      "2.2.2.2",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID: "01944aa0-0000-7000-8000-000000000001",
		},
		Active:             true,
		ConnectionRevision: 1,
	}

	rev := &domain.ConfigurationRevision{
		ID:            "rev-compat-test",
		ContentDigest: "sha256:rev-compat-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     time.Now().UTC(),
	}

	facts := &domain.ConsistentFacts{
		Revision:               rev,
		Nodes:                  []domain.Node{validSS, vlessNode},
		EnabledSubscriptionIDs: make(map[string]bool),
		AsOf:                   time.Now().UTC(),
	}

	// 1. Surge export: should project compatible nodes (exclude VLESS)
	artSurge, snapSurge, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetSurge)
	if err != nil {
		t.Fatalf("expected Surge compatible export to succeed, got: %v", err)
	}
	if len(artSurge.ExcludedNodes) != 1 {
		t.Fatalf("expected 1 excluded node on Surge, got %d", len(artSurge.ExcludedNodes))
	}
	if artSurge.ExcludedNodes[0].NodeID != vlessNode.LogicalID {
		t.Fatalf("expected excluded node ID %s, got %s", vlessNode.LogicalID, artSurge.ExcludedNodes[0].NodeID)
	}
	if len(snapSurge.Nodes) != 1 || snapSurge.Nodes[0].LogicalID != validSS.LogicalID {
		t.Fatalf("expected only SS node in Surge snapshot, got: %+v", snapSurge.Nodes)
	}
	if len(snapSurge.NodeLogicalIDs) != 1 || snapSurge.NodeLogicalIDs[0] != validSS.LogicalID {
		t.Fatalf("expected only SS node ID in Surge snapshot IDs, got: %+v", snapSurge.NodeLogicalIDs)
	}

	// Verify safe exclusion warning in diagnostics
	hasWarning := false
	for _, d := range snapSurge.Diagnostics {
		if d.Severity == resolver.DiagnosticSeverityWarning && d.Target == vlessNode.LogicalID {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Fatalf("expected safe warning diagnostic for excluded node, got diagnostics: %+v", snapSurge.Diagnostics)
	}

	// 2. Quantumult X export: should also project compatible nodes
	artQX, snapQX, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetQuantumultX)
	if err != nil {
		t.Fatalf("expected Quantumult X compatible export to succeed, got: %v", err)
	}
	if len(artQX.ExcludedNodes) != 1 || artQX.ExcludedNodes[0].NodeID != vlessNode.LogicalID {
		t.Fatalf("expected 1 excluded VLESS node on Quantumult X, got: %+v", artQX.ExcludedNodes)
	}
	if len(snapQX.Nodes) != 1 || snapQX.Nodes[0].LogicalID != validSS.LogicalID {
		t.Fatalf("expected only SS node on Quantumult X, got: %+v", snapQX.Nodes)
	}

	// 3. SingBox export: VLESS is supported on SingBox, so 0 exclusions
	artSB, snapSB, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetSingBox)
	if err != nil {
		t.Fatalf("expected SingBox export to succeed, got: %v", err)
	}
	if len(artSB.ExcludedNodes) != 0 {
		t.Fatalf("expected 0 excluded nodes on SingBox, got %d", len(artSB.ExcludedNodes))
	}
	if len(snapSB.Nodes) != 2 {
		t.Fatalf("expected both nodes on SingBox, got %d", len(snapSB.Nodes))
	}
}

func TestBuildCurrentArtifact_AllExcludedNonemptyInput_ReturnsNoCompatibleNodes(t *testing.T) {
	ctx := context.Background()
	svc := publication.NewService(nil, nil)

	// Only VLESS node (unsupported on Surge)
	vlessNodeID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "2.2.2.2", 443, nil)
	vlessNode := domain.Node{
		LogicalID:   vlessNodeID,
		DisplayName: "VLESS Only Node",
		Protocol:    domain.ProtocolVLESS,
		Server:      "2.2.2.2",
		Port:        443,
		Credentials: domain.InboundProtocolCredential{
			UUID: "01944aa0-0000-7000-8000-000000000001",
		},
		Active:             true,
		ConnectionRevision: 1,
	}

	rev := &domain.ConfigurationRevision{
		ID:            "rev-all-excluded",
		ContentDigest: "sha256:rev-all-excluded",
		State:         domain.RevisionStateActive,
		CreatedAt:     time.Now().UTC(),
	}

	facts := &domain.ConsistentFacts{
		Revision:               rev,
		Nodes:                  []domain.Node{vlessNode},
		EnabledSubscriptionIDs: make(map[string]bool),
		AsOf:                   time.Now().UTC(),
		PrimarySubscriptionID:  "sub-primary",
		PrimaryNodeIDs:         []string{vlessNodeID},
	}

	_, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetSurge)
	if err == nil {
		t.Fatal("expected all-excluded nonempty input to fail, got nil error")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Code != "no_compatible_nodes" {
		t.Fatalf("expected no_compatible_nodes error, got: %v", err)
	}
}

func TestBuildCurrentArtifact_NonMihomoChains_FailClosed(t *testing.T) {
	ctx := context.Background()
	svc := publication.NewService(nil, nil)

	validSSID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	validSS := domain.Node{
		LogicalID:   validSSID,
		DisplayName: "SS Chained",
		Protocol:    domain.ProtocolSS,
		Server:      "1.1.1.1",
		Port:        8388,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "secretpassword",
		},
		Active:             true,
		ConnectionRevision: 1,
	}

	rev := &domain.ConfigurationRevision{
		ID:            "rev-chain-fail",
		ContentDigest: "sha256:rev-chain-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     time.Now().UTC(),
	}

	// 1. Active chain binding in facts on non-Mihomo
	factsWithBinding := &domain.ConsistentFacts{
		Revision: rev,
		Nodes:    []domain.Node{validSS},
		ProxyChainBindings: []domain.ProxyChainBinding{
			{
				ID:         "binding-1",
				TargetType: domain.ProxyChainTargetSubscription,
				TargetID:   "sub-target-1",
				Enabled:    true,
			},
		},
		EnabledSubscriptionIDs: map[string]bool{"sub-target-1": true},
		AsOf:                   time.Now().UTC(),
	}

	for _, target := range []domain.CompilerTarget{domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX} {
		_, _, err := svc.BuildCurrentArtifact(ctx, factsWithBinding, target)
		if err == nil {
			t.Fatalf("expected active chain to fail closed for %s, got nil error", target)
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "unsupported_target_capability" {
			t.Fatalf("expected unsupported_target_capability for %s, got: %v", target, err)
		}
	}
}

// Ensure compiler.CapabilityMatrix does not panic
func TestCompilerMatrix_Preserved(t *testing.T) {
	m := compiler.CapabilityMatrix()
	if len(m) != 4 {
		t.Fatalf("expected 4 targets in CapabilityMatrix, got %d", len(m))
	}
}

func TestPreview_FactsPath_SelectionExhaustion_GlobalPASSMatchesLiveAndPureNodesFailClosed(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

	primaryNodeID1 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.1", 8388, nil)
	primaryNodeID2 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.2", 8388, nil)
	secondaryNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "1.1.1.3", 8388, nil)

	nPrimary1 := domain.Node{
		LogicalID:   primaryNodeID1,
		Protocol:    domain.ProtocolSS,
		DisplayName: "HK Primary 1",
		Server:      "1.1.1.1",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary-1",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	nPrimary2 := domain.Node{
		LogicalID:   primaryNodeID2,
		Protocol:    domain.ProtocolSS,
		DisplayName: "US Primary 2",
		Server:      "1.1.1.2",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-primary-2",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	nSecondary := domain.Node{
		LogicalID:   secondaryNodeID,
		Protocol:    domain.ProtocolSS,
		DisplayName: "SG Secondary",
		Server:      "1.1.1.3",
		Port:        8388,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-128-gcm",
			Password: "pass-secondary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()
	expectedUserInfo := "upload=100; download=200; total=1000; expire=1800000000"

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-fallback-regress",
			ContentDigest: "digest-rev-fb",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary1, nPrimary2, nSecondary},
		Groups: []domain.NodeGroup{
			{
				ID:                  groupID,
				Name:                "Normal Group",
				GroupType:           domain.GroupTypeSelect,
				InheritGlobalFilter: true,
				Sources: []domain.MemberSource{
					{
						ID:            "src-1",
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						Position:      0,
						NodeLogicalID: &primaryNodeID1,
					},
				},
			},
		},
		GlobalEmptyFallbackPass: boolPtr(true),
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-fallback-regress",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
				Enabled:       true,
			},
		},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field: domain.FilterFieldDisplayName,
					Op:    domain.FilterOpContains,
					Value: "NONEXISTENT_MATCH",
				},
			},
		},
		NodeSources: map[string][]domain.NodeSource{
			primaryNodeID1:  {{NodeLogicalID: primaryNodeID1, SubscriptionID: "sub-primary"}},
			primaryNodeID2:  {{NodeLogicalID: primaryNodeID2, SubscriptionID: "sub-primary"}},
			secondaryNodeID: {{NodeLogicalID: secondaryNodeID, SubscriptionID: "sub-secondary"}},
		},
		PrimarySubscriptionID: "sub-primary",
		PrimaryNodeIDs:        []string{primaryNodeID1, primaryNodeID2},
		PrimaryUserInfo:       expectedUserInfo,
	}

	pubRepo := newMockPublicationRepo()
	factsLoader := &memoryFactsLoader{facts: facts}
	svc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithConsistentFactsLoader(factsLoader),
	)

	// 1. Mihomo: Live artifact and Preview default must have identical bytes
	liveMihomo, liveSnapMihomo, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("BuildCurrentArtifact for Mihomo failed: %v", err)
	}

	prevMihomo, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: "rev-fallback-regress",
	})
	if err != nil {
		t.Fatalf("Preview for Mihomo failed: %v", err)
	}

	if string(prevMihomo.Content) != string(liveMihomo.Content) {
		t.Fatalf("Preview default and live content mismatch for Mihomo fallback:\nPreview:\n%s\nLive:\n%s", string(prevMihomo.Content), string(liveMihomo.Content))
	}
	if prevMihomo.ContentDigest != liveMihomo.ContentDigest {
		t.Fatalf("Preview content digest %q != live digest %q", prevMihomo.ContentDigest, liveMihomo.ContentDigest)
	}
	if prevMihomo.SnapshotDigest != liveMihomo.SnapshotDigest {
		t.Fatalf("Preview snapshot digest %q != live digest %q", prevMihomo.SnapshotDigest, liveMihomo.SnapshotDigest)
	}
	if prevMihomo.Manifest.NodeCount != 0 {
		t.Fatalf("expected no nodes after global filter, got %d", prevMihomo.Manifest.NodeCount)
	}
	var mihomoConfig struct {
		Proxies     []map[string]any `yaml:"proxies"`
		ProxyGroups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
		Rules []string `yaml:"rules"`
	}
	if err := yaml.Unmarshal(prevMihomo.Content, &mihomoConfig); err != nil {
		t.Fatalf("decode Mihomo preview: %v", err)
	}
	if len(mihomoConfig.Proxies) != 0 || len(mihomoConfig.ProxyGroups) != 1 || mihomoConfig.ProxyGroups[0].Name != "Normal Group" || !containsString(mihomoConfig.ProxyGroups[0].Proxies, "PASS") {
		t.Fatalf("expected configured route with PASS and no nodes, got %+v", mihomoConfig)
	}
	if len(mihomoConfig.Rules) != 1 || mihomoConfig.Rules[0] != "MATCH,Normal Group" {
		t.Fatalf("expected original route rule to remain, got %+v", mihomoConfig.Rules)
	}
	if prevMihomo.Manifest.RulesDigest != liveSnapMihomo.RiskDecisionDigest {
		t.Fatalf("expected rules digest %q, got %q", liveSnapMihomo.RiskDecisionDigest, prevMihomo.Manifest.RulesDigest)
	}

	// Verify draft publication was persisted
	draftPubMihomo, err := pubRepo.GetByID(ctx, prevMihomo.SnapshotID)
	if err != nil {
		t.Fatalf("failed to retrieve draft from repository: %v", err)
	}
	if draftPubMihomo.State != domain.PublicationStateDraft {
		t.Fatalf("expected draft state, got: %s", draftPubMihomo.State)
	}
	if string(draftPubMihomo.Content) != string(prevMihomo.Content) {
		t.Fatal("persisted draft content mismatch")
	}

	// 2. SingBox cannot represent the configured empty selection and must fail closed.
	if _, _, err := svc.BuildCurrentArtifact(ctx, facts, domain.TargetSingBox); err == nil {
		t.Fatal("expected SingBox live export to fail closed for globally filtered empty selection")
	}

	if _, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetSingBox,
		RevisionID: "rev-fallback-regress",
	}); err == nil {
		t.Fatal("expected SingBox preview to fail closed for globally filtered empty selection")
	}
	// 3. Explicit strict mode on fallback:
	// 3a. Mihomo explicit strict succeeds and matches live
	prevStrictMihomo, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: "rev-fallback-regress",
		CompatMode: "strict",
	})
	if err != nil {
		t.Fatalf("Preview explicit strict for Mihomo failed: %v", err)
	}
	if string(prevStrictMihomo.Content) != string(liveMihomo.Content) {
		t.Fatal("Preview explicit strict content does not match live for Mihomo")
	}

	// 3c. A filtered-out VLESS primary node is not resurrected for strict Surge preview.
	vlessPrimaryID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "2.2.2.2", 443, nil)
	nVlessPrimary := domain.Node{
		LogicalID:   vlessPrimaryID,
		Protocol:    domain.ProtocolVLESS,
		DisplayName: "VLESS Primary Unsupported on Surge",
		Server:      "2.2.2.2",
		Port:        443,
		Active:      true,
		Credentials: domain.InboundProtocolCredential{
			UUID: "01944aa0-0000-7000-8000-000000000001",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	factsSurgeFallback := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-fallback-surge",
			ContentDigest: "sha256:rev-fallback-surge",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nVlessPrimary},
		GlobalFilter: &domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "NONEXISTENT_MATCH"},
			},
		},
		NodeSources: map[string][]domain.NodeSource{
			vlessPrimaryID: {{NodeLogicalID: vlessPrimaryID, SubscriptionID: "sub-primary-surge"}},
		},
		PrimarySubscriptionID: "sub-primary-surge",
		PrimaryNodeIDs:        []string{vlessPrimaryID},
	}

	svcSurge := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithConsistentFactsLoader(&memoryFactsLoader{facts: factsSurgeFallback}),
	)

	if _, err := svcSurge.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetSurge,
		RevisionID: "rev-fallback-surge",
		CompatMode: "strict",
	}); err == nil {
		t.Fatal("expected strict Surge preview to fail closed rather than resurrect filtered primary nodes")
	}

	// 3d. Explicit strict mode on normal inventory with incompatible node (no fallback):
	// Surge explicit strict rejects mixed inventory with VLESS, compatible accepts and excludes VLESS
	factsMixedNormal := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-mixed-normal",
			ContentDigest: "sha256:rev-mixed-normal",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{nPrimary1, nVlessPrimary},
	}
	svcMixed := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithConsistentFactsLoader(&memoryFactsLoader{facts: factsMixedNormal}),
	)

	// Explicit strict on mixed inventory
	_, err = svcMixed.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetSurge,
		RevisionID: "rev-mixed-normal",
		CompatMode: "strict",
	})
	if err == nil {
		t.Fatal("expected Surge explicit strict to fail for mixed inventory with VLESS")
	}
	var strictErrMixed *publication.StrictCapabilityError
	if !errors.As(err, &strictErrMixed) {
		t.Fatalf("expected *StrictCapabilityError for mixed inventory, got: %v", err)
	}
	if !strings.HasPrefix(strictErrMixed.SnapshotID, "snapshot_") {
		t.Fatalf("expected valid draft ID, got: %s", strictErrMixed.SnapshotID)
	}

	// Default/compatible on mixed inventory succeeds and excludes VLESS
	prevCompatSurge, err := svcMixed.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetSurge,
		RevisionID: "rev-mixed-normal",
	})
	if err != nil {
		t.Fatalf("expected Surge default preview to succeed on mixed inventory, got: %v", err)
	}
	if prevCompatSurge.Manifest.NodeCount != 1 {
		t.Fatalf("expected 1 compatible node in manifest, got %d", prevCompatSurge.Manifest.NodeCount)
	}
	if prevCompatSurge.Manifest.ExcludedCount != 1 {
		t.Fatalf("expected 1 excluded node in manifest, got %d", prevCompatSurge.Manifest.ExcludedCount)
	}
	if len(prevCompatSurge.Manifest.Excluded) != 1 || prevCompatSurge.Manifest.Excluded[0].NodeID != vlessPrimaryID {
		t.Fatalf("expected VLESS node to be excluded, got: %+v", prevCompatSurge.Manifest.Excluded)
	}
}
