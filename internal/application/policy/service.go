// Package policy provides policy graph orchestration, admission rules, and topology validation use cases.
package policy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// Service orchestrates policy groups, edges, admission rules, and configuration revisions.
type Service struct {
	policyRepo       domain.PolicyRepository
	revisionRepo     domain.RevisionRepository
	nodeRepo         domain.NodeRepository
	auditRepo        domain.AuditRepository
	nodeFilterRepo   domain.NodeFilterRepository
	snapshotProvider func(context.Context, string) (*resolver.ResolvedPolicySnapshot, error)
	inputProvider    func(context.Context, string) (*resolver.ResolveInput, error)
	mu               sync.Mutex
}

// NewService constructs a policy application service from domain repository ports.
func NewService(
	policyRepo domain.PolicyRepository,
	revisionRepo domain.RevisionRepository,
	nodeRepo domain.NodeRepository,
	auditRepo domain.AuditRepository,
	nodeFilterRepo ...domain.NodeFilterRepository,
) *Service {
	s := &Service{
		policyRepo:   policyRepo,
		revisionRepo: revisionRepo,
		nodeRepo:     nodeRepo,
		auditRepo:    auditRepo,
	}
	if len(nodeFilterRepo) > 0 {
		s.nodeFilterRepo = nodeFilterRepo[0]
	}
	return s
}

// SetSnapshotProvider shares publication's read-only input assembly with validation.
func (s *Service) SetSnapshotProvider(provider func(context.Context, string) (*resolver.ResolvedPolicySnapshot, error)) {
	s.snapshotProvider = provider
}

// SetInputProvider shares publication's raw input assembly for accurate draft preview.
func (s *Service) SetInputProvider(provider func(context.Context, string) (*resolver.ResolveInput, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputProvider = provider
}

// SetNodeFilterRepository configures the node filter repository dynamically.
func (s *Service) SetNodeFilterRepository(repo domain.NodeFilterRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodeFilterRepo = repo
}

// CreateGroup creates a new policy group and optional initial edges, enforcing topological safety.
func (s *Service) CreateGroup(ctx context.Context, cmd CreateGroupCommand) (*GroupView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, "empty name")
		return nil, domain.NewValidationError("invalid_group_name", "group name cannot be empty")
	}

	if !cmd.GroupType.IsValid() {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("invalid group type: %s", cmd.GroupType))
		return nil, domain.NewValidationError("invalid_group_type", fmt.Sprintf("unsupported group type: %s", cmd.GroupType))
	}

	groupID := cmd.ID
	if groupID == "" {
		var err error
		groupID, err = domain.NewUUIDv7()
		if err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, "failed to generate UUIDv7")
			return nil, err
		}
	} else if !domain.IsValidUUIDv7(groupID) {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("invalid UUIDv7: %s", groupID))
		return nil, domain.NewValidationError("invalid_group_id", fmt.Sprintf("invalid group UUIDv7: %s", groupID))
	}

	now := domain.NowUTC()
	group := domain.NodeGroup{
		EmptyFallbackPass: cmd.EmptyFallbackPass,
		Sources:           append([]domain.MemberSource(nil), cmd.Sources...),
		ID:                groupID,
		Name:              name,
		GroupType:         cmd.GroupType,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if cmd.NodeFilter != nil {
		if err := cmd.NodeFilter.Validate(); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("invalid node filter: %v", err))
			return nil, err
		}
		group.NodeFilter = cmd.NodeFilter
	}

	// Prepare edges if provided
	edges := make([]domain.GroupEdge, len(cmd.Edges))
	for i, in := range cmd.Edges {
		edgeID := in.ID
		if edgeID == "" {
			var err error
			edgeID, err = domain.NewUUIDv7()
			if err != nil {
				s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, "failed to generate edge UUIDv7")
				return nil, err
			}
		}
		pos := in.Position
		if pos == 0 && i > 0 {
			pos = i
		}
		edges[i] = domain.GroupEdge{
			ID:            edgeID,
			ParentGroupID: groupID,
			ChildGroupID:  in.ChildGroupID,
			NodeLogicalID: in.NodeLogicalID,
			Position:      pos,
		}
	}

	// Validate against current graph state
	existingGroups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing groups: %w", err)
	}

	allGroups := append(existingGroups, group)
	edgeMap := make(map[string][]domain.GroupEdge, len(allGroups))
	for _, g := range existingGroups {
		gEdges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to query edges for group %s: %w", g.ID, err)
		}
		edgeMap[g.ID] = gEdges
	}
	edgeMap[groupID] = edges

	// Run full topological validation
	if err := ValidatePolicyGraph(allGroups, edgeMap, nil, nil); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("topology validation failed: %v", err))
		return nil, err
	}
	if err := s.validateNodeTargets(ctx, edges); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("node target validation failed: %v", err))
		return nil, err
	}

	// Persist all SQLite components atomically; other repository implementations
	// retain their existing port behavior.
	if len(cmd.Sources) > 0 {
		sources := make([]domain.MemberSource, len(cmd.Sources))
		copy(sources, cmd.Sources)
		for i := range sources {
			sources[i].Position = i
			sources[i].GroupID = groupID
			if err := domain.ValidateMemberSource(sources[i]); err != nil {
				s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("invalid source: %v", err))
				return nil, err
			}
		}
		group.Sources = sources
		allGroups := append(existingGroups, group)
		if err := domain.ValidatePolicyTopology(allGroups, nil, nil, false); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("topology validation failed: %v", err))
			return nil, err
		}
		saved, err := s.policyRepo.SaveGroupWithSources(ctx, &group, 0, sources)
		if err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("failed to save group: %v", err))
			return nil, err
		}
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultSuccess, fmt.Sprintf("id=%s, name=%s", saved.ID, saved.Name))
		return toGroupView(saved, nil), nil
	}
	if atomic, ok := s.policyRepo.(domain.AtomicGroupRepository); ok {
		if err := atomic.SaveGroup(ctx, &group, true, &edges, cmd.NodeFilter != nil, cmd.NodeFilter); err != nil {
			return nil, err
		}
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultSuccess, fmt.Sprintf("id=%s", group.ID))
		return toGroupView(&group, edges), nil
	}
	if err := s.policyRepo.CreateGroup(ctx, &group); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("failed to save group: %v", err))
		return nil, err
	}

	if len(edges) > 0 {
		if err := s.policyRepo.SetEdgesForGroup(ctx, groupID, edges); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("failed to save edges: %v", err))
			return nil, err
		}
	}

	if group.NodeFilter != nil && s.nodeFilterRepo != nil {
		err := s.nodeFilterRepo.SetGroupFilter(ctx, &domain.GroupNodeFilter{
			GroupID:   groupID,
			Spec:      *group.NodeFilter,
			UpdatedAt: now,
		})
		if err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultFailure, fmt.Sprintf("failed to save group filter: %v", err))
			return nil, err
		}
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.create", domain.AuditResultSuccess, fmt.Sprintf("id=%s, name=%s", group.ID, group.Name))
	return toGroupView(&group, edges), nil
}

// GetGroup retrieves a single policy group with its edges by ID.
func (s *Service) GetGroup(ctx context.Context, id string) (*GroupView, error) {
	group, err := s.policyRepo.GetGroupByID(ctx, id)
	if err != nil {
		return nil, err
	}

	edges, err := s.policyRepo.ListEdgesByGroup(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch edges for group %s: %w", id, err)
	}

	return toGroupView(group, edges), nil
}

// ListGroups lists policy groups with pagination and their outgoing edges.
func (s *Service) ListGroups(ctx context.Context, query ListGroupsQuery) (*ListGroupsResult, error) {
	allGroups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	// Filter by search text if specified
	search := strings.ToLower(strings.TrimSpace(query.Search))
	var filtered []domain.NodeGroup
	if search == "" {
		filtered = allGroups
	} else {
		for _, g := range allGroups {
			if strings.Contains(strings.ToLower(g.Name), search) || strings.Contains(strings.ToLower(string(g.GroupType)), search) {
				filtered = append(filtered, g)
			}
		}
	}

	total := len(filtered)
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	sliced := filtered[start:end]
	items := make([]GroupView, len(sliced))
	for i, g := range sliced {
		edges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list edges for group %s: %w", g.ID, err)
		}
		items[i] = *toGroupView(&g, edges)
	}

	return &ListGroupsResult{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// UpdateGroup updates group metadata and optionally modifies edges with cycle validation.
func (s *Service) UpdateGroup(ctx context.Context, cmd UpdateGroupCommand) (*GroupView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, err := s.policyRepo.GetGroupByID(ctx, cmd.ID)
	if err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("group %s not found", cmd.ID))
		return nil, err
	}

	if cmd.Name != nil {
		name := strings.TrimSpace(*cmd.Name)
		if name == "" {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, "empty name")
			return nil, domain.NewValidationError("invalid_group_name", "group name cannot be empty")
		}
		group.Name = name
	}

	if cmd.GroupType != nil {
		if !cmd.GroupType.IsValid() {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("invalid group type: %s", *cmd.GroupType))
			return nil, domain.NewValidationError("invalid_group_type", fmt.Sprintf("unsupported group type: %s", *cmd.GroupType))
		}
		group.GroupType = *cmd.GroupType
	}

	existingGroups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing groups: %w", err)
	}

	if cmd.Sources == nil {
		var legacyEdges *[]domain.GroupEdge
		if cmd.Edges != nil {
			list := make([]domain.GroupEdge, len(*cmd.Edges))
			for i, e := range *cmd.Edges {
				list[i] = domain.GroupEdge{ID: e.ID, ParentGroupID: group.ID, ChildGroupID: e.ChildGroupID, NodeLogicalID: e.NodeLogicalID, Position: i}
			}
			if err := s.validateNodeTargets(ctx, list); err != nil {
				return nil, err
			}
			legacyEdges = &list
		}
		sources := group.Sources
		if legacyEdges != nil || cmd.ClearNodeFilter || cmd.NodeFilter != nil {
			filter := cmd.NodeFilter
			if cmd.ClearNodeFilter {
				filter = nil
			}
			converted, err := domain.EditLegacySources(group.ID, sources, legacyEdges, cmd.ClearNodeFilter || cmd.NodeFilter != nil, filter)
			if err != nil {
				return nil, err
			}
			sources = converted
		}
		cmd.Sources = &sources
		if cmd.ExpectedRevision == nil {
			revision := group.Revision
			cmd.ExpectedRevision = &revision
		}
	}
	if cmd.Sources != nil {
		sources := make([]domain.MemberSource, len(*cmd.Sources))
		copy(sources, *cmd.Sources)
		for i := range sources {
			sources[i].Position = i
			sources[i].GroupID = group.ID
			if err := domain.ValidateMemberSource(sources[i]); err != nil {
				s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("invalid source: %v", err))
				return nil, err
			}
		}
		if cmd.EmptyFallbackPass != nil {
			group.EmptyFallbackPass = *cmd.EmptyFallbackPass
		}
		if s.nodeRepo != nil {
			var nodeEdges []domain.GroupEdge
			for _, src := range sources {
				if src.Kind == domain.SourceKindManualNode {
					nodeEdges = append(nodeEdges, domain.GroupEdge{NodeLogicalID: src.NodeLogicalID})
				}
			}
			if err := s.validateNodeTargets(ctx, nodeEdges); err != nil {
				return nil, err
			}
		}
		group.Sources = sources
		for i := range existingGroups {
			if existingGroups[i].ID == group.ID {
				existingGroups[i] = *group
				break
			}
		}
		if err := domain.ValidatePolicyTopology(existingGroups, nil, nil, false); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("validation failed: %v", err))
			return nil, err
		}
		expectedRev := group.Revision
		if cmd.ExpectedRevision != nil {
			if *cmd.ExpectedRevision <= 0 {
				return nil, domain.NewValidationError("invalid_revision", "expected_revision must be positive")
			}
			expectedRev = *cmd.ExpectedRevision
		}
		saved, err := s.policyRepo.SaveGroupWithSources(ctx, group, expectedRev, sources)
		if err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("failed to save group with sources: %v", err))
			return nil, err
		}
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultSuccess, fmt.Sprintf("id=%s", group.ID))
		return toGroupView(saved, nil), nil
	}

	return nil, domain.NewInternalError("sources_required", "group update requires normalized sources")
}

// DeleteGroup deletes a group by ID after verifying no other groups reference it.
func (s *Service) DeleteGroup(ctx context.Context, cmd DeleteGroupCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, err := s.policyRepo.GetGroupByID(ctx, cmd.ID)
	if err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.delete", domain.AuditResultFailure, fmt.Sprintf("group %s not found", cmd.ID))
		return err
	}

	if err := s.policyRepo.DeleteGroup(ctx, cmd.ID); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.delete", domain.AuditResultFailure, fmt.Sprintf("failed to delete group: %v", err))
		return err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.delete", domain.AuditResultSuccess, fmt.Sprintf("id=%s, name=%s", group.ID, group.Name))
	return nil
}

// SetGroupEdges replaces all outgoing edges for a group, enforcing cycle and self-loop checks.
func (s *Service) SetGroupEdges(ctx context.Context, cmd SetGroupEdgesCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure parent group exists
	if _, err := s.policyRepo.GetGroupByID(ctx, cmd.ParentGroupID); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.set_edges", domain.AuditResultFailure, fmt.Sprintf("parent group %s not found", cmd.ParentGroupID))
		return err
	}

	edges := make([]domain.GroupEdge, len(cmd.Edges))
	for i, in := range cmd.Edges {
		edgeID := in.ID
		if edgeID == "" {
			var err error
			edgeID, err = domain.NewUUIDv7()
			if err != nil {
				return err
			}
		}
		pos := in.Position
		if pos == 0 && i > 0 {
			pos = i
		}
		edges[i] = domain.GroupEdge{
			ID:            edgeID,
			ParentGroupID: cmd.ParentGroupID,
			ChildGroupID:  in.ChildGroupID,
			NodeLogicalID: in.NodeLogicalID,
			Position:      pos,
		}
	}

	existingGroups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to query groups: %w", err)
	}

	edgeMap := make(map[string][]domain.GroupEdge, len(existingGroups))
	for _, g := range existingGroups {
		if g.ID == cmd.ParentGroupID {
			edgeMap[g.ID] = edges
		} else {
			gEdges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
			if err != nil {
				return fmt.Errorf("failed to query edges for group %s: %w", g.ID, err)
			}
			edgeMap[g.ID] = gEdges
		}
	}

	if err := ValidatePolicyGraph(existingGroups, edgeMap, nil, nil); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.set_edges", domain.AuditResultFailure, fmt.Sprintf("validation failed: %v", err))
		return err
	}
	if err := s.validateNodeTargets(ctx, edges); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.set_edges", domain.AuditResultFailure, fmt.Sprintf("node target validation failed: %v", err))
		return err
	}

	if err := s.policyRepo.SetEdgesForGroup(ctx, cmd.ParentGroupID, edges); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.set_edges", domain.AuditResultFailure, fmt.Sprintf("failed to save edges: %v", err))
		return err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.set_edges", domain.AuditResultSuccess, fmt.Sprintf("parent_id=%s, count=%d", cmd.ParentGroupID, len(edges)))
	return nil
}

// GetGroupEdges retrieves outgoing edges for a group.
func (s *Service) GetGroupEdges(ctx context.Context, groupID string) ([]GroupEdgeView, error) {
	edges, err := s.policyRepo.ListEdgesByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	views := make([]GroupEdgeView, len(edges))
	for i, e := range edges {
		views[i] = GroupEdgeView{
			ID:            e.ID,
			ParentGroupID: e.ParentGroupID,
			ChildGroupID:  e.ChildGroupID,
			NodeLogicalID: e.NodeLogicalID,
			Position:      e.Position,
		}
	}
	return views, nil
}

// CreateAdmissionRule validates and persists an admission rule.
func (s *Service) CreateAdmissionRule(ctx context.Context, cmd CreateAdmissionRuleCommand) (*AdmissionRuleView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.create", domain.AuditResultFailure, "empty name")
		return nil, domain.NewValidationError("invalid_admission_rule_name", "admission rule name cannot be empty")
	}

	expr := strings.TrimSpace(cmd.Expression)
	if expr == "" {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.create", domain.AuditResultFailure, "empty expression")
		return nil, domain.NewValidationError("invalid_expression", "admission rule expression cannot be empty")
	}

	if !cmd.Action.IsValid() {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.create", domain.AuditResultFailure, fmt.Sprintf("invalid rule action: %s", cmd.Action))
		return nil, domain.NewValidationError("invalid_rule_action", fmt.Sprintf("unsupported rule action: %s", cmd.Action))
	}

	if cmd.Position < 0 {
		return nil, domain.NewValidationError("invalid_position", "position must be non-negative")
	}

	ruleID := cmd.ID
	if ruleID == "" {
		var err error
		ruleID, err = domain.NewUUIDv7()
		if err != nil {
			return nil, err
		}
	}

	revID := strings.TrimSpace(cmd.RevisionID)
	if revID == "" {
		active, err := s.ensureActiveRevisionLocked(ctx)
		if err != nil {
			return nil, err
		}
		if active == nil {
			return nil, domain.NewValidationError("missing_revision_id", "no active revision exists; specify a revision_id or activate a revision")
		}
		revID = active.ID
	}

	rule := domain.AdmissionRule{
		ID:         ruleID,
		RevisionID: revID,
		Name:       name,
		Expression: expr,
		Action:     cmd.Action,
		Position:   cmd.Position,
	}

	if err := s.policyRepo.CreateAdmissionRule(ctx, &rule); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.create", domain.AuditResultFailure, fmt.Sprintf("failed to save admission rule: %v", err))
		return nil, err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.create", domain.AuditResultSuccess, fmt.Sprintf("id=%s, name=%s", rule.ID, rule.Name))
	return &AdmissionRuleView{
		ID:         rule.ID,
		RevisionID: rule.RevisionID,
		Name:       rule.Name,
		Expression: rule.Expression,
		Action:     rule.Action,
		Position:   rule.Position,
	}, nil
}

// CreatePolicyRule validates target existence and MATCH ordering before persisting a policy rule.
func (s *Service) CreatePolicyRule(ctx context.Context, cmd CreatePolicyRuleCommand) (*PolicyRuleView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	expr := strings.TrimSpace(cmd.Expression)
	if expr == "" {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.create", domain.AuditResultFailure, "empty expression")
		return nil, domain.NewValidationError("invalid_expression", "policy rule expression cannot be empty")
	}

	if cmd.Position < 0 {
		return nil, domain.NewValidationError("invalid_position", "position must be non-negative")
	}

	// Verify target group exists
	if _, err := s.policyRepo.GetGroupByID(ctx, cmd.TargetGroupID); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.create", domain.AuditResultFailure, fmt.Sprintf("target group %s not found", cmd.TargetGroupID))
		return nil, domain.NewValidationError("target_group_not_found", fmt.Sprintf("target group %s does not exist", cmd.TargetGroupID))
	}

	ruleID := cmd.ID
	if ruleID == "" {
		var err error
		ruleID, err = domain.NewUUIDv7()
		if err != nil {
			return nil, err
		}
	}

	revID := strings.TrimSpace(cmd.RevisionID)
	if revID == "" {
		active, err := s.ensureActiveRevisionLocked(ctx)
		if err != nil {
			return nil, err
		}
		if active == nil {
			return nil, domain.NewValidationError("missing_revision_id", "no active revision exists; specify a revision_id or activate a revision")
		}
		revID = active.ID
	}

	// Check against existing rules in this revision for MATCH ordering
	existingRules, err := s.policyRepo.ListPolicyRules(ctx, revID)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing policy rules: %w", err)
	}

	newRule := domain.PolicyRule{
		ID:            ruleID,
		RevisionID:    revID,
		TargetGroupID: cmd.TargetGroupID,
		Expression:    expr,
		Position:      cmd.Position,
	}

	combinedRules := append(existingRules, newRule)
	if err := ValidatePolicyRules(combinedRules, nil); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.create", domain.AuditResultFailure, fmt.Sprintf("rule validation failed: %v", err))
		return nil, err
	}

	if err := s.policyRepo.CreatePolicyRule(ctx, &newRule); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.create", domain.AuditResultFailure, fmt.Sprintf("failed to save policy rule: %v", err))
		return nil, err
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.create", domain.AuditResultSuccess, fmt.Sprintf("id=%s, expr=%s", newRule.ID, newRule.Expression))
	return &PolicyRuleView{
		ID:            newRule.ID,
		RevisionID:    newRule.RevisionID,
		TargetGroupID: newRule.TargetGroupID,
		Expression:    newRule.Expression,
		Position:      newRule.Position,
	}, nil
}

// DeleteRule deletes an admission or routing policy rule by ID.
func (s *Service) DeleteRule(ctx context.Context, cmd DeleteRuleCommand) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(cmd.ID)
	if id == "" {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.delete", domain.AuditResultFailure, "empty rule id")
		return domain.NewValidationError("missing_rule_id", "rule id is required")
	}

	kind := strings.ToLower(strings.TrimSpace(cmd.Kind))
	switch kind {
	case "admission":
		if err := s.policyRepo.DeleteAdmissionRule(ctx, id); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.delete", domain.AuditResultFailure, fmt.Sprintf("rule %s: %v", id, err))
			return err
		}
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.delete", domain.AuditResultSuccess, fmt.Sprintf("id=%s", id))
		return nil
	case "policy":
		if err := s.policyRepo.DeletePolicyRule(ctx, id); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.delete", domain.AuditResultFailure, fmt.Sprintf("rule %s: %v", id, err))
			return err
		}
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.delete", domain.AuditResultSuccess, fmt.Sprintf("id=%s", id))
		return nil
	case "":
		err := s.policyRepo.DeletePolicyRule(ctx, id)
		if err == nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.delete", domain.AuditResultSuccess, fmt.Sprintf("id=%s", id))
			return nil
		}
		if domErr, ok := domain.AsDomainError(err); ok && domErr.Category == domain.CategoryNotFound {
			admErr := s.policyRepo.DeleteAdmissionRule(ctx, id)
			if admErr == nil {
				s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "admission_rule.delete", domain.AuditResultSuccess, fmt.Sprintf("id=%s", id))
				return nil
			}
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.delete", domain.AuditResultFailure, fmt.Sprintf("rule %s not found", id))
			return admErr
		}
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.delete", domain.AuditResultFailure, fmt.Sprintf("rule %s: %v", id, err))
		return err
	default:
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_rule.delete", domain.AuditResultFailure, fmt.Sprintf("invalid rule kind: %s", kind))
		return domain.NewValidationError("invalid_rule_kind", fmt.Sprintf("unsupported rule kind: %s", cmd.Kind))
	}
}

// DeletePolicyRule deletes a routing policy rule by ID.
func (s *Service) DeletePolicyRule(ctx context.Context, cmd DeleteRuleCommand) error {
	cmd.Kind = "policy"
	return s.DeleteRule(ctx, cmd)
}

// DeleteAdmissionRule deletes an admission rule by ID.
func (s *Service) DeleteAdmissionRule(ctx context.Context, cmd DeleteRuleCommand) error {
	cmd.Kind = "admission"
	return s.DeleteRule(ctx, cmd)
}

// ListRules retrieves admission and policy rules for a revision.
func (s *Service) ListRules(ctx context.Context, query ListRulesQuery) (*ListRulesResult, error) {
	revID := query.RevisionID
	if revID == "" {
		active, err := s.revisionRepo.GetActive(ctx)
		if err != nil {
			return &ListRulesResult{RevisionID: "", AdmissionRules: []AdmissionRuleView{}, PolicyRules: []PolicyRuleView{}}, nil
		}
		revID = active.ID
	}

	var admViews []AdmissionRuleView
	var polViews []PolicyRuleView

	kind := strings.ToLower(strings.TrimSpace(query.Kind))

	if kind == "" || kind == "admission" {
		admRules, err := s.policyRepo.ListAdmissionRules(ctx, revID)
		if err != nil {
			return nil, fmt.Errorf("failed to query admission rules: %w", err)
		}
		admViews = make([]AdmissionRuleView, len(admRules))
		for i, r := range admRules {
			admViews[i] = AdmissionRuleView{
				ID:         r.ID,
				RevisionID: r.RevisionID,
				Name:       r.Name,
				Expression: r.Expression,
				Action:     r.Action,
				Position:   r.Position,
			}
		}
	}

	if kind == "" || kind == "policy" {
		polRules, err := s.policyRepo.ListPolicyRules(ctx, revID)
		if err != nil {
			return nil, fmt.Errorf("failed to query policy rules: %w", err)
		}
		polViews = make([]PolicyRuleView, len(polRules))
		for i, r := range polRules {
			polViews[i] = PolicyRuleView{
				ID:            r.ID,
				RevisionID:    r.RevisionID,
				TargetGroupID: r.TargetGroupID,
				Expression:    r.Expression,
				Position:      r.Position,
			}
		}
	}

	total := len(admViews) + len(polViews)

	return &ListRulesResult{
		RevisionID:     revID,
		AdmissionRules: admViews,
		PolicyRules:    polViews,
		Page:           query.Page,
		PageSize:       query.PageSize,
		Total:          total,
	}, nil
}

// PreviewGroup generates an in-RAM preview of a policy group draft without persisting to database.
func (s *Service) PreviewGroup(ctx context.Context, cmd PreviewGroupCommand) (*GroupPreviewResponse, error) {
	sources := make([]domain.MemberSource, len(cmd.Sources))
	copy(sources, cmd.Sources)
	for i := range sources {
		sources[i].Position = i
		if err := domain.ValidateMemberSource(sources[i]); err != nil {
			return nil, err
		}
	}

	var groupID string
	var name string
	var groupType domain.GroupType = domain.GroupTypeSelect
	var emptyFallback bool = false
	var revision int64 = 1

	if cmd.GroupID != nil && *cmd.GroupID != "" {
		groupID = *cmd.GroupID
		existing, err := s.policyRepo.GetGroupByID(ctx, groupID)
		if err != nil {
			return nil, err
		}
		name = existing.Name
		groupType = existing.GroupType
		emptyFallback = existing.EmptyFallbackPass
		revision = existing.Revision

		if cmd.ExpectedRevision != nil && *cmd.ExpectedRevision > 0 && *cmd.ExpectedRevision != revision {
			conflictErr := domain.NewConflictError("revision_conflict", fmt.Sprintf("Policy group revision mismatch. Current revision is %d.", revision))
			conflictErr.CurrentRevision = &revision
			return nil, conflictErr
		}
	} else {
		var err error
		groupID, err = domain.NewUUIDv7()
		if err != nil {
			return nil, err
		}
		name = "Preview Group"
	}

	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) != "" {
		name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.GroupType != nil {
		if !cmd.GroupType.IsValid() {
			return nil, domain.NewValidationError("invalid_group_type", fmt.Sprintf("unsupported group type: %s", *cmd.GroupType))
		}
		groupType = *cmd.GroupType
	}
	if cmd.EmptyFallbackPass != nil {
		emptyFallback = *cmd.EmptyFallbackPass
	}

	previewGroup := domain.NodeGroup{
		ID:                groupID,
		Name:              name,
		GroupType:         groupType,
		EmptyFallbackPass: emptyFallback,
		Revision:          revision,
		Sources:           sources,
		CreatedAt:         domain.NowUTC(),
		UpdatedAt:         domain.NowUTC(),
	}

	existingGroups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}
	allSources := make(map[string][]domain.MemberSource, len(existingGroups))
	for _, g := range existingGroups {
		allSources[g.ID] = g.Sources
	}

	found := false
	simulatedGroups := make([]domain.NodeGroup, len(existingGroups))
	copy(simulatedGroups, existingGroups)
	for i := range simulatedGroups {
		if simulatedGroups[i].ID == groupID {
			simulatedGroups[i] = previewGroup
			found = true
			break
		}
	}
	if !found {
		simulatedGroups = append(simulatedGroups, previewGroup)
	}
	allSources[groupID] = sources
	for i := range simulatedGroups {
		simulatedGroups[i].Sources = allSources[simulatedGroups[i].ID]
	}

	if err := domain.ValidatePolicyTopology(simulatedGroups, nil, nil, false); err != nil {
		return nil, err
	}

	var activeRevID string
	if s.revisionRepo != nil {
		active, err := s.revisionRepo.GetActive(ctx)
		if err == nil && active != nil {
			activeRevID = active.ID
		}
	}

	var polRules []domain.PolicyRule
	var admRules []domain.AdmissionRule
	if activeRevID != "" && s.policyRepo != nil {
		polRules, _ = s.policyRepo.ListPolicyRules(ctx, activeRevID)
		admRules, _ = s.policyRepo.ListAdmissionRules(ctx, activeRevID)
	}

	var nodes []domain.Node
	if s.nodeRepo != nil {
		nodes, _ = s.nodeRepo.ListAll(ctx, domain.NodeFilter{
			Scope:          domain.NodeScopeEnabledSubscriptions,
			ActiveOnly:     true,
			ExcludeNotices: true,
		})
	}

	var globalFilter *domain.NodeFilterSpec
	var groupFilters map[string]domain.NodeFilterSpec
	if s.nodeFilterRepo != nil {
		gf, err := s.nodeFilterRepo.GetGlobalFilter(ctx)
		if err == nil && gf != nil && !gf.Spec.IsEmpty() {
			globalFilter = &gf.Spec
		}
		groupFilters, _ = s.nodeFilterRepo.ListGroupFilters(ctx)
	}

	var resInput resolver.ResolveInput
	if s.inputProvider != nil && activeRevID != "" {
		baseInput, err := s.inputProvider(ctx, activeRevID)
		if err != nil {
			return nil, err
		}
		if baseInput == nil {
			return nil, domain.NewInternalError("preview_input_unavailable", "publication input is unavailable")
		}
		if baseInput != nil {
			resInput = *baseInput
			simGroups := make([]domain.NodeGroup, len(baseInput.Groups))
			copy(simGroups, baseInput.Groups)
			simSources := make(map[string][]domain.MemberSource, len(baseInput.GroupSources)+1)
			for k, v := range baseInput.GroupSources {
				simSources[k] = v
			}
			foundGroup := false
			for i := range simGroups {
				if simGroups[i].ID == groupID {
					simGroups[i] = previewGroup
					foundGroup = true
					break
				}
			}
			if !foundGroup {
				simGroups = append(simGroups, previewGroup)
			}
			simSources[groupID] = sources
			for i := range simGroups {
				simGroups[i].Sources = simSources[simGroups[i].ID]
			}
			resInput.Groups = simGroups
			resInput.GroupSources = simSources
		}
	}
	if len(resInput.Groups) == 0 {
		resInput = resolver.ResolveInput{
			RevisionID:         activeRevID,
			InventoryWatermark: "preview-ram",
			CompilerVersion:    "1.0.0",
			Nodes:              nodes,
			Groups:             simulatedGroups,
			GroupSources:       allSources,
			PolicyRules:        polRules,
			AdmissionRules:     admRules,
			GlobalFilter:       globalFilter,
			GroupFilters:       groupFilters,
			DNS: resolver.DNSConfig{
				Enabled:     true,
				Nameservers: []string{"1.1.1.1", "8.8.8.8"},
			},
			AsOf: domain.NowUTC(),
		}
	}

	r := resolver.New()
	snap, err := r.Resolve(ctx, resInput)
	if err != nil {
		return nil, err
	}

	var targetRG *resolver.ResolvedGroup
	for i := range snap.Groups {
		if snap.Groups[i].ID == groupID {
			targetRG = &snap.Groups[i]
			break
		}
	}
	if targetRG == nil {
		return nil, domain.NewInternalError("preview_resolution_failed", "preview group not found in resolved snapshot")
	}

	previewMembers := make([]GroupPreviewMember, len(targetRG.Members))
	for i, m := range targetRG.Members {
		pm := GroupPreviewMember{
			Kind:        m.Kind,
			DisplayName: m.DisplayName,
			Position:    m.Position,
		}
		switch m.Kind {
		case resolver.MemberKindNode:
			targetID := m.TargetID
			pm.NodeLogicalID = &targetID
		case resolver.MemberKindGroup:
			targetID := m.TargetID
			pm.GroupID = &targetID
		case resolver.MemberKindBuiltin:
			name := m.DisplayName
			pm.BuiltinName = &name
		}
		previewMembers[i] = pm
	}

	var groupDiags []resolver.Diagnostic
	for _, d := range snap.Diagnostics {
		if d.Target == groupID || d.Target == name || d.Severity == resolver.DiagnosticSeverityError {
			groupDiags = append(groupDiags, d)
		}
	}

	resp := &GroupPreviewResponse{
		GroupID:         groupID,
		Revision:        revision,
		SelectionDigest: snap.SnapshotDigest,
		Members:         previewMembers,
		NodeCount:       len(targetRG.AllNodeLogicalIDs),
		Diagnostics:     groupDiags,
	}

	if cmd.Target != nil && *cmd.Target != "" {
		res, compErr := compiler.Compile(ctx, snap, *cmd.Target)
		if compErr != nil {
			return nil, compErr
		}
		resp.ContentDigest = &res.ContentDigest
	}
	return resp, nil
}

// ValidateGraph checks the complete graph and rules currently persisted in the database.
// It performs static topology validation, admission rule checks, and dynamic group resolution against active nodes.
func (s *Service) ValidateGraph(ctx context.Context) (*ValidationResult, error) {
	groups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	edgeMap := make(map[string][]domain.GroupEdge, len(groups))
	groupMap := make(map[string]domain.NodeGroup, len(groups))
	for _, g := range groups {
		groupMap[g.ID] = g
		edges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list edges for group %s: %w", g.ID, err)
		}
		edgeMap[g.ID] = edges
	}

	var activeRevID string
	if s.revisionRepo != nil {
		active, err := s.revisionRepo.GetActive(ctx)
		if err != nil {
			var de *domain.DomainError
			if !errors.As(err, &de) || de.Category != domain.CategoryNotFound {
				return nil, fmt.Errorf("validation incomplete: active revision: %w", err)
			}
		}
		if active != nil {
			activeRevID = active.ID
		}
	}

	var polRules []domain.PolicyRule
	var admRules []domain.AdmissionRule
	if activeRevID != "" && s.policyRepo != nil {
		polRules, err = s.policyRepo.ListPolicyRules(ctx, activeRevID)
		if err != nil {
			return nil, fmt.Errorf("validation incomplete: policy rules: %w", err)
		}
		admRules, err = s.policyRepo.ListAdmissionRules(ctx, activeRevID)
		if err != nil {
			return nil, fmt.Errorf("validation incomplete: admission rules: %w", err)
		}
	}

	var issues []ValidationIssue
	var errorsList []string

	// 1. Static topology validation
	topErr := domain.ValidatePolicyTopology(groups, edgeMap, polRules, true)
	if topErr != nil {
		errorsList = append(errorsList, topErr.Error())
		var domErr *domain.DomainError
		code := "topology_invalid"
		if errors.As(topErr, &domErr) {
			code = domErr.Code
		}
		issues = append(issues, ValidationIssue{
			Code:     code,
			Severity: "error",
			Message:  topErr.Error(),
		})
		return &ValidationResult{
			Valid:      false,
			Errors:     errorsList,
			RevisionID: activeRevID,
			Issues:     issues,
		}, topErr
	}

	admErr := ValidateAdmissionRules(admRules)
	if admErr != nil {
		errorsList = append(errorsList, admErr.Error())
		issues = append(issues, ValidationIssue{
			Code:     "invalid_admission_rule",
			Severity: "error",
			Message:  admErr.Error(),
		})
	}

	// 2. Fetch active nodes in CurrentEnabledSubscription scope
	var nodes []domain.Node
	if s.nodeRepo != nil {
		nodes, err = s.nodeRepo.ListAll(ctx, domain.NodeFilter{
			Scope:          domain.NodeScopeEnabledSubscriptions,
			ActiveOnly:     true,
			ExcludeNotices: true,
		})
		if err != nil {
			return nil, fmt.Errorf("validation incomplete: nodes: %w", err)
		}
	}

	// 3. Fetch global & group filters
	var globalFilter *domain.NodeFilterSpec
	var groupFilters map[string]domain.NodeFilterSpec
	if s.nodeFilterRepo != nil {
		gf, err := s.nodeFilterRepo.GetGlobalFilter(ctx)
		if err != nil {
			return nil, fmt.Errorf("validation incomplete: global filter: %w", err)
		}
		if gf != nil && !gf.Spec.IsEmpty() {
			globalFilter = &gf.Spec
		}
		groupFilters, err = s.nodeFilterRepo.ListGroupFilters(ctx)
		if err != nil {
			return nil, fmt.Errorf("validation incomplete: group filters: %w", err)
		}
	}

	// 4. Deterministic Resolver execution (in-memory, no mutation, no external network)
	allSources, _ := s.policyRepo.ListAllSources(ctx)
	if allSources != nil {
		for i := range groups {
			if s, ok := allSources[groups[i].ID]; ok && len(s) > 0 {
				groups[i].Sources = s
			}
		}
	}

	resInput := resolver.ResolveInput{
		RevisionID:         activeRevID,
		InventoryWatermark: "v1",
		CompilerVersion:    "1.0.0",
		Nodes:              nodes,
		Groups:             groups,
		GroupSources:       allSources,
		Edges:              edgeMap,
		PolicyRules:        polRules,
		AdmissionRules:     admRules,
		GlobalFilter:       globalFilter,
		GroupFilters:       groupFilters,
		DNS: resolver.DNSConfig{
			Enabled:     true,
			Nameservers: []string{"1.1.1.1", "8.8.8.8"},
		},
		AsOf: domain.NowUTC(),
	}

	var snap *resolver.ResolvedPolicySnapshot
	var snapErr error
	if s.snapshotProvider != nil && activeRevID != "" {
		snap, snapErr = s.snapshotProvider(ctx, activeRevID)
		if snapErr != nil {
			return nil, fmt.Errorf("validation incomplete: %w", snapErr)
		}
	} else {
		// Standalone services cannot silently validate evidence-dependent filters.
		for _, spec := range groupFilters {
			for _, c := range spec.Conditions {
				if c.Field == domain.FilterFieldSourceSubscriptions || c.Field == domain.FilterFieldProbeVerdict || c.Field == domain.FilterFieldProbeLatencyMS {
					return nil, domain.NewInternalError("validation_incomplete", "publication snapshot provider required for evidence filters")
				}
			}
		}
		if globalFilter != nil {
			for _, c := range globalFilter.Conditions {
				if c.Field == domain.FilterFieldSourceSubscriptions || c.Field == domain.FilterFieldProbeVerdict || c.Field == domain.FilterFieldProbeLatencyMS {
					return nil, domain.NewInternalError("validation_incomplete", "publication snapshot provider required for evidence filters")
				}
			}
		}
		r := resolver.New()
		snap, snapErr = r.Resolve(ctx, resInput)
	}
	if snapErr != nil {
		if topErr == nil {
			errorsList = append(errorsList, snapErr.Error())
			issues = append(issues, ValidationIssue{
				Code:     "resolver_error",
				Severity: "error",
				Message:  snapErr.Error(),
			})
		}
	} else {
		resolvedGroupMap := make(map[string]resolver.ResolvedGroup, len(snap.Groups))
		for _, rg := range snap.Groups {
			resolvedGroupMap[rg.ID] = rg
		}

		// Check each policy rule against its target group
		for _, rule := range polRules {
			rType, rVal := parseRuleExpr(rule.Expression)
			tGroup, exists := groupMap[rule.TargetGroupID]
			if !exists {
				errMsg := fmt.Sprintf("分流规则 %q (位置 %d) 指向的策略组 %s 不存在", rule.Expression, rule.Position, rule.TargetGroupID)
				errorsList = append(errorsList, errMsg)
				issues = append(issues, ValidationIssue{
					Code:            "target_group_not_found",
					Severity:        "error",
					RuleID:          rule.ID,
					Position:        rule.Position,
					Type:            rType,
					Value:           rVal,
					TargetGroupID:   rule.TargetGroupID,
					TargetGroupName: "",
					Message:         errMsg,
				})
				continue
			}

			rg, ok := resolvedGroupMap[rule.TargetGroupID]
			if !ok || (!rg.UsesEmptyPass() && (len(rg.Members) == 0 || len(rg.AllNodeLogicalIDs) == 0)) {
				var errMsg string
				if len(nodes) == 0 {
					errMsg = fmt.Sprintf("当前有效库存为空，分流规则 %q (位置 %d) 指向的策略组 %q (%s) 无可用节点", rule.Expression, rule.Position, tGroup.Name, tGroup.ID)
				} else {
					errMsg = fmt.Sprintf("分流规则 %q (位置 %d) 指向的策略组 %q (%s) 经解析无可用节点 (0 个可用节点)", rule.Expression, rule.Position, tGroup.Name, tGroup.ID)
				}
				errorsList = append(errorsList, errMsg)
				issues = append(issues, ValidationIssue{
					Code:            "empty_routed_group",
					Severity:        "error",
					RuleID:          rule.ID,
					Position:        rule.Position,
					Type:            rType,
					Value:           rVal,
					TargetGroupID:   tGroup.ID,
					TargetGroupName: tGroup.Name,
					Message:         errMsg,
				})
			}
		}

		// Check unrouted empty groups (warnings only)
		routedTargetIDs := make(map[string]bool, len(polRules))
		for _, rule := range polRules {
			routedTargetIDs[rule.TargetGroupID] = true
		}

		for _, rg := range snap.Groups {
			if !rg.UsesEmptyPass() && !routedTargetIDs[rg.ID] && (len(rg.Members) == 0 || len(rg.AllNodeLogicalIDs) == 0) {
				issues = append(issues, ValidationIssue{
					Code:            "empty_group",
					Severity:        "warning",
					TargetGroupID:   rg.ID,
					TargetGroupName: rg.Name,
					Message:         fmt.Sprintf("策略组 %q (%s) 当前解析结果为空 (0 个成员)", rg.Name, rg.ID),
				})
			}
		}

		// Collect other diagnostics from resolver (e.g. node_inactive_or_missing)
		for _, d := range snap.Diagnostics {
			if d.Code != "empty_routed_group" && d.Code != "empty_group" {
				if d.Severity == resolver.DiagnosticSeverityError {
					errorsList = append(errorsList, d.Message)
				}
				issues = append(issues, ValidationIssue{
					Code:     d.Code,
					Severity: string(d.Severity),
					Message:  d.Message,
				})
			}
		}
	}

	// Sort issues: errors first, then warnings; within each, position ASC, then code ASC
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity == "error"
		}
		if issues[i].Position != issues[j].Position {
			return issues[i].Position < issues[j].Position
		}
		return issues[i].Code < issues[j].Code
	})

	// Deduplicate errorsList
	uniqueErrors := make([]string, 0, len(errorsList))
	seenErr := make(map[string]bool)
	for _, e := range errorsList {
		if !seenErr[e] {
			seenErr[e] = true
			uniqueErrors = append(uniqueErrors, e)
		}
	}

	hasErrors := false
	for _, iss := range issues {
		if iss.Severity == "error" {
			hasErrors = true
			break
		}
	}

	return &ValidationResult{
		Valid:      !hasErrors,
		Errors:     uniqueErrors,
		RevisionID: activeRevID,
		Issues:     issues,
	}, nil
}

func parseRuleExpr(expr string) (string, string) {
	expr = strings.TrimSpace(expr)
	if domain.IsMatchRule(expr) {
		return "MATCH", ""
	}
	parts := strings.Split(expr, ",")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), ""
	}
	return "", ""
}

// EnsureActiveRevision idempotently bootstraps an initial active configuration revision
// when the database has no active revision and zero total revisions.
func (s *Service) EnsureActiveRevision(ctx context.Context) (*domain.ConfigurationRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.ensureActiveRevisionLocked(ctx)
}

// ensureActiveRevisionLocked shares the startup and rule-creation gate under s.mu.
func (s *Service) ensureActiveRevisionLocked(ctx context.Context) (*domain.ConfigurationRevision, error) {
	if s.revisionRepo == nil {
		return nil, domain.NewValidationError("missing_revision_id", "revision repository is not configured")
	}

	active, err := s.revisionRepo.GetActive(ctx)
	if err == nil && active != nil {
		return active, nil
	}
	if err != nil {
		if domErr, ok := domain.AsDomainError(err); !ok || domErr.Category != domain.CategoryNotFound {
			return nil, err
		}
	}

	_, total, err := s.revisionRepo.List(ctx, domain.RevisionFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 1},
	})
	if err != nil {
		return nil, err
	}
	if total > 0 {
		return nil, nil
	}

	return s.bootstrapActiveRevisionLocked(ctx)
}

func (s *Service) bootstrapActiveRevisionLocked(ctx context.Context) (*domain.ConfigurationRevision, error) {
	var groups []domain.NodeGroup
	edgeMap := make(map[string][]domain.GroupEdge)
	if s.policyRepo != nil {
		var err error
		groups, err = s.policyRepo.ListGroups(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query groups: %w", err)
		}
		for _, g := range groups {
			edges, eErr := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
			if eErr != nil {
				return nil, fmt.Errorf("failed to query edges: %w", eErr)
			}
			edgeMap[g.ID] = edges
		}
		if err := ValidatePolicyGraph(groups, edgeMap, nil, nil); err != nil {
			return nil, err
		}
	}

	digest, err := ComputeConfigurationDigest(groups, edgeMap)
	if err != nil {
		return nil, fmt.Errorf("failed to compute content digest: %w", err)
	}

	revID, err := domain.NewUUIDv7()
	if err != nil {
		return nil, err
	}

	rev := domain.ConfigurationRevision{
		ID:            revID,
		ContentDigest: digest,
		State:         domain.RevisionStateDraft,
		CreatedAt:     domain.NowUTC(),
	}

	if err := s.revisionRepo.CreateActive(ctx, &rev); err != nil {
		return nil, fmt.Errorf("failed to create and activate initial revision: %w", err)
	}
	rev.State = domain.RevisionStateActive
	s.recordAudit(ctx, domain.ActorKindSystem, "", "revision.create", domain.AuditResultSuccess, fmt.Sprintf("id=%s, digest=%s, bootstrap=true", rev.ID, rev.ContentDigest))
	return &rev, nil
}

// CreateRevision computes a deterministic ContentDigest from the valid graph state and persists a new ConfigurationRevision.
func (s *Service) CreateRevision(ctx context.Context, cmd CreateRevisionCommand) (*domain.ConfigurationRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify the policy graph is strictly valid first
	groups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}

	edgeMap := make(map[string][]domain.GroupEdge, len(groups))
	for _, g := range groups {
		edges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to query edges: %w", err)
		}
		edgeMap[g.ID] = edges
	}

	if err := ValidatePolicyGraph(groups, edgeMap, nil, nil); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "revision.create", domain.AuditResultFailure, fmt.Sprintf("invalid policy graph: %v", err))
		return nil, err
	}

	// Compute stable ContentDigest
	digest, err := computeConfigurationDigest(groups, edgeMap)
	if err != nil {
		return nil, fmt.Errorf("failed to compute content digest: %w", err)
	}

	revID, err := domain.NewUUIDv7()
	if err != nil {
		return nil, err
	}

	state := cmd.State
	if state == "" {
		state = domain.RevisionStateDraft
	}

	rev := domain.ConfigurationRevision{
		ID:            revID,
		ParentID:      cmd.ParentID,
		ContentDigest: digest,
		State:         state,
		CreatedAt:     domain.NowUTC(),
	}

	var saveErr error
	if state == domain.RevisionStateActive {
		// Never persist state=active before activation; both writes must commit together.
		rev.State = domain.RevisionStateDraft
		saveErr = s.revisionRepo.CreateActive(ctx, &rev)
	} else {
		saveErr = s.revisionRepo.Create(ctx, &rev)
	}
	if saveErr != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "revision.create", domain.AuditResultFailure, fmt.Sprintf("failed to save and activate revision: %v", saveErr))
		return nil, saveErr
	}
	rev.State = state

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "revision.create", domain.AuditResultSuccess, fmt.Sprintf("id=%s, digest=%s", rev.ID, rev.ContentDigest))
	return &rev, nil
}

func (s *Service) validateNodeTargets(ctx context.Context, edges []domain.GroupEdge) error {
	if s.nodeRepo == nil {
		return nil
	}
	for _, edge := range edges {
		if edge.NodeLogicalID == nil || strings.TrimSpace(*edge.NodeLogicalID) == "" {
			continue
		}
		logicalID := strings.TrimSpace(*edge.NodeLogicalID)
		node, err := s.nodeRepo.GetByLogicalID(ctx, logicalID)
		if err != nil {
			if domErr, ok := domain.AsDomainError(err); ok && domErr.Category == domain.CategoryNotFound {
				return domain.NewValidationError("node_target_not_found", fmt.Sprintf("target node %s does not exist", logicalID))
			}
			return fmt.Errorf("failed to query target node %s: %w", logicalID, err)
		}
		if node == nil {
			return domain.NewValidationError("node_target_not_found", fmt.Sprintf("target node %s does not exist", logicalID))
		}
	}
	return nil
}

func (s *Service) recordAudit(ctx context.Context, actorKind domain.ActorKind, requestID, action string, result domain.AuditResult, summary string) {
	if s.auditRepo == nil {
		return
	}
	if actorKind == "" {
		actorKind = domain.ActorKindAdmin
	}
	id, err := domain.NewUUIDv7()
	if err != nil {
		return
	}
	event := domain.AuditEvent{
		ID:              id,
		ActorKind:       actorKind,
		RequestID:       requestID,
		Action:          action,
		Result:          result,
		RedactedSummary: domain.RedactSensitiveInfo(summary),
		CreatedAt:       domain.NowUTC(),
	}
	_ = s.auditRepo.Record(ctx, &event)
}

func toGroupView(group *domain.NodeGroup, edges []domain.GroupEdge) *GroupView {
	// The old filter field is only a lossless view of a filter-only source.
	groupCopy := *group
	group = &groupCopy
	group.NodeFilter = nil
	if len(group.Sources) == 1 && group.Sources[0].Kind == domain.SourceKindFilter && group.Sources[0].Operation == domain.OperationAdd {
		group.NodeFilter = group.Sources[0].FilterSpec
	}
	edges = domain.ProjectSourceEdges(group.Sources)
	edgeViews := make([]GroupEdgeView, len(edges))
	for i, e := range edges {
		edgeViews[i] = GroupEdgeView{
			ID:            e.ID,
			ParentGroupID: e.ParentGroupID,
			ChildGroupID:  e.ChildGroupID,
			NodeLogicalID: e.NodeLogicalID,
			Position:      e.Position,
		}
	}
	sources := group.Sources
	if sources == nil {
		sources = make([]domain.MemberSource, 0)
	}
	rev := group.Revision
	if rev <= 0 {
		rev = 1
	}
	return &GroupView{
		EmptyFallbackPass: group.EmptyFallbackPass,
		ID:                group.ID,
		Name:              group.Name,
		GroupType:         group.GroupType,
		Revision:          rev,
		Sources:           sources,
		Edges:             edgeViews,
		NodeFilter:        group.NodeFilter,
		CreatedAt:         group.CreatedAt,
		UpdatedAt:         group.UpdatedAt,
	}
}

// GetGlobalNodeFilter retrieves the singleton global node filter.
func (s *Service) GetGlobalNodeFilter(ctx context.Context) (*domain.GlobalNodeFilter, error) {
	if s.nodeFilterRepo == nil {
		return &domain.GlobalNodeFilter{
			Spec:      domain.NodeFilterSpec{Conditions: []domain.FilterCondition{}},
			UpdatedAt: domain.NowUTC(),
		}, nil
	}
	return s.nodeFilterRepo.GetGlobalFilter(ctx)
}

// SetGlobalNodeFilter updates the singleton global node filter.
func (s *Service) SetGlobalNodeFilter(ctx context.Context, cmd SetGlobalNodeFilterCommand) (*domain.GlobalNodeFilter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := cmd.Spec.Validate(); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy.global_filter.update", domain.AuditResultFailure, fmt.Sprintf("validation failed: %v", err))
		return nil, err
	}

	filter := &domain.GlobalNodeFilter{
		Spec:      cmd.Spec,
		UpdatedAt: domain.NowUTC(),
	}

	if s.nodeFilterRepo != nil {
		if err := s.nodeFilterRepo.SetGlobalFilter(ctx, filter); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy.global_filter.update", domain.AuditResultFailure, fmt.Sprintf("persistence failed: %v", err))
			return nil, err
		}
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy.global_filter.update", domain.AuditResultSuccess, fmt.Sprintf("conditions_count=%d", len(cmd.Spec.Conditions)))
	return filter, nil
}

// ComputeConfigurationDigest computes a canonical SHA-256 content digest for the given policy groups and edges.
func ComputeConfigurationDigest(groups []domain.NodeGroup, edges map[string][]domain.GroupEdge) (string, error) {
	return computeConfigurationDigest(groups, edges)
}

func computeConfigurationDigest(groups []domain.NodeGroup, edges map[string][]domain.GroupEdge) (string, error) {
	return domain.ComputeConfigurationDigest(groups, edges)
}
