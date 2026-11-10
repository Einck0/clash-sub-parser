package sqlite_test

import (
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestGenerationReplacementPreservesImmutableOriginsPinsAndUnknownObservation(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	subID := "observation-sub"
	retentionCreateTestSubscription(t, db, subID, "Synthetic")
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	retentionInsertTestFetch(t, db, "observation-fetch", subID, now, domain.FetchOutcomeSuccess)
	retentionInsertTestPayload(t, db, "observation-payload", subID, "observation-fetch", now, true)
	if _, err := db.ExecContext(ctx, `INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision)VALUES('observation-node','ss','Renamed','192.0.2.1',8388,'{}',1,?,?,1)`, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	nodeID := "observation-node"
	retentionInsertTestEntry(t, db, "observation-entry", "observation-payload", subID, "synthetic-key", "Original Raw Name", domain.EntryKindProxy, &nodeID)
	if _, err := db.ExecContext(ctx, `INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,source_entry_id,created_at)VALUES(?,1,'{}','synthetic','observation-entry',?)`, nodeID, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	rev := &domain.ConfigurationRevision{ID: "frozen-facts-revision", State: domain.RevisionStateActive, ContentDigest: "synthetic", CreatedAt: now}
	if err := sqlite.NewRevisionRepository(db).Create(ctx, rev); err != nil {
		t.Fatal(err)
	}
	facts, err := sqlite.NewConsistentFactsLoader(db).LoadConsistentFacts(ctx, rev.ID, domain.TargetSingBox, now)
	if err != nil {
		t.Fatal(err)
	}
	frozen := facts.NodePayloadEvidence[nodeID]
	if len(frozen) != 1 || frozen[0].PayloadID != "observation-payload" || frozen[0].ContentDigest != "sha256:digest" {
		t.Fatalf("facts did not capture original evidence: %+v", frozen)
	}
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error { return sqlite.ReplaceCurrentPayloadTx(ctx, tx, subID) }); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE nodes SET connection_revision=2 WHERE logical_id=?`, nodeID); err != nil {
		t.Fatal(err)
	}
	if len(facts.NodePayloadEvidence[nodeID]) != 1 || facts.NodePayloadEvidence[nodeID][0].PayloadID != "observation-payload" || facts.NodePayloadEvidence[nodeID][0].ContentDigest != "sha256:digest" {
		t.Fatalf("previous facts changed after refresh/head update: %+v", facts.NodePayloadEvidence[nodeID])
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_payloads`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("old full generation retained: %d %v", count, err)
	}
	var origin, digest, rawName, state, cause string
	if err := db.QueryRowContext(ctx, `SELECT p.origin_payload_id,e.content_digest FROM node_connection_provenance p JOIN subscription_payload_evidence e ON e.payload_id=p.origin_payload_id WHERE p.node_logical_id=? AND p.connection_revision=1`, nodeID).Scan(&origin, &digest); err != nil || origin != "observation-payload" || digest != "sha256:digest" {
		t.Fatalf("immutable origin lost: %s %s %v", origin, digest, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT raw_name FROM node_source_observations WHERE node_logical_id=?`, nodeID).Scan(&rawName); err != nil || rawName != "Original Raw Name" {
		t.Fatalf("historical matching raw name lost: %s %v", rawName, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT relation_state,cause FROM node_source_history WHERE node_logical_id=?`, nodeID).Scan(&state, &cause); err != nil || state != "unknown" || cause != "unresolved" {
		t.Fatalf("entry observation must not invent validation/removal: %s %s %v", state, cause, err)
	}
	// Historical ID pin operations and metadata reads work without a payload shell.
	payloads := sqlite.NewSubscriptionPayloadRepository(db)
	p, err := payloads.GetByID(ctx, "observation-payload")
	if err != nil || p == nil || !p.Pinned || len(p.BodyBlob) != 0 {
		t.Fatalf("pinned evidence lost: %v", err)
	}
	if err := payloads.Pin(ctx, p.ID, false); err != nil {
		t.Fatal(err)
	}
	var source sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT source_entry_id FROM node_connection_versions WHERE node_logical_id=?`, nodeID).Scan(&source); err != nil || source.Valid {
		t.Fatalf("old entry not detached: %v", err)
	}
}

func TestPublicationFrozenEvidenceAtomicConflictRollback(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	rev := &domain.ConfigurationRevision{ID: "evidence-revision", State: domain.RevisionStateActive, ContentDigest: "synthetic", CreatedAt: domain.NowUTC()}
	if err := sqlite.NewRevisionRepository(db).Create(ctx, rev); err != nil {
		t.Fatal(err)
	}
	writer := sqlite.NewPublicationRepository(db).(domain.PublicationEvidenceWriter)
	evidence := domain.PayloadEvidence{PayloadID: "historical-id", SourceIdentity: "sub:deleted", FetchID: "old-fetch", ContentDigest: "old-digest", OriginalCreatedAt: "2026-11-10T00:00:00Z", CapturedAt: "2026-11-10T00:00:00Z", HTTPStatus: 200, EvidenceKind: "payload_origin", Quality: "observed"}
	pub := &domain.Publication{ID: "evidence-pub", RevisionID: rev.ID, Target: domain.TargetSingBox, Content: []byte("immutable synthetic bytes"), ContentDigest: "synthetic", TokenHash: "draft:synthetic", State: domain.PublicationStateDraft, CreatedAt: domain.NowUTC()}
	if err := writer.CreateWithEvidence(ctx, pub, []domain.PayloadEvidence{evidence}); err != nil {
		t.Fatal(err)
	}
	conflict := evidence
	conflict.ContentDigest = "new-digest"
	second := *pub
	second.ID = "failed-evidence-pub"
	second.TokenHash = "draft:failed"
	if err := writer.CreateWithEvidence(ctx, &second, []domain.PayloadEvidence{conflict}); err == nil {
		t.Fatal("conflicting origin must fail whole draft transaction")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM publications WHERE id=?`, second.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed refs left successful draft: %d %v", count, err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_evidence_ref BEFORE INSERT ON publication_payload_refs BEGIN SELECT RAISE(ABORT,'synthetic ref failure'); END`); err != nil {
		t.Fatal(err)
	}
	second.ID = "failed-ref-pub"
	if err := writer.CreateWithEvidence(ctx, &second, []domain.PayloadEvidence{evidence}); err == nil {
		t.Fatal("reference write error was ignored")
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM publications WHERE id=?`, second.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("ref failure left a draft: %d %v", count, err)
	}
}
