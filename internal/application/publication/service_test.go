package publication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// buildSampleSnapshot returns a standard valid snapshot supported across all four compilers.
func buildSampleSnapshot() *resolver.ResolvedPolicySnapshot {
	hkIdentity := domain.NewVerifiedNodeIdentity("node-hk-01", domain.ProtocolTrojan, "hk.example.com", 443, 1, nil)
	usIdentity := domain.NewVerifiedNodeIdentity("node-us-01", domain.ProtocolSS, "us.example.com", 8388, 1, nil)
	nodes := []resolver.ResolvedNode{
		{LogicalID: "node-hk-01", DisplayName: "Hong Kong 01", Protocol: domain.ProtocolTrojan, Active: true, Position: 0, CredentialVersion: 1, Identity: &hkIdentity},
		{LogicalID: "node-us-01", DisplayName: "United States 01", Protocol: domain.ProtocolSS, Active: true, Position: 1, CredentialVersion: 1, Identity: &usIdentity},
	}
	groups := []resolver.ResolvedGroup{
		{
			ID:        "group-select",
			Name:      "ProxySelect",
			GroupType: domain.GroupTypeSelect,
			Members: []resolver.ResolvedGroupMember{
				{Kind: resolver.MemberKindNode, TargetID: "node-hk-01", DisplayName: "Hong Kong 01", Position: 0},
				{Kind: resolver.MemberKindNode, TargetID: "node-us-01", DisplayName: "United States 01", Position: 1},
			},
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

// buildIncompatibleSnapshot returns a snapshot containing protocols (Hysteria2) unsupported by Surge/QX.
func buildIncompatibleSnapshot() *resolver.ResolvedPolicySnapshot {
	snap := buildSampleSnapshot()
	hy2Identity := domain.NewVerifiedNodeIdentity("node-hy2-01", domain.ProtocolHysteria2, "hy2.example.com", 443, 1, nil)
	snap.Nodes = append(snap.Nodes, resolver.ResolvedNode{
		LogicalID:         "node-hy2-01",
		DisplayName:       "Hysteria 01",
		Protocol:          domain.ProtocolHysteria2,
		Active:            true,
		Position:          2,
		CredentialVersion: 1,
		Identity:          &hy2Identity,
	})
	snap.NodeLogicalIDs = append(snap.NodeLogicalIDs, "node-hy2-01")
	return snap
}

func setupSampleVaultSource() (*domain.NodeCredentialVault, *mockCredentialRepo, *mockNodeRepo) {
	masterKey := []byte("01234567890123456789012345678901")
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": masterKey})
	if err != nil {
		panic(fmt.Sprintf("failed to create sample vault: %v", err))
	}
	credRepo := newMockCredentialRepo()
	nodeRepo := newMockNodeRepo()
	now := time.Now().UTC()

	sampleNodes := []struct {
		id     string
		name   string
		proto  domain.Protocol
		server string
		port   int
		cred   domain.InboundProtocolCredential
	}{
		{
			id:     "node-hk-01",
			name:   "Hong Kong 01",
			proto:  domain.ProtocolTrojan,
			server: "hk.example.com",
			port:   443,
			cred:   domain.InboundProtocolCredential{Password: "hk-trojan-secret-password"},
		},
		{
			id:     "node-us-01",
			name:   "United States 01",
			proto:  domain.ProtocolSS,
			server: "us.example.com",
			port:   8388,
			cred:   domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "us-ss-secret-password"},
		},
		{
			id:     "node-hy2-01",
			name:   "Hysteria 01",
			proto:  domain.ProtocolHysteria2,
			server: "hy2.example.com",
			port:   443,
			cred:   domain.InboundProtocolCredential{Password: "hy2-secret-password"},
		},
	}

	var nodes []domain.Node
	for _, sn := range sampleNodes {
		identity := domain.NewVerifiedNodeIdentity(sn.id, sn.proto, sn.server, sn.port, 1, sn.cred.Transport)
		nodes = append(nodes, domain.Node{
			LogicalID:         sn.id,
			Protocol:          sn.proto,
			DisplayName:       sn.name,
			CredentialVersion: 1,
			Identity:          &identity,
			Active:            true,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
		rec, err := vault.Encrypt(&domain.NodeCredentialPayload{
			LogicalID:   sn.id,
			Protocol:    sn.proto,
			Server:      sn.server,
			Port:        sn.port,
			Version:     1,
			Identity:    &identity,
			Credentials: sn.cred,
		})
		if err != nil {
			panic(fmt.Sprintf("encrypt sample node %s: %v", sn.id, err))
		}
		_ = credRepo.Upsert(context.Background(), rec)
	}
	_ = nodeRepo.UpsertBatch(context.Background(), nodes)
	return vault, credRepo, nodeRepo
}

func setupService() (*publication.Service, *mockPublicationRepo, *mockAuditRepo) {
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}
	vault, credRepo, nodeRepo := setupSampleVaultSource()
	svc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithResolver(resolver.New()),
		publication.WithNodeRepository(nodeRepo),
		publication.WithCredentialSource(vault, credRepo),
	)
	return svc, pubRepo, auditRepo
}

func TestPublishSuccess(t *testing.T) {
	svc, _, auditRepo := setupService()
	ctx := context.Background()
	snap := buildSampleSnapshot()

	cmd := publication.PublishCommand{
		Target:    domain.TargetSingBox,
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
	if result.Publication.Target != domain.TargetSingBox {
		t.Fatalf("expected target singbox, got %s", result.Publication.Target)
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
	if result.ContentType != "application/json" {
		t.Fatalf("expected content type application/json, got %s", result.ContentType)
	}
	if result.Filename != "sing-box.json" {
		t.Fatalf("expected filename sing-box.json, got %s", result.Filename)
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

	// Surge does not support Hysteria2 -> must hard fail
	cmd := publication.PublishCommand{
		Target:    domain.TargetSurge,
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
		Target:   domain.TargetSurge,
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
		Target:    domain.TargetSingBox,
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
		Target:    domain.TargetSingBox,
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

func TestServiceRejectsLegacyTargetClashAndUnknownTargets(t *testing.T) {
	svc, _, _ := setupService()
	ctx := context.Background()
	snap := buildSampleSnapshot()

	// 1. Legacy target "clash" must be rejected on Publish and Preview without alias/fallback
	for _, illegalTarget := range []domain.CompilerTarget{"clash", "unknown", "v2ray"} {
		t.Run("publish_"+string(illegalTarget), func(t *testing.T) {
			_, err := svc.Publish(ctx, publication.PublishCommand{
				Target:    illegalTarget,
				Snapshot:  snap,
				ActorKind: domain.ActorKindAdmin,
				RequestID: "req-illegal-target",
			})
			if err == nil {
				t.Fatalf("expected error for illegal target %s, got nil", illegalTarget)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "unsupported_target" || de.Category != domain.CategoryValidation {
				t.Fatalf("expected unsupported_target validation error for %s, got %#v", illegalTarget, err)
			}
		})

		t.Run("preview_"+string(illegalTarget), func(t *testing.T) {
			_, err := svc.Preview(ctx, publication.PreviewQuery{
				Target:   illegalTarget,
				Snapshot: snap,
			})
			if err == nil {
				t.Fatalf("expected error for illegal target %s, got nil", illegalTarget)
			}
			de, ok := domain.AsDomainError(err)
			if !ok || de.Code != "unsupported_target" || de.Category != domain.CategoryValidation {
				t.Fatalf("expected unsupported_target validation error for %s, got %#v", illegalTarget, err)
			}
		})
	}
}

// mockCredentialRepo implements domain.NodeCredentialRepository in memory.
type mockCredentialRepo struct {
	mu      sync.RWMutex
	records map[string]*domain.NodeCredentialRecord
}

func newMockCredentialRepo() *mockCredentialRepo {
	return &mockCredentialRepo{records: make(map[string]*domain.NodeCredentialRecord)}
}

func (m *mockCredentialRepo) key(logicalID string, version int) string {
	return fmt.Sprintf("%s:%d", logicalID, version)
}

func (m *mockCredentialRepo) GetByLogicalID(ctx context.Context, logicalID string, version int) (*domain.NodeCredentialRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.records[m.key(logicalID, version)]
	if !ok {
		return nil, domain.NewNotFoundError("credential_not_found", "credential not found")
	}
	copy := *rec
	return &copy, nil
}

func (m *mockCredentialRepo) GetLatestByLogicalID(ctx context.Context, logicalID string) (*domain.NodeCredentialRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *domain.NodeCredentialRecord
	for _, rec := range m.records {
		if rec.LogicalID == logicalID {
			if latest == nil || rec.Version > latest.Version {
				copy := *rec
				latest = &copy
			}
		}
	}
	if latest == nil {
		return nil, domain.NewNotFoundError("credential_not_found", "credential not found")
	}
	return latest, nil
}

func (m *mockCredentialRepo) Upsert(ctx context.Context, record *domain.NodeCredentialRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *record
	m.records[m.key(record.LogicalID, record.Version)] = &copy
	return nil
}

func (m *mockCredentialRepo) DeleteByLogicalID(ctx context.Context, logicalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, rec := range m.records {
		if rec.LogicalID == logicalID {
			delete(m.records, k)
		}
	}
	return nil
}

// mockNodeRepo implements domain.NodeRepository in memory.
type mockNodeRepo struct {
	mu    sync.RWMutex
	nodes map[string]*domain.Node
}

func newMockNodeRepo() *mockNodeRepo {
	return &mockNodeRepo{nodes: make(map[string]*domain.Node)}
}

func (m *mockNodeRepo) GetByLogicalID(ctx context.Context, logicalID string) (*domain.Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.nodes[logicalID]
	if !ok {
		return nil, domain.NewNotFoundError("node_not_found", "node not found")
	}
	copy := *n
	return &copy, nil
}

func (m *mockNodeRepo) List(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]domain.Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		if filter.ActiveOnly && !n.Active {
			continue
		}
		res = append(res, *n)
	}
	return res, len(res), nil
}

func (m *mockNodeRepo) ListReadModel(ctx context.Context, filter domain.NodeFilter) ([]domain.NodeReadModel, int, error) {
	return nil, 0, nil
}

func (m *mockNodeRepo) GetReadModel(ctx context.Context, logicalID string, policyRevisionID string) (*domain.NodeReadModel, error) {
	return nil, nil
}

func (m *mockNodeRepo) UpsertBatch(ctx context.Context, nodes []domain.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range nodes {
		copy := n
		m.nodes[n.LogicalID] = &copy
	}
	return nil
}

func (m *mockNodeRepo) DeactivateNodesNotIn(ctx context.Context, activeLogicalIDs []string) error {
	return nil
}

// mockRevisionRepo implements domain.RevisionRepository in memory.
type mockRevisionRepo struct {
	mu        sync.RWMutex
	revisions map[string]*domain.ConfigurationRevision
	activeID  string
}

func newMockRevisionRepo() *mockRevisionRepo {
	return &mockRevisionRepo{revisions: make(map[string]*domain.ConfigurationRevision)}
}

func (m *mockRevisionRepo) GetByID(ctx context.Context, id string) (*domain.ConfigurationRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.revisions[id]
	if !ok {
		return nil, domain.NewNotFoundError("revision_not_found", "revision not found")
	}
	copy := *r
	return &copy, nil
}

func (m *mockRevisionRepo) GetActive(ctx context.Context) (*domain.ConfigurationRevision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeID == "" {
		return nil, domain.NewNotFoundError("active_revision_not_found", "no active revision")
	}
	r, ok := m.revisions[m.activeID]
	if !ok {
		return nil, domain.NewNotFoundError("active_revision_not_found", "no active revision")
	}
	copy := *r
	return &copy, nil
}

func (m *mockRevisionRepo) List(ctx context.Context, filter domain.RevisionFilter) ([]domain.ConfigurationRevision, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]domain.ConfigurationRevision, 0, len(m.revisions))
	for _, r := range m.revisions {
		res = append(res, *r)
	}
	return res, len(res), nil
}

func (m *mockRevisionRepo) Create(ctx context.Context, rev *domain.ConfigurationRevision) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *rev
	m.revisions[rev.ID] = &copy
	if rev.State == domain.RevisionStateActive {
		m.activeID = rev.ID
	}
	return nil
}

func (m *mockRevisionRepo) SetActive(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.revisions[id]
	if !ok {
		return domain.NewNotFoundError("revision_not_found", "revision not found")
	}
	for _, other := range m.revisions {
		if other.State == domain.RevisionStateActive {
			other.State = domain.RevisionStateArchived
		}
	}
	r.State = domain.RevisionStateActive
	m.activeID = id
	return nil
}

// mockPolicyRepo implements domain.PolicyRepository in memory.
type mockPolicyRepo struct {
	mu             sync.RWMutex
	groups         map[string]*domain.NodeGroup
	edges          map[string][]domain.GroupEdge
	policyRules    map[string][]domain.PolicyRule
	admissionRules map[string][]domain.AdmissionRule
}

func newMockPolicyRepo() *mockPolicyRepo {
	return &mockPolicyRepo{
		groups:         make(map[string]*domain.NodeGroup),
		edges:          make(map[string][]domain.GroupEdge),
		policyRules:    make(map[string][]domain.PolicyRule),
		admissionRules: make(map[string][]domain.AdmissionRule),
	}
}

func (m *mockPolicyRepo) GetGroupByID(ctx context.Context, id string) (*domain.NodeGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.groups[id]
	if !ok {
		return nil, domain.NewNotFoundError("group_not_found", "group not found")
	}
	copy := *g
	return &copy, nil
}

func (m *mockPolicyRepo) ListGroups(ctx context.Context) ([]domain.NodeGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]domain.NodeGroup, 0, len(m.groups))
	for _, g := range m.groups {
		res = append(res, *g)
	}
	return res, nil
}

func (m *mockPolicyRepo) CreateGroup(ctx context.Context, group *domain.NodeGroup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *group
	m.groups[group.ID] = &copy
	return nil
}

func (m *mockPolicyRepo) UpdateGroup(ctx context.Context, group *domain.NodeGroup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *group
	m.groups[group.ID] = &copy
	return nil
}

func (m *mockPolicyRepo) DeleteGroup(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.groups, id)
	return nil
}

func (m *mockPolicyRepo) ListEdgesByGroup(ctx context.Context, parentGroupID string) ([]domain.GroupEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	edges := m.edges[parentGroupID]
	return append([]domain.GroupEdge(nil), edges...), nil
}

func (m *mockPolicyRepo) SetEdgesForGroup(ctx context.Context, parentGroupID string, edges []GroupEdgeSafe) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edges[parentGroupID] = edges
	return nil
}

type GroupEdgeSafe = domain.GroupEdge

func (m *mockPolicyRepo) ListAdmissionRules(ctx context.Context, revisionID string) ([]domain.AdmissionRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.AdmissionRule(nil), m.admissionRules[revisionID]...), nil
}

func (m *mockPolicyRepo) CreateAdmissionRule(ctx context.Context, rule *domain.AdmissionRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.admissionRules[rule.RevisionID] = append(m.admissionRules[rule.RevisionID], *rule)
	return nil
}

func (m *mockPolicyRepo) ListPolicyRules(ctx context.Context, revisionID string) ([]domain.PolicyRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.PolicyRule(nil), m.policyRules[revisionID]...), nil
}

func (m *mockPolicyRepo) CreatePolicyRule(ctx context.Context, rule *domain.PolicyRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policyRules[rule.RevisionID] = append(m.policyRules[rule.RevisionID], *rule)
	return nil
}

func setupVaultAndNodes(t *testing.T) (*domain.NodeCredentialVault, *mockCredentialRepo, *mockNodeRepo, *resolver.ResolvedPolicySnapshot) {
	masterKey := []byte("01234567890123456789012345678901") // 32 bytes
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": masterKey})
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	credRepo := newMockCredentialRepo()
	nodeRepo := newMockNodeRepo()

	now := time.Now().UTC()

	hy2Transport := map[string]string{
		"sni":              "hy2.sample.com",
		"skip_cert_verify": "true",
	}
	hy2ID := domain.ComputeNodeLogicalID(domain.ProtocolHysteria2, "198.51.100.25", 443, hy2Transport)
	ssID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.26", 8388, nil)

	hy2Identity := domain.NewVerifiedNodeIdentity(hy2ID, domain.ProtocolHysteria2, "198.51.100.25", 443, 1, hy2Transport)
	ssIdentity := domain.NewVerifiedNodeIdentity(ssID, domain.ProtocolSS, "198.51.100.26", 8388, 1, nil)

	// 1. Hysteria2 node
	hy2Node := domain.Node{
		LogicalID:         hy2ID,
		Protocol:          domain.ProtocolHysteria2,
		DisplayName:       "Hy2-Edge",
		CredentialVersion: 1,
		Identity:          &hy2Identity,
		Active:            true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	hy2Payload := &domain.NodeCredentialPayload{
		LogicalID: hy2ID,
		Protocol:  domain.ProtocolHysteria2,
		Server:    "198.51.100.25",
		Port:      443,
		Version:   1,
		Identity:  &hy2Identity,
		Credentials: domain.InboundProtocolCredential{
			Password:  "hy2-super-secret-password",
			Transport: hy2Transport,
		},
	}
	hy2Rec, err := vault.Encrypt(hy2Payload)
	if err != nil {
		t.Fatalf("encrypt hy2: %v", err)
	}
	if err := credRepo.Upsert(context.Background(), hy2Rec); err != nil {
		t.Fatalf("upsert hy2 record: %v", err)
	}

	// 2. Shadowsocks node
	ssNode := domain.Node{
		LogicalID:         ssID,
		Protocol:          domain.ProtocolSS,
		DisplayName:       "SS-Edge",
		CredentialVersion: 1,
		Identity:          &ssIdentity,
		Active:            true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	ssPayload := &domain.NodeCredentialPayload{
		LogicalID: ssID,
		Protocol:  domain.ProtocolSS,
		Server:    "198.51.100.26",
		Port:      8388,
		Version:   1,
		Identity:  &ssIdentity,
		Credentials: domain.InboundProtocolCredential{
			Method:   "aes-256-gcm",
			Password: "ss-super-secret-password",
		},
	}
	ssRec, err := vault.Encrypt(ssPayload)
	if err != nil {
		t.Fatalf("encrypt ss: %v", err)
	}
	if err := credRepo.Upsert(context.Background(), ssRec); err != nil {
		t.Fatalf("upsert ss record: %v", err)
	}

	_ = nodeRepo.UpsertBatch(context.Background(), []domain.Node{hy2Node, ssNode})

	groupID := domain.MustNewUUIDv7()
	snapshot := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "snap-hy2-ss-valid",
		CompilerVersion: "1.0.0",
		Nodes: []resolver.ResolvedNode{
			{LogicalID: hy2ID, DisplayName: "Hy2-Edge", Protocol: domain.ProtocolHysteria2, Active: true, Position: 0, CredentialVersion: 1, Identity: &hy2Identity},
			{LogicalID: ssID, DisplayName: "SS-Edge", Protocol: domain.ProtocolSS, Active: true, Position: 1, CredentialVersion: 1, Identity: &ssIdentity},
		},
		Groups: []resolver.ResolvedGroup{
			{
				ID:             groupID,
				Name:           "PROXY",
				GroupType:      domain.GroupTypeSelect,
				Members:        []resolver.ResolvedGroupMember{{Kind: resolver.MemberKindNode, TargetID: hy2ID, DisplayName: "Hy2-Edge", Position: 0}, {Kind: resolver.MemberKindNode, TargetID: ssID, DisplayName: "SS-Edge", Position: 1}},
				NodeLogicalIDs: []string{hy2ID, ssID},
				Position:       0,
			},
		},
		Rules: []resolver.ResolvedRule{
			{ID: domain.MustNewUUIDv7(), TargetGroupID: groupID, TargetGroupName: "PROXY", Expression: "MATCH", Position: 0, IsTerminal: true},
		},
	}

	return vault, credRepo, nodeRepo, snapshot
}

func TestMihomoPublishAndPreview_EncryptedVaultSuccessAndRestartRecovery(t *testing.T) {
	ctx := context.Background()
	vault, credRepo, nodeRepo, _ := setupVaultAndNodes(t)
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}
	revRepo := newMockRevisionRepo()
	policyRepo := newMockPolicyRepo()

	revID := domain.MustNewUUIDv7()
	groupID := domain.MustNewUUIDv7()
	ruleID := domain.MustNewUUIDv7()
	edge1ID := domain.MustNewUUIDv7()
	edge2ID := domain.MustNewUUIDv7()

	_ = revRepo.Create(ctx, &domain.ConfigurationRevision{
		ID:            revID,
		ContentDigest: "sha256:rev-digest-1",
		State:         domain.RevisionStateActive,
	})
	_ = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
		ID:        groupID,
		Name:      "PROXY",
		GroupType: domain.GroupTypeSelect,
	})
	hy2ID := domain.ComputeNodeLogicalID(domain.ProtocolHysteria2, "198.51.100.25", 443, map[string]string{
		"sni":              "hy2.sample.com",
		"skip_cert_verify": "true",
	})
	ssID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.26", 8388, nil)
	_ = policyRepo.SetEdgesForGroup(ctx, groupID, []domain.GroupEdge{
		{ID: edge1ID, ParentGroupID: groupID, NodeLogicalID: &hy2ID, Position: 0},
		{ID: edge2ID, ParentGroupID: groupID, NodeLogicalID: &ssID, Position: 1},
	})
	_ = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            ruleID,
		RevisionID:    revID,
		TargetGroupID: groupID,
		Expression:    "MATCH",
		Position:      0,
	})

	svc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithNodeRepository(nodeRepo),
		publication.WithCredentialSource(vault, credRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithPolicyRepository(policyRepo),
	)

	// 1. Preview Mihomo with credentials -> 200 OK
	previewRes, err := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: revID,
	})
	if err != nil {
		t.Fatalf("Preview Mihomo failed: %v", err)
	}

	previewContent := string(previewRes.Content)
	if !strings.Contains(previewContent, "server: 198.51.100.25") || !strings.Contains(previewContent, "password: hy2-super-secret-password") || !strings.Contains(previewContent, "sni: hy2.sample.com") {
		t.Fatalf("Preview Mihomo missing expected Hysteria2 credentials:\n%s", previewContent)
	}
	if !strings.Contains(previewContent, "server: 198.51.100.26") || !strings.Contains(previewContent, "password: ss-super-secret-password") || !strings.Contains(previewContent, "cipher: aes-256-gcm") {
		t.Fatalf("Preview Mihomo missing expected Shadowsocks credentials:\n%s", previewContent)
	}

	// 2. Publish Mihomo with credentials -> creates publication
	pubRes, err := svc.Publish(ctx, publication.PublishCommand{
		Target:     domain.TargetMihomo,
		RevisionID: revID,
		ActorKind:  domain.ActorKindAdmin,
		RequestID:  "req-publish-mihomo",
	})
	if err != nil {
		t.Fatalf("Publish Mihomo failed: %v", err)
	}

	if pubRes.Publication.Target != domain.TargetMihomo {
		t.Fatalf("expected target mihomo, got %s", pubRes.Publication.Target)
	}
	if pubRes.ContentType != "application/yaml" {
		t.Fatalf("expected content type application/yaml, got %s", pubRes.ContentType)
	}
	if pubRes.Filename != "mihomo.yaml" {
		t.Fatalf("expected filename mihomo.yaml, got %s", pubRes.Filename)
	}
	if pubRes.ContentDigest != previewRes.ContentDigest {
		t.Fatalf("publish ContentDigest %s does not match preview ContentDigest %s", pubRes.ContentDigest, previewRes.ContentDigest)
	}

	// 3. Serve via ResolveAndServe with valid token -> returns exact artifact
	art, err := svc.ResolveAndServe(ctx, pubRes.Publication.ID, pubRes.RawToken)
	if err != nil {
		t.Fatalf("ResolveAndServe failed: %v", err)
	}
	if art.ContentDigest != pubRes.ContentDigest {
		t.Fatalf("served ContentDigest %s does not match published %s", art.ContentDigest, pubRes.ContentDigest)
	}
	if string(art.Content) != previewContent {
		t.Fatal("served artifact content does not match preview content")
	}

	// 4. Audit security: verify secrets and raw tokens are not leaked
	auditRepo.mu.Lock()
	for _, ev := range auditRepo.events {
		if strings.Contains(ev.RedactedSummary, pubRes.RawToken) {
			t.Fatalf("audit event leaked raw token: %s", ev.RedactedSummary)
		}
		if strings.Contains(ev.RedactedSummary, "hy2-super-secret-password") || strings.Contains(ev.RedactedSummary, "ss-super-secret-password") {
			t.Fatalf("audit event leaked password: %s", ev.RedactedSummary)
		}
	}
	auditRepo.mu.Unlock()

	// 5. Simulated process restart: clear in-memory artifacts map and re-resolve
	// We instantiate a fresh service pointing to the same repositories
	restartedSvc := publication.NewService(
		pubRepo,
		auditRepo,
		publication.WithNodeRepository(nodeRepo),
		publication.WithCredentialSource(vault, credRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithPolicyRepository(policyRepo),
	)

	recoveredArt, err := restartedSvc.ResolveAndServe(ctx, pubRes.Publication.ID, pubRes.RawToken)
	if err != nil {
		t.Fatalf("restart recovery ResolveAndServe failed: %v", err)
	}
	if recoveredArt.ContentDigest != pubRes.ContentDigest {
		t.Fatalf("recovered ContentDigest %s does not match original %s", recoveredArt.ContentDigest, pubRes.ContentDigest)
	}
	if string(recoveredArt.Content) != previewContent {
		t.Fatal("recovered content does not match original preview content")
	}
}

func TestMihomoPublishAndPreview_FailClosedScenarios(t *testing.T) {
	ctx := context.Background()

	// 1. Missing credentials for node in repo fails closed
	t.Run("MissingCredentialsFailClosed", func(t *testing.T) {
		vault, credRepo, nodeRepo, snapshot := setupVaultAndNodes(t)
		hy2ID := snapshot.Nodes[0].LogicalID
		_ = credRepo.DeleteByLogicalID(ctx, hy2ID) // remove credentials for hy2

		svc := publication.NewService(
			newMockPublicationRepo(),
			&mockAuditRepo{},
			publication.WithNodeRepository(nodeRepo),
			publication.WithCredentialSource(vault, credRepo),
		)

		_, err := svc.Publish(ctx, publication.PublishCommand{
			Target:    domain.TargetMihomo,
			Snapshot:  snapshot,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-missing-cred",
		})
		if err == nil {
			t.Fatal("expected missing credentials to fail closed")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "unsupported_target_capability" {
			t.Fatalf("expected unsupported_target_capability, got %v", err)
		}
	})

	// 2. Unconfigured vault/credRepo fails closed
	t.Run("UnconfiguredVaultFailsClosed", func(t *testing.T) {
		_, _, nodeRepo, snapshot := setupVaultAndNodes(t)
		svc := publication.NewService(
			newMockPublicationRepo(),
			&mockAuditRepo{},
			publication.WithNodeRepository(nodeRepo),
			// no WithCredentialSource
		)

		_, err := svc.Publish(ctx, publication.PublishCommand{
			Target:    domain.TargetMihomo,
			Snapshot:  snapshot,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-no-vault",
		})
		if err == nil {
			t.Fatal("expected unconfigured vault to fail closed")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "unsupported_target_capability" {
			t.Fatalf("expected unsupported_target_capability, got %v", err)
		}
	})

	// 3. Credential version mismatch fails closed
	t.Run("VersionMismatchFailsClosed", func(t *testing.T) {
		vault, credRepo, nodeRepo, snapshot := setupVaultAndNodes(t)
		hy2ID := snapshot.Nodes[0].LogicalID
		// Bump version in nodeRepo without updating credRepo
		node, _ := nodeRepo.GetByLogicalID(ctx, hy2ID)
		node.CredentialVersion = 2
		_ = nodeRepo.UpsertBatch(ctx, []domain.Node{*node})

		svc := publication.NewService(
			newMockPublicationRepo(),
			&mockAuditRepo{},
			publication.WithNodeRepository(nodeRepo),
			publication.WithCredentialSource(vault, credRepo),
		)

		_, err := svc.Publish(ctx, publication.PublishCommand{
			Target:    domain.TargetMihomo,
			Snapshot:  snapshot,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-version-mismatch",
		})
		if err == nil {
			t.Fatal("expected version mismatch to fail closed")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "unsupported_target_capability" {
			t.Fatalf("expected unsupported_target_capability, got %v", err)
		}
	})

	// 4. Tampered ciphertext fails closed without secret leak
	t.Run("TamperedCiphertextFailsClosed", func(t *testing.T) {
		vault, credRepo, nodeRepo, snapshot := setupVaultAndNodes(t)
		hy2ID := snapshot.Nodes[0].LogicalID
		rec, _ := credRepo.GetByLogicalID(ctx, hy2ID, 1)
		rec.Ciphertext[0] ^= 0xff // corrupt ciphertext byte
		_ = credRepo.Upsert(ctx, rec)

		svc := publication.NewService(
			newMockPublicationRepo(),
			&mockAuditRepo{},
			publication.WithNodeRepository(nodeRepo),
			publication.WithCredentialSource(vault, credRepo),
		)

		_, err := svc.Publish(ctx, publication.PublishCommand{
			Target:    domain.TargetMihomo,
			Snapshot:  snapshot,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-tampered",
		})
		if err == nil {
			t.Fatal("expected tampered ciphertext to fail closed")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Code != "unsupported_target_capability" {
			t.Fatalf("expected unsupported_target_capability, got %v", err)
		}
		if strings.Contains(err.Error(), "01234567890123456789012345678901") {
			t.Fatal("error message leaked encryption key")
		}
	})

	// 5. Logical ID mismatch fails closed
	t.Run("LogicalIDMismatchFailsClosed", func(t *testing.T) {
		vault, credRepo, nodeRepo, snapshot := setupVaultAndNodes(t)
		hy2ID := snapshot.Nodes[0].LogicalID
		rec, _ := credRepo.GetByLogicalID(ctx, hy2ID, 1)
		_ = credRepo.DeleteByLogicalID(ctx, hy2ID)
		// Put record back under key hy2ID:1 but with mismatched LogicalID field
		rec.LogicalID = "wrong-logical-id"
		credRepo.mu.Lock()
		credRepo.records[hy2ID+":1"] = rec
		credRepo.mu.Unlock()

		svc := publication.NewService(
			newMockPublicationRepo(),
			&mockAuditRepo{},
			publication.WithNodeRepository(nodeRepo),
			publication.WithCredentialSource(vault, credRepo),
		)

		_, err := svc.Publish(ctx, publication.PublishCommand{
			Target:    domain.TargetMihomo,
			Snapshot:  snapshot,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-logical-id-mismatch",
		})
		if err == nil {
			t.Fatal("expected logical ID mismatch to fail closed")
		}
	})
}

func TestResolveAndServe_LegacyClashRetiredContract(t *testing.T) {
	ctx := context.Background()
	pubRepo := newMockPublicationRepo()
	auditRepo := &mockAuditRepo{}

	svc := publication.NewService(pubRepo, auditRepo)

	rawToken := "pub_test_valid_token_123"
	tokenHash := sha256.Sum256([]byte(rawToken))
	pubID := "pub-clash-legacy"

	legacyPub := domain.Publication{
		ID:              pubID,
		Target:          domain.CompilerTarget("clash"), // legacy retired target
		SnapshotDigest:  "snap-legacy-digest",
		CompilerVersion: "1.0.0",
		TokenHash:       hex.EncodeToString(tokenHash[:]),
		State:           domain.PublicationStateActive,
		CreatedAt:       time.Now().UTC(),
	}
	_ = pubRepo.Create(ctx, &legacyPub)

	// 1. Missing token -> 401 Unauthorized (Auth check before retired target check)
	_, err := svc.ResolveAndServe(ctx, pubID, "")
	if !errors.Is(err, publication.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for missing token, got %v", err)
	}

	// 2. Invalid token -> 401 Unauthorized (Auth check before retired target check)
	_, err = svc.ResolveAndServe(ctx, pubID, "pub_invalid_token")
	if !errors.Is(err, publication.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for invalid token, got %v", err)
	}

	// 3. Revoked publication -> 403 Revoked (Revocation check before retired target check)
	now := time.Now().UTC()
	_ = pubRepo.Revoke(ctx, pubID, now)
	_, err = svc.ResolveAndServe(ctx, pubID, rawToken)
	if !errors.Is(err, publication.ErrRevoked) {
		t.Fatalf("expected ErrRevoked for revoked clash publication, got %v", err)
	}

	// 4. Un-revoke to active: with valid token -> returns ErrUnsupportedTarget
	revivedPub := legacyPub
	revivedPub.State = domain.PublicationStateActive
	_ = pubRepo.Create(ctx, &revivedPub)
	pubRepo.mu.Lock()
	pubRepo.publications[pubID] = &revivedPub
	pubRepo.tokenMap[revivedPub.TokenHash] = &revivedPub
	pubRepo.mu.Unlock()

	_, err = svc.ResolveAndServe(ctx, pubID, rawToken)
	if !errors.Is(err, publication.ErrUnsupportedTarget) {
		t.Fatalf("expected ErrUnsupportedTarget for valid token on legacy clash, got %v", err)
	}

	// 5. Admin can still inspect detail with Target="clash"
	detail, err := svc.Get(ctx, pubID)
	if err != nil {
		t.Fatalf("Get publication detail failed: %v", err)
	}
	if detail.Publication.Target != "clash" {
		t.Fatalf("expected Target clash in detail, got %s", detail.Publication.Target)
	}
	if detail.Publication.Target.IsValid() {
		t.Fatal("expected legacy target clash IsValid() to be false")
	}
}

func TestAllFourTargets_UnifiedCredentialVaultAndRestartCredentialRotationGuard(t *testing.T) {
	ctx := context.Background()
	targets := []domain.CompilerTarget{
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	}

	for _, target := range targets {
		t.Run("FullLifecycle_"+string(target), func(t *testing.T) {
			vault, _, _ := setupSampleVaultSource()
			credRepo := newMockCredentialRepo()
			nodeRepo := newMockNodeRepo()
			pubRepo := newMockPublicationRepo()
			auditRepo := &mockAuditRepo{}
			revRepo := newMockRevisionRepo()
			policyRepo := newMockPolicyRepo()

			revID := domain.MustNewUUIDv7()
			groupID := domain.MustNewUUIDv7()
			hkID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "hk.example.com", 443, nil)
			usID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "us.example.com", 8388, nil)
			hkIdentity := domain.NewVerifiedNodeIdentity(hkID, domain.ProtocolTrojan, "hk.example.com", 443, 1, nil)
			usIdentity := domain.NewVerifiedNodeIdentity(usID, domain.ProtocolSS, "us.example.com", 8388, 1, nil)
			now := time.Now().UTC()
			_ = nodeRepo.UpsertBatch(ctx, []domain.Node{
				{LogicalID: hkID, Protocol: domain.ProtocolTrojan, DisplayName: "Hong Kong 01", CredentialVersion: 1, Identity: &hkIdentity, Active: true, CreatedAt: now, UpdatedAt: now},
				{LogicalID: usID, Protocol: domain.ProtocolSS, DisplayName: "United States 01", CredentialVersion: 1, Identity: &usIdentity, Active: true, CreatedAt: now, UpdatedAt: now},
			})
			hkRec, _ := vault.Encrypt(&domain.NodeCredentialPayload{
				LogicalID:   hkID,
				Protocol:    domain.ProtocolTrojan,
				Server:      "hk.example.com",
				Port:        443,
				Version:     1,
				Identity:    &hkIdentity,
				Credentials: domain.InboundProtocolCredential{Password: "hk-trojan-secret-password"},
			})
			usRec, _ := vault.Encrypt(&domain.NodeCredentialPayload{
				LogicalID:   usID,
				Protocol:    domain.ProtocolSS,
				Server:      "us.example.com",
				Port:        8388,
				Version:     1,
				Identity:    &usIdentity,
				Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "us-ss-secret-password"},
			})
			_ = credRepo.Upsert(ctx, hkRec)
			_ = credRepo.Upsert(ctx, usRec)

			_ = revRepo.Create(ctx, &domain.ConfigurationRevision{
				ID:            revID,
				ContentDigest: "sha256:rev-4target",
				State:         domain.RevisionStateActive,
			})
			_ = policyRepo.CreateGroup(ctx, &domain.NodeGroup{
				ID:        groupID,
				Name:      "ProxySelect",
				GroupType: domain.GroupTypeSelect,
			})
			_ = policyRepo.SetEdgesForGroup(ctx, groupID, []domain.GroupEdge{
				{ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &hkID, Position: 0},
				{ID: domain.MustNewUUIDv7(), ParentGroupID: groupID, NodeLogicalID: &usID, Position: 1},
			})
			_ = policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
				ID:            domain.MustNewUUIDv7(),
				RevisionID:    revID,
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Position:      0,
			})

			svc := publication.NewService(
				pubRepo,
				auditRepo,
				publication.WithNodeRepository(nodeRepo),
				publication.WithCredentialSource(vault, credRepo),
				publication.WithRevisionRepository(revRepo),
				publication.WithPolicyRepository(policyRepo),
			)

			// 1. Preflight
			preRes, err := svc.Preflight(ctx, publication.PreflightCommand{
				Target:     target,
				RevisionID: revID,
			})
			if err != nil || !preRes.Allowed {
				t.Fatalf("preflight failed for %s: allowed=%v err=%v", target, preRes != nil && preRes.Allowed, err)
			}

			// 2. Preview
			prevRes, err := svc.Preview(ctx, publication.PreviewQuery{
				Target:     target,
				RevisionID: revID,
			})
			if err != nil {
				t.Fatalf("preview failed for %s: %v", target, err)
			}
			if !strings.Contains(string(prevRes.Content), "hk-trojan-secret-password") || !strings.Contains(string(prevRes.Content), "us-ss-secret-password") {
				t.Fatalf("preview for %s missing decrypted vault credentials:\n%s", target, string(prevRes.Content))
			}

			// 3. Publish
			pubRes, err := svc.Publish(ctx, publication.PublishCommand{
				Target:     target,
				RevisionID: revID,
				ActorKind:  domain.ActorKindAdmin,
				RequestID:  "req-pub-" + string(target),
			})
			if err != nil {
				t.Fatalf("publish failed for %s: %v", target, err)
			}
			if pubRes.ContentDigest != prevRes.ContentDigest {
				t.Fatalf("content digest mismatch on %s: publish=%s preview=%s", target, pubRes.ContentDigest, prevRes.ContentDigest)
			}

			// 4. Restart recovery with unchanged credentials succeeds
			restartedSvc := publication.NewService(
				pubRepo,
				auditRepo,
				publication.WithNodeRepository(nodeRepo),
				publication.WithCredentialSource(vault, credRepo),
				publication.WithRevisionRepository(revRepo),
				publication.WithPolicyRepository(policyRepo),
			)
			art, err := restartedSvc.ResolveAndServe(ctx, pubRes.Publication.ID, pubRes.RawToken)
			if err != nil {
				t.Fatalf("restart recovery failed for %s: %v", target, err)
			}
			if art.ContentDigest != pubRes.ContentDigest || string(art.Content) != string(prevRes.Content) {
				t.Fatalf("restart recovered content mismatch for %s", target)
			}

			// 5. Credential version rotation after publish: restart recovery MUST reject and NOT silently serve v2 credentials
			hkV2Identity := domain.NewVerifiedNodeIdentity(hkID, domain.ProtocolTrojan, "hk.example.com", 443, 2, nil)
			hkNode, _ := nodeRepo.GetByLogicalID(ctx, hkID)
			hkNode.CredentialVersion = 2
			hkNode.Identity = &hkV2Identity
			_ = nodeRepo.UpsertBatch(ctx, []domain.Node{*hkNode})
			v2Rec, err := vault.Encrypt(&domain.NodeCredentialPayload{
				LogicalID: hkID,
				Protocol:  domain.ProtocolTrojan,
				Server:    "hk.example.com",
				Port:      443,
				Version:   2,
				Identity:  &hkV2Identity,
				Credentials: domain.InboundProtocolCredential{
					Password: "hk-trojan-rotated-v2-password",
				},
			})
			if err != nil {
				t.Fatalf("encrypt v2: %v", err)
			}
			_ = credRepo.Upsert(ctx, v2Rec)

			restartedAfterRotationSvc := publication.NewService(
				pubRepo,
				auditRepo,
				publication.WithNodeRepository(nodeRepo),
				publication.WithCredentialSource(vault, credRepo),
				publication.WithRevisionRepository(revRepo),
				publication.WithPolicyRepository(policyRepo),
			)
			_, err = restartedAfterRotationSvc.ResolveAndServe(ctx, pubRes.Publication.ID, pubRes.RawToken)
			if !errors.Is(err, publication.ErrIntegrityCheckFailed) {
				t.Fatalf("expected ErrIntegrityCheckFailed when credential version changed across restart for %s, got %v", target, err)
			}
		})

		t.Run("FailClosedWithoutVault_"+string(target), func(t *testing.T) {
			_, _, nodeRepo := setupSampleVaultSource()
			svcNoVault := publication.NewService(
				newMockPublicationRepo(),
				&mockAuditRepo{},
				publication.WithNodeRepository(nodeRepo),
			)
			snap := buildSampleSnapshot()
			if _, err := svcNoVault.Preflight(ctx, publication.PreflightCommand{Target: target, Snapshot: snap}); err == nil {
				t.Fatalf("expected Preflight without vault to fail closed for %s", target)
			}
			if _, err := svcNoVault.Preview(ctx, publication.PreviewQuery{Target: target, Snapshot: snap}); err == nil {
				t.Fatalf("expected Preview without vault to fail closed for %s", target)
			}
			if _, err := svcNoVault.Publish(ctx, publication.PublishCommand{Target: target, Snapshot: snap}); err == nil {
				t.Fatalf("expected Publish without vault to fail closed for %s", target)
			}
		})
	}
}

func TestZeroNodeEmptySnapshot_ValidAcrossAllFourTargets(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setupService()
	emptySnap := &resolver.ResolvedPolicySnapshot{
		SnapshotDigest:  "sha256:empty-zero-node-snapshot",
		CompilerVersion: "1.0.0",
		Nodes:           []resolver.ResolvedNode{},
		Groups:          []resolver.ResolvedGroup{},
		Rules:           []resolver.ResolvedRule{},
	}

	for _, target := range []domain.CompilerTarget{
		domain.TargetMihomo,
		domain.TargetSingBox,
		domain.TargetSurge,
		domain.TargetQuantumultX,
	} {
		preRes, err := svc.Preflight(ctx, publication.PreflightCommand{Target: target, Snapshot: emptySnap})
		if err != nil || !preRes.Allowed {
			t.Fatalf("expected 0-node empty snapshot preflight to succeed for %s, got allowed=%v err=%v", target, preRes != nil && preRes.Allowed, err)
		}
		prevRes, err := svc.Preview(ctx, publication.PreviewQuery{Target: target, Snapshot: emptySnap})
		if err != nil || len(prevRes.Content) == 0 {
			t.Fatalf("expected 0-node empty snapshot preview to succeed for %s, got err=%v", target, err)
		}
		pubRes, err := svc.Publish(ctx, publication.PublishCommand{
			Target:    target,
			Snapshot:  emptySnap,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-empty-" + string(target),
		})
		if err != nil || pubRes.ContentDigest != prevRes.ContentDigest {
			t.Fatalf("expected 0-node empty snapshot publish to succeed for %s, got err=%v", target, err)
		}
	}
}

func TestResolveAndServe_FailClosedOnMissingBindingsTamperingAndDeletion(t *testing.T) {
	ctx := context.Background()

	t.Run("MissingBindingsLegacyRowReturnsErrIntegrityCheckFailed", func(t *testing.T) {
		vault, credRepo, nodeRepo := setupSampleVaultSource()
		pubRepo := newMockPublicationRepo()
		auditRepo := &mockAuditRepo{}
		svc := publication.NewService(
			pubRepo,
			auditRepo,
			publication.WithNodeRepository(nodeRepo),
			publication.WithCredentialSource(vault, credRepo),
		)

		rawToken := "pub_legacy_unmigrated_token_001"
		sum := sha256.Sum256([]byte(rawToken))
		legacyPub := domain.Publication{
			ID:              "0191e4a0-0000-7000-8000-000000000801",
			Target:          domain.TargetMihomo,
			SnapshotDigest:  "sha256:legacy-snap",
			CompilerVersion: "1.0.0",
			TokenHash:       hex.EncodeToString(sum[:]),
			State:           domain.PublicationStateActive,
			CreatedAt:       time.Now().UTC(),
		}
		_ = pubRepo.Create(ctx, &legacyPub)

		_, err := svc.ResolveAndServe(ctx, legacyPub.ID, rawToken)
		if !errors.Is(err, publication.ErrIntegrityCheckFailed) {
			t.Fatalf("expected ErrIntegrityCheckFailed for legacy row missing bindings, got %v", err)
		}
	})

	t.Run("DeletedCredentialOrTamperedArtifactReturnsErrIntegrityCheckFailed", func(t *testing.T) {
		vault, credRepo, nodeRepo := setupSampleVaultSource()
		pubRepo := newMockPublicationRepo()
		auditRepo := &mockAuditRepo{}
		svc := publication.NewService(
			pubRepo,
			auditRepo,
			publication.WithNodeRepository(nodeRepo),
			publication.WithCredentialSource(vault, credRepo),
		)

		snap := buildSampleSnapshot()
		pubRes, err := svc.Publish(ctx, publication.PublishCommand{
			Target:    domain.TargetMihomo,
			Snapshot:  snap,
			ActorKind: domain.ActorKindAdmin,
			RequestID: "req-integrity-check",
		})
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}

		// Verify Get from restarted service returns persisted metadata
		restartedSvc := publication.NewService(
			pubRepo,
			auditRepo,
			publication.WithNodeRepository(nodeRepo),
			publication.WithCredentialSource(vault, credRepo),
		)
		detail, err := restartedSvc.Get(ctx, pubRes.Publication.ID)
		if err != nil {
			t.Fatalf("Get after restart failed: %v", err)
		}
		if detail.ContentDigest != pubRes.ContentDigest || detail.Filename != "mihomo.yaml" || detail.ContentType != "application/yaml" {
			t.Fatalf("unexpected Get metadata after restart: %+v", detail)
		}

		// Tamper with ArtifactCiphertext in repo -> ErrIntegrityCheckFailed
		pubRepo.mu.Lock()
		origCipher := append([]byte(nil), pubRepo.publications[pubRes.Publication.ID].ArtifactCiphertext...)
		pubRepo.publications[pubRes.Publication.ID].ArtifactCiphertext[0] ^= 0xff
		pubRepo.mu.Unlock()

		_, err = restartedSvc.ResolveAndServe(ctx, pubRes.Publication.ID, pubRes.RawToken)
		if !errors.Is(err, publication.ErrIntegrityCheckFailed) {
			t.Fatalf("expected ErrIntegrityCheckFailed on tampered ciphertext, got %v", err)
		}

		// Restore ciphertext, then delete credential from vault repo -> ErrIntegrityCheckFailed
		pubRepo.mu.Lock()
		pubRepo.publications[pubRes.Publication.ID].ArtifactCiphertext = origCipher
		pubRepo.mu.Unlock()

		_ = credRepo.DeleteByLogicalID(ctx, "node-hk-01")
		_, err = restartedSvc.ResolveAndServe(ctx, pubRes.Publication.ID, pubRes.RawToken)
		if !errors.Is(err, publication.ErrIntegrityCheckFailed) {
			t.Fatalf("expected ErrIntegrityCheckFailed on deleted node credential, got %v", err)
		}
	})
}
