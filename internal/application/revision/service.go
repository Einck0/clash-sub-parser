// Package revision provides review, history inspection, and activation use cases for configuration revisions.
package revision

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
)

// Action carries audit attribution context for an operation.
type Action struct {
	RequestID string
	ActorKind domain.ActorKind
}

// RevisionDetail encapsulates a configuration revision along with its associated policy and admission rules.
type RevisionDetail struct {
	domain.ConfigurationRevision
	PolicyRules    []domain.PolicyRule    `json:"policy_rules"`
	AdmissionRules []domain.AdmissionRule `json:"admission_rules"`
}

// Service manages configuration revision review, history inspection, and explicit activation.
type Service struct {
	revisions  domain.RevisionRepository
	policyRepo domain.PolicyRepository
	auditRepo  domain.AuditRepository
	mu         sync.Mutex
}

// Option configures Service dependencies.
type Option func(*Service)

// WithPolicyRepository injects a policy repository for loading revision rules and validating topology.
func WithPolicyRepository(repo domain.PolicyRepository) Option {
	return func(s *Service) {
		s.policyRepo = repo
	}
}

// NewService constructs a new revision application service.
func NewService(revisions domain.RevisionRepository, auditRepo domain.AuditRepository, opts ...Option) *Service {
	s := &Service{
		revisions: revisions,
		auditRepo: auditRepo,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// List queries configuration revision history with pagination and optional state filtering.
func (s *Service) List(ctx context.Context, filter domain.RevisionFilter) ([]domain.ConfigurationRevision, int, error) {
	if filter.Pagination.Page < 0 || filter.Pagination.PageSize < 0 {
		return nil, 0, domain.NewValidationError("invalid_pagination", "page and page_size cannot be negative")
	}
	return s.revisions.List(ctx, filter)
}

// GetActive retrieves the currently active configuration revision.
func (s *Service) GetActive(ctx context.Context) (*domain.ConfigurationRevision, error) {
	return s.revisions.GetActive(ctx)
}

// Get inspects a configuration revision and its associated rules without state changes.
func (s *Service) Get(ctx context.Context, id string) (*RevisionDetail, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, domain.NewValidationError("missing_revision_id", "revision id is required")
	}
	rev, err := s.revisions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.loadDetail(ctx, rev)
}

// Review verifies a draft revision and records an audited review event.
// Unactivated drafts are strictly validated as drafts and remain non-active.
func (s *Service) Review(ctx context.Context, id string, action Action) (*RevisionDetail, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		s.recordAudit(ctx, action, "revision.review", domain.AuditResultFailure, "missing revision id")
		return nil, domain.NewValidationError("missing_revision_id", "revision id is required")
	}

	rev, err := s.revisions.GetByID(ctx, id)
	if err != nil {
		s.recordAudit(ctx, action, "revision.review", domain.AuditResultFailure, fmt.Sprintf("revision %s: %v", id, err))
		return nil, err
	}

	if rev.State != domain.RevisionStateDraft {
		s.recordAudit(ctx, action, "revision.review", domain.AuditResultFailure, fmt.Sprintf("revision %s not draft (current: %s)", id, rev.State))
		return nil, domain.NewConflictError("revision_not_draft", fmt.Sprintf("only draft revisions can be reviewed; revision %s is %s", id, rev.State))
	}

	detail, err := s.loadDetail(ctx, rev)
	if err != nil {
		s.recordAudit(ctx, action, "revision.review", domain.AuditResultFailure, fmt.Sprintf("failed to load detail for %s: %v", id, err))
		return nil, err
	}

	s.recordAudit(ctx, action, "revision.review", domain.AuditResultSuccess, fmt.Sprintf("revision_id=%s digest=%s state=draft rules=%d", rev.ID, rev.ContentDigest, len(detail.PolicyRules)+len(detail.AdmissionRules)))
	return detail, nil
}

// Activate explicitly promotes a draft revision to active state, archiving any prior active revision.
// Unactivated drafts never participate in runtime consumers (scheduler, probe, resolver, publication)
// until this explicit method is invoked.
func (s *Service) Activate(ctx context.Context, id string, action Action) (*domain.ConfigurationRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, "missing revision id")
		return nil, domain.NewValidationError("missing_revision_id", "revision id is required")
	}

	rev, err := s.revisions.GetByID(ctx, id)
	if err != nil {
		s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, fmt.Sprintf("revision %s: %v", id, err))
		return nil, err
	}

	if rev.State == domain.RevisionStateActive {
		s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, fmt.Sprintf("revision %s already active", id))
		return nil, domain.NewConflictError("revision_already_active", fmt.Sprintf("revision %s is already active", id))
	}

	if rev.State != domain.RevisionStateDraft {
		s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, fmt.Sprintf("revision %s not draft (current: %s)", id, rev.State))
		return nil, domain.NewConflictError("revision_not_draft", fmt.Sprintf("only draft revisions can be activated; revision %s is %s", id, rev.State))
	}

	// Validate policy graph and rules if policy repository is configured
	if s.policyRepo != nil {
		groups, gErr := s.policyRepo.ListGroups(ctx)
		if gErr != nil {
			s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, fmt.Sprintf("failed to list groups for %s: %v", id, gErr))
			return nil, fmt.Errorf("failed to list groups for activation validation: %w", gErr)
		}
		if len(groups) > 0 {
			edgeMap := make(map[string][]domain.GroupEdge, len(groups))
			for _, g := range groups {
				edges, eErr := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
				if eErr != nil {
					s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, fmt.Sprintf("failed to list edges for group %s: %v", g.ID, eErr))
					return nil, fmt.Errorf("failed to list edges for group %s: %w", g.ID, eErr)
				}
				edgeMap[g.ID] = edges
			}
			polRules, _ := s.policyRepo.ListPolicyRules(ctx, id)
			admRules, _ := s.policyRepo.ListAdmissionRules(ctx, id)
			if vErr := policy.ValidatePolicyGraph(groups, edgeMap, polRules, admRules); vErr != nil {
				s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, fmt.Sprintf("policy graph validation failed for %s: %v", id, vErr))
				return nil, vErr
			}
		}
	}

	if err := s.revisions.SetActive(ctx, id); err != nil {
		s.recordAudit(ctx, action, "revision.activate", domain.AuditResultFailure, fmt.Sprintf("failed to set active for %s: %v", id, err))
		return nil, err
	}

	rev.State = domain.RevisionStateActive
	s.recordAudit(ctx, action, "revision.activate", domain.AuditResultSuccess, fmt.Sprintf("revision_id=%s digest=%s state=active", rev.ID, rev.ContentDigest))
	return rev, nil
}

func (s *Service) loadDetail(ctx context.Context, rev *domain.ConfigurationRevision) (*RevisionDetail, error) {
	detail := &RevisionDetail{
		ConfigurationRevision: *rev,
		PolicyRules:           make([]domain.PolicyRule, 0),
		AdmissionRules:        make([]domain.AdmissionRule, 0),
	}
	if s.policyRepo != nil {
		pRules, err := s.policyRepo.ListPolicyRules(ctx, rev.ID)
		if err == nil && pRules != nil {
			detail.PolicyRules = pRules
		}
		aRules, err := s.policyRepo.ListAdmissionRules(ctx, rev.ID)
		if err == nil && aRules != nil {
			detail.AdmissionRules = aRules
		}
	}
	return detail, nil
}

func (s *Service) recordAudit(ctx context.Context, action Action, name string, result domain.AuditResult, summary string) {
	if s.auditRepo == nil {
		return
	}
	actor := action.ActorKind
	if !actor.IsValid() {
		actor = domain.ActorKindAnonymous
	}
	_ = s.auditRepo.Record(ctx, &domain.AuditEvent{
		ID:              domain.MustNewUUIDv7(),
		ActorKind:       actor,
		RequestID:       action.RequestID,
		Action:          name,
		Result:          result,
		RedactedSummary: domain.RedactSensitiveInfo(summary),
		CreatedAt:       domain.NowUTC(),
	})
}
