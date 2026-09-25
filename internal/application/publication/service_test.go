package publication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// mockPublicationRepo implements domain.PublicationRepository in-memory for testing.
type mockPublicationRepo struct {
	mu           sync.RWMutex
	publications map[string]*domain.Publication
	tokenMap     map[string]*domain.Publication
}

func newMockPublicationRepo() *mockPublicationRepo {
	return &mockPublicationRepo{
		publications: make(map[string]*domain.Publication),
		tokenMap:     make(map[string]*domain.Publication),
	}
}

func (m *mockPublicationRepo) GetByID(ctx context.Context, id string) (*domain.Publication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pub, ok := m.publications[id]
	if !ok {
		return nil, domain.NewNotFoundError("publication_not_found", "publication not found")
	}
	copy := *pub
	return &copy, nil
}

func (m *mockPublicationRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Publication, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pub, ok := m.tokenMap[tokenHash]
	if !ok {
		return nil, domain.NewNotFoundError("publication_not_found", "publication for token hash not found")
	}
	copy := *pub
	return &copy, nil
}

func (m *mockPublicationRepo) Create(ctx context.Context, pub *domain.Publication) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tokenMap[pub.TokenHash]; exists {
		return domain.NewConflictError("duplicate_token_hash", "token hash already exists")
	}
	copy := *pub
	m.publications[pub.ID] = &copy
	m.tokenMap[pub.TokenHash] = &copy
	return nil
}

func (m *mockPublicationRepo) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pub, ok := m.publications[id]
	if !ok {
		return domain.NewNotFoundError("publication_not_found", "publication not found")
	}
	pub.State = domain.PublicationStateRevoked
	pub.RevokedAt = &revokedAt
	return nil
}

// mockAuditRepo implements domain.AuditRepository in-memory for testing.
type mockAuditRepo struct {
	mu     sync.Mutex
	events []domain.AuditEvent
}

func (m *mockAuditRepo) Record(ctx context.Context, event *domain.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, *event)
	return nil
}

func (m *mockAuditRepo) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.AuditEvent(nil), m.events...), len(m.events), nil
}

// buildSampleSnapshot returns a standard valid snapshot supported across all five compilers.
func buildSampleSnapshot() *resolver.ResolvedPolicySnapshot {
	nodes := []resolver.ResolvedNode{
		{LogicalID: "node-hk-01", DisplayName: "Hong Kong 01", Protocol: domain.ProtocolTrojan, Active: true, Position: 0},
		{LogicalID: "node-us-01", DisplayName: "United States 01", Protocol: domain.ProtocolSS, Active: true, Position: 1},
	}
	groups := []resolver.ResolvedGroup{
		{
			ID:                "group-select",
			Name:              "ProxySelect",
			GroupType:         domain.GroupTypeSelect,
			NodeLogicalIDs:    []string{"node-hk-01", "node-us-01"},
			AllNodeLogicalIDs: []string{"node-hk-01", "node-us-01"},
			Position:          0,
		},
	}
	rules := []resolver.ResolvedRule{
		{ID: "rule-1", TargetGroupID: "group-select", TargetGroupName: "ProxySelect", Expression: "DOMAIN-SUFFIX,google.com", Position: 0},
		{ID: "rule-match", TargetGroupID: "group-select", TargetGroupName: "ProxySelect", Expression: "MATCH", Position: 1, IsTerminal: true},
	}

	raw := "Hong Kong 01|node-hk-01|trojan|United States 01|node-us-01|ss|ProxySelect|select|DOMAIN-SUFFIX,google.com|MATCH"
	sum := sha256.Sum256([]byte(raw))
	snapDigest := hex.EncodeToString(sum[:])

	return &resolver.ResolvedPolicySnapshot{
		InputDigest:     "sha256:input-sample",
		SnapshotDigest:  snapDigest,
		CompilerVersion: "1.0.0",
		Nodes:           nodes,
		NodeLogicalIDs:  []string{"node-hk-01", "node-us-01"},
		Groups:          groups,
		Rules:           rules,
		DNS:             resolver.DNSConfig{Enabled: true, Nameservers: []string{"1.1.1.1", "8.8.8.8"}},
		ResolvedAt:      time.Now().UTC(),
	}
}

// buildIncompatibleSnapshot returns a snapshot containing protocols (Hysteria2) unsupported by Clash/Surge.
func buildIncompatibleSnapshot() *resolver.ResolvedPolicySnapshot {
	snap := buildSampleSnapshot()
	snap.Nodes = append(snap.Nodes, resolver.ResolvedNode{
		LogicalID:   "node-hy2-01",
		DisplayName: "Hysteria 01",
		Protocol:    domain.ProtocolHysteria2,
		Active:      true,
		Position:    2,
	})
	snap.NodeLogicalIDs = append(snap.NodeLogicalIDs, "node-hy2-01")
	return snap
}

func setupService() (*publication.Service, *mockPublicationRepo, *mockAuditRepo) {
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}
	svc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithResolver(resolver.New()),
	)
	return svc, pubRepo, auditRepo
}

func TestPublishSuccess(t *testing.T) {
	svc, _, auditRepo := setupService()
	ctx := context.Background()
	snap := buildSampleSnapshot()

	cmd := publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-publish-001",
	}

	result, err := svc.Publish(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	if result.Publication.ID == "" {
		t.Fatal("expected non-empty publication ID")
	}
	if result.Publication.Target != domain.TargetClash {
		t.Fatalf("expected target clash, got %s", result.Publication.Target)
	}
	if result.Publication.State != domain.PublicationStateActive {
		t.Fatalf("expected state active, got %s", result.Publication.State)
	}
	if result.Publication.SnapshotDigest != snap.SnapshotDigest {
		t.Fatalf("expected snapshot digest %s, got %s", snap.SnapshotDigest, result.Publication.SnapshotDigest)
	}
	if !strings.HasPrefix(result.RawToken, "pub_") {
		t.Fatalf("expected raw token to start with pub_, got %s", result.RawToken)
	}
	if result.ExportURL != "/publish/v1/"+result.Publication.ID+"?token="+result.RawToken {
		t.Fatalf("unexpected export URL: %s", result.ExportURL)
	}
	if result.ContentType != "application/yaml" {
		t.Fatalf("expected content type application/yaml, got %s", result.ContentType)
	}
	if result.Filename != "clash.yaml" {
		t.Fatalf("expected filename clash.yaml, got %s", result.Filename)
	}
	if result.ContentDigest == "" {
		t.Fatal("expected non-empty content digest")
	}

	// Verify audit log recorded
	auditRepo.mu.Lock()
	if len(auditRepo.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditRepo.events))
	}
	ev := auditRepo.events[0]
	auditRepo.mu.Unlock()

	if ev.Action != "publication.create" {
		t.Fatalf("expected audit action publication.create, got %s", ev.Action)
	}
	if ev.Result != domain.AuditResultSuccess {
		t.Fatalf("expected audit result success, got %s", ev.Result)
	}
	// Verify raw token is NOT leaked in audit summary
	if strings.Contains(ev.RedactedSummary, result.RawToken) {
		t.Fatalf("raw token leaked in audit summary: %s", ev.RedactedSummary)
	}
}

func TestPublishIncompatibleTargetHardFails(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()
	incompatibleSnap := buildIncompatibleSnapshot()

	// QuantumultX does not support Hysteria2 -> must hard fail
	cmd := publication.PublishCommand{
		Target:    domain.TargetQuantumultX,
		Snapshot:  incompatibleSnap,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-fail-001",
	}

	_, err := svc.Publish(ctx, cmd)
	if err == nil {
		t.Fatal("expected error for incompatible target, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) {
		t.Fatalf("expected domain error, got %T: %v", err, err)
	}
	if domErr.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %s", domErr.Code)
	}
	if domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected category validation, got %s", domErr.Category)
	}
}

func TestPreviewIncompatibleTargetReturnsValidationDomainError(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()
	incompatibleSnap := buildIncompatibleSnapshot()

	query := publication.PreviewQuery{
		Target:   domain.TargetQuantumultX,
		Snapshot: incompatibleSnap,
	}

	_, err := svc.Preview(ctx, query)
	if err == nil {
		t.Fatal("expected error for incompatible preview target, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) {
		t.Fatalf("expected domain error, got %T: %v", err, err)
	}
	if domErr.Code != "unsupported_target_capability" {
		t.Fatalf("expected code unsupported_target_capability, got %s", domErr.Code)
	}
	if domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected category validation, got %s", domErr.Category)
	}
}

func TestPublishPreflightBlocksRiskAndDoesNotCreatePublication(t *testing.T) {
	svc, repo, audit := setupService()
	snapshot := buildSampleSnapshot()
	snapshot.RiskPolicyRevision = "0191e4a0-0000-7000-8000-000000000001"
	snapshot.ExcludedNodeIDs = []string{"node-hk-01"}
	snapshot.Diagnostics = []resolver.Diagnostic{{
		Severity: resolver.DiagnosticSeverityWarning,
		Code:     "risk_blocked",
		Message:  "node node-hk-01 was excluded by risk policy",
		Target:   "node-hk-01",
	}}

	_, err := svc.Publish(context.Background(), publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snapshot,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-risk-blocked",
	})
	if err == nil {
		t.Fatal("expected risk preflight to reject publication")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Category != domain.CategoryConflict || de.Code != "publication_preflight_rejected" {
		t.Fatalf("expected publication preflight conflict, got %v", err)
	}
	if len(repo.publications) != 0 {
		t.Fatalf("expected no publication after preflight rejection, got %d", len(repo.publications))
	}
	if len(audit.events) != 1 || audit.events[0].Result != domain.AuditResultFailure {
		t.Fatalf("expected one failed audit event, got %+v", audit.events)
	}
}

func TestPublishPreflightRejectsUnpublishableReview(t *testing.T) {
	svc, repo, _ := setupService()
	snapshot := buildSampleSnapshot()
	snapshot.RiskPolicyRevision = "0191e4a0-0000-7000-8000-000000000001"
	snapshot.ExcludedNodeIDs = []string{"node-us-01"}
	snapshot.Diagnostics = []resolver.Diagnostic{{
		Severity: resolver.DiagnosticSeverityWarning,
		Code:     "risk_review",
		Message:  "node node-us-01 was excluded pending risk review",
		Target:   "node-us-01",
	}}

	_, err := svc.Publish(context.Background(), publication.PublishCommand{
		Target:    domain.TargetClash,
		Snapshot:  snapshot,
		ActorKind: domain.ActorKindAdmin,
		RequestID: "req-risk-review",
	})
	if err == nil {
		t.Fatal("expected unpublishable review to reject publication")
	}
	de, ok := domain.AsDomainError(err)
	if !ok || de.Category != domain.CategoryConflict || de.Code != "publication_preflight_rejected" {
		t.Fatalf("expected publication preflight conflict, got %v", err)
	}
	if len(repo.publications) != 0 {
		t.Fatalf("expected no publication after review rejection, got %d", len(repo.publications))
	}
}
