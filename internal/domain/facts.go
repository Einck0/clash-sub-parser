package domain

import (
	"context"
	"time"
)

// ConsistentFacts holds an immutable snapshot of all database facts loaded within a single read transaction.
type ConsistentFacts struct {
	AsOf               time.Time
	Revision           *ConfigurationRevision
	Groups             []NodeGroup
	GroupSources       map[string][]MemberSource
	GroupEdges         map[string][]GroupEdge
	PolicyRules        []PolicyRule
	AdmissionRules     []AdmissionRule
	Nodes              []Node
	GlobalFilter       *NodeFilterSpec
	GroupFilters       map[string]NodeFilterSpec
	NodeSources        map[string][]NodeSource
	LatestObservations map[string]map[ProbeKind]ProbeObservation
	ActiveRiskPolicy   *RiskPolicyRevision
	RiskObservations   []IPRiskObservation
	PrimaryUserInfo    string
	DNSSettings        *DNSSettings
}

// ConsistentFactsLoader defines the persistence port for transactional loading of consistent domain facts.
type ConsistentFactsLoader interface {
	LoadConsistentFacts(ctx context.Context, revisionID string, target CompilerTarget, asOf time.Time) (*ConsistentFacts, error)
}
