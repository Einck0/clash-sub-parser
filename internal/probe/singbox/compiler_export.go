package singbox

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
	box "github.com/sagernet/sing-box"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/tuic"
	"github.com/sagernet/sing-box/protocol/wireguard"
)

// ExportOptionContext returns a context populated with official sing-box option registries
// for all seven supported protocols (including WireGuard endpoint, Hysteria2, and TUIC).
func ExportOptionContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	initRegistries()
	outReg := include.OutboundRegistry()
	hysteria2.RegisterOutbound(outReg)
	tuic.RegisterOutbound(outReg)
	epReg := include.EndpointRegistry()
	wireguard.RegisterEndpoint(epReg)
	return box.Context(ctx, inboundReg, outReg, epReg, dnsReg, serviceReg, certReg)
}

var validShadowsocksMethods = map[string]bool{
	"none":                          true,
	"aes-128-gcm":                   true,
	"aes-192-gcm":                   true,
	"aes-256-gcm":                   true,
	"chacha20-ietf-poly1305":        true,
	"xchacha20-ietf-poly1305":       true,
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
	"aes-128-ctr":                   true,
	"aes-192-ctr":                   true,
	"aes-256-ctr":                   true,
	"aes-128-cfb":                   true,
	"aes-192-cfb":                   true,
	"aes-256-cfb":                   true,
	"rc4-md5":                       true,
	"chacha20-ietf":                 true,
	"xchacha20":                     true,
}

var validVMessSecurities = map[string]bool{
	"":                  true,
	"auto":              true,
	"none":              true,
	"zero":              true,
	"aes-128-cfb":       true,
	"aes-128-gcm":       true,
	"chacha20-poly1305": true,
}

// BuildExportNodeOption constructs an official sing-box option.Outbound or option.Endpoint
// from a plaintext domain.Node for configuration export.
func BuildExportNodeOption(tag string, node domain.Node) (*option.Outbound, *option.Endpoint, error) {
	server := strings.TrimSpace(node.Server)
	if server == "" {
		return nil, nil, ErrMissingServer
	}
	if node.Port < 1 || node.Port > 65535 {
		return nil, nil, ErrInvalidPort
	}

	node.DisplayName = tag
	node.Server = server
	cfg := NodeConfigFromNode(node)
	m := node.Credentials.Transport

	switch node.Protocol {
	case domain.ProtocolSS:
		method := strings.ToLower(strings.TrimSpace(cfg.Method))
		if !validShadowsocksMethods[method] {
			return nil, nil, domain.NewValidationError(
				"unsupported_ss_cipher",
				fmt.Sprintf("unsupported shadowsocks cipher %q in sing-box", cfg.Method),
			)
		}
		cfg.Method = method
		if err := validateV2RayNetworkName(cfg.Protocol, networkName(cfg), false); err != nil {
			return nil, nil, err
		}
		out, err := buildShadowsocksOutbound(cfg, tag)
		if err != nil {
			return nil, nil, err
		}
		if opts, ok := out.Options.(*option.ShadowsocksOutboundOptions); ok {
			opts.Network = ""
		}
		return &out, nil, nil

	case domain.ProtocolVMess:
		sec := strings.ToLower(strings.TrimSpace(cfg.Method))
		if !validVMessSecurities[sec] {
			return nil, nil, domain.NewValidationError(
				"unsupported_vmess_security",
				fmt.Sprintf("unsupported vmess security %q in sing-box", cfg.Method),
			)
		}
		cfg.Method = sec
		if err := validateV2RayNetworkName(cfg.Protocol, networkName(cfg), true); err != nil {
			return nil, nil, err
		}
		out, err := buildVMessOutbound(cfg, tag)
		if err != nil {
			return nil, nil, err
		}
		if opts, ok := out.Options.(*option.VMessOutboundOptions); ok {
			opts.Network = ""
			if cfg.ClientFingerprint != "" {
				if opts.TLS == nil {
					return nil, nil, domain.NewValidationError(
						"utls_without_tls",
						"client fingerprint requires TLS in vmess outbound",
					)
				}
				opts.TLS.UTLS = &option.OutboundUTLSOptions{
					Enabled:     true,
					Fingerprint: cfg.ClientFingerprint,
				}
			}
		}
		return &out, nil, nil

	case domain.ProtocolVLESS:
		if err := validateV2RayNetworkName(cfg.Protocol, networkName(cfg), true); err != nil {
			return nil, nil, err
		}
		flow := ""
		if m != nil {
			flow = strings.TrimSpace(m["flow"])
		}
		if flow != "" && flow != "xtls-rprx-vision" {
			return nil, nil, domain.NewValidationError(
				"unsupported_vless_flow",
				fmt.Sprintf("unsupported vless flow %q in sing-box", flow),
			)
		}
		if flow != "" {
			cfg.TLS = true
		}
		out, err := buildVLESSOutbound(cfg, tag)
		if err != nil {
			return nil, nil, err
		}
		if opts, ok := out.Options.(*option.VLESSOutboundOptions); ok {
			opts.Network = ""
			opts.Flow = flow
		}
		return &out, nil, nil

	case domain.ProtocolTrojan:
		if err := validateV2RayNetworkName(cfg.Protocol, networkName(cfg), true); err != nil {
			return nil, nil, err
		}
		out, err := buildTrojanOutbound(cfg, tag)
		if err != nil {
			return nil, nil, err
		}
		if opts, ok := out.Options.(*option.TrojanOutboundOptions); ok {
			opts.Network = ""
			if cfg.ClientFingerprint != "" && opts.TLS != nil {
				opts.TLS.UTLS = &option.OutboundUTLSOptions{
					Enabled:     true,
					Fingerprint: cfg.ClientFingerprint,
				}
			}
		}
		return &out, nil, nil

	case domain.ProtocolHysteria2:
		if err := validateQuicTransportNetwork(cfg.Protocol, networkName(cfg)); err != nil {
			return nil, nil, err
		}
		if cfg.Hy2Obfs != "" && cfg.Hy2Obfs != C.Hysteria2ObfsTypeSalamander {
			return nil, nil, domain.NewValidationError(
				"unsupported_hy2_obfs",
				fmt.Sprintf("unsupported hysteria2 obfs type %q in sing-box", cfg.Hy2Obfs),
			)
		}
		if cfg.Hy2Obfs != "" && cfg.Hy2ObfsPassword == "" {
			return nil, nil, domain.NewValidationError(
				"missing_hy2_obfs_password",
				"missing required obfs-password for hysteria2 obfs",
			)
		}
		if cfg.Hy2Obfs == "" && cfg.Hy2ObfsPassword != "" {
			return nil, nil, domain.NewValidationError(
				"missing_hy2_obfs_type",
				"missing required obfs type for hysteria2 obfs-password",
			)
		}
		upMbps, err := parseBandwidthMbps(m, "up")
		if err != nil {
			return nil, nil, err
		}
		downMbps, err := parseBandwidthMbps(m, "down")
		if err != nil {
			return nil, nil, err
		}
		out, err := buildHysteria2Outbound(cfg, tag)
		if err != nil {
			return nil, nil, err
		}
		if opts, ok := out.Options.(*option.Hysteria2OutboundOptions); ok {
			opts.Network = ""
			opts.UpMbps = upMbps
			opts.DownMbps = downMbps
		}
		return &out, nil, nil

	case domain.ProtocolTUIC:
		if err := validateQuicTransportNetwork(cfg.Protocol, networkName(cfg)); err != nil {
			return nil, nil, err
		}
		cc := strings.ToLower(strings.TrimSpace(cfg.TUICCongestionControl))
		switch cc {
		case "", "cubic", "new_reno", "bbr":
			cfg.TUICCongestionControl = cc
		default:
			return nil, nil, domain.NewValidationError(
				"unsupported_tuic_congestion_control",
				fmt.Sprintf("unsupported tuic congestion_control %q in sing-box", cfg.TUICCongestionControl),
			)
		}
		mode := strings.ToLower(strings.TrimSpace(cfg.TUICUDPRelayMode))
		switch mode {
		case "", "native", "quic":
			cfg.TUICUDPRelayMode = mode
		default:
			return nil, nil, domain.NewValidationError(
				"unsupported_tuic_udp_relay_mode",
				fmt.Sprintf("unsupported tuic udp_relay_mode %q in sing-box", cfg.TUICUDPRelayMode),
			)
		}
		out, err := buildTUICOutbound(cfg, tag)
		if err != nil {
			return nil, nil, err
		}
		if opts, ok := out.Options.(*option.TUICOutboundOptions); ok {
			opts.Network = ""
		}
		return &out, nil, nil

	case domain.ProtocolWireGuard:
		ep, err := buildWireGuardEndpoint(cfg, tag)
		if err != nil {
			return nil, nil, err
		}
		if opts, ok := ep.Options.(*option.WireGuardEndpointOptions); ok {
			hasIPv6 := false
			for _, prefix := range opts.Address {
				if prefix.Addr().Is6() {
					hasIPv6 = true
					break
				}
			}
			if hasIPv6 && len(opts.Peers) > 0 {
				opts.Peers[0].AllowedIPs = []netip.Prefix{
					netip.MustParsePrefix("0.0.0.0/0"),
					netip.MustParsePrefix("::/0"),
				}
			}
		}
		return nil, &ep, nil

	default:
		return nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedProto, node.Protocol)
	}
}

func validateV2RayNetworkName(proto domain.Protocol, netName string, allowV2RayTransports bool) error {
	switch netName {
	case "", "tcp", "udp":
		return nil
	case "ws", "grpc", "http", "httpupgrade":
		if allowV2RayTransports {
			return nil
		}
	}
	return domain.NewValidationError(
		"unsupported_transport_network",
		fmt.Sprintf("unsupported network %q in %s transport", netName, proto),
	)
}

func validateQuicTransportNetwork(proto domain.Protocol, netName string) error {
	switch netName {
	case "", "quic", "udp", "tcp":
		return nil
	default:
		return domain.NewValidationError(
			"unsupported_transport_network",
			fmt.Sprintf("unsupported network %q in %s transport", netName, proto),
		)
	}
}

func parseBandwidthMbps(m map[string]string, key string) (int, error) {
	if m == nil {
		return 0, nil
	}
	raw := strings.TrimSpace(m[key])
	if raw == "" {
		return 0, nil
	}
	normalized := strings.ToLower(raw)
	for _, suffix := range []string{"mbps", "m"} {
		if strings.HasSuffix(normalized, suffix) {
			normalized = strings.TrimSpace(strings.TrimSuffix(normalized, suffix))
			break
		}
	}
	val, err := strconv.Atoi(normalized)
	if err != nil || val <= 0 {
		return 0, domain.NewValidationError(
			"invalid_hy2_bandwidth",
			fmt.Sprintf("invalid hysteria2 %s bandwidth %q", key, raw),
		)
	}
	return val, nil
}
