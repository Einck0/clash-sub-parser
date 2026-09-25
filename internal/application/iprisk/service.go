// Package iprisk provides multi-source risk decision evaluation and deterministic digest calculation.
package iprisk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
)

// EvaluateNodeInput describes input for evaluating risk on a single node.
type EvaluateNodeInput struct {
	NodeLogicalID      string
	ExitIdentityDigest string
	Policy             *domain.RiskPolicy
	PolicyRevisionID   string
	EvaluatedAt        time.Time
	Observations       []domain.IPRiskObservation
}

// NodeEvaluation is the result of evaluating a single node against a risk policy.
type NodeEvaluation struct {
	Decision domain.RiskDecision
	Summary  domain.IPRiskSummary
	RiskBand domain.RiskBand
	Action   domain.RiskAction
}

// EvaluateBatchInput describes input for batch risk evaluation over multiple nodes.
type EvaluateBatchInput struct {
	NodeLogicalIDs      []string
	ExitIdentityDigests map[string]string
	Policy              *domain.RiskPolicy
	PolicyRevisionID    string
	InventoryWatermark  string
	EvaluatedAt         time.Time
	Observations        []domain.IPRiskObservation
}

// RiskDiagnostic records a diagnosis for an admitted, reviewed, or blocked node.
type RiskDiagnostic struct {
	NodeLogicalID string            `json:"node_logical_id"`
	Decision      domain.RiskAction `json:"decision"`
	RiskBand      domain.RiskBand   `json:"risk_band"`
	ReasonCode    string            `json:"reason_code"`
	Message       string            `json:"message"`
}

// BatchEvaluationResult holds sorted, deterministic evaluation outputs and stable decision digest.
type BatchEvaluationResult struct {
	PolicyRevisionID   string
	InventoryWatermark string
	EvaluatedAt        time.Time
	DecisionDigest     string
	Decisions          []domain.RiskDecision
	DecisionMap        map[string]domain.RiskDecision
	Summaries          []domain.IPRiskSummary
	SummaryMap         map[string]domain.IPRiskSummary
	AdmittedNodeIDs    []string
	ExcludedNodeIDs    []string
	Diagnostics        []RiskDiagnostic
}

// Action carries audit attribution context for an operation.
type Action struct {
	RequestID string
	ActorKind domain.ActorKind
}

// CreateRiskPolicyCommand defines input for creating a risk policy revision.
type CreateRiskPolicyCommand struct {
	RevisionID        string                       `json:"revision_id,omitempty"`
	ProviderSelection domain.RiskProviderSelection `json:"provider_selection"`
	MaxObservationAge time.Duration                `json:"max_observation_age"`
	MinimumConfidence int                          `json:"minimum_confidence"`
	ScoreBands        []domain.ScoreBand           `json:"score_bands"`
	TraitRules        []domain.TraitRule           `json:"trait_rules,omitempty"`
	UnknownAction     domain.RiskAction            `json:"unknown_action,omitempty"`
	ConflictAction    domain.RiskAction            `json:"conflict_action,omitempty"`
	ReviewAction      domain.RiskAction            `json:"review_action,omitempty"`
	RequestID         string                       `json:"request_id,omitempty"`
	ActorKind         domain.ActorKind             `json:"actor_kind,omitempty"`
}

// BindGroupRiskPolicyCommand defines input for associating a policy group with a risk policy revision.
type BindGroupRiskPolicyCommand struct {
	GroupID          string           `json:"group_id"`
	PolicyRevisionID string           `json:"policy_revision_id"`
	RequestID        string           `json:"request_id,omitempty"`
	ActorKind        domain.ActorKind `json:"actor_kind,omitempty"`
}

// RiskPolicyReviewDetail encapsulates review verification details of a risk policy revision.
type RiskPolicyReviewDetail struct {
	Revision        domain.RiskPolicyRevision `json:"revision"`
	Valid           bool                      `json:"valid"`
	ScoreBandsCount int                       `json:"score_bands_count"`
	TraitRulesCount int                       `json:"trait_rules_count"`
	ProvidersCount  int                       `json:"providers_count"`
	ReviewedAt      time.Time                 `json:"reviewed_at"`
}

// GroupRiskEvaluationResult represents evaluation of a group's members under its bound risk policy.
type GroupRiskEvaluationResult struct {
	GroupID          string           `json:"group_id"`
	PolicyRevisionID string           `json:"policy_revision_id,omitempty"`
	PolicyActive     bool             `json:"policy_active"`
	DecisionDigest   string           `json:"decision_digest,omitempty"`
	AdmittedMembers  []string         `json:"admitted_members"`
	ExcludedMembers  []string         `json:"excluded_members"`
	Diagnostics      []RiskDiagnostic `json:"diagnostics,omitempty"`
}

// Service orchestrates multi-provider risk decisions and deterministic digests.
type Service struct {
	obsRepo     domain.IPRiskObservationRepository
	policyRepo  domain.RiskPolicyRevisionRepository
	bindingRepo domain.RiskPolicyGroupBindingRepository
	groupRepo   domain.PolicyRepository
	nodeRepo    domain.NodeRepository
	auditRepo   domain.AuditRepository
	mu          sync.Mutex
}

// Option configures optional Service dependencies.
type Option func(*Service)

// WithBindingRepository injects a group binding repository.
func WithBindingRepository(repo domain.RiskPolicyGroupBindingRepository) Option {
	return func(s *Service) {
		s.bindingRepo = repo
	}
}

// WithGroupRepository injects a policy repository for group and edge queries.
func WithGroupRepository(repo domain.PolicyRepository) Option {
	return func(s *Service) {
		s.groupRepo = repo
	}
}

// WithNodeRepository injects a node repository for inventory lookups.
func WithNodeRepository(repo domain.NodeRepository) Option {
	return func(s *Service) {
		s.nodeRepo = repo
	}
}

// WithAuditRepository injects an audit repository for recording administrative operations.
func WithAuditRepository(repo domain.AuditRepository) Option {
	return func(s *Service) {
		s.auditRepo = repo
	}
}

// NewService constructs a risk decision application service.
func NewService(
	obsRepo domain.IPRiskObservationRepository,
	policyRepo domain.RiskPolicyRevisionRepository,
	opts ...Option,
) *Service {
	s := &Service{
		obsRepo:    obsRepo,
		policyRepo: policyRepo,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// EvaluateNode evaluates a single node against the specified or resolved policy.
func (s *Service) EvaluateNode(ctx context.Context, input EvaluateNodeInput) (*NodeEvaluation, error) {
	if !domain.IsValidLogicalID(input.NodeLogicalID) {
		return nil, domain.NewValidationError("invalid_node_logical_id", "node logical ID is invalid")
	}

	policy, err := s.resolvePolicy(ctx, input.Policy, input.PolicyRevisionID)
	if err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}

	evaluatedAt := input.EvaluatedAt
	if evaluatedAt.IsZero() {
		evaluatedAt = domain.NowUTC()
	}

	observations := input.Observations
	if len(observations) == 0 && s.obsRepo != nil {
		fetched, _, err := s.obsRepo.List(ctx, domain.IPRiskObservationFilter{
			NodeLogicalID: input.NodeLogicalID,
			Page:          1,
			PageSize:      100,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query risk observations for node %s: %w", input.NodeLogicalID, err)
		}
		observations = fetched
	}

	return evaluateNodeCore(*policy, input.NodeLogicalID, input.ExitIdentityDigest, observations, evaluatedAt)
}

// EvaluateBatch evaluates a set of nodes and produces deterministic decisions, summaries, and digest.
func (s *Service) EvaluateBatch(ctx context.Context, input EvaluateBatchInput) (*BatchEvaluationResult, error) {
	policy, err := s.resolvePolicy(ctx, input.Policy, input.PolicyRevisionID)
	if err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}

	evaluatedAt := input.EvaluatedAt
	if evaluatedAt.IsZero() {
		evaluatedAt = domain.NowUTC()
	}

	// Index pre-provided observations by node logical ID
	obsByNode := make(map[string][]domain.IPRiskObservation)
	for _, obs := range input.Observations {
		obsByNode[obs.NodeLogicalID] = append(obsByNode[obs.NodeLogicalID], obs)
	}

	// Canonicalize and deduplicate node logical IDs
	seenNodes := make(map[string]struct{}, len(input.NodeLogicalIDs))
	uniqueNodeIDs := make([]string, 0, len(input.NodeLogicalIDs))
	for _, nodeID := range input.NodeLogicalIDs {
		if !domain.IsValidLogicalID(nodeID) {
			return nil, domain.NewValidationError("invalid_node_logical_id", fmt.Sprintf("invalid node logical ID: %s", nodeID))
		}
		if _, exists := seenNodes[nodeID]; !exists {
			seenNodes[nodeID] = struct{}{}
			uniqueNodeIDs = append(uniqueNodeIDs, nodeID)
		}
	}
	sort.Strings(uniqueNodeIDs)

	result := &BatchEvaluationResult{
		PolicyRevisionID:   policy.RevisionID,
		InventoryWatermark: input.InventoryWatermark,
		EvaluatedAt:        evaluatedAt,
		Decisions:          make([]domain.RiskDecision, 0, len(uniqueNodeIDs)),
		DecisionMap:        make(map[string]domain.RiskDecision, len(uniqueNodeIDs)),
		Summaries:          make([]domain.IPRiskSummary, 0, len(uniqueNodeIDs)),
		SummaryMap:         make(map[string]domain.IPRiskSummary, len(uniqueNodeIDs)),
		AdmittedNodeIDs:    make([]string, 0),
		ExcludedNodeIDs:    make([]string, 0),
		Diagnostics:        make([]RiskDiagnostic, 0),
	}

	for _, nodeID := range uniqueNodeIDs {
		nodeObs := obsByNode[nodeID]
		if len(nodeObs) == 0 && s.obsRepo != nil {
			fetched, _, fetchErr := s.obsRepo.List(ctx, domain.IPRiskObservationFilter{
				NodeLogicalID: nodeID,
				Page:          1,
				PageSize:      100,
			})
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to fetch observations for node %s: %w", nodeID, fetchErr)
			}
			nodeObs = fetched
		}

		expectedExitDigest := ""
		if input.ExitIdentityDigests != nil {
			expectedExitDigest = input.ExitIdentityDigests[nodeID]
		}

		eval, evalErr := evaluateNodeCore(*policy, nodeID, expectedExitDigest, nodeObs, evaluatedAt)
		if evalErr != nil {
			return nil, fmt.Errorf("failed to evaluate node %s: %w", nodeID, evalErr)
		}

		result.Decisions = append(result.Decisions, eval.Decision)
		result.DecisionMap[nodeID] = eval.Decision
		result.Summaries = append(result.Summaries, eval.Summary)
		result.SummaryMap[nodeID] = eval.Summary

		// Group admission classification
		switch eval.Decision.Decision {
		case domain.RiskActionAllow:
			result.AdmittedNodeIDs = append(result.AdmittedNodeIDs, nodeID)
		case domain.RiskActionBlock:
			result.ExcludedNodeIDs = append(result.ExcludedNodeIDs, nodeID)
			result.Diagnostics = append(result.Diagnostics, RiskDiagnostic{
				NodeLogicalID: nodeID,
				Decision:      eval.Decision.Decision,
				RiskBand:      eval.Summary.RiskBand,
				ReasonCode:    eval.Decision.ReasonCode,
				Message:       fmt.Sprintf("node %s blocked by risk policy: %s", nodeID, eval.Decision.ReasonCode),
			})
		case domain.RiskActionReview:
			if policy.EffectiveReviewAction() == domain.RiskActionAllow {
				result.AdmittedNodeIDs = append(result.AdmittedNodeIDs, nodeID)
			} else {
				result.ExcludedNodeIDs = append(result.ExcludedNodeIDs, nodeID)
			}
			result.Diagnostics = append(result.Diagnostics, RiskDiagnostic{
				NodeLogicalID: nodeID,
				Decision:      eval.Decision.Decision,
				RiskBand:      eval.Summary.RiskBand,
				ReasonCode:    eval.Decision.ReasonCode,
				Message:       fmt.Sprintf("node %s requires review under risk policy: %s", nodeID, eval.Decision.ReasonCode),
			})
		default: // unknown
			result.ExcludedNodeIDs = append(result.ExcludedNodeIDs, nodeID)
			result.Diagnostics = append(result.Diagnostics, RiskDiagnostic{
				NodeLogicalID: nodeID,
				Decision:      eval.Decision.Decision,
				RiskBand:      eval.Summary.RiskBand,
				ReasonCode:    eval.Decision.ReasonCode,
				Message:       fmt.Sprintf("node %s unknown risk state: %s", nodeID, eval.Decision.ReasonCode),
			})
		}
	}

	sort.Strings(result.AdmittedNodeIDs)
	sort.Strings(result.ExcludedNodeIDs)

	digest, err := ComputeDecisionDigest(policy.RevisionID, input.InventoryWatermark, evaluatedAt, result.Decisions)
	if err != nil {
		return nil, fmt.Errorf("failed to compute decision digest: %w", err)
	}
	result.DecisionDigest = digest

	return result, nil
}

func (s *Service) resolvePolicy(ctx context.Context, explicit *domain.RiskPolicy, revisionID string) (*domain.RiskPolicy, error) {
	if explicit != nil {
		return explicit, nil
	}
	if s.policyRepo == nil {
		return nil, domain.NewValidationError("missing_risk_policy", "risk policy is required")
	}
	if revisionID != "" {
		rev, err := s.policyRepo.GetByID(ctx, revisionID)
		if err != nil {
			return nil, err
		}
		return &rev.RiskPolicy, nil
	}
	active, err := s.policyRepo.GetActive(ctx)
	if err != nil {
		return nil, err
	}
	return &active.RiskPolicy, nil
}

// ComputeDecisionDigest calculates a deterministic, stable SHA-256 digest for a set of decisions.
func ComputeDecisionDigest(
	policyRevisionID string,
	watermark string,
	evaluatedAt time.Time,
	decisions []domain.RiskDecision,
) (string, error) {
	// Canonicalize: sort decisions by NodeLogicalID ascending
	orderedDecisions := make([]domain.RiskDecision, len(decisions))
	copy(orderedDecisions, decisions)
	sort.SliceStable(orderedDecisions, func(i, j int) bool {
		return orderedDecisions[i].NodeLogicalID < orderedDecisions[j].NodeLogicalID
	})

	canonicalItems := make([]canonicalDecisionItem, len(orderedDecisions))
	for i, d := range orderedDecisions {
		digests := append([]string(nil), d.ObservationDigestSet...)
		sort.Strings(digests)
		// Deduplicate digests
		uniqueDigests := make([]string, 0, len(digests))
		for j, dig := range digests {
			if j == 0 || dig != digests[j-1] {
				uniqueDigests = append(uniqueDigests, dig)
			}
		}
		canonicalItems[i] = canonicalDecisionItem{
			NodeLogicalID:        d.NodeLogicalID,
			Decision:             string(d.Decision),
			ReasonCode:           d.ReasonCode,
			ObservationDigestSet: uniqueDigests,
		}
	}

	payload := canonicalDecisionDigestPayload{
		PolicyRevisionID:   policyRevisionID,
		InventoryWatermark: watermark,
		EvaluatedAt:        evaluatedAt.UTC().Format(time.RFC3339Nano),
		Decisions:          canonicalItems,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal decision digest payload: %w", err)
	}

	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}

type canonicalDecisionItem struct {
	NodeLogicalID        string   `json:"node_logical_id"`
	Decision             string   `json:"decision"`
	ReasonCode           string   `json:"reason_code"`
	ObservationDigestSet []string `json:"observation_digest_set"`
}

type canonicalDecisionDigestPayload struct {
	PolicyRevisionID   string                  `json:"policy_revision_id"`
	InventoryWatermark string                  `json:"inventory_watermark,omitempty"`
	EvaluatedAt        string                  `json:"evaluated_at"`
	Decisions          []canonicalDecisionItem `json:"decisions"`
}

type providerEvalResult struct {
	Ref            domain.ProviderRef
	Action         domain.RiskAction
	RiskBand       domain.RiskBand
	ReasonCode     string
	ObservedAt     time.Time
	ExpiresAt      time.Time
	Status         domain.IPRiskStatus
	EvidenceDigest string
	IsAvailable    bool
}

func evaluateNodeCore(
	policy domain.RiskPolicy,
	nodeLogicalID string,
	expectedExitDigest string,
	observations []domain.IPRiskObservation,
	evaluatedAt time.Time,
) (*NodeEvaluation, error) {
	// Group observations by provider key (provider + \x00 + schemaVersion)
	// and keep only the latest one per provider
	latestByProvider := make(map[string]domain.IPRiskObservation)
	for _, obs := range observations {
		if obs.NodeLogicalID != nodeLogicalID {
			continue
		}
		key := obs.Provider + "\x00" + obs.ProviderSchemaVersion
		existing, exists := latestByProvider[key]
		if !exists || obs.ObservedAt.After(existing.ObservedAt) || (obs.ObservedAt.Equal(existing.ObservedAt) && obs.ID > existing.ID) {
			latestByProvider[key] = obs
		}
	}

	// Evaluate each declared provider
	providerResults := make([]providerEvalResult, len(policy.ProviderSelection.Providers))
	for i, ref := range policy.ProviderSelection.Providers {
		key := ref.Provider + "\x00" + ref.SchemaVersion
		var obsPtr *domain.IPRiskObservation
		if obs, ok := latestByProvider[key]; ok {
			obsCopy := obs
			obsPtr = &obsCopy
		}
		providerResults[i] = evaluateSingleProviderObservation(policy, ref, obsPtr, expectedExitDigest, evaluatedAt)
	}

	var finalAction domain.RiskAction
	var finalBand domain.RiskBand
	var finalReason string
	var chosenResult providerEvalResult
	var digestSet []string

	// Collect unique valid digests
	digestMap := make(map[string]struct{})
	for _, r := range providerResults {
		if r.EvidenceDigest != "" {
			digestMap[r.EvidenceDigest] = struct{}{}
		}
	}
	digestSet = make([]string, 0, len(digestMap))
	for dig := range digestMap {
		digestSet = append(digestSet, dig)
	}
	sort.Strings(digestSet)

	switch policy.ProviderSelection.Mode {
	case domain.RiskFusionSingleProvider:
		res := providerResults[0]
		chosenResult = res
		if res.Action == domain.RiskActionUnknown || !res.IsAvailable {
			finalAction = policy.EffectiveUnknownAction()
			finalBand = domain.RiskBandUnknown
			finalReason = res.ReasonCode
		} else {
			finalAction = res.Action
			finalBand = res.RiskBand
			finalReason = res.ReasonCode
		}

	case domain.RiskFusionAllMustAllow:
		// 1. Any provider blocked -> block
		hasBlock := false
		var blockingRes providerEvalResult
		for _, r := range providerResults {
			if r.Action == domain.RiskActionBlock {
				hasBlock = true
				blockingRes = r
				break
			}
		}
		if hasBlock {
			finalAction = domain.RiskActionBlock
			finalBand = maxBandAcross(providerResults, domain.RiskBandCritical)
			finalReason = "all_must_allow_blocked"
			chosenResult = blockingRes
			break
		}

		// 2. Any provider missing / unknown / unavailable -> unknown_action (default review)
		hasUnknown := false
		var unknownRes providerEvalResult
		for _, r := range providerResults {
			if r.Action == domain.RiskActionUnknown || !r.IsAvailable {
				hasUnknown = true
				unknownRes = r
				break
			}
		}
		if hasUnknown {
			finalAction = policy.EffectiveUnknownAction()
			finalBand = domain.RiskBandUnknown
			finalReason = "all_must_allow_" + unknownRes.ReasonCode
			chosenResult = unknownRes
			break
		}

		// 3. Any provider review -> review
		hasReview := false
		var reviewRes providerEvalResult
		for _, r := range providerResults {
			if r.Action == domain.RiskActionReview {
				hasReview = true
				reviewRes = r
				break
			}
		}
		if hasReview {
			finalAction = domain.RiskActionReview
			finalBand = maxBandAcross(providerResults, domain.RiskBandMedium)
			finalReason = "all_must_allow_review"
			chosenResult = reviewRes
			break
		}

		// 4. All providers allowed -> allow
		finalAction = domain.RiskActionAllow
		finalBand = maxBandAcross(providerResults, domain.RiskBandLow)
		finalReason = "all_must_allow_passed"
		chosenResult = providerResults[0]

	case domain.RiskFusionHighestRisk:
		// 1. Any provider blocked -> block
		hasBlock := false
		var blockingRes providerEvalResult
		for _, r := range providerResults {
			if r.Action == domain.RiskActionBlock {
				hasBlock = true
				if blockingRes.Ref.Provider == "" || bandRank(r.RiskBand) > bandRank(blockingRes.RiskBand) {
					blockingRes = r
				}
			}
		}
		if hasBlock {
			finalAction = domain.RiskActionBlock
			finalBand = maxBandAcross(providerResults, domain.RiskBandCritical)
			finalReason = "highest_risk_blocked"
			chosenResult = blockingRes
			break
		}

		// 2. Incomplete or missing data cannot become low risk
		hasUnknown := false
		var unknownRes providerEvalResult
		for _, r := range providerResults {
			if r.Action == domain.RiskActionUnknown || !r.IsAvailable {
				hasUnknown = true
				unknownRes = r
				break
			}
		}
		if hasUnknown {
			// If another provider gave review, we can flag review; otherwise apply unknown action
			hasReview := false
			var reviewRes providerEvalResult
			for _, r := range providerResults {
				if r.Action == domain.RiskActionReview {
					hasReview = true
					reviewRes = r
					break
				}
			}
			if hasReview {
				finalAction = domain.RiskActionReview
				finalBand = domain.RiskBandMedium
				finalReason = "highest_risk_review"
				chosenResult = reviewRes
			} else {
				finalAction = policy.EffectiveUnknownAction()
				finalBand = domain.RiskBandUnknown
				finalReason = "highest_risk_" + unknownRes.ReasonCode
				chosenResult = unknownRes
			}
			break
		}

		// 3. All providers available -> pick provider with highest risk band
		best := providerResults[0]
		for _, r := range providerResults[1:] {
			if bandRank(r.RiskBand) > bandRank(best.RiskBand) {
				best = r
			} else if bandRank(r.RiskBand) == bandRank(best.RiskBand) && actionRank(r.Action) > actionRank(best.Action) {
				best = r
			}
		}
		finalAction = best.Action
		finalBand = best.RiskBand
		finalReason = fmt.Sprintf("highest_risk_%s", best.Action)
		chosenResult = best

	default:
		return nil, domain.NewValidationError("invalid_risk_fusion_mode", fmt.Sprintf("unsupported risk fusion mode: %s", policy.ProviderSelection.Mode))
	}

	// Normalize reason code for domain validation compliance
	finalReason = normalizeReasonCode(finalReason)

	// Construct and validate RiskDecision
	decision := domain.RiskDecision{
		NodeLogicalID:        nodeLogicalID,
		PolicyRevisionID:     policy.RevisionID,
		Decision:             finalAction,
		ReasonCode:           finalReason,
		ObservationDigestSet: digestSet,
		EvaluatedAt:          evaluatedAt,
	}
	if err := decision.Validate(); err != nil {
		return nil, fmt.Errorf("constructed invalid RiskDecision: %w", err)
	}

	// Construct and validate IPRiskSummary
	policyRevID := policy.RevisionID
	summary := domain.IPRiskSummary{
		Decision:              finalAction,
		RiskBand:              finalBand,
		Provider:              chosenResult.Ref.Provider,
		ProviderSchemaVersion: chosenResult.Ref.SchemaVersion,
		ObservedAt:            chosenResult.ObservedAt,
		ExpiresAt:             chosenResult.ExpiresAt,
		Status:                chosenResult.Status,
		ReasonCode:            finalReason,
		PolicyRevisionID:      &policyRevID,
	}
	if err := summary.Validate(); err != nil {
		return nil, fmt.Errorf("constructed invalid IPRiskSummary: %w", err)
	}

	return &NodeEvaluation{
		Decision: decision,
		Summary:  summary,
		RiskBand: finalBand,
		Action:   finalAction,
	}, nil
}

func evaluateSingleProviderObservation(
	policy domain.RiskPolicy,
	ref domain.ProviderRef,
	obs *domain.IPRiskObservation,
	expectedExitDigest string,
	evaluatedAt time.Time,
) providerEvalResult {
	defaultTTL := policy.MaxObservationAge
	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour
	}

	if obs == nil {
		return providerEvalResult{
			Ref:            ref,
			Action:         domain.RiskActionUnknown,
			RiskBand:       domain.RiskBandUnknown,
			ReasonCode:     "missing_observation",
			ObservedAt:     evaluatedAt,
			ExpiresAt:      evaluatedAt.Add(defaultTTL),
			Status:         domain.IPRiskStatusUnknown,
			EvidenceDigest: "",
			IsAvailable:    false,
		}
	}

	// 1. Status availability check
	if obs.Status != domain.IPRiskStatusAvailable {
		reason := "observation_status_" + strings.ToLower(string(obs.Status))
		return providerEvalResult{
			Ref:            ref,
			Action:         domain.RiskActionUnknown,
			RiskBand:       domain.RiskBandUnknown,
			ReasonCode:     reason,
			ObservedAt:     obs.ObservedAt,
			ExpiresAt:      obs.ExpiresAt,
			Status:         obs.Status,
			EvidenceDigest: "",
			IsAvailable:    false,
		}
	}

	// 2. Exit identity proof check
	if expectedExitDigest != "" && obs.ExitIdentityDigest != expectedExitDigest {
		return providerEvalResult{
			Ref:            ref,
			Action:         domain.RiskActionUnknown,
			RiskBand:       domain.RiskBandUnknown,
			ReasonCode:     "exit_identity_mismatch",
			ObservedAt:     obs.ObservedAt,
			ExpiresAt:      obs.ExpiresAt,
			Status:         domain.IPRiskStatusUnknown,
			EvidenceDigest: "",
			IsAvailable:    false,
		}
	}

	// 3. TTL Expiry check
	if !evaluatedAt.Before(obs.ExpiresAt) {
		return providerEvalResult{
			Ref:            ref,
			Action:         domain.RiskActionUnknown,
			RiskBand:       domain.RiskBandUnknown,
			ReasonCode:     "observation_expired",
			ObservedAt:     obs.ObservedAt,
			ExpiresAt:      obs.ExpiresAt,
			Status:         domain.IPRiskStatusStale,
			EvidenceDigest: "",
			IsAvailable:    false,
		}
	}

	// 4. Maximum observation age check
	if policy.MaxObservationAge > 0 && evaluatedAt.Sub(obs.ObservedAt) > policy.MaxObservationAge {
		return providerEvalResult{
			Ref:            ref,
			Action:         domain.RiskActionUnknown,
			RiskBand:       domain.RiskBandUnknown,
			ReasonCode:     "observation_exceeded_max_age",
			ObservedAt:     obs.ObservedAt,
			ExpiresAt:      obs.ExpiresAt,
			Status:         domain.IPRiskStatusStale,
			EvidenceDigest: "",
			IsAvailable:    false,
		}
	}

	// 5. Confidence check
	if policy.MinimumConfidence > 0 {
		if obs.Confidence == nil {
			return providerEvalResult{
				Ref:            ref,
				Action:         domain.RiskActionUnknown,
				RiskBand:       domain.RiskBandUnknown,
				ReasonCode:     "missing_confidence",
				ObservedAt:     obs.ObservedAt,
				ExpiresAt:      obs.ExpiresAt,
				Status:         obs.Status,
				EvidenceDigest: obs.EvidenceDigest,
				IsAvailable:    false,
			}
		}
		if *obs.Confidence < policy.MinimumConfidence {
			return providerEvalResult{
				Ref:            ref,
				Action:         domain.RiskActionUnknown,
				RiskBand:       domain.RiskBandUnknown,
				ReasonCode:     "insufficient_confidence",
				ObservedAt:     obs.ObservedAt,
				ExpiresAt:      obs.ExpiresAt,
				Status:         obs.Status,
				EvidenceDigest: obs.EvidenceDigest,
				IsAvailable:    false,
			}
		}
	}

	// 6. Trait rules evaluation
	var traitAction domain.RiskAction
	var traitReason string
	for _, trait := range obs.AnonymizerTraits {
		for _, rule := range policy.TraitRules {
			if rule.Trait == trait {
				if actionRank(rule.Action) > actionRank(traitAction) {
					traitAction = rule.Action
					traitReason = "trait_rule_" + strings.ToLower(string(trait))
				}
			}
		}
	}

	// 7. Score band evaluation
	if obs.Score == nil {
		if traitAction == domain.RiskActionBlock {
			return providerEvalResult{
				Ref:            ref,
				Action:         domain.RiskActionBlock,
				RiskBand:       domain.RiskBandCritical,
				ReasonCode:     traitReason,
				ObservedAt:     obs.ObservedAt,
				ExpiresAt:      obs.ExpiresAt,
				Status:         obs.Status,
				EvidenceDigest: obs.EvidenceDigest,
				IsAvailable:    true,
			}
		}
		return providerEvalResult{
			Ref:            ref,
			Action:         domain.RiskActionUnknown,
			RiskBand:       domain.RiskBandUnknown,
			ReasonCode:     "missing_score",
			ObservedAt:     obs.ObservedAt,
			ExpiresAt:      obs.ExpiresAt,
			Status:         obs.Status,
			EvidenceDigest: obs.EvidenceDigest,
			IsAvailable:    false,
		}
	}

	var scoreBand domain.RiskBand = domain.RiskBandUnknown
	var scoreAction domain.RiskAction = domain.RiskActionUnknown
	var scoreReason string
	for _, band := range policy.ScoreBands {
		if *obs.Score >= band.Min && *obs.Score <= band.Max {
			scoreBand = band.Band
			scoreAction = band.Action
			scoreReason = "score_band_" + strings.ToLower(string(band.Band))
			break
		}
	}

	// Combine trait action and score band action
	finalAction := scoreAction
	finalBand := scoreBand
	finalReason := scoreReason

	if actionRank(traitAction) > actionRank(scoreAction) {
		finalAction = traitAction
		finalReason = traitReason
		if traitAction == domain.RiskActionBlock {
			finalBand = maxBand(scoreBand, domain.RiskBandHigh)
		} else if traitAction == domain.RiskActionReview {
			finalBand = maxBand(scoreBand, domain.RiskBandMedium)
		}
	}

	return providerEvalResult{
		Ref:            ref,
		Action:         finalAction,
		RiskBand:       finalBand,
		ReasonCode:     finalReason,
		ObservedAt:     obs.ObservedAt,
		ExpiresAt:      obs.ExpiresAt,
		Status:         obs.Status,
		EvidenceDigest: obs.EvidenceDigest,
		IsAvailable:    true,
	}
}

func bandRank(b domain.RiskBand) int {
	switch b {
	case domain.RiskBandCritical:
		return 4
	case domain.RiskBandHigh:
		return 3
	case domain.RiskBandMedium:
		return 2
	case domain.RiskBandLow:
		return 1
	default:
		return 0
	}
}

func actionRank(a domain.RiskAction) int {
	switch a {
	case domain.RiskActionBlock:
		return 3
	case domain.RiskActionReview:
		return 2
	case domain.RiskActionAllow:
		return 1
	default:
		return 0
	}
}

func maxBand(a, b domain.RiskBand) domain.RiskBand {
	if bandRank(a) >= bandRank(b) {
		return a
	}
	return b
}

func maxBandAcross(results []providerEvalResult, fallback domain.RiskBand) domain.RiskBand {
	var highest domain.RiskBand = fallback
	for _, r := range results {
		if r.RiskBand != domain.RiskBandUnknown && bandRank(r.RiskBand) > bandRank(highest) {
			highest = r.RiskBand
		}
	}
	return highest
}

func normalizeReasonCode(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	reason = strings.ReplaceAll(reason, " ", "_")
	reason = strings.ReplaceAll(reason, "-", "_")
	if reason == "" {
		return "risk_decision_evaluated"
	}
	return reason
}

// CreatePolicy creates a new versioned risk policy revision in an initial inactive state.
func (s *Service) CreatePolicy(ctx context.Context, cmd CreateRiskPolicyCommand) (*domain.RiskPolicyRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	action := Action{RequestID: cmd.RequestID, ActorKind: cmd.ActorKind}
	revID := strings.TrimSpace(cmd.RevisionID)
	if revID == "" {
		var err error
		revID, err = domain.NewUUIDv7()
		if err != nil {
			s.recordAudit(ctx, action, "risk_policy.create", domain.AuditResultFailure, "failed to generate UUIDv7")
			return nil, err
		}
	} else if !domain.IsValidUUIDv7(revID) {
		s.recordAudit(ctx, action, "risk_policy.create", domain.AuditResultFailure, "invalid revision UUIDv7")
		return nil, domain.NewValidationError("invalid_risk_policy_revision_id", fmt.Sprintf("invalid risk policy revision UUIDv7: %s", revID))
	}

	policy := domain.RiskPolicy{
		RevisionID:        revID,
		ProviderSelection: cmd.ProviderSelection,
		MaxObservationAge: cmd.MaxObservationAge,
		MinimumConfidence: cmd.MinimumConfidence,
		ScoreBands:        cmd.ScoreBands,
		TraitRules:        cmd.TraitRules,
		UnknownAction:     cmd.UnknownAction,
		ConflictAction:    cmd.ConflictAction,
		ReviewAction:      cmd.ReviewAction,
	}

	if err := policy.Validate(); err != nil {
		s.recordAudit(ctx, action, "risk_policy.create", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	if s.policyRepo == nil {
		return nil, domain.NewInternalError("missing_policy_repository", "policy repository is not configured")
	}

	rev := &domain.RiskPolicyRevision{
		RiskPolicy: policy,
		Active:     false,
		CreatedAt:  domain.NowUTC(),
	}

	if err := s.policyRepo.Create(ctx, rev); err != nil {
		s.recordAudit(ctx, action, "risk_policy.create", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	s.recordAudit(ctx, action, "risk_policy.create", domain.AuditResultSuccess,
		fmt.Sprintf("revision_id=%s mode=%s bands=%d active=false", rev.RevisionID, rev.ProviderSelection.Mode, len(rev.ScoreBands)))
	return rev, nil
}

// ListPolicies lists risk policy revisions with pagination and optional active filter.
func (s *Service) ListPolicies(ctx context.Context, filter domain.RiskPolicyFilter) ([]domain.RiskPolicyRevision, int, error) {
	if filter.Page < 0 || filter.PageSize < 0 {
		return nil, 0, domain.NewValidationError("invalid_pagination", "page and page_size cannot be negative")
	}
	if s.policyRepo == nil {
		return nil, 0, domain.NewInternalError("missing_policy_repository", "policy repository is not configured")
	}
	return s.policyRepo.List(ctx, filter)
}

// GetPolicy retrieves a risk policy revision by its UUIDv7 identifier.
func (s *Service) GetPolicy(ctx context.Context, id string) (*domain.RiskPolicyRevision, error) {
	id = strings.TrimSpace(id)
	if !domain.IsValidUUIDv7(id) {
		return nil, domain.NewValidationError("invalid_risk_policy_revision_id", "risk policy revision ID must be UUIDv7")
	}
	if s.policyRepo == nil {
		return nil, domain.NewInternalError("missing_policy_repository", "policy repository is not configured")
	}
	return s.policyRepo.GetByID(ctx, id)
}

// GetActivePolicy retrieves the currently active risk policy revision.
func (s *Service) GetActivePolicy(ctx context.Context) (*domain.RiskPolicyRevision, error) {
	if s.policyRepo == nil {
		return nil, domain.NewInternalError("missing_policy_repository", "policy repository is not configured")
	}
	return s.policyRepo.GetActive(ctx)
}

// ReviewPolicy inspects and validates a risk policy revision, recording an audited event.
func (s *Service) ReviewPolicy(ctx context.Context, id string, action Action) (*RiskPolicyReviewDetail, error) {
	id = strings.TrimSpace(id)
	if !domain.IsValidUUIDv7(id) {
		s.recordAudit(ctx, action, "risk_policy.review", domain.AuditResultFailure, "invalid revision UUIDv7")
		return nil, domain.NewValidationError("invalid_risk_policy_revision_id", "risk policy revision ID must be UUIDv7")
	}
	if s.policyRepo == nil {
		return nil, domain.NewInternalError("missing_policy_repository", "policy repository is not configured")
	}

	rev, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		s.recordAudit(ctx, action, "risk_policy.review", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	if valErr := rev.RiskPolicy.Validate(); valErr != nil {
		s.recordAudit(ctx, action, "risk_policy.review", domain.AuditResultFailure, valErr.Error())
		return nil, valErr
	}

	detail := &RiskPolicyReviewDetail{
		Revision:        *rev,
		Valid:           true,
		ScoreBandsCount: len(rev.ScoreBands),
		TraitRulesCount: len(rev.TraitRules),
		ProvidersCount:  len(rev.ProviderSelection.Providers),
		ReviewedAt:      domain.NowUTC(),
	}

	s.recordAudit(ctx, action, "risk_policy.review", domain.AuditResultSuccess,
		fmt.Sprintf("revision_id=%s valid=true bands=%d traits=%d providers=%d",
			rev.RevisionID, detail.ScoreBandsCount, detail.TraitRulesCount, detail.ProvidersCount))
	return detail, nil
}

// ActivatePolicy promotes a policy revision to active status and deactivates any previously active revision.
func (s *Service) ActivatePolicy(ctx context.Context, id string, action Action) (*domain.RiskPolicyRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	if !domain.IsValidUUIDv7(id) {
		s.recordAudit(ctx, action, "risk_policy.activate", domain.AuditResultFailure, "invalid revision UUIDv7")
		return nil, domain.NewValidationError("invalid_risk_policy_revision_id", "risk policy revision ID must be UUIDv7")
	}
	if s.policyRepo == nil {
		return nil, domain.NewInternalError("missing_policy_repository", "policy repository is not configured")
	}

	rev, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		s.recordAudit(ctx, action, "risk_policy.activate", domain.AuditResultFailure, err.Error())
		return nil, err
	}
	if err := rev.RiskPolicy.Validate(); err != nil {
		s.recordAudit(ctx, action, "risk_policy.activate", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	if err := s.policyRepo.SetActive(ctx, id, true); err != nil {
		s.recordAudit(ctx, action, "risk_policy.activate", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	updated, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, action, "risk_policy.activate", domain.AuditResultSuccess,
		fmt.Sprintf("revision_id=%s active=true", id))
	return updated, nil
}

// DeactivatePolicy sets a risk policy revision to inactive.
func (s *Service) DeactivatePolicy(ctx context.Context, id string, action Action) (*domain.RiskPolicyRevision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	if !domain.IsValidUUIDv7(id) {
		s.recordAudit(ctx, action, "risk_policy.deactivate", domain.AuditResultFailure, "invalid revision UUIDv7")
		return nil, domain.NewValidationError("invalid_risk_policy_revision_id", "risk policy revision ID must be UUIDv7")
	}
	if s.policyRepo == nil {
		return nil, domain.NewInternalError("missing_policy_repository", "policy repository is not configured")
	}

	if _, err := s.policyRepo.GetByID(ctx, id); err != nil {
		s.recordAudit(ctx, action, "risk_policy.deactivate", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	if err := s.policyRepo.SetActive(ctx, id, false); err != nil {
		s.recordAudit(ctx, action, "risk_policy.deactivate", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	updated, err := s.policyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, action, "risk_policy.deactivate", domain.AuditResultSuccess,
		fmt.Sprintf("revision_id=%s active=false", id))
	return updated, nil
}

// BindGroup binds a policy group to a risk policy revision.
func (s *Service) BindGroup(ctx context.Context, cmd BindGroupRiskPolicyCommand) (*domain.RiskPolicyGroupBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	action := Action{RequestID: cmd.RequestID, ActorKind: cmd.ActorKind}
	groupID := strings.TrimSpace(cmd.GroupID)
	polID := strings.TrimSpace(cmd.PolicyRevisionID)

	if !domain.IsValidUUIDv7(groupID) {
		s.recordAudit(ctx, action, "risk_policy.bind_group", domain.AuditResultFailure, "invalid group UUIDv7")
		return nil, domain.NewValidationError("invalid_group_id", "group ID must be a valid UUIDv7")
	}
	if !domain.IsValidUUIDv7(polID) {
		s.recordAudit(ctx, action, "risk_policy.bind_group", domain.AuditResultFailure, "invalid policy revision UUIDv7")
		return nil, domain.NewValidationError("invalid_policy_revision_id", "policy revision ID must be a valid UUIDv7")
	}

	if s.groupRepo != nil {
		if _, err := s.groupRepo.GetGroupByID(ctx, groupID); err != nil {
			s.recordAudit(ctx, action, "risk_policy.bind_group", domain.AuditResultFailure, err.Error())
			return nil, err
		}
	}
	if s.policyRepo != nil {
		if _, err := s.policyRepo.GetByID(ctx, polID); err != nil {
			s.recordAudit(ctx, action, "risk_policy.bind_group", domain.AuditResultFailure, err.Error())
			return nil, err
		}
	}
	if s.bindingRepo == nil {
		return nil, domain.NewInternalError("missing_binding_repository", "group binding repository is not configured")
	}

	now := domain.NowUTC()
	binding := &domain.RiskPolicyGroupBinding{
		GroupID:          groupID,
		PolicyRevisionID: polID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.bindingRepo.Set(ctx, binding); err != nil {
		s.recordAudit(ctx, action, "risk_policy.bind_group", domain.AuditResultFailure, err.Error())
		return nil, err
	}

	s.recordAudit(ctx, action, "risk_policy.bind_group", domain.AuditResultSuccess,
		fmt.Sprintf("group_id=%s policy_revision_id=%s", groupID, polID))
	return binding, nil
}

// UnbindGroup removes the risk policy binding from a policy group.
func (s *Service) UnbindGroup(ctx context.Context, groupID string, action Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	groupID = strings.TrimSpace(groupID)
	if !domain.IsValidUUIDv7(groupID) {
		s.recordAudit(ctx, action, "risk_policy.unbind_group", domain.AuditResultFailure, "invalid group UUIDv7")
		return domain.NewValidationError("invalid_group_id", "group ID must be a valid UUIDv7")
	}
	if s.bindingRepo == nil {
		return domain.NewInternalError("missing_binding_repository", "group binding repository is not configured")
	}

	if err := s.bindingRepo.Delete(ctx, groupID); err != nil {
		s.recordAudit(ctx, action, "risk_policy.unbind_group", domain.AuditResultFailure, err.Error())
		return err
	}

	s.recordAudit(ctx, action, "risk_policy.unbind_group", domain.AuditResultSuccess,
		fmt.Sprintf("group_id=%s unbound=true", groupID))
	return nil
}

// GetGroupBinding retrieves the risk policy binding for a group.
func (s *Service) GetGroupBinding(ctx context.Context, groupID string) (*domain.RiskPolicyGroupBinding, error) {
	groupID = strings.TrimSpace(groupID)
	if !domain.IsValidUUIDv7(groupID) {
		return nil, domain.NewValidationError("invalid_group_id", "group ID must be a valid UUIDv7")
	}
	if s.bindingRepo == nil {
		return nil, domain.NewInternalError("missing_binding_repository", "group binding repository is not configured")
	}
	return s.bindingRepo.GetByGroupID(ctx, groupID)
}

// ListGroupBindings lists all risk policy group bindings.
func (s *Service) ListGroupBindings(ctx context.Context) ([]domain.RiskPolicyGroupBinding, error) {
	if s.bindingRepo == nil {
		return nil, domain.NewInternalError("missing_binding_repository", "group binding repository is not configured")
	}
	return s.bindingRepo.List(ctx)
}

// EvaluateGroupMembers evaluates a group's members under its bound risk policy.
// If the group is unbound or the bound risk policy is inactive, all members are admitted.
func (s *Service) EvaluateGroupMembers(ctx context.Context, groupID string) (*GroupRiskEvaluationResult, error) {
	groupID = strings.TrimSpace(groupID)
	if !domain.IsValidUUIDv7(groupID) {
		return nil, domain.NewValidationError("invalid_group_id", "group ID must be a valid UUIDv7")
	}

	if s.groupRepo == nil {
		return nil, domain.NewInternalError("missing_group_repository", "group repository is not configured")
	}

	// 1. Verify group exists and retrieve its members
	if _, err := s.groupRepo.GetGroupByID(ctx, groupID); err != nil {
		return nil, err
	}

	edges, err := s.groupRepo.ListEdgesByGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to query group edges: %w", err)
	}

	var memberNodeIDs []string
	seenMembers := make(map[string]struct{})
	for _, edge := range edges {
		if edge.NodeLogicalID != nil && *edge.NodeLogicalID != "" {
			nid := *edge.NodeLogicalID
			if _, exists := seenMembers[nid]; !exists {
				seenMembers[nid] = struct{}{}
				memberNodeIDs = append(memberNodeIDs, nid)
			}
		}
		if edge.ChildGroupID != nil && *edge.ChildGroupID != "" {
			childEdges, cErr := s.groupRepo.ListEdgesByGroup(ctx, *edge.ChildGroupID)
			if cErr == nil {
				for _, ce := range childEdges {
					if ce.NodeLogicalID != nil && *ce.NodeLogicalID != "" {
						nid := *ce.NodeLogicalID
						if _, exists := seenMembers[nid]; !exists {
							seenMembers[nid] = struct{}{}
							memberNodeIDs = append(memberNodeIDs, nid)
						}
					}
				}
			}
		}
	}
	sort.Strings(memberNodeIDs)

	res := &GroupRiskEvaluationResult{
		GroupID:         groupID,
		PolicyActive:    false,
		AdmittedMembers: memberNodeIDs,
		ExcludedMembers: make([]string, 0),
		Diagnostics:     make([]RiskDiagnostic, 0),
	}

	// 2. Check if group has a binding
	if s.bindingRepo == nil {
		return res, nil
	}

	binding, err := s.bindingRepo.GetByGroupID(ctx, groupID)
	if err != nil {
		// Not bound: returns all members admitted
		return res, nil
	}

	res.PolicyRevisionID = binding.PolicyRevisionID

	// 3. Check bound policy revision
	if s.policyRepo == nil {
		return res, nil
	}

	rev, err := s.policyRepo.GetByID(ctx, binding.PolicyRevisionID)
	if err != nil {
		return nil, fmt.Errorf("bound risk policy %s not found: %w", binding.PolicyRevisionID, err)
	}

	// Core invariant: unactivated policy does NOT affect members!
	if !rev.Active {
		res.PolicyActive = false
		return res, nil
	}

	// Policy is active: evaluate members
	res.PolicyActive = true
	batchRes, err := s.EvaluateBatch(ctx, EvaluateBatchInput{
		NodeLogicalIDs:   memberNodeIDs,
		Policy:           &rev.RiskPolicy,
		PolicyRevisionID: rev.RevisionID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate members for group %s: %w", groupID, err)
	}

	res.DecisionDigest = batchRes.DecisionDigest
	res.AdmittedMembers = batchRes.AdmittedNodeIDs
	res.ExcludedMembers = batchRes.ExcludedNodeIDs
	res.Diagnostics = batchRes.Diagnostics

	return res, nil
}

func (s *Service) recordAudit(ctx context.Context, action Action, op string, result domain.AuditResult, summary string) {
	if s.auditRepo == nil {
		return
	}
	actor := action.ActorKind
	if actor == "" {
		actor = domain.ActorKindAdmin
	}
	redacted := domain.RedactSensitiveInfo(summary)
	_ = s.auditRepo.Record(ctx, &domain.AuditEvent{
		ID:              domain.MustNewUUIDv7(),
		ActorKind:       actor,
		RequestID:       action.RequestID,
		Action:          op,
		Result:          result,
		RedactedSummary: redacted,
		CreatedAt:       domain.NowUTC(),
	})
}
