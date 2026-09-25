package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FilterField represents the node attribute or evidence dimension to filter against.
type FilterField string

const (
	FilterFieldDisplayName         FilterField = "display_name"
	FilterFieldProtocol            FilterField = "protocol"
	FilterFieldSourceSubscriptions FilterField = "source_subscription_ids"
	FilterFieldProbeVerdict        FilterField = "probe_verdict"
	FilterFieldProbeLatencyMS      FilterField = "probe_latency_ms"
)

var validFilterFields = map[FilterField]bool{
	FilterFieldDisplayName:         true,
	FilterFieldProtocol:            true,
	FilterFieldSourceSubscriptions: true,
	FilterFieldProbeVerdict:        true,
	FilterFieldProbeLatencyMS:      true,
}

func (f FilterField) IsValid() bool {
	return validFilterFields[f]
}

func ParseFilterField(s string) (FilterField, error) {
	norm := FilterField(strings.ToLower(strings.TrimSpace(s)))
	// Alias support: source_subscription_id -> source_subscription_ids
	if norm == "source_subscription_id" {
		norm = FilterFieldSourceSubscriptions
	}
	if norm == "probe:latency_ms" || norm == "latency_ms" {
		norm = FilterFieldProbeLatencyMS
	}
	if norm == "probe:verdict" || norm == "verdict" {
		norm = FilterFieldProbeVerdict
	}
	if !norm.IsValid() {
		return "", NewValidationError("invalid_filter_field", fmt.Sprintf("unsupported filter field: %s", s))
	}
	return norm, nil
}

// FilterOp represents the comparison operator for a filter condition.
type FilterOp string

const (
	FilterOpContains    FilterOp = "contains"
	FilterOpNotContains FilterOp = "not_contains"
	FilterOpEquals      FilterOp = "equals"
	FilterOpNotEquals   FilterOp = "not_equals"
	FilterOpLTE         FilterOp = "lte"
)

var validFilterOps = map[FilterOp]bool{
	FilterOpContains:    true,
	FilterOpNotContains: true,
	FilterOpEquals:      true,
	FilterOpNotEquals:   true,
	FilterOpLTE:         true,
}

func (o FilterOp) IsValid() bool {
	return validFilterOps[o]
}

func ParseFilterOp(s string) (FilterOp, error) {
	norm := FilterOp(strings.ToLower(strings.TrimSpace(s)))
	if norm == "<=" {
		norm = FilterOpLTE
	}
	if !norm.IsValid() {
		return "", NewValidationError("invalid_filter_op", fmt.Sprintf("unsupported filter operator: %s", s))
	}
	return norm, nil
}

// FilterCondition represents a single bounded AND condition.
type FilterCondition struct {
	Field            FilterField `json:"field"`
	Op               FilterOp    `json:"op"`
	Value            string      `json:"value,omitempty"`
	ProbeKind        *ProbeKind  `json:"probe_kind,omitempty"`
	FreshnessSeconds *int        `json:"freshness_seconds,omitempty"`
}

// Validate checks that the filter condition has valid fields, operators, values, and bounds.
func (c FilterCondition) Validate() error {
	field, err := ParseFilterField(string(c.Field))
	if err != nil {
		return err
	}
	c.Field = field

	op, err := ParseFilterOp(string(c.Op))
	if err != nil {
		return err
	}
	c.Op = op

	val := strings.TrimSpace(c.Value)

	switch c.Field {
	case FilterFieldDisplayName:
		if c.Op != FilterOpContains && c.Op != FilterOpNotContains {
			return NewValidationError("invalid_filter_op", fmt.Sprintf("display_name only supports contains/not_contains, got %s", c.Op))
		}
		if val == "" {
			return NewValidationError("invalid_filter_value", "display_name filter value cannot be empty")
		}
		if len(val) > 255 {
			return NewValidationError("invalid_filter_value", "display_name filter value cannot exceed 255 characters")
		}
		if c.ProbeKind != nil {
			return NewValidationError("invalid_filter_field", "display_name filter condition must not specify probe_kind")
		}
		if c.FreshnessSeconds != nil {
			return NewValidationError("invalid_filter_field", "display_name filter condition must not specify freshness_seconds")
		}

	case FilterFieldProtocol:
		if c.Op != FilterOpEquals && c.Op != FilterOpNotEquals {
			return NewValidationError("invalid_filter_op", fmt.Sprintf("protocol only supports equals/not_equals, got %s", c.Op))
		}
		if _, err := ParseProtocol(val); err != nil {
			return NewValidationError("invalid_filter_value", fmt.Sprintf("invalid protocol value: %s", val))
		}
		if c.ProbeKind != nil || c.FreshnessSeconds != nil {
			return NewValidationError("invalid_filter_field", "protocol filter condition must not specify probe parameters")
		}

	case FilterFieldSourceSubscriptions:
		if c.Op != FilterOpContains && c.Op != FilterOpNotContains {
			return NewValidationError("invalid_filter_op", fmt.Sprintf("source_subscription_ids only supports contains/not_contains, got %s", c.Op))
		}
		if val == "" {
			return NewValidationError("invalid_filter_value", "source_subscription_ids value cannot be empty")
		}
		if !IsValidUUIDv7(val) {
			return NewValidationError("invalid_filter_value", fmt.Sprintf("subscription id must be a valid UUIDv7: %s", val))
		}
		if c.ProbeKind != nil || c.FreshnessSeconds != nil {
			return NewValidationError("invalid_filter_field", "source subscription filter condition must not specify probe parameters")
		}

	case FilterFieldProbeVerdict:
		if c.Op != FilterOpEquals && c.Op != FilterOpNotEquals {
			return NewValidationError("invalid_filter_op", fmt.Sprintf("probe_verdict only supports equals/not_equals, got %s", c.Op))
		}
		if _, err := ParseProbeVerdict(val); err != nil {
			return NewValidationError("invalid_filter_value", fmt.Sprintf("invalid probe verdict: %s", val))
		}
		if c.ProbeKind == nil || !c.ProbeKind.IsValid() {
			return NewValidationError("invalid_probe_kind", "probe_verdict condition requires a valid probe_kind")
		}
		if c.FreshnessSeconds != nil && (*c.FreshnessSeconds < 1 || *c.FreshnessSeconds > 604800) {
			return NewValidationError("invalid_freshness_seconds", fmt.Sprintf("freshness_seconds must be between 1 and 604800, got %d", *c.FreshnessSeconds))
		}

	case FilterFieldProbeLatencyMS:
		if c.Op != FilterOpLTE {
			return NewValidationError("invalid_filter_op", fmt.Sprintf("probe_latency_ms only supports lte, got %s", c.Op))
		}
		latency, err := strconv.ParseInt(val, 10, 64)
		if err != nil || latency < 0 || latency > 60000 {
			return NewValidationError("invalid_filter_value", fmt.Sprintf("latency_ms must be an integer between 0 and 60000, got: %s", val))
		}
		if c.ProbeKind == nil || !c.ProbeKind.IsValid() {
			return NewValidationError("invalid_probe_kind", "probe_latency_ms condition requires a valid probe_kind")
		}
		if c.FreshnessSeconds != nil && (*c.FreshnessSeconds < 1 || *c.FreshnessSeconds > 604800) {
			return NewValidationError("invalid_freshness_seconds", fmt.Sprintf("freshness_seconds must be between 1 and 604800, got %d", *c.FreshnessSeconds))
		}

	default:
		return NewValidationError("invalid_filter_field", fmt.Sprintf("unsupported filter field: %s", c.Field))
	}

	return nil
}

// NodeFilterSpec represents a structured set of AND filter conditions.
type NodeFilterSpec struct {
	Conditions []FilterCondition `json:"conditions"`
}

// IsEmpty returns whether the filter has no conditions (evaluates to true for all nodes).
func (s *NodeFilterSpec) IsEmpty() bool {
	return s == nil || len(s.Conditions) == 0
}

// Validate verifies that the filter spec contains at most 32 conditions and all conditions are valid.
func (s *NodeFilterSpec) Validate() error {
	if s == nil || len(s.Conditions) == 0 {
		return nil
	}
	if len(s.Conditions) > 32 {
		return NewValidationError("too_many_conditions",
			fmt.Sprintf("maximum of 32 filter conditions allowed, got %d", len(s.Conditions)))
	}
	for i, cond := range s.Conditions {
		if err := cond.Validate(); err != nil {
			return fmt.Errorf("condition %d: %w", i, err)
		}
	}
	return nil
}

// GlobalNodeFilter represents the singleton global filter configuration.
type GlobalNodeFilter struct {
	Spec      NodeFilterSpec `json:"spec"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// GroupNodeFilter represents the per-group node filter binding.
type GroupNodeFilter struct {
	GroupID   string         `json:"group_id"`
	Spec      NodeFilterSpec `json:"spec"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// MatchesCondition evaluates a single FilterCondition against a node and its context.
// Returns (true, "") if matched, or (false, reason) if rejected.
func MatchesCondition(c FilterCondition, node Node, sources []NodeSource, latestObs map[ProbeKind]ProbeObservation, asOf time.Time) (bool, string) {
	field, _ := ParseFilterField(string(c.Field))
	op, _ := ParseFilterOp(string(c.Op))

	switch field {
	case FilterFieldDisplayName:
		match := strings.Contains(strings.ToLower(node.DisplayName), strings.ToLower(c.Value))
		if op == FilterOpContains && !match {
			return false, fmt.Sprintf("display_name does not contain %q", c.Value)
		}
		if op == FilterOpNotContains && match {
			return false, fmt.Sprintf("display_name contains %q", c.Value)
		}
		return true, ""

	case FilterFieldProtocol:
		match := strings.EqualFold(string(node.Protocol), c.Value)
		if op == FilterOpEquals && !match {
			return false, fmt.Sprintf("protocol %s != %s", node.Protocol, c.Value)
		}
		if op == FilterOpNotEquals && match {
			return false, fmt.Sprintf("protocol %s == %s", node.Protocol, c.Value)
		}
		return true, ""

	case FilterFieldSourceSubscriptions:
		hasSource := false
		for _, s := range sources {
			if s.SubscriptionID == c.Value {
				hasSource = true
				break
			}
		}
		if op == FilterOpContains && !hasSource {
			return false, fmt.Sprintf("node not from subscription %s", c.Value)
		}
		if op == FilterOpNotContains && hasSource {
			return false, fmt.Sprintf("node from excluded subscription %s", c.Value)
		}
		return true, ""

	case FilterFieldProbeVerdict, FilterFieldProbeLatencyMS:
		if c.ProbeKind == nil {
			return false, "missing probe kind in filter condition"
		}
		obs, found := latestObs[*c.ProbeKind]
		if !found {
			return false, fmt.Sprintf("no %s probe observation available", *c.ProbeKind)
		}

		// Fail-closed if credential version is unversioned/unknown or mismatched
		if !obs.HasValidCredentialVersion(node.CredentialVersion) {
			return false, fmt.Sprintf("observation credential version mismatch (node=%d)", node.CredentialVersion)
		}

		// Freshness check: default 86400s (24h) if not specified
		freshnessSec := 86400
		if c.FreshnessSeconds != nil && *c.FreshnessSeconds > 0 {
			freshnessSec = *c.FreshnessSeconds
		}
		freshnessDuration := time.Duration(freshnessSec) * time.Second
		if asOf.Sub(obs.ObservedAt) > freshnessDuration {
			return false, fmt.Sprintf("observation expired (freshness %ds exceeded)", freshnessSec)
		}

		if field == FilterFieldProbeVerdict {
			expectedVerdict := ProbeVerdict(strings.ToLower(strings.TrimSpace(c.Value)))
			match := obs.Verdict == expectedVerdict
			if op == FilterOpEquals && !match {
				return false, fmt.Sprintf("probe %s verdict %s != %s", *c.ProbeKind, obs.Verdict, expectedVerdict)
			}
			if op == FilterOpNotEquals && match {
				return false, fmt.Sprintf("probe %s verdict %s == %s", *c.ProbeKind, obs.Verdict, expectedVerdict)
			}
			return true, ""
		}

		if field == FilterFieldProbeLatencyMS {
			// Quality latency condition only valid for available verdict
			if obs.Verdict != VerdictAvailable {
				return false, fmt.Sprintf("probe %s verdict is %s (latency requires available)", *c.ProbeKind, obs.Verdict)
			}
			maxLatency, err := strconv.ParseInt(c.Value, 10, 64)
			if err != nil {
				return false, "invalid latency threshold"
			}
			if obs.LatencyMS > maxLatency {
				return false, fmt.Sprintf("probe latency %dms > threshold %dms", obs.LatencyMS, maxLatency)
			}
			return true, ""
		}

	default:
		return false, fmt.Sprintf("unknown filter field %s", field)
	}

	return true, ""
}

// MatchesFilter evaluates all conditions in a NodeFilterSpec with AND semantics.
// Returns (true, "") if all conditions pass, or (false, reason) on first failure.
func MatchesFilter(spec *NodeFilterSpec, node Node, sources []NodeSource, latestObs map[ProbeKind]ProbeObservation, asOf time.Time) (bool, string) {
	if spec == nil || spec.IsEmpty() {
		return true, ""
	}
	for _, cond := range spec.Conditions {
		if matched, reason := MatchesCondition(cond, node, sources, latestObs, asOf); !matched {
			return false, reason
		}
	}
	return true, ""
}
