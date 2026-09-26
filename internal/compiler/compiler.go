// Package compiler renders one resolved policy snapshot for a named client target.
package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

// Capability describes the semantics accepted by a target renderer.
type Capability struct {
	Protocols  map[domain.Protocol]bool
	GroupTypes map[domain.GroupType]bool
	RuleKinds  map[string]bool
}

// CapabilityError identifies the exact snapshot location that cannot be rendered.
type CapabilityError struct {
	Target   domain.CompilerTarget
	Location string
	Feature  string
	Reason   string
}

func (e *CapabilityError) Error() string {
	return fmt.Sprintf("target %s cannot render %s at %s: %s", e.Target, e.Feature, e.Location, e.Reason)
}

// Result is an immutable renderer result with both input and content digests.
type Result struct {
	Content        []byte
	ContentType    string
	Filename       string
	Target         domain.CompilerTarget
	SnapshotDigest string
	ContentDigest  string
}

// Targets returns the supported targets in stable order.
func Targets() []domain.CompilerTarget {
	return []domain.CompilerTarget{domain.TargetMihomo, domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX}
}

func capabilityForTarget(target domain.CompilerTarget) (Capability, bool) {
	switch target {
	case domain.TargetMihomo:
		return mihomoCapability(), true
	case domain.TargetSingBox:
		return singBoxCapability(), true
	case domain.TargetSurge:
		return surgeCapability(), true
	case domain.TargetQuantumultX:
		return quantumultXCapability(), true
	default:
		return Capability{}, false
	}
}

// CapabilityMatrix returns a deep copy so callers cannot mutate compiler policy.
func CapabilityMatrix() map[domain.CompilerTarget]Capability {
	targets := Targets()
	copyMatrix := make(map[domain.CompilerTarget]Capability, len(targets))
	for _, target := range targets {
		capability, ok := capabilityForTarget(target)
		if !ok {
			continue
		}
		copyMatrix[target] = Capability{
			Protocols:  copyProtocolSet(capability.Protocols),
			GroupTypes: copyGroupSet(capability.GroupTypes),
			RuleKinds:  copyStringSet(capability.RuleKinds),
		}
	}
	return copyMatrix
}

// NodeCredentialInput binds a resolved node to its authenticated decrypted credential payload.
type NodeCredentialInput struct {
	Node       resolver.ResolvedNode
	Credential *domain.NodeCredentialPayload
}

// CompileOptions specifies optional compilation arguments, such as authenticated credentials.
type CompileOptions struct {
	Credentials map[string]*domain.NodeCredentialPayload
}

// CompileOption applies an option to CompileOptions.
type CompileOption func(*CompileOptions)

// WithCredentials configures authenticated node credentials for compilation.
func WithCredentials(credentials map[string]*domain.NodeCredentialPayload) CompileOption {
	return func(opts *CompileOptions) {
		opts.Credentials = credentials
	}
}

// WithCredentialInputs configures authenticated node credentials from a list of inputs.
func WithCredentialInputs(inputs []NodeCredentialInput) CompileOption {
	return func(opts *CompileOptions) {
		if opts.Credentials == nil {
			opts.Credentials = make(map[string]*domain.NodeCredentialPayload, len(inputs))
		}
		for _, input := range inputs {
			if input.Credential != nil {
				opts.Credentials[input.Node.LogicalID] = input.Credential
			}
		}
	}
}

// CompileMihomo renders a deterministic Mihomo YAML subscription from a resolved snapshot
// and authenticated node credentials. Missing or invalid credentials fail closed.
func CompileMihomo(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot, credentials map[string]*domain.NodeCredentialPayload) (Result, error) {
	return Compile(ctx, snapshot, domain.TargetMihomo, WithCredentials(credentials))
}

// Compile validates target capabilities before rendering a deterministic result.
func Compile(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget, opts ...CompileOption) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if snapshot == nil {
		return Result{}, fmt.Errorf("resolved policy snapshot is required")
	}
	capability, ok := capabilityForTarget(target)
	if !ok {
		return Result{}, &CapabilityError{Target: target, Location: "target", Feature: string(target), Reason: "unknown compiler target"}
	}
	if err := validate(snapshot, target, capability); err != nil {
		return Result{}, err
	}

	var options CompileOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.Credentials != nil {
		for i, node := range snapshot.Nodes {
			if _, err := validateCredentialEnvelope(target, i, node, options.Credentials); err != nil {
				return Result{}, err
			}
		}
	}

	content, contentType, filename, err := render(snapshot, target, options.Credentials)
	if err != nil {
		return Result{}, err
	}
	hash := sha256.Sum256(content)
	return Result{
		Content: append([]byte(nil), content...), ContentType: contentType, Filename: filename,
		Target: target, SnapshotDigest: snapshot.SnapshotDigest, ContentDigest: hex.EncodeToString(hash[:]),
	}, nil
}

func isBuiltInPolicyTarget(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE":
		return true
	default:
		return false
	}
}

func validate(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget, capability Capability) error {
	nodeIDs := make(map[string]bool, len(snapshot.Nodes))
	nodeNames := make(map[string]bool, len(snapshot.Nodes))
	for i, node := range snapshot.Nodes {
		loc := fmt.Sprintf("nodes[%d]", i)
		if !capability.Protocols[node.Protocol] {
			return &CapabilityError{Target: target, Location: loc, Feature: string(node.Protocol), Reason: "protocol is not supported"}
		}
		if strings.TrimSpace(node.LogicalID) == "" {
			return &CapabilityError{Target: target, Location: loc, Feature: string(node.Protocol), Reason: "node logical ID is required"}
		}
		if strings.TrimSpace(node.DisplayName) == "" {
			return &CapabilityError{Target: target, Location: loc, Feature: string(node.Protocol), Reason: "node display name is required"}
		}
		nodeIDs[node.LogicalID] = true
		nodeNames[node.DisplayName] = true
	}

	groupIDs := make(map[string]bool, len(snapshot.Groups))
	groupNames := make(map[string]bool, len(snapshot.Groups))
	for i, group := range snapshot.Groups {
		loc := fmt.Sprintf("groups[%d]", i)
		if !capability.GroupTypes[group.GroupType] {
			return &CapabilityError{Target: target, Location: loc, Feature: string(group.GroupType), Reason: "policy group type is not supported"}
		}
		if strings.TrimSpace(group.Name) == "" {
			return &CapabilityError{Target: target, Location: loc, Feature: string(group.GroupType), Reason: "policy group name is required"}
		}
		if strings.TrimSpace(group.ID) != "" {
			groupIDs[group.ID] = true
		}
		groupNames[group.Name] = true
	}

	for i, group := range snapshot.Groups {
		loc := fmt.Sprintf("groups[%d]", i)
		for _, member := range group.Members {
			memberName := strings.TrimSpace(member.DisplayName)
			if memberName == "" {
				return &CapabilityError{Target: target, Location: loc, Feature: group.Name, Reason: "group member display name is required"}
			}
			switch member.Kind {
			case resolver.MemberKindNode:
				if (strings.TrimSpace(member.TargetID) != "" && !nodeIDs[member.TargetID]) || !nodeNames[memberName] {
					return &CapabilityError{Target: target, Location: loc, Feature: memberName, Reason: "group member references unknown node"}
				}
			case resolver.MemberKindGroup:
				if (strings.TrimSpace(member.TargetID) != "" && !groupIDs[member.TargetID]) || !groupNames[memberName] {
					return &CapabilityError{Target: target, Location: loc, Feature: memberName, Reason: "group member references unknown policy group"}
				}
			case "":
				if !nodeNames[memberName] && !groupNames[memberName] && !isBuiltInPolicyTarget(memberName) {
					return &CapabilityError{Target: target, Location: loc, Feature: memberName, Reason: "group member references unknown target"}
				}
			default:
				return &CapabilityError{Target: target, Location: loc, Feature: string(member.Kind), Reason: "unsupported group member kind"}
			}
		}
	}

	for i, rule := range snapshot.Rules {
		loc := fmt.Sprintf("rules[%d]", i)
		expr := strings.TrimSpace(rule.Expression)
		kind := ruleKind(expr)
		if kind == "" {
			return &CapabilityError{Target: target, Location: loc, Feature: "rule", Reason: "routing rule expression is required"}
		}
		if !capability.RuleKinds[kind] {
			return &CapabilityError{Target: target, Location: loc, Feature: kind, Reason: "routing rule kind is not supported"}
		}
		if !domain.IsMatchRule(expr) {
			parts := strings.Split(expr, ",")
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				return &CapabilityError{Target: target, Location: loc, Feature: kind, Reason: "routing rule value is required"}
			}
		}
		targetName := strings.TrimSpace(rule.TargetGroupName)
		if targetName == "" {
			return &CapabilityError{Target: target, Location: loc, Feature: kind, Reason: "routing rule target group is required"}
		}
		if !isBuiltInPolicyTarget(targetName) {
			if len(snapshot.Groups) == 0 || !groupNames[targetName] {
				return &CapabilityError{Target: target, Location: loc, Feature: targetName, Reason: "routing rule references unknown target group"}
			}
			if strings.TrimSpace(rule.TargetGroupID) != "" && len(groupIDs) > 0 && !groupIDs[rule.TargetGroupID] {
				return &CapabilityError{Target: target, Location: loc, Feature: targetName, Reason: "routing rule target group ID mismatch"}
			}
		}
	}
	return nil
}

func ruleKind(expression string) string {
	kind := strings.ToUpper(strings.TrimSpace(expression))
	if comma := strings.IndexByte(kind, ','); comma >= 0 {
		kind = kind[:comma]
	}
	if space := strings.IndexByte(kind, ' '); space >= 0 {
		kind = kind[:space]
	}
	return kind
}

// validateCredentialEnvelope verifies that a node has a matching, non-empty credential payload envelope
// and protocol-complete credential fields without leaking sensitive values in error reasons.
func validateCredentialEnvelope(target domain.CompilerTarget, index int, node resolver.ResolvedNode, credentials map[string]*domain.NodeCredentialPayload) (*domain.NodeCredentialPayload, error) {
	loc := fmt.Sprintf("nodes[%d]", index)
	feature := string(node.Protocol)
	if credentials == nil {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   fmt.Sprintf("node credentials are required for %s compilation (fail closed)", target),
		}
	}
	cred, ok := credentials[node.LogicalID]
	if !ok || cred == nil {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   fmt.Sprintf("missing credential payload for node %s", node.DisplayName),
		}
	}
	if strings.TrimSpace(node.LogicalID) == "" || cred.LogicalID != node.LogicalID {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "credential payload logical ID mismatch",
		}
	}
	if !node.Protocol.IsValid() || cred.Protocol != node.Protocol {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "credential payload protocol mismatch",
		}
	}
	if cred.Version < 0 {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "invalid version in credential payload",
		}
	}
	if strings.TrimSpace(cred.Server) == "" {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "missing server address in credential payload",
		}
	}
	if cred.Port <= 0 || cred.Port > 65535 {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "invalid port in credential payload",
		}
	}
	if node.CredentialVersion > 0 && cred.Version != node.CredentialVersion {
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "credential payload version mismatch",
		}
	}

	requiresVerifiedIdentity := node.Identity != nil || cred.Identity != nil ||
		(strings.HasPrefix(node.LogicalID, "node_") && domain.IsValidLogicalID(node.LogicalID))
	if requiresVerifiedIdentity {
		if cred.Identity == nil {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing verified identity binding in credential payload",
			}
		}
		if err := cred.Identity.ValidateNonEmpty(); err != nil {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid verified identity binding in credential payload",
			}
		}
		if cred.Identity.LogicalID != node.LogicalID {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "credential identity binding logical ID mismatch",
			}
		}
		if cred.Identity.Protocol != node.Protocol {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "credential identity binding protocol mismatch",
			}
		}
		if cred.Identity.Version != cred.Version {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "credential identity binding version mismatch",
			}
		}
		if !cred.Identity.MatchesEndpoint(cred.Protocol, cred.Server, cred.Port) {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "credential identity binding endpoint mismatch",
			}
		}
		if strings.TrimSpace(cred.Identity.TransportDigest) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing transport digest in credential identity binding",
			}
		}
		if cred.Credentials.Transport != nil && cred.Identity.TransportDigest != domain.CanonicalTransportDigest(cred.Credentials.Transport) {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "credential identity binding transport digest mismatch",
			}
		}

		if node.Identity != nil {
			if err := node.Identity.ValidateNonEmpty(); err != nil {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "invalid verified identity on resolved node",
				}
			}
			if node.Identity.LogicalID != node.LogicalID || node.Identity.Protocol != node.Protocol {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "resolved node identity mismatch",
				}
			}
			if node.Identity.Version != cred.Version {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "credential payload version mismatch with node identity",
				}
			}
			if !node.Identity.MatchesEndpoint(cred.Protocol, cred.Server, cred.Port) ||
				!node.Identity.MatchesEndpoint(cred.Identity.Protocol, cred.Identity.Server, cred.Identity.Port) {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "credential endpoint does not match verified node identity",
				}
			}
			if strings.TrimSpace(node.Identity.TransportDigest) == "" ||
				node.Identity.TransportDigest != cred.Identity.TransportDigest {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "credential transport digest does not match verified node identity",
				}
			}
		}

		if strings.HasPrefix(node.LogicalID, "node_") && domain.IsValidLogicalID(node.LogicalID) {
			idWithTransport := domain.ComputeNodeLogicalID(cred.Protocol, cred.Server, cred.Port, cred.Credentials.Transport)
			idWithoutTransport := domain.ComputeNodeLogicalID(cred.Protocol, cred.Server, cred.Port, nil)
			if idWithTransport != node.LogicalID && (node.Identity != nil || idWithoutTransport != node.LogicalID) {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "credential endpoint does not match node logical identity",
				}
			}
		}
	}

	c := cred.Credentials
	switch node.Protocol {
	case domain.ProtocolSS:
		if strings.TrimSpace(c.Method) == "" || strings.TrimSpace(c.Password) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required cipher or password in shadowsocks credentials",
			}
		}
	case domain.ProtocolVMess:
		if strings.TrimSpace(c.UUID) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required uuid in vmess credentials",
			}
		}
		if c.AlterID < 0 {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid alterId in vmess credentials",
			}
		}
	case domain.ProtocolVLESS:
		if strings.TrimSpace(c.UUID) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required uuid in vless credentials",
			}
		}
		if c.Transport != nil && strings.TrimSpace(c.Transport["sid"]) != "" && strings.TrimSpace(c.Transport["pbk"]) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required reality public key in vless credentials",
			}
		}
	case domain.ProtocolTrojan:
		if strings.TrimSpace(c.Password) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required password in trojan credentials",
			}
		}
	case domain.ProtocolHysteria2:
		if strings.TrimSpace(c.Password) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required password in hysteria2 credentials",
			}
		}
	case domain.ProtocolWireGuard:
		if strings.TrimSpace(c.PrivateKey) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required private_key in wireguard credentials",
			}
		}
		if strings.TrimSpace(c.PublicKey) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required public_key in wireguard credentials",
			}
		}
		if len(c.LocalAddress) == 0 {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required local_address in wireguard credentials",
			}
		}
		for _, rawAddr := range c.LocalAddress {
			addr := strings.TrimSpace(rawAddr)
			if addr == "" {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "invalid local_address CIDR in wireguard credentials",
				}
			}
			if _, err := netip.ParsePrefix(addr); err != nil {
				return nil, &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "invalid local_address CIDR in wireguard credentials",
				}
			}
		}
		if len(c.Reserved) > 0 && len(c.Reserved) != 3 {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid reserved bytes in wireguard credentials",
			}
		}
		if c.MTU < 0 || c.MTU > 65535 {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid mtu in wireguard credentials",
			}
		}
	case domain.ProtocolTUIC:
		if strings.TrimSpace(c.UUID) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required uuid in tuic credentials",
			}
		}
		if strings.TrimSpace(c.Password) == "" {
			return nil, &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required password in tuic credentials",
			}
		}
	default:
		return nil, &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "protocol is not supported",
		}
	}

	return cred, nil
}

func render(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget, credentials map[string]*domain.NodeCredentialPayload) ([]byte, string, string, error) {
	switch target {
	case domain.TargetMihomo:
		content, err := renderMihomo(snapshot, credentials)
		return content, "application/yaml", "mihomo.yaml", err
	case domain.TargetSingBox:
		content, err := renderSingBox(snapshot, credentials)
		return content, "application/json", "sing-box.json", err
	case domain.TargetSurge:
		content, err := renderSurge(snapshot, credentials)
		return content, "text/plain; charset=utf-8", "surge.conf", err
	case domain.TargetQuantumultX:
		content, err := renderQuantumultX(snapshot, credentials)
		return content, "text/plain; charset=utf-8", "quantumult-x.conf", err
	default:
		return nil, "", "", fmt.Errorf("unknown compiler target %s", target)
	}
}

func protocolSet(values ...domain.Protocol) map[domain.Protocol]bool {
	result := make(map[domain.Protocol]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func groupSet(values ...domain.GroupType) map[domain.GroupType]bool {
	result := make(map[domain.GroupType]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func ruleSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func copyProtocolSet(values map[domain.Protocol]bool) map[domain.Protocol]bool {
	result := make(map[domain.Protocol]bool, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func copyGroupSet(values map[domain.GroupType]bool) map[domain.GroupType]bool {
	result := make(map[domain.GroupType]bool, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func copyStringSet(values map[string]bool) map[string]bool {
	result := make(map[string]bool, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

// SortedCapabilities exposes target names without leaking the backing map.
func SortedCapabilities() []domain.CompilerTarget {
	result := Targets()
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
