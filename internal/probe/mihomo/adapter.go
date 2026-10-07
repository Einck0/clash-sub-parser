package mihomo

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"clash-sub-parser/internal/domain"
	"github.com/metacubex/mihomo/adapter"
	_ "github.com/metacubex/mihomo/config" // init() sets dns.ParseNameServer, required by ParseProxy
	"github.com/metacubex/mihomo/constant"
)

var (
	coreVersionOnce sync.Once
	coreVersionStr  string
)

// CoreVersion returns the runtime dependency version of Mihomo, e.g. "mihomo/v1.19.32",
// or "" if unknown.
func CoreVersion() string {
	coreVersionOnce.Do(func() {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		for _, dep := range bi.Deps {
			if dep.Path == "github.com/metacubex/mihomo" {
				if dep.Version != "" && dep.Version != "(devel)" {
					coreVersionStr = "mihomo/" + dep.Version
				}
				return
			}
		}
	})
	return coreVersionStr
}

// NodeToMapping converts a domain.Node into a configuration map suitable for Mihomo adapter.ParseProxy.
func NodeToMapping(node domain.Node) (map[string]any, error) {
	if strings.TrimSpace(node.Server) == "" || node.Port < 1 || node.Port > 65535 {
		return nil, domain.ErrCredentialsUnavailable
	}

	name := strings.TrimSpace(node.DisplayName)
	if name == "" {
		name = strings.TrimSpace(node.LogicalID)
	}
	if name == "" {
		name = "node-probe"
	}

	c := node.Credentials
	m := map[string]any{
		"name":   name,
		"server": node.Server,
		"port":   node.Port,
	}

	switch node.Protocol {
	case domain.ProtocolSS:
		m["type"] = "ss"
		m["cipher"] = c.Method
		m["password"] = c.Password
		if plugin := transportValue(c, "plugin"); plugin != "" {
			m["plugin"] = plugin
			if pluginOpts := transportValue(c, "plugin-opts", "plugin_opts"); pluginOpts != "" {
				m["plugin-opts"] = parsePluginOpts(pluginOpts)
			}
		}

	case domain.ProtocolVMess:
		m["type"] = "vmess"
		m["uuid"] = strings.TrimSpace(c.UUID)
		m["alterId"] = c.AlterID
		cipher := strings.TrimSpace(c.Method)
		if cipher == "" {
			cipher = "auto"
		}
		m["cipher"] = cipher
		net := strings.ToLower(transportValue(c, "network"))
		if net == "" {
			net = "tcp"
		}
		m["network"] = net

		tlsVal := transportValue(c, "tls")
		hasTLS := domain.IsTruthy(tlsVal)
		if hasTLS {
			m["tls"] = true
		}
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["servername"] = sni
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}
		if alpn := extractALPN(c); len(alpn) > 0 {
			m["alpn"] = alpn
		}
		if fp := transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"); fp != "" {
			m["client-fingerprint"] = fp
		}
		if net == "ws" {
			wsOpts := map[string]any{}
			if path := transportValue(c, "path"); path != "" {
				wsOpts["path"] = path
			}
			if host := transportValue(c, "host"); host != "" {
				wsOpts["headers"] = map[string]any{"Host": host}
			}
			m["ws-opts"] = wsOpts
		} else if net == "grpc" {
			if svc := transportValue(c, "service_name", "serviceName", "grpc-service-name"); svc != "" {
				m["grpc-opts"] = map[string]any{"grpc-service-name": svc}
			}
		}

	case domain.ProtocolVLESS:
		m["type"] = "vless"
		m["uuid"] = strings.TrimSpace(c.UUID)
		net := strings.ToLower(transportValue(c, "network"))
		if net == "" {
			net = "tcp"
		}
		m["network"] = net

		pbk := transportValue(c, "pbk", "public-key", "public_key", "reality-public-key", "reality_public_key")
		sid := transportValue(c, "sid", "short-id", "short_id", "reality-short-id", "reality_short_id")
		tlsVal := transportValue(c, "tls")
		hasTLS := domain.IsTruthy(tlsVal) || pbk != ""
		if hasTLS {
			m["tls"] = true
		}
		if pbk != "" {
			realityOpts := map[string]any{"public-key": pbk}
			if sid != "" {
				realityOpts["short-id"] = sid
			}
			m["reality-opts"] = realityOpts
		}
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["servername"] = sni
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}
		if alpn := extractALPN(c); len(alpn) > 0 {
			m["alpn"] = alpn
		}
		if flow := transportValue(c, "flow"); flow != "" {
			m["flow"] = flow
		}
		if fp := transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"); fp != "" {
			m["client-fingerprint"] = fp
		}
		if echJSON := transportValue(c, "ech-opts", "ech_opts"); echJSON != "" {
			if echOpts, err := unmarshalJSONMap(echJSON); err == nil && len(echOpts) > 0 {
				m["ech-opts"] = echOpts
			}
		}
		if net == "ws" {
			wsOpts := map[string]any{}
			if path := transportValue(c, "path"); path != "" {
				wsOpts["path"] = path
			}
			if host := transportValue(c, "host"); host != "" {
				wsOpts["headers"] = map[string]any{"Host": host}
			}
			m["ws-opts"] = wsOpts
		} else if net == "grpc" {
			if svc := transportValue(c, "service_name", "serviceName", "grpc-service-name"); svc != "" {
				m["grpc-opts"] = map[string]any{"grpc-service-name": svc}
			}
		} else if net == "xhttp" {
			xhttpOpts := map[string]any{}
			if xhttpJSON := transportValue(c, "xhttp-opts", "xhttp_opts"); xhttpJSON != "" {
				if parsed, err := unmarshalJSONMap(xhttpJSON); err == nil && len(parsed) > 0 {
					xhttpOpts = parsed
				}
			}
			if path := transportValue(c, "path"); path != "" && xhttpOpts["path"] == nil {
				xhttpOpts["path"] = path
			}
			if host := transportValue(c, "host"); host != "" && xhttpOpts["host"] == nil {
				xhttpOpts["host"] = host
			}
			if mode := transportValue(c, "mode"); mode != "" && xhttpOpts["mode"] == nil {
				xhttpOpts["mode"] = mode
			}
			if hJSON := transportValue(c, "headers"); hJSON != "" && xhttpOpts["headers"] == nil {
				var headers map[string]string
				if err := json.Unmarshal([]byte(hJSON), &headers); err == nil && len(headers) > 0 {
					xhttpOpts["headers"] = headers
				}
			}
			m["xhttp-opts"] = xhttpOpts
		}

	case domain.ProtocolTrojan:
		m["type"] = "trojan"
		m["password"] = c.Password
		net := strings.ToLower(transportValue(c, "network"))
		if net != "" {
			m["network"] = net
		}
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["sni"] = sni
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}
		if alpn := extractALPN(c); len(alpn) > 0 {
			m["alpn"] = alpn
		}
		if fp := transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"); fp != "" {
			m["client-fingerprint"] = fp
		}
		if net == "ws" {
			wsOpts := map[string]any{}
			if path := transportValue(c, "path"); path != "" {
				wsOpts["path"] = path
			}
			if host := transportValue(c, "host"); host != "" {
				wsOpts["headers"] = map[string]any{"Host": host}
			}
			m["ws-opts"] = wsOpts
		} else if net == "grpc" {
			if svc := transportValue(c, "service_name", "serviceName", "grpc-service-name"); svc != "" {
				m["grpc-opts"] = map[string]any{"grpc-service-name": svc}
			}
		}

	case domain.ProtocolHysteria2:
		m["type"] = "hysteria2"
		m["password"] = c.Password
		ports := domain.ExtractHy2Ports(c.Transport)
		if ports == "" {
			ports = transportValue(c, "ports", "server_ports", "mport")
		}
		if ports != "" {
			m["ports"] = ports
		}
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["sni"] = sni
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}
		if alpn := extractALPN(c); len(alpn) > 0 {
			m["alpn"] = alpn
		}
		if up := transportValue(c, "up"); up != "" {
			m["up"] = up
		}
		if down := transportValue(c, "down"); down != "" {
			m["down"] = down
		}
		if obfs := transportValue(c, "obfs"); obfs != "" {
			m["obfs"] = obfs
		}
		if obfsPass := transportValue(c, "obfs-password", "obfs_password"); obfsPass != "" {
			m["obfs-password"] = obfsPass
		}

	case domain.ProtocolTUIC:
		m["type"] = "tuic"
		m["uuid"] = strings.TrimSpace(c.UUID)
		m["password"] = c.Password
		if ip := transportValue(c, "ip"); ip != "" {
			m["ip"] = ip
		}
		if cc := transportValue(c, "congestion_control", "congestion-control", "congestion_controller", "congestion-controller"); cc != "" {
			m["congestion-controller"] = strings.ToLower(cc)
		}
		if urm := transportValue(c, "udp_relay_mode", "udp-relay-mode"); urm != "" {
			m["udp-relay-mode"] = strings.ToLower(urm)
		}
		if alpn := extractALPN(c); len(alpn) > 0 {
			m["alpn"] = alpn
		}
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["sni"] = sni
		}
		if c.DisableSNI || domain.HasTUICDisableSNI(c.Transport) {
			m["disable-sni"] = true
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}

	case domain.ProtocolWireGuard:
		m["type"] = "wireguard"
		ipv4, ipv6 := splitWireGuardAddresses(c.LocalAddress)
		if ipv4 != "" {
			m["ip"] = ipv4
		}
		if ipv6 != "" {
			m["ipv6"] = ipv6
		}
		m["private-key"] = strings.TrimSpace(c.PrivateKey)
		m["public-key"] = strings.TrimSpace(c.PublicKey)
		if psk := c.EffectivePreSharedKey(); psk != "" {
			m["preshared-key"] = psk
		}
		if len(c.Reserved) > 0 {
			reserved := make([]int, len(c.Reserved))
			for idx, b := range c.Reserved {
				reserved[idx] = int(b)
			}
			m["reserved"] = reserved
		}
		if c.MTU > 0 {
			m["mtu"] = c.MTU
		}
		var dns []string
		for _, d := range c.DNS {
			if trimmed := strings.TrimSpace(d); trimmed != "" {
				dns = append(dns, trimmed)
			}
		}
		if len(dns) > 0 {
			m["dns"] = dns
			m["remote-dns-resolve"] = true
		}
		m["udp"] = true

	case domain.ProtocolHTTP:
		m["type"] = "http"
		if c.Username != "" {
			m["username"] = c.Username
		}
		if c.Password != "" {
			m["password"] = c.Password
		}
		if domain.IsTruthy(transportValue(c, "tls")) {
			m["tls"] = true
		}
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["sni"] = sni
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}
		if hJSON := transportValue(c, "headers"); hJSON != "" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(hJSON), &headers); err == nil && len(headers) > 0 {
				m["headers"] = headers
			}
		}

	case domain.ProtocolSocks5:
		m["type"] = "socks5"
		if c.Username != "" {
			m["username"] = c.Username
		}
		if c.Password != "" {
			m["password"] = c.Password
		}
		if domain.IsTruthy(transportValue(c, "tls")) {
			m["tls"] = true
		}
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["sni"] = sni
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}
		if udpVal := transportValue(c, "udp"); udpVal != "" {
			m["udp"] = domain.IsTruthy(udpVal)
		}

	case domain.ProtocolAnyTLS:
		m["type"] = "anytls"
		m["password"] = c.Password
		if sni := transportValue(c, "sni", "servername", "serverName", "peer"); sni != "" {
			m["sni"] = sni
		}
		if alpn := extractALPN(c); len(alpn) > 0 {
			m["alpn"] = alpn
		}
		if fp := transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"); fp != "" {
			m["client-fingerprint"] = fp
		}
		if domain.HasInsecureTransport(c.Transport) {
			m["skip-cert-verify"] = true
		}
		if udpVal := transportValue(c, "udp"); udpVal != "" {
			m["udp"] = domain.IsTruthy(udpVal)
		}
		if echJSON := transportValue(c, "ech-opts", "ech_opts"); echJSON != "" {
			if echOpts, err := unmarshalJSONMap(echJSON); err == nil && len(echOpts) > 0 {
				m["ech-opts"] = echOpts
			}
		}
		if idleCheck := transportValue(c, "idle-session-check-interval", "idle_session_check_interval"); idleCheck != "" {
			if v, err := strconv.Atoi(idleCheck); err == nil {
				m["idle-session-check-interval"] = v
			}
		}
		if idleTimeout := transportValue(c, "idle-session-timeout", "idle_session_timeout"); idleTimeout != "" {
			if v, err := strconv.Atoi(idleTimeout); err == nil {
				m["idle-session-timeout"] = v
			}
		}
		if minIdle := transportValue(c, "min-idle-session", "min_idle_session"); minIdle != "" {
			if v, err := strconv.Atoi(minIdle); err == nil {
				m["min-idle-session"] = v
			}
		}
		if disableReuse := transportValue(c, "disable-reuse", "disable_reuse"); disableReuse != "" {
			m["disable-reuse"] = domain.IsTruthy(disableReuse)
		}

	default:
		return nil, fmt.Errorf("unsupported protocol: %s", node.Protocol)
	}

	return m, nil
}

// ParseProxy converts a domain.Node into a Mihomo constant.Proxy instance.
func ParseProxy(node domain.Node) (constant.Proxy, error) {
	mapping, err := NodeToMapping(node)
	if err != nil {
		return nil, err
	}
	proxy, err := adapter.ParseProxy(mapping)
	if err != nil {
		return nil, fmt.Errorf("%w: mihomo ParseProxy failed for %s (%s): %v", domain.ErrClientBuildFailed, node.LogicalID, node.Protocol, err)
	}
	return proxy, nil
}

func unmarshalJSONMap(s string) (map[string]any, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var out map[string]any
	if err := d.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func transportValue(c domain.InboundProtocolCredential, keys ...string) string {
	if c.Transport == nil {
		return ""
	}
	for _, k := range keys {
		if val, ok := c.Transport[k]; ok {
			trimmed := strings.TrimSpace(val)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func extractALPN(c domain.InboundProtocolCredential) []string {
	alpnStr := transportValue(c, "alpn")
	if alpnStr == "" {
		return nil
	}
	parts := strings.Split(alpnStr, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

func splitWireGuardAddresses(addrs []string) (string, string) {
	var ipv4, ipv6 string
	for _, raw := range addrs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(s)
		if err != nil {
			addr, err := netip.ParseAddr(s)
			if err != nil {
				continue
			}
			if addr.Is4() && ipv4 == "" {
				ipv4 = addr.String()
			} else if addr.Is6() && ipv6 == "" {
				ipv6 = addr.String()
			}
			continue
		}
		if prefix.Addr().Is4() && ipv4 == "" {
			ipv4 = prefix.Addr().String()
		} else if prefix.Addr().Is6() && ipv6 == "" {
			ipv6 = prefix.Addr().String()
		}
	}
	return ipv4, ipv6
}

func parsePluginOpts(opts string) map[string]any {
	res := map[string]any{}
	for _, part := range strings.Split(opts, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if n, err := strconv.Atoi(v); err == nil {
				res[k] = n
			} else if b, err := strconv.ParseBool(v); err == nil {
				res[k] = b
			} else {
				res[k] = v
			}
		} else {
			res[part] = true
		}
	}
	return res
}
