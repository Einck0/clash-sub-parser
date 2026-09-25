// Package policy provides policy graph orchestration, admission rules, and topology validation use cases.
package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"clash-sub-parser/internal/domain"
)

// Service orchestrates policy groups, edges, admission rules, and configuration revisions.
type Service struct {
	policyRepo     domain.PolicyRepository
	revisionRepo   domain.RevisionRepository
	nodeRepo       domain.NodeRepository
	auditRepo      domain.AuditRepository
	nodeFilterRepo domain.NodeFilterRepository
	mu             sync.Mutex
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
		ID:        groupID,
		Name:      name,
		GroupType: cmd.GroupType,
		CreatedAt: now,
		UpdatedAt: now,
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

	// Persist
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

	if s.nodeFilterRepo != nil {
		gf, err := s.nodeFilterRepo.GetGroupFilter(ctx, id)
		if err == nil && gf != nil {
			group.NodeFilter = &gf.Spec
		}
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

	var groupFilters map[string]domain.NodeFilterSpec
	if s.nodeFilterRepo != nil {
		gf, err := s.nodeFilterRepo.ListGroupFilters(ctx)
		if err == nil {
			groupFilters = gf
		}
	}

	sliced := filtered[start:end]
	items := make([]GroupView, len(sliced))
	for i, g := range sliced {
		if groupFilters != nil {
			if spec, ok := groupFilters[g.ID]; ok {
				g.NodeFilter = &spec
			}
		}
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

	edgeMap := make(map[string][]domain.GroupEdge, len(existingGroups))
	for _, g := range existingGroups {
		gEdges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to query edges for group %s: %w", g.ID, err)
		}
		edgeMap[g.ID] = gEdges
	}

	var newEdges []domain.GroupEdge
	if cmd.Edges != nil {
		newEdges = make([]domain.GroupEdge, len(*cmd.Edges))
		for i, in := range *cmd.Edges {
			edgeID := in.ID
			if edgeID == "" {
				var err error
				edgeID, err = domain.NewUUIDv7()
				if err != nil {
					return nil, err
				}
			}
			newEdges[i] = domain.GroupEdge{
				ID:            edgeID,
				ParentGroupID: cmd.ID,
				ChildGroupID:  in.ChildGroupID,
				NodeLogicalID: in.NodeLogicalID,
				Position:      i,
			}
		}
		edgeMap[cmd.ID] = newEdges
	} else {
		newEdges = edgeMap[cmd.ID]
	}

	if err := ValidatePolicyGraph(existingGroups, edgeMap, nil, nil); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("validation failed: %v", err))
		return nil, err
	}
	if cmd.Edges != nil {
		if err := s.validateNodeTargets(ctx, newEdges); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("node target validation failed: %v", err))
			return nil, err
		}
	}

	group.UpdatedAt = domain.NowUTC()
	if err := s.policyRepo.UpdateGroup(ctx, group); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("failed to update group: %v", err))
		return nil, err
	}

	if cmd.Edges != nil {
		if err := s.policyRepo.SetEdgesForGroup(ctx, cmd.ID, newEdges); err != nil {
			s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("failed to update edges: %v", err))
			return nil, err
		}
	}

	if cmd.ClearNodeFilter {
		group.NodeFilter = nil
		if s.nodeFilterRepo != nil {
			_ = s.nodeFilterRepo.DeleteGroupFilter(ctx, cmd.ID)
		}
	} else if cmd.NodeFilter != nil {
		if cmd.NodeFilter.IsEmpty() {
			group.NodeFilter = nil
			if s.nodeFilterRepo != nil {
				_ = s.nodeFilterRepo.DeleteGroupFilter(ctx, cmd.ID)
			}
		} else {
			if err := cmd.NodeFilter.Validate(); err != nil {
				s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("invalid node filter: %v", err))
				return nil, err
			}
			group.NodeFilter = cmd.NodeFilter
			if s.nodeFilterRepo != nil {
				err := s.nodeFilterRepo.SetGroupFilter(ctx, &domain.GroupNodeFilter{
					GroupID:   group.ID,
					Spec:      *cmd.NodeFilter,
					UpdatedAt: group.UpdatedAt,
				})
				if err != nil {
					s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultFailure, fmt.Sprintf("failed to update group filter: %v", err))
					return nil, err
				}
			}
		}
	} else if s.nodeFilterRepo != nil {
		gf, err := s.nodeFilterRepo.GetGroupFilter(ctx, group.ID)
		if err == nil && gf != nil {
			group.NodeFilter = &gf.Spec
		}
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "policy_group.update", domain.AuditResultSuccess, fmt.Sprintf("id=%s", cmd.ID))
	return toGroupView(group, newEdges), nil
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

	if s.nodeFilterRepo != nil {
		_ = s.nodeFilterRepo.DeleteGroupFilter(ctx, cmd.ID)
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

	revID := cmd.RevisionID
	if revID == "" {
		active, err := s.revisionRepo.GetActive(ctx)
		if err != nil {
			return nil, domain.NewValidationError("missing_revision_id", "revision_id is required when no active revision exists")
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

	revID := cmd.RevisionID
	if revID == "" {
		active, err := s.revisionRepo.GetActive(ctx)
		if err != nil {
			return nil, domain.NewValidationError("missing_revision_id", "revision_id is required when no active revision exists")
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

// ValidateGraph checks the complete graph and rules currently persisted in the database.
func (s *Service) ValidateGraph(ctx context.Context) (*ValidationResult, error) {
	groups, err := s.policyRepo.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups: %w", err)
	}

	edgeMap := make(map[string][]domain.GroupEdge, len(groups))
	for _, g := range groups {
		edges, err := s.policyRepo.ListEdgesByGroup(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list edges for group %s: %w", g.ID, err)
		}
		edgeMap[g.ID] = edges
	}

	var activeRevID string
	if active, err := s.revisionRepo.GetActive(ctx); err == nil && active != nil {
		activeRevID = active.ID
	}

	var polRules []domain.PolicyRule
	var admRules []domain.AdmissionRule
	if activeRevID != "" {
		polRules, _ = s.policyRepo.ListPolicyRules(ctx, activeRevID)
		admRules, _ = s.policyRepo.ListAdmissionRules(ctx, activeRevID)
	}

	if err := ValidatePolicyGraph(groups, edgeMap, polRules, admRules); err != nil {
		return &ValidationResult{
			Valid:  false,
			Errors: []string{err.Error()},
		}, err
	}

	return &ValidationResult{
		Valid:  true,
		Errors: nil,
	}, nil
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

	if err := s.revisionRepo.Create(ctx, &rev); err != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "revision.create", domain.AuditResultFailure, fmt.Sprintf("failed to save revision: %v", err))
		return nil, err
	}

	if state == domain.RevisionStateActive {
		_ = s.revisionRepo.SetActive(ctx, revID)
	}

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
	return &GroupView{
		ID:         group.ID,
		Name:       group.Name,
		GroupType:  group.GroupType,
		Edges:      edgeViews,
		NodeFilter: group.NodeFilter,
		CreatedAt:  group.CreatedAt,
		UpdatedAt:  group.UpdatedAt,
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

type canonicalSnapshot struct {
	Groups []domain.NodeGroup            `json:"groups"`
	Edges  map[string][]domain.GroupEdge `json:"edges"`
}

func computeConfigurationDigest(groups []domain.NodeGroup, edges map[string][]domain.GroupEdge) (string, error) {
	sortedGroups := make([]domain.NodeGroup, len(groups))
	copy(sortedGroups, groups)
	sort.SliceStable(sortedGroups, func(i, j int) bool {
		return sortedGroups[i].ID < sortedGroups[j].ID
	})

	canonicalEdges := make(map[string][]domain.GroupEdge, len(edges))
	for k, list := range edges {
		sortedList := make([]domain.GroupEdge, len(list))
		copy(sortedList, list)
		sort.SliceStable(sortedList, func(i, j int) bool {
			return sortedList[i].Position < sortedList[j].Position
		})
		canonicalEdges[k] = sortedList
	}

	snap := canonicalSnapshot{
		Groups: sortedGroups,
		Edges:  canonicalEdges,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
