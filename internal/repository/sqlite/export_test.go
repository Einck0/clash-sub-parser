package sqlite

import (
	"context"
	"database/sql"
	"time"

	"clash-sub-parser/internal/domain"
)

// Exported internal helpers for white-box transactional consistency testing.
var (
	LoadRevisionInternal                         = loadRevision
	LoadAdmissionRulesInternal                   = loadAdmissionRules
	LoadMihomoGraphInternal                      = loadMihomoGraph
	LoadGlobalNodeFilterInternal                 = loadGlobalNodeFilter
	LoadActiveNodesInternal                      = loadActiveNodes
	LoadActiveRiskPolicyAndObservationsInternal = loadActiveRiskPolicyAndObservations
)

// LoadConsistentFactsFromTx loads domain facts from an explicitly caller-managed transaction.
func LoadConsistentFactsFromTx(ctx context.Context, tx *sql.Tx, revisionID string, target domain.CompilerTarget, asOf time.Time) (*ConsistentFacts, error) {
	if asOf.IsZero() {
		asOf = domain.NowUTC()
	}

	facts := &ConsistentFacts{
		AsOf:               asOf,
		GroupSources:       make(map[string][]domain.MemberSource),
		GroupEdges:         make(map[string][]domain.GroupEdge),
		GroupFilters:       make(map[string]domain.NodeFilterSpec),
		NodeSources:        make(map[string][]domain.NodeSource),
		LatestObservations: make(map[string]map[domain.ProbeKind]domain.ProbeObservation),
	}

	rev, err := loadRevision(ctx, tx, revisionID)
	if err != nil {
		return nil, err
	}
	facts.Revision = rev

	admRules, err := loadAdmissionRules(ctx, tx, rev.ID)
	if err != nil {
		return nil, err
	}
	facts.AdmissionRules = admRules

	if target == domain.TargetMihomo {
		if err := loadMihomoGraph(ctx, tx, rev, facts); err != nil {
			return nil, err
		}
	}

	gf, err := loadGlobalNodeFilter(ctx, tx)
	if err != nil {
		return nil, err
	}
	facts.GlobalFilter = gf

	nodes, nodeIDs, err := loadActiveNodes(ctx, tx)
	if err != nil {
		return nil, err
	}
	facts.Nodes = nodes

	needsSources, needsObservations := inspectFilterDependencies(facts.GlobalFilter, facts.GroupFilters, facts.Groups)
	if needsSources && len(nodeIDs) > 0 {
		if err := loadNodeSources(ctx, tx, nodeIDs, facts.NodeSources); err != nil {
			return nil, err
		}
	}
	if needsObservations && len(nodeIDs) > 0 {
		if err := loadProbeObservations(ctx, tx, nodeIDs, facts.LatestObservations); err != nil {
			return nil, err
		}
	}

	if err := loadActiveRiskPolicyAndObservations(ctx, tx, nodeIDs, facts); err != nil {
		return nil, err
	}

	return facts, nil
}
