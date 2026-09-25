// Package parser normalizes supported subscription formats without retaining credentials.
package parser

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
	"gopkg.in/yaml.v3"
)

// NormalizedNode is the non-secret normalized representation used by inventory reconciliation.
type NormalizedNode struct {
	Node      domain.Node
	Server    string
	Port      int
	Transport map[string]string
}

// Result reports parsed nodes and malformed or unsupported input entries that were skipped.
type Result struct {
	Nodes    []NormalizedNode
	Rejected int
}

// Parse accepts a Clash or Mihomo YAML proxy list, a Base64 subscription, or URL lines.
func Parse(content []byte) (Result, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return Result{}, domain.NewValidationError("empty_subscription", "subscription content is empty")
	}

	if nodes, ok, err := parseYAML([]byte(trimmed)); ok {
		return nodes, err
	}
	if decoded, ok := decodeBase64(trimmed); ok {
		return parseURLLines(decoded)
	}
	return parseURLLines(trimmed)
}

func parseYAML(content []byte) (Result, bool, error) {
	var document struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(content, &document); err != nil || document.Proxies == nil {
		return Result{}, false, nil
	}

	result := Result{Nodes: make([]NormalizedNode, 0, len(document.Proxies))}
	for _, proxy := range document.Proxies {
		node, err := parseYAMLProxy(proxy)
		if err != nil {
			result.Rejected++
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	if len(result.Nodes) == 0 {
		return Result{}, true, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
	}
	return result, true, nil
}

func parseYAMLProxy(proxy map[string]any) (NormalizedNode, error) {
	protocol, err := protocolFor(value(proxy, "type"))
	if err != nil {
		return NormalizedNode{}, err
	}
	server, port, err := endpoint(value(proxy, "server"), value(proxy, "port"))
	if err != nil {
		return NormalizedNode{}, err
	}
	transport := yamlTransport(proxy, protocol)
	secrets := yamlSecrets(proxy, protocol)
	return newNormalizedNode(protocol, value(proxy, "name"), server, port, transport, secrets), nil
}

func yamlTransport(proxy map[string]any, protocol domain.Protocol) map[string]string {
	transport := map[string]string{"network": "tcp"}
	if network := value(proxy, "network"); network != "" {
		transport["network"] = strings.ToLower(network)
	}
	if protocol == domain.ProtocolWireGuard {
		transport["network"] = "wireguard"
	}
	if protocol == domain.ProtocolHysteria2 || protocol == domain.ProtocolTUIC {
		transport["network"] = "quic"
	}
	if tlsEnabled(proxy) || protocolUsesTLS(protocol) {
		transport["tls"] = "true"
	}
	copyIfPresent(transport, "sni", value(proxy, "sni", "servername", "serverName"))
	if ws, ok := proxy["ws-opts"].(map[string]any); ok {
		copyIfPresent(transport, "path", value(ws, "path"))
		if headers, ok := ws["headers"].(map[string]any); ok {
			copyIfPresent(transport, "host", value(headers, "Host", "host"))
		}
	}
	if grpc, ok := proxy["grpc-opts"].(map[string]any); ok {
		copyIfPresent(transport, "service_name", value(grpc, "grpc-service-name", "service-name"))
	}
	copyIfPresent(transport, "path", value(proxy, "path"))
	copyIfPresent(transport, "host", value(proxy, "host"))
	return transport
}

func yamlSecrets(proxy map[string]any, protocol domain.Protocol) []string {
	keys := []string{"password", "uuid", "private-key", "private_key", "psk"}
	if protocol == domain.ProtocolSS {
		keys = append(keys, "cipher")
	}
	secrets := make([]string, 0, len(keys))
	for _, key := range keys {
		if secret := value(proxy, key); secret != "" {
			secrets = append(secrets, secret)
		}
	}
	return secrets
}

func parseURLLines(content string) (Result, error) {
	result := Result{}
	for _, line := range strings.Fields(content) {
		node, err := parseURL(line)
		if err != nil {
			result.Rejected++
			continue
		}
		result.Nodes = append(result.Nodes, node)
	}
	if len(result.Nodes) == 0 {
		return Result{}, domain.NewValidationError("no_supported_nodes", "subscription contains no supported nodes")
	}
	return result, nil
}

func parseURL(raw string) (NormalizedNode, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return NormalizedNode{}, fmt.Errorf("invalid subscription URL")
	}
	protocol, err := protocolFor(u.Scheme)
	if err != nil {
		return NormalizedNode{}, err
	}
	if protocol == domain.ProtocolVMess {
		return parseVMess(raw)
	}
	server, port, err := endpoint(u.Hostname(), u.Port())
	if err != nil {
		return NormalizedNode{}, err
	}
	transport := urlTransport(u, protocol)
	return newNormalizedNode(protocol, fragmentName(u), server, port, transport, urlSecrets(u, protocol)), nil
}

func parseVMess(raw string) (NormalizedNode, error) {
	encoded := strings.TrimPrefix(raw, "vmess://")
	decoded, ok := decodeBase64(encoded)
	if !ok {
		return NormalizedNode{}, fmt.Errorf("invalid VMess payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(decoded), &payload); err != nil {
		return NormalizedNode{}, fmt.Errorf("invalid VMess payload")
	}
	server, port, err := endpoint(value(payload, "add", "server"), value(payload, "port"))
	if err != nil {
		return NormalizedNode{}, err
	}
	transport := map[string]string{"network": strings.ToLower(defaultValue(value(payload, "net", "network", "anet"), "tcp"))}
	copyIfPresent(transport, "tls", value(payload, "tls", "security"))
	if tlsValue := strings.ToLower(transport["tls"]); tlsValue == "tls" || tlsValue == "reality" || tlsValue == "true" {
		transport["tls"] = "true"
	} else {
		delete(transport, "tls")
	}
	copyIfPresent(transport, "sni", value(payload, "sni"))
	copyIfPresent(transport, "host", value(payload, "host"))
	copyIfPresent(transport, "path", value(payload, "path"))
	return newNormalizedNode(domain.ProtocolVMess, value(payload, "ps", "name"), server, port, transport, []string{value(payload, "id", "uuid")}), nil
}

func urlTransport(u *url.URL, protocol domain.Protocol) map[string]string {
	query := u.Query()
	transport := map[string]string{"network": strings.ToLower(defaultValue(query.Get("type"), "tcp"))}
	if protocol == domain.ProtocolWireGuard {
		transport["network"] = "wireguard"
	}
	if protocol == domain.ProtocolHysteria2 || protocol == domain.ProtocolTUIC {
		transport["network"] = "quic"
	}
	security := strings.ToLower(query.Get("security"))
	if security == "tls" || security == "reality" || protocolUsesTLS(protocol) {
		transport["tls"] = "true"
	}
	copyIfPresent(transport, "sni", query.Get("sni"))
	copyIfPresent(transport, "host", query.Get("host"))
	copyIfPresent(transport, "path", query.Get("path"))
	copyIfPresent(transport, "service_name", defaultValue(query.Get("serviceName"), query.Get("service_name")))
	copyIfPresent(transport, "alpn", query.Get("alpn"))
	return transport
}

func urlSecrets(u *url.URL, protocol domain.Protocol) []string {
	secrets := []string{}
	if u.User != nil {
		secrets = append(secrets, u.User.Username())
		if password, ok := u.User.Password(); ok {
			secrets = append(secrets, password)
		}
	}
	query := u.Query()
	for _, key := range []string{"password", "uuid", "private_key", "private-key", "psk"} {
		if value := query.Get(key); value != "" {
			secrets = append(secrets, value)
		}
	}
	if protocol == domain.ProtocolSS && u.User != nil {
		if decoded, ok := decodeBase64(u.User.Username()); ok {
			secrets = append(secrets, decoded)
		}
	}
	return secrets
}

func newNormalizedNode(protocol domain.Protocol, name, server string, port int, transport map[string]string, secrets []string) NormalizedNode {
	server = strings.ToLower(strings.TrimSpace(server))
	for key, value := range transport {
		transport[key] = strings.TrimSpace(value)
	}
	logicalID := domain.ComputeNodeLogicalID(protocol, server, port, transport)
	secretRef := opaqueSecretRef(protocol, server, port, secrets)
	if name == "" {
		name = net.JoinHostPort(server, strconv.Itoa(port))
	}
	return NormalizedNode{
		Node: domain.Node{
			LogicalID:                 logicalID,
			Protocol:                  protocol,
			DisplayName:               name,
			NormalizedConfigSecretRef: secretRef,
			Active:                    true,
		},
		Server: server, Port: port, Transport: transport,
	}
}

func opaqueSecretRef(protocol domain.Protocol, server string, port int, secrets []string) string {
	hash := sha256.Sum256([]byte(strings.Join(append([]string{string(protocol), server, strconv.Itoa(port)}, secrets...), "\x00")))
	return "secret_" + hex.EncodeToString(hash[:16])
}

func endpoint(server, rawPort string) (string, int, error) {
	server = strings.Trim(strings.TrimSpace(server), "[]")
	port, err := strconv.Atoi(strings.TrimSpace(rawPort))
	if server == "" || err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid node endpoint")
	}
	return server, port, nil
}

func protocolFor(raw string) (domain.Protocol, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ss", "shadowsocks":
		return domain.ProtocolSS, nil
	case "vmess":
		return domain.ProtocolVMess, nil
	case "vless":
		return domain.ProtocolVLESS, nil
	case "trojan":
		return domain.ProtocolTrojan, nil
	case "hysteria2", "hy2":
		return domain.ProtocolHysteria2, nil
	case "wireguard", "wg":
		return domain.ProtocolWireGuard, nil
	case "tuic":
		return domain.ProtocolTUIC, nil
	default:
		return "", fmt.Errorf("unsupported node protocol")
	}
}

func decodeBase64(value string) (string, bool) {
	value = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, value)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return string(decoded), true
		}
	}
	return "", false
}

func value(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			switch typed := raw.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					return trimmed
				}
			case int:
				return strconv.Itoa(typed)
			case int64:
				return strconv.FormatInt(typed, 10)
			case float64:
				return strconv.Itoa(int(typed))
			case bool:
				return strconv.FormatBool(typed)
			}
		}
	}
	return ""
}

func tlsEnabled(proxy map[string]any) bool {
	return strings.EqualFold(value(proxy, "tls", "security"), "true") || strings.EqualFold(value(proxy, "tls", "security"), "tls") || strings.EqualFold(value(proxy, "tls", "security"), "reality")
}

func protocolUsesTLS(protocol domain.Protocol) bool {
	return protocol == domain.ProtocolTrojan || protocol == domain.ProtocolHysteria2 || protocol == domain.ProtocolTUIC
}

func copyIfPresent(target map[string]string, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		target[key] = value
	}
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fragmentName(u *url.URL) string {
	name, err := url.PathUnescape(u.Fragment)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}
