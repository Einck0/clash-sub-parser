package domain

import (
	"time"
)

// ImpactedBinding describes a group_edge that explicitly points to a node being reset.
type ImpactedBinding struct {
	EdgeID        string `json:"edge_id"`
	ParentGroupID string `json:"parent_group_id"`
	NodeLogicalID string `json:"node_logical_id"`
	Position      int    `json:"position"`
	TargetType    string `json:"target_type"`
}

// RebindPlanItem captures connection information for an impacted binding to allow strict re-binding
// if and only if a fresh node matching the exact connection (via ComputeConnectionLogicalID) is ingested.
type RebindPlanItem struct {
	EdgeID            string   `json:"edge_id"`
	ParentGroupID     string   `json:"parent_group_id"`
	OldNodeLogicalID  string   `json:"old_node_logical_id"`
	ExactConnectionID string   `json:"exact_connection_id"`
	Protocol          Protocol `json:"protocol"`
	Server            string   `json:"server"`
	Port              int      `json:"port"`
	Position          int      `json:"position"`
	Status            string   `json:"status"` // e.g. "unbound_pending_fresh_match"
	DiagnosticMessage string   `json:"diagnostic_message"`
}

// NodeInventoryResetReport encapsulates the audit metrics and results of a clean-slate node reset operation.
type NodeInventoryResetReport struct {
	DryRun                   bool              `json:"dry_run"`
	PreCounts                map[string]int    `json:"pre_counts"`
	PostCounts               map[string]int    `json:"post_counts"`
	DeletedCounts            map[string]int    `json:"deleted_counts"`
	FKViolations             []string          `json:"fk_violations"`
	DetachedPublishedEntries int               `json:"detached_published_entries"`
	PreservedPublications    int               `json:"preserved_publications"`
	PreservedAssetsUntouched bool              `json:"preserved_assets_untouched"`
	AssetFingerprint         string            `json:"asset_fingerprint"`
	SourceURLsRestored       map[string]bool   `json:"source_urls_restored,omitempty"`
	ImpactedBindingsCount    int               `json:"impacted_bindings_count"`
	ImpactedBindings         []ImpactedBinding `json:"impacted_bindings,omitempty"`
	RebindPlanCount          int               `json:"rebind_plan_count"`
	RebindPlan               []RebindPlanItem  `json:"rebind_plan,omitempty"`
	RebindManifestPath       string            `json:"rebind_manifest_path,omitempty"`
	ExecutedAt               time.Time         `json:"executed_at"`
}

// SourceTokenRepairResult encapsulates the summary of restoring source authentication tokens.
type SourceTokenRepairResult struct {
	RestoredSources []string          `json:"restored_sources"`
	RestoredStatus  map[string]bool   `json:"restored_status"`
	RedactedURLs    map[string]string `json:"redacted_urls"`
	RestoredCount   int               `json:"restored_count"`
	SkippedCount    int               `json:"skipped_count"`
	ExecutedAt      time.Time         `json:"executed_at"`
}
