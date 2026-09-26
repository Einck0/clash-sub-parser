package singbox

import (
	"net"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
)

func applyTransportFields(cfg *NodeConfig, m map[string]string) {
	if m == nil {
		return
	}
	if cfg.Network == "" && m["network"] != "" {
		cfg.Network = m["network"]
	}
	if !cfg.TLS && domain.IsTruthy(m["tls"]) {
		cfg.TLS = true
	}
	if cfg.SNI == "" && m["sni"] != "" {
		cfg.SNI = m["sni"]
	}
	if cfg.Path == "" && m["path"] != "" {
		cfg.Path = m["path"]
	}
	if cfg.Headers == nil && m["host"] != "" {
		cfg.Headers = map[string]string{"Host": m["host"]}
	}
	if cfg.ServiceName == "" && m["service_name"] != "" {
		cfg.ServiceName = m["service_name"]
	}
	if len(cfg.ALPN) == 0 {
		if alpnStr := strings.TrimSpace(m["alpn"]); alpnStr != "" {
			parts := strings.Split(alpnStr, ",")
			for _, p := range parts {
				if trimmed := strings.TrimSpace(p); trimmed != "" {
					cfg.ALPN = append(cfg.ALPN, trimmed)
				}
			}
		}
	}
	if cfg.RealityPublicKey == "" {
		for _, k := range []string{"pbk", "reality_public_key"} {
			if v := strings.TrimSpace(m[k]); v != "" {
				cfg.RealityPublicKey = v
				break
			}
		}
	}
	if cfg.RealityShortID == "" {
		for _, k := range []string{"sid", "reality_short_id"} {
			if v := strings.TrimSpace(m[k]); v != "" {
				cfg.RealityShortID = v
				break
			}
		}
	}
	if cfg.ClientFingerprint == "" {
		for _, k := range []string{"fp", "client_fingerprint"} {
			if v := strings.TrimSpace(m[k]); v != "" {
				cfg.ClientFingerprint = v
				break
			}
		}
	}
	if cfg.Hy2Obfs == "" && strings.TrimSpace(m["obfs"]) != "" {
		cfg.Hy2Obfs = strings.TrimSpace(m["obfs"])
	}
	if cfg.Hy2ObfsPassword == "" {
		for _, k := range []string{"obfs-password", "obfs_password"} {
			if v := strings.TrimSpace(m[k]); v != "" {
				cfg.Hy2ObfsPassword = v
				break
			}
		}
	}
	if domain.HasInsecureTransport(m) {
		cfg.SkipCertVerify = true
	}
	if cfg.Hy2Ports == "" {
		if p := domain.ExtractHy2Ports(m); p != "" {
			cfg.Hy2Ports = p
		}
	}
	if domain.HasTUICDisableSNI(m) {
		cfg.TUICDisableSNI = true
	}
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

	applyTransportFields(&cfg, norm.Transport)

	if payload != nil {
		creds := payload.Credentials
		cfg.Password = creds.Password
		cfg.UUID = creds.UUID
		cfg.Method = creds.Method
		cfg.AlterID = creds.AlterID
		cfg.PrivateKey = creds.PrivateKey
		cfg.PublicKey = creds.PublicKey
		cfg.PresharedKey = creds.EffectivePreSharedKey()
		cfg.Username = creds.Username

		if len(creds.LocalAddress) > 0 {
			cfg.LocalAddress = append([]string(nil), creds.LocalAddress...)
		}
		if len(creds.Reserved) > 0 {
			cfg.Reserved = append([]uint8(nil), creds.Reserved...)
		}
		if creds.MTU > 0 {
			cfg.MTU = uint32(creds.MTU)
		}
		if creds.CongestionControl != "" {
			cfg.TUICCongestionControl = creds.CongestionControl
		}
		if creds.UDPRelayMode != "" {
			cfg.TUICUDPRelayMode = creds.UDPRelayMode
		}
		if creds.DisableSNI {
			cfg.TUICDisableSNI = true
		}
		if len(creds.ALPN) > 0 && len(cfg.ALPN) == 0 {
			cfg.ALPN = append([]string(nil), creds.ALPN...)
		}
		if creds.SNI != "" && cfg.SNI == "" {
			cfg.SNI = creds.SNI
		}

		applyTransportFields(&cfg, creds.Transport)
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
