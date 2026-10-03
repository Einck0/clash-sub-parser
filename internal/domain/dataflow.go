package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// SubscriptionPayload represents raw un-parsed subscription body bytes and fetch metadata.
type SubscriptionPayload struct {
	ID             string            `json:"id"`
	SubscriptionID string            `json:"subscription_id"`
	FetchID        string            `json:"fetch_id"`
	ContentDigest  string            `json:"content_digest"`
	BodyBlob       []byte            `json:"-"`
	HTTPStatus     int               `json:"http_status"`
	HeadersJSON    string            `json:"headers_json"`
	Pinned         bool              `json:"pinned"`
	CreatedAt      time.Time         `json:"created_at"`
}

// EntryKind specifies whether a subscription entry is an actionable proxy, an informational notice, or unknown.
type EntryKind string

const (
	EntryKindProxy   EntryKind = "proxy"
	EntryKindNotice  EntryKind = "notice"
	EntryKindUnknown EntryKind = "unknown"
)

func (k EntryKind) IsValid() bool {
	return k == EntryKindProxy || k == EntryKindNotice || k == EntryKindUnknown
}

// SubscriptionEntry stores each raw parsed proxy or notice entry derived from a payload.
type SubscriptionEntry struct {
	ID                    string          `json:"id"`
	PayloadID             string          `json:"payload_id"`
	SubscriptionID        string          `json:"subscription_id"`
	Ordinal               int             `json:"ordinal"`
	SourceKey             string          `json:"source_key"`
	RawName               string          `json:"raw_name"`
	Protocol              Protocol        `json:"protocol"`
	Server                string          `json:"server"`
	Port                  int             `json:"port"`
	EntryKind             EntryKind       `json:"entry_kind"`
	ClassificationReason  string          `json:"classification_reason"`
	ClassificationVersion string          `json:"classification_version"`
	SourceProvenanceJSON  string          `json:"source_provenance_json"`
	UserKindOverride      *EntryKind      `json:"user_kind_override,omitempty"`
	OverrideAnchor        string          `json:"override_anchor,omitempty"`
	OverrideReason        string          `json:"override_reason,omitempty"`
	OverrideAt            *time.Time      `json:"override_at,omitempty"`
	ActorRef              string          `json:"actor_ref,omitempty"`
	ParsedConfigJSON      string          `json:"parsed_config_json"`
	ParserVersion         string          `json:"parser_version"`
	WarningsJSON          string          `json:"warnings_json"`
	NodeLogicalID         *string         `json:"node_logical_id,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
}

// EffectiveKind resolves the effective entry kind considering user overrides.
func (e *SubscriptionEntry) EffectiveKind() EntryKind {
	if e.UserKindOverride != nil && e.UserKindOverride.IsValid() {
		return *e.UserKindOverride
	}
	return e.EntryKind
}

// PublicationManifest represents the comprehensive manifest of a publication snapshot draft.
type PublicationManifest struct {
	NodeCount             int                    `json:"node_count"`
	ExcludedCount         int                    `json:"excluded_count"`
	PayloadIDs            []string               `json:"payload_ids"`
	ConfigurationRevision string                 `json:"configuration_revision,omitempty"`
	RulesDigest           string                 `json:"rules_digest,omitempty"`
	Included              []ManifestIncludedNode `json:"included"`
	Excluded              []ManifestExcludedNode `json:"excluded"`
	TargetEngineVersion   string                 `json:"target_engine_version"`
	MappingVersion        string                 `json:"mapping_version"`
}

// ManifestIncludedNode tracks an included node and its connection revision in the publication.
type ManifestIncludedNode struct {
	NodeID             string `json:"node_id"`
	ConnectionRevision int64  `json:"connection_revision"`
}

// ManifestExcludedNode tracks an excluded node and its diagnostic exclusion reason.
type ManifestExcludedNode struct {
	NodeID string `json:"node_id"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// NodeConnectionVersion holds an immutable, revisioned snapshot of a node's connection parameters.
type NodeConnectionVersion struct {
	NodeLogicalID       string    `json:"node_logical_id"`
	ConnectionRevision  int64     `json:"connection_revision"`
	EffectiveConfigJSON string    `json:"effective_config_json"`
	ConfigFingerprint   string    `json:"config_fingerprint"`
	SourceEntryID       *string   `json:"source_entry_id,omitempty"`
	SchemaVersion       int       `json:"schema_version"`
	CreatedAt           time.Time `json:"created_at"`
}

// NodeConnectionHead tracks the authoritative pointer to a node's current connection revision.
type NodeConnectionHead struct {
	LogicalID          string    `json:"logical_id"`
	ConnectionRevision int64     `json:"connection_revision"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// NodeOverride holds an explicit user field-level override that survives refreshes.
type NodeOverride struct {
	NodeLogicalID     string    `json:"node_logical_id"`
	FieldPath         string    `json:"field_path"`
	OverrideValueJSON string    `json:"override_value_json"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PublicationPayloadRef represents a reference between a publication and a source payload,
// preventing garbage collection of referenced payloads via SQLite foreign key RESTRICT.
type PublicationPayloadRef struct {
	PublicationID string `json:"publication_id"`
	PayloadID     string `json:"payload_id"`
}

// SafeDetail represents the strict allowlist of diagnostic fields for probe observations.
type SafeDetail struct {
	Stage     string `json:"stage,omitempty"`
	Code      string `json:"code,omitempty"`
	Core      string `json:"core,omitempty"`
	Transport string `json:"transport,omitempty"`
	Status    int    `json:"status,omitempty"`
	Timeout   int    `json:"timeout,omitempty"`
}

// SanitizeSafeDetail accepts arbitrary structured data and retains ONLY the strict allowlist fields.
// It explicitly strips raw err.Error(), full URLs, query strings, headers, passwords, UUIDs, tokens, and body.
func SanitizeSafeDetail(raw map[string]any) SafeDetail {
	if raw == nil {
		return SafeDetail{}
	}
	var d SafeDetail
	if v, ok := raw["stage"].(string); ok {
		d.Stage = strings.TrimSpace(v)
	}
	if v, ok := raw["code"].(string); ok {
		d.Code = strings.TrimSpace(v)
	}
	if v, ok := raw["core"].(string); ok {
		d.Core = strings.TrimSpace(v)
	}
	if v, ok := raw["transport"].(string); ok {
		d.Transport = strings.TrimSpace(v)
	}
	if v, ok := raw["status"].(float64); ok {
		d.Status = int(v)
	} else if v, ok := raw["status"].(int); ok {
		d.Status = v
	}
	if v, ok := raw["timeout"].(float64); ok {
		d.Timeout = int(v)
	} else if v, ok := raw["timeout"].(int); ok {
		d.Timeout = v
	}
	return d
}

// ComputeConnectionFingerprint generates a canonical SHA-256 fingerprint for node connection parameters.
func ComputeConnectionFingerprint(server string, port int, creds InboundProtocolCredential) string {
	type canonicalParams struct {
		Server      string                    `json:"server"`
		Port        int                       `json:"port"`
		Credentials InboundProtocolCredential `json:"credentials"`
	}
	p := canonicalParams{
		Server:      strings.ToLower(strings.TrimSpace(server)),
		Port:        port,
		Credentials: creds,
	}
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// NoticeClassifier evaluates whether a subscription entry is an informational notice pseudo-node.
// It requires BOTH verified source rule AND exact combo match:
// 1. Verified Source Gate: Only enabled for verified source contexts (such as Dogegg or verified rule_version).
// 2. Exact Rule Match (ALL 4 conditions):
//    a) server IN ('127.0.0.1', 'localhost', '0.0.0.0')
//    b) port == 1
//    c) uuid == '00000000-0000-4000-8000-000000000000'
//    d) name matches known notice keywords (剩余流量, 套餐到期, 新域名, 公告, 通知)
func ClassifySubscriptionEntry(
	isVerifiedSource bool,
	server string,
	port int,
	creds InboundProtocolCredential,
	rawName string,
) (kind EntryKind, reason string, version string) {
	normServer := strings.ToLower(strings.TrimSpace(server))
	isLoopbackOrZero := normServer == "127.0.0.1" || normServer == "localhost" || normServer == "0.0.0.0"
	isPort1 := port == 1
	isZeroUUID := strings.TrimSpace(creds.UUID) == "00000000-0000-4000-8000-000000000000" ||
		strings.TrimSpace(creds.Password) == "00000000-0000-4000-8000-000000000000"

	hasNoticeKeyword := strings.Contains(rawName, "剩余流量") ||
		strings.Contains(rawName, "套餐到期") ||
		strings.Contains(rawName, "新域名") ||
		strings.Contains(rawName, "公告") ||
		strings.Contains(rawName, "通知")

	exactComboMatch := isLoopbackOrZero && isPort1 && isZeroUUID && hasNoticeKeyword

	if exactComboMatch {
		if isVerifiedSource {
			return EntryKindNotice, "Verified source notice combo rule match", "v2-proven-combo"
		}
		// Unverified source meeting notice features -> keep as unknown for user audit, never auto-promote
		return EntryKindUnknown, "Matches notice pattern but source lacks verified rule gate", "v2-proven-combo"
	}

	// Normal proxies (even with local private IPs if port != 1 and valid credentials) -> proxy
	return EntryKindProxy, "Normal proxy", "v2-proven-combo"
}
