package publication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
)

func TestPublicationFrozenArtifactCannotBeReplacedByRecompilation(t *testing.T) {
	for _, scenario := range []string{"missing", "corrupt", "frozen-without-manifest", "frozen"} {
		t.Run(scenario, func(t *testing.T) {
			content := []byte("frozen synthetic bytes")
			sum := sha256.Sum256(content)
			pub := &domain.Publication{ID: "pub", Target: domain.TargetMihomo, State: domain.PublicationStateActive,
				Content: content, ContentDigest: hex.EncodeToString(sum[:]), SnapshotDigest: "original-snapshot"}
			wantError := ""
			switch scenario {
			case "missing":
				pub.Content = nil
				wantError = "publication_artifact_unavailable"
			case "corrupt":
				pub.Content = []byte("changed bytes")
				wantError = "publication_content_integrity_failed"
			case "frozen":
				pub.Manifest = &domain.PublicationManifest{Excluded: []domain.ManifestExcludedNode{{NodeID: "excluded", Code: "unsupported_target_capability"}}}
			}
			repo := newMockPublicationRepo()
			if err := repo.Create(context.Background(), pub); err != nil {
				t.Fatal(err)
			}
			svc := publication.NewService(repo, nil)
			artifact, err := svc.ResolveAndServeAuthorized(context.Background(), pub.ID, func(string) bool { return true })
			if wantError != "" {
				domainErr, ok := domain.AsDomainError(err)
				if !ok || domainErr.Code != wantError || artifact != nil {
					t.Fatalf("expected %s without replacement bytes: %+v %v", wantError, artifact, err)
				}
				return
			}
			if err != nil || string(artifact.Content) != string(content) || artifact.ContentDigest != pub.ContentDigest || artifact.SnapshotDigest != pub.SnapshotDigest {
				t.Fatalf("frozen publication changed: %+v %v", artifact, err)
			}
			if scenario == "frozen" && len(artifact.ExcludedNodes) != 1 {
				t.Fatal("frozen manifest lost")
			}
		})
	}
}
