package publication_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
)

type memorySubscriptionLinkRepo struct {
	links map[string]*domain.SubscriptionLink
}

func newMemorySubscriptionLinkRepo() *memorySubscriptionLinkRepo {
	return &memorySubscriptionLinkRepo{links: make(map[string]*domain.SubscriptionLink)}
}

func (r *memorySubscriptionLinkRepo) Create(ctx context.Context, link *domain.SubscriptionLink) error {
	if link.IsDefault && link.State == domain.SubscriptionLinkStateActive {
		for _, l := range r.links {
			if l.Target == link.Target && l.IsDefault && l.State == domain.SubscriptionLinkStateActive {
				*link = *l
				return nil
			}
		}
	}
	r.links[link.ID] = link
	return nil
}

func (r *memorySubscriptionLinkRepo) GetByID(ctx context.Context, id string) (*domain.SubscriptionLink, error) {
	l, ok := r.links[id]
	if !ok {
		return nil, domain.NewNotFoundError("not_found", "link not found")
	}
	return l, nil
}

func (r *memorySubscriptionLinkRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.SubscriptionLink, error) {
	for _, l := range r.links {
		if l.TokenHash == tokenHash && l.State == domain.SubscriptionLinkStateActive {
			return l, nil
		}
	}
	return nil, domain.NewNotFoundError("not_found", "link not found")
}

func (r *memorySubscriptionLinkRepo) GetDefaultByTarget(ctx context.Context, target domain.CompilerTarget) (*domain.SubscriptionLink, error) {
	for _, l := range r.links {
		if l.Target == target && l.IsDefault && l.State == domain.SubscriptionLinkStateActive {
			return l, nil
		}
	}
	return nil, domain.NewNotFoundError("not_found", "default link not found")
}

func (r *memorySubscriptionLinkRepo) List(ctx context.Context) ([]domain.SubscriptionLink, error) {
	res := make([]domain.SubscriptionLink, 0, len(r.links))
	for _, l := range r.links {
		res = append(res, *l)
	}
	return res, nil
}

func (r *memorySubscriptionLinkRepo) UpdateLabel(ctx context.Context, id string, label string, updatedAt time.Time) error {
	l, ok := r.links[id]
	if !ok {
		return domain.NewNotFoundError("not_found", "link not found")
	}
	l.Label = label
	l.UpdatedAt = updatedAt
	return nil
}

func (r *memorySubscriptionLinkRepo) RotateToken(ctx context.Context, id string, newToken, newTokenHash string, updatedAt time.Time) error {
	l, ok := r.links[id]
	if !ok {
		return domain.NewNotFoundError("not_found", "link not found")
	}
	if !l.IsActive() {
		return domain.NewConflictError("revoked", "link is revoked")
	}
	l.Token = newToken
	l.TokenHash = newTokenHash
	l.UpdatedAt = updatedAt
	return nil
}

func (r *memorySubscriptionLinkRepo) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	l, ok := r.links[id]
	if !ok {
		return domain.NewNotFoundError("not_found", "link not found")
	}
	return l.Revoke(revokedAt)
}

type memoryFactsLoader struct {
	facts *domain.ConsistentFacts
	err   error
}

func (m *memoryFactsLoader) LoadConsistentFacts(ctx context.Context, revisionID string, target domain.CompilerTarget, asOf time.Time) (*domain.ConsistentFacts, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.facts, nil
}

func TestBuildCurrentArtifact_HermeticAndDeterministic(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	svc := publication.NewService(nil, nil)
	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()

	nodeID := "node_0123456789abcdef"

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:            "rev-1",
			ContentDigest: "digest-rev-1",
			State:         domain.RevisionStateActive,
			CreatedAt:     now,
		},
		Nodes: []domain.Node{
			{
				LogicalID:   nodeID,
				Protocol:    domain.ProtocolSS,
				DisplayName: "HK Server",
				Server:      "1.1.1.1",
				Port:        8388,
				Active:      true,
				Credentials: domain.InboundProtocolCredential{
					Method:   "aes-128-gcm",
					Password: "secret-password",
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		Groups: []domain.NodeGroup{
			{
				ID:        groupID,
				Name:      "Proxy",
				GroupType: domain.GroupTypeSelect,
				Sources: []domain.MemberSource{
					{
						ID:            "src-1",
						GroupID:       groupID,
						Operation:     domain.OperationAdd,
						Kind:          domain.SourceKindManualNode,
						NodeLogicalID: ptr(nodeID),
						Position:      0,
					},
				},
			},
		},
		GroupSources: map[string][]domain.MemberSource{
			groupID: {
				{
					ID:            "src-1",
					GroupID:       groupID,
					Operation:     domain.OperationAdd,
					Kind:          domain.SourceKindManualNode,
					NodeLogicalID: ptr(nodeID),
					Position:      0,
				},
			},
		},
		GroupEdges: map[string][]domain.GroupEdge{
			groupID: {
				{
					ID:            "edge-1",
					ParentGroupID: groupID,
					NodeLogicalID: ptr(nodeID),
					Position:      0,
				},
			},
		},
		PolicyRules: []domain.PolicyRule{
			{
				ID:            ruleID,
				RevisionID:    "rev-1",
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
			},
		},
	}

	// 1. Build Mihomo artifact
	artMihomo, snapMihomo, err := svc.BuildCurrentArtifact(context.Background(), facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("BuildCurrentArtifact(Mihomo) failed: %v", err)
	}
	if artMihomo.Target != domain.TargetMihomo || len(artMihomo.Content) == 0 {
		t.Fatalf("unexpected Mihomo artifact: %+v", artMihomo)
	}
	if len(snapMihomo.Nodes) != 1 || len(snapMihomo.Groups) != 1 {
		t.Fatalf("unexpected snapshot counts: nodes=%d groups=%d", len(snapMihomo.Nodes), len(snapMihomo.Groups))
	}

	// Determinism check: building again from same facts produces identical ContentDigest
	artMihomo2, _, err := svc.BuildCurrentArtifact(context.Background(), facts, domain.TargetMihomo)
	if err != nil {
		t.Fatalf("second build failed: %v", err)
	}
	if artMihomo.ContentDigest != artMihomo2.ContentDigest {
		t.Fatalf("build not deterministic: %s vs %s", artMihomo.ContentDigest, artMihomo2.ContentDigest)
	}

	// 2. Build SingBox artifact (pure nodes)
	artSingBox, snapSingBox, err := svc.BuildCurrentArtifact(context.Background(), facts, domain.TargetSingBox)
	if err != nil {
		t.Fatalf("BuildCurrentArtifact(SingBox) failed: %v", err)
	}
	if artSingBox.Target != domain.TargetSingBox || len(artSingBox.Content) == 0 {
		t.Fatalf("unexpected SingBox artifact: %+v", artSingBox)
	}
	if len(snapSingBox.Nodes) != 1 {
		t.Fatalf("expected 1 node in SingBox snapshot, got %d", len(snapSingBox.Nodes))
	}
}

func TestBuildCurrentArtifact_EmptyExportableNodesReturnsPreflightError(t *testing.T) {
	now := time.Now().UTC()
	svc := publication.NewService(nil, nil)

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:        "rev-empty",
			State:     domain.RevisionStateActive,
			CreatedAt: now,
		},
		Nodes: []domain.Node{}, // 0 nodes
	}

	_, _, err := svc.BuildCurrentArtifact(context.Background(), facts, domain.TargetSingBox)
	if err == nil {
		t.Fatal("expected preflight error for empty exportable nodes")
	}
}

func TestGenerateLiveArtifact_And_CheckSubscriptionLink(t *testing.T) {
	now := time.Now().UTC()
	linkRepo := newMemorySubscriptionLinkRepo()
	link := &domain.SubscriptionLink{
		ID:        "link-live-1",
		Label:     "Live Link",
		Target:    domain.TargetSingBox,
		Token:     "pub_valid_token_1234567890abcdef",
		TokenHash: publication.HashToken("pub_valid_token_1234567890abcdef"),
		State:     domain.SubscriptionLinkStateActive,
		IsDefault: true,
	}
	_ = linkRepo.Create(context.Background(), link)

	facts := &domain.ConsistentFacts{
		AsOf: now,
		Revision: &domain.ConfigurationRevision{
			ID:        "rev-live-1",
			State:     domain.RevisionStateActive,
			CreatedAt: now,
		},
		Nodes: []domain.Node{
			{
				LogicalID:   "node_0123456789abcdee",
				Protocol:    domain.ProtocolSS,
				DisplayName: "US Server",
				Server:      "2.2.2.2",
				Port:        8388,
				Active:      true,
				Credentials: domain.InboundProtocolCredential{
					Method:   "aes-128-gcm",
					Password: "p",
				},
			},
		},
	}
	loader := &memoryFactsLoader{facts: facts}

	svc := publication.NewService(
		nil, nil,
		publication.WithSubscriptionLinkRepository(linkRepo),
		publication.WithConsistentFactsLoader(loader),
	)

	// 1. Generate with valid token
	art, err := svc.GenerateLiveArtifact(context.Background(), "link-live-1", "pub_valid_token_1234567890abcdef")
	if err != nil {
		t.Fatalf("GenerateLiveArtifact failed: %v", err)
	}
	if art.Target != domain.TargetSingBox || len(art.Content) == 0 {
		t.Fatalf("unexpected artifact: %+v", art)
	}

	// 2. Generate with invalid token -> unauthorized
	_, err = svc.GenerateLiveArtifact(context.Background(), "link-live-1", "wrong_token")
	if err != publication.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}

	// 3. Generate on non-existent link -> not found
	_, err = svc.GenerateLiveArtifact(context.Background(), "non-existent", "pub_valid_token_1234567890abcdef")
	if err != publication.ErrLinkNotFound {
		t.Fatalf("expected ErrLinkNotFound, got %v", err)
	}

	// 4. Check subscription link (diagnostic query)
	checkRes, err := svc.CheckSubscriptionLink(context.Background(), "link-live-1")
	if err != nil {
		t.Fatalf("CheckSubscriptionLink failed: %v", err)
	}
	if !checkRes.Allowed || checkRes.NodeCount != 1 || checkRes.Target != domain.TargetSingBox {
		t.Fatalf("unexpected check result: %+v", checkRes)
	}

	// 5. Revoke link -> subsequent generate and check must return ErrLinkRevoked
	_ = linkRepo.Revoke(context.Background(), "link-live-1", time.Now().UTC())

	_, err = svc.GenerateLiveArtifact(context.Background(), "link-live-1", "pub_valid_token_1234567890abcdef")
	if err != publication.ErrLinkRevoked {
		t.Fatalf("expected ErrLinkRevoked, got %v", err)
	}

	_, err = svc.CheckSubscriptionLink(context.Background(), "link-live-1")
	if err != publication.ErrLinkRevoked {
		t.Fatalf("expected ErrLinkRevoked on check, got %v", err)
	}
}

func TestBuildCurrentArtifact_TimeAdvancementTTL_ObservationExpired(t *testing.T) {
	t0 := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	nodeID := "node_0123456789abcdef"
	revUUID := domain.MustNewUUIDv7()

	ipriskSvc := iprisk.NewService(nil, nil)

	svc := publication.NewService(
		nil, nil,
		publication.WithIPRiskService(ipriskSvc),
	)

	obs := makeTestObservation(nodeID, 10, 90)
	obs.ObservedAt = t0
	obs.ExpiresAt = t0.Add(1 * time.Hour)
	obs.Provider = "ipinfo"
	obs.ProviderSchemaVersion = "v1"

	policyRev := makeTestRiskPolicy(revUUID, domain.RiskActionBlock)
	policyRev.RiskPolicy.UnknownAction = domain.RiskActionBlock

	facts := &domain.ConsistentFacts{
		AsOf: t0.Add(10 * time.Minute),
		Revision: &domain.ConfigurationRevision{
			ID:        "rev-ttl-1",
			State:     domain.RevisionStateActive,
			CreatedAt: t0,
		},
		Nodes: []domain.Node{
			{
				LogicalID:   nodeID,
				Protocol:    domain.ProtocolSS,
				DisplayName: "TTL Test Node",
				Server:      "1.2.3.4",
				Port:        8388,
				Active:      true,
				Credentials: domain.InboundProtocolCredential{
					Method:   "aes-128-gcm",
					Password: "p",
				},
			},
		},
		ActiveRiskPolicy: policyRev,
		RiskObservations: []domain.IPRiskObservation{obs},
	}

	// 1. Fresh time: BuildCurrentArtifact MUST succeed
	artFresh, _, err := svc.BuildCurrentArtifact(context.Background(), facts, domain.TargetSingBox)
	if err != nil {
		t.Fatalf("expected successful build when observation is fresh: %v", err)
	}
	if len(artFresh.Content) == 0 {
		t.Fatal("expected non-empty artifact content")
	}

	// 2. Time advancement without any database write:
	// Advance AsOf to t0 + 2 hours (1 hour past ExpiresAt)
	factsExpired := *facts
	factsExpired.AsOf = t0.Add(2 * time.Hour)

	_, _, errExpired := svc.BuildCurrentArtifact(context.Background(), &factsExpired, domain.TargetSingBox)
	if errExpired == nil {
		t.Fatal("expected BuildCurrentArtifact to fail closed when observation is expired by time advancement alone")
	}
}
