package domain

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Pagination encapsulates request and response paging parameters.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// NodeScope represents the query scope for node listings.
type NodeScope string

const (
	NodeScopeDefault              NodeScope = ""
	NodeScopeEnabledSubscriptions NodeScope = "enabled_subscriptions"
	NodeScopeAllAssets            NodeScope = "all_assets"
)

func (s NodeScope) IsValid() bool {
	return s == NodeScopeDefault || s == NodeScopeEnabledSubscriptions || s == NodeScopeAllAssets
}

func ParseNodeScope(raw string) (NodeScope, error) {
	norm := NodeScope(strings.ToLower(strings.TrimSpace(raw)))
	if norm == "" {
		return NodeScopeDefault, nil
	}
	if norm == NodeScopeEnabledSubscriptions || norm == NodeScopeAllAssets {
		return norm, nil
	}
	return "", NewValidationError("invalid_node_scope", fmt.Sprintf("unsupported node scope: %s (supported: %s, %s)", raw, NodeScopeEnabledSubscriptions, NodeScopeAllAssets))
}

// NodeFilter defines server-side query filters for nodes.
type NodeFilter struct {
	Pagination          Pagination     `json:"pagination"`
	LogicalIDs          []string       `json:"logical_ids,omitempty"`
	Protocols           []Protocol     `json:"protocols,omitempty"`
	Countries           []string       `json:"countries,omitempty"`
	ActiveOnly          bool           `json:"active_only"`
	ExcludeNotices      bool           `json:"exclude_notices,omitempty"`
	SearchText          string         `json:"search_text,omitempty"`
	SortBy              string         `json:"sort_by,omitempty"`
	SortOrder           string         `json:"sort_order,omitempty"`
	Scope               NodeScope      `json:"scope,omitempty"`
	SubscriptionID      string         `json:"subscription_id,omitempty"`
	RiskDecisions       []RiskAction   `json:"risk_decisions,omitempty"`
	RiskBands           []RiskBand     `json:"risk_bands,omitempty"`
	RiskProviders       []string       `json:"risk_providers,omitempty"`
	RiskStatuses        []IPRiskStatus `json:"risk_statuses,omitempty"`
	RiskPolicyRevisions []string       `json:"risk_policy_revisions,omitempty"`
}

// SubscriptionFilter defines query filters for subscriptions.
type SubscriptionFilter struct {
	Pagination  Pagination
	EnabledOnly bool
	SearchText  string
}

// AuditFilter defines query filters for audit events.
type AuditFilter struct {
	Pagination Pagination
	ActorKind  *ActorKind
	Action     string
	From       *time.Time
	To         *time.Time
}

// RevisionFilter defines query filters for configuration revisions.
type RevisionFilter struct {
	Pagination Pagination
	State      *ConfigurationRevisionState
}

// SubscriptionRepository defines the persistence port for subscriptions.
type SubscriptionRepository interface {
	GetByID(ctx context.Context, id string) (*Subscription, error)
	List(ctx context.Context, filter SubscriptionFilter) ([]Subscription, int, error)
	Create(ctx context.Context, sub *Subscription) error
	Update(ctx context.Context, sub *Subscription) error
	Delete(ctx context.Context, id string) error
}

// SubscriptionFetchRepository defines the persistence port for fetch audit records.
type SubscriptionFetchRepository interface {
	GetByID(ctx context.Context, id string) (*SubscriptionFetch, error)
	ListBySubscription(ctx context.Context, subID string, limit int) ([]SubscriptionFetch, error)
	Create(ctx context.Context, fetch *SubscriptionFetch) error
}

// NodeRepository defines the persistence port for normalized nodes.
type NodeRepository interface {
	GetByLogicalID(ctx context.Context, logicalID string) (*Node, error)
	List(ctx context.Context, filter NodeFilter) ([]Node, int, error)
	ListReadModel(ctx context.Context, filter NodeFilter) ([]NodeReadModel, int, error)
	GetReadModel(ctx context.Context, logicalID string, policyRevisionID string) (*NodeReadModel, error)
	UpsertBatch(ctx context.Context, nodes []Node) error
	DeactivateNodesNotIn(ctx context.Context, activeLogicalIDs []string) error
}

// NodeSourceRepository defines the persistence port for node-subscription associations.
type NodeSourceRepository interface {
	ListByNode(ctx context.Context, logicalID string) ([]NodeSource, error)
	ListByNodes(ctx context.Context, logicalIDs []string) (map[string][]NodeSource, error)
	ListBySubscription(ctx context.Context, subID string) ([]NodeSource, error)
	Upsert(ctx context.Context, src *NodeSource) error
	DeleteBySubscriptionAndFetch(ctx context.Context, subID string, currentFetchID string) error
}

// NodeSourceHistoryRepository defines the persistence port for the immutable node provenance history ledger.
type NodeSourceHistoryRepository interface {
	InsertBatch(ctx context.Context, records []NodeSourceHistory) error
	ListByNodeLogicalID(ctx context.Context, nodeLogicalID string) ([]NodeSourceHistory, error)
	ListBySubscriptionID(ctx context.Context, subscriptionID string) ([]NodeSourceHistory, error)
}

// ProbeRunRepository defines the persistence port for probe run jobs.
type ProbeRunRepository interface {
	GetByID(ctx context.Context, id string) (*ProbeRun, error)
	GetByIdempotencyKey(ctx context.Context, actorScope string, key string) (*ProbeRun, error)
	Create(ctx context.Context, run *ProbeRun) error
	UpdateState(ctx context.Context, id string, state ProbeRunState) error
	List(ctx context.Context, state *ProbeRunState, page, pageSize int) ([]ProbeRun, int, error)
	ListActive(ctx context.Context) ([]ProbeRun, error)
}

// ProbeObservationRepository defines the persistence port for probe observations.
type ProbeObservationRepository interface {
	GetByID(ctx context.Context, id string) (*ProbeObservation, error)
	ListByRun(ctx context.Context, runID string) ([]ProbeObservation, error)
	ListByNode(ctx context.Context, nodeLogicalID string, limit int) ([]ProbeObservation, error)
	ListLatestByNodes(ctx context.Context, nodeLogicalIDs []string, kinds []ProbeKind) (map[string]map[ProbeKind]ProbeObservation, error)
	Create(ctx context.Context, obs *ProbeObservation) error
}

// ProbeScheduleRepository defines the persistence port for periodic probe schedules, batches, and leases.
type ProbeScheduleRepository interface {
	Get(ctx context.Context) (*ProbeSchedule, error)
	Update(ctx context.Context, schedule *ProbeSchedule) error

	GetBatchByID(ctx context.Context, id string) (*ProbeBatch, error)
	ListBatches(ctx context.Context, page, pageSize int) ([]ProbeBatch, int, error)
	GetBatchByWindow(ctx context.Context, generation int64, windowAt time.Time) (*ProbeBatch, error)
	CreateBatch(ctx context.Context, batch *ProbeBatch) error
	UpdateBatch(ctx context.Context, batch *ProbeBatch) error

	AcquireLease(ctx context.Context, batchID string, owner string, leaseDuration time.Duration) (bool, error)
	HeartbeatLease(ctx context.Context, batchID string, owner string, leaseDuration time.Duration) error
	ReleaseLease(ctx context.Context, batchID string, owner string) error
}

// NodeFilterRepository defines the persistence port for global and group node filter specifications.
type NodeFilterRepository interface {
	GetGlobalFilter(ctx context.Context) (*GlobalNodeFilter, error)
	SetGlobalFilter(ctx context.Context, filter *GlobalNodeFilter) error

	GetGroupFilter(ctx context.Context, groupID string) (*GroupNodeFilter, error)
	SetGroupFilter(ctx context.Context, filter *GroupNodeFilter) error
	DeleteGroupFilter(ctx context.Context, groupID string) error
	ListGroupFilters(ctx context.Context) (map[string]NodeFilterSpec, error)
}

// PolicyRepository defines the persistence port for policy groups, edges, and rules.
type PolicyRepository interface {
	GetGroupByID(ctx context.Context, id string) (*NodeGroup, error)
	ListGroups(ctx context.Context) ([]NodeGroup, error)
	CreateGroup(ctx context.Context, group *NodeGroup) error
	UpdateGroup(ctx context.Context, group *NodeGroup) error
	DeleteGroup(ctx context.Context, id string) error

	ListEdgesByGroup(ctx context.Context, parentGroupID string) ([]GroupEdge, error)
	SetEdgesForGroup(ctx context.Context, parentGroupID string, edges []GroupEdge) error

	ListAdmissionRules(ctx context.Context, revisionID string) ([]AdmissionRule, error)
	CreateAdmissionRule(ctx context.Context, rule *AdmissionRule) error
	DeleteAdmissionRule(ctx context.Context, id string) error

	ListPolicyRules(ctx context.Context, revisionID string) ([]PolicyRule, error)
	CreatePolicyRule(ctx context.Context, rule *PolicyRule) error
	DeletePolicyRule(ctx context.Context, id string) error
}

// RevisionRepository defines the persistence port for configuration revisions.
type RevisionRepository interface {
	GetByID(ctx context.Context, id string) (*ConfigurationRevision, error)
	GetActive(ctx context.Context) (*ConfigurationRevision, error)
	List(ctx context.Context, filter RevisionFilter) ([]ConfigurationRevision, int, error)
	Create(ctx context.Context, rev *ConfigurationRevision) error
	// CreateActive inserts and activates a revision in one transaction; failures leave no new revision.
	CreateActive(ctx context.Context, rev *ConfigurationRevision) error
	SetActive(ctx context.Context, id string) error
}

// PublicationRepository defines the atomic persistence port for immutable configuration publications.
type PublicationRepository interface {
	GetByID(ctx context.Context, id string) (*Publication, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*Publication, error)
	Create(ctx context.Context, pub *Publication) error
	Revoke(ctx context.Context, id string, revokedAt time.Time) error
	Activate(ctx context.Context, id, tokenHash string) error
}

// SettingsRepository defines the persistence port for global settings.
type SettingsRepository interface {
	Get(ctx context.Context) (*Settings, error)
	Update(ctx context.Context, settings *Settings) error
	UpdateAdminToken(ctx context.Context, tokenVerifier string) error
	// UpdateAuthSettings atomically sets both switches and optionally replaces the token verifier.
	// A nil verifier preserves the currently configured system token.
	UpdateAuthSettings(ctx context.Context, adminAuthEnabled, exportAuthEnabled bool, tokenVerifier *string) error
}

// IPRiskObservationRepository defines append-only normalized risk evidence persistence.
type IPRiskObservationRepository interface {
	GetByID(ctx context.Context, id string) (*IPRiskObservation, error)
	List(ctx context.Context, filter IPRiskObservationFilter) ([]IPRiskObservation, int, error)
	Create(ctx context.Context, observation *IPRiskObservation) error
}

// IPRiskProviderSettingsRepository defines provider deployment setting persistence.
type IPRiskProviderSettingsRepository interface {
	Get(ctx context.Context, provider, schemaVersion string) (*IPRiskProviderSettings, error)
	List(ctx context.Context, enabledOnly bool) ([]IPRiskProviderSettings, error)
	Upsert(ctx context.Context, settings *IPRiskProviderSettings) error
}

// RiskPolicyRevisionRepository defines versioned risk policy persistence.
type RiskPolicyRevisionRepository interface {
	GetByID(ctx context.Context, id string) (*RiskPolicyRevision, error)
	GetActive(ctx context.Context) (*RiskPolicyRevision, error)
	Create(ctx context.Context, revision *RiskPolicyRevision) error
	SetActive(ctx context.Context, id string, active bool) error
	List(ctx context.Context, filter RiskPolicyFilter) ([]RiskPolicyRevision, int, error)
}

// RiskPolicyGroupBindingRepository manages bindings between node groups and risk policy revisions.
type RiskPolicyGroupBindingRepository interface {
	GetByGroupID(ctx context.Context, groupID string) (*RiskPolicyGroupBinding, error)
	List(ctx context.Context) ([]RiskPolicyGroupBinding, error)
	Set(ctx context.Context, binding *RiskPolicyGroupBinding) error
	Delete(ctx context.Context, groupID string) error
}

// AuditRepository defines the persistence port for audit logs.
type AuditRepository interface {
	Record(ctx context.Context, event *AuditEvent) error
	List(ctx context.Context, filter AuditFilter) ([]AuditEvent, int, error)
}

// SubscriptionPayloadRepository manages raw subscription payloads.
type SubscriptionPayloadRepository interface {
	Save(ctx context.Context, payload *SubscriptionPayload) error
	GetByID(ctx context.Context, id string) (*SubscriptionPayload, error)
	GetLatestBySubscription(ctx context.Context, subscriptionID string) (*SubscriptionPayload, error)
	GetByDigest(ctx context.Context, contentDigest string) (*SubscriptionPayload, error)
	Pin(ctx context.Context, id string, pinned bool) error
	PruneUnreferenced(ctx context.Context, olderThan time.Time) (int64, error)
}

// SubscriptionEntryRepository manages parsed subscription entries.
type SubscriptionEntryRepository interface {
	SaveBatch(ctx context.Context, entries []SubscriptionEntry) error
	ListByPayload(ctx context.Context, payloadID string) ([]SubscriptionEntry, error)
	ListBySubscription(ctx context.Context, subscriptionID string, limit, offset int) ([]SubscriptionEntry, int, error)
	ListLatestBySubscription(ctx context.Context, subscriptionID string) ([]SubscriptionEntry, error)
	GetByID(ctx context.Context, id string) (*SubscriptionEntry, error)
	SetUserOverride(ctx context.Context, entryID string, userKindOverride *EntryKind, overrideAnchor, reason, actorRef string, overrideAt time.Time) error
}

// NodeConnectionVersionRepository manages versioned immutable connection configurations.
type NodeConnectionVersionRepository interface {
	Save(ctx context.Context, version *NodeConnectionVersion) error
	GetByRevision(ctx context.Context, nodeLogicalID string, revision int64) (*NodeConnectionVersion, error)
	GetHead(ctx context.Context, nodeLogicalID string) (*NodeConnectionVersion, error)
	ListByNode(ctx context.Context, nodeLogicalID string) ([]NodeConnectionVersion, error)
	SetHead(ctx context.Context, nodeLogicalID string, revision int64, updatedAt time.Time) error
}

// NodeOverrideRepository manages explicit user field-level overrides.
type NodeOverrideRepository interface {
	Save(ctx context.Context, override *NodeOverride) error
	GetByNode(ctx context.Context, nodeLogicalID string) ([]NodeOverride, error)
	GetAll(ctx context.Context) (map[string][]NodeOverride, error)
	Delete(ctx context.Context, nodeLogicalID, fieldPath string) error
}

// PublicationPayloadRefRepository manages references between publications and payloads.
type PublicationPayloadRefRepository interface {
	AddRefs(ctx context.Context, publicationID string, payloadIDs []string) error
	ListPayloadIDsByPublication(ctx context.Context, publicationID string) ([]string, error)
	DeleteRefsByPublication(ctx context.Context, publicationID string) error
	ResolvePayloadIDsForNodes(ctx context.Context, nodes []ManifestIncludedNode) ([]string, error)
}
