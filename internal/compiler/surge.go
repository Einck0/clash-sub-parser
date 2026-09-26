package compiler

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
)

const (
	surgeDefaultTestURL   = "http://www.gstatic.com/generate_204"
	surgeDefaultInterval  = 300
	surgeDefaultTimeout   = 5
	surgeDefaultTolerance = 50
)

var surgeSupportedSSCiphers = map[string]bool{
	"aes-128-gcm":             true,
	"aes-192-gcm":             true,
	"aes-256-gcm":             true,
	"chacha20-ietf-poly1305":  true,
	"xchacha20-ietf-poly1305": true,
	"2022-blake3-aes-128-gcm": true,
	"2022-blake3-aes-256-gcm": true,
	"rc4-md5":                 true,
	"aes-128-cfb":             true,
	"aes-192-cfb":             true,
	"aes-256-cfb":             true,
	"aes-128-ctr":             true,
	"aes-192-ctr":             true,
	"aes-256-ctr":             true,
	"bf-cfb":                  true,
	"camellia-128-cfb":        true,
	"camellia-192-cfb":        true,
	"camellia-256-cfb":        true,
	"salsa20":                 true,
	"chacha20":                true,
	"chacha20-ietf":           true,
	"none":                    true,
}

var surgeSupportedVMessCiphers = map[string]bool{
	"":                       true,
	"auto":                   true,
	"aes-128-gcm":            true,
	"chacha20-poly1305":      true,
	"chacha20-ietf-poly1305": true,
	"none":                   true,
	"zero":                   true,
}

// surgeCapability declares the protocols supported by Surge 5 node-only export.
func surgeCapability() Capability {
	return Capability{
		Protocols: protocolSet(
			domain.ProtocolSS,
			domain.ProtocolVMess,
			domain.ProtocolTrojan,
			domain.ProtocolHysteria2,
			domain.ProtocolTUIC,
			domain.ProtocolWireGuard,
		),
		GroupTypes: groupSet(),
		RuleKinds:  ruleSet(),
	}
}

func renderSurge(snapshot *resolver.ResolvedPolicySnapshot) ([]byte, error) {
	var b strings.Builder

	for i, node := range snapshot.Nodes {
		line, err := renderSurgeProxyLine(i, node)
		if err != nil {
			return nil, err
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	if b.Len() == 0 {
		b.WriteByte('\n')
	}

	return []byte(b.String()), nil
}

func renderSurgeProxyLine(index int, node resolver.ResolvedNode) (string, error) {
	loc := fmt.Sprintf("nodes[%d]", index)
	feature := string(node.Protocol)

	if err := validateCredentialEnvelope(domain.TargetSurge, index, node); err != nil {
		return "", err
	}

	name := strings.TrimSpace(node.DisplayName)
	if !isSurgeSafeIdentifier(name) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  feature,
			Reason:   "node display name contains characters invalid in Surge [Proxy] line",
		}
	}

	server := strings.TrimSpace(node.Server)
	if !isSurgeSafeToken(server) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  feature,
			Reason:   "server address contains characters invalid in Surge [Proxy] line",
		}
	}

	c := node.Credentials
	transport := c.Transport

	if hasSurgeUnsupportedRealityOrFlow(transport) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  feature,
			Reason:   "reality or xtls flow transport is not supported by surge",
		}
	}

	switch node.Protocol {
	case domain.ProtocolSS:
		return renderSurgeSSProxy(loc, name, server, node.Port, c)
	case domain.ProtocolVMess:
		return renderSurgeVMessProxy(loc, name, server, node.Port, c)
	case domain.ProtocolTrojan:
		return renderSurgeTrojanProxy(loc, name, server, node.Port, c)
	case domain.ProtocolHysteria2:
		return renderSurgeHysteria2Proxy(loc, name, server, node.Port, c)
	case domain.ProtocolTUIC:
		return renderSurgeTUICProxy(loc, name, server, node.Port, c)
	case domain.ProtocolWireGuard:
		return renderSurgeWireGuardProxy(loc, name, server, node.Port, c)
	default:
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  feature,
			Reason:   "protocol is not supported",
		}
	}
}

func renderSurgeHysteria2Proxy(loc, name, server string, port int, c domain.InboundProtocolCredential) (string, error) {
	password := strings.TrimSpace(c.Password)
	if !isSurgeSafeValue(password) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolHysteria2),
			Reason:   "hysteria2 password contains characters invalid in Surge [Proxy] line",
		}
	}

	transport := c.Transport
	netMode := strings.ToLower(strings.TrimSpace(transport["network"]))
	if netMode != "" && netMode != "quic" && netMode != "udp" && netMode != "tcp" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolHysteria2),
			Reason:   fmt.Sprintf("unsupported network %q in hysteria2 transport", netMode),
		}
	}

	params := []string{
		fmt.Sprintf("%s = hysteria2", name),
		server,
		strconv.Itoa(port),
		"password=" + password,
	}

	if downRaw := strings.TrimSpace(transport["down"]); downRaw != "" {
		downMbps, err := parseSurgeBandwidthMbps(downRaw)
		if err != nil {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolHysteria2),
				Reason:   err.Error(),
			}
		}
		params = append(params, fmt.Sprintf("download-bandwidth=%d", downMbps))
	}

	sni := surgeFirstNonEmpty(c.SNI, transport["sni"], transport["servername"], transport["serverName"], transport["peer"])
	if sni != "" {
		if !isSurgeSafeValue(sni) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolHysteria2),
				Reason:   "invalid sni in hysteria2 transport",
			}
		}
		params = append(params, "sni="+sni)
	}

	if domain.HasInsecureTransport(transport) {
		params = append(params, "skip-cert-verify=true")
	}

	return strings.Join(params, ", "), nil
}

func renderSurgeTUICProxy(loc, name, server string, port int, c domain.InboundProtocolCredential) (string, error) {
	uuid := strings.TrimSpace(c.UUID)
	password := strings.TrimSpace(c.Password)
	if !isSurgeSafeValue(uuid) || !isSurgeSafeValue(password) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolTUIC),
			Reason:   "tuic credentials contain characters invalid in Surge [Proxy] line",
		}
	}

	transport := c.Transport
	netMode := strings.ToLower(strings.TrimSpace(transport["network"]))
	if netMode != "" && netMode != "quic" && netMode != "udp" && netMode != "tcp" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolTUIC),
			Reason:   fmt.Sprintf("unsupported network %q in tuic transport", netMode),
		}
	}

	params := []string{
		fmt.Sprintf("%s = tuic", name),
		server,
		strconv.Itoa(port),
		"uuid=" + uuid,
		"password=" + password,
	}

	sni := surgeFirstNonEmpty(c.SNI, transport["sni"], transport["servername"])
	if sni != "" {
		if !isSurgeSafeValue(sni) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolTUIC),
				Reason:   "invalid sni in tuic transport",
			}
		}
		params = append(params, "sni="+sni)
	}

	var alpnList []string
	for _, a := range c.ALPN {
		if trimmed := strings.TrimSpace(a); trimmed != "" {
			alpnList = append(alpnList, trimmed)
		}
	}
	if len(alpnList) == 0 && transport != nil {
		for _, a := range strings.Split(transport["alpn"], ",") {
			if trimmed := strings.TrimSpace(a); trimmed != "" {
				alpnList = append(alpnList, trimmed)
			}
		}
	}
	if len(alpnList) > 0 {
		alpnJoined := strings.Join(alpnList, ":")
		if !isSurgeSafeValue(alpnJoined) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolTUIC),
				Reason:   "invalid alpn in tuic transport",
			}
		}
		params = append(params, "alpn="+alpnJoined)
	}

	if domain.HasInsecureTransport(transport) {
		params = append(params, "skip-cert-verify=true")
	}

	return strings.Join(params, ", "), nil
}

func renderSurgeWireGuardProxy(loc, name, server string, port int, c domain.InboundProtocolCredential) (string, error) {
	privateKey := strings.TrimSpace(c.PrivateKey)
	publicKey := strings.TrimSpace(c.PublicKey)
	if !isSurgeSafeValue(privateKey) || !isSurgeSafeValue(publicKey) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolWireGuard),
			Reason:   "wireguard keys contain characters invalid in Surge [Proxy] line",
		}
	}

	var selfIP, selfIPv6 string
	for _, rawAddr := range c.LocalAddress {
		pfx, err := netip.ParsePrefix(strings.TrimSpace(rawAddr))
		if err != nil {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolWireGuard),
				Reason:   "invalid local_address CIDR in wireguard credentials",
			}
		}
		if pfx.Addr().Is4() && selfIP == "" {
			selfIP = pfx.Addr().String()
		} else if pfx.Addr().Is6() && selfIPv6 == "" {
			selfIPv6 = pfx.Addr().String()
		}
	}

	params := []string{
		fmt.Sprintf("%s = wireguard", name),
		server,
		strconv.Itoa(port),
	}
	if selfIP != "" {
		params = append(params, "self-ip="+selfIP)
	}
	if selfIPv6 != "" {
		params = append(params, "self-ip-v6="+selfIPv6)
	}
	params = append(params, "private-key="+privateKey, "peer-public-key="+publicKey)

	if psk := strings.TrimSpace(c.PreSharedKey); psk != "" {
		if !isSurgeSafeValue(psk) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolWireGuard),
				Reason:   "wireguard pre-shared-key contains characters invalid in Surge [Proxy] line",
			}
		}
		params = append(params, "preshared-key="+psk)
	}
	if len(c.Reserved) == 3 {
		params = append(params, fmt.Sprintf("client-id=%d/%d/%d", c.Reserved[0], c.Reserved[1], c.Reserved[2]))
	}
	if c.MTU > 0 {
		params = append(params, fmt.Sprintf("mtu=%d", c.MTU))
	}

	return strings.Join(params, ", "), nil
}

func parseSurgeBandwidthMbps(raw string) (int, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	for _, suffix := range []string{"mbps", "m"} {
		if strings.HasSuffix(normalized, suffix) {
			normalized = strings.TrimSpace(strings.TrimSuffix(normalized, suffix))
			break
		}
	}
	val, err := strconv.Atoi(normalized)
	if err != nil || val <= 0 {
		return 0, fmt.Errorf("invalid hysteria2 bandwidth %q", raw)
	}
	return val, nil
}

func renderSurgeSSProxy(loc, name, server string, port int, c domain.InboundProtocolCredential) (string, error) {
	cipher := strings.ToLower(strings.TrimSpace(c.Method))
	if !surgeSupportedSSCiphers[cipher] {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolSS),
			Reason:   "unsupported shadowsocks encrypt-method for surge",
		}
	}

	password := strings.TrimSpace(c.Password)
	if !isSurgeSafeValue(password) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolSS),
			Reason:   "shadowsocks password contains characters invalid in Surge [Proxy] line",
		}
	}

	transport := c.Transport
	netMode := strings.ToLower(strings.TrimSpace(transport["network"]))
	if netMode != "" && netMode != "tcp" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolSS),
			Reason:   fmt.Sprintf("unsupported network %q in shadowsocks transport", netMode),
		}
	}
	if strings.TrimSpace(transport["service_name"]) != "" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolSS),
			Reason:   "grpc transport is not supported in shadowsocks for surge",
		}
	}
	if domain.IsTruthy(transport["tls"]) || strings.EqualFold(strings.TrimSpace(transport["security"]), "tls") {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolSS),
			Reason:   "standalone tls wrapper is not supported in shadowsocks for surge",
		}
	}

	params := []string{
		fmt.Sprintf("%s = ss", name),
		server,
		strconv.Itoa(port),
		"encrypt-method=" + cipher,
		"password=" + password,
	}

	obfs := strings.ToLower(strings.TrimSpace(transport["obfs"]))
	if obfs != "" {
		if obfs != "http" && obfs != "tls" {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolSS),
				Reason:   fmt.Sprintf("unsupported shadowsocks obfs mode %q for surge", obfs),
			}
		}
		params = append(params, "obfs="+obfs)
		if host := strings.TrimSpace(transport["host"]); host != "" {
			if !isSurgeSafeValue(host) {
				return "", &CapabilityError{
					Target:   domain.TargetSurge,
					Location: loc,
					Feature:  string(domain.ProtocolSS),
					Reason:   "invalid obfs-host in shadowsocks transport",
				}
			}
			params = append(params, "obfs-host="+host)
		}
		if path := strings.TrimSpace(transport["path"]); path != "" {
			if !isSurgeSafeValue(path) {
				return "", &CapabilityError{
					Target:   domain.TargetSurge,
					Location: loc,
					Feature:  string(domain.ProtocolSS),
					Reason:   "invalid obfs-uri in shadowsocks transport",
				}
			}
			params = append(params, "obfs-uri="+path)
		}
	} else if strings.TrimSpace(transport["path"]) != "" || strings.TrimSpace(transport["host"]) != "" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolSS),
			Reason:   "transport host or path on shadowsocks requires obfs mode for surge",
		}
	}

	return strings.Join(params, ", "), nil
}

func renderSurgeVMessProxy(loc, name, server string, port int, c domain.InboundProtocolCredential) (string, error) {
	uuid := strings.TrimSpace(c.UUID)
	if !isSurgeSafeValue(uuid) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolVMess),
			Reason:   "vmess uuid contains characters invalid in Surge [Proxy] line",
		}
	}

	cipher := strings.ToLower(strings.TrimSpace(c.Method))
	if !surgeSupportedVMessCiphers[cipher] {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolVMess),
			Reason:   "unsupported vmess cipher for surge",
		}
	}

	transport := c.Transport
	netMode := strings.ToLower(strings.TrimSpace(transport["network"]))
	if netMode != "" && netMode != "tcp" && netMode != "ws" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolVMess),
			Reason:   fmt.Sprintf("unsupported network %q in vmess transport", netMode),
		}
	}
	if strings.TrimSpace(transport["service_name"]) != "" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolVMess),
			Reason:   "grpc service_name is not supported in vmess transport for surge",
		}
	}

	aead := "true"
	if c.AlterID > 0 {
		aead = "false"
	}

	params := []string{
		fmt.Sprintf("%s = vmess", name),
		server,
		strconv.Itoa(port),
		"username=" + uuid,
		"vmess-aead=" + aead,
	}

	wsParams, err := buildSurgeWSParams(loc, string(domain.ProtocolVMess), netMode, transport)
	if err != nil {
		return "", err
	}
	params = append(params, wsParams...)

	tlsEnabled := domain.IsTruthy(transport["tls"]) || strings.EqualFold(strings.TrimSpace(transport["security"]), "tls")
	if tlsEnabled {
		params = append(params, "tls=true")
	}

	sni := surgeFirstNonEmpty(c.SNI, transport["sni"], transport["servername"])
	if sni != "" {
		if !isSurgeSafeValue(sni) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolVMess),
				Reason:   "invalid sni in vmess transport",
			}
		}
		params = append(params, "sni="+sni)
	}

	if domain.HasInsecureTransport(transport) {
		params = append(params, "skip-cert-verify=true")
	}

	return strings.Join(params, ", "), nil
}

func renderSurgeTrojanProxy(loc, name, server string, port int, c domain.InboundProtocolCredential) (string, error) {
	password := strings.TrimSpace(c.Password)
	if !isSurgeSafeValue(password) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolTrojan),
			Reason:   "trojan password contains characters invalid in Surge [Proxy] line",
		}
	}

	transport := c.Transport
	netMode := strings.ToLower(strings.TrimSpace(transport["network"]))
	if netMode != "" && netMode != "tcp" && netMode != "ws" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolTrojan),
			Reason:   fmt.Sprintf("unsupported network %q in trojan transport", netMode),
		}
	}
	if strings.TrimSpace(transport["service_name"]) != "" {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(domain.ProtocolTrojan),
			Reason:   "grpc service_name is not supported in trojan transport for surge",
		}
	}

	params := []string{
		fmt.Sprintf("%s = trojan", name),
		server,
		strconv.Itoa(port),
		"password=" + password,
	}

	wsParams, err := buildSurgeWSParams(loc, string(domain.ProtocolTrojan), netMode, transport)
	if err != nil {
		return "", err
	}
	params = append(params, wsParams...)

	sni := surgeFirstNonEmpty(c.SNI, transport["sni"], transport["servername"])
	if sni != "" {
		if !isSurgeSafeValue(sni) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  string(domain.ProtocolTrojan),
				Reason:   "invalid sni in trojan transport",
			}
		}
		params = append(params, "sni="+sni)
	}

	if domain.HasInsecureTransport(transport) {
		params = append(params, "skip-cert-verify=true")
	}

	return strings.Join(params, ", "), nil
}

func buildSurgeWSParams(loc, feature, netMode string, transport map[string]string) ([]string, error) {
	path := strings.TrimSpace(transport["path"])
	host := strings.TrimSpace(transport["host"])
	if netMode != "ws" {
		if path != "" || host != "" {
			return nil, &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  feature,
				Reason:   "transport path or host requires ws network for surge",
			}
		}
		return nil, nil
	}

	params := []string{"ws=true"}
	if path != "" {
		if !isSurgeSafeValue(path) {
			return nil, &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid ws-path in transport",
			}
		}
		params = append(params, "ws-path="+path)
	}
	if host != "" {
		if !isSurgeSafeValue(host) || strings.ContainsAny(host, ": ") {
			return nil, &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  feature,
				Reason:   "invalid ws-headers host in transport",
			}
		}
		params = append(params, "ws-headers=Host:"+host)
	}
	return params, nil
}

func renderSurgeGroupLine(index int, group resolver.ResolvedGroup) (string, error) {
	loc := fmt.Sprintf("groups[%d]", index)
	name := strings.TrimSpace(group.Name)
	if !isSurgeSafeIdentifier(name) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(group.GroupType),
			Reason:   "policy group name contains characters invalid in Surge [Proxy Group] line",
		}
	}

	members := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		memberName := strings.TrimSpace(member.DisplayName)
		if !isSurgeSafeIdentifier(memberName) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  memberName,
				Reason:   "group member name contains characters invalid in Surge [Proxy Group] line",
			}
		}
		members = append(members, memberName)
	}
	if len(members) == 0 {
		members = []string{"DIRECT"}
	}
	memberJoined := strings.Join(members, ", ")

	switch group.GroupType {
	case domain.GroupTypeSelect:
		return fmt.Sprintf("%s = select, %s", name, memberJoined), nil
	case domain.GroupTypeURLTest:
		return fmt.Sprintf(
			"%s = url-test, %s, url=%s, interval=%d, timeout=%d, tolerance=%d",
			name,
			memberJoined,
			surgeDefaultTestURL,
			surgeDefaultInterval,
			surgeDefaultTimeout,
			surgeDefaultTolerance,
		), nil
	case domain.GroupTypeFallback:
		return fmt.Sprintf(
			"%s = fallback, %s, url=%s, interval=%d, timeout=%d",
			name,
			memberJoined,
			surgeDefaultTestURL,
			surgeDefaultInterval,
			surgeDefaultTimeout,
		), nil
	default:
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  string(group.GroupType),
			Reason:   "policy group type is not supported",
		}
	}
}

func renderSurgeRuleLine(index, totalRules int, rule resolver.ResolvedRule) (string, error) {
	loc := fmt.Sprintf("rules[%d]", index)
	expr := strings.TrimSpace(rule.Expression)
	kind := ruleKind(expr)
	targetGroup := strings.TrimSpace(rule.TargetGroupName)
	if !isSurgeSafeIdentifier(targetGroup) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule target group contains characters invalid in Surge [Rule] line",
		}
	}

	if domain.IsMatchRule(expr) {
		if index != totalRules-1 {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   "terminal MATCH/FINAL rule must be positioned last in Surge [Rule]",
			}
		}
		parts := strings.Split(expr, ",")
		if len(parts) == 1 {
			return fmt.Sprintf("FINAL, %s", targetGroup), nil
		}
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[1]), "dns-failed") {
			return fmt.Sprintf("FINAL, %s, dns-failed", targetGroup), nil
		}
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  kind,
			Reason:   "unsupported option on FINAL/MATCH rule for surge",
		}
	}

	parts := strings.Split(expr, ",")
	if len(parts) < 2 {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule value is required",
		}
	}
	val := strings.TrimSpace(parts[1])
	if val == "" || !isSurgeSafeValue(val) {
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule value is invalid for surge",
		}
	}
	extra := parts[2:]

	switch kind {
	case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD":
		if strings.ContainsAny(val, " \t") {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   "domain rule value must not contain whitespace",
			}
		}
		if len(extra) == 0 {
			return fmt.Sprintf("%s,%s, %s", kind, val, targetGroup), nil
		}
		if len(extra) == 1 && strings.EqualFold(strings.TrimSpace(extra[0]), "extended-matching") {
			return fmt.Sprintf("%s,%s, %s, extended-matching", kind, val, targetGroup), nil
		}
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  kind,
			Reason:   "unsupported domain rule option for surge",
		}

	case "IP-CIDR":
		pfx, err := netip.ParsePrefix(val)
		if err != nil || !pfx.Addr().Is4() {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   "invalid IPv4 CIDR in IP-CIDR rule for surge",
			}
		}
		return formatSurgeRuleWithOptionalNoResolve(loc, kind, val, targetGroup, extra)

	case "IP-CIDR6":
		pfx, err := netip.ParsePrefix(val)
		if err != nil || !pfx.Addr().Is6() {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   "invalid IPv6 CIDR in IP-CIDR6 rule for surge",
			}
		}
		return formatSurgeRuleWithOptionalNoResolve(loc, kind, val, targetGroup, extra)

	case "GEOIP":
		if !isASCIIAlphaCode(val) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   "invalid country code in GEOIP rule for surge",
			}
		}
		return formatSurgeRuleWithOptionalNoResolve(loc, kind, strings.ToUpper(val), targetGroup, extra)

	case "PROCESS-NAME", "USER-AGENT", "URL-REGEX":
		if len(extra) > 0 {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("unsupported extra option in %s rule for surge", kind),
			}
		}
		return fmt.Sprintf("%s,%s, %s", kind, val, targetGroup), nil

	case "SRC-IP":
		if len(extra) > 0 {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   "unsupported extra option in SRC-IP rule for surge",
			}
		}
		if _, err := netip.ParseAddr(val); err != nil {
			if _, pfxErr := netip.ParsePrefix(val); pfxErr != nil {
				return "", &CapabilityError{
					Target:   domain.TargetSurge,
					Location: loc,
					Feature:  kind,
					Reason:   "invalid IP or CIDR in SRC-IP rule for surge",
				}
			}
		}
		return fmt.Sprintf("SRC-IP,%s, %s", val, targetGroup), nil

	case "DEST-PORT", "IN-PORT":
		if len(extra) > 0 {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("unsupported extra option in %s rule for surge", kind),
			}
		}
		if !isValidSurgePortSpec(val) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   fmt.Sprintf("invalid port specification in %s rule for surge", kind),
			}
		}
		return fmt.Sprintf("%s,%s, %s", kind, val, targetGroup), nil

	case "RULE-SET":
		if !isValidSurgeRuleSetSource(val) {
			return "", &CapabilityError{
				Target:   domain.TargetSurge,
				Location: loc,
				Feature:  kind,
				Reason:   "Surge RULE-SET requires an http/https URL or built-in SYSTEM/LAN source",
			}
		}
		return formatSurgeRuleWithOptionalNoResolve(loc, kind, val, targetGroup, extra)

	default:
		return "", &CapabilityError{
			Target:   domain.TargetSurge,
			Location: loc,
			Feature:  kind,
			Reason:   "routing rule kind is not supported",
		}
	}
}

func formatSurgeRuleWithOptionalNoResolve(loc, kind, val, targetGroup string, extra []string) (string, error) {
	if len(extra) == 0 {
		return fmt.Sprintf("%s,%s, %s", kind, val, targetGroup), nil
	}
	if len(extra) == 1 && strings.EqualFold(strings.TrimSpace(extra[0]), "no-resolve") {
		return fmt.Sprintf("%s,%s, %s, no-resolve", kind, val, targetGroup), nil
	}
	return "", &CapabilityError{
		Target:   domain.TargetSurge,
		Location: loc,
		Feature:  kind,
		Reason:   fmt.Sprintf("unsupported option in %s rule for surge", kind),
	}
}

func hasSurgeUnsupportedRealityOrFlow(transport map[string]string) bool {
	if transport == nil {
		return false
	}
	for _, k := range []string{"pbk", "sid", "flow"} {
		if strings.TrimSpace(transport[k]) != "" {
			return true
		}
	}
	return strings.EqualFold(strings.TrimSpace(transport["security"]), "reality")
}

func isSurgeSafeIdentifier(s string) bool {
	return s != "" && !strings.ContainsAny(s, "\r\n\x00,=")
}

func isSurgeSafeToken(s string) bool {
	return s != "" && !strings.ContainsAny(s, "\r\n\x00,= \t")
}

func isSurgeSafeValue(s string) bool {
	return s != "" && !strings.ContainsAny(s, "\r\n\x00,")
}

func isASCIIAlphaCode(s string) bool {
	if len(s) < 2 || len(s) > 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

func isValidSurgePortSpec(s string) bool {
	if before, after, ok := strings.Cut(s, "-"); ok {
		start, err1 := strconv.Atoi(strings.TrimSpace(before))
		end, err2 := strconv.Atoi(strings.TrimSpace(after))
		return err1 == nil && err2 == nil && start >= 1 && start <= end && end <= 65535
	}
	port, err := strconv.Atoi(s)
	return err == nil && port >= 1 && port <= 65535
}

func isValidSurgeRuleSetSource(s string) bool {
	upper := strings.ToUpper(strings.TrimSpace(s))
	if upper == "SYSTEM" || upper == "LAN" {
		return true
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

func surgeFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
