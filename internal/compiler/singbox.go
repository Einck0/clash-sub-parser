package compiler

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	probesingbox "clash-sub-parser/internal/probe/singbox"
	"clash-sub-parser/internal/resolver"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badoption"
)

const (
	defaultSingBoxURLTestURL       = "https://www.gstatic.com/generate_204"
	defaultSingBoxURLTestInterval  = 300 * time.Second
	defaultSingBoxURLTestTolerance = 50
	singGeoIPRuleSetPrefix         = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/"
	singGeositeRuleSetPrefix       = "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/"
)

func singBoxCapability() Capability {
	return Capability{
		Protocols: protocolSet(
			domain.ProtocolSS,
			domain.ProtocolVMess,
			domain.ProtocolVLESS,
			domain.ProtocolTrojan,
			domain.ProtocolHysteria2,
			domain.ProtocolWireGuard,
			domain.ProtocolTUIC,
		),
		GroupTypes: groupSet(
			domain.GroupTypeSelect,
			domain.GroupTypeURLTest,
		),
		RuleKinds: ruleSet(
			"DOMAIN",
			"DOMAIN-SUFFIX",
			"DOMAIN-KEYWORD",
			"IP-CIDR",
			"IP-CIDR6",
			"GEOIP",
			"GEOSITE",
			"SRC-IP-CIDR",
			"SRC-PORT",
			"DST-PORT",
			"PORT",
			"PROCESS-NAME",
			"RULE-SET",
			"MATCH",
		),
	}
}

func renderSingBox(snapshot *resolver.ResolvedPolicySnapshot) ([]byte, error) {
	var (
		endpoints []option.Endpoint
		outbounds []option.Outbound
	)

	seenTags := make(map[string]bool, len(snapshot.Nodes)+len(snapshot.Groups))
	nodeNameByID := make(map[string]string, len(snapshot.Nodes))
	groupNameByID := make(map[string]string, len(snapshot.Groups))
	definedGroups := make(map[string]bool, len(snapshot.Groups))

	for i, node := range snapshot.Nodes {
		loc := fmt.Sprintf("nodes[%d]", i)
		if err := validateCredentialEnvelope(domain.TargetSingBox, i, node); err != nil {
			return nil, err
		}
		tag := strings.TrimSpace(node.DisplayName)
		if seenTags[tag] {
			return nil, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  tag,
				Reason:   "duplicate outbound/endpoint tag in sing-box configuration",
			}
		}
		seenTags[tag] = true
		nodeNameByID[node.LogicalID] = tag

		out, ep, err := probesingbox.BuildExportNodeOption(tag, domain.Node{
			LogicalID:   node.LogicalID,
			DisplayName: tag,
			Protocol:    node.Protocol,
			Server:      node.Server,
			Port:        node.Port,
			Credentials: node.Credentials,
		})
		if err != nil {
			return nil, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  string(node.Protocol),
				Reason:   err.Error(),
			}
		}
		if ep != nil {
			endpoints = append(endpoints, *ep)
		}
		if out != nil {
			outbounds = append(outbounds, *out)
		}
	}

	for _, group := range snapshot.Groups {
		if group.ID != "" {
			groupNameByID[group.ID] = strings.TrimSpace(group.Name)
		}
		definedGroups[strings.TrimSpace(group.Name)] = true
	}

	var requiredBuiltInOutbounds []string
	requireBuiltInOutbound := func(tag string) {
		if seenTags[tag] {
			return
		}
		for _, existing := range requiredBuiltInOutbounds {
			if existing == tag {
				return
			}
		}
		requiredBuiltInOutbounds = append(requiredBuiltInOutbounds, tag)
	}

	for i, group := range snapshot.Groups {
		loc := fmt.Sprintf("groups[%d]", i)
		groupTag := strings.TrimSpace(group.Name)
		if seenTags[groupTag] {
			return nil, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  groupTag,
				Reason:   "duplicate group outbound tag in sing-box configuration",
			}
		}
		seenTags[groupTag] = true

		members := resolveSingBoxGroupMembers(group, nodeNameByID, groupNameByID)
		if len(members) == 0 {
			return nil, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  groupTag,
				Reason:   "policy group must contain at least one member in sing-box",
			}
		}

		for _, member := range members {
			if !definedGroups[member] && !seenTags[member] && isBuiltInPolicyTarget(member) {
				if err := validateSingBoxBuiltInTarget(loc, member); err != nil {
					return nil, err
				}
				requireBuiltInOutbound(member)
			}
		}

		switch group.GroupType {
		case domain.GroupTypeSelect:
			outbounds = append(outbounds, option.Outbound{
				Type: C.TypeSelector,
				Tag:  groupTag,
				Options: &option.SelectorOutboundOptions{
					Outbounds: members,
				},
			})
		case domain.GroupTypeURLTest:
			outbounds = append(outbounds, option.Outbound{
				Type: C.TypeURLTest,
				Tag:  groupTag,
				Options: &option.URLTestOutboundOptions{
					Outbounds: members,
					URL:       defaultSingBoxURLTestURL,
					Interval:  badoption.Duration(defaultSingBoxURLTestInterval),
					Tolerance: defaultSingBoxURLTestTolerance,
				},
			})
		default:
			return nil, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  string(group.GroupType),
				Reason:   "policy group type is not supported",
			}
		}
	}

	var (
		routeRules []option.Rule
		ruleSets   []option.RuleSet
		finalTag   string
	)
	ruleSetURLByTag := make(map[string]string)
	registerRuleSet := func(loc, feature, tag, rawURL, format string) error {
		if existingURL, exists := ruleSetURLByTag[tag]; exists {
			if existingURL != rawURL {
				return &CapabilityError{
					Target:   domain.TargetSingBox,
					Location: loc,
					Feature:  feature,
					Reason:   fmt.Sprintf("conflicting rule-set URL for tag %q", tag),
				}
			}
			return nil
		}
		ruleSetURLByTag[tag] = rawURL
		ruleSets = append(ruleSets, option.RuleSet{
			Type:   C.RuleSetTypeRemote,
			Tag:    badoption.Listable[string]{tag},
			Format: format,
			RemoteOptions: option.RemoteRuleSet{
				URL: rawURL,
			},
		})
		return nil
	}

	for i, rule := range snapshot.Rules {
		loc := fmt.Sprintf("rules[%d]", i)
		expr := strings.TrimSpace(rule.Expression)
		kind := ruleKind(expr)
		targetName := strings.TrimSpace(rule.TargetGroupName)

		if !definedGroups[targetName] && isBuiltInPolicyTarget(targetName) {
			if err := validateSingBoxBuiltInTarget(loc, targetName); err != nil {
				return nil, err
			}
		}

		if domain.IsMatchRule(expr) {
			if !definedGroups[targetName] && isBuiltInPolicyTarget(targetName) {
				requireBuiltInOutbound(targetName)
			}
			finalTag = targetName
			continue
		}

		defaultRule, err := buildSingBoxDefaultRule(loc, kind, expr, registerRuleSet)
		if err != nil {
			return nil, err
		}

		action, needOutbound := buildSingBoxRuleAction(targetName, definedGroups)
		if needOutbound {
			requireBuiltInOutbound(targetName)
		}
		defaultRule.RuleAction = action

		routeRules = append(routeRules, option.Rule{
			Type:           C.RuleTypeDefault,
			DefaultOptions: defaultRule,
		})
	}

	for _, builtInTag := range requiredBuiltInOutbounds {
		if seenTags[builtInTag] {
			continue
		}
		seenTags[builtInTag] = true
		switch strings.ToUpper(builtInTag) {
		case "DIRECT":
			outbounds = append(outbounds, option.Outbound{
				Type:    C.TypeDirect,
				Tag:     builtInTag,
				Options: &option.DirectOutboundOptions{},
			})
		case "REJECT", "REJECT-DROP":
			outbounds = append(outbounds, option.Outbound{
				Type:    C.TypeBlock,
				Tag:     builtInTag,
				Options: &option.StubOptions{},
			})
		}
	}

	var options option.Options
	if len(endpoints) > 0 {
		options.Endpoints = endpoints
	}
	if len(outbounds) > 0 {
		options.Outbounds = outbounds
	}
	if len(routeRules) > 0 || len(ruleSets) > 0 || finalTag != "" {
		options.Route = &option.RouteOptions{
			Rules:   routeRules,
			RuleSet: ruleSets,
			Final:   finalTag,
		}
	}

	var buf bytes.Buffer
	enc := singjson.NewEncoderContext(probesingbox.ExportOptionContext(context.Background()), &buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(options); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func resolveSingBoxGroupMembers(group resolver.ResolvedGroup, nodeNameByID, groupNameByID map[string]string) []string {
	if len(group.Members) > 0 {
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			if name := strings.TrimSpace(member.DisplayName); name != "" {
				members = append(members, name)
			}
		}
		return members
	}

	members := make([]string, 0, len(group.ChildGroupIDs)+len(group.NodeLogicalIDs))
	for _, childID := range group.ChildGroupIDs {
		if name := groupNameByID[childID]; name != "" {
			members = append(members, name)
		}
	}
	for _, nodeID := range group.NodeLogicalIDs {
		if name := nodeNameByID[nodeID]; name != "" {
			members = append(members, name)
		}
	}
	return members
}

func validateSingBoxBuiltInTarget(loc, targetName string) error {
	switch strings.ToUpper(strings.TrimSpace(targetName)) {
	case "DIRECT", "REJECT", "REJECT-DROP":
		return nil
	default:
		return &CapabilityError{
			Target:   domain.TargetSingBox,
			Location: loc,
			Feature:  targetName,
			Reason:   fmt.Sprintf("built-in target %q is not supported in sing-box", targetName),
		}
	}
}

func buildSingBoxRuleAction(targetName string, definedGroups map[string]bool) (option.RuleAction, bool) {
	if !definedGroups[targetName] {
		switch strings.ToUpper(targetName) {
		case "REJECT":
			return option.RuleAction{
				Action: C.RuleActionTypeReject,
			}, false
		case "REJECT-DROP":
			return option.RuleAction{
				Action: C.RuleActionTypeReject,
				RejectOptions: option.RejectActionOptions{
					Method: C.RuleActionRejectMethodDrop,
				},
			}, false
		case "DIRECT":
			return option.RuleAction{
				Action: C.RuleActionTypeRoute,
				RouteOptions: option.RouteActionOptions{
					Outbound: targetName,
				},
			}, true
		}
	}
	return option.RuleAction{
		Action: C.RuleActionTypeRoute,
		RouteOptions: option.RouteActionOptions{
			Outbound: targetName,
		},
	}, false
}

func buildSingBoxDefaultRule(
	loc, kind, expr string,
	registerRuleSet func(loc, feature, tag, rawURL, format string) error,
) (option.DefaultRule, error) {
	parts := strings.Split(expr, ",")
	if len(parts) < 2 {
		return option.DefaultRule{}, &CapabilityError{
			Target:   domain.TargetSingBox,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule value is required",
		}
	}
	val := strings.TrimSpace(parts[1])
	if val == "" {
		return option.DefaultRule{}, &CapabilityError{
			Target:   domain.TargetSingBox,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule value is required",
		}
	}

	var rule option.DefaultRule
	switch kind {
	case "DOMAIN":
		if strings.ContainsAny(val, " \t\r\n/") {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid domain value %q", val),
			}
		}
		rule.Domain = badoption.Listable[string]{val}

	case "DOMAIN-SUFFIX":
		if strings.ContainsAny(val, " \t\r\n/") {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid domain suffix value %q", val),
			}
		}
		rule.DomainSuffix = badoption.Listable[string]{val}

	case "DOMAIN-KEYWORD":
		if strings.ContainsAny(val, " \t\r\n") {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid domain keyword value %q", val),
			}
		}
		rule.DomainKeyword = badoption.Listable[string]{val}

	case "IP-CIDR":
		prefix, err := netip.ParsePrefix(val)
		if err != nil || !prefix.Addr().Is4() {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid IPv4 CIDR %q", val),
			}
		}
		rule.IPCIDR = badoption.Listable[string]{prefix.String()}

	case "IP-CIDR6":
		prefix, err := netip.ParsePrefix(val)
		if err != nil || !prefix.Addr().Is6() {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid IPv6 CIDR %q", val),
			}
		}
		rule.IPCIDR = badoption.Listable[string]{prefix.String()}

	case "SRC-IP-CIDR":
		prefix, err := netip.ParsePrefix(val)
		if err != nil {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid source IP CIDR %q", val),
			}
		}
		rule.SourceIPCIDR = badoption.Listable[string]{prefix.String()}

	case "SRC-PORT":
		port, portRange, err := parseSingBoxPortSpec(val)
		if err != nil {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   err.Error(),
			}
		}
		if portRange != "" {
			rule.SourcePortRange = badoption.Listable[string]{portRange}
		} else {
			rule.SourcePort = badoption.Listable[uint16]{port}
		}

	case "DST-PORT", "PORT":
		port, portRange, err := parseSingBoxPortSpec(val)
		if err != nil {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   err.Error(),
			}
		}
		if portRange != "" {
			rule.PortRange = badoption.Listable[string]{portRange}
		} else {
			rule.Port = badoption.Listable[uint16]{port}
		}

	case "PROCESS-NAME":
		if strings.ContainsAny(val, "\r\n\x00") {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid process name %q", val),
			}
		}
		rule.ProcessName = badoption.Listable[string]{val}

	case "GEOIP":
		code := strings.ToLower(val)
		if !isValidSingBoxRuleSetToken(code) {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid GEOIP code %q", val),
			}
		}
		tag := "geoip-" + code
		rawURL := singGeoIPRuleSetPrefix + tag + ".srs"
		if err := registerRuleSet(loc, kind, tag, rawURL, C.RuleSetFormatBinary); err != nil {
			return option.DefaultRule{}, err
		}
		rule.RuleSet = badoption.Listable[string]{tag}

	case "GEOSITE":
		name := strings.ToLower(val)
		if !isValidSingBoxRuleSetToken(name) {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid GEOSITE name %q", val),
			}
		}
		tag := "geosite-" + name
		rawURL := singGeositeRuleSetPrefix + tag + ".srs"
		if err := registerRuleSet(loc, kind, tag, rawURL, C.RuleSetFormatBinary); err != nil {
			return option.DefaultRule{}, err
		}
		rule.RuleSet = badoption.Listable[string]{tag}

	case "RULE-SET":
		tag, rawURL, format, err := resolveSingBoxRuleSetSpec(parts[1:])
		if err != nil {
			return option.DefaultRule{}, &CapabilityError{
				Target:   domain.TargetSingBox,
				Location: loc,
				Feature:  kind,
				Reason:   err.Error(),
			}
		}
		if err := registerRuleSet(loc, kind, tag, rawURL, format); err != nil {
			return option.DefaultRule{}, err
		}
		rule.RuleSet = badoption.Listable[string]{tag}

	default:
		return option.DefaultRule{}, &CapabilityError{
			Target:   domain.TargetSingBox,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule kind is not supported",
		}
	}

	return rule, nil
}

func parseSingBoxPortSpec(raw string) (uint16, string, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "-") || strings.Contains(raw, ":") {
		sep := "-"
		if strings.Contains(raw, ":") {
			sep = ":"
		}
		bounds := strings.Split(raw, sep)
		if len(bounds) != 2 {
			return 0, "", fmt.Errorf("invalid port range %q", raw)
		}
		startStr := strings.TrimSpace(bounds[0])
		endStr := strings.TrimSpace(bounds[1])
		start, err1 := strconv.Atoi(startStr)
		end, err2 := strconv.Atoi(endStr)
		if err1 != nil || err2 != nil || start < 1 || end > 65535 || start > end {
			return 0, "", fmt.Errorf("invalid port range %q", raw)
		}
		return 0, fmt.Sprintf("%d:%d", start, end), nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("invalid port %q", raw)
	}
	return uint16(port), "", nil
}

func isValidSingBoxRuleSetToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == '@':
		default:
			return false
		}
	}
	return true
}

func resolveSingBoxRuleSetSpec(args []string) (string, string, string, error) {
	first := strings.TrimSpace(args[0])
	if first == "" {
		return "", "", "", fmt.Errorf("rule-set identifier or URL is required")
	}

	if len(args) >= 2 && strings.TrimSpace(args[1]) != "" {
		second := strings.TrimSpace(args[1])
		if strings.HasPrefix(strings.ToLower(second), "http://") || strings.HasPrefix(strings.ToLower(second), "https://") {
			if !isValidSingBoxRuleSetToken(first) {
				return "", "", "", fmt.Errorf("invalid rule-set tag %q", first)
			}
			format, err := validateSingBoxRuleSetURL(second)
			if err != nil {
				return "", "", "", err
			}
			return first, second, format, nil
		}
	}

	if strings.HasPrefix(strings.ToLower(first), "http://") || strings.HasPrefix(strings.ToLower(first), "https://") {
		format, err := validateSingBoxRuleSetURL(first)
		if err != nil {
			return "", "", "", err
		}
		u, _ := url.Parse(first)
		base := path.Base(u.Path)
		ext := path.Ext(base)
		tag := strings.TrimSuffix(base, ext)
		if !isValidSingBoxRuleSetToken(tag) {
			tag = "ruleset"
		}
		return tag, first, format, nil
	}

	if !isValidSingBoxRuleSetToken(first) {
		return "", "", "", fmt.Errorf("invalid rule-set tag %q", first)
	}
	lower := strings.ToLower(first)
	if strings.HasPrefix(lower, "geoip-") && len(lower) > len("geoip-") {
		return first, singGeoIPRuleSetPrefix + lower + ".srs", C.RuleSetFormatBinary, nil
	}
	if strings.HasPrefix(lower, "geosite-") && len(lower) > len("geosite-") {
		return first, singGeositeRuleSetPrefix + lower + ".srs", C.RuleSetFormatBinary, nil
	}
	return first, singGeositeRuleSetPrefix + "geosite-" + lower + ".srs", C.RuleSetFormatBinary, nil
}

func validateSingBoxRuleSetURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path == "" || u.Path == "/" {
		return "", fmt.Errorf("invalid rule-set URL %q", rawURL)
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".json":
		return C.RuleSetFormatSource, nil
	default:
		return C.RuleSetFormatBinary, nil
	}
}
