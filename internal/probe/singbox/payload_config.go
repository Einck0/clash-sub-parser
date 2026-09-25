package singbox

import (
	"net"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
)

func isTruthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

func hasInsecureOption(m map[string]string) bool {
	if m == nil {
		return false
	}
	for k, v := range m {
		kLower := strings.ToLower(strings.TrimSpace(k))
		if strings.Contains(kLower, "insecure") ||
			strings.Contains(kLower, "skip_cert") ||
			strings.Contains(kLower, "skip-cert") ||
			strings.Contains(kLower, "skipcert") {
			if isTruthy(v) {
				return true
			}
		}
	}
	return false
}

func extractHy2Ports(m map[string]string) string {
	if m == nil {
		return ""
	}
	for _, k := range []string{"ports", "server_ports", "hy2_ports"} {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

func hasTUICDisableSNI(m map[string]string) bool {
	if m == nil {
		return false
	}
	for _, k := range []string{"disable_sni", "disable-sni", "tuic_disable_sni"} {
		if isTruthy(m[k]) {
			return true
		}
	}
	return false
}

// NodeConfigFromPayload builds an ephemeral NodeConfig directly from decrypted NodeCredentialPayload.
func NodeConfigFromPayload(norm parser.NormalizedNode, payload *domain.NodeCredentialPayload) NodeConfig {
	cfg := NodeConfig{
		LogicalID:   norm.Node.LogicalID,
		DisplayName: norm.Node.DisplayName,
		Protocol:    norm.Node.Protocol,
		Server:      norm.Server,
		Port:        norm.Port,
		Transport:   norm.Transport,
	}

	if norm.Transport != nil {
		cfg.Network = norm.Transport["network"]
		cfg.TLS = isTruthy(norm.Transport["tls"])
		cfg.SNI = norm.Transport["sni"]
		cfg.Path = norm.Transport["path"]
		if host, ok := norm.Transport["host"]; ok {
			cfg.Headers = map[string]string{"Host": host}
		}
		cfg.ServiceName = norm.Transport["service_name"]
		if alpnStr, ok := norm.Transport["alpn"]; ok && alpnStr != "" {
			cfg.ALPN = []string{alpnStr}
		}
		if hasInsecureOption(norm.Transport) {
			cfg.SkipCertVerify = true
		}
		if p := extractHy2Ports(norm.Transport); p != "" {
			cfg.Hy2Ports = p
		}
		if hasTUICDisableSNI(norm.Transport) {
			cfg.TUICDisableSNI = true
		}
	}

	if payload != nil {
		creds := payload.Credentials
		cfg.Password = creds.Password
		cfg.UUID = creds.UUID
		cfg.Method = creds.Method
		cfg.AlterID = creds.AlterID
		cfg.PrivateKey = creds.PrivateKey
		cfg.PublicKey = creds.PublicKey
		cfg.PresharedKey = creds.PresharedKey
		cfg.Username = creds.Username

		if creds.Transport != nil {
			if cfg.Network == "" && creds.Transport["network"] != "" {
				cfg.Network = creds.Transport["network"]
			}
			if !cfg.TLS && isTruthy(creds.Transport["tls"]) {
				cfg.TLS = true
			}
			if cfg.SNI == "" && creds.Transport["sni"] != "" {
				cfg.SNI = creds.Transport["sni"]
			}
			if cfg.Path == "" && creds.Transport["path"] != "" {
				cfg.Path = creds.Transport["path"]
			}
			if cfg.Headers == nil && creds.Transport["host"] != "" {
				cfg.Headers = map[string]string{"Host": creds.Transport["host"]}
			}
			if hasInsecureOption(creds.Transport) {
				cfg.SkipCertVerify = true
			}
			if cfg.Hy2Ports == "" {
				if p := extractHy2Ports(creds.Transport); p != "" {
					cfg.Hy2Ports = p
				}
			}
			if hasTUICDisableSNI(creds.Transport) {
				cfg.TUICDisableSNI = true
			}
		}
	}

	// When the transport does not explicitly provide TLS identity or HTTP routing,
	// preserve the original server hostname (not a later pinned socket IP).
	if cfg.Server != "" && net.ParseIP(strings.Trim(cfg.Server, "[]")) == nil {
		usesTLS := cfg.TLS || cfg.Protocol == domain.ProtocolTrojan || cfg.Protocol == domain.ProtocolHysteria2 || cfg.Protocol == domain.ProtocolTUIC
		if usesTLS && cfg.SNI == "" {
			cfg.SNI = cfg.Server
		}
		if cfg.Network == "ws" || cfg.Network == "http" || cfg.Network == "httpupgrade" || cfg.Network == "grpc" {
			if cfg.Headers == nil {
				cfg.Headers = make(map[string]string)
			}
			if _, ok := cfg.Headers["Host"]; !ok {
				if cfg.Network == "grpc" && cfg.SNI != "" {
					cfg.Headers["Host"] = cfg.SNI
				} else {
					cfg.Headers["Host"] = cfg.Server
				}
			}
		}
	}
	return cfg
}
