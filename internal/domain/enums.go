package domain

import (
	"fmt"
	"strings"
)

// Protocol represents the supported node transport protocols.
type Protocol string

const (
	ProtocolSS        Protocol = "ss"
	ProtocolVMess     Protocol = "vmess"
	ProtocolVLESS     Protocol = "vless"
	ProtocolTrojan    Protocol = "trojan"
	ProtocolHysteria2 Protocol = "hysteria2"
	ProtocolWireGuard Protocol = "wireguard"
	ProtocolTUIC      Protocol = "tuic"
)

var validProtocols = map[Protocol]bool{
	ProtocolSS:        true,
	ProtocolVMess:     true,
	ProtocolVLESS:     true,
	ProtocolTrojan:    true,
	ProtocolHysteria2: true,
	ProtocolWireGuard: true,
	ProtocolTUIC:      true,
}

func (p Protocol) IsValid() bool {
	return validProtocols[p]
}

func ParseProtocol(s string) (Protocol, error) {
	p := Protocol(strings.ToLower(strings.TrimSpace(s)))
	if !p.IsValid() {
		return "", NewValidationError("invalid_protocol", fmt.Sprintf("unsupported protocol: %s", s))
	}
	return p, nil
}

// ProbeVerdict represents the evidence-based evaluation of a node capability probe.
type ProbeVerdict string

const (
	VerdictAvailable  ProbeVerdict = "available"
	VerdictRestricted ProbeVerdict = "restricted"
	VerdictUnknown    ProbeVerdict = "unknown"
	VerdictError      ProbeVerdict = "error"
	VerdictStale      ProbeVerdict = "stale"
)

var validVerdicts = map[ProbeVerdict]bool{
	VerdictAvailable:  true,
	VerdictRestricted: true,
	VerdictUnknown:    true,
	VerdictError:      true,
	VerdictStale:      true,
}

func (v ProbeVerdict) IsValid() bool {
	return validVerdicts[v]
}

func ParseProbeVerdict(s string) (ProbeVerdict, error) {
	v := ProbeVerdict(strings.ToLower(strings.TrimSpace(s)))
	if !v.IsValid() {
		return "", NewValidationError("invalid_probe_verdict", fmt.Sprintf("unsupported probe verdict: %s", s))
	}
	return v, nil
}

// ProbeRunState represents the lifecycle states of a probe run job.
type ProbeRunState string

const (
	ProbeRunStateQueued    ProbeRunState = "queued"
	ProbeRunStateRunning   ProbeRunState = "running"
	ProbeRunStateSucceeded ProbeRunState = "succeeded"
	ProbeRunStateFailed    ProbeRunState = "failed"
	ProbeRunStateCancelled ProbeRunState = "cancelled"
	ProbeRunStateExpired   ProbeRunState = "expired"
)

var validProbeRunStates = map[ProbeRunState]bool{
	ProbeRunStateQueued:    true,
	ProbeRunStateRunning:   true,
	ProbeRunStateSucceeded: true,
	ProbeRunStateFailed:    true,
	ProbeRunStateCancelled: true,
	ProbeRunStateExpired:   true,
}

func (s ProbeRunState) IsValid() bool {
	return validProbeRunStates[s]
}

func (s ProbeRunState) IsTerminal() bool {
	return s == ProbeRunStateSucceeded || s == ProbeRunStateFailed ||
		s == ProbeRunStateCancelled || s == ProbeRunStateExpired
}

func ParseProbeRunState(s string) (ProbeRunState, error) {
	st := ProbeRunState(strings.ToLower(strings.TrimSpace(s)))
	if !st.IsValid() {
		return "", NewValidationError("invalid_probe_run_state", fmt.Sprintf("unsupported probe run state: %s", s))
	}
	return st, nil
}

// ProbeKind represents the category of capability probe.
type ProbeKind string

const (
	ProbeKindBaseline  ProbeKind = "baseline"
	ProbeKindGeo       ProbeKind = "geo"
	ProbeKindStreaming ProbeKind = "streaming"
	ProbeKindAI        ProbeKind = "ai"
	ProbeKindSpeed     ProbeKind = "speed"
	ProbeKindIPRisk    ProbeKind = "ip_risk"
)

var validProbeKinds = map[ProbeKind]bool{
	ProbeKindBaseline:  true,
	ProbeKindGeo:       true,
	ProbeKindStreaming: true,
	ProbeKindAI:        true,
	ProbeKindSpeed:     true,
	ProbeKindIPRisk:    true,
}

func (k ProbeKind) IsValid() bool {
	return validProbeKinds[k]
}

func ParseProbeKind(s string) (ProbeKind, error) {
	k := ProbeKind(strings.ToLower(strings.TrimSpace(s)))
	if !k.IsValid() {
		return "", NewValidationError("invalid_probe_kind", fmt.Sprintf("unsupported probe kind: %s", s))
	}
	return k, nil
}

// CompilerTarget represents the export configuration targets.
type CompilerTarget string

const (
	TargetClash       CompilerTarget = "clash"
	TargetMihomo      CompilerTarget = "mihomo"
	TargetSingBox     CompilerTarget = "singbox"
	TargetSurge       CompilerTarget = "surge"
	TargetQuantumultX CompilerTarget = "qx"
)

var validCompilerTargets = map[CompilerTarget]bool{
	TargetClash:       true,
	TargetMihomo:      true,
	TargetSingBox:     true,
	TargetSurge:       true,
	TargetQuantumultX: true,
}

func (t CompilerTarget) IsValid() bool {
	return validCompilerTargets[t]
}

func ParseCompilerTarget(s string) (CompilerTarget, error) {
	t := CompilerTarget(strings.ToLower(strings.TrimSpace(s)))
	if !t.IsValid() {
		return "", NewValidationError("invalid_compiler_target", fmt.Sprintf("unsupported compiler target: %s", s))
	}
	return t, nil
}

// ConfigurationRevisionState represents the lifecycle state of a configuration revision.
type ConfigurationRevisionState string

const (
	RevisionStateDraft    ConfigurationRevisionState = "draft"
	RevisionStateActive   ConfigurationRevisionState = "active"
	RevisionStateArchived ConfigurationRevisionState = "archived"
)

var validRevisionStates = map[ConfigurationRevisionState]bool{
	RevisionStateDraft:    true,
	RevisionStateActive:   true,
	RevisionStateArchived: true,
}

func (s ConfigurationRevisionState) IsValid() bool {
	return validRevisionStates[s]
}

func ParseRevisionState(s string) (ConfigurationRevisionState, error) {
	st := ConfigurationRevisionState(strings.ToLower(strings.TrimSpace(s)))
	if !st.IsValid() {
		return "", NewValidationError("invalid_revision_state", fmt.Sprintf("unsupported revision state: %s", s))
	}
	return st, nil
}

// PublicationState represents whether a publication is active or revoked.
type PublicationState string

const (
	PublicationStateActive  PublicationState = "active"
	PublicationStateRevoked PublicationState = "revoked"
)

var validPublicationStates = map[PublicationState]bool{
	PublicationStateActive:  true,
	PublicationStateRevoked: true,
}

func (s PublicationState) IsValid() bool {
	return validPublicationStates[s]
}

func ParsePublicationState(s string) (PublicationState, error) {
	st := PublicationState(strings.ToLower(strings.TrimSpace(s)))
	if !st.IsValid() {
		return "", NewValidationError("invalid_publication_state", fmt.Sprintf("unsupported publication state: %s", s))
	}
	return st, nil
}

// FetchOutcome represents the outcome of a subscription fetch.
type FetchOutcome string

const (
	FetchOutcomeSuccess FetchOutcome = "success"
	FetchOutcomeFailed  FetchOutcome = "failed"
	FetchOutcomePartial FetchOutcome = "partial"
)

var validFetchOutcomes = map[FetchOutcome]bool{
	FetchOutcomeSuccess: true,
	FetchOutcomeFailed:  true,
	FetchOutcomePartial: true,
}

func (o FetchOutcome) IsValid() bool {
	return validFetchOutcomes[o]
}

func ParseFetchOutcome(s string) (FetchOutcome, error) {
	fo := FetchOutcome(strings.ToLower(strings.TrimSpace(s)))
	if !fo.IsValid() {
		return "", NewValidationError("invalid_fetch_outcome", fmt.Sprintf("unsupported fetch outcome: %s", s))
	}
	return fo, nil
}

// GroupType represents policy group types.
type GroupType string

const (
	GroupTypeSelect      GroupType = "select"
	GroupTypeURLTest     GroupType = "urltest"
	GroupTypeFallback    GroupType = "fallback"
	GroupTypeLoadBalance GroupType = "loadbalance"
)

var validGroupTypes = map[GroupType]bool{
	GroupTypeSelect:      true,
	GroupTypeURLTest:     true,
	GroupTypeFallback:    true,
	GroupTypeLoadBalance: true,
}

func (g GroupType) IsValid() bool {
	return validGroupTypes[g]
}

func ParseGroupType(s string) (GroupType, error) {
	gt := GroupType(strings.ToLower(strings.TrimSpace(s)))
	if !gt.IsValid() {
		return "", NewValidationError("invalid_group_type", fmt.Sprintf("unsupported group type: %s", s))
	}
	return gt, nil
}

// RuleAction represents actions for admission or policy rules.
type RuleAction string

const (
	RuleActionAllow      RuleAction = "allow"
	RuleActionReject     RuleAction = "reject"
	RuleActionQuarantine RuleAction = "quarantine"
)

var validRuleActions = map[RuleAction]bool{
	RuleActionAllow:      true,
	RuleActionReject:     true,
	RuleActionQuarantine: true,
}

func (a RuleAction) IsValid() bool {
	return validRuleActions[a]
}

func ParseRuleAction(s string) (RuleAction, error) {
	ra := RuleAction(strings.ToLower(strings.TrimSpace(s)))
	if !ra.IsValid() {
		return "", NewValidationError("invalid_rule_action", fmt.Sprintf("unsupported rule action: %s", s))
	}
	return ra, nil
}

// ActorKind represents the initiator of an audit event.
type ActorKind string

const (
	ActorKindAdmin     ActorKind = "admin"
	ActorKindSystem    ActorKind = "system"
	ActorKindWorker    ActorKind = "worker"
	ActorKindAnonymous ActorKind = "anonymous"
)

var validActorKinds = map[ActorKind]bool{
	ActorKindAdmin:     true,
	ActorKindSystem:    true,
	ActorKindWorker:    true,
	ActorKindAnonymous: true,
}

func (a ActorKind) IsValid() bool {
	return validActorKinds[a]
}

func ParseActorKind(s string) (ActorKind, error) {
	ak := ActorKind(strings.ToLower(strings.TrimSpace(s)))
	if !ak.IsValid() {
		return "", NewValidationError("invalid_actor_kind", fmt.Sprintf("unsupported actor kind: %s", s))
	}
	return ak, nil
}

// AuditResult represents the result of an audited action.
type AuditResult string

const (
	AuditResultSuccess AuditResult = "success"
	AuditResultFailure AuditResult = "failure"
)

var validAuditResults = map[AuditResult]bool{
	AuditResultSuccess: true,
	AuditResultFailure: true,
}

func (r AuditResult) IsValid() bool {
	return validAuditResults[r]
}

func ParseAuditResult(s string) (AuditResult, error) {
	ar := AuditResult(strings.ToLower(strings.TrimSpace(s)))
	if !ar.IsValid() {
		return "", NewValidationError("invalid_audit_result", fmt.Sprintf("unsupported audit result: %s", s))
	}
	return ar, nil
}
