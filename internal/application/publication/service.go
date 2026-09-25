package publication

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// Service coordinates publication creation, export token generation, active revocation,
// and deterministic client export delivery.
type Service struct {
	pubRepo      domain.PublicationRepository
	auditRepo    domain.AuditRepository
	policyRepo   domain.PolicyRepository
	revisionRepo domain.RevisionRepository
	nodeRepo     domain.NodeRepository
	resolver     resolver.Resolver

	ipriskSvc   *iprisk.Service
	riskPolicy  domain.RiskPolicyRevisionRepository
	riskBinding domain.RiskPolicyGroupBindingRepository
	riskObs     domain.IPRiskObservationRepository

	nodeFilterRepo domain.NodeFilterRepository
	sourceRepo     domain.NodeSourceRepository
	obsRepo        domain.ProbeObservationRepository

	mu        sync.RWMutex
	artifacts map[string]*Artifact // keyed by publication ID
}

// Option configures optional Service dependencies.
type Option func(*Service)

// WithPolicyRepository sets the policy repository for resolving snapshots.
func WithPolicyRepository(repo domain.PolicyRepository) Option {
	return func(s *Service) {
		s.policyRepo = repo
	}
}

// WithRevisionRepository sets the revision repository for resolving snapshots.
func WithRevisionRepository(repo domain.RevisionRepository) Option {
	return func(s *Service) {
		s.revisionRepo = repo
	}
}

// WithNodeRepository sets the node repository for resolving snapshots.
func WithNodeRepository(repo domain.NodeRepository) Option {
	return func(s *Service) {
		s.nodeRepo = repo
	}
}

// WithResolver sets the deterministic policy resolver implementation.
func WithResolver(res resolver.Resolver) Option {
	return func(s *Service) {
		s.resolver = res
	}
}

// WithIPRiskService sets the IP risk decision service for preflight evaluation and recomputation.
func WithIPRiskService(svc *iprisk.Service) Option {
	return func(s *Service) {
		s.ipriskSvc = svc
	}
}

// WithRiskPolicyRepository sets the risk policy revision repository.
func WithRiskPolicyRepository(repo domain.RiskPolicyRevisionRepository) Option {
	return func(s *Service) {
		s.riskPolicy = repo
	}
}

// WithRiskBindingRepository sets the risk policy group binding repository.
func WithRiskBindingRepository(repo domain.RiskPolicyGroupBindingRepository) Option {
	return func(s *Service) {
		s.riskBinding = repo
	}
}

// WithRiskObservationRepository sets the risk observation repository.
func WithRiskObservationRepository(repo domain.IPRiskObservationRepository) Option {
	return func(s *Service) {
		s.riskObs = repo
	}
}

// WithNodeFilterRepository sets the node filter repository for snapshot resolution.
func WithNodeFilterRepository(repo domain.NodeFilterRepository) Option {
	return func(s *Service) {
		s.nodeFilterRepo = repo
	}
}

// WithNodeSourceRepository sets the node source repository for snapshot resolution.
func WithNodeSourceRepository(repo domain.NodeSourceRepository) Option {
	return func(s *Service) {
		s.sourceRepo = repo
	}
}

// WithProbeObservationRepository sets the probe observation repository for snapshot resolution.
func WithProbeObservationRepository(repo domain.ProbeObservationRepository) Option {
	return func(s *Service) {
		s.obsRepo = repo
	}
}

// SetNodeFilterRepository allows dynamic configuration of the node filter repository.
func (s *Service) SetNodeFilterRepository(repo domain.NodeFilterRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodeFilterRepo = repo
}

// SetNodeSourceRepository allows dynamic configuration of the node source repository.
func (s *Service) SetNodeSourceRepository(repo domain.NodeSourceRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sourceRepo = repo
}

// SetProbeObservationRepository allows dynamic configuration of the probe observation repository.
func (s *Service) SetProbeObservationRepository(repo domain.ProbeObservationRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.obsRepo = repo
}

// NewService constructs a new publication application service.
func NewService(pubRepo domain.PublicationRepository, auditRepo domain.AuditRepository, opts ...Option) *Service {
	s := &Service{
		pubRepo:   pubRepo,
		auditRepo: auditRepo,
		resolver:  resolver.New(),
		artifacts: make(map[string]*Artifact),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.ipriskSvc == nil && s.riskPolicy != nil && s.riskObs != nil {
		ipriskOpts := make([]iprisk.Option, 0)
		if s.riskBinding != nil {
			ipriskOpts = append(ipriskOpts, iprisk.WithBindingRepository(s.riskBinding))
		}
		if s.nodeRepo != nil {
			ipriskOpts = append(ipriskOpts, iprisk.WithNodeRepository(s.nodeRepo))
		}
		if s.policyRepo != nil {
			ipriskOpts = append(ipriskOpts, iprisk.WithGroupRepository(s.policyRepo))
		}
		if s.auditRepo != nil {
			ipriskOpts = append(ipriskOpts, iprisk.WithAuditRepository(s.auditRepo))
		}
		s.ipriskSvc = iprisk.NewService(s.riskObs, s.riskPolicy, ipriskOpts...)
	}
	return s
}

// Preflight evaluates whether a resolved snapshot can be published safely, returning actionable diagnostics.
func (s *Service) Preflight(ctx context.Context, cmd PreflightCommand) (*PreflightResult, error) {
	if !isValidTarget(cmd.Target) {
		return nil, domain.NewValidationError("unsupported_target", fmt.Sprintf("unsupported compiler target: %s", cmd.Target))
	}

	snapshot := cmd.Snapshot
	if snapshot == nil {
		var err error
		snapshot, err = s.resolveSnapshot(ctx, cmd.RevisionID)
		if err != nil {
			return nil, err
		}
	}

	preflight := s.evaluatePreflight(ctx, snapshot)
	return &preflight, nil
}

// Publish creates an immutable publication for a target compiler from a resolved policy snapshot.
func (s *Service) Publish(ctx context.Context, cmd PublishCommand) (*PublishResult, error) {
	if !isValidTarget(cmd.Target) {
		return nil, domain.NewValidationError("unsupported_target", fmt.Sprintf("unsupported compiler target: %s", cmd.Target))
	}

	snapshot := cmd.Snapshot
	if snapshot == nil {
		var err error
		snapshot, err = s.resolveSnapshot(ctx, cmd.RevisionID)
		if err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, fmt.Sprintf("failed to resolve snapshot: %v", err))
			return nil, err
		}
	}

	preflight := s.evaluatePreflight(ctx, snapshot)
	if !preflight.Allowed {
		err := newPreflightError(preflight)
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, preflightSummary(snapshot, preflight))
		return nil, err
	}

	compileRes, err := compiler.Compile(ctx, snapshot, cmd.Target)
	if err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, fmt.Sprintf("compiler failed for target %s: %v", cmd.Target, err))
		var capErr *compiler.CapabilityError
		if errors.As(err, &capErr) {
			return nil, domain.NewValidationError("unsupported_target_capability", capErr.Error())
		}
		return nil, err
	}

	rawToken, tokenHash, err := generateExportToken()
	if err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, "token generation failed")
		return nil, err
	}

	pubID, err := domain.NewUUIDv7()
	if err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, "UUIDv7 generation failed")
		return nil, err
	}

	compilerVer := snapshot.CompilerVersion
	if compilerVer == "" {
		compilerVer = "1.0.0"
	}

	now := domain.NowUTC()
	pub := domain.Publication{
		ID:              pubID,
		Target:          cmd.Target,
		SnapshotDigest:  snapshot.SnapshotDigest,
		CompilerVersion: compilerVer,
		TokenHash:       tokenHash,
		State:           domain.PublicationStateActive,
		CreatedAt:       now,
	}

	if err := s.pubRepo.Create(ctx, &pub); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, fmt.Sprintf("persistence failed: %v", err))
		return nil, err
	}

	artifact := &Artifact{
		PublicationID:  pubID,
		Target:         cmd.Target,
		Content:        compileRes.Content,
		ContentType:    compileRes.ContentType,
		Filename:       compileRes.Filename,
		ContentDigest:  compileRes.ContentDigest,
		SnapshotDigest: compileRes.SnapshotDigest,
	}

	s.mu.Lock()
	s.artifacts[pubID] = artifact
	s.mu.Unlock()

	summary := fmt.Sprintf("created immutable publication %s for target %s (snapshot: %s, content: %s)", pub.ID, pub.Target, pub.SnapshotDigest, compileRes.ContentDigest)
	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultSuccess, summary)

	return &PublishResult{
		Publication:    pub,
		RawToken:       rawToken,
		ExportURL:      "/publish/v1/" + pubID + "?token=" + rawToken,
		ContentDigest:  compileRes.ContentDigest,
		SnapshotDigest: compileRes.SnapshotDigest,
		ContentType:    compileRes.ContentType,
		Filename:       compileRes.Filename,
		Size:           len(compileRes.Content),
	}, nil
}

// Preview renders compiled client configuration without persisting a publication,
// using the identical ResolvedPolicySnapshot semantics.
func (s *Service) Preview(ctx context.Context, query PreviewQuery) (*PreviewResult, error) {
	if !isValidTarget(query.Target) {
		return nil, domain.NewValidationError("unsupported_target", fmt.Sprintf("unsupported compiler target: %s", query.Target))
	}

	snapshot := query.Snapshot
	if snapshot == nil {
		var err error
		snapshot, err = s.resolveSnapshot(ctx, query.RevisionID)
		if err != nil {
			return nil, err
		}
	}

	compileRes, err := compiler.Compile(ctx, snapshot, query.Target)
	if err != nil {
		var capErr *compiler.CapabilityError
		if errors.As(err, &capErr) {
			return nil, domain.NewValidationError("unsupported_target_capability", capErr.Error())
		}
		return nil, err
	}

	return &PreviewResult{
		Target:         query.Target,
		SnapshotDigest: compileRes.SnapshotDigest,
		ContentDigest:  compileRes.ContentDigest,
		Content:        compileRes.Content,
		ContentType:    compileRes.ContentType,
		Filename:       compileRes.Filename,
		Diagnostics:    snapshot.Diagnostics,
		FilterCounts:   snapshot.FilterCounts,
	}, nil
}

// Revoke actively revokes an existing publication, ensuring immediate rejection on export endpoints.
func (s *Service) Revoke(ctx context.Context, cmd RevokeCommand) (*domain.Publication, error) {
	pub, err := s.pubRepo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	now := domain.NowUTC()
	if err := pub.Revoke(now); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.revoke", domain.AuditResultFailure, fmt.Sprintf("revoke failed: %v", err))
		return nil, err
	}

	if err := s.pubRepo.Revoke(ctx, cmd.ID, now); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.revoke", domain.AuditResultFailure, fmt.Sprintf("persistence revoke failed: %v", err))
		return nil, err
	}

	s.mu.Lock()
	delete(s.artifacts, cmd.ID)
	s.mu.Unlock()

	summary := fmt.Sprintf("revoked publication %s (target: %s, snapshot: %s)", pub.ID, pub.Target, pub.SnapshotDigest)
	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.revoke", domain.AuditResultSuccess, summary)

	return pub, nil
}

// Get retrieves detailed publication state.
func (s *Service) Get(ctx context.Context, id string) (*PublicationDetail, error) {
	pub, err := s.pubRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	detail := &PublicationDetail{
		Publication: *pub,
		ExportURL:   "/publish/v1/" + pub.ID,
	}

	s.mu.RLock()
	art, ok := s.artifacts[id]
	s.mu.RUnlock()

	if ok && art != nil {
		detail.ContentDigest = art.ContentDigest
		detail.SnapshotDigest = art.SnapshotDigest
		detail.ContentType = art.ContentType
		detail.Filename = art.Filename
	}

	return detail, nil
}

// ResolveAndServe resolves and serves the immutable compiled artifact for client subscription endpoints.
// If the publication is revoked or the token is invalid, it returns a domain error without fallback.
func (s *Service) ResolveAndServe(ctx context.Context, publicationID, token string) (*Artifact, error) {
	if strings.TrimSpace(publicationID) == "" {
		return nil, ErrNotFound
	}

	pub, err := s.pubRepo.GetByID(ctx, publicationID)
	if err != nil {
		return nil, ErrNotFound
	}

	// 1. Check revocation: revoked publications must immediately fail with ErrRevoked, never fallback!
	if pub.State == domain.PublicationStateRevoked || !pub.IsActive() {
		return nil, ErrRevoked
	}

	// 2. Validate Target-bound Export Token
	if strings.TrimSpace(token) == "" {
		return nil, ErrUnauthorized
	}

	tokenHash := hashToken(token)
	if subtle.ConstantTimeCompare([]byte(pub.TokenHash), []byte(tokenHash)) != 1 {
		return nil, ErrUnauthorized
	}

	// 3. Retrieve immutable compiled artifact
	s.mu.RLock()
	art, ok := s.artifacts[publicationID]
	s.mu.RUnlock()

	if ok && art != nil {
		return art, nil
	}

	// 4. If artifact is not in memory (e.g. process restart), attempt deterministic recompile
	snapshot, err := s.resolveSnapshot(ctx, "")
	if err == nil && snapshot != nil && snapshot.SnapshotDigest == pub.SnapshotDigest {
		res, err := compiler.Compile(ctx, snapshot, pub.Target)
		if err == nil {
			recoveredArt := &Artifact{
				PublicationID:  pub.ID,
				Target:         pub.Target,
				Content:        res.Content,
				ContentType:    res.ContentType,
				Filename:       res.Filename,
				ContentDigest:  res.ContentDigest,
				SnapshotDigest: res.SnapshotDigest,
			}
			s.mu.Lock()
			s.artifacts[publicationID] = recoveredArt
			s.mu.Unlock()
			return recoveredArt, nil
		}
	}

	return nil, ErrNotFound
}

// IsPublicationToken checks if the provided bearer/query token is recognized as a publication export token.
// This is called by AdminAuthMiddleware to strictly forbid publication export tokens from administrative API access.
func (s *Service) IsPublicationToken(ctx context.Context, token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	if strings.HasPrefix(token, "pub_") {
		return true
	}
	tokenHash := hashToken(token)
	pub, err := s.pubRepo.GetByTokenHash(ctx, tokenHash)
	return err == nil && pub != nil
}

// ValidateToken validates whether a token belongs to a specific publication or any active publication.
func (s *Service) ValidateToken(ctx context.Context, publicationID, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, nil
	}
	tokenHash := hashToken(token)

	if publicationID != "" {
		pub, err := s.pubRepo.GetByID(ctx, publicationID)
		if err != nil {
			return false, nil
		}
		if !pub.IsActive() {
			return false, nil
		}
		return subtle.ConstantTimeCompare([]byte(pub.TokenHash), []byte(tokenHash)) == 1, nil
	}

	pub, err := s.pubRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil || pub == nil {
		return false, nil
	}
	return pub.IsActive(), nil
}

func (s *Service) resolveSnapshot(ctx context.Context, revisionID string) (*resolver.ResolvedPolicySnapshot, error) {
	if s.policyRepo == nil || s.revisionRepo == nil || s.nodeRepo == nil || s.resolver == nil {
		return nil, domain.NewInternalError("missing_dependencies", "policy, revision, or node repository not configured")
	}

	var revID string
	if revisionID != "" {
		rev, err := s.revisionRepo.GetByID(ctx, revisionID)
		if err != nil {
			var de *domain.DomainError
			if errors.As(err, &de) && de.Category == domain.CategoryNotFound {
				return nil, domain.NewNotFoundError("revision_not_found", fmt.Sprintf("configuration revision %s not found", revisionID))
			}
			return nil, err
		}
		if rev == nil {
			return nil, domain.NewNotFoundError("revision_not_found", fmt.Sprintf("configuration revision %s not found", revisionID))
		}
		revID = rev.ID
	} else {
		activeRev, err := s.revisionRepo.GetActive(ctx)
		if err != nil {
			var de *domain.DomainError
			if errors.As(err, &de) && de.Category == domain.CategoryNotFound {
				return nil, domain.NewConflictError("no_active_revision", "cannot resolve policy without an active configuration revision")
			}
			return nil, err
		}
		if activeRev == nil {
			return nil, domain.NewConflictError("no_active_revision", "cannot resolve policy without an active configuration revision")
		}
		revID = activeRev.ID
	}

	groups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list policy groups: %w", err)
	}

	edgeMap := make(map[string][]domain.GroupEdge, len(groups))
	for _, g := range groups {
		edges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list edges for group %s: %w", g.ID, err)
		}
		edgeMap[g.ID] = edges
	}

	policyRules, err := s.policyRepo.ListPolicyRules(ctx, revID)
	if err != nil {
		return nil, fmt.Errorf("failed to list policy rules for revision %s: %w", revID, err)
	}

	admissionRules, err := s.policyRepo.ListAdmissionRules(ctx, revID)
	if err != nil {
		return nil, fmt.Errorf("failed to list admission rules for revision %s: %w", revID, err)
	}

	nodes, _, err := s.nodeRepo.List(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list active nodes: %w", err)
	}

	var globalFilter *domain.NodeFilterSpec
	var groupFilters map[string]domain.NodeFilterSpec
	if s.nodeFilterRepo != nil {
		gf, err := s.nodeFilterRepo.GetGlobalFilter(ctx)
		if err == nil && gf != nil && !gf.Spec.IsEmpty() {
			globalFilter = &gf.Spec
		}
		gfs, err := s.nodeFilterRepo.ListGroupFilters(ctx)
		if err == nil {
			groupFilters = gfs
		}
	}

	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.LogicalID
	}

	var nodeSources map[string][]domain.NodeSource
	if s.sourceRepo != nil && len(nodeIDs) > 0 {
		sources, err := s.sourceRepo.ListByNodes(ctx, nodeIDs)
		if err == nil {
			nodeSources = sources
		}
	}

	var latestObs map[string]map[domain.ProbeKind]domain.ProbeObservation
	if s.obsRepo != nil && len(nodeIDs) > 0 {
		obs, err := s.obsRepo.ListLatestByNodes(ctx, nodeIDs, nil)
		if err == nil {
			latestObs = obs
		}
	}

	input := resolver.ResolveInput{
		RevisionID:         revID,
		InventoryWatermark: "v1",
		CompilerVersion:    "1.0.0",
		Nodes:              nodes,
		Groups:             groups,
		Edges:              edgeMap,
		PolicyRules:        policyRules,
		AdmissionRules:     admissionRules,
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		},
		GlobalFilter:       globalFilter,
		GroupFilters:       groupFilters,
		NodeSources:        nodeSources,
		LatestObservations: latestObs,
		AsOf:               domain.NowUTC(),
	}

	if s.ipriskSvc != nil {
		activePolicy, pErr := s.ipriskSvc.GetActivePolicy(ctx)
		if pErr == nil && activePolicy != nil && activePolicy.Active {
			nodeIDs := make([]string, len(nodes))
			for i, n := range nodes {
				nodeIDs[i] = n.LogicalID
			}
			batchRes, bErr := s.ipriskSvc.EvaluateBatch(ctx, iprisk.EvaluateBatchInput{
				NodeLogicalIDs:   nodeIDs,
				Policy:           &activePolicy.RiskPolicy,
				PolicyRevisionID: activePolicy.RevisionID,
				EvaluatedAt:      domain.NowUTC(),
			})
			if bErr == nil && batchRes != nil {
				input.RiskPolicyRevision = activePolicy.RevisionID
				input.RiskDecisionDigest = batchRes.DecisionDigest
				evalAt := batchRes.EvaluatedAt
				input.RiskEvaluatedAt = &evalAt
				input.RiskReviewAction = activePolicy.EffectiveReviewAction()
				input.RiskDecisions = batchRes.Decisions
			}
		}
	}

	return s.resolver.Resolve(ctx, input)
}

func (s *Service) evaluatePreflight(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot) PreflightResult {
	result := PreflightResult{Allowed: true, Diagnostics: make([]PreflightDiagnostic, 0)}
	if snapshot == nil {
		return result
	}
	result.SnapshotDigest = snapshot.SnapshotDigest
	result.PolicyRevision = snapshot.RiskPolicyRevision

	seen := make(map[string]struct{})
	addDiagnostic := func(d PreflightDiagnostic) {
		key := d.Code + ":" + d.Target
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result.Diagnostics = append(result.Diagnostics, d)
	}

	// 1. Inspect existing snapshot diagnostics
	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.Code == "risk_blocked" || diagnostic.Code == "risk_review" || diagnostic.Code == "risk_unknown" || diagnostic.Severity == resolver.DiagnosticSeverityError || diagnostic.Code == "empty_routed_group" {
			result.Allowed = false
			addDiagnostic(PreflightDiagnostic{
				Severity: diagnostic.Severity,
				Code:     diagnostic.Code,
				Message:  diagnostic.Message,
				Target:   diagnostic.Target,
			})
		}
	}

	// 2. Perform real-time risk recomputation over snapshot members
	if s.ipriskSvc != nil {
		var policyRev *domain.RiskPolicyRevision
		if snapshot.RiskPolicyRevision != "" {
			policyRev, _ = s.ipriskSvc.GetPolicy(ctx, snapshot.RiskPolicyRevision)
		}
		if policyRev == nil || !policyRev.Active {
			policyRev, _ = s.ipriskSvc.GetActivePolicy(ctx)
		}

		if policyRev != nil && policyRev.Active {
			if result.PolicyRevision == "" {
				result.PolicyRevision = policyRev.RevisionID
			}

			nodeIDSet := make(map[string]struct{})
			for _, n := range snapshot.Nodes {
				nodeIDSet[n.LogicalID] = struct{}{}
			}
			for _, g := range snapshot.Groups {
				for _, nid := range g.NodeLogicalIDs {
					nodeIDSet[nid] = struct{}{}
				}
				for _, nid := range g.AllNodeLogicalIDs {
					nodeIDSet[nid] = struct{}{}
				}
			}

			nodeIDs := make([]string, 0, len(nodeIDSet))
			for nid := range nodeIDSet {
				if domain.IsValidLogicalID(nid) {
					nodeIDs = append(nodeIDs, nid)
				}
			}
			sort.Strings(nodeIDs)

			if len(nodeIDs) > 0 {
				batchRes, err := s.ipriskSvc.EvaluateBatch(ctx, iprisk.EvaluateBatchInput{
					NodeLogicalIDs:   nodeIDs,
					Policy:           &policyRev.RiskPolicy,
					PolicyRevisionID: policyRev.RevisionID,
					EvaluatedAt:      domain.NowUTC(),
				})
				if err == nil && batchRes != nil {
					reviewAction := policyRev.EffectiveReviewAction()
					unknownAction := policyRev.EffectiveUnknownAction()

					for _, decision := range batchRes.Decisions {
						switch decision.Decision {
						case domain.RiskActionBlock:
							result.Allowed = false
							addDiagnostic(PreflightDiagnostic{
								Severity: resolver.DiagnosticSeverityError,
								Code:     "risk_blocked",
								Message:  fmt.Sprintf("node %s was blocked by risk policy: %s", decision.NodeLogicalID, decision.ReasonCode),
								Target:   decision.NodeLogicalID,
							})
						case domain.RiskActionReview:
							if reviewAction != domain.RiskActionAllow {
								result.Allowed = false
								addDiagnostic(PreflightDiagnostic{
									Severity: resolver.DiagnosticSeverityWarning,
									Code:     "risk_review",
									Message:  fmt.Sprintf("node %s requires risk review and cannot be published: %s", decision.NodeLogicalID, decision.ReasonCode),
									Target:   decision.NodeLogicalID,
								})
							}
						case domain.RiskActionUnknown:
							if unknownAction != domain.RiskActionAllow {
								result.Allowed = false
								addDiagnostic(PreflightDiagnostic{
									Severity: resolver.DiagnosticSeverityWarning,
									Code:     "risk_unknown",
									Message:  fmt.Sprintf("node %s has unknown risk and cannot be published: %s", decision.NodeLogicalID, decision.ReasonCode),
									Target:   decision.NodeLogicalID,
								})
							}
						}
					}
				}
			}
		}
	}

	return result
}

func newPreflightError(result PreflightResult) error {
	return &PreflightError{Result: result}
}

func preflightSummary(snapshot *resolver.ResolvedPolicySnapshot, result PreflightResult) string {
	codes := make([]string, 0, len(result.Diagnostics))
	for _, d := range result.Diagnostics {
		if d.Code != "" {
			codes = append(codes, d.Code)
		}
	}
	snapDigest := ""
	if snapshot != nil {
		snapDigest = snapshot.SnapshotDigest
	}
	return fmt.Sprintf("publication preflight rejected by policy revision %s (snapshot: %s, diagnostics: %d, codes: [%s])",
		result.PolicyRevision, snapDigest, len(result.Diagnostics), strings.Join(codes, ", "))
}

func (s *Service) recordAudit(ctx context.Context, actorKind domain.ActorKind, requestID, action string, result domain.AuditResult, summary string) {
	if s.auditRepo == nil {
		return
	}
	id, err := domain.NewUUIDv7()
	if err != nil {
		return
	}
	if actorKind == "" {
		actorKind = domain.ActorKindSystem
	}
	ev := &domain.AuditEvent{
		ID:              id,
		ActorKind:       actorKind,
		RequestID:       requestID,
		Action:          action,
		Result:          result,
		RedactedSummary: summary,
		CreatedAt:       domain.NowUTC(),
	}
	_ = s.auditRepo.Record(ctx, ev)
}

func isValidTarget(target domain.CompilerTarget) bool {
	for _, t := range compiler.Targets() {
		if t == target {
			return true
		}
	}
	return false
}

func generateExportToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate crypto random bytes: %w", err)
	}
	rawToken := "pub_" + hex.EncodeToString(b)
	return rawToken, hashToken(rawToken), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
