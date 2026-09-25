package resolver

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Resolver defines the single deterministic policy resolution contract.
type Resolver interface {
	Resolve(ctx context.Context, input ResolveInput) (*ResolvedPolicySnapshot, error)
}

type defaultResolver struct{}

// New constructs a new deterministic Resolver instance.
func New() Resolver {
	return &defaultResolver{}
}

// Resolve executes deterministic policy graph resolution and produces a ResolvedPolicySnapshot with a stable digest.
func (r *defaultResolver) Resolve(ctx context.Context, input ResolveInput) (*ResolvedPolicySnapshot, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	compilerVer := input.CompilerVersion
	if compilerVer == "" {
		compilerVer = "1.0.0"
	}
	input.CompilerVersion = compilerVer

	// 1. Validate topology, self-loops, cycles, and rule references
	if err := validateTopology(input.Groups, input.Edges, input.PolicyRules); err != nil {
		return nil, err
	}

	// 2. Evaluate node admission, explicit exclusions, and risk policy decisions
	riskExcludedIDs, riskDiagnostics := evaluateRiskAdmission(input.Nodes, input.RiskPolicyRevision, input.RiskDecisions, input.RiskReviewAction)
	excludedNodeIDs := append(append([]string(nil), input.ExcludedNodeIDs...), riskExcludedIDs...)
	admissionRes := evaluateNodeAdmission(input.Nodes, input.AdmissionRules, excludedNodeIDs)

	// 3. Build sorted admitted nodes (sorted by DisplayName ASC, then LogicalID ASC)
	sortedAdmittedNodes := make([]ResolvedNode, len(admissionRes.AdmittedNodes))
	for i, n := range admissionRes.AdmittedNodes {
		sortedAdmittedNodes[i] = ResolvedNode{
			LogicalID:   n.LogicalID,
			DisplayName: n.DisplayName,
			Protocol:    n.Protocol,
			Active:      n.Active,
			Position:    i,
		}
	}
	sort.SliceStable(sortedAdmittedNodes, func(i, j int) bool {
		if sortedAdmittedNodes[i].DisplayName == sortedAdmittedNodes[j].DisplayName {
			return sortedAdmittedNodes[i].LogicalID < sortedAdmittedNodes[j].LogicalID
		}
		return sortedAdmittedNodes[i].DisplayName < sortedAdmittedNodes[j].DisplayName
	})
	for i := range sortedAdmittedNodes {
		sortedAdmittedNodes[i].Position = i
	}

	admittedLogicalIDs := make([]string, len(sortedAdmittedNodes))
	for i, n := range sortedAdmittedNodes {
		admittedLogicalIDs[i] = n.LogicalID
	}

	// 4. Resolve groups, ordered members, and recursive node expansion
	resolvedGroups, groupDiagnostics := resolveGroups(input.Groups, input.Edges, admissionRes.AdmittedNodeMap)

	// 5. Resolve rules and normalize terminal MATCH/FINAL rules
	resolvedRules := resolveRules(input.PolicyRules, input.Groups)

	// 6. Assemble diagnostics
	allDiagnostics := make([]Diagnostic, 0, len(admissionRes.Diagnostics)+len(groupDiagnostics)+len(riskDiagnostics))
	allDiagnostics = append(allDiagnostics, admissionRes.Diagnostics...)
	allDiagnostics = append(allDiagnostics, riskDiagnostics...)
	allDiagnostics = append(allDiagnostics, groupDiagnostics...)

	// Sort diagnostics deterministically by Code ASC, Target ASC, Message ASC
	sort.SliceStable(allDiagnostics, func(i, j int) bool {
		if allDiagnostics[i].Code != allDiagnostics[j].Code {
			return allDiagnostics[i].Code < allDiagnostics[j].Code
		}
		if allDiagnostics[i].Target != allDiagnostics[j].Target {
			return allDiagnostics[i].Target < allDiagnostics[j].Target
		}
		return allDiagnostics[i].Message < allDiagnostics[j].Message
	})

	// 7. Compute InputDigest
	inputDigest, err := computeInputDigest(input)
	if err != nil {
		return nil, fmt.Errorf("failed to compute input digest: %w", err)
	}

	// 8. Build initial snapshot object (without volatile timestamps in digest)
	snap := &ResolvedPolicySnapshot{
		InputDigest:        inputDigest,
		RevisionID:         input.RevisionID,
		InventoryWatermark: input.InventoryWatermark,
		CompilerVersion:    input.CompilerVersion,
		Nodes:              sortedAdmittedNodes,
		NodeLogicalIDs:     admittedLogicalIDs,
		Groups:             resolvedGroups,
		Rules:              resolvedRules,
		DNS:                input.DNS,
		Diagnostics:        allDiagnostics,
		RiskPolicyRevision: input.RiskPolicyRevision,
		RiskDecisionDigest: input.RiskDecisionDigest,
		RiskEvaluatedAt:    input.RiskEvaluatedAt,
		AdmittedNodeIDs:    admissionRes.AdmittedNodeIDs,
		ExcludedNodeIDs:    admissionRes.ExcludedNodeIDs,
		ResolvedAt:         time.Now().UTC(),
	}

	// 9. Compute stable SnapshotDigest
	snapDigest, err := computeSnapshotDigest(snap)
	if err != nil {
		return nil, fmt.Errorf("failed to compute snapshot digest: %w", err)
	}
	snap.SnapshotDigest = snapDigest

	return snap, nil
}
