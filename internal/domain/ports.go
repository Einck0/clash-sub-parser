package domain

import (
	"context"
	"time"
)

// Pagination encapsulates request and response paging parameters.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// NodeFilter defines server-side query filters for nodes.
type NodeFilter struct {
	Pagination          Pagination     `json:"pagination"`
	Protocols           []Protocol     `json:"protocols,omitempty"`
	Countries           []string       `json:"countries,omitempty"`
	ActiveOnly          bool           `json:"active_only"`
	SearchText          string         `json:"search_text,omitempty"`
	SortBy              string         `json:"sort_by,omitempty"`
	SortOrder           string         `json:"sort_order,omitempty"`
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
	ListBySubscription(ctx context.Context, subID string) ([]NodeSource, error)
	Upsert(ctx context.Context, src *NodeSource) error
	DeleteBySubscriptionAndFetch(ctx context.Context, subID string, currentFetchID string) error
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
	Create(ctx context.Context, obs *ProbeObservation) error
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

	ListPolicyRules(ctx context.Context, revisionID string) ([]PolicyRule, error)
	CreatePolicyRule(ctx context.Context, rule *PolicyRule) error
}

// RevisionRepository defines the persistence port for configuration revisions.
type RevisionRepository interface {
	GetByID(ctx context.Context, id string) (*ConfigurationRevision, error)
	GetActive(ctx context.Context) (*ConfigurationRevision, error)
	List(ctx context.Context, filter RevisionFilter) ([]ConfigurationRevision, int, error)
	Create(ctx context.Context, rev *ConfigurationRevision) error
	SetActive(ctx context.Context, id string) error
}

// PublicationRepository defines the persistence port for configuration publications.
type PublicationRepository interface {
	GetByID(ctx context.Context, id string) (*Publication, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*Publication, error)
	Create(ctx context.Context, pub *Publication) error
	Revoke(ctx context.Context, id string, revokedAt time.Time) error
}

// SettingsRepository defines the persistence port for global settings.
type SettingsRepository interface {
	Get(ctx context.Context) (*Settings, error)
	Update(ctx context.Context, settings *Settings) error
	UpdateAdminToken(ctx context.Context, tokenVerifier string) error
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

// NodeCredentialRepository defines persistence for AEAD-encrypted node credentials.
type NodeCredentialRepository interface {
	GetByLogicalID(ctx context.Context, logicalID string, version int) (*NodeCredentialRecord, error)
	GetLatestByLogicalID(ctx context.Context, logicalID string) (*NodeCredentialRecord, error)
	Upsert(ctx context.Context, record *NodeCredentialRecord) error
	DeleteByLogicalID(ctx context.Context, logicalID string) error
}
