package parser

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

// ParsedNodeWithCredentials holds a normalized node alongside its extracted protocol-specific credentials.
type ParsedNodeWithCredentials struct {
	Normalized  NormalizedNode
	Credentials domain.InboundProtocolCredential
}

// ExtractResult reports parsed nodes with extracted credentials, and counts of rejected entries.
type ExtractResult struct {
	Items    []ParsedNodeWithCredentials
	Rejected int
}

// ExtractWithCredentials parses subscription content, extracting both normalized representation and in-memory credentials.
func ExtractWithCredentials(content []byte) (ExtractResult, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return ExtractResult{}, domain.NewValidationError("empty_subscription", "subscription content is empty")
	}

	if items, ok, err := extractYAML([]byte(trimmed)); ok {
		return items, err
	}
	if decoded, ok := decodeBase64(trimmed); ok {
		return extractURLLines(decoded)
	}
	return extractURLLines(trimmed)
}

func extractYAML(content []byte) (ExtractResult, bool, error) {
	var document struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(content, &document); err != nil || document.Proxies == nil {
		return ExtractResult{}, false, nil
	}

	result := ExtractResult{Items: make([]ParsedNodeWithCredentials, 0, len(document.Proxies))}
	for _, proxy := range document.Proxies {
		node, creds, err := extractYAMLProxy(proxy)
		if err != nil {
			result.Rejected++
			continue
		}
		result.Items = append(result.Items, ParsedNodeWithCredentials{
			Normalized:  node,
			Credentials: creds,
		})
	}
	if len(result.Items) == 0 {
		return ExtractResult{}, true, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
	}
	return result, true, nil
}

// ExtractProxyMap parses a single proxy map into a NormalizedNode and domain.InboundProtocolCredential.
func ExtractProxyMap(proxy map[string]any) (NormalizedNode, domain.InboundProtocolCredential, error) {
	return extractYAMLProxy(proxy)
}

func extractYAMLProxy(proxy map[string]any) (NormalizedNode, domain.InboundProtocolCredential, error) {
	protocol, err := protocolFor(value(proxy, "type"))
	if err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}
	server, port, err := endpoint(value(proxy, "server"), value(proxy, "port"))
	if err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}
	transport := yamlTransport(proxy, protocol)

	psk := value(proxy, "pre-shared-key", "pre_shared_key", "preshared-key", "preshared_key", "psk")
	pubKey := value(proxy, "public-key", "public_key", "peer-public-key", "peer_public_key")
	rawReserved := proxy["reserved"]
	if peers, ok := proxy["peers"].([]any); ok && len(peers) > 0 {
		if firstPeer, ok := peers[0].(map[string]any); ok {
			if pubKey == "" {
				pubKey = value(firstPeer, "public-key", "public_key")
			}
			if psk == "" {
				psk = value(firstPeer, "pre-shared-key", "pre_shared_key", "preshared-key", "preshared_key", "psk")
			}
			if rawReserved == nil {
				rawReserved = firstPeer["reserved"]
			}
		}
	}

	var alterID int
	if aidStr := value(proxy, "alterId", "alter_id", "aid"); aidStr != "" {
		if v, convErr := strconv.Atoi(aidStr); convErr == nil && v >= 0 {
			alterID = v
		}
	}

	creds := domain.InboundProtocolCredential{
		Password:     value(proxy, "password"),
		UUID:         value(proxy, "uuid"),
		Method:       value(proxy, "cipher"),
		AlterID:      alterID,
		PrivateKey:   value(proxy, "private-key", "private_key"),
		PublicKey:    pubKey,
		PresharedKey: psk,
		PreSharedKey: psk,
		Username:     value(proxy, "username", "user"),
		Transport:    transport,
	}

	if protocol == domain.ProtocolWireGuard {
		localAddrs, addrErr := parseWireGuardAddresses(
			proxy["local-address"],
			proxy["local_address"],
			proxy["address"],
			value(proxy, "ip"),
			value(proxy, "ipv6"),
		)
		if addrErr != nil {
			return NormalizedNode{}, domain.InboundProtocolCredential{}, addrErr
		}
		creds.LocalAddress = localAddrs

		reserved, resErr := parseWireGuardReserved(rawReserved)
		if resErr != nil {
			return NormalizedNode{}, domain.InboundProtocolCredential{}, resErr
		}
		creds.Reserved = reserved

		if mtuStr := value(proxy, "mtu"); mtuStr != "" {
			mtuVal, mtuErr := strconv.Atoi(mtuStr)
			if mtuErr != nil || mtuVal <= 0 || mtuVal > 65535 {
				return NormalizedNode{}, domain.InboundProtocolCredential{}, fmt.Errorf("invalid WireGuard mtu %q", mtuStr)
			}
			creds.MTU = mtuVal
		}
		creds.DNS = parseStringList(proxy["dns"])
	}

	if protocol == domain.ProtocolTUIC {
		creds.CongestionControl = value(proxy, "congestion-controller", "congestion_controller", "congestion-control", "congestion_control")
		creds.UDPRelayMode = value(proxy, "udp-relay-mode", "udp_relay_mode")
		creds.ALPN = parseStringList(proxy["alpn"])
		creds.SNI = value(proxy, "sni", "servername", "serverName")
		creds.DisableSNI = domain.IsTruthy(value(proxy, "disable-sni", "disable_sni"))
	}

	if err := validateCredentials(protocol, creds); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	norm := newNormalizedNode(protocol, value(proxy, "name"), server, port, transport, creds)
	return norm, norm.Credentials, nil
}

func extractURLLines(content string) (ExtractResult, error) {
	result := ExtractResult{}
	for _, line := range strings.Fields(content) {
		node, creds, err := extractURL(line)
		if err != nil {
			result.Rejected++
			continue
		}
		result.Items = append(result.Items, ParsedNodeWithCredentials{
			Normalized:  node,
			Credentials: creds,
		})
	}
	if len(result.Items) == 0 {
		return ExtractResult{}, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
	}
	return result, nil
}

func extractURL(raw string) (NormalizedNode, domain.InboundProtocolCredential, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, fmt.Errorf("invalid subscription URL")
	}
	protocol, err := protocolFor(u.Scheme)
	if err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}
	if protocol == domain.ProtocolVMess {
		return extractVMess(raw)
	}
	server, port, err := endpoint(u.Hostname(), u.Port())
	if err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}
	transport := urlTransport(u, protocol)

	query := u.Query()
	creds := domain.InboundProtocolCredential{
		Transport: transport,
	}

	if u.User != nil {
		creds.Username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			creds.Password = pw
		}
	}

	switch protocol {
	case domain.ProtocolSS:
		if u.User != nil {
			if decoded, ok := decodeBase64(u.User.Username()); ok {
				parts := strings.SplitN(decoded, ":", 2)
				if len(parts) == 2 {
					creds.Method = parts[0]
					creds.Password = parts[1]
				}
			} else if creds.Password != "" {
				creds.Method = creds.Username
			}
		}
	case domain.ProtocolVLESS:
		if u.User != nil {
			creds.UUID = u.User.Username()
		}
		if creds.UUID == "" {
			creds.UUID = firstQuery(query, "uuid")
		}
	case domain.ProtocolTrojan:
		if creds.Password == "" && u.User != nil {
			creds.Password = u.User.Username()
		}
	case domain.ProtocolHysteria2:
		if creds.Password == "" && u.User != nil {
			creds.Password = u.User.Username()
		}
		if creds.Password == "" {
			creds.Password = firstQuery(query, "password", "auth")
		}
	case domain.ProtocolWireGuard:
		if u.User != nil && u.User.Username() != "" {
			creds.PrivateKey = u.User.Username()
		}
		if creds.PrivateKey == "" {
			creds.PrivateKey = firstQuery(query, "private_key", "private-key", "privateKey")
		}
		creds.PublicKey = firstQuery(query, "public_key", "public-key", "publicKey", "peer_public_key", "peer-public-key")
		psk := firstQuery(query, "pre_shared_key", "pre-shared-key", "preshared_key", "preshared-key", "psk", "preSharedKey", "presharedKey")
		creds.PresharedKey = psk
		creds.PreSharedKey = psk

		localAddrs, addrErr := parseWireGuardAddresses(
			query["local_address"],
			query["local-address"],
			query["address"],
			query["ip"],
			query["ipv6"],
		)
		if addrErr != nil {
			return NormalizedNode{}, domain.InboundProtocolCredential{}, addrErr
		}
		creds.LocalAddress = localAddrs

		if rawRes := firstQuery(query, "reserved"); rawRes != "" {
			reserved, resErr := parseWireGuardReserved(rawRes)
			if resErr != nil {
				return NormalizedNode{}, domain.InboundProtocolCredential{}, resErr
			}
			creds.Reserved = reserved
		}
		if mtuStr := firstQuery(query, "mtu"); mtuStr != "" {
			mtuVal, mtuErr := strconv.Atoi(mtuStr)
			if mtuErr != nil || mtuVal <= 0 || mtuVal > 65535 {
				return NormalizedNode{}, domain.InboundProtocolCredential{}, fmt.Errorf("invalid WireGuard mtu %q", mtuStr)
			}
			creds.MTU = mtuVal
		}
		creds.DNS = parseStringList(query["dns"])
	case domain.ProtocolTUIC:
		if u.User != nil {
			creds.UUID = u.User.Username()
			if pw, ok := u.User.Password(); ok {
				creds.Password = pw
			}
		}
		if creds.UUID == "" {
			creds.UUID = firstQuery(query, "uuid")
		}
		if creds.Password == "" {
			creds.Password = firstQuery(query, "password")
		}
		creds.CongestionControl = firstQuery(query, "congestion_control", "congestion-control", "congestion_controller", "congestion-controller")
		creds.UDPRelayMode = firstQuery(query, "udp_relay_mode", "udp-relay-mode")
		creds.ALPN = parseStringList(query["alpn"])
		creds.SNI = firstQuery(query, "sni", "servername", "serverName", "peer")
		creds.DisableSNI = domain.IsTruthy(firstQuery(query, "disable_sni", "disable-sni"))
	}

	if err := validateCredentials(protocol, creds); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	norm := newNormalizedNode(protocol, fragmentName(u), server, port, transport, creds)
	return norm, norm.Credentials, nil
}

func extractVMess(raw string) (NormalizedNode, domain.InboundProtocolCredential, error) {
	encoded := strings.TrimPrefix(raw, "vmess://")
	decoded, ok := decodeBase64(encoded)
	if !ok {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, fmt.Errorf("invalid VMess payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(decoded), &payload); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, fmt.Errorf("invalid VMess payload")
	}
	server, port, err := endpoint(value(payload, "add", "server"), value(payload, "port"))
	if err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}
	transport := map[string]string{"network": strings.ToLower(defaultValue(value(payload, "net", "network", "anet"), "tcp"))}
	copyIfPresent(transport, "tls", value(payload, "tls", "security"))
	if tlsValue := strings.ToLower(transport["tls"]); domain.IsTruthy(tlsValue) || tlsValue == "tls" || tlsValue == "reality" {
		transport["tls"] = "true"
	} else {
		delete(transport, "tls")
	}
	copyIfPresent(transport, "sni", value(payload, "sni"))
	copyIfPresent(transport, "host", value(payload, "host"))
	copyIfPresent(transport, "path", value(payload, "path"))

	uuid := value(payload, "id", "uuid")
	var alterID int
	if aidStr := value(payload, "aid", "alterId", "alter_id"); aidStr != "" {
		if v, convErr := strconv.Atoi(aidStr); convErr == nil && v >= 0 {
			alterID = v
		}
	}

	creds := domain.InboundProtocolCredential{
		UUID:      uuid,
		Method:    value(payload, "scy", "security"),
		AlterID:   alterID,
		Transport: transport,
	}

	if err := validateCredentials(domain.ProtocolVMess, creds); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	norm := newNormalizedNode(domain.ProtocolVMess, value(payload, "ps", "name"), server, port, transport, creds)
	return norm, norm.Credentials, nil
}

func validateCredentials(proto domain.Protocol, creds domain.InboundProtocolCredential) error {
	switch proto {
	case domain.ProtocolSS:
		if strings.TrimSpace(creds.Password) == "" {
			return fmt.Errorf("missing Shadowsocks password")
		}
	case domain.ProtocolVMess:
		if strings.TrimSpace(creds.UUID) == "" {
			return fmt.Errorf("missing VMess UUID")
		}
	case domain.ProtocolVLESS:
		if strings.TrimSpace(creds.UUID) == "" {
			return fmt.Errorf("missing VLESS UUID")
		}
	case domain.ProtocolTrojan:
		if strings.TrimSpace(creds.Password) == "" {
			return fmt.Errorf("missing Trojan password")
		}
	case domain.ProtocolHysteria2:
		if strings.TrimSpace(creds.Password) == "" {
			return fmt.Errorf("missing Hysteria2 password")
		}
	case domain.ProtocolWireGuard:
		if strings.TrimSpace(creds.PrivateKey) == "" {
			return fmt.Errorf("missing WireGuard private key")
		}
		if strings.TrimSpace(creds.PublicKey) == "" {
			return fmt.Errorf("missing WireGuard public key")
		}
		if len(creds.LocalAddress) == 0 {
			return fmt.Errorf("missing WireGuard local address")
		}
		for _, addr := range creds.LocalAddress {
			if _, err := netip.ParsePrefix(strings.TrimSpace(addr)); err != nil {
				return fmt.Errorf("invalid WireGuard local address CIDR %q: %w", addr, err)
			}
		}
	case domain.ProtocolTUIC:
		if strings.TrimSpace(creds.UUID) == "" {
			return fmt.Errorf("missing TUIC UUID")
		}
		if strings.TrimSpace(creds.Password) == "" {
			return fmt.Errorf("missing TUIC password")
		}
	}
	return nil
}
