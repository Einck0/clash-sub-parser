package parser

import (
	"encoding/base64"
	"encoding/json"
	"net"
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
	Items        []ParsedNodeWithCredentials
	Rejected     int
	Diagnostics  []Diagnostic
	ReasonCounts []ReasonCount
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
		if result, isYAML, err := extractYAML([]byte(decoded)); isYAML {
			result.wrapped()
			return result, err
		}
		result, err := extractURLLines(decoded)
		result.wrapped()
		return result, err
	}
	return extractURLLines(trimmed)
}

func extractYAML(content []byte) (ExtractResult, bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		// Syntax errors cannot safely be recovered by splitting YAML text.
		if strings.HasPrefix(strings.TrimSpace(string(content)), "proxies:") {
			result := ExtractResult{}
			result.reject("yaml", 1, "", parseIssue("invalid_document", "document"))
			return result, true, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
		}
		return ExtractResult{}, false, nil
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return ExtractResult{}, false, nil
	}
	root := document.Content[0]
	// Decode only the root field, letting yaml.v3 enforce duplicate-key and
	// merge semantics without expanding all proxy entries together. Keeping a
	// Node here preserves individual-entry isolation below.
	var fields struct {
		Proxies yaml.Node `yaml:"proxies"`
	}
	if err := root.Decode(&fields); err != nil {
		result := ExtractResult{}
		result.reject("yaml", 1, "", parseIssue("invalid_document", "proxies"))
		return result, true, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
	}
	proxies := &fields.Proxies
	if proxies.Kind == 0 {
		return ExtractResult{}, false, nil
	}
	// A field decoded into yaml.Node retains its alias. Follow only this
	// top-level reference, never recursively expand an untrusted graph.
	seen := make(map[*yaml.Node]bool)
	for proxies != nil && proxies.Kind == yaml.AliasNode {
		if seen[proxies] {
			proxies = nil
			break
		}
		seen[proxies] = true
		proxies = proxies.Alias
	}
	result := ExtractResult{}
	if proxies == nil || proxies.Kind != yaml.SequenceNode {
		result.reject("yaml", 1, "", parseIssue("invalid_entry", "proxies"))
	} else {
		for i, entry := range proxies.Content {
			var proxy map[string]any
			if err := entry.Decode(&proxy); err != nil || proxy == nil {
				result.reject("yaml", i+1, "", parseIssue("invalid_entry", "entry"))
				continue
			}
			node, creds, err := extractYAMLProxy(proxy)
			if err != nil {
				result.reject("yaml", i+1, knownProtocol(value(proxy, "type")), err)
				continue
			}
			result.Items = append(result.Items, ParsedNodeWithCredentials{Normalized: node, Credentials: creds})
		}
	}
	if len(result.Items) == 0 {
		return result, true, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
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
	if protocol == domain.ProtocolSS {
		for _, key := range []string{"plugin-opts", "plugin_opts"} {
			if raw, present := proxy[key]; present && raw != nil {
				if _, ok := raw.(string); !ok {
					if _, ok := raw.(map[string]any); !ok {
						return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "plugin_opts")
					}
				}
				if opts, ok := raw.(map[string]any); ok && len(opts) > 0 && transport[key] == "" {
					return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "plugin_opts")
				}
			}
		}
	}

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

	if protocol == domain.ProtocolVMess {
		creds.Method = value(proxy, "cipher", "scy")
	}

	if protocol == domain.ProtocolHTTP {
		if headers, ok := proxy["headers"].(map[string]any); ok {
			if hBytes, err := json.Marshal(headers); err == nil {
				transport["headers"] = string(hBytes)
			}
		}
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
			return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "local_address")
		}
		creds.LocalAddress = localAddrs

		reserved, resErr := parseWireGuardReserved(rawReserved)
		if resErr != nil {
			return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "reserved")
		}
		creds.Reserved = reserved

		if mtuStr := value(proxy, "mtu"); mtuStr != "" {
			mtuVal, mtuErr := strconv.Atoi(mtuStr)
			if mtuErr != nil || mtuVal <= 0 || mtuVal > 65535 {
				return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "mtu")
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
	for i, line := range strings.Fields(content) {
		node, creds, err := extractURL(line)
		if err != nil {
			scheme, _, _ := strings.Cut(line, ":")
			result.reject("url", i+1, knownProtocol(scheme), err)
			continue
		}
		result.Items = append(result.Items, ParsedNodeWithCredentials{
			Normalized:  node,
			Credentials: creds,
		})
	}
	if len(result.Items) == 0 {
		return result, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
	}
	return result, nil
}

func extractURL(raw string) (NormalizedNode, domain.InboundProtocolCredential, error) {
	// Legacy SS encodes the whole authority, unlike SIP002 userinfo.
	if strings.HasPrefix(strings.ToLower(raw), "ss://") {
		body, fragment, hasFragment := strings.Cut(raw[len("ss://"):], "#")
		authority, query, hasQuery := strings.Cut(body, "?")
		if !strings.Contains(authority, "@") {
			if decoded, ok := decodeBase64(authority); ok {
				at := strings.LastIndex(decoded, "@")
				if at < 0 {
					return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_payload", "authority")
				}
				user, address := decoded[:at], decoded[at+1:]
				if _, _, err := net.SplitHostPort(address); err != nil {
					return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_endpoint", "endpoint")
				}
				raw = "ss://" + base64.RawURLEncoding.EncodeToString([]byte(user)) + "@" + address
				if hasQuery {
					raw += "?" + query
				}
				if hasFragment {
					raw += "#" + fragment
				}
			}
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_url", "url")
	}
	protocol, err := protocolFor(u.Scheme)
	if err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}
	if protocol == domain.ProtocolVMess {
		prefix := u.Scheme + "://"
		if !strings.HasPrefix(raw, prefix) {
			return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_payload", "payload")
		}
		return extractVMess(raw[len(prefix):])
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
		creds.Username = ""
		if plugin := query.Get("plugin"); plugin != "" {
			name, opts, _ := strings.Cut(plugin, ";")
			transport["plugin"] = name
			copyIfPresent(transport, "plugin-opts", opts)
		}
		if u.User != nil {
			if _, plain := u.User.Password(); plain {
				creds.Method = u.User.Username()
			} else if decoded, ok := decodeBase64(u.User.Username()); ok {
				parts := strings.SplitN(decoded, ":", 2)
				if len(parts) == 2 {
					creds.Method = parts[0]
					creds.Password = parts[1]
				}
			}
		}
	case domain.ProtocolVLESS:
		creds.Username = ""
		if u.User != nil {
			creds.UUID = u.User.Username()
		}
		if creds.UUID == "" {
			creds.UUID = firstQuery(query, "uuid")
		}
	case domain.ProtocolTrojan:
		creds.Username = ""
		if u.User != nil {
			creds.Password = u.User.Username()
		}
	case domain.ProtocolHysteria2:
		creds.Username = ""
		if u.User != nil {
			creds.Password = u.User.Username()
		}
		if creds.Password == "" {
			creds.Password = firstQuery(query, "password", "auth")
		}
	case domain.ProtocolWireGuard:
		creds.Username = ""
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
			return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "local_address")
		}
		creds.LocalAddress = localAddrs

		if rawRes := firstQuery(query, "reserved"); rawRes != "" {
			reserved, resErr := parseWireGuardReserved(rawRes)
			if resErr != nil {
				return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "reserved")
			}
			creds.Reserved = reserved
		}
		if mtuStr := firstQuery(query, "mtu"); mtuStr != "" {
			mtuVal, mtuErr := strconv.Atoi(mtuStr)
			if mtuErr != nil || mtuVal <= 0 || mtuVal > 65535 {
				return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_field", "mtu")
			}
			creds.MTU = mtuVal
		}
		creds.DNS = parseStringList(query["dns"])
	case domain.ProtocolTUIC:
		creds.Username = ""
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
	case domain.ProtocolHTTP, domain.ProtocolSocks5:
		if creds.Username == "" {
			creds.Username = firstQuery(query, "user", "username")
		}
		if creds.Password == "" {
			creds.Password = firstQuery(query, "password", "pass")
		}
	case domain.ProtocolAnyTLS:
		creds.Username = ""
		if u.User != nil {
			creds.Password = u.User.Username()
			if pw, ok := u.User.Password(); ok {
				creds.Password = pw
			}
		}
		if creds.Password == "" {
			creds.Password = firstQuery(query, "password", "auth")
		}
	}

	if err := validateCredentials(protocol, creds); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	norm := newNormalizedNode(protocol, fragmentName(u), server, port, transport, creds)
	return norm, norm.Credentials, nil
}

func extractVMess(raw string) (NormalizedNode, domain.InboundProtocolCredential, error) {
	decoded, ok := decodeBase64(raw)
	if !ok {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_payload", "payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(decoded), &payload); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, parseIssue("invalid_payload", "payload")
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

	method := value(payload, "scy", "cipher")
	if method == "" {
		security := value(payload, "security")
		if !strings.EqualFold(security, "tls") && !strings.EqualFold(security, "reality") {
			method = security
		}
	}
	method = defaultValue(method, "auto")

	creds := domain.InboundProtocolCredential{
		UUID:      uuid,
		Method:    method,
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
		if strings.TrimSpace(creds.Method) == "" {
			return parseIssue("missing_credential", "cipher")
		}
		if strings.TrimSpace(creds.Password) == "" {
			return parseIssue("missing_credential", "password")
		}
	case domain.ProtocolVMess, domain.ProtocolVLESS:
		if strings.TrimSpace(creds.UUID) == "" {
			return parseIssue("missing_credential", "uuid")
		}
	case domain.ProtocolTrojan, domain.ProtocolHysteria2, domain.ProtocolAnyTLS:
		if strings.TrimSpace(creds.Password) == "" {
			return parseIssue("missing_credential", "password")
		}
	case domain.ProtocolWireGuard:
		if strings.TrimSpace(creds.PrivateKey) == "" {
			return parseIssue("missing_credential", "private_key")
		}
		if strings.TrimSpace(creds.PublicKey) == "" {
			return parseIssue("missing_credential", "public_key")
		}
		if len(creds.LocalAddress) == 0 {
			return parseIssue("missing_credential", "local_address")
		}
		for _, addr := range creds.LocalAddress {
			if _, err := netip.ParsePrefix(strings.TrimSpace(addr)); err != nil {
				return parseIssue("invalid_field", "local_address")
			}
		}
	case domain.ProtocolTUIC:
		if strings.TrimSpace(creds.UUID) == "" {
			return parseIssue("missing_credential", "uuid")
		}
		if strings.TrimSpace(creds.Password) == "" {
			return parseIssue("missing_credential", "password")
		}
	case domain.ProtocolHTTP, domain.ProtocolSocks5:
		// HTTP and SOCKS5 may be unauthenticated.
	}
	return nil
}
