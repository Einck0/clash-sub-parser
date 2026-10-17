package publication

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
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
	payloadRefRepo domain.PublicationPayloadRefRepository

	subLinkRepo    domain.SubscriptionLinkRepository
	factsLoader    domain.ConsistentFactsLoader
	concurrencySem chan struct{}
}

// Option configures optional Service dependencies.
type Option func(*Service)

// WithPayloadRefRepository sets the publication payload ref repository.
func WithPayloadRefRepository(repo domain.PublicationPayloadRefRepository) Option {
	return func(s *Service) {
		s.payloadRefRepo = repo
	}
}

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

// WithSubscriptionLinkRepository sets the subscription link repository for live subscriptions.
func WithSubscriptionLinkRepository(repo domain.SubscriptionLinkRepository) Option {
	return func(s *Service) {
		s.subLinkRepo = repo
	}
}

// WithConsistentFactsLoader sets the transactional consistent facts loader.
func WithConsistentFactsLoader(loader domain.ConsistentFactsLoader) Option {
	return func(s *Service) {
		s.factsLoader = loader
	}
}

// WithConcurrencyLimit sets the bounded concurrency for live artifact compilation.
func WithConcurrencyLimit(limit int) Option {
	return func(s *Service) {
		if limit > 0 {
			s.concurrencySem = make(chan struct{}, limit)
		}
	}
}

// SetNodeFilterRepository allows dynamic configuration of the node filter repository.
func (s *Service) SetNodeFilterRepository(repo domain.NodeFilterRepository) {
	s.nodeFilterRepo = repo
}

// SetNodeSourceRepository allows dynamic configuration of the node source repository.
func (s *Service) SetNodeSourceRepository(repo domain.NodeSourceRepository) {
	s.sourceRepo = repo
}

// SetProbeObservationRepository allows dynamic configuration of the probe observation repository.
func (s *Service) SetProbeObservationRepository(repo domain.ProbeObservationRepository) {
	s.obsRepo = repo
}

// SetSubscriptionLinkRepository allows dynamic configuration of the subscription link repository.
func (s *Service) SetSubscriptionLinkRepository(repo domain.SubscriptionLinkRepository) {
	s.subLinkRepo = repo
}

// SetConsistentFactsLoader allows dynamic configuration of the consistent facts loader.
func (s *Service) SetConsistentFactsLoader(loader domain.ConsistentFactsLoader) {
	s.factsLoader = loader
}

// NewService constructs a new publication application service.
func NewService(pubRepo domain.PublicationRepository, auditRepo domain.AuditRepository, opts ...Option) *Service {
	s := &Service{
		pubRepo:   pubRepo,
		auditRepo: auditRepo,
		resolver:  resolver.New(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.concurrencySem == nil {
		s.concurrencySem = make(chan struct{}, 16)
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
		snapshot, err = s.resolveTargetSnapshot(ctx, cmd.RevisionID, cmd.Target)
		if err != nil {
			return nil, err
		}
	}

	optInPrune := cmd.PruneUnavailableOptionalGroups || cmd.OmitUnavailableOptionalGroups
	if optInPrune {
		prunedSnap, _, err := compiler.PruneUnavailableOptionalGroups(snapshot)
		if err != nil {
			return nil, err
		}
		snapshot = prunedSnap
	}

	preflight := s.evaluatePreflight(ctx, snapshot, cmd.Target)
	if preflight.Allowed {
		if _, err := compiler.Compile(ctx, snapshot, cmd.Target); err != nil {
			var capErr *compiler.CapabilityError
			if errors.As(err, &capErr) {
				return nil, domain.NewValidationError("unsupported_target_capability", capErr.Error())
			}
			return nil, err
		}
	}
	return &preflight, nil
}

// Publish activates an immutable snapshot draft as an active publication.
func (s *Service) Publish(ctx context.Context, cmd PublishCommand) (*PublishResult, error) {
	if !isValidTarget(cmd.Target) {
		return nil, domain.NewValidationError("unsupported_target", fmt.Sprintf("unsupported compiler target: %s", cmd.Target))
	}

	snapshotID := strings.TrimSpace(cmd.SnapshotID)
	if snapshotID == "" {
		snapshot := cmd.Snapshot
		if snapshot == nil {
			var err error
			snapshot, err = s.resolveTargetSnapshot(ctx, cmd.RevisionID, cmd.Target)
			if err != nil {
				return nil, err
			}
		}

		preflight := s.evaluatePreflight(ctx, snapshot, cmd.Target)
		if !preflight.Allowed {
			err := newPreflightError(preflight)
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, preflightSummary(snapshot, preflight))
			return nil, err
		}

		prevRes, err := s.Preview(ctx, PreviewQuery{
			Target:                         cmd.Target,
			RevisionID:                     cmd.RevisionID,
			Snapshot:                       snapshot,
			PruneUnavailableOptionalGroups: cmd.PruneUnavailableOptionalGroups,
			OmitUnavailableOptionalGroups:  cmd.OmitUnavailableOptionalGroups,
		})
		if err != nil {
			return nil, err
		}
		snapshotID = prevRes.SnapshotID
	}

	pub, err := s.pubRepo.GetByID(ctx, snapshotID)
	if err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, fmt.Sprintf("failed to load snapshot: %v", err))
		return nil, err
	}

	if pub.Target != cmd.Target {
		return nil, domain.NewValidationError("snapshot_target_mismatch", fmt.Sprintf("snapshot target %s does not match requested target %s", pub.Target, cmd.Target))
	}

	if pub.State != domain.PublicationStateDraft && pub.State != domain.PublicationStateActive {
		return nil, domain.NewValidationError("snapshot_not_publishable", fmt.Sprintf("snapshot state %s is not publishable", pub.State))
	}

	if len(pub.Content) == 0 {
		return nil, domain.NewValidationError("snapshot_not_publishable", "snapshot contains errors and is not publishable")
	}

	// Validate frozen content before activation
	if err := ValidatePublicationContent(pub.Content, pub.Target); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, fmt.Sprintf("content validation failed: %v", err))
		return nil, err
	}

	// Idempotent re-activation
	if pub.State == domain.PublicationStateActive {
		rawToken := "active"
		return &PublishResult{
			Publication:    *pub,
			RawToken:       rawToken,
			ExportURL:      "/publish/v1/" + pub.ID + "?token=" + rawToken,
			ContentDigest:  pub.ContentDigest,
			SnapshotDigest: pub.SnapshotDigest,
			ContentType:    pub.ContentType,
			Filename:       pub.Filename,
			Size:           len(pub.Content),
			SnapshotID:     pub.ID,
		}, nil
	}

	rawToken, tokenHash, err := generateExportToken()
	if err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, "token generation failed")
		return nil, err
	}

	if err := s.pubRepo.Activate(ctx, pub.ID, tokenHash); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultFailure, fmt.Sprintf("activation failed: %v", err))
		return nil, err
	}

	pub.State = domain.PublicationStateActive
	pub.TokenHash = tokenHash

	summary := fmt.Sprintf("published immutable publication %s for target %s (snapshot: %s, content: %s)", pub.ID, pub.Target, pub.SnapshotDigest, pub.ContentDigest)
	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.create", domain.AuditResultSuccess, summary)

	return &PublishResult{
		Publication:    *pub,
		RawToken:       rawToken,
		ExportURL:      "/publish/v1/" + pub.ID + "?token=" + rawToken,
		ContentDigest:  pub.ContentDigest,
		SnapshotDigest: pub.SnapshotDigest,
		ContentType:    pub.ContentType,
		Filename:       pub.Filename,
		Size:           len(pub.Content),
		SnapshotID:     pub.ID,
	}, nil
}

// Preview renders compiled client configuration without persisting an active publication,
// generating an immutable draft snapshot with full manifest.
func (s *Service) Preview(ctx context.Context, query PreviewQuery) (*PreviewResult, error) {
	if !isValidTarget(query.Target) {
		return nil, domain.NewValidationError("unsupported_target", fmt.Sprintf("unsupported compiler target: %s", query.Target))
	}

	compatMode := strings.ToLower(strings.TrimSpace(query.CompatMode))
	if compatMode == "" {
		compatMode = "strict"
	}

	snapshot := query.Snapshot
	if snapshot == nil {
		var err error
		snapshot, err = s.resolveTargetSnapshot(ctx, query.RevisionID, query.Target)
		if err != nil {
			return nil, err
		}
	}

	optInPrune := query.PruneUnavailableOptionalGroups || query.OmitUnavailableOptionalGroups
	if optInPrune {
		prunedSnap, _, err := compiler.PruneUnavailableOptionalGroups(snapshot)
		if err != nil {
			return nil, err
		}
		snapshot = prunedSnap
	}

	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.Code == "filtered_nodes_empty" {
			return nil, newPreflightError(s.evaluatePreflight(ctx, snapshot, query.Target))
		}
	}

	included := make([]domain.ManifestIncludedNode, len(snapshot.Nodes))
	for i, n := range snapshot.Nodes {
		included[i] = domain.ManifestIncludedNode{
			NodeID:             n.LogicalID,
			ConnectionRevision: n.ConnectionRevision,
		}
	}

	var payloadIDs []string
	if s.payloadRefRepo != nil && len(included) > 0 {
		resolved, err := s.payloadRefRepo.ResolvePayloadIDsForNodes(ctx, included)
		if err == nil && len(resolved) > 0 {
			payloadIDs = resolved
		}
	}
	manifestPayloadIDs := payloadIDs
	if manifestPayloadIDs == nil {
		manifestPayloadIDs = []string{}
	}

	revID := strings.TrimSpace(query.RevisionID)
	if revID == "" && snapshot.RevisionID != "" {
		revID = strings.TrimSpace(snapshot.RevisionID)
	}

	if compatMode == "strict" {
		diags := compiler.ValidateTargetCapabilities(snapshot, query.Target)
		if len(diags) > 0 {
			draftID := "snapshot_" + domain.MustNewUUIDv7()
			now := domain.NowUTC()
			compilerVer := snapshot.CompilerVersion
			if compilerVer == "" {
				compilerVer = "1.0.0"
			}
			draftPub := domain.Publication{
				ID:              draftID,
				RevisionID:      revID,
				Target:          query.Target,
				SnapshotDigest:  snapshot.SnapshotDigest,
				ContentDigest:   "",
				ContentType:     "",
				Filename:        "",
				Content:         []byte{},
				CompilerVersion: compilerVer,
				TokenHash:       "draft_failed:" + draftID,
				State:           domain.PublicationStateDraft,
				CreatedAt:       now,
			}
			_ = s.pubRepo.Create(ctx, &draftPub)

			code := "unsupported_target_capability"
			msg := "target capability validation failed in strict mode"
			if len(diags) > 0 {
				msg = fmt.Sprintf("target capability validation failed in strict mode: %s", diags[0].Message)
			}
			return nil, &StrictCapabilityError{
				Code:        code,
				Message:     msg,
				Diagnostics: diags,
				SnapshotID:  draftID,
			}
		}

		preflight := s.evaluatePreflight(ctx, snapshot, query.Target)
		if !preflight.Allowed {
			return nil, newPreflightError(preflight)
		}

		compileRes, err := compiler.Compile(ctx, snapshot, query.Target)
		if err != nil {
			var capErr *compiler.CapabilityError
			if errors.As(err, &capErr) {
				return nil, domain.NewValidationError("unsupported_target_capability", capErr.Error())
			}
			return nil, err
		}

		snapshotID := "snapshot_" + domain.MustNewUUIDv7()
		now := domain.NowUTC()
		compilerVer := snapshot.CompilerVersion
		if compilerVer == "" {
			compilerVer = "1.0.0"
		}
		draftPub := domain.Publication{
			ID:              snapshotID,
			RevisionID:      revID,
			Target:          query.Target,
			SnapshotDigest:  snapshot.SnapshotDigest,
			ContentDigest:   compileRes.ContentDigest,
			ContentType:     compileRes.ContentType,
			Filename:        compileRes.Filename,
			Content:         append([]byte(nil), compileRes.Content...),
			CompilerVersion: compilerVer,
			TokenHash:       "draft:" + snapshotID,
			State:           domain.PublicationStateDraft,
			CreatedAt:       now,
		}
		if err := s.pubRepo.Create(ctx, &draftPub); err != nil {
			return nil, fmt.Errorf("failed to persist preview draft: %w", err)
		}

		if s.payloadRefRepo != nil && len(payloadIDs) > 0 {
			_ = s.payloadRefRepo.AddRefs(ctx, snapshotID, payloadIDs)
		}

		manifest := domain.PublicationManifest{
			NodeCount:             len(snapshot.Nodes),
			ExcludedCount:         len(snapshot.ExcludedNodeIDs),
			PayloadIDs:            manifestPayloadIDs,
			ConfigurationRevision: revID,
			RulesDigest:           snapshot.RiskDecisionDigest,
			Included:              included,
			Excluded:              make([]domain.ManifestExcludedNode, 0),
			TargetEngineVersion:   "1.0.0",
			MappingVersion:        "1.0.0",
		}

		previewDiags := append([]resolver.Diagnostic(nil), snapshot.Diagnostics...)
		if len(included) > 0 && len(payloadIDs) == 0 {
			previewDiags = append(previewDiags, resolver.Diagnostic{
				Code:     "legacy_payload_unreferenced",
				Message:  "included nodes have no raw subscription payload reference (legacy or synthetic nodes)",
				Severity: resolver.DiagnosticSeverityWarning,
			})
		}

		return &PreviewResult{
			SnapshotID:     snapshotID,
			Target:         query.Target,
			SnapshotDigest: compileRes.SnapshotDigest,
			ContentDigest:  compileRes.ContentDigest,
			Content:        compileRes.Content,
			ContentType:    compileRes.ContentType,
			Filename:       compileRes.Filename,
			Manifest:       manifest,
			Diagnostics:    previewDiags,
			FilterCounts:   snapshot.FilterCounts,
		}, nil
	}

	// Compatible mode
	compatSnap, excludedNodes := compiler.FilterCompatibleSnapshot(snapshot, query.Target)
	if query.Target == domain.TargetMihomo {
		for _, g := range compatSnap.Groups {
			if !g.UsesEmptyPass() && len(g.Members) == 0 {
				return nil, domain.NewValidationError("empty_group_not_allowed", fmt.Sprintf("policy group %q has 0 members in compatible mode", g.Name))
			}
		}
	}

	compileRes, err := compiler.Compile(ctx, compatSnap, query.Target)
	if err != nil {
		return nil, err
	}

	snapshotID := "snapshot_" + domain.MustNewUUIDv7()
	now := domain.NowUTC()
	compilerVer := compatSnap.CompilerVersion
	if compilerVer == "" {
		compilerVer = "1.0.0"
	}
	draftPub := domain.Publication{
		ID:              snapshotID,
		RevisionID:      revID,
		Target:          query.Target,
		SnapshotDigest:  compatSnap.SnapshotDigest,
		ContentDigest:   compileRes.ContentDigest,
		ContentType:     compileRes.ContentType,
		Filename:        compileRes.Filename,
		Content:         append([]byte(nil), compileRes.Content...),
		CompilerVersion: compilerVer,
		TokenHash:       "draft:" + snapshotID,
		State:           domain.PublicationStateDraft,
		CreatedAt:       now,
	}
	if err := s.pubRepo.Create(ctx, &draftPub); err != nil {
		return nil, fmt.Errorf("failed to persist preview draft: %w", err)
	}

	compatIncluded := make([]domain.ManifestIncludedNode, len(compatSnap.Nodes))
	for i, n := range compatSnap.Nodes {
		compatIncluded[i] = domain.ManifestIncludedNode{
			NodeID:             n.LogicalID,
			ConnectionRevision: n.ConnectionRevision,
		}
	}

	var compatPayloadIDs []string
	if s.payloadRefRepo != nil && len(compatIncluded) > 0 {
		resolved, err := s.payloadRefRepo.ResolvePayloadIDsForNodes(ctx, compatIncluded)
		if err == nil && len(resolved) > 0 {
			compatPayloadIDs = resolved
		}
	}
	manifestCompatPayloadIDs := compatPayloadIDs
	if manifestCompatPayloadIDs == nil {
		manifestCompatPayloadIDs = []string{}
	}

	if s.payloadRefRepo != nil && len(compatPayloadIDs) > 0 {
		_ = s.payloadRefRepo.AddRefs(ctx, snapshotID, compatPayloadIDs)
	}

	manifest := domain.PublicationManifest{
		NodeCount:             len(compatSnap.Nodes),
		ExcludedCount:         len(excludedNodes),
		PayloadIDs:            manifestCompatPayloadIDs,
		ConfigurationRevision: revID,
		RulesDigest:           compatSnap.RiskDecisionDigest,
		Included:              compatIncluded,
		Excluded:              excludedNodes,
		TargetEngineVersion:   "1.0.0",
		MappingVersion:        "1.0.0",
	}

	compatDiags := append([]resolver.Diagnostic(nil), compatSnap.Diagnostics...)
	if len(compatIncluded) > 0 && len(compatPayloadIDs) == 0 {
		compatDiags = append(compatDiags, resolver.Diagnostic{
			Code:     "legacy_payload_unreferenced",
			Message:  "included nodes have no raw subscription payload reference (legacy or synthetic nodes)",
			Severity: resolver.DiagnosticSeverityWarning,
		})
	}

	return &PreviewResult{
		SnapshotID:     snapshotID,
		Target:         query.Target,
		SnapshotDigest: compileRes.SnapshotDigest,
		ContentDigest:  compileRes.ContentDigest,
		Content:        compileRes.Content,
		ContentType:    compileRes.ContentType,
		Filename:       compileRes.Filename,
		Manifest:       manifest,
		Diagnostics:    compatDiags,
		FilterCounts:   compatSnap.FilterCounts,
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

	summary := fmt.Sprintf("revoked publication %s (target: %s, snapshot: %s)", pub.ID, pub.Target, pub.SnapshotDigest)
	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "publication.revoke", domain.AuditResultSuccess, summary)

	return pub, nil
}

// Get retrieves detailed publication state from persisted fields.
func (s *Service) Get(ctx context.Context, id string) (*PublicationDetail, error) {
	pub, err := s.pubRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	detail := &PublicationDetail{
		Publication:    *pub,
		ExportURL:      "/publish/v1/" + pub.ID,
		ContentDigest:  pub.ContentDigest,
		SnapshotDigest: pub.SnapshotDigest,
		ContentType:    pub.ContentType,
		Filename:       pub.Filename,
	}

	return detail, nil
}

// ResolveAndServe resolves and serves an artifact using its publication-specific token.
func (s *Service) ResolveAndServe(ctx context.Context, publicationID, token string) (*Artifact, error) {
	return s.ResolveAndServeAuthorized(ctx, publicationID, func(pubTokenHash string) bool {
		if strings.TrimSpace(token) == "" {
			return false
		}
		digest := hashToken(token)
		return subtle.ConstantTimeCompare([]byte(pubTokenHash), []byte(digest)) == 1
	})
}

// ResolveAndServeAuthorized checks publication existence/revocation before applying
// the caller's auth policy; the same immutable artifact path serves every mode.
func (s *Service) ResolveAndServeAuthorized(ctx context.Context, publicationID string, authorized func(pubTokenHash string) bool) (*Artifact, error) {
	if strings.TrimSpace(publicationID) == "" {
		return nil, ErrNotFound
	}

	pub, err := s.pubRepo.GetByID(ctx, publicationID)
	if err != nil {
		return nil, ErrNotFound
	}

	// 1. Check revocation (403)
	if pub.State == domain.PublicationStateRevoked || !pub.IsActive() {
		return nil, ErrRevoked
	}

	// 2. Authorize only active publications, without exposing content to invalid credentials.
	if authorized == nil || !authorized(pub.TokenHash) {
		return nil, ErrUnauthorized
	}

	// 3. Target check (422)
	if !pub.Target.IsValid() || !isValidTarget(pub.Target) {
		return nil, ErrUnsupportedTarget
	}

	// 4. Serve persisted plaintext content if present and matching digest
	if len(pub.Content) > 0 {
		sum := sha256.Sum256(pub.Content)
		digest := hex.EncodeToString(sum[:])
		if subtle.ConstantTimeCompare([]byte(digest), []byte(strings.TrimSpace(pub.ContentDigest))) == 1 {
			return &Artifact{
				PublicationID:  pub.ID,
				Target:         pub.Target,
				Content:        append([]byte(nil), pub.Content...),
				ContentType:    pub.ContentType,
				Filename:       pub.Filename,
				ContentDigest:  pub.ContentDigest,
				SnapshotDigest: pub.SnapshotDigest,
			}, nil
		}
	}

	// 5. Fallback for legacy publications without persisted Content: recompile by RevisionID
	snapshot, err := s.resolveTargetSnapshot(ctx, pub.RevisionID, pub.Target)
	if err != nil {
		return nil, err
	}

	res, compileErr := compiler.Compile(ctx, snapshot, pub.Target)
	if compileErr != nil {
		var capErr *compiler.CapabilityError
		if errors.As(compileErr, &capErr) {
			return nil, domain.NewValidationError("unsupported_target_capability", capErr.Error())
		}
		return nil, compileErr
	}

	return &Artifact{
		PublicationID:  pub.ID,
		Target:         pub.Target,
		Content:        res.Content,
		ContentType:    res.ContentType,
		Filename:       res.Filename,
		ContentDigest:  res.ContentDigest,
		SnapshotDigest: res.SnapshotDigest,
	}, nil
}

// IsPublicationToken checks whether a token belongs to an active publication or active subscription link.
// This is called by AdminAuthMiddleware to forbid valid export tokens from protected admin APIs.
func (s *Service) IsPublicationToken(ctx context.Context, token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	tokenHash := hashToken(token)
	if s.pubRepo != nil {
		pub, err := s.pubRepo.GetByTokenHash(ctx, tokenHash)
		if err == nil && pub != nil && pub.IsActive() {
			return true
		}
	}
	if s.subLinkRepo != nil {
		link, err := s.subLinkRepo.GetByTokenHash(ctx, tokenHash)
		if err == nil && link != nil && link.IsActive() {
			return true
		}
	}
	return false
}

// ValidateToken validates whether a token belongs to a specific publication/link or any active publication/link.
func (s *Service) ValidateToken(ctx context.Context, publicationID, token string) (bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, nil
	}
	tokenHash := hashToken(token)

	if publicationID != "" {
		if s.pubRepo != nil {
			pub, err := s.pubRepo.GetByID(ctx, publicationID)
			if err == nil && pub != nil && pub.IsActive() {
				return subtle.ConstantTimeCompare([]byte(pub.TokenHash), []byte(tokenHash)) == 1, nil
			}
		}
		if s.subLinkRepo != nil {
			link, err := s.subLinkRepo.GetByID(ctx, publicationID)
			if err == nil && link != nil && link.IsActive() {
				return subtle.ConstantTimeCompare([]byte(link.TokenHash), []byte(tokenHash)) == 1, nil
			}
		}
		return false, nil
	}

	if s.pubRepo != nil {
		pub, err := s.pubRepo.GetByTokenHash(ctx, tokenHash)
		if err == nil && pub != nil && pub.IsActive() {
			return true, nil
		}
	}
	if s.subLinkRepo != nil {
		link, err := s.subLinkRepo.GetByTokenHash(ctx, tokenHash)
		if err == nil && link != nil && link.IsActive() {
			return true, nil
		}
	}
	return false, nil
}

// ResolvePolicySnapshot is read-only and is shared by policy validation.
func (s *Service) ResolvePolicySnapshot(ctx context.Context, revisionID string) (*resolver.ResolvedPolicySnapshot, error) {
	return s.resolveSnapshot(ctx, revisionID)
}

// AssembleResolveInput compiles all necessary domain inputs (nodes, group sources, filters, risk decisions) into a resolver.ResolveInput.
func (s *Service) AssembleResolveInput(ctx context.Context, revisionID string) (*resolver.ResolveInput, error) {
	return s.assembleResolveInput(ctx, revisionID, true)
}

func (s *Service) assembleResolveInput(ctx context.Context, revisionID string, includeGraph bool) (*resolver.ResolveInput, error) {
	if s.policyRepo == nil || s.revisionRepo == nil || s.nodeRepo == nil || s.resolver == nil {
		return nil, domain.NewInternalError("missing_dependencies", "policy, revision, or node repository not configured")
	}

	var revID string
	var selectedRevision *domain.ConfigurationRevision
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
		selectedRevision = rev
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

	var groups []domain.NodeGroup
	var err error
	if includeGraph {
		groups, err = s.policyRepo.ListGroups(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list policy groups: %w", err)
		}
	}
	if includeGraph && selectedRevision != nil && selectedRevision.State != domain.RevisionStateActive {
		if snapshots, ok := s.revisionRepo.(domain.RevisionSnapshotRepository); ok {
			groups, err = snapshots.GetPolicySnapshot(ctx, revID)
			if err != nil {
				return nil, err
			}
		}
	}

	groupSources := make(map[string][]domain.MemberSource, len(groups))
	for _, group := range groups {
		groupSources[group.ID] = group.Sources
	}

	edgeMap := make(map[string][]domain.GroupEdge, len(groups))
	for _, g := range groups {
		edgeMap[g.ID] = domain.ProjectSourceEdges(g.Sources)
	}

	var policyRules []domain.PolicyRule
	if includeGraph {
		policyRules, err = s.policyRepo.ListPolicyRules(ctx, revID)
		if err != nil {
			return nil, fmt.Errorf("failed to list policy rules for revision %s: %w", revID, err)
		}
	}

	admissionRules, err := s.policyRepo.ListAdmissionRules(ctx, revID)
	if err != nil {
		return nil, fmt.Errorf("failed to list admission rules for revision %s: %w", revID, err)
	}

	nodes, err := s.nodeRepo.ListAll(ctx, domain.NodeFilter{
		Scope:          domain.NodeScopeEnabledSubscriptions,
		ActiveOnly:     true,
		ExcludeNotices: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list active nodes: %w", err)
	}

	var globalFilter *domain.NodeFilterSpec
	var groupFilters map[string]domain.NodeFilterSpec
	if s.nodeFilterRepo != nil {
		gf, err := s.nodeFilterRepo.GetGlobalFilter(ctx)
		if err != nil {
			return nil, domain.NewInternalError("global_filter_unavailable", "failed to read global node filter")
		}
		if gf != nil && !gf.Spec.IsEmpty() {
			globalFilter = &gf.Spec
		}

	}

	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.LogicalID
	}

	needsSources, needsObservations := filterDependencies(globalFilter, groupFilters, groups)
	var nodeSources map[string][]domain.NodeSource
	if needsSources && s.sourceRepo == nil {
		return nil, domain.NewInternalError("node_sources_unavailable", "node sources required by configured filter are unavailable")
	}
	if needsSources && s.sourceRepo != nil && len(nodeIDs) > 0 {
		nodeSources, err = s.sourceRepo.ListByNodes(ctx, nodeIDs)
		if err != nil {
			return nil, domain.NewInternalError("node_sources_unavailable", "failed to read node sources for configured filter")
		}
	}

	var latestObs map[string]map[domain.ProbeKind]domain.ProbeObservation
	if needsObservations && s.obsRepo == nil {
		return nil, domain.NewInternalError("probe_observations_unavailable", "probe observations required by configured filter are unavailable")
	}
	if needsObservations && s.obsRepo != nil && len(nodeIDs) > 0 {
		latestObs, err = s.obsRepo.ListLatestByNodes(ctx, nodeIDs, nil)
		if err != nil {
			return nil, domain.NewInternalError("probe_observations_unavailable", "failed to read probe observations for configured filter")
		}
	}

	input := resolver.ResolveInput{
		RevisionID:         revID,
		InventoryWatermark: "v1",
		CompilerVersion:    "1.0.0",
		Nodes:              nodes,
		Groups:             groups,
		GroupSources:       groupSources,
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
		if pErr != nil {
			var de *domain.DomainError
			if !errors.As(pErr, &de) || de.Category != domain.CategoryNotFound {
				return nil, domain.NewInternalError("risk_policy_unavailable", "failed to read active risk policy")
			}
		}
		if activePolicy != nil && activePolicy.Active {
			nodeIDs := make([]string, len(nodes))
			for i, n := range nodes {
				nodeIDs[i] = n.LogicalID
			}
			batchRes, bErr := s.ipriskSvc.EvaluateBatch(ctx, iprisk.EvaluateBatchInput{
				NodeLogicalIDs:   nodeIDs,
				Policy:           &activePolicy.RiskPolicy,
				PolicyRevisionID: activePolicy.RevisionID,
				EvaluatedAt:      input.AsOf,
			})
			if bErr != nil {
				return nil, domain.NewInternalError("risk_evaluation_unavailable", "failed to evaluate active risk policy")
			}
			if batchRes != nil {
				input.RiskPolicyRevision = activePolicy.RevisionID
				input.RiskDecisionDigest = batchRes.DecisionDigest
				evalAt := batchRes.EvaluatedAt
				input.RiskEvaluatedAt = &evalAt
				input.RiskReviewAction = activePolicy.EffectiveReviewAction()
				input.RiskDecisions = batchRes.Decisions
			}
		}
	}

	return &input, nil
}

func (s *Service) resolveSnapshot(ctx context.Context, revisionID string) (*resolver.ResolvedPolicySnapshot, error) {
	return s.resolveTargetSnapshot(ctx, revisionID, domain.TargetMihomo)
}

func (s *Service) resolveTargetSnapshot(ctx context.Context, revisionID string, target domain.CompilerTarget) (*resolver.ResolvedPolicySnapshot, error) {
	input, err := s.assembleResolveInput(ctx, revisionID, target == domain.TargetMihomo)
	if err != nil {
		return nil, err
	}

	// Pure-node formats do not consume a policy graph. Do not validate an
	// irrelevant cyclic/missing/invalid group before compiling admitted nodes.
	resolveInput := *input
	if target != domain.TargetMihomo {
		resolveInput.Groups = nil
		resolveInput.Edges = nil
		resolveInput.GroupSources = nil
		resolveInput.GroupFilters = nil
		resolveInput.PolicyRules = nil
	}
	snapshot, err := s.resolver.Resolve(ctx, resolveInput)
	if err != nil {
		return nil, err
	}
	counts := snapshot.FilterCounts
	if counts != nil && counts.AdmittedTotal > 0 {
		globallyEmpty := input.GlobalFilter != nil && !input.GlobalFilter.IsEmpty() && counts.GlobalFilteredTotal == 0
		groupEmpty := false
		if !globallyEmpty && counts.GroupFilteredTotal == 0 {
			for _, group := range input.Groups {
				hasFilter := false
				for _, source := range group.Sources {
					if !source.FilterSpec.IsEmpty() {
						hasFilter = true
						break
					}
				}
				if count := counts.GroupCounts[group.ID]; hasFilter && count.Candidate > 0 && count.Excluded > 0 {
					groupEmpty = true
					break
				}
			}
		}
		if globallyEmpty || groupEmpty {
			snapshot.Diagnostics = append(snapshot.Diagnostics, resolver.Diagnostic{
				Severity: resolver.DiagnosticSeverityError,
				Code:     "filtered_nodes_empty",
				Message:  "configured node filters left no exportable nodes; check probe observations, freshness and filter conditions",
			})
		}
	}
	return snapshot, nil
}

// filterDependencies includes persisted group filters and inline group filters; the
// resolver applies both and evaluates every condition with AND semantics.
func filterDependencies(global *domain.NodeFilterSpec, groupFilters map[string]domain.NodeFilterSpec, groups []domain.NodeGroup) (sources, observations bool) {
	inspect := func(spec *domain.NodeFilterSpec) {
		if spec == nil {
			return
		}
		for _, condition := range spec.Conditions {
			switch condition.Field {
			case domain.FilterFieldSourceSubscriptions:
				sources = true
			case domain.FilterFieldProbeVerdict, domain.FilterFieldProbeLatencyMS:
				observations = true
			}
		}
	}
	inspect(global)
	for _, spec := range groupFilters {
		inspect(&spec)
	}
	for _, group := range groups {
		inspect(group.NodeFilter)
		for _, src := range group.Sources {
			if src.FilterSpec != nil {
				inspect(src.FilterSpec)
			}
		}
	}
	return sources, observations
}

func (s *Service) evaluatePreflight(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) PreflightResult {
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
		if target != domain.TargetMihomo && diagnostic.Code != "filtered_nodes_empty" {
			if diagnostic.Code != "risk_blocked" && diagnostic.Code != "risk_review" && diagnostic.Code != "risk_unknown" {
				continue
			}
		}
		if diagnostic.Code == "risk_blocked" || diagnostic.Code == "risk_review" || diagnostic.Code == "risk_unknown" || diagnostic.Severity == resolver.DiagnosticSeverityError || diagnostic.Code == "empty_routed_group" || diagnostic.Code == "required_nonempty" {
			result.Allowed = false
			addDiagnostic(PreflightDiagnostic{
				Severity: diagnostic.Severity,
				Code:     diagnostic.Code,
				Message:  diagnostic.Message,
				Target:   diagnostic.Target,
			})
		}
	}

	if target == domain.TargetMihomo {
		capDiags := compiler.ValidateTargetCapabilities(snapshot, target)
		for _, cd := range capDiags {
			result.Allowed = false
			addDiagnostic(PreflightDiagnostic{
				Severity: resolver.DiagnosticSeverityError,
				Code:     cd.Code,
				Message:  cd.Message,
				Target:   cd.Target,
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
			if target == domain.TargetMihomo {
				for _, g := range snapshot.Groups {
					for _, nid := range g.NodeLogicalIDs {
						nodeIDSet[nid] = struct{}{}
					}
					for _, nid := range g.AllNodeLogicalIDs {
						nodeIDSet[nid] = struct{}{}
					}
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

func NewPreflightError(result PreflightResult) error {
	return &PreflightError{Result: result}
}

func newPreflightError(result PreflightResult) error {
	return NewPreflightError(result)
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

// HashToken computes the SHA-256 hex digest of a token.
func HashToken(token string) string {
	return hashToken(token)
}

// ValidatePublicationContent performs frozen-bytes verification of compiled output
// before publication commit/activation to guarantee official kernel schema conformance.
func ValidatePublicationContent(content []byte, target domain.CompilerTarget) error {
	if len(content) == 0 {
		return domain.NewValidationError("snapshot_not_publishable", "content is empty")
	}

	if target == domain.TargetMihomo {
		var rawCfg struct {
			Proxies     []map[string]any `yaml:"proxies"`
			ProxyGroups []struct {
				Name    string   `yaml:"name"`
				Type    string   `yaml:"type"`
				Proxies []string `yaml:"proxies"`
				Use     []string `yaml:"use"`
			} `yaml:"proxy-groups"`
			Rules []string `yaml:"rules"`
		}

		if err := yaml.Unmarshal(content, &rawCfg); err != nil {
			return domain.NewValidationError("invalid_publication_content", fmt.Sprintf("failed to parse mihomo YAML: %v", err))
		}

		groupNames := make(map[string]bool, len(rawCfg.ProxyGroups))
		for _, pg := range rawCfg.ProxyGroups {
			if pg.Name != "" {
				groupNames[pg.Name] = true
			}
		}

		for _, pg := range rawCfg.ProxyGroups {
			if len(pg.Proxies) == 0 && len(pg.Use) == 0 {
				return domain.NewValidationError("required_nonempty", fmt.Sprintf("proxy group %q has no proxies (required_nonempty)", pg.Name))
			}
		}

		for _, r := range rawCfg.Rules {
			targetGroup := extractMihomoRuleTarget(r)
			if targetGroup != "" && targetGroup != "DIRECT" && targetGroup != "REJECT" && targetGroup != "REJECT-DROP" && targetGroup != "PASS" {
				if !groupNames[targetGroup] {
					return domain.NewValidationError("dangling_rule_target", fmt.Sprintf("rule %q targets non-existent group %q", r, targetGroup))
				}
			}
		}
	}

	if target == domain.TargetSingBox {
		var rawCfg map[string]any
		if err := json.Unmarshal(content, &rawCfg); err != nil {
			return domain.NewValidationError("invalid_publication_content", fmt.Sprintf("failed to parse sing-box JSON: %v", err))
		}
	}

	return nil
}

func extractMihomoRuleTarget(ruleLine string) string {
	parts := strings.Split(ruleLine, ",")
	if len(parts) < 2 {
		return ""
	}
	kind := strings.ToUpper(strings.TrimSpace(parts[0]))
	if kind == "MATCH" {
		return strings.TrimSpace(parts[1])
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if strings.EqualFold(last, "no-resolve") {
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[len(parts)-2])
		}
		return ""
	}
	return last
}

func generateSubscriptionToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate crypto random bytes: %w", err)
	}
	rawToken := "sub_" + hex.EncodeToString(b)
	return rawToken, hashToken(rawToken), nil
}

// BuildCurrentArtifact executes pure, deterministic, side-effect-free configuration compilation from consistent facts.
// It performs in-memory policy resolution, preflight checks, and kernel schema validation without writing drafts or references.
func (s *Service) BuildCurrentArtifact(ctx context.Context, facts *domain.ConsistentFacts, target domain.CompilerTarget) (*Artifact, *resolver.ResolvedPolicySnapshot, error) {
	if !isValidTarget(target) {
		return nil, nil, domain.NewValidationError("unsupported_target", fmt.Sprintf("unsupported compiler target: %s", target))
	}
	if facts == nil || facts.Revision == nil {
		return nil, nil, domain.NewConflictError("no_active_revision", "cannot resolve policy without an active configuration revision")
	}

	revID := facts.Revision.ID
	input := resolver.ResolveInput{
		RevisionID:         revID,
		InventoryWatermark: "v1",
		CompilerVersion:    "1.0.0",
		Nodes:              facts.Nodes,
		AdmissionRules:     facts.AdmissionRules,
		GlobalFilter:       facts.GlobalFilter,
		NodeSources:        facts.NodeSources,
		LatestObservations: facts.LatestObservations,
		AsOf:               facts.AsOf,
	}

	if target == domain.TargetMihomo {
		input.Groups = facts.Groups
		input.GroupSources = facts.GroupSources
		input.Edges = facts.GroupEdges
		input.PolicyRules = facts.PolicyRules
		input.GroupFilters = facts.GroupFilters
		input.DNS = resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		}
	}

	if facts.ActiveRiskPolicy != nil && facts.ActiveRiskPolicy.Active && len(facts.Nodes) > 0 && s.ipriskSvc != nil {
		nodeIDs := make([]string, len(facts.Nodes))
		for i, n := range facts.Nodes {
			nodeIDs[i] = n.LogicalID
		}
		riskObs := facts.RiskObservations
		if riskObs == nil {
			riskObs = []domain.IPRiskObservation{}
		}
		batchRes, bErr := s.ipriskSvc.EvaluateBatch(ctx, iprisk.EvaluateBatchInput{
			NodeLogicalIDs:           nodeIDs,
			Policy:                   &facts.ActiveRiskPolicy.RiskPolicy,
			PolicyRevisionID:         facts.ActiveRiskPolicy.RevisionID,
			EvaluatedAt:              facts.AsOf,
			Observations:             riskObs,
			DisableObservationLookup: true,
		})
		if bErr != nil {
			return nil, nil, domain.NewInternalError("risk_evaluation_unavailable", "failed to evaluate active risk policy")
		}
		if batchRes != nil {
			input.RiskPolicyRevision = facts.ActiveRiskPolicy.RevisionID
			input.RiskDecisionDigest = batchRes.DecisionDigest
			evalAt := batchRes.EvaluatedAt
			input.RiskEvaluatedAt = &evalAt
			input.RiskReviewAction = facts.ActiveRiskPolicy.EffectiveReviewAction()
			input.RiskDecisions = batchRes.Decisions
		}
	}

	snapshot, err := s.resolver.Resolve(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	counts := snapshot.FilterCounts
	if counts != nil && counts.AdmittedTotal > 0 {
		globallyEmpty := input.GlobalFilter != nil && !input.GlobalFilter.IsEmpty() && counts.GlobalFilteredTotal == 0
		groupEmpty := false
		if !globallyEmpty && counts.GroupFilteredTotal == 0 && target == domain.TargetMihomo {
			for _, group := range input.Groups {
				hasFilter := false
				for _, source := range group.Sources {
					if source.FilterSpec != nil && !source.FilterSpec.IsEmpty() {
						hasFilter = true
						break
					}
				}
				if count := counts.GroupCounts[group.ID]; hasFilter && count.Candidate > 0 && count.Excluded > 0 {
					groupEmpty = true
					break
				}
			}
		}
		if globallyEmpty || groupEmpty {
			snapshot.Diagnostics = append(snapshot.Diagnostics, resolver.Diagnostic{
				Severity: resolver.DiagnosticSeverityError,
				Code:     "filtered_nodes_empty",
				Message:  "configured node filters left no exportable nodes; check probe observations, freshness and filter conditions",
			})
		}
	}

	if len(snapshot.Nodes) == 0 {
		hasEmptyDiag := false
		for _, d := range snapshot.Diagnostics {
			if d.Code == "filtered_nodes_empty" {
				hasEmptyDiag = true
				break
			}
		}
		if !hasEmptyDiag {
			snapshot.Diagnostics = append(snapshot.Diagnostics, resolver.Diagnostic{
				Severity: resolver.DiagnosticSeverityError,
				Code:     "filtered_nodes_empty",
				Message:  "configured node filters left no exportable nodes; check probe observations, freshness and filter conditions",
			})
		}
	}

	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.Code == "filtered_nodes_empty" {
			preflight := s.evaluatePreflightFromSnapshot(ctx, snapshot, target, facts.ActiveRiskPolicy, facts.RiskObservations, facts.AsOf)
			return nil, snapshot, newPreflightError(preflight)
		}
	}

	// Live subscriptions are strictly validated for kernel capabilities
	diags := compiler.ValidateTargetCapabilities(snapshot, target)
	if len(diags) > 0 {
		msg := "target capability validation failed in strict mode"
		if len(diags) > 0 {
			msg = fmt.Sprintf("target capability validation failed in strict mode: %s", diags[0].Message)
		}
		return nil, snapshot, &StrictCapabilityError{
			Code:        "unsupported_target_capability",
			Message:     msg,
			Diagnostics: diags,
		}
	}

	preflight := s.evaluatePreflightFromSnapshot(ctx, snapshot, target, facts.ActiveRiskPolicy, facts.RiskObservations, facts.AsOf)
	if !preflight.Allowed {
		return nil, snapshot, newPreflightError(preflight)
	}

	compileRes, err := compiler.Compile(ctx, snapshot, target)
	if err != nil {
		var capErr *compiler.CapabilityError
		if errors.As(err, &capErr) {
			return nil, snapshot, domain.NewValidationError("unsupported_target_capability", capErr.Error())
		}
		return nil, snapshot, err
	}

	if err := ValidatePublicationContent(compileRes.Content, target); err != nil {
		return nil, snapshot, err
	}

	return &Artifact{
		Target:         target,
		Content:        compileRes.Content,
		ContentType:    compileRes.ContentType,
		Filename:       compileRes.Filename,
		ContentDigest:  compileRes.ContentDigest,
		SnapshotDigest: compileRes.SnapshotDigest,
	}, snapshot, nil
}

func (s *Service) evaluatePreflightFromSnapshot(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget, activePolicy *domain.RiskPolicyRevision, observations []domain.IPRiskObservation, asOf time.Time) PreflightResult {
	result := PreflightResult{Allowed: true, Diagnostics: make([]PreflightDiagnostic, 0)}
	if snapshot == nil {
		return result
	}
	result.SnapshotDigest = snapshot.SnapshotDigest
	if activePolicy != nil {
		result.PolicyRevision = activePolicy.RevisionID
	}

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
		if target != domain.TargetMihomo && diagnostic.Code != "filtered_nodes_empty" {
			if diagnostic.Code != "risk_blocked" && diagnostic.Code != "risk_review" && diagnostic.Code != "risk_unknown" {
				continue
			}
		}
		if diagnostic.Code == "risk_blocked" || diagnostic.Code == "risk_review" || diagnostic.Code == "risk_unknown" || diagnostic.Severity == resolver.DiagnosticSeverityError || diagnostic.Code == "empty_routed_group" || diagnostic.Code == "required_nonempty" {
			result.Allowed = false
			addDiagnostic(PreflightDiagnostic{
				Severity: diagnostic.Severity,
				Code:     diagnostic.Code,
				Message:  diagnostic.Message,
				Target:   diagnostic.Target,
			})
		}
	}

	if target == domain.TargetMihomo {
		capDiags := compiler.ValidateTargetCapabilities(snapshot, target)
		for _, cd := range capDiags {
			result.Allowed = false
			addDiagnostic(PreflightDiagnostic{
				Severity: resolver.DiagnosticSeverityError,
				Code:     cd.Code,
				Message:  cd.Message,
				Target:   cd.Target,
			})
		}
	}

	// 2. Perform real-time risk recomputation over snapshot members using in-memory observations
	if s.ipriskSvc != nil && activePolicy != nil && activePolicy.Active {
		nodeIDSet := make(map[string]struct{})
		for _, n := range snapshot.Nodes {
			nodeIDSet[n.LogicalID] = struct{}{}
		}
		if target == domain.TargetMihomo {
			for _, g := range snapshot.Groups {
				for _, nid := range g.NodeLogicalIDs {
					nodeIDSet[nid] = struct{}{}
				}
				for _, nid := range g.AllNodeLogicalIDs {
					nodeIDSet[nid] = struct{}{}
				}
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
			riskObs := observations
			if riskObs == nil {
				riskObs = []domain.IPRiskObservation{}
			}
			batchRes, err := s.ipriskSvc.EvaluateBatch(ctx, iprisk.EvaluateBatchInput{
				NodeLogicalIDs:           nodeIDs,
				Policy:                   &activePolicy.RiskPolicy,
				PolicyRevisionID:         activePolicy.RevisionID,
				EvaluatedAt:              asOf,
				Observations:             riskObs,
				DisableObservationLookup: true,
			})
			if err == nil && batchRes != nil {
				reviewAction := activePolicy.EffectiveReviewAction()
				unknownAction := activePolicy.EffectiveUnknownAction()

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

	return result
}

// GenerateTargetArtifact coordinates the dynamic on-request compilation of consistent facts for a given compiler target.
func (s *Service) GenerateTargetArtifact(ctx context.Context, target domain.CompilerTarget) (*Artifact, error) {
	if s.factsLoader == nil {
		return nil, domain.NewInternalError("missing_loader", "consistent facts loader not configured")
	}

	if s.concurrencySem != nil {
		select {
		case s.concurrencySem <- struct{}{}:
			defer func() { <-s.concurrencySem }()
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, ErrRateLimitExceeded
		}
	}

	facts, err := s.factsLoader.LoadConsistentFacts(ctx, "", target, domain.NowUTC())
	if err != nil {
		return nil, err
	}

	artifact, _, err := s.BuildCurrentArtifact(ctx, facts, target)
	if err != nil {
		return nil, err
	}
	return artifact, nil
}

// GenerateLiveArtifact coordinates the end-to-end generation of an authorized client subscription export.
func (s *Service) GenerateLiveArtifact(ctx context.Context, linkID string, token string) (*Artifact, error) {
	if s.subLinkRepo == nil {
		return nil, domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}
	if s.factsLoader == nil {
		return nil, domain.NewInternalError("missing_loader", "consistent facts loader not configured")
	}

	// Bounded concurrency check
	select {
	case s.concurrencySem <- struct{}{}:
		defer func() { <-s.concurrencySem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrRateLimitExceeded
	}

	link, err := s.subLinkRepo.GetByID(ctx, linkID)
	if err != nil {
		return nil, ErrLinkNotFound
	}
	if !link.IsActive() {
		return nil, ErrLinkRevoked
	}
	if subtle.ConstantTimeCompare([]byte(link.TokenHash), []byte(hashToken(token))) != 1 {
		return nil, ErrUnauthorized
	}

	facts, err := s.factsLoader.LoadConsistentFacts(ctx, "", link.Target, domain.NowUTC())
	if err != nil {
		return nil, err
	}

	artifact, _, err := s.BuildCurrentArtifact(ctx, facts, link.Target)
	if err != nil {
		return nil, err
	}
	artifact.PublicationID = link.ID
	return artifact, nil
}

// CheckSubscriptionLink performs real-time diagnostic evaluation of a link without persistent side-effects.
func (s *Service) CheckSubscriptionLink(ctx context.Context, id string) (*CheckSubscriptionLinkResult, error) {
	if s.subLinkRepo == nil {
		return nil, domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}
	if s.factsLoader == nil {
		return nil, domain.NewInternalError("missing_loader", "consistent facts loader not configured")
	}

	// Bounded concurrency check
	select {
	case s.concurrencySem <- struct{}{}:
		defer func() { <-s.concurrencySem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrRateLimitExceeded
	}

	link, err := s.subLinkRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrLinkNotFound
	}
	if !link.IsActive() {
		return nil, ErrLinkRevoked
	}

	facts, err := s.factsLoader.LoadConsistentFacts(ctx, "", link.Target, domain.NowUTC())
	if err != nil {
		return nil, err
	}

	artifact, snapshot, err := s.BuildCurrentArtifact(ctx, facts, link.Target)
	if err == nil {
		return &CheckSubscriptionLinkResult{
			ID:            link.ID,
			Target:        link.Target,
			Allowed:       true,
			AsOf:          facts.AsOf.Format(time.RFC3339Nano),
			ContentDigest: artifact.ContentDigest,
			NodeCount:     len(snapshot.Nodes),
			Message:       "Configuration generated successfully",
		}, nil
	}

	var pfErr *PreflightError
	if errors.As(err, &pfErr) {
		nodeCount := 0
		if snapshot != nil {
			nodeCount = len(snapshot.Nodes)
		}
		return &CheckSubscriptionLinkResult{
			ID:          link.ID,
			Target:      link.Target,
			Allowed:     false,
			Diagnostics: pfErr.Result.Diagnostics,
			AsOf:        facts.AsOf.Format(time.RFC3339Nano),
			NodeCount:   nodeCount,
			Message:     "Configuration blocked by preflight checks",
		}, nil
	}

	var capErr *StrictCapabilityError
	if errors.As(err, &capErr) {
		var diags []PreflightDiagnostic
		for _, cd := range capErr.Diagnostics {
			diags = append(diags, PreflightDiagnostic{
				Severity: resolver.DiagnosticSeverityError,
				Code:     cd.Code,
				Message:  cd.Message,
				Target:   cd.Target,
			})
		}
		nodeCount := 0
		if snapshot != nil {
			nodeCount = len(snapshot.Nodes)
		}
		return &CheckSubscriptionLinkResult{
			ID:          link.ID,
			Target:      link.Target,
			Allowed:     false,
			Diagnostics: diags,
			AsOf:        facts.AsOf.Format(time.RFC3339Nano),
			NodeCount:   nodeCount,
			Message:     capErr.Message,
		}, nil
	}

	return nil, err
}

// ListSubscriptionLinks lists all registered subscription links.
func (s *Service) ListSubscriptionLinks(ctx context.Context) ([]domain.SubscriptionLink, error) {
	if s.subLinkRepo == nil {
		return nil, domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}
	return s.subLinkRepo.List(ctx)
}

// CreateSubscriptionLink creates a new link or idempotently returns the existing active default link for target.
func (s *Service) CreateSubscriptionLink(ctx context.Context, cmd CreateSubscriptionLinkCommand) (*domain.SubscriptionLink, error) {
	if !isValidTarget(cmd.Target) {
		return nil, domain.NewValidationError("unsupported_target", fmt.Sprintf("unsupported compiler target: %s", cmd.Target))
	}
	if s.subLinkRepo == nil {
		return nil, domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}

	if cmd.IsDefault {
		existing, err := s.subLinkRepo.GetDefaultByTarget(ctx, cmd.Target)
		if err == nil && existing != nil && existing.IsActive() {
			return existing, nil
		}
	}

	rawToken, tokenHash, err := generateSubscriptionToken()
	if err != nil {
		return nil, err
	}

	link := &domain.SubscriptionLink{
		ID:        domain.MustNewUUIDv7(),
		Label:     strings.TrimSpace(cmd.Label),
		Target:    cmd.Target,
		Token:     rawToken,
		TokenHash: tokenHash,
		State:     domain.SubscriptionLinkStateActive,
		IsDefault: cmd.IsDefault,
		CreatedAt: domain.NowUTC(),
		UpdatedAt: domain.NowUTC(),
	}

	if err := s.subLinkRepo.Create(ctx, link); err != nil {
		return nil, err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "subscription_link.create", domain.AuditResultSuccess,
		fmt.Sprintf("created subscription link %s for target %s (default: %v)", link.ID, link.Target, link.IsDefault))

	return link, nil
}

// GetSubscriptionLink returns a single subscription link by ID.
func (s *Service) GetSubscriptionLink(ctx context.Context, id string) (*domain.SubscriptionLink, error) {
	if s.subLinkRepo == nil {
		return nil, domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}
	return s.subLinkRepo.GetByID(ctx, id)
}

// RotateSubscriptionLinkToken replaces the credentials of an active link with new values.
func (s *Service) RotateSubscriptionLinkToken(ctx context.Context, id string, actorKind domain.ActorKind, requestID string) (*domain.SubscriptionLink, error) {
	if s.subLinkRepo == nil {
		return nil, domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}
	link, err := s.subLinkRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !link.IsActive() {
		return nil, ErrLinkRevoked
	}
	rawToken, tokenHash, err := generateSubscriptionToken()
	if err != nil {
		return nil, err
	}
	now := domain.NowUTC()
	if err := s.subLinkRepo.RotateToken(ctx, id, rawToken, tokenHash, now); err != nil {
		return nil, err
	}
	link.Token = rawToken
	link.TokenHash = tokenHash
	link.UpdatedAt = now

	s.recordAudit(ctx, actorKind, requestID, "subscription_link.rotate", domain.AuditResultSuccess,
		fmt.Sprintf("rotated token for subscription link %s", link.ID))

	return link, nil
}

// RevokeSubscriptionLink marks an active link as revoked.
func (s *Service) RevokeSubscriptionLink(ctx context.Context, id string, actorKind domain.ActorKind, requestID string) error {
	if s.subLinkRepo == nil {
		return domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}
	link, err := s.subLinkRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if link.State == domain.SubscriptionLinkStateRevoked {
		return nil
	}
	now := domain.NowUTC()
	if err := s.subLinkRepo.Revoke(ctx, id, now); err != nil {
		return err
	}

	s.recordAudit(ctx, actorKind, requestID, "subscription_link.revoke", domain.AuditResultSuccess,
		fmt.Sprintf("revoked subscription link %s", id))

	return nil
}

// UpdateSubscriptionLinkLabel updates the label of a subscription link.
func (s *Service) UpdateSubscriptionLinkLabel(ctx context.Context, id, label string) error {
	if s.subLinkRepo == nil {
		return domain.NewInternalError("missing_repository", "subscription link repository not configured")
	}
	return s.subLinkRepo.UpdateLabel(ctx, id, label, domain.NowUTC())
}
