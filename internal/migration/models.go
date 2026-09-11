package migration

import (
	"database/sql"
)

// Options defines configuration for verification
type Options struct {
	DBPath       string
	OutputPath   string
	MarkdownPath string
	Strict       bool
}

// EntityStats tracks count and checksum for an entity type
type EntityStats struct {
	Count        int      `json:"count"`
	ValidCount   int      `json:"valid_count"`
	InvalidCount int      `json:"invalid_count"`
	Checksum     string   `json:"checksum"`
	SampleIDs    []int64  `json:"sample_ids,omitempty"`
}

// EntitiesSummary groups core audited entities
type EntitiesSummary struct {
	Subscriptions EntityStats `json:"subscriptions"`
	Nodes         EntityStats `json:"nodes"`
	NodeGroups    EntityStats `json:"node_groups"`
	Rules         EntityStats `json:"rules"`
	ProbeResults  EntityStats `json:"probe_results"`
}

// FieldValidation represents the result of checking an individual field or invariant
type FieldValidation struct {
	Entity  string `json:"entity"`
	Field   string `json:"field"`
	Checked int    `json:"checked"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Notes   string `json:"notes,omitempty"`
}

// VerificationReport contains the complete structured dry-run audit report
type VerificationReport struct {
	Timestamp          string            `json:"timestamp"`
	Engine             string            `json:"engine"`
	DatabasePath       string            `json:"database_path"`
	DatabaseSizeBytes  int64             `json:"database_size_bytes"`
	DatabaseSHA256     string            `json:"database_sha256"`
	IsReadOnlyEnforced bool              `json:"is_read_only_enforced"`
	DurationMs         int64             `json:"duration_ms"`
	TotalTables        int               `json:"total_tables"`
	TableCounts        map[string]int64  `json:"table_counts"`
	Entities           EntitiesSummary   `json:"entities"`
	ProtocolStats      map[string]int    `json:"protocol_stats"`
	ProbeStatusStats   map[string]int    `json:"probe_status_stats"`
	RuleTypeStats      map[string]int    `json:"rule_type_stats"`
	FieldChecks        []FieldValidation `json:"field_checks"`
	ValidationErrors   int               `json:"validation_errors"`
	ValidationWarnings int               `json:"validation_warnings"`
	Verdict            string            `json:"verdict"`
	Summary            string            `json:"summary"`
}

// Subscription represents a row in subscriptions table
type Subscription struct {
	ID                    int64
	Name                  string
	URL                   string
	UpdateInterval        sql.NullInt64
	IsPrimary             int
	NodePrefix            sql.NullString
	FilterRegex           sql.NullString
	RawNodes              sql.NullString
	LastFetchedAt         sql.NullString
	FetchComments         sql.NullString
	LastFetchError        sql.NullString
	FetchFailedCount      sql.NullInt64
	SubscriptionUserinfo  sql.NullString
	ProfileUpdateInterval sql.NullString
	ProfileWebPageURL     sql.NullString
	IncludeNodeNames      sql.NullString
	ExcludeNodeNames      sql.NullString
	SourceNodes           sql.NullString
	ManualNodes           sql.NullString
	Enabled               int
	NodeRenames           sql.NullString
	ProxyChain            sql.NullString
	NodeProxyChains       sql.NullString
	FilterMinSpeedMbps    sql.NullFloat64
	FilterMediaUnlock     sql.NullString
}

// Node represents a row in nodes table
type Node struct {
	ID                 int64
	LogicalID          string
	Name               string
	Protocol           string
	Server             string
	Port               int
	NormalizedPayload  string
	PayloadFingerprint string
	LifecycleState     string
	CreatedAt          sql.NullString
	UpdatedAt          sql.NullString
}

// NodeGroup represents a row in node_groups table
type NodeGroup struct {
	ID                  int64
	Name                string
	Kind                string
	GroupType           string
	SortOrder           sql.NullInt64
	RegexRules          sql.NullString
	IncludeNodes        sql.NullString
	IncludeGroupIDs     sql.NullString
	ExcludeNodes        sql.NullString
	URLTestConfig       sql.NullString
	LoadBalanceConfig   sql.NullString
	FallbackConfig      sql.NullString
	IncludeGroupNodesIDs sql.NullString
	IncludeEntries      sql.NullString
	AddFallback         sql.NullInt64
	ExcludeGroupIDs     sql.NullString
	FilterMinSpeedMbps  sql.NullFloat64
	FilterMediaUnlock   sql.NullString
}

// Rule represents a row in rules table
type Rule struct {
	ID        int64
	Name      string
	Category  string
	Type      string
	Value     string
	Proxy     string
	Options   sql.NullString
	SortOrder sql.NullInt64
	Enabled   sql.NullInt64
}

// NodeProbeResult represents a row in node_probe_results table
type NodeProbeResult struct {
	ID           int64
	NodeKey      string
	Name         string
	Server       string
	Port         int
	Type         string
	Status       string
	LatencyMs    sql.NullInt64
	SpeedMbps    sql.NullFloat64
	IP           sql.NullString
	Country      sql.NullString
	ASN          sql.NullInt64
	Organization sql.NullString
	Media        sql.NullString
	Error        sql.NullString
	CheckedAt    sql.NullInt64
}
