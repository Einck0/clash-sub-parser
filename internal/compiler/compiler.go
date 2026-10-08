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

// CompileOptions specifies optional compilation arguments.
type CompileOptions struct{}

// CompileOption applies an option to CompileOptions.
type CompileOption func(*CompileOptions)

// CompileMihomo renders a deterministic Mihomo YAML subscription from a resolved snapshot.
func CompileMihomo(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot) (Result, error) {
	return Compile(ctx, snapshot, domain.TargetMihomo)
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
	for i, node := range snapshot.Nodes {
		if err := validateCredentialEnvelope(target, i, node); err != nil {
			return Result{}, err
		}
	}

	content, contentType, filename, err := render(snapshot, target)
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

	if target != domain.TargetMihomo {
		for i, group := range snapshot.Groups {
			if group.UsesEmptyPass() {
				return &CapabilityError{Target: target, Location: fmt.Sprintf("groups[%d]", i), Feature: "empty_fallback_pass", Reason: "target does not support explicit empty-group PASS routing"}
			}
		}
		return nil
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

// validateCredentialEnvelope verifies that a node has a valid endpoint and protocol-complete
// credential fields without leaking sensitive values in error reasons.
func validateCredentialEnvelope(target domain.CompilerTarget, index int, node resolver.ResolvedNode) error {
	loc := fmt.Sprintf("nodes[%d]", index)
	feature := string(node.Protocol)
	if strings.TrimSpace(node.LogicalID) == "" {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "node logical ID is required",
		}
	}
	if !node.Protocol.IsValid() {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "protocol is not supported",
		}
	}
	if strings.TrimSpace(node.Server) == "" {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "missing server address in credential payload",
		}
	}
	if node.Port <= 0 || node.Port > 65535 {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "invalid port in credential payload",
		}
	}

	c := node.Credentials
	if target == domain.TargetSingBox && c.Transport != nil && strings.ToLower(c.Transport["network"]) == "xhttp" {
		return &CapabilityError{
			Target:   domain.TargetSingBox,
			Location: fmt.Sprintf("nodes[%d].transport.network", index),
			Feature:  "xhttp",
			Reason:   "sing-box does not support xhttp transport protocol",
		}
	}
	switch node.Protocol {
	case domain.ProtocolSS:
		if strings.TrimSpace(c.Method) == "" || strings.TrimSpace(c.Password) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required cipher or password in shadowsocks credentials",
			}
		}
	case domain.ProtocolVMess:
		if strings.TrimSpace(c.UUID) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required uuid in vmess credentials",
			}
		}
		if c.AlterID < 0 {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid alterId in vmess credentials",
			}
		}
	case domain.ProtocolVLESS:
		if strings.TrimSpace(c.UUID) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required uuid in vless credentials",
			}
		}
		if c.Transport != nil && strings.TrimSpace(c.Transport["sid"]) != "" && strings.TrimSpace(c.Transport["pbk"]) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required reality public key in vless credentials",
			}
		}
	case domain.ProtocolTrojan:
		if strings.TrimSpace(c.Password) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required password in trojan credentials",
			}
		}
	case domain.ProtocolHysteria2:
		if strings.TrimSpace(c.Password) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required password in hysteria2 credentials",
			}
		}
	case domain.ProtocolWireGuard:
		if strings.TrimSpace(c.PrivateKey) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required private_key in wireguard credentials",
			}
		}
		if strings.TrimSpace(c.PublicKey) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required public_key in wireguard credentials",
			}
		}
		if len(c.LocalAddress) == 0 {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required local_address in wireguard credentials",
			}
		}
		for _, rawAddr := range c.LocalAddress {
			addr := strings.TrimSpace(rawAddr)
			if addr == "" {
				return &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "invalid local_address CIDR in wireguard credentials",
				}
			}
			if _, err := netip.ParsePrefix(addr); err != nil {
				return &CapabilityError{
					Target:   target,
					Location: loc,
					Feature:  feature,
					Reason:   "invalid local_address CIDR in wireguard credentials",
				}
			}
		}
		if len(c.Reserved) > 0 && len(c.Reserved) != 3 {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid reserved bytes in wireguard credentials",
			}
		}
		if c.MTU < 0 || c.MTU > 65535 {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid mtu in wireguard credentials",
			}
		}
	case domain.ProtocolTUIC:
		if strings.TrimSpace(c.UUID) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required uuid in tuic credentials",
			}
		}
		if strings.TrimSpace(c.Password) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required password in tuic credentials",
			}
		}
	case domain.ProtocolHTTP:
		// Username / Password optional, server & port already validated
	case domain.ProtocolSocks5:
		// Username / Password optional, server & port already validated
	case domain.ProtocolAnyTLS:
		if strings.TrimSpace(c.Password) == "" {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   "missing required password in anytls credentials",
			}
		}
	default:
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "protocol is not supported",
		}
	}

	return nil
}

func render(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) ([]byte, string, string, error) {
	switch target {
	case domain.TargetMihomo:
		content, err := renderMihomo(snapshot)
		return content, "application/yaml", "mihomo.yaml", err
	case domain.TargetSingBox:
		content, err := renderSingBox(snapshot)
		return content, "application/json", "sing-box.json", err
	case domain.TargetSurge:
		content, err := renderSurge(snapshot)
		return content, "text/plain; charset=utf-8", "surge.conf", err
	case domain.TargetQuantumultX:
		content, err := renderQuantumultX(snapshot)
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

// CapabilityDiagnostic describes a compiler capability diagnostic for an export target.
type CapabilityDiagnostic struct {
	NodeID  string `json:"node_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Target  string `json:"target,omitempty"`
}

// ValidateTargetCapabilities evaluates all nodes and groups against a target and returns ALL diagnostics.
func ValidateTargetCapabilities(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) []CapabilityDiagnostic {
	if snapshot == nil {
		return nil
	}
	capability, ok := capabilityForTarget(target)
	if !ok {
		return []CapabilityDiagnostic{{
			Code:    "unsupported_target",
			Message: fmt.Sprintf("unknown compiler target: %s", target),
			Target:  string(target),
		}}
	}

	var diags []CapabilityDiagnostic

	// Check each node
	for i, node := range snapshot.Nodes {
		if !capability.Protocols[node.Protocol] {
			diags = append(diags, CapabilityDiagnostic{
				NodeID:  node.LogicalID,
				Code:    "unsupported_target_capability",
				Message: fmt.Sprintf("target %s does not support protocol %s (protocol is not supported)", target, node.Protocol),
				Target:  string(target),
			})
			continue
		}
		if err := validateCredentialEnvelope(target, i, node); err != nil {
			diags = append(diags, CapabilityDiagnostic{
				NodeID:  node.LogicalID,
				Code:    "unsupported_target_capability",
				Message: err.Error(),
				Target:  string(target),
			})
		}
	}

	// PASS routing is native to Mihomo only, not a nodes-only substitution.
	if target != domain.TargetMihomo {
		for _, g := range snapshot.Groups {
			if g.UsesEmptyPass() {
				diags = append(diags, CapabilityDiagnostic{Code: "unsupported_target_capability", Message: "target does not support explicit empty-group PASS routing", Target: string(target)})
			}
		}
	}
	// Target-specific group and rule topology validation
	if target == domain.TargetMihomo {
		groupNameSet := make(map[string]bool, len(snapshot.Groups)*2)
		for _, g := range snapshot.Groups {
			if g.Name != "" {
				groupNameSet[g.Name] = true
			}
			if g.ID != "" {
				groupNameSet[g.ID] = true
			}
		}

		// 1. Check each rendered proxy group: must be non-empty (required_nonempty)
		for _, g := range snapshot.Groups {
			if !g.UsesEmptyPass() && (len(g.Members) == 0 || (g.EmptyFallbackPass && len(g.AllNodeLogicalIDs) == 0)) {
				diags = append(diags, CapabilityDiagnostic{
					Code:    "required_nonempty",
					Message: fmt.Sprintf("proxy group %q has no proxies (required_nonempty)", g.Name),
					Target:  string(target),
				})
			}
			// Check member references to child groups
			for _, m := range g.Members {
				if m.Kind == resolver.MemberKindGroup {
					tID := strings.TrimSpace(m.TargetID)
					tName := strings.TrimSpace(m.DisplayName)
					if (tID != "" && !groupNameSet[tID]) && (tName != "" && !groupNameSet[tName]) {
						diags = append(diags, CapabilityDiagnostic{
							Code:    "dangling_group_reference",
							Message: fmt.Sprintf("group %q references non-existent child group %q", g.Name, tName),
							Target:  string(target),
						})
					}
				}
			}
		}

		// 2. Check each routing rule: target group must exist in rendered groups
		for _, r := range snapshot.Rules {
			tName := strings.TrimSpace(r.TargetGroupName)
			tID := strings.TrimSpace(r.TargetGroupID)
			if tName != "" && tName != "DIRECT" && tName != "REJECT" && tName != "REJECT-DROP" && tName != "PASS" {
				if !groupNameSet[tName] && !groupNameSet[tID] {
					diags = append(diags, CapabilityDiagnostic{
						Code:    "dangling_rule_target",
						Message: fmt.Sprintf("rule %q targets non-existent group %q", r.Expression, tName),
						Target:  string(target),
					})
				}
			}
		}
	}

	return diags
}

// FilterCompatibleSnapshot creates a new ResolvedPolicySnapshot with incompatible nodes excluded.
func FilterCompatibleSnapshot(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) (*resolver.ResolvedPolicySnapshot, []domain.ManifestExcludedNode) {
	if snapshot == nil {
		return nil, nil
	}
	capability, ok := capabilityForTarget(target)
	if !ok {
		return snapshot, nil
	}

	var admittedNodes []resolver.ResolvedNode
	var excludedNodes []domain.ManifestExcludedNode
	admittedSet := make(map[string]bool)

	for i, node := range snapshot.Nodes {
		var reason string
		if !capability.Protocols[node.Protocol] {
			reason = fmt.Sprintf("target %s does not support protocol %s", target, node.Protocol)
		} else if err := validateCredentialEnvelope(target, i, node); err != nil {
			reason = err.Error()
		}

		if reason != "" {
			excludedNodes = append(excludedNodes, domain.ManifestExcludedNode{
				NodeID: node.LogicalID,
				Code:   "unsupported_target_capability",
				Reason: reason,
			})
		} else {
			admittedNodes = append(admittedNodes, node)
			admittedSet[node.LogicalID] = true
			admittedSet[node.DisplayName] = true
		}
	}

	// Build filtered groups
	filteredGroups := make([]resolver.ResolvedGroup, len(snapshot.Groups))
	for gi, group := range snapshot.Groups {
		var filteredMembers []resolver.ResolvedGroupMember
		for _, member := range group.Members {
			if member.Kind == resolver.MemberKindNode {
				if admittedSet[member.TargetID] || admittedSet[member.DisplayName] {
					filteredMembers = append(filteredMembers, member)
				}
			} else {
				// Keep group or built-in target references
				filteredMembers = append(filteredMembers, member)
			}
		}
		filteredGroups[gi] = group
		filteredGroups[gi].Members = filteredMembers
	}

	copySnap := *snapshot
	copySnap.Nodes = admittedNodes
	copySnap.Groups = filteredGroups

	return &copySnap, excludedNodes
}

// SortedCapabilities exposes target names without leaking the backing map.
func SortedCapabilities() []domain.CompilerTarget {
	result := Targets()
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
