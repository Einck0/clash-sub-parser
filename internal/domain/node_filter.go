package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
	lru "github.com/hashicorp/golang-lru/v2"
)

// Bounds apply to both validation and matching. RE2 remains the first choice;
// regexp2 is used only when RE2 cannot compile the original expression.
const filterRegexTimeout = 50 * time.Millisecond
const filterRegexInputLimit = 4096

type filterRegex struct {
	re2          *regexp.Regexp
	backtracking *regexp2.Regexp
}

var filterRegexCache, _ = lru.New[string, *filterRegex](128)

func compileFilterRegex(pattern string) (*filterRegex, error) {
	pattern = strings.TrimSpace(pattern)
	if len(pattern) == 0 || len(pattern) > 1024 {
		return nil, fmt.Errorf("regex pattern must contain 1 to 1024 bytes")
	}
	if cached, ok := filterRegexCache.Get(pattern); ok {
		return cached, nil
	}
	// Preserve the established case-insensitive name-filter contract.
	pat := pattern
	if !strings.HasPrefix(pat, "(?i)") {
		pat = "(?i)" + pat
	}
	compiled := &filterRegex{}
	if re, err := regexp.Compile(pat); err == nil {
		compiled.re2 = re
	} else {
		re, err := regexp2.Compile(pat, regexp2.None)
		if err != nil {
			return nil, err
		}
		re.MatchTimeout = filterRegexTimeout
		compiled.backtracking = re
	}
	filterRegexCache.Add(pattern, compiled)
	return compiled, nil
}

// FilterRegex wraps either standard RE2 or PCRE/backtracking regexp2.
type FilterRegex = filterRegex

// CompileFilterRegex compiles a pattern with case-insensitive contract and regex cache.
func CompileFilterRegex(pattern string) (*FilterRegex, error) {
	return compileFilterRegex(pattern)
}

// Match tests whether the string matches the compiled filter regex.
func (r *filterRegex) Match(input string) (bool, error) {
	return r.match(input)
}

func (r *filterRegex) match(input string) (bool, error) {
	if len(input) > filterRegexInputLimit {
		return false, fmt.Errorf("regex input exceeds %d bytes", filterRegexInputLimit)
	}
	if r.re2 != nil {
		return r.re2.MatchString(input), nil
	}
	return r.backtracking.MatchString(input)
}

// FilterField represents the node attribute or evidence dimension to filter against.
type FilterField string

const (
	FilterFieldDisplayName         FilterField = "display_name"
	FilterFieldProtocol            FilterField = "protocol"
	FilterFieldSourceSubscriptions FilterField = "source_subscription_ids"
	FilterFieldProbeVerdict        FilterField = "probe_verdict"
	FilterFieldProbeLatencyMS      FilterField = "probe_latency_ms"
	FilterFieldHealthStatus        FilterField = "health_status"
	FilterFieldExitRegion          FilterField = "exit_region"
	FilterFieldPlatformVerdict     FilterField = "platform_verdict"
)

var validFilterFields = map[FilterField]bool{
	FilterFieldDisplayName:         true,
	FilterFieldProtocol:            true,
	FilterFieldSourceSubscriptions: true,
	FilterFieldProbeVerdict:        true,
	FilterFieldProbeLatencyMS:      true,
	FilterFieldHealthStatus:        true,
	FilterFieldExitRegion:          true,
	FilterFieldPlatformVerdict:     true,
}

func (f FilterField) IsValid() bool {
	return validFilterFields[f]
}

func ParseFilterField(s string) (FilterField, error) {
	norm := FilterField(strings.ToLower(strings.TrimSpace(s)))
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
	FilterOpRegex       FilterOp = "regex"
	FilterOpNotRegex    FilterOp = "not_regex"
	FilterOpEquals      FilterOp = "equals"
	FilterOpNotEquals   FilterOp = "not_equals"
	FilterOpLTE         FilterOp = "lte"
)

var validFilterOps = map[FilterOp]bool{
	FilterOpContains:    true,
	FilterOpNotContains: true,
	FilterOpRegex:       true,
	FilterOpNotRegex:    true,
	FilterOpEquals:      true,
	FilterOpNotEquals:   true,
	FilterOpLTE:         true,
}

func (o FilterOp) IsValid() bool {
	return validFilterOps[o]
}

func ParseFilterOp(s string) (FilterOp, error) {
	norm := FilterOp(strings.ToLower(strings.TrimSpace(s)))
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
	Platform         string      `json:"platform,omitempty"`
	FreshnessSeconds *int        `json:"freshness_seconds,omitempty"`
}

// Validate checks that the filter condition has valid fields, operators, values, and bounds.
func validPlatformForKind(kind ProbeKind, platform string) bool {
	platforms := map[ProbeKind]map[string]bool{
		ProbeKindAI: {"openai": true, "claude": true, "gemini": true},
		ProbeKindStreaming: {"netflix": true, "disney": true, "youtube": true},
	}
	return platforms[kind][strings.ToLower(strings.TrimSpace(platform))]
}

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
		if c.Op != FilterOpContains && c.Op != FilterOpNotContains && c.Op != FilterOpRegex && c.Op != FilterOpNotRegex {
			return NewValidationError("invalid_filter_op", fmt.Sprintf("display_name only supports contains/not_contains/regex/not_regex, got %s", c.Op))
		}
		if val == "" {
			return NewValidationError("invalid_filter_value", "display_name filter value cannot be empty")
		}
		maxLen := 255
		if c.Op == FilterOpRegex || c.Op == FilterOpNotRegex {
			maxLen = 1024
		}
		if len(val) > maxLen {
			return NewValidationError("invalid_filter_value", fmt.Sprintf("display_name filter value cannot exceed %d characters", maxLen))
		}
		if c.Op == FilterOpRegex || c.Op == FilterOpNotRegex {
			if _, err := compileFilterRegex(val); err != nil {
				return NewValidationError("invalid_filter_value", fmt.Sprintf("invalid regex pattern: %v", err))
			}
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

	case FilterFieldHealthStatus:
		if c.Op != FilterOpEquals && c.Op != FilterOpNotEquals {
			return NewValidationError("invalid_filter_op", "health_status only supports equals/not_equals")
		}
		if val != string(HealthCategoryHealthy) && val != string(HealthCategoryUnhealthy) && val != string(HealthCategoryUntested) {
			return NewValidationError("invalid_filter_value", "health_status must be healthy, unhealthy, or untested")
		}
		if c.ProbeKind != nil || c.Platform != "" || c.FreshnessSeconds != nil {
			return NewValidationError("invalid_filter_field", "health_status does not accept probe parameters")
		}

	case FilterFieldExitRegion:
		if c.Op != FilterOpEquals && c.Op != FilterOpNotEquals {
			return NewValidationError("invalid_filter_op", "exit_region only supports equals/not_equals")
		}
		if len(val) != 2 || val[0] < 'A' || val[0] > 'Z' || val[1] < 'A' || val[1] > 'Z' {
			return NewValidationError("invalid_filter_value", "exit_region must be an ISO alpha-2 code")
		}
		if c.ProbeKind != nil || c.Platform != "" || c.FreshnessSeconds != nil {
			return NewValidationError("invalid_filter_field", "exit_region does not accept probe parameters")
		}

	case FilterFieldPlatformVerdict:
		if c.Op != FilterOpEquals && c.Op != FilterOpNotEquals {
			return NewValidationError("invalid_filter_op", "platform_verdict only supports equals/not_equals")
		}
		if val != "available" && val != "restricted" && val != "unknown" && val != "error" {
			return NewValidationError("invalid_filter_value", "platform_verdict must be available, restricted, unknown, or error")
		}
		if c.ProbeKind == nil || (*c.ProbeKind != ProbeKindAI && *c.ProbeKind != ProbeKindStreaming) {
			return NewValidationError("invalid_probe_kind", "platform_verdict requires ai or streaming probe_kind")
		}
		if !validPlatformForKind(*c.ProbeKind, c.Platform) {
			return NewValidationError("invalid_platform", "unsupported platform for probe kind")
		}
		if c.FreshnessSeconds != nil {
			return NewValidationError("invalid_filter_field", "platform_verdict does not accept freshness_seconds")
		}

	case FilterFieldProbeLatencyMS:
		if c.ProbeKind != nil && *c.ProbeKind != ProbeKindBaseline {
			return NewValidationError("invalid_probe_kind", "probe_latency_ms is fixed to baseline")
		}
		if c.Op != FilterOpLTE {
			return NewValidationError("invalid_filter_op", fmt.Sprintf("probe_latency_ms only supports lte, got %s", c.Op))
		}
		latency, err := strconv.ParseInt(val, 10, 64)
		if err != nil || latency < 0 || latency > 60000 {
			return NewValidationError("invalid_filter_value", fmt.Sprintf("latency_ms must be an integer between 0 and 60000, got: %s", val))
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

// NormalizeForSave strips deprecated freshness_seconds so persisted filters no longer store natural-time TTLs.
func (s *NodeFilterSpec) NormalizeForSave() {
	if s == nil {
		return
	}
	for i := range s.Conditions {
		s.Conditions[i].FreshnessSeconds = nil
	}
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
		if op == FilterOpRegex || op == FilterOpNotRegex {
			re, err := compileFilterRegex(c.Value)
			if err != nil {
				return false, fmt.Sprintf("invalid regex: %v", err)
			}
			matched, err := re.match(node.DisplayName)
			// In particular, not_regex must not invert errors/timeouts into success.
			if err != nil {
				return false, fmt.Sprintf("regex match failed: %v", err)
			}
			if op == FilterOpRegex && !matched {
				return false, fmt.Sprintf("display_name does not match regex %q", c.Value)
			}
			if op == FilterOpNotRegex && matched {
				return false, fmt.Sprintf("display_name matches excluded regex %q", c.Value)
			}
			return true, ""
		}
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

	case FilterFieldHealthStatus:
		category := ClassifyNodeHealth(latestObs, node.ConnectionRevision, asOf, 0)
		return compareFilterValue(string(category), strings.ToLower(strings.TrimSpace(c.Value)), op)

	case FilterFieldExitRegion:
		obs, ok := latestObs[ProbeKindGeo]
		if !ok || obs.Kind != ProbeKindGeo || obs.ConnectionRevision == nil || *obs.ConnectionRevision != node.ConnectionRevision || node.ConnectionRevision < 1 || obs.ObservedAt.IsZero() || obs.ObservedAt.After(asOf) || obs.Verdict != VerdictAvailable || strings.TrimSpace(obs.Region) == "" {
			return false, "no current successful geo region evidence"
		}
		return compareFilterValue(strings.ToUpper(strings.TrimSpace(obs.Region)), strings.ToUpper(strings.TrimSpace(c.Value)), op)

	case FilterFieldPlatformVerdict:
		if c.ProbeKind == nil {
			return false, "missing probe kind for platform verdict"
		}
		kind := *c.ProbeKind
		obs, ok := latestObs[kind]
		if !ok || obs.Kind != kind || obs.ConnectionRevision == nil || *obs.ConnectionRevision != node.ConnectionRevision || node.ConnectionRevision < 1 || obs.ObservedAt.IsZero() || obs.ObservedAt.After(asOf) {
			return false, "no current platform observation available"
		}
		capability, ok := obs.Platforms[strings.ToLower(strings.TrimSpace(c.Platform))]
		if !ok {
			return false, "platform evidence missing"
		}
		return compareFilterValue(string(capability.Verdict), strings.ToLower(strings.TrimSpace(c.Value)), op)

	case FilterFieldProbeVerdict, FilterFieldProbeLatencyMS:
		kind := ProbeKindBaseline
		if c.ProbeKind != nil {
			kind = *c.ProbeKind
		}
		obs, found := latestObs[kind]
		if !found {
			return false, fmt.Sprintf("no %s probe observation available", kind)
		}
		if obs.ConnectionRevision == nil || *obs.ConnectionRevision != node.ConnectionRevision || node.ConnectionRevision < 1 || obs.ObservedAt.IsZero() || obs.ObservedAt.After(asOf) {
			return false, fmt.Sprintf("observation not from current valid connection revision")
		}

		if field == FilterFieldProbeVerdict {
			expectedVerdict := ProbeVerdict(strings.ToLower(strings.TrimSpace(c.Value)))
			match := obs.Verdict == expectedVerdict
			if op == FilterOpEquals && !match {
				return false, fmt.Sprintf("probe %s verdict %s != %s", kind, obs.Verdict, expectedVerdict)
			}
			if op == FilterOpNotEquals && match {
				return false, fmt.Sprintf("probe %s verdict %s == %s", kind, obs.Verdict, expectedVerdict)
			}
			return true, ""
		}

		if field == FilterFieldProbeLatencyMS {
			health, hasLatency := EvaluateBaselineHealth(&obs, node.ConnectionRevision, asOf, 0)
			if kind != ProbeKindBaseline || health != BaselineHealthy || !hasLatency || obs.LatencyMS <= 0 || obs.Verdict != VerdictAvailable {
				return false, "baseline latency requires current healthy evidence with real positive latency"
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
func compareFilterValue(actual, expected string, op FilterOp) (bool, string) {
	match := strings.EqualFold(actual, expected)
	if (op == FilterOpEquals && !match) || (op == FilterOpNotEquals && match) {
		return false, fmt.Sprintf("value %q does not satisfy %s %q", actual, op, expected)
	}
	return true, ""
}

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
