package publication_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

func TestEmptyInventoryPreflightAndPreviewRejectWithoutRoutedPass(t *testing.T) {
	for _, target := range []domain.CompilerTarget{domain.TargetMihomo, domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX} {
		t.Run(string(target), func(t *testing.T) {
			svc := publication.NewService(newMockPublicationRepo(), nil)
			snapshot := &resolver.ResolvedPolicySnapshot{RevisionID: "empty-revision"}
			preflight, err := svc.Preflight(context.Background(), publication.PreflightCommand{Target: target, Snapshot: snapshot})
			if err != nil || preflight.Allowed {
				t.Fatalf("empty inventory preflight accepted: %+v %v", preflight, err)
			}
			if len(preflight.Diagnostics) != 1 || preflight.Diagnostics[0].Code != "filtered_nodes_empty" {
				t.Fatalf("missing empty inventory diagnostic: %+v", preflight.Diagnostics)
			}
			_, err = svc.Preview(context.Background(), publication.PreviewQuery{Target: target, Snapshot: snapshot})
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "filtered_nodes_empty" {
				t.Fatalf("empty inventory preview did not reject consistently: %v", err)
			}
		})
	}
}
