// Package compiler renders one resolved policy snapshot for a named client target.
package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
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
	return []domain.CompilerTarget{domain.TargetClash, domain.TargetMihomo, domain.TargetSingBox, domain.TargetSurge, domain.TargetQuantumultX}
}

var capabilities = map[domain.CompilerTarget]Capability{
	domain.TargetClash: {
		Protocols:  protocolSet(domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolVLESS, domain.ProtocolTrojan),
		GroupTypes: groupSet(domain.GroupTypeSelect, domain.GroupTypeURLTest, domain.GroupTypeFallback, domain.GroupTypeLoadBalance),
		RuleKinds:  ruleSet("DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6", "GEOIP", "SRC-IP-CIDR", "SRC-PORT", "DST-PORT", "PORT", "PROCESS-NAME", "MATCH"),
	},
	domain.TargetMihomo: {
		Protocols:  protocolSet(domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolVLESS, domain.ProtocolTrojan, domain.ProtocolHysteria2, domain.ProtocolWireGuard, domain.ProtocolTUIC),
		GroupTypes: groupSet(domain.GroupTypeSelect, domain.GroupTypeURLTest, domain.GroupTypeFallback, domain.GroupTypeLoadBalance),
		RuleKinds:  ruleSet("DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DOMAIN-REGEX", "GEOSITE", "IP-CIDR", "IP-CIDR6", "GEOIP", "IP-ASN", "SRC-GEOIP", "SRC-IP-CIDR", "SRC-IP-CIDR6", "SRC-PORT", "DST-PORT", "PORT", "IN-PORT", "IN-TYPE", "IN-USER", "IN-NAME", "PROCESS-NAME", "PROCESS-PATH", "PROCESS-NAME-REGEX", "PROCESS-PATH-REGEX", "PACKAGE-NAME", "RULE-SET", "AND", "OR", "NOT", "SUB-RULE", "MATCH"),
	},
	domain.TargetSingBox: {
		Protocols:  protocolSet(domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolVLESS, domain.ProtocolTrojan, domain.ProtocolHysteria2, domain.ProtocolWireGuard, domain.ProtocolTUIC),
		GroupTypes: groupSet(domain.GroupTypeSelect, domain.GroupTypeURLTest, domain.GroupTypeFallback, domain.GroupTypeLoadBalance),
		RuleKinds:  ruleSet("DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6", "GEOIP", "GEOSITE", "PROCESS-NAME", "MATCH"),
	},
	domain.TargetSurge: {
		Protocols:  protocolSet(domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolVLESS, domain.ProtocolTrojan),
		GroupTypes: groupSet(domain.GroupTypeSelect, domain.GroupTypeURLTest, domain.GroupTypeFallback, domain.GroupTypeLoadBalance),
		RuleKinds:  ruleSet("DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6", "GEOIP", "PROCESS-NAME", "SRC-IP", "DEST-PORT", "IN-PORT", "RULE-SET", "USER-AGENT", "URL-REGEX", "MATCH"),
	},
	domain.TargetQuantumultX: {
		Protocols:  protocolSet(domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolVLESS, domain.ProtocolTrojan),
		GroupTypes: groupSet(domain.GroupTypeSelect),
		RuleKinds:  ruleSet("DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6", "GEOIP", "MATCH"),
	},
}

// CapabilityMatrix returns a deep copy so callers cannot mutate compiler policy.
func CapabilityMatrix() map[domain.CompilerTarget]Capability {
	copyMatrix := make(map[domain.CompilerTarget]Capability, len(capabilities))
	for target, capability := range capabilities {
		copyMatrix[target] = Capability{
			Protocols:  copyProtocolSet(capability.Protocols),
			GroupTypes: copyGroupSet(capability.GroupTypes),
			RuleKinds:  copyStringSet(capability.RuleKinds),
		}
	}
	return copyMatrix
}

// Compile validates target capabilities before rendering a deterministic result.
func Compile(ctx context.Context, snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if snapshot == nil {
		return Result{}, fmt.Errorf("resolved policy snapshot is required")
	}
	capability, ok := capabilities[target]
	if !ok {
		return Result{}, &CapabilityError{Target: target, Location: "target", Feature: string(target), Reason: "unknown compiler target"}
	}
	if err := validate(snapshot, target, capability); err != nil {
		return Result{}, err
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

func validate(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget, capability Capability) error {
	for i, node := range snapshot.Nodes {
		if !capability.Protocols[node.Protocol] {
			return &CapabilityError{Target: target, Location: fmt.Sprintf("nodes[%d]", i), Feature: string(node.Protocol), Reason: "protocol is not supported"}
		}
	}
	for i, group := range snapshot.Groups {
		if !capability.GroupTypes[group.GroupType] {
			return &CapabilityError{Target: target, Location: fmt.Sprintf("groups[%d]", i), Feature: string(group.GroupType), Reason: "policy group type is not supported"}
		}
	}
	for i, rule := range snapshot.Rules {
		kind := strings.ToUpper(strings.TrimSpace(rule.Expression))
		if comma := strings.IndexByte(kind, ','); comma >= 0 {
			kind = kind[:comma]
		}
		if space := strings.IndexByte(kind, ' '); space >= 0 {
			kind = kind[:space]
		}
		if !capability.RuleKinds[kind] {
			return &CapabilityError{Target: target, Location: fmt.Sprintf("rules[%d]", i), Feature: kind, Reason: "routing rule kind is not supported"}
		}
	}
	return nil
}

func render(snapshot *resolver.ResolvedPolicySnapshot, target domain.CompilerTarget) ([]byte, string, string, error) {
	switch target {
	case domain.TargetClash, domain.TargetMihomo:
		value := renderClash(snapshot)
		content, err := yaml.Marshal(value)
		return content, "application/yaml", string(target) + ".yaml", err
	case domain.TargetSingBox:
		content, err := json.MarshalIndent(renderSingBox(snapshot), "", "  ")
		if err == nil {
			content = append(content, '\n')
		}
		return content, "application/json", "sing-box.json", err
	case domain.TargetSurge:
		return []byte(renderSurge(snapshot)), "text/plain; charset=utf-8", "surge.conf", nil
	case domain.TargetQuantumultX:
		return []byte(renderQuantumultX(snapshot)), "text/plain; charset=utf-8", "quantumult-x.conf", nil
	default:
		return nil, "", "", fmt.Errorf("unknown compiler target %s", target)
	}
}

type clashConfig struct {
	Proxies     []proxyConfig `yaml:"proxies"`
	ProxyGroups []groupConfig `yaml:"proxy-groups"`
	Rules       []string      `yaml:"rules"`
}

type proxyConfig struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Server string `yaml:"server"`
	Port   int    `yaml:"port"`
}

type groupConfig struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Proxies []string `yaml:"proxies"`
}

func renderClash(snapshot *resolver.ResolvedPolicySnapshot) clashConfig {
	proxies := make([]proxyConfig, 0, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		proxies = append(proxies, proxyConfig{Name: node.DisplayName, Type: string(node.Protocol), Server: node.LogicalID, Port: 443})
	}
	groups := make([]groupConfig, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.DisplayName)
		}
		groups = append(groups, groupConfig{Name: group.Name, Type: string(group.GroupType), Proxies: members})
	}
	rules := make([]string, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		rules = append(rules, rule.Expression+","+rule.TargetGroupName)
	}
	return clashConfig{Proxies: proxies, ProxyGroups: groups, Rules: rules}
}

type singBoxConfig struct {
	Outbounds []singBoxOutbound `json:"outbounds"`
	Route     singBoxRoute      `json:"route"`
}

type singBoxOutbound struct {
	Type       string   `json:"type"`
	Tag        string   `json:"tag"`
	Server     string   `json:"server,omitempty"`
	ServerPort int      `json:"server_port,omitempty"`
	Outbounds  []string `json:"outbounds,omitempty"`
}

type singBoxRoute struct {
	Rules []singBoxRule `json:"rules"`
}

type singBoxRule struct {
	Expression string `json:"expression"`
	Outbound   string `json:"outbound"`
}

func renderSingBox(snapshot *resolver.ResolvedPolicySnapshot) singBoxConfig {
	outbounds := make([]singBoxOutbound, 0, len(snapshot.Nodes)+len(snapshot.Groups))
	for _, node := range snapshot.Nodes {
		outbounds = append(outbounds, singBoxOutbound{Type: string(node.Protocol), Tag: node.DisplayName, Server: node.LogicalID, ServerPort: 443})
	}
	for _, group := range snapshot.Groups {
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.DisplayName)
		}
		outbounds = append(outbounds, singBoxOutbound{Type: string(group.GroupType), Tag: group.Name, Outbounds: members})
	}
	rules := make([]singBoxRule, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		rules = append(rules, singBoxRule{Expression: rule.Expression, Outbound: rule.TargetGroupName})
	}
	return singBoxConfig{Outbounds: outbounds, Route: singBoxRoute{Rules: rules}}
}

func renderSurge(snapshot *resolver.ResolvedPolicySnapshot) string {
	var b strings.Builder
	b.WriteString("[General]\nloglevel = notify\n\n[Proxy]\n")
	for _, node := range snapshot.Nodes {
		fmt.Fprintf(&b, "%s = %s, %s, 443\n", node.DisplayName, node.Protocol, node.LogicalID)
	}
	b.WriteString("\n[Proxy Group]\n")
	for _, group := range snapshot.Groups {
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.DisplayName)
		}
		fmt.Fprintf(&b, "%s = %s, %s\n", group.Name, group.GroupType, strings.Join(members, ", "))
	}
	b.WriteString("\n[Rule]\n")
	for _, rule := range snapshot.Rules {
		fmt.Fprintf(&b, "%s, %s\n", rule.Expression, rule.TargetGroupName)
	}
	return b.String()
}

func renderQuantumultX(snapshot *resolver.ResolvedPolicySnapshot) string {
	var b strings.Builder
	b.WriteString("[general]\nnetwork_check_url = http://cp.cloudflare.com/generate_204\n\n[server_local]\n")
	for _, node := range snapshot.Nodes {
		fmt.Fprintf(&b, "%s = %s, address=%s, port=443\n", node.DisplayName, node.Protocol, node.LogicalID)
	}
	b.WriteString("\n[policy]\n")
	for _, group := range snapshot.Groups {
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.DisplayName)
		}
		fmt.Fprintf(&b, "%s = static, %s\n", group.Name, strings.Join(members, ", "))
	}
	b.WriteString("\n[filter_local]\n")
	for _, rule := range snapshot.Rules {
		fmt.Fprintf(&b, "%s, %s\n", rule.Expression, rule.TargetGroupName)
	}
	return b.String()
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
