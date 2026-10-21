package resolver

import (
	"context"
	"fmt"
	"sort"
	"time"

	"clash-sub-parser/internal/domain"
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

func dnsStringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		strings, ok := value.([]string)
		if ok {
			return append([]string(nil), strings...)
		}
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

// Resolve executes deterministic policy graph resolution and produces a ResolvedPolicySnapshot with a stable digest.
func (r *defaultResolver) Resolve(ctx context.Context, input ResolveInput) (*ResolvedPolicySnapshot, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if input.DNS.Config != nil {
		config := make(map[string]any, len(input.DNS.Config))
		for key, value := range input.DNS.Config {
			config[key] = value
		}
		config["enable"] = input.DNS.Enabled
		input.DNS.Config = config
		input.DNS.Nameservers = dnsStringList(config["nameserver"])
		input.DNS.Fallback = dnsStringList(config["fallback"])
	} else {
		config := map[string]any{"enable": input.DNS.Enabled}
		if input.DNS.Nameservers != nil {
			config["nameserver"] = input.DNS.Nameservers
		}
		if input.DNS.Fallback != nil {
			config["fallback"] = input.DNS.Fallback
		}
		input.DNS.Config = config
	}

	compilerVer := input.CompilerVersion
	if compilerVer == "" {
		compilerVer = "1.0.0"
	}
	input.CompilerVersion = compilerVer

	// Sources are the sole graph input. Legacy edges/filters never resurrect an empty pipeline.
	input.Groups = append([]domain.NodeGroup(nil), input.Groups...)
	for i := range input.Groups {
		if input.GroupSources != nil {
			input.Groups[i].Sources = input.GroupSources[input.Groups[i].ID]
		}
		input.Groups[i].NodeFilter = nil
	}
	input.Edges = nil
	input.GroupFilters = nil
	// 1. Validate topology, self-loops, cycles, and rule references
	if err := validateTopology(input.Groups, nil, input.PolicyRules); err != nil {
		return nil, err
	}

	// Validate filters if present
	if input.GlobalFilter != nil {
		if err := input.GlobalFilter.Validate(); err != nil {
			return nil, err
		}
	}
	for gid, gf := range input.GroupFilters {
		if err := gf.Validate(); err != nil {
			return nil, fmt.Errorf("group %s filter invalid: %w", gid, err)
		}
	}
	for _, g := range input.Groups {
		if g.NodeFilter != nil {
			if err := g.NodeFilter.Validate(); err != nil {
				return nil, fmt.Errorf("group %s (%s) filter invalid: %w", g.Name, g.ID, err)
			}
		}
	}

	asOf := input.AsOf
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	// 2. Evaluate node admission, explicit exclusions, and risk policy decisions
	riskExcludedIDs, riskDiagnostics := evaluateRiskAdmission(input.Nodes, input.RiskPolicyRevision, input.RiskDecisions, input.RiskReviewAction)
	excludedNodeIDs := append(append([]string(nil), input.ExcludedNodeIDs...), riskExcludedIDs...)
	admissionRes := evaluateNodeAdmission(input.Nodes, input.AdmissionRules, excludedNodeIDs)

	// 3. Two-level filtering: Global Filter evaluation over admitted nodes
	var globalFilterDiagnostics []Diagnostic
	globalAdmittedNodes := make([]domain.Node, 0, len(admissionRes.AdmittedNodes))
	globalAdmittedMap := make(map[string]domain.Node, len(admissionRes.AdmittedNodes))
	globalExcludedIDs := make([]string, 0)

	for _, node := range admissionRes.AdmittedNodes {
		if input.GlobalFilter == nil || input.GlobalFilter.IsEmpty() {
			globalAdmittedNodes = append(globalAdmittedNodes, node)
			globalAdmittedMap[node.LogicalID] = node
			continue
		}

		sources := input.NodeSources[node.LogicalID]
		latestObs := input.LatestObservations[node.LogicalID]
		matched, reason := domain.MatchesFilter(input.GlobalFilter, node, sources, latestObs, asOf)
		if matched {
			globalAdmittedNodes = append(globalAdmittedNodes, node)
			globalAdmittedMap[node.LogicalID] = node
		} else {
			globalExcludedIDs = append(globalExcludedIDs, node.LogicalID)
			globalFilterDiagnostics = append(globalFilterDiagnostics, Diagnostic{
				Severity: DiagnosticSeverityInfo,
				Code:     "global_filter_excluded",
				Message:  fmt.Sprintf("node %q excluded by global filter: %s", node.DisplayName, reason),
				Target:   node.LogicalID,
				Reason:   reason,
			})
		}
	}

	// 4. Resolve groups, ordered members, dynamic groups, and nested group expansion
	resolvedGroups, groupDiagnostics, groupFilterCounts := resolveGroups(
		input.Groups,
		input.Edges,
		globalAdmittedNodes,
		globalAdmittedMap,
		admissionRes.AdmittedNodeMap,
		input.GroupFilters,
		input.GlobalFilter,
		input.NodeSources,
		input.LatestObservations,
		asOf,
		input.PolicyRules,
	)

	// 5. Resolve rules and normalize terminal MATCH/FINAL rules
	resolvedRules := resolveRules(input.PolicyRules, input.Groups)

	// 6. Assemble diagnostics
	allDiagnostics := make([]Diagnostic, 0, len(admissionRes.Diagnostics)+len(riskDiagnostics)+len(globalFilterDiagnostics)+len(groupDiagnostics))
	allDiagnostics = append(allDiagnostics, admissionRes.Diagnostics...)
	allDiagnostics = append(allDiagnostics, riskDiagnostics...)
	allDiagnostics = append(allDiagnostics, globalFilterDiagnostics...)
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

	uniqueGroupNodes := make(map[string]struct{})
	for _, g := range resolvedGroups {
		for _, nid := range g.AllNodeLogicalIDs {
			uniqueGroupNodes[nid] = struct{}{}
		}
	}

	// snapshot.Nodes contains all nodes that passed global filter plus any node
	// referenced by any resolved policy group (ensuring non-inheriting groups like WARP have proxies in YAML).
	includedNodeIDs := make(map[string]bool)
	for _, n := range globalAdmittedNodes {
		includedNodeIDs[n.LogicalID] = true
	}
	for nid := range uniqueGroupNodes {
		includedNodeIDs[nid] = true
	}

	snapshotNodes := make([]ResolvedNode, 0, len(includedNodeIDs))
	for nid := range includedNodeIDs {
		if node, ok := admissionRes.AdmittedNodeMap[nid]; ok {
			snapshotNodes = append(snapshotNodes, ResolvedNode{
				LogicalID:          node.LogicalID,
				DisplayName:        node.DisplayName,
				Protocol:           node.Protocol,
				Server:             node.Server,
				Port:               node.Port,
				Credentials:        node.Credentials,
				Active:             node.Active,
				ConnectionRevision: node.ConnectionRevision,
			})
		}
	}
	sort.SliceStable(snapshotNodes, func(i, j int) bool {
		if snapshotNodes[i].DisplayName == snapshotNodes[j].DisplayName {
			return snapshotNodes[i].LogicalID < snapshotNodes[j].LogicalID
		}
		return snapshotNodes[i].DisplayName < snapshotNodes[j].DisplayName
	})
	snapshotLogicalIDs := make([]string, len(snapshotNodes))
	for i := range snapshotNodes {
		snapshotNodes[i].Position = i
		snapshotLogicalIDs[i] = snapshotNodes[i].LogicalID
	}

	finalAdmittedIDs := make([]string, len(snapshotNodes))
	copy(finalAdmittedIDs, snapshotLogicalIDs)
	sort.Strings(finalAdmittedIDs)

	finalExcludedIDs := make([]string, 0)
	for _, n := range input.Nodes {
		if !includedNodeIDs[n.LogicalID] {
			finalExcludedIDs = append(finalExcludedIDs, n.LogicalID)
		}
	}
	sort.Strings(finalExcludedIDs)

	filterCounts := &FilterLayerCounts{
		RawTotal:            len(input.Nodes),
		AdmittedTotal:       len(admissionRes.AdmittedNodes),
		GlobalFilteredTotal: len(globalAdmittedNodes),
		GroupFilteredTotal:  len(uniqueGroupNodes),
		GroupCounts:         groupFilterCounts,
	}

	// 8. Build initial snapshot object (without volatile timestamps in digest)
	snap := &ResolvedPolicySnapshot{
		InputDigest:        inputDigest,
		RevisionID:         input.RevisionID,
		InventoryWatermark: input.InventoryWatermark,
		CompilerVersion:    input.CompilerVersion,
		Nodes:              snapshotNodes,
		NodeLogicalIDs:     snapshotLogicalIDs,
		Groups:             resolvedGroups,
		Rules:              resolvedRules,
		DNS:                input.DNS,
		Diagnostics:        allDiagnostics,
		RiskPolicyRevision: input.RiskPolicyRevision,
		RiskDecisionDigest: input.RiskDecisionDigest,
		RiskEvaluatedAt:    input.RiskEvaluatedAt,
		AdmittedNodeIDs:    finalAdmittedIDs,
		ExcludedNodeIDs:    finalExcludedIDs,
		FilterCounts:       filterCounts,
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
