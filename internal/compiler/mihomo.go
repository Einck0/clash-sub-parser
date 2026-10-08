package compiler

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/resolver"
	"gopkg.in/yaml.v3"
)

const (
	defaultMihomoProbeURL          = "https://www.gstatic.com/generate_204"
	defaultMihomoProbeInterval     = 300
	defaultMihomoURLTestTolerance  = 50
	defaultMihomoLoadBalancePolicy = "consistent-hashing"
	defaultMihomoRuleSetInterval   = 86400
)

func mihomoCapability() Capability {
	return Capability{
		Protocols: protocolSet(
			domain.ProtocolSS,
			domain.ProtocolVMess,
			domain.ProtocolVLESS,
			domain.ProtocolTrojan,
			domain.ProtocolHysteria2,
			domain.ProtocolWireGuard,
			domain.ProtocolTUIC,
			domain.ProtocolHTTP,
			domain.ProtocolSocks5,
			domain.ProtocolAnyTLS,
		),
		GroupTypes: groupSet(
			domain.GroupTypeSelect,
			domain.GroupTypeURLTest,
			domain.GroupTypeFallback,
			domain.GroupTypeLoadBalance,
		),
		RuleKinds: ruleSet(
			"DOMAIN",
			"DOMAIN-SUFFIX",
			"DOMAIN-KEYWORD",
			"IP-CIDR",
			"IP-CIDR6",
			"GEOIP",
			"GEOSITE",
			"RULE-SET",
			"SRC-IP-CIDR",
			"SRC-PORT",
			"DST-PORT",
			"PORT",
			"PROCESS-NAME",
			"MATCH",
		),
	}
}

func mihomoError(location, feature, reason string) error {
	return &CapabilityError{
		Target:   domain.TargetMihomo,
		Location: location,
		Feature:  feature,
		Reason:   reason,
	}
}

var allowedMihomoTransportKeys = map[domain.Protocol]map[string]bool{
	domain.ProtocolSS: {
		"network":     true,
		"plugin":      true,
		"plugin-opts": true,
		"plugin_opts": true,
	},
	domain.ProtocolVMess: {
		"network":            true,
		"tls":                true,
		"sni":                true,
		"servername":         true,
		"serverName":         true,
		"peer":               true,
		"skip_cert_verify":   true,
		"skip-cert-verify":   true,
		"skipcert":           true,
		"insecure":           true,
		"allowInsecure":      true,
		"alpn":               true,
		"fp":                 true,
		"fingerprint":        true,
		"client-fingerprint": true,
		"client_fingerprint": true,
		"path":               true,
		"host":               true,
		"service_name":       true,
		"serviceName":        true,
		"grpc-service-name":  true,
	},
	domain.ProtocolVLESS: {
		"network":            true,
		"tls":                true,
		"sni":                true,
		"servername":         true,
		"serverName":         true,
		"peer":               true,
		"skip_cert_verify":   true,
		"skip-cert-verify":   true,
		"skipcert":           true,
		"insecure":           true,
		"allowInsecure":      true,
		"alpn":               true,
		"flow":               true,
		"fp":                 true,
		"fingerprint":        true,
		"client-fingerprint": true,
		"client_fingerprint": true,
		"pbk":                true,
		"public-key":         true,
		"public_key":         true,
		"reality-public-key": true,
		"reality_public_key": true,
		"sid":                true,
		"short-id":           true,
		"short_id":           true,
		"reality-short-id":   true,
		"reality_short_id":   true,
		"path":               true,
		"host":               true,
		"service_name":       true,
		"serviceName":        true,
		"grpc-service-name":  true,
		"mode":               true,
		"headers":            true,
		"xhttp-opts":         true,
		"xhttp_opts":         true,
		"ech-opts":           true,
		"ech_opts":           true,
		"udp":                true,
		"encryption":         true,
		"extra":              true,
	},
	domain.ProtocolTrojan: {
		"network":            true,
		"tls":                true,
		"sni":                true,
		"servername":         true,
		"serverName":         true,
		"peer":               true,
		"skip_cert_verify":   true,
		"skip-cert-verify":   true,
		"skipcert":           true,
		"insecure":           true,
		"allowInsecure":      true,
		"alpn":               true,
		"fp":                 true,
		"fingerprint":        true,
		"client-fingerprint": true,
		"client_fingerprint": true,
		"path":               true,
		"host":               true,
		"service_name":       true,
		"serviceName":        true,
		"grpc-service-name":  true,
	},
	domain.ProtocolHysteria2: {
		"network":          true,
		"tls":              true,
		"sni":              true,
		"servername":       true,
		"serverName":       true,
		"peer":             true,
		"skip_cert_verify": true,
		"skip-cert-verify": true,
		"skipcert":         true,
		"insecure":         true,
		"allowInsecure":    true,
		"alpn":             true,
		"up":               true,
		"down":             true,
		"obfs":             true,
		"obfs-password":    true,
		"obfs_password":    true,
		"ports":            true,
		"server_ports":     true,
		"hy2_ports":        true,
		"mport":            true,
	},
	domain.ProtocolWireGuard: {
		"network": true,
	},
	domain.ProtocolTUIC: {
		"network":               true,
		"tls":                   true,
		"sni":                   true,
		"servername":            true,
		"serverName":            true,
		"peer":                  true,
		"skip_cert_verify":      true,
		"skip-cert-verify":      true,
		"skipcert":              true,
		"insecure":              true,
		"allowInsecure":         true,
		"alpn":                  true,
		"disable_sni":           true,
		"disable-sni":           true,
		"tuic_disable_sni":      true,
		"congestion_control":    true,
		"congestion-control":    true,
		"congestion_controller": true,
		"congestion-controller": true,
		"udp_relay_mode":        true,
		"udp-relay-mode":        true,
		"ports":                 true,
		"server_ports":          true,
		"hy2_ports":             true,
		"mport":                 true,
	},
	domain.ProtocolHTTP: {
		"network":          true,
		"tls":              true,
		"sni":              true,
		"servername":       true,
		"serverName":       true,
		"peer":             true,
		"skip_cert_verify": true,
		"skip-cert-verify": true,
		"skipcert":         true,
		"insecure":         true,
		"allowInsecure":    true,
		"headers":          true,
	},
	domain.ProtocolSocks5: {
		"network":          true,
		"tls":              true,
		"sni":              true,
		"servername":       true,
		"serverName":       true,
		"peer":             true,
		"skip_cert_verify": true,
		"skip-cert-verify": true,
		"skipcert":         true,
		"insecure":         true,
		"allowInsecure":    true,
		"udp":              true,
	},
	domain.ProtocolAnyTLS: {
		"network":                     true,
		"tls":                         true,
		"sni":                         true,
		"servername":                  true,
		"serverName":                  true,
		"peer":                        true,
		"skip_cert_verify":            true,
		"skip-cert-verify":            true,
		"skipcert":                    true,
		"insecure":                    true,
		"allowInsecure":               true,
		"alpn":                        true,
		"fp":                          true,
		"fingerprint":                 true,
		"client-fingerprint":          true,
		"client_fingerprint":          true,
		"udp":                         true,
		"ech-opts":                    true,
		"ech_opts":                    true,
		"idle-session-check-interval": true,
		"idle_session_check_interval": true,
		"idle-session-timeout":        true,
		"idle_session_timeout":        true,
		"min-idle-session":            true,
		"min_idle_session":            true,
		"disable-reuse":               true,
		"disable_reuse":               true,
	},
}

func validateMihomoCredentials(snapshot *resolver.ResolvedPolicySnapshot) error {
	if len(snapshot.Nodes) == 0 {
		return nil
	}
	for i, node := range snapshot.Nodes {
		loc := fmt.Sprintf("nodes[%d]", i)
		if err := validateCredentialEnvelope(domain.TargetMihomo, i, node); err != nil {
			return err
		}

		c := node.Credentials
		if err := validateMihomoTransportKeys(loc, node.Protocol, c.Transport); err != nil {
			return err
		}

		switch node.Protocol {
		case domain.ProtocolSS:
			net := strings.ToLower(transportValue(c, "network"))
			if net != "" && net != "tcp" && net != "udp" {
				return mihomoError(loc, "ss", fmt.Sprintf("unsupported network %q in ss transport", net))
			}
		case domain.ProtocolVMess:
			net := strings.ToLower(transportValue(c, "network"))
			if net != "" && net != "tcp" && net != "ws" && net != "grpc" {
				return mihomoError(loc, "vmess", fmt.Sprintf("unsupported network %q in vmess transport", net))
			}
		case domain.ProtocolVLESS:
			net := strings.ToLower(transportValue(c, "network"))
			if net != "" && net != "tcp" && net != "ws" && net != "grpc" && net != "xhttp" {
				return mihomoError(loc, "vless", fmt.Sprintf("unsupported network %q in vless transport", net))
			}
			pbk := transportValue(c, "pbk", "public-key", "public_key", "reality-public-key", "reality_public_key")
			sid := transportValue(c, "sid", "short-id", "short_id", "reality-short-id", "reality_short_id")
			if sid != "" && pbk == "" {
				return mihomoError(loc, "vless", "missing required reality public key in vless credentials")
			}
			flow := transportValue(c, "flow")
			if flow != "" && flow != "xtls-rprx-vision" && flow != "xtls-rprx-vision-udp443" {
				return mihomoError(loc, "vless", fmt.Sprintf("unsupported flow %q in vless transport", flow))
			}
		case domain.ProtocolTrojan:
			net := strings.ToLower(transportValue(c, "network"))
			if net != "" && net != "tcp" && net != "ws" && net != "grpc" {
				return mihomoError(loc, "trojan", fmt.Sprintf("unsupported network %q in trojan transport", net))
			}
		case domain.ProtocolHysteria2:
			net := strings.ToLower(transportValue(c, "network"))
			if net != "" && net != "quic" && net != "udp" && net != "tcp" {
				return mihomoError(loc, "hysteria2", fmt.Sprintf("unsupported network %q in hysteria2 transport", net))
			}
			obfs := transportValue(c, "obfs")
			obfsPw := transportValue(c, "obfs-password", "obfs_password")
			if obfs != "" && obfs != "salamander" {
				return mihomoError(loc, "hysteria2", fmt.Sprintf("unsupported obfs mode %q in hysteria2 transport", obfs))
			}
			if obfs == "salamander" && obfsPw == "" {
				return mihomoError(loc, "hysteria2", "missing required obfs-password for hysteria2 salamander obfs")
			}
			if obfs == "" && obfsPw != "" {
				return mihomoError(loc, "hysteria2", "obfs-password requires obfs mode in hysteria2 transport")
			}
		case domain.ProtocolWireGuard:
			net := strings.ToLower(transportValue(c, "network"))
			if net != "" && net != "wireguard" && net != "udp" {
				return mihomoError(loc, "wireguard", fmt.Sprintf("unsupported network %q in wireguard transport", net))
			}
			if _, _, err := splitMihomoWireGuardAddresses(loc, c.LocalAddress); err != nil {
				return err
			}
		case domain.ProtocolTUIC:
			net := strings.ToLower(transportValue(c, "network"))
			if net != "" && net != "quic" && net != "udp" && net != "tcp" {
				return mihomoError(loc, "tuic", fmt.Sprintf("unsupported network %q in tuic transport", net))
			}
			cc := strings.ToLower(transportValue(c, "congestion_control", "congestion-control", "congestion_controller", "congestion-controller"))
			if cc != "" && cc != "cubic" && cc != "new_reno" && cc != "bbr" {
				return mihomoError(loc, "tuic", fmt.Sprintf("unsupported congestion controller %q in tuic credentials", cc))
			}
			mode := strings.ToLower(transportValue(c, "udp_relay_mode", "udp-relay-mode"))
			if mode != "" && mode != "native" && mode != "quic" {
				return mihomoError(loc, "tuic", fmt.Sprintf("unsupported udp relay mode %q in tuic credentials", mode))
			}
		case domain.ProtocolAnyTLS:
			if strings.TrimSpace(c.Password) == "" {
				return mihomoError(loc, "anytls", "missing required password in anytls credentials")
			}
		}
	}
	return nil
}

func validateMihomoTransportKeys(loc string, proto domain.Protocol, transport map[string]string) error {
	if len(transport) == 0 {
		return nil
	}
	allowed := allowedMihomoTransportKeys[proto]
	for rawKey, rawVal := range transport {
		if strings.TrimSpace(rawVal) == "" {
			continue
		}
		key := strings.TrimSpace(rawKey)
		if strings.HasPrefix(key, "extra") || strings.HasPrefix(key, "unknown") {
			continue
		}
		if !allowed[key] {
			return mihomoError(loc, string(proto), fmt.Sprintf("unsupported transport parameter %q for %s", key, proto))
		}
	}
	return nil
}

func splitMihomoWireGuardAddresses(loc string, addrs []string) (string, string, error) {
	var ipv4, ipv6 string
	for _, raw := range addrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return "", "", mihomoError(loc, "wireguard", "invalid local_address CIDR in wireguard credentials")
		}
		if prefix.Addr().Is4() {
			if ipv4 != "" && ipv4 != prefix.String() {
				return "", "", mihomoError(loc, "wireguard", "multiple IPv4 local_address CIDRs cannot be represented in mihomo wireguard")
			}
			ipv4 = prefix.String()
		} else if prefix.Addr().Is6() {
			if ipv6 != "" && ipv6 != prefix.String() {
				return "", "", mihomoError(loc, "wireguard", "multiple IPv6 local_address CIDRs cannot be represented in mihomo wireguard")
			}
			ipv6 = prefix.String()
		}
	}
	if ipv4 == "" && ipv6 == "" {
		return "", "", mihomoError(loc, "wireguard", "missing required local_address in wireguard credentials")
	}
	return ipv4, ipv6, nil
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
	for _, key := range keys {
		if c.Transport != nil {
			if v := strings.TrimSpace(c.Transport[key]); v != "" {
				return v
			}
		}
		switch key {
		case "sni", "servername", "serverName", "peer":
			if v := strings.TrimSpace(c.SNI); v != "" {
				return v
			}
		case "disable_sni", "disable-sni", "tuic_disable_sni":
			if c.DisableSNI {
				return "true"
			}
		case "congestion_control", "congestion-control", "congestion_controller", "congestion-controller":
			if v := strings.TrimSpace(c.CongestionControl); v != "" {
				return v
			}
		case "udp_relay_mode", "udp-relay-mode":
			if v := strings.TrimSpace(c.UDPRelayMode); v != "" {
				return v
			}
		}
	}
	return ""
}

func extractMihomoALPN(c domain.InboundProtocolCredential) []string {
	if len(c.ALPN) > 0 {
		out := make([]string, 0, len(c.ALPN))
		for _, item := range c.ALPN {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if c.Transport == nil {
		return nil
	}
	raw := strings.TrimSpace(c.Transport["alpn"])
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

type mihomoConfig struct {
	Proxies       []any                         `yaml:"proxies"`
	ProxyGroups   []mihomoGroup                 `yaml:"proxy-groups"`
	RuleProviders map[string]mihomoRuleProvider `yaml:"rule-providers,omitempty"`
	Rules         []string                      `yaml:"rules"`
}

type mihomoGroup struct {
	EmptyFallback string   `yaml:"empty-fallback,omitempty"`
	Name          string   `yaml:"name"`
	Type          string   `yaml:"type"`
	Proxies       []string `yaml:"proxies"`
	URL           string   `yaml:"url,omitempty"`
	Interval      int      `yaml:"interval,omitempty"`
	Tolerance     int      `yaml:"tolerance,omitempty"`
	Strategy      string   `yaml:"strategy,omitempty"`
}

type mihomoRuleProvider struct {
	Type     string `yaml:"type"`
	Behavior string `yaml:"behavior"`
	URL      string `yaml:"url"`
	Interval int    `yaml:"interval"`
}

type mihomoHysteria2Proxy struct {
	Name           string   `yaml:"name"`
	Type           string   `yaml:"type"`
	Server         string   `yaml:"server"`
	Port           int      `yaml:"port"`
	Ports          string   `yaml:"ports,omitempty"`
	Password       string   `yaml:"password"`
	SNI            string   `yaml:"sni,omitempty"`
	SkipCertVerify bool     `yaml:"skip-cert-verify,omitempty"`
	ALPN           []string `yaml:"alpn,omitempty"`
	Up             string   `yaml:"up,omitempty"`
	Down           string   `yaml:"down,omitempty"`
	Obfs           string   `yaml:"obfs,omitempty"`
	ObfsPassword   string   `yaml:"obfs-password,omitempty"`
}

type mihomoSSProxy struct {
	Name       string         `yaml:"name"`
	Type       string         `yaml:"type"`
	Server     string         `yaml:"server"`
	Port       int            `yaml:"port"`
	Cipher     string         `yaml:"cipher"`
	Password   string         `yaml:"password"`
	Plugin     string         `yaml:"plugin,omitempty"`
	PluginOpts map[string]any `yaml:"plugin-opts,omitempty"`
}

type mihomoWSOpts struct {
	Path    string            `yaml:"path,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type mihomoGRPCOpts struct {
	GRPCServiceName string `yaml:"grpc-service-name,omitempty"`
}

type mihomoRealityOpts struct {
	PublicKey string `yaml:"public-key"`
	ShortID   string `yaml:"short-id,omitempty"`
}

type mihomoVMessProxy struct {
	Name              string          `yaml:"name"`
	Type              string          `yaml:"type"`
	Server            string          `yaml:"server"`
	Port              int             `yaml:"port"`
	UUID              string          `yaml:"uuid"`
	AlterID           int             `yaml:"alterId"`
	Cipher            string          `yaml:"cipher"`
	Network           string          `yaml:"network,omitempty"`
	TLS               bool            `yaml:"tls,omitempty"`
	Servername        string          `yaml:"servername,omitempty"`
	SkipCertVerify    bool            `yaml:"skip-cert-verify,omitempty"`
	ALPN              []string        `yaml:"alpn,omitempty"`
	ClientFingerprint string          `yaml:"client-fingerprint,omitempty"`
	WSOpts            *mihomoWSOpts   `yaml:"ws-opts,omitempty"`
	GRPCOpts          *mihomoGRPCOpts `yaml:"grpc-opts,omitempty"`
}

type mihomoXHTTPOpts struct {
	Path    string            `yaml:"path,omitempty"`
	Host    string            `yaml:"host,omitempty"`
	Mode    string            `yaml:"mode,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type mihomoVLESSProxy struct {
	Name              string             `yaml:"name"`
	Type              string             `yaml:"type"`
	Server            string             `yaml:"server"`
	Port              int                `yaml:"port"`
	UUID              string             `yaml:"uuid"`
	Network           string             `yaml:"network,omitempty"`
	TLS               bool               `yaml:"tls,omitempty"`
	Servername        string             `yaml:"servername,omitempty"`
	SkipCertVerify    bool               `yaml:"skip-cert-verify,omitempty"`
	ALPN              []string           `yaml:"alpn,omitempty"`
	Flow              string             `yaml:"flow,omitempty"`
	ClientFingerprint string             `yaml:"client-fingerprint,omitempty"`
	RealityOpts       *mihomoRealityOpts `yaml:"reality-opts,omitempty"`
	ECHOpts           map[string]any     `yaml:"ech-opts,omitempty"`
	WSOpts            *mihomoWSOpts      `yaml:"ws-opts,omitempty"`
	GRPCOpts          *mihomoGRPCOpts    `yaml:"grpc-opts,omitempty"`
	XHTTPOpts         any                `yaml:"xhttp-opts,omitempty"`
}

type mihomoHTTPProxy struct {
	Name           string            `yaml:"name"`
	Type           string            `yaml:"type"`
	Server         string            `yaml:"server"`
	Port           int               `yaml:"port"`
	Username       string            `yaml:"username,omitempty"`
	Password       string            `yaml:"password,omitempty"`
	TLS            bool              `yaml:"tls,omitempty"`
	SNI            string            `yaml:"sni,omitempty"`
	SkipCertVerify bool              `yaml:"skip-cert-verify,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
}

type mihomoSocks5Proxy struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	Server         string `yaml:"server"`
	Port           int    `yaml:"port"`
	Username       string `yaml:"username,omitempty"`
	Password       string `yaml:"password,omitempty"`
	TLS            bool   `yaml:"tls,omitempty"`
	SNI            string `yaml:"sni,omitempty"`
	SkipCertVerify bool   `yaml:"skip-cert-verify,omitempty"`
	UDP            bool   `yaml:"udp,omitempty"`
}

type mihomoAnyTLSProxy struct {
	Name                     string         `yaml:"name"`
	Type                     string         `yaml:"type"`
	Server                   string         `yaml:"server"`
	Port                     int            `yaml:"port"`
	Password                 string         `yaml:"password"`
	SNI                      string         `yaml:"sni,omitempty"`
	ALPN                     []string       `yaml:"alpn,omitempty"`
	ClientFingerprint        string         `yaml:"client-fingerprint,omitempty"`
	SkipCertVerify           bool           `yaml:"skip-cert-verify,omitempty"`
	UDP                      bool           `yaml:"udp,omitempty"`
	ECHOpts                  map[string]any `yaml:"ech-opts,omitempty"`
	IdleSessionCheckInterval int            `yaml:"idle-session-check-interval,omitempty"`
	IdleSessionTimeout       int            `yaml:"idle-session-timeout,omitempty"`
	MinIdleSession           int            `yaml:"min-idle-session,omitempty"`
	DisableReuse             bool           `yaml:"disable-reuse,omitempty"`
}

type mihomoTrojanProxy struct {
	Name              string          `yaml:"name"`
	Type              string          `yaml:"type"`
	Server            string          `yaml:"server"`
	Port              int             `yaml:"port"`
	Password          string          `yaml:"password"`
	Network           string          `yaml:"network,omitempty"`
	SNI               string          `yaml:"sni,omitempty"`
	SkipCertVerify    bool            `yaml:"skip-cert-verify,omitempty"`
	ALPN              []string        `yaml:"alpn,omitempty"`
	ClientFingerprint string          `yaml:"client-fingerprint,omitempty"`
	WSOpts            *mihomoWSOpts   `yaml:"ws-opts,omitempty"`
	GRPCOpts          *mihomoGRPCOpts `yaml:"grpc-opts,omitempty"`
}

type mihomoWireGuardProxy struct {
	Name             string   `yaml:"name"`
	Type             string   `yaml:"type"`
	Server           string   `yaml:"server"`
	Port             int      `yaml:"port"`
	IP               string   `yaml:"ip,omitempty"`
	IPv6             string   `yaml:"ipv6,omitempty"`
	PrivateKey       string   `yaml:"private-key"`
	PublicKey        string   `yaml:"public-key"`
	PreSharedKey     string   `yaml:"pre-shared-key,omitempty"`
	Reserved         []int    `yaml:"reserved,omitempty,flow"`
	MTU              int      `yaml:"mtu,omitempty"`
	DNS              []string `yaml:"dns,omitempty"`
	RemoteDNSResolve bool     `yaml:"remote-dns-resolve,omitempty"`
	UDP              bool     `yaml:"udp"`
}

type mihomoTUICProxy struct {
	Name                 string   `yaml:"name"`
	Type                 string   `yaml:"type"`
	Server               string   `yaml:"server"`
	Port                 int      `yaml:"port"`
	UUID                 string   `yaml:"uuid"`
	Password             string   `yaml:"password"`
	CongestionController string   `yaml:"congestion-controller,omitempty"`
	UDPRelayMode         string   `yaml:"udp-relay-mode,omitempty"`
	ALPN                 []string `yaml:"alpn,omitempty"`
	SNI                  string   `yaml:"sni,omitempty"`
	DisableSNI           bool     `yaml:"disable-sni,omitempty"`
	SkipCertVerify       bool     `yaml:"skip-cert-verify,omitempty"`
}

func renderMihomo(snapshot *resolver.ResolvedPolicySnapshot) ([]byte, error) {
	if err := validateMihomoCredentials(snapshot); err != nil {
		return nil, err
	}

	proxies := make([]any, 0, len(snapshot.Nodes))
	for i, node := range snapshot.Nodes {
		loc := fmt.Sprintf("nodes[%d]", i)
		c := node.Credentials
		switch node.Protocol {
		case domain.ProtocolHysteria2:
			ports := domain.ExtractHy2Ports(c.Transport)
			if ports == "" {
				ports = transportValue(c, "mport")
			}
			p := mihomoHysteria2Proxy{
				Name:           node.DisplayName,
				Type:           "hysteria2",
				Server:         node.Server,
				Port:           node.Port,
				Ports:          ports,
				Password:       c.Password,
				SNI:            transportValue(c, "sni", "servername", "serverName", "peer"),
				SkipCertVerify: domain.HasInsecureTransport(c.Transport),
				ALPN:           extractMihomoALPN(c),
				Up:             transportValue(c, "up"),
				Down:           transportValue(c, "down"),
				Obfs:           transportValue(c, "obfs"),
				ObfsPassword:   transportValue(c, "obfs-password", "obfs_password"),
			}
			proxies = append(proxies, p)
		case domain.ProtocolSS:
			p := mihomoSSProxy{
				Name:     node.DisplayName,
				Type:     "ss",
				Server:   node.Server,
				Port:     node.Port,
				Cipher:   c.Method,
				Password: c.Password,
				Plugin:   transportValue(c, "plugin"),
			}
			proxies = append(proxies, p)
		case domain.ProtocolVMess:
			cipher := strings.TrimSpace(c.Method)
			if cipher == "" {
				cipher = "auto"
			}
			p := mihomoVMessProxy{
				Name:              node.DisplayName,
				Type:              "vmess",
				Server:            node.Server,
				Port:              node.Port,
				UUID:              strings.TrimSpace(c.UUID),
				AlterID:           c.AlterID,
				Cipher:            cipher,
				Network:           strings.ToLower(transportValue(c, "network")),
				TLS:               domain.IsTruthy(transportValue(c, "tls")),
				Servername:        transportValue(c, "sni", "servername", "serverName", "peer"),
				SkipCertVerify:    domain.HasInsecureTransport(c.Transport),
				ALPN:              extractMihomoALPN(c),
				ClientFingerprint: transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"),
			}
			if p.Network == "ws" {
				ws := &mihomoWSOpts{}
				if path := transportValue(c, "path"); path != "" {
					ws.Path = path
				}
				if host := transportValue(c, "host"); host != "" {
					ws.Headers = map[string]string{"Host": host}
				}
				p.WSOpts = ws
			} else if p.Network == "grpc" {
				if svc := transportValue(c, "service_name", "serviceName", "grpc-service-name"); svc != "" {
					p.GRPCOpts = &mihomoGRPCOpts{GRPCServiceName: svc}
				}
			}
			proxies = append(proxies, p)
		case domain.ProtocolVLESS:
			pbk := transportValue(c, "pbk", "public-key", "public_key", "reality-public-key", "reality_public_key")
			sid := transportValue(c, "sid", "short-id", "short_id", "reality-short-id", "reality_short_id")
			var realityOpts *mihomoRealityOpts
			if pbk != "" {
				realityOpts = &mihomoRealityOpts{
					PublicKey: pbk,
					ShortID:   sid,
				}
			}
			p := mihomoVLESSProxy{
				Name:              node.DisplayName,
				Type:              "vless",
				Server:            node.Server,
				Port:              node.Port,
				UUID:              strings.TrimSpace(c.UUID),
				Network:           strings.ToLower(transportValue(c, "network")),
				TLS:               domain.IsTruthy(transportValue(c, "tls")) || realityOpts != nil,
				Servername:        transportValue(c, "sni", "servername", "serverName", "peer"),
				SkipCertVerify:    domain.HasInsecureTransport(c.Transport),
				ALPN:              extractMihomoALPN(c),
				Flow:              transportValue(c, "flow"),
				ClientFingerprint: transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"),
				RealityOpts:       realityOpts,
			}
			if echJSON := transportValue(c, "ech-opts", "ech_opts"); echJSON != "" {
				if echOpts, err := unmarshalJSONMap(echJSON); err == nil && len(echOpts) > 0 {
					p.ECHOpts = echOpts
				}
			}
			if p.Network == "ws" {
				ws := &mihomoWSOpts{}
				if path := transportValue(c, "path"); path != "" {
					ws.Path = path
				}
				if host := transportValue(c, "host"); host != "" {
					ws.Headers = map[string]string{"Host": host}
				}
				p.WSOpts = ws
			} else if p.Network == "grpc" {
				if svc := transportValue(c, "service_name", "serviceName", "grpc-service-name"); svc != "" {
					p.GRPCOpts = &mihomoGRPCOpts{GRPCServiceName: svc}
				}
			} else if p.Network == "xhttp" {
				if xhttpJSON := transportValue(c, "xhttp-opts", "xhttp_opts"); xhttpJSON != "" {
					if xhttpMap, err := unmarshalJSONMap(xhttpJSON); err == nil && len(xhttpMap) > 0 {
						p.XHTTPOpts = xhttpMap
					}
				}
				if p.XHTTPOpts == nil {
					xhttp := &mihomoXHTTPOpts{}
					if path := transportValue(c, "path"); path != "" {
						xhttp.Path = path
					}
					if host := transportValue(c, "host"); host != "" {
						xhttp.Host = host
					}
					if mode := transportValue(c, "mode"); mode != "" {
						xhttp.Mode = mode
					}
					if hJSON := transportValue(c, "headers"); hJSON != "" {
						var headers map[string]string
						if err := json.Unmarshal([]byte(hJSON), &headers); err == nil && len(headers) > 0 {
							xhttp.Headers = headers
						}
					}
					p.XHTTPOpts = xhttp
				}
			}
			proxies = append(proxies, p)
		case domain.ProtocolTrojan:
			p := mihomoTrojanProxy{
				Name:              node.DisplayName,
				Type:              "trojan",
				Server:            node.Server,
				Port:              node.Port,
				Password:          c.Password,
				Network:           strings.ToLower(transportValue(c, "network")),
				SNI:               transportValue(c, "sni", "servername", "serverName", "peer"),
				SkipCertVerify:    domain.HasInsecureTransport(c.Transport),
				ALPN:              extractMihomoALPN(c),
				ClientFingerprint: transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"),
			}
			if p.Network == "ws" {
				ws := &mihomoWSOpts{}
				if path := transportValue(c, "path"); path != "" {
					ws.Path = path
				}
				if host := transportValue(c, "host"); host != "" {
					ws.Headers = map[string]string{"Host": host}
				}
				p.WSOpts = ws
			} else if p.Network == "grpc" {
				if svc := transportValue(c, "service_name", "serviceName", "grpc-service-name"); svc != "" {
					p.GRPCOpts = &mihomoGRPCOpts{GRPCServiceName: svc}
				}
			}
			proxies = append(proxies, p)
		case domain.ProtocolWireGuard:
			ipv4, ipv6, err := splitMihomoWireGuardAddresses(loc, c.LocalAddress)
			if err != nil {
				return nil, err
			}
			var reserved []int
			if len(c.Reserved) > 0 {
				reserved = make([]int, len(c.Reserved))
				for idx, b := range c.Reserved {
					reserved[idx] = int(b)
				}
			}
			var dns []string
			for _, d := range c.DNS {
				if trimmed := strings.TrimSpace(d); trimmed != "" {
					dns = append(dns, trimmed)
				}
			}
			p := mihomoWireGuardProxy{
				Name:             node.DisplayName,
				Type:             "wireguard",
				Server:           node.Server,
				Port:             node.Port,
				IP:               ipv4,
				IPv6:             ipv6,
				PrivateKey:       strings.TrimSpace(c.PrivateKey),
				PublicKey:        strings.TrimSpace(c.PublicKey),
				PreSharedKey:     c.EffectivePreSharedKey(),
				Reserved:         reserved,
				MTU:              c.MTU,
				DNS:              dns,
				RemoteDNSResolve: len(dns) > 0,
				UDP:              true,
			}
			proxies = append(proxies, p)
		case domain.ProtocolTUIC:
			p := mihomoTUICProxy{
				Name:                 node.DisplayName,
				Type:                 "tuic",
				Server:               node.Server,
				Port:                 node.Port,
				UUID:                 strings.TrimSpace(c.UUID),
				Password:             c.Password,
				CongestionController: strings.ToLower(transportValue(c, "congestion_control", "congestion-control", "congestion_controller", "congestion-controller")),
				UDPRelayMode:         strings.ToLower(transportValue(c, "udp_relay_mode", "udp-relay-mode")),
				ALPN:                 extractMihomoALPN(c),
				SNI:                  transportValue(c, "sni", "servername", "serverName", "peer"),
				DisableSNI:           c.DisableSNI || domain.HasTUICDisableSNI(c.Transport),
				SkipCertVerify:       domain.HasInsecureTransport(c.Transport),
			}
			proxies = append(proxies, p)
		case domain.ProtocolHTTP:
			var headers map[string]string
			if hJSON := transportValue(c, "headers"); hJSON != "" {
				_ = json.Unmarshal([]byte(hJSON), &headers)
			}
			p := mihomoHTTPProxy{
				Name:           node.DisplayName,
				Type:           "http",
				Server:         node.Server,
				Port:           node.Port,
				Username:       c.Username,
				Password:       c.Password,
				TLS:            domain.IsTruthy(transportValue(c, "tls")),
				SNI:            transportValue(c, "sni", "servername", "serverName", "peer"),
				SkipCertVerify: domain.HasInsecureTransport(c.Transport),
				Headers:        headers,
			}
			proxies = append(proxies, p)
		case domain.ProtocolSocks5:
			p := mihomoSocks5Proxy{
				Name:           node.DisplayName,
				Type:           "socks5",
				Server:         node.Server,
				Port:           node.Port,
				Username:       c.Username,
				Password:       c.Password,
				TLS:            domain.IsTruthy(transportValue(c, "tls")),
				SNI:            transportValue(c, "sni", "servername", "serverName", "peer"),
				SkipCertVerify: domain.HasInsecureTransport(c.Transport),
				UDP:            domain.IsTruthy(transportValue(c, "udp")),
			}
			proxies = append(proxies, p)
		case domain.ProtocolAnyTLS:
			var echOpts map[string]any
			if echJSON := transportValue(c, "ech-opts", "ech_opts"); echJSON != "" {
				if parsed, err := unmarshalJSONMap(echJSON); err == nil && len(parsed) > 0 {
					echOpts = parsed
				}
			}
			idleCheck, _ := strconv.Atoi(transportValue(c, "idle-session-check-interval", "idle_session_check_interval"))
			idleTimeout, _ := strconv.Atoi(transportValue(c, "idle-session-timeout", "idle_session_timeout"))
			minIdle, _ := strconv.Atoi(transportValue(c, "min-idle-session", "min_idle_session"))

			p := mihomoAnyTLSProxy{
				Name:                     node.DisplayName,
				Type:                     "anytls",
				Server:                   node.Server,
				Port:                     node.Port,
				Password:                 c.Password,
				SNI:                      transportValue(c, "sni", "servername", "serverName", "peer"),
				ALPN:                     extractMihomoALPN(c),
				ClientFingerprint:        transportValue(c, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"),
				SkipCertVerify:           domain.HasInsecureTransport(c.Transport),
				UDP:                      domain.IsTruthy(transportValue(c, "udp")),
				ECHOpts:                  echOpts,
				IdleSessionCheckInterval: idleCheck,
				IdleSessionTimeout:       idleTimeout,
				MinIdleSession:           minIdle,
				DisableReuse:             domain.IsTruthy(transportValue(c, "disable-reuse", "disable_reuse")),
			}
			proxies = append(proxies, p)
		}
	}

	groups := make([]mihomoGroup, 0, len(snapshot.Groups))
	groupNameSet := make(map[string]bool, len(snapshot.Groups)*2)
	for _, g := range snapshot.Groups {
		if g.Name != "" {
			groupNameSet[g.Name] = true
		}
		if g.ID != "" {
			groupNameSet[g.ID] = true
		}
	}

	for i, group := range snapshot.Groups {
		loc := fmt.Sprintf("groups[%d]", i)
		if !group.UsesEmptyPass() && (len(group.Members) == 0 || (group.EmptyFallbackPass && len(group.AllNodeLogicalIDs) == 0)) {
			return nil, mihomoError(loc, group.Name, fmt.Sprintf("proxy group %q has no proxies (required_nonempty)", group.Name))
		}
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			if member.Kind == resolver.MemberKindGroup {
				tID := strings.TrimSpace(member.TargetID)
				tName := strings.TrimSpace(member.DisplayName)
				if (tID != "" && !groupNameSet[tID]) && (tName != "" && !groupNameSet[tName]) {
					return nil, mihomoError(loc, group.Name, fmt.Sprintf("group %q references non-existent child group %q", group.Name, tName))
				}
			}
			members = append(members, member.DisplayName)
		}
		if group.UsesEmptyPass() {
			members = []string{"PASS"}
		}
		mg, err := buildMihomoGroup(loc, group, members)
		if err != nil {
			return nil, err
		}
		// Never let a flagged nonempty group fall back to COMPATIBLE's DIRECT
		// if its native health selection later runs out of candidates.
		if group.EmptyFallbackPass {
			mg.EmptyFallback = "REJECT"
		}
		if group.UsesEmptyPass() {
			mg.EmptyFallback = "PASS"
		}
		groups = append(groups, mg)
	}

	// Validate rule targets against rendered groups
	for i, rule := range snapshot.Rules {
		loc := fmt.Sprintf("rules[%d]", i)
		tName := strings.TrimSpace(rule.TargetGroupName)
		tID := strings.TrimSpace(rule.TargetGroupID)
		if tName != "" && tName != "DIRECT" && tName != "REJECT" && tName != "REJECT-DROP" && tName != "PASS" {
			if !groupNameSet[tName] && !groupNameSet[tID] {
				return nil, mihomoError(loc, tName, fmt.Sprintf("rule %q targets non-existent group %q", rule.Expression, tName))
			}
		}
	}

	rules, ruleProviders, err := renderMihomoRules(snapshot.Rules)
	if err != nil {
		return nil, err
	}

	return yaml.Marshal(mihomoConfig{
		Proxies:       proxies,
		ProxyGroups:   groups,
		RuleProviders: ruleProviders,
		Rules:         rules,
	})
}

func buildMihomoGroup(loc string, group resolver.ResolvedGroup, members []string) (mihomoGroup, error) {
	switch group.GroupType {
	case domain.GroupTypeSelect:
		return mihomoGroup{
			Name:    group.Name,
			Type:    "select",
			Proxies: members,
		}, nil
	case domain.GroupTypeURLTest:
		return mihomoGroup{
			Name:      group.Name,
			Type:      "url-test",
			Proxies:   members,
			URL:       defaultMihomoProbeURL,
			Interval:  defaultMihomoProbeInterval,
			Tolerance: defaultMihomoURLTestTolerance,
		}, nil
	case domain.GroupTypeFallback:
		return mihomoGroup{
			Name:     group.Name,
			Type:     "fallback",
			Proxies:  members,
			URL:      defaultMihomoProbeURL,
			Interval: defaultMihomoProbeInterval,
		}, nil
	case domain.GroupTypeLoadBalance:
		return mihomoGroup{
			Name:     group.Name,
			Type:     "load-balance",
			Proxies:  members,
			URL:      defaultMihomoProbeURL,
			Interval: defaultMihomoProbeInterval,
			Strategy: defaultMihomoLoadBalancePolicy,
		}, nil
	default:
		return mihomoGroup{}, mihomoError(loc, string(group.GroupType), "policy group type is not supported")
	}
}

func renderMihomoRules(resolvedRules []resolver.ResolvedRule) ([]string, map[string]mihomoRuleProvider, error) {
	rules := make([]string, 0, len(resolvedRules))
	var providers map[string]mihomoRuleProvider

	for i, rule := range resolvedRules {
		loc := fmt.Sprintf("rules[%d]", i)
		expr := strings.TrimSpace(rule.Expression)
		kind := ruleKind(expr)
		target := strings.TrimSpace(rule.TargetGroupName)
		parts := strings.Split(expr, ",")

		if domain.IsMatchRule(expr) {
			if len(parts) > 1 {
				return nil, nil, mihomoError(loc, kind, "MATCH rule does not accept parameters")
			}
			rules = append(rules, "MATCH,"+target)
			continue
		}

		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return nil, nil, mihomoError(loc, kind, "routing rule value is required")
		}
		val := strings.TrimSpace(parts[1])
		if strings.ContainsAny(val, " \t\r\n") {
			return nil, nil, mihomoError(loc, kind, fmt.Sprintf("invalid whitespace in %s rule value", kind))
		}

		switch kind {
		case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "GEOSITE", "PROCESS-NAME":
			if len(parts) > 2 {
				return nil, nil, mihomoError(loc, kind, fmt.Sprintf("unsupported parameter %q in %s rule", strings.TrimSpace(parts[2]), kind))
			}
			rules = append(rules, fmt.Sprintf("%s,%s,%s", kind, val, target))

		case "GEOIP":
			noResolve, err := parseOptionalNoResolve(loc, kind, parts[2:])
			if err != nil {
				return nil, nil, err
			}
			line := fmt.Sprintf("GEOIP,%s,%s", val, target)
			if noResolve {
				line += ",no-resolve"
			}
			rules = append(rules, line)

		case "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR":
			prefix, err := netip.ParsePrefix(val)
			if err != nil {
				return nil, nil, mihomoError(loc, kind, fmt.Sprintf("invalid CIDR %q in %s rule", val, kind))
			}
			if kind == "IP-CIDR6" && !prefix.Addr().Is6() {
				return nil, nil, mihomoError(loc, kind, "IP-CIDR6 rule requires an IPv6 CIDR prefix")
			}
			if kind == "IP-CIDR" && !prefix.Addr().Is4() {
				return nil, nil, mihomoError(loc, kind, "IP-CIDR rule requires an IPv4 CIDR prefix")
			}
			noResolve, err := parseOptionalNoResolve(loc, kind, parts[2:])
			if err != nil {
				return nil, nil, err
			}
			line := fmt.Sprintf("%s,%s,%s", kind, val, target)
			if noResolve {
				line += ",no-resolve"
			}
			rules = append(rules, line)

		case "SRC-PORT", "DST-PORT", "PORT":
			if len(parts) > 2 {
				return nil, nil, mihomoError(loc, kind, fmt.Sprintf("unsupported parameter %q in %s rule", strings.TrimSpace(parts[2]), kind))
			}
			if err := validateMihomoPortExpr(loc, kind, val); err != nil {
				return nil, nil, err
			}
			renderedKind := kind
			if renderedKind == "PORT" {
				renderedKind = "DST-PORT"
			}
			rules = append(rules, fmt.Sprintf("%s,%s,%s", renderedKind, val, target))

		case "RULE-SET":
			providerName, providerURL, noResolve, err := parseMihomoRuleSetExpr(loc, parts[1:], providers)
			if err != nil {
				return nil, nil, err
			}
			if providers == nil {
				providers = make(map[string]mihomoRuleProvider)
			}
			if existing, exists := providers[providerName]; exists && existing.URL != providerURL {
				return nil, nil, mihomoError(loc, kind, fmt.Sprintf("conflicting URLs for RULE-SET provider %q", providerName))
			}
			providers[providerName] = mihomoRuleProvider{
				Type:     "http",
				Behavior: "classical",
				URL:      providerURL,
				Interval: defaultMihomoRuleSetInterval,
			}
			line := fmt.Sprintf("RULE-SET,%s,%s", providerName, target)
			if noResolve {
				line += ",no-resolve"
			}
			rules = append(rules, line)

		default:
			return nil, nil, mihomoError(loc, kind, "routing rule kind is not supported")
		}
	}

	return rules, providers, nil
}

func parseOptionalNoResolve(loc, kind string, extra []string) (bool, error) {
	if len(extra) == 0 {
		return false, nil
	}
	if len(extra) > 1 {
		return false, mihomoError(loc, kind, fmt.Sprintf("too many parameters in %s rule", kind))
	}
	opt := strings.ToLower(strings.TrimSpace(extra[0]))
	if opt != "no-resolve" {
		return false, mihomoError(loc, kind, fmt.Sprintf("unsupported parameter %q in %s rule", strings.TrimSpace(extra[0]), kind))
	}
	return true, nil
}

func validateMihomoPortExpr(loc, kind, val string) error {
	segments := strings.Split(val, "/")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return mihomoError(loc, kind, "invalid port expression in routing rule")
		}
		if strings.Contains(seg, "-") {
			bounds := strings.Split(seg, "-")
			if len(bounds) != 2 {
				return mihomoError(loc, kind, "invalid port range in routing rule")
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || start < 1 || start > 65535 || end < 1 || end > 65535 || start > end {
				return mihomoError(loc, kind, "port range out of bounds in routing rule")
			}
			continue
		}
		port, err := strconv.Atoi(seg)
		if err != nil || port < 1 || port > 65535 {
			return mihomoError(loc, kind, "port number out of bounds in routing rule")
		}
	}
	return nil
}

func parseMihomoRuleSetExpr(loc string, args []string, existingProviders map[string]mihomoRuleProvider) (string, string, bool, error) {
	if len(args) == 0 {
		return "", "", false, mihomoError(loc, "RULE-SET", "routing rule value is required")
	}
	first := strings.TrimSpace(args[0])
	if first == "" {
		return "", "", false, mihomoError(loc, "RULE-SET", "routing rule value is required")
	}

	if isRuleSetURLCandidate(first) {
		validURL, err := validateRuleSetURL(loc, first)
		if err != nil {
			return "", "", false, err
		}
		noResolve, err := parseOptionalNoResolve(loc, "RULE-SET", args[1:])
		if err != nil {
			return "", "", false, err
		}
		providerName := sanitizeRuleSetProviderName(validURL)
		return providerName, validURL, noResolve, nil
	}

	if !isValidRuleSetIdentifier(first) {
		return "", "", false, mihomoError(loc, "RULE-SET", fmt.Sprintf("invalid RULE-SET provider name %q", first))
	}

	if len(args) >= 2 && isRuleSetSecondArgURLOrPath(strings.TrimSpace(args[1])) {
		validURL, err := validateRuleSetURL(loc, strings.TrimSpace(args[1]))
		if err != nil {
			return "", "", false, err
		}
		noResolve, err := parseOptionalNoResolve(loc, "RULE-SET", args[2:])
		if err != nil {
			return "", "", false, err
		}
		return first, validURL, noResolve, nil
	}

	noResolve, err := parseOptionalNoResolve(loc, "RULE-SET", args[1:])
	if err != nil {
		return "", "", false, err
	}
	if existing, ok := existingProviders[first]; ok && existing.URL != "" {
		return first, existing.URL, noResolve, nil
	}
	return "", "", false, mihomoError(loc, "RULE-SET", fmt.Sprintf("missing explicit https:// URL for RULE-SET provider %q", first))
}

func isRuleSetURLCandidate(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.Contains(lower, "://")
}

func isRuleSetSecondArgURLOrPath(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "://") ||
		strings.Contains(lower, "/") ||
		strings.HasPrefix(lower, ".") ||
		strings.HasSuffix(lower, ".invalid") ||
		strings.HasSuffix(lower, ".yaml") ||
		strings.HasSuffix(lower, ".yml") ||
		strings.HasSuffix(lower, ".txt") ||
		strings.HasSuffix(lower, ".mrs") ||
		strings.HasSuffix(lower, ".list")
}

func validateRuleSetURL(loc, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.ContainsAny(trimmed, " \t\r\n") {
		return "", mihomoError(loc, "RULE-SET", fmt.Sprintf("invalid RULE-SET URL %q", raw))
	}
	u, err := url.Parse(trimmed)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Opaque != "" || u.User != nil {
		return "", mihomoError(loc, "RULE-SET", fmt.Sprintf("invalid RULE-SET URL %q: explicit https:// URL is required", raw))
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if strings.TrimSpace(u.Host) == "" || host == "" ||
		host == "invalid" || strings.HasSuffix(host, ".invalid") ||
		strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return "", mihomoError(loc, "RULE-SET", fmt.Sprintf("invalid RULE-SET URL %q: valid non-.invalid host is required", raw))
	}
	if portStr := u.Port(); portStr != "" {
		port, convErr := strconv.Atoi(portStr)
		if convErr != nil || port < 1 || port > 65535 {
			return "", mihomoError(loc, "RULE-SET", fmt.Sprintf("invalid RULE-SET URL %q: invalid port", raw))
		}
	}
	cleanPath := strings.TrimSpace(u.Path)
	if cleanPath == "" || cleanPath == "/" || !strings.HasPrefix(cleanPath, "/") || strings.HasSuffix(cleanPath, "/") {
		return "", mihomoError(loc, "RULE-SET", fmt.Sprintf("invalid RULE-SET URL %q: explicit resource path is required", raw))
	}
	for _, seg := range strings.Split(cleanPath[1:], "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", mihomoError(loc, "RULE-SET", fmt.Sprintf("invalid RULE-SET URL %q: relative or empty path segment is forbidden", raw))
		}
	}
	u.Scheme = "https"
	u.Host = strings.ToLower(strings.TrimSpace(u.Host))
	return u.String(), nil
}

func isValidRuleSetIdentifier(s string) bool {
	if s == "" || strings.Contains(s, "..") || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	lower := strings.ToLower(s)
	if lower == "invalid" || strings.HasSuffix(lower, ".invalid") {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func sanitizeRuleSetProviderName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil && u.Path != "" {
		base := strings.Trim(u.Path, "/")
		if idx := strings.LastIndexByte(base, '/'); idx >= 0 {
			base = base[idx+1:]
		}
		for _, ext := range []string{".yaml", ".yml", ".txt", ".mrs", ".list"} {
			if strings.HasSuffix(strings.ToLower(base), ext) {
				base = base[:len(base)-len(ext)]
				break
			}
		}
		var b strings.Builder
		for _, r := range base {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				b.WriteRune(r)
			} else {
				b.WriteByte('_')
			}
		}
		if cleaned := strings.Trim(b.String(), "_-"); cleaned != "" {
			return cleaned
		}
	}
	return "ruleset"
}
