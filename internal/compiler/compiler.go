// Package compiler renders one resolved policy snapshot for a named client target.
package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	probesingbox "clash-sub-parser/internal/probe/singbox"
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

// CompileMihomo renders a deterministic Mihomo YAML subscription from a resolved snapshot.
func CompileMihomo(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot) (Result, error) {
	return Compile(ctx, snapshot, domain.TargetMihomo)
}

// Compile validates target capabilities before rendering a deterministic result.
func Compile(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) (Result, error) {
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
	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.Code == "invalid_group_topology" && diagnostic.Severity == resolver.DiagnosticSeverityError {
			return &CapabilityError{Target: target, Location: "groups", Feature: "member", Reason: diagnostic.Message}
		}
	}
	if target == domain.TargetMihomo {
		if diagnostics := chainTopologyDiagnostics(snapshot); len(diagnostics) > 0 {
			return &CapabilityError{Target: target, Location: "nodes", Feature: "dialer-proxy", Reason: diagnostics[0].Message}
		}
	}
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
		if target != domain.TargetMihomo && strings.TrimSpace(node.DialerProxy) != "" {
			return &CapabilityError{Target: target, Location: loc, Feature: "dialer-proxy", Reason: "dialer-proxy is only supported on mihomo target"}
		}
		if target != domain.TargetMihomo && hasConfiguredPlugin(node.Credentials) {
			return &CapabilityError{Target: target, Location: loc, Feature: "plugin", Reason: fmt.Sprintf("target %s does not support plugin or plugin-opts", target)}
		}
		trimmedName := strings.TrimSpace(node.DisplayName)
		if nodeNames[trimmedName] {
			return &CapabilityError{Target: target, Location: loc, Feature: node.DisplayName, Reason: fmt.Sprintf("duplicate node display name %q", node.DisplayName)}
		}
		nodeIDs[node.LogicalID] = true
		nodeNames[trimmedName] = true
		nodeNames[node.DisplayName] = true
	}

	if target != domain.TargetMihomo {
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
			case resolver.MemberKindBuiltin:
				if !isBuiltInPolicyTarget(memberName) {
					return &CapabilityError{Target: target, Location: loc, Feature: memberName, Reason: fmt.Sprintf("unsupported builtin policy target %q", memberName)}
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

func isSensitiveKey(kLower string) bool {
	return strings.Contains(kLower, "password") ||
		strings.Contains(kLower, "secret") ||
		strings.Contains(kLower, "psk") ||
		strings.Contains(kLower, "preshared") ||
		strings.Contains(kLower, "token") ||
		strings.Contains(kLower, "auth") ||
		strings.Contains(kLower, "private") ||
		strings.Contains(kLower, "key") ||
		strings.Contains(kLower, "uuid") ||
		strings.Contains(kLower, "username")
}

// extractPluginSecrets uses the exact renderer syntax and scalar conversion.
// Incomplete/opaque values cannot authorize a diagnostic display name.
func extractPluginSecrets(raw string, addSecret func(string)) bool {
	options, err := parsePluginOptionMap("plugin-opts", raw)
	if err != nil {
		return false
	}
	complete := true
	for key, value := range options {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "headers" {
			headers, err := parsePluginHeaders("plugin-opts", "v2ray-plugin", value)
			if err != nil {
				complete = false
				continue
			}
			for _, header := range headers {
				addSecret(header)
				// Header authentication grammars are opaque here. A whole header
				// value is not proof that every embedded credential was extracted.
				if strings.TrimSpace(header) != "" {
					complete = false
				}
			}
			continue
		}
		scalar, err := parsePluginString("plugin-opts", "shadowsocks", key, value)
		if err != nil {
			// Never stringify objects/arrays/null that the scalar renderer rejects.
			complete = false
			continue
		}
		if isSensitiveKey(key) {
			addSecret(scalar)
		}
	}
	return complete
}

// CollectCredentialSecrets returns credential values for diagnostic redaction,
// including normalized secrets in plugin-opts/plugin_opts. The second result
// reports whether the option values are fully understood for safe display;
// callers must omit labels when it is false. It does NOT authorize rendering:
// schema, alias conflict and target checks remain the renderer's responsibility.
// Callers must never expose the returned secrets.
func CollectCredentialSecrets(creds domain.InboundProtocolCredential) ([]string, bool) {
	var secrets []string
	complete := true
	addSecret := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			secrets = append(secrets, s)
		}
	}

	addSecret(creds.Password)
	addSecret(creds.UUID)
	addSecret(creds.Username)
	addSecret(creds.PrivateKey)
	addSecret(creds.PublicKey)
	addSecret(creds.EffectivePreSharedKey())
	addSecret(creds.PreSharedKey)
	addSecret(creds.PresharedKey)

	if creds.Transport != nil {
		for k, v := range creds.Transport {
			vTrim := strings.TrimSpace(v)
			if vTrim == "" {
				continue
			}
			kLower := strings.ToLower(strings.TrimSpace(k))
			if isSensitiveKey(kLower) {
				addSecret(vTrim)
			}
			if kLower == "plugin-opts" || kLower == "plugin_opts" {
				if !extractPluginSecrets(vTrim, addSecret) {
					complete = false
				}
			}
		}
	}

	return secrets, complete
}

func sanitizeReason(reason string, creds domain.InboundProtocolCredential) string {
	if reason == "" {
		return reason
	}
	secrets, complete := CollectCredentialSecrets(creds)
	if !complete {
		// Never echo an unproven reason when option decoding was incomplete.
		// Shared parser errors have fixed, input-free text; retain that useful
		// syntax diagnosis instead of inventing a tolerant second parser.
		for _, alias := range []string{"plugin-opts", "plugin_opts"} {
			if raw := strings.TrimSpace(creds.Transport[alias]); raw != "" {
				if _, err := parsePluginOptionMap("plugin-opts", raw); err != nil {
					var capErr *CapabilityError
					if errors.As(err, &capErr) {
						return capErr.Reason
					}
				}
			}
		}
		return "invalid or opaque shadowsocks plugin options; diagnostic details omitted"
	}
	sort.Slice(secrets, func(i, j int) bool {
		return len(secrets[i]) > len(secrets[j])
	})

	seen := make(map[string]bool, len(secrets))
	for _, s := range secrets {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true

		if len(trimmed) >= 4 {
			if strings.Contains(reason, trimmed) {
				reason = strings.ReplaceAll(reason, trimmed, "[REDACTED]")
			}
			lower := strings.ToLower(trimmed)
			if lower != trimmed && strings.Contains(reason, lower) {
				reason = strings.ReplaceAll(reason, lower, "[REDACTED]")
			}
		} else {
			if reason == trimmed {
				reason = "[REDACTED]"
				continue
			}
			re := regexp.MustCompile(`(^|[^a-zA-Z0-9_])` + regexp.QuoteMeta(trimmed) + `($|[^a-zA-Z0-9_])`)
			for strings.Contains(reason, trimmed) && re.MatchString(reason) {
				reason = re.ReplaceAllString(reason, `${1}[REDACTED]${2}`)
			}
		}
	}
	return reason
}

func sanitizeCapabilityError(err *CapabilityError, creds domain.InboundProtocolCredential) *CapabilityError {
	if err == nil {
		return nil
	}
	copyErr := *err
	copyErr.Reason = sanitizeReason(copyErr.Reason, creds)
	copyErr.Feature = sanitizeReason(copyErr.Feature, creds)
	return &copyErr
}

func hasConfiguredPlugin(creds domain.InboundProtocolCredential) bool {
	if creds.Transport == nil {
		return false
	}
	for k, v := range creds.Transport {
		if strings.TrimSpace(v) == "" {
			continue
		}
		kLower := strings.ToLower(strings.TrimSpace(k))
		if kLower == "plugin" || kLower == "plugin-opts" || kLower == "plugin_opts" {
			return true
		}
	}
	return false
}

// checkNodeRenderability evaluates whether a single node can be rendered by the target.
// It checks generic identifiers, protocol support, non-Mihomo dialer proxy, credential
// completeness, and invokes the target's native single-node renderer or validator.
// If the node cannot be rendered, it returns a sanitized CapabilityError that never leaks secrets.
func checkNodeRenderability(target domain.CompilerTarget, index int, node resolver.ResolvedNode) *CapabilityError {
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
	if strings.TrimSpace(node.DisplayName) == "" {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   "node display name is required",
		}
	}

	capability, ok := capabilityForTarget(target)
	if !ok {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   fmt.Sprintf("unknown compiler target: %s", target),
		}
	}

	if !node.Protocol.IsValid() || !capability.Protocols[node.Protocol] {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   fmt.Sprintf("target %s does not support protocol %s (protocol is not supported)", target, node.Protocol),
		}
	}

	if target != domain.TargetMihomo && (strings.TrimSpace(node.DialerProxy) != "" || strings.TrimSpace(node.ChainBindingID) != "") {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  "dialer-proxy",
			Reason:   fmt.Sprintf("target %s does not support dialer-proxy", target),
		}
	}

	if target != domain.TargetMihomo && hasConfiguredPlugin(node.Credentials) {
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  "plugin",
			Reason:   fmt.Sprintf("target %s does not support plugin or plugin-opts", target),
		}
	}

	if err := validateCredentialEnvelope(target, index, node); err != nil {
		var capErr *CapabilityError
		if errors.As(err, &capErr) {
			return sanitizeCapabilityError(capErr, node.Credentials)
		}
		return &CapabilityError{
			Target:   target,
			Location: loc,
			Feature:  feature,
			Reason:   sanitizeReason(err.Error(), node.Credentials),
		}
	}

	switch target {
	case domain.TargetMihomo:
		singleSnap := resolver.ResolvedPolicySnapshot{
			Nodes: []resolver.ResolvedNode{node},
		}
		if err := validateMihomoCredentials(&singleSnap); err != nil {
			var capErr *CapabilityError
			if errors.As(err, &capErr) {
				capErr.Location = loc
				return sanitizeCapabilityError(capErr, node.Credentials)
			}
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   sanitizeReason(err.Error(), node.Credentials),
			}
		}

	case domain.TargetSingBox:
		tag := strings.TrimSpace(node.DisplayName)
		_, _, err := probesingbox.BuildExportNodeOption(tag, domain.Node{
			LogicalID:   node.LogicalID,
			DisplayName: tag,
			Protocol:    node.Protocol,
			Server:      node.Server,
			Port:        node.Port,
			Credentials: node.Credentials,
		})
		if err != nil {
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   sanitizeReason(err.Error(), node.Credentials),
			}
		}

	case domain.TargetSurge:
		_, err := renderSurgeProxyLine(index, node)
		if err != nil {
			var capErr *CapabilityError
			if errors.As(err, &capErr) {
				return sanitizeCapabilityError(capErr, node.Credentials)
			}
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   sanitizeReason(err.Error(), node.Credentials),
			}
		}

	case domain.TargetQuantumultX:
		_, err := renderQuantumultXServerLine(index, node)
		if err != nil {
			var capErr *CapabilityError
			if errors.As(err, &capErr) {
				return sanitizeCapabilityError(capErr, node.Credentials)
			}
			return &CapabilityError{
				Target:   target,
				Location: loc,
				Feature:  feature,
				Reason:   sanitizeReason(err.Error(), node.Credentials),
			}
		}
	}

	return nil
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
	_, ok := capabilityForTarget(target)
	if !ok {
		return []CapabilityDiagnostic{{
			Code:    "unsupported_target",
			Message: fmt.Sprintf("unknown compiler target: %s", target),
			Target:  string(target),
		}}
	}

	var diags []CapabilityDiagnostic
	for _, diagnostic := range snapshot.Diagnostics {
		if diagnostic.Code == "invalid_group_topology" && diagnostic.Severity == resolver.DiagnosticSeverityError {
			diags = append(diags, CapabilityDiagnostic{Code: diagnostic.Code, Message: diagnostic.Message, Target: string(target)})
		}
	}

	seenRenderableNames := make(map[string]bool)
	// Check each node using canonical per-node renderability evaluation
	for i, node := range snapshot.Nodes {
		if err := validateResolvedNodeFormat(node); err != nil {
			diags = append(diags, CapabilityDiagnostic{
				NodeID: node.LogicalID, Code: "format_error", Message: err.Error(), Target: string(target),
			})
			continue
		}
		if err := checkNodeRenderability(target, i, node); err != nil {
			diags = append(diags, CapabilityDiagnostic{
				NodeID:  node.LogicalID,
				Code:    "unsupported_target_capability",
				Message: err.Reason,
				Target:  string(target),
			})
			continue
		}

		name := strings.TrimSpace(node.DisplayName)
		if seenRenderableNames[name] {
			diags = append(diags, CapabilityDiagnostic{
				NodeID:  node.LogicalID,
				Code:    "unsupported_target_capability",
				Message: fmt.Sprintf("duplicate node display name %q", node.DisplayName),
				Target:  string(target),
			})
		} else {
			seenRenderableNames[name] = true
		}
	}

	// Target-specific group and rule topology validation (Mihomo only)
	if target == domain.TargetMihomo {
		diags = append(diags, chainTopologyDiagnostics(snapshot)...)
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
			if !groupHasExportDestination(snapshot, g) {
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

// FilterCompatibleSnapshot 按目标排除不可渲染节点，保留原快照和严格拓扑校验
func FilterCompatibleSnapshot(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) (*resolver.ResolvedPolicySnapshot, []domain.ManifestExcludedNode) {
	if snapshot == nil {
		return nil, nil
	}
	_, ok := capabilityForTarget(target)
	if !ok {
		return snapshot, nil
	}

	var admittedNodes []resolver.ResolvedNode
	var excludedNodes []domain.ManifestExcludedNode
	admittedSet := make(map[string]bool)
	admittedNameSet := make(map[string]bool)
	excludedSet := make(map[string]bool)
	excludedNameSet := make(map[string]bool)

	for i, node := range snapshot.Nodes {
		if err := validateResolvedNodeFormat(node); err != nil {
			excludedNodes = append(excludedNodes, domain.ManifestExcludedNode{
				NodeID: node.LogicalID, Code: "format_error", Reason: err.Error(),
			})
			continue
		}
		if err := checkNodeRenderability(target, i, node); err != nil {
			excludedNodes = append(excludedNodes, domain.ManifestExcludedNode{
				NodeID: node.LogicalID,
				Code:   "unsupported_target_capability",
				Reason: err.Reason,
			})
			continue
		}

		name := strings.TrimSpace(node.DisplayName)
		copyNode := node
		copyNode.Position = len(admittedNodes)
		admittedNodes = append(admittedNodes, copyNode)
		admittedSet[node.LogicalID] = true
		admittedNameSet[node.DisplayName] = true
		if name != node.DisplayName {
			admittedNameSet[name] = true
		}
	}

	// 入口被排除后继续移除依赖它的节点，不能把原代理链改为直接连接
	if target == domain.TargetMihomo {
		admittedNodes, excludedNodes = filterChainDependencies(snapshot, admittedNodes, excludedNodes)
		admittedSet = make(map[string]bool, len(admittedNodes))
		admittedNameSet = make(map[string]bool, len(admittedNodes))
		for i := range admittedNodes {
			admittedNodes[i].Position = i
			admittedSet[admittedNodes[i].LogicalID] = true
			admittedNameSet[strings.TrimSpace(admittedNodes[i].DisplayName)] = true
		}
	}

	admittedLogicalIDs := make([]string, len(admittedNodes))
	for i, node := range admittedNodes {
		admittedLogicalIDs[i] = node.LogicalID
	}

	for _, node := range snapshot.Nodes {
		if !admittedSet[node.LogicalID] {
			excludedSet[node.LogicalID] = true
			excludedNameSet[strings.TrimSpace(node.DisplayName)] = true
		}
	}

	// 先按已解析 ID 固定成员类型，再移除节点，避免同名分组影响 PASS 重算
	boundGroups, bindingDiagnostics := bindProjectionGroupMembers(snapshot, target)
	filteredGroups := make([]resolver.ResolvedGroup, len(boundGroups))
	for gi, group := range boundGroups {
		filteredGroup := group
		var filteredMembers []resolver.ResolvedGroupMember
		for _, member := range group.Members {
			name := strings.TrimSpace(member.DisplayName)
			if member.Kind == resolver.MemberKindNode || member.Kind == "" {
				if (member.TargetID != "" && excludedSet[member.TargetID]) || (member.TargetID == "" && excludedNameSet[name] && !admittedNameSet[name]) {
					continue
				}
			}
			filteredMembers = append(filteredMembers, member)
		}
		filteredGroup.Members = filteredMembers

		if group.Sources != nil {
			filteredGroup.Sources = append([]domain.MemberSource(nil), group.Sources...)
		}

		filteredGroups[gi] = filteredGroup
	}

	copySnap := *snapshot
	copySnap.Nodes = admittedNodes
	copySnap.NodeLogicalIDs = admittedLogicalIDs
	copySnap.Groups = filteredGroups
	if snapshot.Rules != nil {
		copySnap.Rules = append([]resolver.ResolvedRule(nil), snapshot.Rules...)
	}
	if snapshot.Diagnostics != nil {
		copySnap.Diagnostics = append([]resolver.Diagnostic(nil), snapshot.Diagnostics...)
	}
	if snapshot.ChainEffects != nil {
		copySnap.ChainEffects = append([]resolver.ChainEffect(nil), snapshot.ChainEffects...)
	}
	copySnap.Diagnostics = append(copySnap.Diagnostics, bindingDiagnostics...)
	if target == domain.TargetMihomo {
		for _, diagnostic := range chainTopologyDiagnostics(snapshot) {
			copySnap.Diagnostics = append(copySnap.Diagnostics, resolver.Diagnostic{
				Severity: resolver.DiagnosticSeverityError, Code: diagnostic.Code, Message: diagnostic.Message, Target: string(target),
			})
		}
	}
	refreshProjectedGroups(&copySnap)
	refreshProjectionSummaries(snapshot, &copySnap, excludedNodes, target)
	if digest, err := resolver.ComputeSnapshotDigest(&copySnap); err == nil {
		copySnap.SnapshotDigest = digest
	} else {
		copySnap.SnapshotDigest = ""
		copySnap.Diagnostics = append(copySnap.Diagnostics, resolver.Diagnostic{
			Severity: resolver.DiagnosticSeverityError, Code: "snapshot_digest_failed",
			Message: "could not compute target projection digest", Target: string(target),
		})
	}

	return &copySnap, excludedNodes
}

func validateResolvedNodeFormat(node resolver.ResolvedNode) error {
	return parser.ValidateNodeFormat(domain.Node{
		LogicalID: node.LogicalID, DisplayName: node.DisplayName, Protocol: node.Protocol,
		Server: node.Server, Port: node.Port, Credentials: node.Credentials,
	})
}

// SortedCapabilities exposes target names without leaking the backing map.
func SortedCapabilities() []domain.CompilerTarget {
	result := Targets()
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
