package publication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// mockRiskPolicyRepo implements domain.RiskPolicyRevisionRepository in-memory for testing.
type mockRiskPolicyRepo struct {
	policies map[string]*domain.RiskPolicyRevision
	activeID string
}

func newMockRiskPolicyRepo() *mockRiskPolicyRepo {
	return &mockRiskPolicyRepo{
		policies: make(map[string]*domain.RiskPolicyRevision),
	}
}

func (m *mockRiskPolicyRepo) GetByID(ctx context.Context, id string) (*domain.RiskPolicyRevision, error) {
	p, ok := m.policies[id]
	if !ok {
		return nil, domain.NewNotFoundError("risk_policy_not_found", "policy revision not found")
	}
	copy := *p
	return &copy, nil
}

func (m *mockRiskPolicyRepo) GetActive(ctx context.Context) (*domain.RiskPolicyRevision, error) {
	if m.activeID == "" {
		return nil, domain.NewNotFoundError("no_active_risk_policy", "no active risk policy revision")
	}
	return m.GetByID(ctx, m.activeID)
}

func (m *mockRiskPolicyRepo) Create(ctx context.Context, rev *domain.RiskPolicyRevision) error {
	copy := *rev
	m.policies[rev.RevisionID] = &copy
	if rev.Active {
		m.activeID = rev.RevisionID
	}
	return nil
}

func (m *mockRiskPolicyRepo) SetActive(ctx context.Context, id string, active bool) error {
	p, ok := m.policies[id]
	if !ok {
		return domain.NewNotFoundError("risk_policy_not_found", "policy revision not found")
	}
	p.Active = active
	if active {
		m.activeID = id
	} else if m.activeID == id {
		m.activeID = ""
	}
	return nil
}

func (m *mockRiskPolicyRepo) List(ctx context.Context, filter domain.RiskPolicyFilter) ([]domain.RiskPolicyRevision, int, error) {
	res := make([]domain.RiskPolicyRevision, 0, len(m.policies))
	for _, p := range m.policies {
		if filter.Active != nil && p.Active != *filter.Active {
			continue
		}
		res = append(res, *p)
	}
	return res, len(res), nil
}

// mockRiskObsRepo implements domain.IPRiskObservationRepository in-memory for testing.
type mockRiskObsRepo struct {
	observations []domain.IPRiskObservation
}

func newMockRiskObsRepo() *mockRiskObsRepo {
	return &mockRiskObsRepo{
		observations: make([]domain.IPRiskObservation, 0),
	}
}

func (m *mockRiskObsRepo) GetByID(ctx context.Context, id string) (*domain.IPRiskObservation, error) {
	for _, o := range m.observations {
		if o.ID == id {
			copy := o
			return &copy, nil
		}
	}
	return nil, domain.NewNotFoundError("observation_not_found", "observation not found")
}

func (m *mockRiskObsRepo) List(ctx context.Context, filter domain.IPRiskObservationFilter) ([]domain.IPRiskObservation, int, error) {
	res := make([]domain.IPRiskObservation, 0)
	for _, o := range m.observations {
		if filter.NodeLogicalID != "" && o.NodeLogicalID != filter.NodeLogicalID {
			continue
		}
		res = append(res, o)
	}
	return res, len(res), nil
}

func (m *mockRiskObsRepo) Create(ctx context.Context, obs *domain.IPRiskObservation) error {
	m.observations = append(m.observations, *obs)
	return nil
}

func makeTestRiskPolicy(revisionID string, reviewAction domain.RiskAction) *domain.RiskPolicyRevision {
	if revisionID == "" {
		revisionID = domain.MustNewUUIDv7()
	}
	return &domain.RiskPolicyRevision{
		RiskPolicy: domain.RiskPolicy{
			RevisionID: revisionID,
			ProviderSelection: domain.RiskProviderSelection{
				Mode: domain.RiskFusionSingleProvider,
				Providers: []domain.ProviderRef{
					{Provider: "ipinfo", SchemaVersion: "v1"},
				},
			},
			MaxObservationAge: 24 * time.Hour,
			MinimumConfidence: 50,
			ScoreBands: []domain.ScoreBand{
				{Min: 0, Max: 25, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
				{Min: 26, Max: 60, Band: domain.RiskBandMedium, Action: domain.RiskActionReview},
				{Min: 61, Max: 85, Band: domain.RiskBandHigh, Action: domain.RiskActionBlock},
				{Min: 86, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
			},
			UnknownAction:  domain.RiskActionReview,
			ConflictAction: domain.RiskActionReview,
			ReviewAction:   reviewAction,
		},
		Active:    true,
		CreatedAt: time.Now().UTC(),
	}
}

func makeTestObservation(nodeID string, score, confidence int) domain.IPRiskObservation {
	id := domain.MustNewUUIDv7()
	scorePtr := &score
	confPtr := &confidence
	hexDigest := strings.Repeat("a", 64)
	h := sha256.Sum256([]byte(id + "ipinfo" + "v1"))
	now := time.Now().UTC()
	return domain.IPRiskObservation{
		ID:                    id,
		NodeLogicalID:         nodeID,
		ExitIdentityDigest:    "sha256:" + hexDigest,
		Provider:              "ipinfo",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(12 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 scorePtr,
		Confidence:            confPtr,
		NetworkClass:          domain.NetworkClassResidential,
		EvidenceDigest:        "sha256:" + hex.EncodeToString(h[:]),
		RedactedSummary:       "observation clean summary",
	}
}

func buildSnapshotWithValidNodes(hkID, usID string) *resolver.ResolvedPolicySnapshot {
	nodes := []resolver.ResolvedNode{
		{LogicalID: hkID, DisplayName: "Hong Kong 01", Protocol: domain.ProtocolTrojan, Active: true, Position: 0},
		{LogicalID: usID, DisplayName: "United States 01", Protocol: domain.ProtocolSS, Active: true, Position: 1},
	}
	groups := []resolver.ResolvedGroup{
		{
			ID:                "group-select",
			Name:              "ProxySelect",
			GroupType:         domain.GroupTypeSelect,
			NodeLogicalIDs:    []string{hkID, usID},
			AllNodeLogicalIDs: []string{hkID, usID},
			Position:          0,
		},
	}
	rules := []resolver.ResolvedRule{
		{ID: "rule-1", TargetGroupID: "group-select", TargetGroupName: "ProxySelect", Expression: "DOMAIN-SUFFIX,google.com", Position: 0},
		{ID: "rule-match", TargetGroupID: "group-select", TargetGroupName: "ProxySelect", Expression: "MATCH", Position: 1, IsTerminal: true},
	}
	raw := "Hong Kong 01|" + hkID + "|trojan|United States 01|" + usID + "|ss|ProxySelect|select|DOMAIN-SUFFIX,google.com|MATCH"
	sum := sha256.Sum256([]byte(raw))
	snapDigest := hex.EncodeToString(sum[:])
	return &resolver.ResolvedPolicySnapshot{
		InputDigest:     "sha256:input-sample",
		SnapshotDigest:  snapDigest,
		CompilerVersion: "1.0.0",
		Nodes:           nodes,
		NodeLogicalIDs:  []string{hkID, usID},
		Groups:          groups,
		Rules:           rules,
		DNS:             resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1", "8.8.8.8"}},
		ResolvedAt:      time.Now().UTC(),
	}
}

func TestPreflightStandaloneDiagnosticAllowed(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()
	snap := buildSampleSnapshot()

	cmd := publication.PreflightCommand{
		Target:   domain.TargetClash,
		Snapshot: snap,
	}

	res, err := svc.Preflight(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected preflight to be allowed, got diagnostics: %+v", res.Diagnostics)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(res.Diagnostics))
	}
}

func TestPreflightStandaloneDiagnosticRejection(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()
	snap := buildSampleSnapshot()
	snap.RiskPolicyRevision = "0191e4a0-0000-7000-8000-000000000001"
	snap.Diagnostics = []resolver.Diagnostic{{
		Severity: resolver.DiagnosticSeverityWarning,
		Code:     "risk_blocked",
		Message:  "node node-hk-01 was excluded by risk policy",
		Target:   "node-hk-01",
	}}

	cmd := publication.PreflightCommand{
		Target:   domain.TargetClash,
		Snapshot: snap,
	}

	res, err := svc.Preflight(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if res.Allowed {
		t.Fatal("expected preflight to be rejected (allowed=false)")
	}
	if len(res.Diagnostics) != 1 || res.Diagnostics[0].Code != "risk_blocked" {
		t.Fatalf("expected 1 risk_blocked diagnostic, got %+v", res.Diagnostics)
	}
}

func TestPreflightRecomputesRiskAndBlocksHighRiskNode(t *testing.T) {
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}
	riskPolicyRepo := newMockRiskPolicyRepo()
	riskObsRepo := newMockRiskObsRepo()

	revID := domain.MustNewUUIDv7()
	policyRev := makeTestRiskPolicy(revID, domain.RiskActionBlock)
	_ = riskPolicyRepo.Create(context.Background(), policyRev)

	hkID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "hk.example.com", 443, nil)
	usID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "us.example.com", 8388, nil)

	// Add high-risk observation for hkID (score=85 -> block)
	obs := makeTestObservation(hkID, 85, 90)
	_ = riskObsRepo.Create(context.Background(), &obs)

	ipriskSvc := iprisk.NewService(riskObsRepo, riskPolicyRepo)

	svc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithIPRiskService(ipriskSvc),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	ctx := context.Background()
	snap := buildSnapshotWithValidNodes(hkID, usID)
	snap.Diagnostics = nil

	// 1. Preflight diagnostic check should catch that hkID is recomputed as blocked
	preflightRes, err := svc.Preflight(ctx, publication.PreflightCommand{
		Target:   domain.TargetClash,
		Snapshot: snap,
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if preflightRes.Allowed {
		t.Fatal("expected preflight to be rejected due to high-risk observation")
	}
	foundBlocked := false
	for _, d := range preflightRes.Diagnostics {
		if d.Target == hkID && d.Code == "risk_blocked" {
			foundBlocked = true
			break
		}
	}
	if !foundBlocked {
		t.Fatalf("expected risk_blocked diagnostic for hkID %s, got: %+v", hkID, preflightRes.Diagnostics)
	}

	// 2. Publish must also be intercepted with a conflict error
	_, err = svc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-blocked-node",
	})
	if err == nil {
		t.Fatal("expected publish to be rejected by preflight interceptor")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Category != domain.CategoryConflict || de.Code != "publication_preflight_rejected" {
		t.Fatalf("expected publication_preflight_rejected conflict error, got %v", err)
	}

	// 3. Verify no partial publication was created
	if len(pubRepo.publications) != 0 {
		t.Fatalf("expected 0 publications created, got %d", len(pubRepo.publications))
	}

	// 4. Verify audit log recorded rejection with zero raw IP leakage
	auditRepo.mu.Lock()
	defer auditRepo.mu.Unlock()
	if len(auditRepo.events) == 0 {
		t.Fatal("expected audit event to be recorded")
	}
	lastEvent := auditRepo.events[len(auditRepo.events)-1]
	if lastEvent.Result != domain.AuditResultFailure {
		t.Fatalf("expected audit failure, got %s", lastEvent.Result)
	}
	if strings.Contains(lastEvent.RedactedSummary, "192.168.") || strings.Contains(lastEvent.RedactedSummary, "10.0.") {
		t.Fatalf("audit summary leaked IP: %s", lastEvent.RedactedSummary)
	}
}

func TestPreflightRecomputationAllowsReviewWhenConfigured(t *testing.T) {
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}
	riskPolicyRepo := newMockRiskPolicyRepo()
	riskObsRepo := newMockRiskObsRepo()

	revID := domain.MustNewUUIDv7()
	policyRev := makeTestRiskPolicy(revID, domain.RiskActionAllow)
	_ = riskPolicyRepo.Create(context.Background(), policyRev)

	hkID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "hk.example.com", 443, nil)
	usID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "us.example.com", 8388, nil)

	// Medium-risk observation (score=50 -> review)
	obs := makeTestObservation(hkID, 50, 90)
	_ = riskObsRepo.Create(context.Background(), &obs)

	ipriskSvc := iprisk.NewService(riskObsRepo, riskPolicyRepo)

	svc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithIPRiskService(ipriskSvc),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	ctx := context.Background()
	snap := buildSnapshotWithValidNodes(hkID, usID)
	snap.Diagnostics = nil

	// Preflight should allow because reviewAction is allow
	preflightRes, err := svc.Preflight(ctx, publication.PreflightCommand{
		Target:   domain.TargetClash,
		Snapshot: snap,
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if !preflightRes.Allowed {
		t.Fatalf("expected preflight to allow review when reviewAction=allow, got diagnostics: %+v", preflightRes.Diagnostics)
	}

	// Publish should succeed
	pubRes, err := svc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-review-allowed",
	})
	if err != nil {
		t.Fatalf("expected publish to succeed, got %v", err)
	}
	if pubRes.Publication.ID == "" {
		t.Fatal("expected publication to be created")
	}
}

func TestExistingPublicationImmutableToSubsequentRiskObservations(t *testing.T) {
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}
	riskPolicyRepo := newMockRiskPolicyRepo()
	riskObsRepo := newMockRiskObsRepo()

	revID := domain.MustNewUUIDv7()
	policyRev := makeTestRiskPolicy(revID, domain.RiskActionBlock)
	_ = riskPolicyRepo.Create(context.Background(), policyRev)

	hkID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "hk.example.com", 443, nil)
	usID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "us.example.com", 8388, nil)

	// Initially low-risk observation for hkID (score=10 -> allow)
	obs1 := makeTestObservation(hkID, 10, 90)
	_ = riskObsRepo.Create(context.Background(), &obs1)

	// Initially low-risk observation for usID (score=15 -> allow)
	obsUS := makeTestObservation(usID, 15, 90)
	_ = riskObsRepo.Create(context.Background(), &obsUS)

	ipriskSvc := iprisk.NewService(riskObsRepo, riskPolicyRepo)

	svc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithIPRiskService(ipriskSvc),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	ctx := context.Background()
	snap := buildSnapshotWithValidNodes(hkID, usID)
	snap.Diagnostics = nil

	// 1. First publication succeeds
	pubRes1, err := svc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-initial-pub",
	})
	if err != nil {
		t.Fatalf("initial publish failed: %v", err)
	}

	initialPubID := pubRes1.Publication.ID
	initialToken := pubRes1.RawToken
	initialContentDigest := pubRes1.ContentDigest

	// 2. Serve initial publication -> succeeds
	artifact1, err := svc.ResolveAndServe(ctx, initialPubID, initialToken)
	if err != nil {
		t.Fatalf("failed to serve initial publication: %v", err)
	}
	if artifact1.ContentDigest != initialContentDigest {
		t.Fatalf("content digest mismatch: %s vs %s", artifact1.ContentDigest, initialContentDigest)
	}

	// 3. New high-risk observation arrives for hkID (score=99 -> block)
	obs2 := makeTestObservation(hkID, 99, 99)
	obs2.ObservedAt = obs1.ObservedAt.Add(time.Second)
	_ = riskObsRepo.Create(ctx, &obs2)

	// 4. Serving the EXISTING publication must STILL succeed with the EXACT SAME content digest!
	artifact2, err := svc.ResolveAndServe(ctx, initialPubID, initialToken)
	if err != nil {
		t.Fatalf("existing immutable publication serving failed after subsequent observation: %v", err)
	}
	if artifact2.ContentDigest != initialContentDigest {
		t.Fatalf("immutable publication content changed after subsequent observation! %s vs %s", artifact2.ContentDigest, initialContentDigest)
	}

	// 5. Creating a NEW publication now MUST be intercepted by preflight!
	_, err = svc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-second-pub",
	})
	if err == nil {
		t.Fatal("expected new publication to be blocked by preflight after observation changed to high risk")
	}

	// 6. The existing publication MUST NOT be replaced, mutated, or revoked
	existingDetail, err := svc.Get(ctx, initialPubID)
	if err != nil {
		t.Fatalf("failed to get existing publication: %v", err)
	}
	if existingDetail.Publication.State != domain.PublicationStateActive {
		t.Fatalf("expected existing publication to remain active, got: %s", existingDetail.Publication.State)
	}
	if existingDetail.ContentDigest != initialContentDigest {
		t.Fatalf("existing publication content digest was altered: %s vs %s", existingDetail.ContentDigest, initialContentDigest)
	}
	if len(pubRepo.publications) != 1 {
		t.Fatalf("expected exactly 1 publication in repo, got %d", len(pubRepo.publications))
	}
}

func TestPreflight_BlocksEmptyRoutedGroup(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()

	snap := buildSampleSnapshot()
	snap.Diagnostics = []resolver.Diagnostic{
		{
			Severity: resolver.DiagnosticSeverityError,
			Code:     "empty_routed_group",
			Message:  "routed group Proxy has 0 nodes after filtering",
			Target:   "grp-proxy-1",
		},
	}

	preRes, err := svc.Preflight(ctx, publication.PreflightCommand{
		Target:   domain.TargetClash,
		Snapshot: snap,
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if preRes.Allowed {
		t.Fatalf("expected preflight Allowed to be false for empty_routed_group")
	}
	if len(preRes.Diagnostics) != 1 || preRes.Diagnostics[0].Code != "empty_routed_group" {
		t.Fatalf("expected empty_routed_group diagnostic, got %+v", preRes.Diagnostics)
	}
}

func TestPublish_BlocksEmptyRoutedGroup(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()

	snap := buildSampleSnapshot()
	snap.Diagnostics = []resolver.Diagnostic{
		{
			Severity: resolver.DiagnosticSeverityError,
			Code:     "empty_routed_group",
			Message:  "routed group Proxy has 0 nodes after filtering",
			Target:   "grp-proxy-1",
		},
	}

	_, err := svc.Publish(ctx, publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-blocked-pub",
	})
	if err == nil {
		t.Fatalf("expected publish to be blocked for empty_routed_group, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if domErr.Code != "publication_preflight_rejected" {
		t.Fatalf("expected publication_preflight_rejected code, got %s", domErr.Code)
	}
}
