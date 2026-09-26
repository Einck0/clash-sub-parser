// Package parser normalizes supported subscription formats and extracts node configurations.
package parser

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
)

// NormalizedNode is the normalized node representation used by inventory reconciliation.
type NormalizedNode struct {
	Node        domain.Node
	Server      string
	Port        int
	Transport   map[string]string
	Credentials domain.InboundProtocolCredential
}

// Result reports parsed nodes and malformed or unsupported input entries that were skipped.
type Result struct {
	Nodes    []NormalizedNode
	Rejected int
}

// Parse accepts a Clash or Mihomo YAML proxy list, a Base64 subscription, or URL lines.
// It delegates to ExtractWithCredentials and returns normalized nodes with full credentials.
func Parse(content []byte) (Result, error) {
	extracted, err := ExtractWithCredentials(content)
	if err != nil {
		return Result{}, err
	}
	nodes := make([]NormalizedNode, 0, len(extracted.Items))
	for _, item := range extracted.Items {
		nodes = append(nodes, item.Normalized)
	}
	return Result{
		Nodes:    nodes,
		Rejected: extracted.Rejected,
	}, nil
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
	if alpnList := parseStringList(proxy["alpn"]); len(alpnList) > 0 {
		copyIfPresent(transport, "alpn", strings.Join(alpnList, ","))
	}

	if protocol == domain.ProtocolVLESS {
		copyIfPresent(transport, "flow", value(proxy, "flow"))
		copyIfPresent(transport, "fp", value(proxy, "client-fingerprint", "client_fingerprint", "fingerprint", "fp"))
		if reality, ok := proxy["reality-opts"].(map[string]any); ok {
			copyIfPresent(transport, "pbk", value(reality, "public-key", "public_key", "pbk"))
			copyIfPresent(transport, "sid", value(reality, "short-id", "short_id", "sid"))
		} else if reality, ok := proxy["reality_opts"].(map[string]any); ok {
			copyIfPresent(transport, "pbk", value(reality, "public-key", "public_key", "pbk"))
			copyIfPresent(transport, "sid", value(reality, "short-id", "short_id", "sid"))
		}
		copyIfPresent(transport, "pbk", value(proxy, "pbk", "reality-public-key", "reality_public_key"))
		copyIfPresent(transport, "sid", value(proxy, "sid", "short-id", "short_id", "reality-short-id", "reality_short_id"))
		if transport["pbk"] != "" {
			transport["tls"] = "true"
		}
	}

	if protocol == domain.ProtocolHysteria2 {
		copyIfPresent(transport, "up", value(proxy, "up"))
		copyIfPresent(transport, "down", value(proxy, "down"))
		copyIfPresent(transport, "obfs", value(proxy, "obfs"))
		copyIfPresent(transport, "obfs-password", value(proxy, "obfs-password", "obfs_password"))
		copyIfPresent(transport, "server_ports", value(proxy, "ports", "server_ports", "hy2_ports", "mport"))
	}

	if protocol == domain.ProtocolTUIC {
		if domain.IsTruthy(value(proxy, "disable-sni", "disable_sni")) {
			transport["disable_sni"] = "true"
		}
	}

	if domain.IsTruthy(value(proxy, "skip-cert-verify", "skip_cert_verify", "insecure")) {
		transport["skip_cert_verify"] = "true"
	}
	return transport
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
	copyIfPresent(transport, "sni", firstQuery(query, "sni", "servername", "serverName", "peer"))
	copyIfPresent(transport, "host", query.Get("host"))
	copyIfPresent(transport, "path", query.Get("path"))
	copyIfPresent(transport, "service_name", defaultValue(query.Get("serviceName"), query.Get("service_name")))
	if alpnList := parseStringList(query["alpn"]); len(alpnList) > 0 {
		copyIfPresent(transport, "alpn", strings.Join(alpnList, ","))
	}

	if protocol == domain.ProtocolVLESS {
		copyIfPresent(transport, "flow", firstQuery(query, "flow"))
		copyIfPresent(transport, "fp", firstQuery(query, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"))
		copyIfPresent(transport, "pbk", firstQuery(query, "pbk", "public-key", "public_key", "reality-public-key", "reality_public_key"))
		copyIfPresent(transport, "sid", firstQuery(query, "sid", "short-id", "short_id", "reality-short-id", "reality_short_id"))
		if transport["pbk"] != "" {
			transport["tls"] = "true"
		}
	}

	if protocol == domain.ProtocolHysteria2 {
		copyIfPresent(transport, "up", firstQuery(query, "up", "upmbps", "up_mbps"))
		copyIfPresent(transport, "down", firstQuery(query, "down", "downmbps", "down_mbps"))
		copyIfPresent(transport, "obfs", firstQuery(query, "obfs"))
		copyIfPresent(transport, "obfs-password", firstQuery(query, "obfs-password", "obfs_password"))
		copyIfPresent(transport, "server_ports", firstQuery(query, "ports", "server_ports", "hy2_ports", "mport"))
	}

	if protocol == domain.ProtocolTUIC {
		if domain.IsTruthy(firstQuery(query, "disable_sni", "disable-sni")) {
			transport["disable_sni"] = "true"
		}
	}

	if domain.IsTruthy(firstQuery(query, "insecure", "allowInsecure", "skip-cert-verify", "skip_cert_verify")) {
		transport["skip_cert_verify"] = "true"
	}
	return transport
}

func newNormalizedNode(protocol domain.Protocol, name, server string, port int, transport map[string]string, creds domain.InboundProtocolCredential) NormalizedNode {
	server = strings.ToLower(strings.TrimSpace(server))
	normTransport := make(map[string]string, len(transport))
	for key, value := range transport {
		trimmed := strings.TrimSpace(value)
		transport[key] = trimmed
		if key == "obfs-password" || key == "obfs_password" {
			continue
		}
		normTransport[key] = trimmed
	}
	logicalID := domain.ComputeNodeLogicalID(protocol, server, port, normTransport)
	if name == "" {
		name = net.JoinHostPort(server, strconv.Itoa(port))
	}
	creds.Transport = transport
	return NormalizedNode{
		Node: domain.Node{
			LogicalID:   logicalID,
			Protocol:    protocol,
			DisplayName: name,
			Server:      server,
			Port:        port,
			Credentials: creds,
			Active:      true,
		},
		Server:      server,
		Port:        port,
		Transport:   normTransport,
		Credentials: creds,
	}
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
	raw, ok := decodeBase64Bytes(value)
	if !ok {
		return "", false
	}
	return string(raw), true
}

func decodeBase64Bytes(value string) ([]byte, bool) {
	value = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, value)
	if value == "" {
		return nil, false
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, true
		}
	}
	return nil, false
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

func firstQuery(query url.Values, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(query.Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func parseStringList(raw any) []string {
	var out []string
	seen := make(map[string]bool)
	addTokens := func(s string) {
		for _, part := range strings.Split(s, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" && !seen[trimmed] {
				seen[trimmed] = true
				out = append(out, trimmed)
			}
		}
	}
	switch typed := raw.(type) {
	case string:
		addTokens(typed)
	case []string:
		for _, item := range typed {
			addTokens(item)
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				addTokens(s)
			} else if item != nil {
				addTokens(fmt.Sprint(item))
			}
		}
	}
	return out
}

func parseWireGuardAddresses(rawValues ...any) ([]string, error) {
	var rawTokens []string
	for _, rv := range rawValues {
		for _, token := range parseStringList(rv) {
			rawTokens = append(rawTokens, token)
		}
	}
	if len(rawTokens) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(rawTokens))
	seen := make(map[string]bool, len(rawTokens))
	for _, token := range rawTokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		var canonical string
		if strings.Contains(token, "/") {
			prefix, err := netip.ParsePrefix(token)
			if err != nil {
				return nil, fmt.Errorf("invalid WireGuard local_address %q: %w", token, err)
			}
			canonical = prefix.String()
		} else {
			addr, err := netip.ParseAddr(token)
			if err != nil {
				return nil, fmt.Errorf("invalid WireGuard local_address %q: %w", token, err)
			}
			bits := 32
			if addr.Is6() {
				bits = 128
			}
			canonical = netip.PrefixFrom(addr, bits).String()
		}
		if !seen[canonical] {
			seen[canonical] = true
			out = append(out, canonical)
		}
	}
	return out, nil
}

func parseWireGuardReserved(raw any) ([]uint8, error) {
	if raw == nil {
		return nil, nil
	}
	switch typed := raw.(type) {
	case []uint8:
		if len(typed) == 0 {
			return nil, nil
		}
		if len(typed) != 3 {
			return nil, fmt.Errorf("WireGuard reserved must contain 3 bytes, got %d", len(typed))
		}
		return append([]uint8(nil), typed...), nil
	case []int:
		if len(typed) == 0 {
			return nil, nil
		}
		if len(typed) != 3 {
			return nil, fmt.Errorf("WireGuard reserved must contain 3 bytes, got %d", len(typed))
		}
		out := make([]uint8, 3)
		for i, v := range typed {
			if v < 0 || v > 255 {
				return nil, fmt.Errorf("WireGuard reserved byte out of range: %d", v)
			}
			out[i] = uint8(v)
		}
		return out, nil
	case []any:
		if len(typed) == 0 {
			return nil, nil
		}
		if len(typed) != 3 {
			return nil, fmt.Errorf("WireGuard reserved must contain 3 bytes, got %d", len(typed))
		}
		out := make([]uint8, 3)
		for i, elem := range typed {
			val, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(elem)))
			if err != nil || val < 0 || val > 255 {
				return nil, fmt.Errorf("invalid WireGuard reserved element %v", elem)
			}
			out[i] = uint8(val)
		}
		return out, nil
	case string:
		s := strings.TrimSpace(typed)
		if s == "" {
			return nil, nil
		}
		trimmed := strings.Trim(s, "[]")
		if strings.Contains(trimmed, ",") {
			parts := strings.Split(trimmed, ",")
			if len(parts) != 3 {
				return nil, fmt.Errorf("WireGuard reserved must contain 3 bytes, got %d", len(parts))
			}
			out := make([]uint8, 3)
			for i, p := range parts {
				val, err := strconv.Atoi(strings.TrimSpace(p))
				if err != nil || val < 0 || val > 255 {
					return nil, fmt.Errorf("invalid WireGuard reserved byte %q", p)
				}
				out[i] = uint8(val)
			}
			return out, nil
		}
		if decoded, ok := decodeBase64Bytes(s); ok && len(decoded) == 3 {
			return []uint8{decoded[0], decoded[1], decoded[2]}, nil
		}
		return nil, fmt.Errorf("invalid WireGuard reserved value %q", s)
	default:
		return nil, fmt.Errorf("unsupported WireGuard reserved type %T", raw)
	}
}

func tlsEnabled(proxy map[string]any) bool {
	v := value(proxy, "tls", "security")
	return domain.IsTruthy(v) || strings.EqualFold(v, "tls") || strings.EqualFold(v, "reality")
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
