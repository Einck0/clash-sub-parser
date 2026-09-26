package compiler

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

var validQXSSCiphers = map[string]bool{
	"none":                          true,
	"rc4":                           true,
	"rc4-md5":                       true,
	"aes-128-cfb":                   true,
	"aes-192-cfb":                   true,
	"aes-256-cfb":                   true,
	"aes-128-ctr":                   true,
	"aes-192-ctr":                   true,
	"aes-256-ctr":                   true,
	"bf-cfb":                        true,
	"camellia-128-cfb":              true,
	"camellia-192-cfb":              true,
	"camellia-256-cfb":              true,
	"salsa20":                       true,
	"chacha20":                      true,
	"chacha20-ietf":                 true,
	"aes-128-gcm":                   true,
	"aes-192-gcm":                   true,
	"aes-256-gcm":                   true,
	"chacha20-ietf-poly1305":        true,
	"chacha20-poly1305":             true,
	"xchacha20-ietf-poly1305":       true,
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
}

var validQXVMessMethods = map[string]bool{
	"auto":                   true,
	"none":                   true,
	"zero":                   true,
	"aes-128-cfb":            true,
	"aes-128-gcm":            true,
	"chacha20-poly1305":      true,
	"chacha20-ietf-poly1305": true,
}

func quantumultXCapability() Capability {
	return Capability{
		Protocols:  protocolSet(domain.ProtocolSS, domain.ProtocolVMess, domain.ProtocolTrojan),
		GroupTypes: groupSet(domain.GroupTypeSelect),
		RuleKinds:  ruleSet("DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6", "GEOIP", "MATCH"),
	}
}

func hasUnsafeQXChars(s string) bool {
	return strings.ContainsAny(s, ",\r\n")
}

func isQuantumultXBuiltInPolicy(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DIRECT", "REJECT":
		return true
	default:
		return false
	}
}

func renderQuantumultX(snapshot *resolver.ResolvedPolicySnapshot) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("resolved policy snapshot is required")
	}

	serverLines := make([]string, 0, len(snapshot.Nodes))
	for i, node := range snapshot.Nodes {
		if err := validateCredentialEnvelope(domain.TargetQuantumultX, i, node); err != nil {
			return nil, err
		}
		line, err := renderQuantumultXServerLine(i, node)
		if err != nil {
			return nil, err
		}
		serverLines = append(serverLines, line)
	}

	policyLines := make([]string, 0, len(snapshot.Groups))
	for i, group := range snapshot.Groups {
		loc := fmt.Sprintf("groups[%d]", i)
		if group.GroupType != domain.GroupTypeSelect {
			return nil, &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  string(group.GroupType),
				Reason:   "policy group type is not supported",
			}
		}
		groupName := strings.TrimSpace(group.Name)
		if groupName == "" || hasUnsafeQXChars(groupName) {
			return nil, &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  string(group.GroupType),
				Reason:   "invalid policy group name",
			}
		}
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			memberName := strings.TrimSpace(member.DisplayName)
			if memberName == "" || hasUnsafeQXChars(memberName) {
				return nil, &CapabilityError{
					Target:   domain.TargetQuantumultX,
					Location: loc,
					Feature:  groupName,
					Reason:   "invalid group member display name",
				}
			}
			if isBuiltInPolicyTarget(memberName) {
				if !isQuantumultXBuiltInPolicy(memberName) {
					return nil, &CapabilityError{
						Target:   domain.TargetQuantumultX,
						Location: loc,
						Feature:  memberName,
						Reason:   "built-in policy target is not supported by Quantumult X",
					}
				}
				memberName = strings.ToUpper(memberName)
			}
			members = append(members, memberName)
		}
		if len(members) == 0 {
			members = []string{"DIRECT"}
		}
		policyLines = append(policyLines, fmt.Sprintf("static = %s, %s", groupName, strings.Join(members, ", ")))
	}

	ruleLines := make([]string, 0, len(snapshot.Rules))
	for i, rule := range snapshot.Rules {
		if ruleKind(rule.Expression) == "MATCH" && i != len(snapshot.Rules)-1 {
			return nil, &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: fmt.Sprintf("rules[%d]", i),
				Feature:  "MATCH",
				Reason:   "terminal MATCH rule must be positioned last in Quantumult X [filter_local]",
			}
		}
		line, err := renderQuantumultXRuleLine(i, rule)
		if err != nil {
			return nil, err
		}
		ruleLines = append(ruleLines, line)
	}

	var b strings.Builder
	b.WriteString("[general]\nnetwork_check_url = http://cp.cloudflare.com/generate_204\n\n[server_local]\n")
	for _, line := range serverLines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n[policy]\n")
	for _, line := range policyLines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n[filter_local]\n")
	for _, line := range ruleLines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

func renderQuantumultXServerLine(index int, node resolver.ResolvedNode) (string, error) {
	loc := fmt.Sprintf("nodes[%d]", index)
	feature := string(node.Protocol)

	tag := strings.TrimSpace(node.DisplayName)
	server := strings.TrimSpace(node.Server)
	if tag == "" || server == "" || hasUnsafeQXChars(tag) || hasUnsafeQXChars(server) {
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  feature,
			Reason:   "invalid characters in node display name or server address",
		}
	}
	if server == strings.TrimSpace(node.LogicalID) {
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  feature,
			Reason:   "placeholder logical ID server address is not allowed",
		}
	}

	endpoint := net.JoinHostPort(server, strconv.Itoa(node.Port))
	tMap := node.Credentials.Transport

	if tMap != nil {
		if strings.TrimSpace(tMap["pbk"]) != "" || strings.TrimSpace(tMap["sid"]) != "" {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "reality transport is not supported by Quantumult X",
			}
		}
		if strings.TrimSpace(tMap["flow"]) != "" {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "xtls flow is not supported by Quantumult X",
			}
		}
		if strings.TrimSpace(tMap["service_name"]) != "" {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "grpc service_name is not supported by Quantumult X",
			}
		}
	}

	netType := strings.ToLower(strings.TrimSpace(tMap["network"]))
	rawTLS := strings.ToLower(strings.TrimSpace(tMap["tls"]))
	tlsEnabled := domain.IsTruthy(rawTLS) || rawTLS == "tls"
	obfs := strings.ToLower(strings.TrimSpace(tMap["obfs"]))
	host := strings.TrimSpace(tMap["host"])
	sni := strings.TrimSpace(tMap["sni"])
	path := strings.TrimSpace(tMap["path"])
	insecure := domain.HasInsecureTransport(tMap)

	if hasUnsafeQXChars(host) || hasUnsafeQXChars(sni) || hasUnsafeQXChars(path) || hasUnsafeQXChars(obfs) {
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  feature,
			Reason:   "invalid characters in transport options",
		}
	}

	switch node.Protocol {
	case domain.ProtocolSS:
		method := strings.ToLower(strings.TrimSpace(node.Credentials.Method))
		password := strings.TrimSpace(node.Credentials.Password)
		if hasUnsafeQXChars(method) || hasUnsafeQXChars(password) {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid characters in shadowsocks credentials",
			}
		}
		if !validQXSSCiphers[method] {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "unsupported cipher in shadowsocks credentials",
			}
		}

		parts := []string{
			fmt.Sprintf("shadowsocks = %s", endpoint),
			"method=" + method,
			"password=" + password,
		}

		switch netType {
		case "", "tcp":
			if obfs != "" {
				if obfs != "http" && obfs != "tls" && obfs != "ws" && obfs != "wss" {
					return "", &CapabilityError{
						Target:   domain.TargetQuantumultX,
						Location: loc,
						Feature:  feature,
						Reason:   "unsupported obfs mode in shadowsocks transport",
					}
				}
			} else if tlsEnabled {
				obfs = "tls"
			}
			if obfs != "" {
				parts = append(parts, "obfs="+obfs)
				obfsHost := host
				if obfsHost == "" {
					obfsHost = sni
				}
				if obfsHost != "" {
					parts = append(parts, "obfs-host="+obfsHost)
				}
				if path != "" && (obfs == "http" || obfs == "ws" || obfs == "wss") {
					parts = append(parts, "obfs-uri="+path)
				}
				if (obfs == "tls" || obfs == "wss") && insecure {
					parts = append(parts, "tls-verification=false")
				}
			}
		case "ws":
			if obfs != "" && obfs != "ws" && obfs != "wss" {
				return "", &CapabilityError{
					Target:   domain.TargetQuantumultX,
					Location: loc,
					Feature:  feature,
					Reason:   "unsupported obfs mode in shadowsocks ws transport",
				}
			}
			mode := "ws"
			if tlsEnabled || obfs == "wss" {
				mode = "wss"
			}
			parts = append(parts, "obfs="+mode)
			if host != "" {
				parts = append(parts, "obfs-host="+host)
			}
			if path != "" {
				parts = append(parts, "obfs-uri="+path)
			}
			if mode == "wss" {
				tlsHost := sni
				if tlsHost == "" {
					tlsHost = host
				}
				if tlsHost != "" {
					parts = append(parts, "tls-host="+tlsHost)
				}
				if insecure {
					parts = append(parts, "tls-verification=false")
				}
			}
		default:
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "unsupported network in shadowsocks transport",
			}
		}

		parts = append(parts, "tag="+tag)
		return strings.Join(parts, ", "), nil

	case domain.ProtocolVMess:
		uuid := strings.TrimSpace(node.Credentials.UUID)
		method := strings.ToLower(strings.TrimSpace(node.Credentials.Method))
		if method == "" {
			method = "auto"
		}
		if hasUnsafeQXChars(uuid) || hasUnsafeQXChars(method) {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid characters in vmess credentials",
			}
		}
		if !validQXVMessMethods[method] {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "unsupported cipher in vmess credentials",
			}
		}

		parts := []string{
			fmt.Sprintf("vmess = %s", endpoint),
			"method=" + method,
			"password=" + uuid,
		}

		switch netType {
		case "", "tcp":
			if obfs == "http" {
				if tlsEnabled {
					return "", &CapabilityError{
						Target:   domain.TargetQuantumultX,
						Location: loc,
						Feature:  feature,
						Reason:   "unsupported http obfs with tls in vmess transport",
					}
				}
				parts = append(parts, "obfs=http")
				if host != "" {
					parts = append(parts, "obfs-host="+host)
				}
				if path != "" {
					parts = append(parts, "obfs-uri="+path)
				}
			} else if obfs != "" && obfs != "over-tls" {
				return "", &CapabilityError{
					Target:   domain.TargetQuantumultX,
					Location: loc,
					Feature:  feature,
					Reason:   "unsupported obfs mode in vmess transport",
				}
			} else if tlsEnabled || obfs == "over-tls" {
				parts = append(parts, "obfs=over-tls")
				tlsHost := sni
				if tlsHost == "" {
					tlsHost = host
				}
				if tlsHost != "" {
					parts = append(parts, "tls-host="+tlsHost)
				}
				if insecure {
					parts = append(parts, "tls-verification=false")
				}
			}
		case "http":
			if tlsEnabled {
				return "", &CapabilityError{
					Target:   domain.TargetQuantumultX,
					Location: loc,
					Feature:  feature,
					Reason:   "unsupported http network with tls in vmess transport",
				}
			}
			parts = append(parts, "obfs=http")
			if host != "" {
				parts = append(parts, "obfs-host="+host)
			}
			if path != "" {
				parts = append(parts, "obfs-uri="+path)
			}
		case "ws":
			if obfs != "" && obfs != "ws" && obfs != "wss" {
				return "", &CapabilityError{
					Target:   domain.TargetQuantumultX,
					Location: loc,
					Feature:  feature,
					Reason:   "unsupported obfs mode in vmess ws transport",
				}
			}
			mode := "ws"
			if tlsEnabled || obfs == "wss" {
				mode = "wss"
			}
			parts = append(parts, "obfs="+mode)
			if host != "" {
				parts = append(parts, "obfs-host="+host)
			}
			if path != "" {
				parts = append(parts, "obfs-uri="+path)
			}
			if mode == "wss" {
				tlsHost := sni
				if tlsHost == "" {
					tlsHost = host
				}
				if tlsHost != "" {
					parts = append(parts, "tls-host="+tlsHost)
				}
				if insecure {
					parts = append(parts, "tls-verification=false")
				}
			}
		default:
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "unsupported network in vmess transport",
			}
		}

		parts = append(parts, "tag="+tag)
		return strings.Join(parts, ", "), nil

	case domain.ProtocolTrojan:
		password := strings.TrimSpace(node.Credentials.Password)
		if hasUnsafeQXChars(password) {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid characters in trojan credentials",
			}
		}
		if rawTLS == "false" || rawTLS == "0" || rawTLS == "no" || rawTLS == "off" {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "trojan requires tls in Quantumult X",
			}
		}

		parts := []string{
			fmt.Sprintf("trojan = %s", endpoint),
			"password=" + password,
		}

		tlsVerify := "true"
		if insecure {
			tlsVerify = "false"
		}
		tlsHost := sni
		if tlsHost == "" {
			tlsHost = host
		}

		switch netType {
		case "", "tcp":
			if obfs != "" && obfs != "over-tls" {
				return "", &CapabilityError{
					Target:   domain.TargetQuantumultX,
					Location: loc,
					Feature:  feature,
					Reason:   "unsupported obfs mode in trojan transport",
				}
			}
			parts = append(parts, "over-tls=true")
			if tlsHost != "" {
				parts = append(parts, "tls-host="+tlsHost)
			}
			parts = append(parts, "tls-verification="+tlsVerify)
		case "ws":
			if obfs != "" && obfs != "ws" && obfs != "wss" {
				return "", &CapabilityError{
					Target:   domain.TargetQuantumultX,
					Location: loc,
					Feature:  feature,
					Reason:   "unsupported obfs mode in trojan ws transport",
				}
			}
			parts = append(parts, "obfs=wss")
			if host != "" {
				parts = append(parts, "obfs-host="+host)
			}
			if path != "" {
				parts = append(parts, "obfs-uri="+path)
			}
			if tlsHost != "" {
				parts = append(parts, "tls-host="+tlsHost)
			}
			parts = append(parts, "tls-verification="+tlsVerify)
		default:
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  feature,
				Reason:   "unsupported network in trojan transport",
			}
		}

		parts = append(parts, "tag="+tag)
		return strings.Join(parts, ", "), nil

	default:
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  feature,
			Reason:   "protocol is not supported",
		}
	}
}

func renderQuantumultXRuleLine(index int, rule resolver.ResolvedRule) (string, error) {
	loc := fmt.Sprintf("rules[%d]", index)
	expr := strings.TrimSpace(rule.Expression)
	kind := ruleKind(expr)
	if kind == "" {
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  "rule",
			Reason:   "routing rule expression is required",
		}
	}

	targetName := strings.TrimSpace(rule.TargetGroupName)
	if targetName == "" || hasUnsafeQXChars(targetName) {
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  kind,
			Reason:   "invalid routing rule target group",
		}
	}
	if isBuiltInPolicyTarget(targetName) {
		if !isQuantumultXBuiltInPolicy(targetName) {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  targetName,
				Reason:   "built-in policy target is not supported by Quantumult X",
			}
		}
		targetName = strings.ToUpper(targetName)
	}

	if kind == "MATCH" {
		parts := strings.Split(expr, ",")
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" || hasUnsafeQXChars(expr) {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  "MATCH",
				Reason:   "MATCH rule does not accept parameters",
			}
		}
		return fmt.Sprintf("final, %s", targetName), nil
	}

	parts := strings.Split(expr, ",")
	if len(parts) != 2 {
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule expression must have exactly one value",
		}
	}
	val := strings.TrimSpace(parts[1])
	if val == "" || hasUnsafeQXChars(val) {
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  kind,
			Reason:   "invalid routing rule value",
		}
	}

	switch kind {
	case "DOMAIN":
		return fmt.Sprintf("host, %s, %s", val, targetName), nil
	case "DOMAIN-SUFFIX":
		return fmt.Sprintf("host-suffix, %s, %s", val, targetName), nil
	case "DOMAIN-KEYWORD":
		return fmt.Sprintf("host-keyword, %s, %s", val, targetName), nil
	case "IP-CIDR":
		prefix, err := netip.ParsePrefix(val)
		if err != nil || !prefix.Addr().Is4() {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  "IP-CIDR",
				Reason:   "invalid IPv4 CIDR in IP-CIDR rule",
			}
		}
		return fmt.Sprintf("ip-cidr, %s, %s", val, targetName), nil
	case "IP-CIDR6":
		prefix, err := netip.ParsePrefix(val)
		if err != nil || !prefix.Addr().Is6() {
			return "", &CapabilityError{
				Target:   domain.TargetQuantumultX,
				Location: loc,
				Feature:  "IP-CIDR6",
				Reason:   "invalid IPv6 CIDR in IP-CIDR6 rule",
			}
		}
		return fmt.Sprintf("ip6-cidr, %s, %s", val, targetName), nil
	case "GEOIP":
		return fmt.Sprintf("geoip, %s, %s", val, targetName), nil
	default:
		return "", &CapabilityError{
			Target:   domain.TargetQuantumultX,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule kind is not supported",
		}
	}
}
