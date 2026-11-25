package sqlite_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestPublicationManifestRoundTripAndActivation(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewPublicationRepository(db)
	want := &domain.PublicationManifest{
		NodeCount: 1, ExcludedCount: 1,
		PayloadIDs: []string{"origin-1"}, ConfigurationRevision: "revision-1", RulesDigest: "rules-digest",
		Included:            []domain.ManifestIncludedNode{{NodeID: "compatible-node", ConnectionRevision: 3}},
		Excluded:            []domain.ManifestExcludedNode{{NodeID: "incompatible-node", DisplayName: "Legacy node · 香港", Code: "unsupported_target_capability", Reason: "Mihomo does not support this transport parameter"}},
		TargetEngineVersion: "1.0.0", MappingVersion: "1.0.0",
	}
	pub := &domain.Publication{
		ID: "manifest-pub", Target: domain.TargetMihomo, SnapshotDigest: "snapshot-digest",
		Content: []byte("synthetic immutable artifact"), ContentDigest: "content-digest",
		CompilerVersion: "1.0.0", TokenHash: "draft:manifest-pub", State: domain.PublicationStateDraft,
		Manifest: want, CreatedAt: domain.NowUTC(),
	}
	evidence := domain.PayloadEvidence{PayloadID: "origin-1", SourceIdentity: "sub:synthetic", FetchID: "fetch-1", ContentDigest: "origin-digest", OriginalCreatedAt: "2026-11-15T00:00:00Z", CapturedAt: "2026-11-15T00:00:00Z", HTTPStatus: 200, EvidenceKind: "payload_origin", Quality: "observed"}
	if err := repo.(domain.PublicationEvidenceWriter).CreateWithEvidence(ctx, pub, []domain.PayloadEvidence{evidence}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, pub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Manifest, want) {
		t.Fatalf("draft manifest changed: got %+v, want %+v", got.Manifest, want)
	}
	got.Manifest.Excluded[0].Reason = "caller mutation"
	if err := repo.Activate(ctx, pub.ID, "active-manifest-token-hash"); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetByTokenHash(ctx, "active-manifest-token-hash")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Manifest, want) || string(got.Content) != string(pub.Content) || !got.IsActive() {
		t.Fatalf("activation changed frozen publication: %+v", got)
	}
	if err := repo.Revoke(ctx, pub.ID, domain.NowUTC()); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetByID(ctx, pub.ID)
	if err != nil || !reflect.DeepEqual(got.Manifest, want) || string(got.Content) != string(pub.Content) {
		t.Fatalf("revocation changed frozen artifact or manifest: %+v, %v", got, err)
	}
}

func TestPublicationManifestMigrationPreservesHistoricPublication(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.RunUpTo(ctx, 31); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO publications (id,target,snapshot_digest,compiler_version,token_hash,state,created_at,content) VALUES ('historic','clash','old-snapshot','old-compiler','old-token-hash','active','2026-11-01T00:00:00Z',?)`, []byte("historic immutable content")); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewPublicationRepository(db)
	got, err := repo.GetByTokenHash(ctx, "old-token-hash")
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest != nil || string(got.Content) != "historic immutable content" || string(got.Target) != "clash" || got.SnapshotDigest != "old-snapshot" || !got.IsActive() {
		t.Fatalf("migration changed historical publication: %+v", got)
	}
	// Callers that do not have a manifest can still save/read a publication.
	fresh := &domain.Publication{ID: "without-manifest", Target: domain.TargetMihomo, TokenHash: "without-manifest-token", State: domain.PublicationStateDraft}
	if err := repo.Create(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetByID(ctx, fresh.ID)
	if err != nil || got.Manifest != nil {
		t.Fatalf("missing manifest must remain explicitly unknown: %+v, %v", got, err)
	}
}

func TestPublicationManifestDecodeErrorDoesNotExposeStoredJSON(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	repo := sqlite.NewPublicationRepository(db)
	pub := &domain.Publication{ID: "damaged-manifest", Target: domain.TargetMihomo, TokenHash: "damaged-manifest-token", State: domain.PublicationStateDraft}
	if err := repo.Create(ctx, pub); err != nil {
		t.Fatal(err)
	}
	// Synthetic sentinel is intentionally not a real credential.
	if _, err := db.ExecContext(ctx, `UPDATE publications SET manifest_json=? WHERE id=?`, `{"node_count":"sensitive-sentinel"}`, pub.ID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, pub.ID)
	if err == nil || got != nil || !strings.Contains(err.Error(), "decode publication manifest") || strings.Contains(err.Error(), "sensitive-sentinel") {
		t.Fatalf("damaged manifest must fail explicitly without raw JSON: %+v, %v", got, err)
	}
}
