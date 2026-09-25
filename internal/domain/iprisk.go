package domain

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

// RiskAction is the local action produced by a risk policy.
type RiskAction string

const (
	RiskActionAllow   RiskAction = "allow"
	RiskActionReview  RiskAction = "review"
	RiskActionBlock   RiskAction = "block"
	RiskActionUnknown RiskAction = "unknown"
)

func (a RiskAction) IsValid() bool {
	return a == RiskActionAllow || a == RiskActionReview || a == RiskActionBlock || a == RiskActionUnknown
}

// ParseRiskAction parses a policy action or unknown decision action.
func ParseRiskAction(value string) (RiskAction, error) {
	action := RiskAction(strings.ToLower(strings.TrimSpace(value)))
	if !action.IsValid() {
		return "", NewValidationError("invalid_risk_action", fmt.Sprintf("unsupported risk action: %s", value))
	}
	return action, nil
}

func (a RiskAction) isPolicyAction() bool {
	return a == RiskActionAllow || a == RiskActionReview || a == RiskActionBlock
}

// RiskBand is the policy-owned risk classification, not a provider label.
type RiskBand string

const (
	RiskBandLow      RiskBand = "low"
	RiskBandMedium   RiskBand = "medium"
	RiskBandHigh     RiskBand = "high"
	RiskBandCritical RiskBand = "critical"
	RiskBandUnknown  RiskBand = "unknown"
)

func (b RiskBand) IsValid() bool {
	return b == RiskBandLow || b == RiskBandMedium || b == RiskBandHigh ||
		b == RiskBandCritical || b == RiskBandUnknown
}

// ParseRiskBand parses a normalized policy risk band.
func ParseRiskBand(value string) (RiskBand, error) {
	band := RiskBand(strings.ToLower(strings.TrimSpace(value)))
	if !band.IsValid() {
		return "", NewValidationError("invalid_risk_band", fmt.Sprintf("unsupported risk band: %s", value))
	}
	return band, nil
}

// IPRiskStatus describes the freshness and availability of an IP risk observation.
type IPRiskStatus string

const (
	IPRiskStatusAvailable IPRiskStatus = "available"
	IPRiskStatusUnknown   IPRiskStatus = "unknown"
	IPRiskStatusError     IPRiskStatus = "error"
	IPRiskStatusStale     IPRiskStatus = "stale"
)

func (s IPRiskStatus) IsValid() bool {
	return s == IPRiskStatusAvailable || s == IPRiskStatusUnknown ||
		s == IPRiskStatusError || s == IPRiskStatusStale
}

// ParseIPRiskStatus parses a normalized IP risk observation status.
func ParseIPRiskStatus(value string) (IPRiskStatus, error) {
	status := IPRiskStatus(strings.ToLower(strings.TrimSpace(value)))
	if !status.IsValid() {
		return "", NewValidationError("invalid_ip_risk_status", fmt.Sprintf("unsupported IP risk status: %s", value))
	}
	return status, nil
}

// NetworkClass is the normalized provider network category.
type NetworkClass string

const (
	NetworkClassResidential NetworkClass = "residential"
	NetworkClassDatacenter  NetworkClass = "datacenter"
	NetworkClassMobile      NetworkClass = "mobile"
	NetworkClassBusiness    NetworkClass = "business"
	NetworkClassUnknown     NetworkClass = "unknown"
)

func (c NetworkClass) IsValid() bool {
	return c == NetworkClassResidential || c == NetworkClassDatacenter ||
		c == NetworkClassMobile || c == NetworkClassBusiness || c == NetworkClassUnknown
}

// ParseNetworkClass parses a normalized provider network class.
func ParseNetworkClass(value string) (NetworkClass, error) {
	class := NetworkClass(strings.ToLower(strings.TrimSpace(value)))
	if !class.IsValid() {
		return "", NewValidationError("invalid_network_class", fmt.Sprintf("unsupported network class: %s", value))
	}
	return class, nil
}

// AnonymizerTrait is a normalized provider trait.
type AnonymizerTrait string

const (
	TraitProxy            AnonymizerTrait = "proxy"
	TraitVPN              AnonymizerTrait = "vpn"
	TraitTor              AnonymizerTrait = "tor"
	TraitResidentialProxy AnonymizerTrait = "residential_proxy"
	TraitHosting          AnonymizerTrait = "hosting"
	TraitUnknown          AnonymizerTrait = "unknown"
)

func (t AnonymizerTrait) IsValid() bool {
	return t == TraitProxy || t == TraitVPN || t == TraitTor ||
		t == TraitResidentialProxy || t == TraitHosting || t == TraitUnknown
}

// ParseAnonymizerTrait parses a normalized anonymizer trait.
func ParseAnonymizerTrait(value string) (AnonymizerTrait, error) {
	trait := AnonymizerTrait(strings.ToLower(strings.TrimSpace(value)))
	if !trait.IsValid() {
		return "", NewValidationError("invalid_anonymizer_trait", fmt.Sprintf("unsupported anonymizer trait: %s", value))
	}
	return trait, nil
}

// RiskFusionMode controls how observations from selected providers are combined.
type RiskFusionMode string

const (
	RiskFusionSingleProvider RiskFusionMode = "single_provider"
	RiskFusionAllMustAllow   RiskFusionMode = "all_must_allow"
	RiskFusionHighestRisk    RiskFusionMode = "highest_risk"
)

func (m RiskFusionMode) IsValid() bool {
	return m == RiskFusionSingleProvider || m == RiskFusionAllMustAllow || m == RiskFusionHighestRisk
}

// ParseRiskFusionMode parses a policy provider fusion mode.
func ParseRiskFusionMode(value string) (RiskFusionMode, error) {
	mode := RiskFusionMode(strings.ToLower(strings.TrimSpace(value)))
	if !mode.IsValid() {
		return "", NewValidationError("invalid_risk_fusion_mode", fmt.Sprintf("unsupported risk fusion mode: %s", value))
	}
	return mode, nil
}

// ProviderRef identifies a versioned provider contract without carrying credentials.
type ProviderRef struct {
	Provider      string `json:"provider"`
	SchemaVersion string `json:"schema_version"`
}

// RiskProviderSelection declares the provider set and explicit fusion mode.
type RiskProviderSelection struct {
	Mode      RiskFusionMode `json:"mode"`
	Providers []ProviderRef  `json:"providers"`
}

// ScoreBand maps an inclusive score interval to a policy-owned band and action.
type ScoreBand struct {
	Min    int        `json:"min"`
	Max    int        `json:"max"`
	Band   RiskBand   `json:"band"`
	Action RiskAction `json:"action"`
}

// TraitRule maps a normalized anonymizer trait to a policy action.
type TraitRule struct {
	Trait  AnonymizerTrait `json:"trait"`
	Action RiskAction      `json:"action"`
}

const (
	MaxIPRiskIdentifierLength      = 64
	MaxIPRiskRedactedSummaryLength = 512
)

var (
	isoCountryCodeRegex   = regexp.MustCompile(`^[A-Z]{2}$`)
	ipv4Regex             = regexp.MustCompile(`(?:^|[^0-9])((?:[0-9]{1,3}\.){3}[0-9]{1,3})(?:$|[^0-9])`)
	ipv6Regex             = regexp.MustCompile(`(?i)(?:[0-9a-f]{0,4}:){2,}[0-9a-f:]{0,4}`)
	urlQueryRegex         = regexp.MustCompile(`(?i)\bhttps?://[^\s?]+\?[^\s]+`)
	reasonCodeRegex       = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
	ipRiskIdentifierRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
)

// ExitIdentity is the non-reversible identity proof for one node egress.
type ExitIdentity struct {
	NodeLogicalID  string    `json:"node_logical_id"`
	ObservedAt     time.Time `json:"observed_at"`
	IdentityDigest string    `json:"identity_digest"`
	CountryCode    string    `json:"country_code,omitempty"`
	ASN            *int      `json:"asn,omitempty"`
}

// Validate enforces an identity contract without accepting a complete IP address.
func (e ExitIdentity) Validate() error {
	if !IsValidLogicalID(e.NodeLogicalID) {
		return NewValidationError("invalid_exit_node_logical_id", "exit identity has an invalid node logical ID")
	}
	if !validDigest(e.IdentityDigest) {
		return NewValidationError("invalid_exit_identity_digest", "exit identity digest is invalid")
	}
	if e.ObservedAt.IsZero() {
		return NewValidationError("missing_exit_observed_at", "exit identity observed_at is required")
	}
	if e.CountryCode != "" && !isoCountryCodeRegex.MatchString(e.CountryCode) {
		return NewValidationError("invalid_exit_country_code", "exit identity country_code must be ISO 3166-1 alpha-2")
	}
	if e.ASN != nil && *e.ASN <= 0 {
		return NewValidationError("invalid_exit_asn", "exit identity ASN must be positive")
	}
	return nil
}

// IPRiskObservation is an immutable, minimized provider result.
type IPRiskObservation struct {
	ID                    string            `json:"id"`
	NodeLogicalID         string            `json:"node_logical_id"`
	ExitIdentityDigest    string            `json:"exit_identity_digest"`
	Provider              string            `json:"provider"`
	ProviderSchemaVersion string            `json:"provider_schema_version"`
	ObservedAt            time.Time         `json:"observed_at"`
	ExpiresAt             time.Time         `json:"expires_at"`
	Status                IPRiskStatus      `json:"status"`
	Score                 *int              `json:"score,omitempty"`
	Confidence            *int              `json:"confidence,omitempty"`
	NetworkClass          NetworkClass      `json:"network_class"`
	AnonymizerTraits      []AnonymizerTrait `json:"anonymizer_traits,omitempty"`
	EvidenceDigest        string            `json:"evidence_digest"`
	RedactedSummary       string            `json:"redacted_summary,omitempty"`
}

// Validate enforces the safe persistence contract for an observation.
func (o IPRiskObservation) Validate() error {
	if !IsValidUUIDv7(o.ID) {
		return NewValidationError("invalid_ip_risk_observation_id", "IP risk observation id must be UUIDv7")
	}
	if !IsValidLogicalID(o.NodeLogicalID) {
		return NewValidationError("invalid_ip_risk_node_logical_id", "IP risk observation has an invalid node logical ID")
	}
	if !validDigest(o.ExitIdentityDigest) || !validDigest(o.EvidenceDigest) {
		return NewValidationError("invalid_ip_risk_digest", "IP risk observation digest is invalid")
	}
	if err := validateIPRiskIdentifier("provider", o.Provider); err != nil {
		return err
	}
	if err := validateIPRiskIdentifier("schema_version", o.ProviderSchemaVersion); err != nil {
		return err
	}
	if o.ObservedAt.IsZero() || o.ExpiresAt.IsZero() || !o.ExpiresAt.After(o.ObservedAt) {
		return NewValidationError("invalid_ip_risk_ttl", "IP risk observation expires_at must be after observed_at")
	}
	if !o.Status.IsValid() || !o.NetworkClass.IsValid() {
		return NewValidationError("invalid_ip_risk_status", "IP risk observation status or network class is invalid")
	}
	if err := validatePercent("score", o.Score); err != nil {
		return err
	}
	if err := validatePercent("confidence", o.Confidence); err != nil {
		return err
	}
	seen := make(map[AnonymizerTrait]struct{}, len(o.AnonymizerTraits))
	for _, trait := range o.AnonymizerTraits {
		if !trait.IsValid() {
			return NewValidationError("invalid_ip_risk_trait", fmt.Sprintf("unsupported anonymizer trait: %s", trait))
		}
		if _, exists := seen[trait]; exists {
			return NewValidationError("duplicate_ip_risk_trait", "anonymizer traits must be unique")
		}
		seen[trait] = struct{}{}
	}
	if _, unknown := seen[TraitUnknown]; unknown && len(seen) != 1 {
		return NewValidationError("ambiguous_ip_risk_traits", "unknown anonymizer trait cannot be combined with another trait")
	}
	if err := validateRedactedSummary(o.RedactedSummary); err != nil {
		return err
	}
	return nil
}

// RiskPolicy is a versioned local policy and never contains provider credentials.
type RiskPolicy struct {
	RevisionID        string                `json:"revision_id"`
	ProviderSelection RiskProviderSelection `json:"provider_selection"`
	MaxObservationAge time.Duration         `json:"max_observation_age"`
	MinimumConfidence int                   `json:"minimum_confidence"`
	ScoreBands        []ScoreBand           `json:"score_bands"`
	TraitRules        []TraitRule           `json:"trait_rules,omitempty"`
	UnknownAction     RiskAction            `json:"unknown_action,omitempty"`
	ConflictAction    RiskAction            `json:"conflict_action,omitempty"`
	ReviewAction      RiskAction            `json:"review_action,omitempty"`
}

// EffectiveUnknownAction preserves the safe default when older policy data omits it.
func (p RiskPolicy) EffectiveUnknownAction() RiskAction {
	if p.UnknownAction.isPolicyAction() {
		return p.UnknownAction
	}
	return RiskActionReview
}

// EffectiveConflictAction preserves the safe default when policy data omits it.
func (p RiskPolicy) EffectiveConflictAction() RiskAction {
	if p.ConflictAction.isPolicyAction() {
		return p.ConflictAction
	}
	return RiskActionReview
}

// EffectiveReviewAction preserves the safe default when policy data omits it.
func (p RiskPolicy) EffectiveReviewAction() RiskAction {
	if p.ReviewAction.isPolicyAction() {
		return p.ReviewAction
	}
	return RiskActionReview
}

// Validate enforces complete, non-overlapping score bands and safe policy actions.
func (p RiskPolicy) Validate() error {
	if !IsValidUUIDv7(p.RevisionID) {
		return NewValidationError("invalid_risk_policy_revision_id", "risk policy revision_id must be UUIDv7")
	}
	if !p.ProviderSelection.Mode.IsValid() || len(p.ProviderSelection.Providers) == 0 {
		return NewValidationError("invalid_risk_provider_selection", "risk policy provider selection is incomplete")
	}
	if p.ProviderSelection.Mode == RiskFusionSingleProvider && len(p.ProviderSelection.Providers) != 1 {
		return NewValidationError("invalid_risk_provider_selection", "single-provider fusion requires exactly one provider")
	}
	seenProviders := make(map[string]struct{}, len(p.ProviderSelection.Providers))
	for _, provider := range p.ProviderSelection.Providers {
		if err := validateIPRiskIdentifier("provider", provider.Provider); err != nil {
			return NewValidationError("invalid_risk_provider_reference", "risk policy provider reference is incomplete")
		}
		if err := validateIPRiskIdentifier("schema_version", provider.SchemaVersion); err != nil {
			return NewValidationError("invalid_risk_provider_reference", "risk policy provider reference is incomplete")
		}
		key := provider.Provider + "\x00" + provider.SchemaVersion
		if _, exists := seenProviders[key]; exists {
			return NewValidationError("duplicate_risk_provider_reference", "risk policy provider references must be unique")
		}
		seenProviders[key] = struct{}{}
	}
	if p.MaxObservationAge <= 0 || p.MinimumConfidence < 0 || p.MinimumConfidence > 100 {
		return NewValidationError("invalid_risk_policy_threshold", "risk policy age and confidence thresholds are invalid")
	}
	if err := validateScoreBands(p.ScoreBands); err != nil {
		return err
	}
	for _, rule := range p.TraitRules {
		if !rule.Trait.IsValid() || rule.Trait == TraitUnknown || !rule.Action.isPolicyAction() {
			return NewValidationError("invalid_risk_trait_rule", "risk policy trait rule is invalid")
		}
	}
	for i, left := range p.TraitRules {
		for _, right := range p.TraitRules[i+1:] {
			if left.Trait == right.Trait {
				return NewValidationError("duplicate_risk_trait_rule", "risk policy trait rules must be unique")
			}
		}
	}
	for name, action := range map[string]RiskAction{
		"unknown_action": p.UnknownAction, "conflict_action": p.ConflictAction, "review_action": p.ReviewAction,
	} {
		if action != "" && !action.isPolicyAction() {
			return NewValidationError("invalid_"+name, "risk policy action must be allow, review or block")
		}
	}
	return nil
}

// RiskDecision is a deterministic read model derived from a policy and observations.
type RiskDecision struct {
	NodeLogicalID        string     `json:"node_logical_id"`
	PolicyRevisionID     string     `json:"policy_revision_id"`
	Decision             RiskAction `json:"decision"`
	ReasonCode           string     `json:"reason_code"`
	ObservationDigestSet []string   `json:"observation_digest_set,omitempty"`
	EvaluatedAt          time.Time  `json:"evaluated_at"`
}

// IPRiskSummary is the API-safe risk read model for node inventory responses.
type IPRiskSummary struct {
	Decision              RiskAction   `json:"decision"`
	RiskBand              RiskBand     `json:"risk_band"`
	Provider              string       `json:"provider"`
	ProviderSchemaVersion string       `json:"provider_schema_version"`
	ObservedAt            time.Time    `json:"observed_at"`
	ExpiresAt             time.Time    `json:"expires_at"`
	Status                IPRiskStatus `json:"status"`
	ReasonCode            string       `json:"reason_code"`
	PolicyRevisionID      *string      `json:"policy_revision_id,omitempty"`
}

// Validate checks that an API summary contains only stable, non-sensitive fields.
func (s IPRiskSummary) Validate() error {
	if !s.Decision.IsValid() || !s.RiskBand.IsValid() || !s.Status.IsValid() {
		return NewValidationError("invalid_ip_risk_summary", "IP risk summary contains an invalid state")
	}
	if err := validateIPRiskIdentifier("provider", s.Provider); err != nil {
		return err
	}
	if err := validateIPRiskIdentifier("schema_version", s.ProviderSchemaVersion); err != nil {
		return err
	}
	if s.ObservedAt.IsZero() || s.ExpiresAt.IsZero() || !s.ExpiresAt.After(s.ObservedAt) {
		return NewValidationError("invalid_ip_risk_summary_ttl", "IP risk summary timestamps are invalid")
	}
	if !reasonCodeRegex.MatchString(s.ReasonCode) || containsSensitiveIPRiskText(s.ReasonCode) {
		return NewValidationError("invalid_ip_risk_summary_reason", "IP risk summary reason code is unsafe")
	}
	if s.PolicyRevisionID != nil && !IsValidUUIDv7(*s.PolicyRevisionID) {
		return NewValidationError("invalid_ip_risk_summary_policy", "IP risk summary policy revision is invalid")
	}
	return nil
}

// Validate checks that a decision contains only safe, stable references.
func (d RiskDecision) Validate() error {
	if !IsValidLogicalID(d.NodeLogicalID) || !IsValidUUIDv7(d.PolicyRevisionID) {
		return NewValidationError("invalid_risk_decision_reference", "risk decision references are invalid")
	}
	if !d.Decision.IsValid() || !reasonCodeRegex.MatchString(d.ReasonCode) || containsSensitiveIPRiskText(d.ReasonCode) || d.EvaluatedAt.IsZero() {
		return NewValidationError("invalid_risk_decision", "risk decision is incomplete or reason code is unsafe")
	}
	seen := make(map[string]struct{}, len(d.ObservationDigestSet))
	for _, digest := range d.ObservationDigestSet {
		if !validDigest(digest) {
			return NewValidationError("invalid_risk_decision_digest", "risk decision contains an invalid observation digest")
		}
		if _, exists := seen[digest]; exists {
			return NewValidationError("duplicate_risk_decision_digest", "risk decision observation digests must be unique")
		}
		seen[digest] = struct{}{}
	}
	return nil
}

func validatePercent(name string, value *int) error {
	if value != nil && (*value < 0 || *value > 100) {
		return NewValidationError("invalid_ip_risk_"+name, name+" must be between 0 and 100")
	}
	return nil
}

func validateScoreBands(bands []ScoreBand) error {
	if len(bands) == 0 {
		return NewValidationError("missing_risk_score_bands", "risk policy score bands are required")
	}
	ordered := append([]ScoreBand(nil), bands...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Min < ordered[j].Min })
	if ordered[0].Min != 0 {
		return NewValidationError("incomplete_risk_score_bands", "risk policy score bands must start at zero")
	}
	for i, band := range ordered {
		if band.Min < 0 || band.Max > 100 || band.Min > band.Max || !band.Band.IsValid() ||
			band.Band == RiskBandUnknown || !band.Action.isPolicyAction() {
			return NewValidationError("invalid_risk_score_band", "risk policy score band is invalid")
		}
		if i > 0 && band.Min != ordered[i-1].Max+1 {
			return NewValidationError("overlapping_or_incomplete_risk_score_bands", "risk policy score bands must be contiguous and non-overlapping")
		}
	}
	if ordered[len(ordered)-1].Max != 100 {
		return NewValidationError("incomplete_risk_score_bands", "risk policy score bands must end at one hundred")
	}
	return nil
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func validateIPRiskIdentifier(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return NewValidationError("missing_ip_risk_"+field, fmt.Sprintf("IP risk %s is required", field))
	}
	if len(trimmed) > MaxIPRiskIdentifierLength || !ipRiskIdentifierRegex.MatchString(trimmed) {
		return NewValidationError("invalid_ip_risk_"+field, fmt.Sprintf("IP risk %s format is invalid", field))
	}
	if containsSensitiveIPRiskText(trimmed) {
		return NewSecurityError("sensitive_ip_risk_"+field, fmt.Sprintf("IP risk %s contains sensitive data", field))
	}
	return nil
}

func validateRedactedSummary(summary string) error {
	if len(summary) > MaxIPRiskRedactedSummaryLength {
		return NewValidationError("invalid_ip_risk_summary_length", fmt.Sprintf("IP risk summary length exceeds maximum of %d", MaxIPRiskRedactedSummaryLength))
	}
	if strings.Contains(summary, "{") || strings.Contains(summary, "}") ||
		strings.Contains(summary, "[") || strings.Contains(summary, "]") {
		return NewSecurityError("raw_json_ip_risk_summary", "IP risk summary must not contain raw JSON")
	}
	if containsSensitiveIPRiskText(summary) {
		return NewSecurityError("sensitive_ip_risk_summary", "IP risk summary contains sensitive data")
	}
	return nil
}

func containsSensitiveIPRiskText(value string) bool {
	lower := strings.ToLower(value)
	return containsIPAddress(value) || urlQueryRegex.MatchString(value) || querySecretRegex.MatchString(value) ||
		strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") ||
		strings.Contains(lower, "cookie") || strings.Contains(lower, "raw_payload") ||
		strings.Contains(lower, "raw json") || strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "token=") || strings.Contains(lower, "secret=") ||
		strings.Contains(lower, "password=")
}

func containsIPAddress(value string) bool {
	if match := ipv4Regex.FindStringSubmatch(value); len(match) > 1 && net.ParseIP(match[1]) != nil {
		return true
	}
	for _, match := range ipv6Regex.FindAllString(value, -1) {
		if strings.Count(match, ":") >= 2 && net.ParseIP(strings.Trim(match, ":")) != nil {
			return true
		}
	}
	return false
}
