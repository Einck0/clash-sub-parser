package legacy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// QuarantineCategory classifies reasons why a legacy item or field was quarantined.
type QuarantineCategory string

const (
	QuarantineCategoryUnknownColumn    QuarantineCategory = "unknown_column"
	QuarantineCategoryLegacyBlob       QuarantineCategory = "legacy_blob"
	QuarantineCategoryInvalidReference QuarantineCategory = "invalid_reference"
	QuarantineCategoryValidationFailed QuarantineCategory = "validation_failed"
	QuarantineCategoryUnmappedTable    QuarantineCategory = "unmapped_table"
)

// QuarantineItem records a quarantined field, entity, or invalid record without exposing raw values.
type QuarantineItem struct {
	Table         string             `json:"table"`
	RecordIDHash  string             `json:"record_id_hash"`
	FieldOrEntity string             `json:"field_or_entity"`
	Category      QuarantineCategory `json:"category"`
	Reason        string             `json:"reason"`
}

// SecretExclusionItem logs a confidential field or credential excluded from the target and reports.
type SecretExclusionItem struct {
	Table        string `json:"table"`
	RecordIDHash string `json:"record_id_hash"`
	Field        string `json:"field"`
	Action       string `json:"action"`
}

// ImportCounts tracks quantitative migration metrics.
type ImportCounts struct {
	SubscriptionsImported int `json:"subscriptions_imported"`
	NodeGroupsImported    int `json:"node_groups_imported"`
	GroupEdgesImported    int `json:"group_edges_imported"`
	PolicyRulesImported   int `json:"policy_rules_imported"`
	NodesImported         int `json:"nodes_imported"`
	QuarantineCount       int `json:"quarantine_count"`
	SecretExclusionCount  int `json:"secret_exclusion_count"`
}

// Options configures the legacy offline allowlist importer.
type Options struct {
	SourcePath string
	TargetPath string
	DryRun     bool
	ReportPath string
	Format     string
}

// ImportReport encapsulates the results of the legacy migration.
type ImportReport struct {
	DryRun              bool                  `json:"dry_run"`
	SourcePath          string                `json:"source_path"`
	SourceDigest        string                `json:"source_digest"`
	SchemaFingerprint   string                `json:"schema_fingerprint"`
	TargetRevisionID    string                `json:"target_revision_id"`
	TargetRevisionState string                `json:"target_revision_state"`
	Counts              ImportCounts          `json:"counts"`
	Quarantine          []QuarantineItem      `json:"quarantine"`
	SecretExclusions    []SecretExclusionItem `json:"secret_exclusions"`
	Timestamp           time.Time             `json:"timestamp"`
}

// MarshalJSON encodes the report into clean JSON.
func (r *ImportReport) MarshalJSON() ([]byte, error) {
	type Alias ImportReport
	return json.MarshalIndent(&struct {
		*Alias
	}{
		Alias: (*Alias)(r),
	}, "", "  ")
}

// FormatText formats the report into a structured, readable text summary for operator review.
func (r *ImportReport) FormatText() string {
	var b strings.Builder

	mode := "ACTUAL IMPORT (DRAFT REVISION)"
	if r.DryRun {
		mode = "DRY-RUN PREVIEW (NO WRITES)"
	}

	b.WriteString("=== CSP 1.0 Legacy Database Import Report ===\n")
	b.WriteString(fmt.Sprintf("Mode:                  %s\n", mode))
	b.WriteString(fmt.Sprintf("Timestamp:             %s\n", r.Timestamp.UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Source Database:       %s\n", r.SourcePath))
	b.WriteString(fmt.Sprintf("Source SHA-256:        %s\n", r.SourceDigest))
	b.WriteString(fmt.Sprintf("Schema Fingerprint:    %s\n", r.SchemaFingerprint))
	b.WriteString(fmt.Sprintf("Target Revision ID:    %s\n", r.TargetRevisionID))
	b.WriteString(fmt.Sprintf("Target Revision State: %s\n", r.TargetRevisionState))
	b.WriteString("\n--- Import Counts ---\n")
	b.WriteString(fmt.Sprintf("Subscriptions:         %d\n", r.Counts.SubscriptionsImported))
	b.WriteString(fmt.Sprintf("Node Groups:           %d\n", r.Counts.NodeGroupsImported))
	b.WriteString(fmt.Sprintf("Group Edges:           %d\n", r.Counts.GroupEdgesImported))
	b.WriteString(fmt.Sprintf("Policy Rules:          %d\n", r.Counts.PolicyRulesImported))
	b.WriteString(fmt.Sprintf("Nodes:                 %d\n", r.Counts.NodesImported))
	b.WriteString(fmt.Sprintf("Quarantined Items:     %d\n", r.Counts.QuarantineCount))
	b.WriteString(fmt.Sprintf("Secret Exclusions:     %d\n", r.Counts.SecretExclusionCount))

	if len(r.SecretExclusions) > 0 {
		b.WriteString("\n--- Secret Exclusions (Redacted) ---\n")
		for _, s := range r.SecretExclusions {
			b.WriteString(fmt.Sprintf("- Table: %s, KeyHash: %s, Field: %s (%s)\n", s.Table, s.RecordIDHash, s.Field, s.Action))
		}
	}

	if len(r.Quarantine) > 0 {
		b.WriteString("\n--- Quarantine Items (Sanitized) ---\n")
		for _, q := range r.Quarantine {
			b.WriteString(fmt.Sprintf("- Table: %s, KeyHash: %s, Entity: %s, Category: %s, Reason: %s\n",
				q.Table, q.RecordIDHash, q.FieldOrEntity, q.Category, q.Reason))
		}
	}

	b.WriteString("\nNotice: Imported content is saved under a 'draft' configuration revision.\n")
	b.WriteString("Explicit administrator review and activation is required before use in scheduling, probing, or publication.\n")
	return b.String()
}
