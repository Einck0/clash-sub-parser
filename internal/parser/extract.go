package parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

// ParsedNodeWithCredentials holds a normalized node alongside its extracted protocol-specific credentials.
// This structure is held strictly in-memory during fetch ingestion and is NEVER persisted directly or logged.
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
	secrets := yamlSecrets(proxy, protocol)
	norm := newNormalizedNode(protocol, value(proxy, "name"), server, port, transport, secrets)

	creds := domain.InboundProtocolCredential{
		Password:     value(proxy, "password"),
		UUID:         value(proxy, "uuid"),
		Method:       value(proxy, "cipher"),
		PrivateKey:   value(proxy, "private-key", "private_key"),
		PublicKey:    value(proxy, "public-key", "public_key"),
		PresharedKey: value(proxy, "psk"),
		Username:     value(proxy, "username", "user"),
		Transport:    transport,
	}

	// Protocol specific validation
	if err := validateCredentials(protocol, creds); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	return norm, creds, nil
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
	secrets := urlSecrets(u, protocol)
	norm := newNormalizedNode(protocol, fragmentName(u), server, port, transport, secrets)

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
		creds.UUID = u.User.Username()
	case domain.ProtocolTrojan:
		if creds.Password == "" && u.User != nil {
			creds.Password = u.User.Username()
		}
	case domain.ProtocolHysteria2:
		if creds.Password == "" && u.User != nil {
			creds.Password = u.User.Username()
		}
	case domain.ProtocolWireGuard:
		if u.User != nil {
			creds.PrivateKey = u.User.Username()
		}
		creds.PublicKey = query.Get("public_key")
		creds.PresharedKey = query.Get("preshared_key")
	case domain.ProtocolTUIC:
		if u.User != nil {
			creds.UUID = u.User.Username()
			if pw, ok := u.User.Password(); ok {
				creds.Password = pw
			}
		}
	}

	if err := validateCredentials(protocol, creds); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	return norm, creds, nil
}

func extractVMess(raw string) (NormalizedNode, domain.InboundProtocolCredential, error) {
	norm, err := parseVMess(raw)
	if err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	encoded := strings.TrimPrefix(raw, "vmess://")
	decoded, ok := decodeBase64(encoded)
	if !ok {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, fmt.Errorf("invalid VMess payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(decoded), &payload); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, fmt.Errorf("invalid VMess payload")
	}

	creds := domain.InboundProtocolCredential{
		UUID:      value(payload, "id", "uuid"),
		Method:    value(payload, "scy", "security"),
		Transport: norm.Transport,
	}

	if err := validateCredentials(domain.ProtocolVMess, creds); err != nil {
		return NormalizedNode{}, domain.InboundProtocolCredential{}, err
	}

	return norm, creds, nil
}

func validateCredentials(proto domain.Protocol, creds domain.InboundProtocolCredential) error {
	switch proto {
	case domain.ProtocolSS:
		if creds.Password == "" {
			return fmt.Errorf("missing Shadowsocks password")
		}
	case domain.ProtocolVMess:
		if creds.UUID == "" {
			return fmt.Errorf("missing VMess UUID")
		}
	case domain.ProtocolVLESS:
		if creds.UUID == "" {
			return fmt.Errorf("missing VLESS UUID")
		}
	case domain.ProtocolTrojan:
		if creds.Password == "" {
			return fmt.Errorf("missing Trojan password")
		}
	case domain.ProtocolHysteria2:
		if creds.Password == "" {
			return fmt.Errorf("missing Hysteria2 password")
		}
	case domain.ProtocolWireGuard:
		if creds.PrivateKey == "" {
			return fmt.Errorf("missing WireGuard private key")
		}
	case domain.ProtocolTUIC:
		if creds.UUID == "" {
			return fmt.Errorf("missing TUIC UUID")
		}
	}
	return nil
}
