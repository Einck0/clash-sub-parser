package singbox

import (
	"fmt"
	"net/netip"
	"strings"

	"clash-sub-parser/internal/domain"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// BuildOptions validates a NodeConfig and generates in-memory sing-box Options.
func BuildOptions(config NodeConfig) (option.Options, string, error) {
	if config.Server == "" {
		return option.Options{}, "", ErrMissingServer
	}
	if config.Port < 1 || config.Port > 65535 {
		return option.Options{}, "", ErrInvalidPort
	}
	if config.SkipCertVerify || hasInsecureOption(config.Transport) {
		return option.Options{}, "", domain.NewValidationError(
			"insecure_tls",
			"certificate verification bypass (skip-cert-verify/insecure) is prohibited",
		)
	}
	if config.Protocol == domain.ProtocolHysteria2 && (strings.TrimSpace(config.Hy2Ports) != "" || extractHy2Ports(config.Transport) != "") {
		return option.Options{}, "", domain.NewValidationError(
			"unsupported_hy2_ports",
			"hysteria2 port hopping is prohibited",
		)
	}
	if config.Protocol == domain.ProtocolTUIC && (config.TUICDisableSNI || hasTUICDisableSNI(config.Transport)) {
		return option.Options{}, "", domain.NewValidationError(
			"unsupported_tuic_disable_sni",
			"tuic disable_sni is prohibited",
		)
	}

	tag := config.LogicalID
	if tag == "" {
		tag = "node-probe"
	}

	options := option.Options{
		Log: &option.LogOptions{
			Disabled: true,
		},
		Route: &option.RouteOptions{
			Final: tag,
		},
	}

	switch config.Protocol {
	case domain.ProtocolSS:
		out, err := buildShadowsocksOutbound(config, tag)
		if err != nil {
			return option.Options{}, "", err
		}
		options.Outbounds = []option.Outbound{out}

	case domain.ProtocolVMess:
		out, err := buildVMessOutbound(config, tag)
		if err != nil {
			return option.Options{}, "", err
		}
		options.Outbounds = []option.Outbound{out}

	case domain.ProtocolVLESS:
		out, err := buildVLESSOutbound(config, tag)
		if err != nil {
			return option.Options{}, "", err
		}
		options.Outbounds = []option.Outbound{out}

	case domain.ProtocolTrojan:
		out, err := buildTrojanOutbound(config, tag)
		if err != nil {
			return option.Options{}, "", err
		}
		options.Outbounds = []option.Outbound{out}

	case domain.ProtocolHysteria2:
		out, err := buildHysteria2Outbound(config, tag)
		if err != nil {
			return option.Options{}, "", err
		}
		options.Outbounds = []option.Outbound{out}

	case domain.ProtocolWireGuard:
		ep, err := buildWireGuardEndpoint(config, tag)
		if err != nil {
			return option.Options{}, "", err
		}
		options.Endpoints = []option.Endpoint{ep}

	case domain.ProtocolTUIC:
		out, err := buildTUICOutbound(config, tag)
		if err != nil {
			return option.Options{}, "", err
		}
		options.Outbounds = []option.Outbound{out}

	case domain.Protocol("socks"), domain.Protocol("socks5"):
		out := buildSOCKSOutbound(config, tag)
		options.Outbounds = []option.Outbound{out}

	case domain.Protocol("http"):
		out := buildHTTPOutbound(config, tag)
		options.Outbounds = []option.Outbound{out}

	default:
		return option.Options{}, "", fmt.Errorf("%w: %s", ErrUnsupportedProto, config.Protocol)
	}

	return options, tag, nil
}

func serverOptions(config NodeConfig) option.ServerOptions {
	return option.ServerOptions{
		Server:     config.Server,
		ServerPort: uint16(config.Port),
	}
}

func networkName(config NodeConfig) string {
	if config.Network != "" {
		return strings.ToLower(config.Network)
	}
	if config.Transport != nil && config.Transport["network"] != "" {
		return strings.ToLower(config.Transport["network"])
	}
	return "tcp"
}

func buildShadowsocksOutbound(config NodeConfig, tag string) (option.Outbound, error) {
	method := config.Method
	if method == "" {
		method = "aes-256-gcm"
	}
	return option.Outbound{
		Type: C.TypeShadowsocks,
		Tag:  tag,
		Options: &option.ShadowsocksOutboundOptions{
			ServerOptions: serverOptions(config),
			Method:        method,
			Password:      config.Password,
			Network:       option.NetworkList(networkName(config)),
		},
	}, nil
}

func buildVMessOutbound(config NodeConfig, tag string) (option.Outbound, error) {
	security := config.Method
	if security == "" {
		security = "auto"
	}
	out := &option.VMessOutboundOptions{
		ServerOptions: serverOptions(config),
		UUID:          config.UUID,
		Security:      security,
		AlterId:       config.AlterID,
		Network:       option.NetworkList(networkName(config)),
	}

	if config.TLS || (config.Transport != nil && config.Transport["tls"] == "true") {
		out.TLS = &option.OutboundTLSOptions{
			Enabled:    true,
			ServerName: config.SNI,
			Insecure:   config.SkipCertVerify,
		}
		if len(config.ALPN) > 0 {
			out.TLS.ALPN = badoption.Listable[string](config.ALPN)
		}
	}

	transport, err := buildV2RayTransport(config, out.TLS != nil && out.TLS.Enabled)
	if err != nil {
		return option.Outbound{}, err
	}
	out.Transport = transport
	return option.Outbound{
		Type:    C.TypeVMess,
		Tag:     tag,
		Options: out,
	}, nil
}

func buildVLESSOutbound(config NodeConfig, tag string) (option.Outbound, error) {
	out := &option.VLESSOutboundOptions{
		ServerOptions: serverOptions(config),
		UUID:          config.UUID,
		Network:       option.NetworkList(networkName(config)),
	}

	if config.TLS || config.RealityPublicKey != "" || (config.Transport != nil && config.Transport["tls"] == "true") {
		tlsOpt := &option.OutboundTLSOptions{
			Enabled:    true,
			ServerName: config.SNI,
			Insecure:   config.SkipCertVerify,
		}
		if len(config.ALPN) > 0 {
			tlsOpt.ALPN = badoption.Listable[string](config.ALPN)
		}
		if config.ClientFingerprint != "" {
			tlsOpt.UTLS = &option.OutboundUTLSOptions{
				Enabled:     true,
				Fingerprint: config.ClientFingerprint,
			}
		}
		if config.RealityPublicKey != "" {
			tlsOpt.Reality = &option.OutboundRealityOptions{
				Enabled:   true,
				PublicKey: config.RealityPublicKey,
				ShortID:   config.RealityShortID,
			}
			if tlsOpt.UTLS == nil {
				tlsOpt.UTLS = &option.OutboundUTLSOptions{
					Enabled:     true,
					Fingerprint: "chrome",
				}
			}
		}
		out.TLS = tlsOpt
	}

	transport, err := buildV2RayTransport(config, out.TLS != nil && out.TLS.Enabled)
	if err != nil {
		return option.Outbound{}, err
	}
	out.Transport = transport
	return option.Outbound{
		Type:    C.TypeVLESS,
		Tag:     tag,
		Options: out,
	}, nil
}

func buildTrojanOutbound(config NodeConfig, tag string) (option.Outbound, error) {
	tlsOpt := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: config.SNI,
		Insecure:   config.SkipCertVerify,
	}
	if len(config.ALPN) > 0 {
		tlsOpt.ALPN = badoption.Listable[string](config.ALPN)
	}

	transport, err := buildV2RayTransport(config, true)
	if err != nil {
		return option.Outbound{}, err
	}
	out := &option.TrojanOutboundOptions{
		ServerOptions: serverOptions(config),
		Password:      config.Password,
		Network:       option.NetworkList(networkName(config)),
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
			TLS: tlsOpt,
		},
		Transport: transport,
	}

	return option.Outbound{
		Type:    C.TypeTrojan,
		Tag:     tag,
		Options: out,
	}, nil
}

func buildHysteria2Outbound(config NodeConfig, tag string) (option.Outbound, error) {
	tlsOpt := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: config.SNI,
		Insecure:   config.SkipCertVerify,
	}
	if len(config.ALPN) > 0 {
		tlsOpt.ALPN = badoption.Listable[string](config.ALPN)
	}

	out := &option.Hysteria2OutboundOptions{
		ServerOptions: serverOptions(config),
		Password:      config.Password,
		Network:       option.NetworkList(networkName(config)),
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
			TLS: tlsOpt,
		},
	}

	if config.Hy2Ports != "" {
		out.ServerPorts = badoption.Listable[string]{config.Hy2Ports}
	}
	if config.Hy2Obfs != "" {
		out.Obfs = &option.Hysteria2Obfs{
			Type:     config.Hy2Obfs,
			Password: config.Hy2ObfsPassword,
		}
	}

	return option.Outbound{
		Type:    C.TypeHysteria2,
		Tag:     tag,
		Options: out,
	}, nil
}

func buildTUICOutbound(config NodeConfig, tag string) (option.Outbound, error) {
	tlsOpt := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: config.SNI,
		Insecure:   config.SkipCertVerify,
		DisableSNI: config.TUICDisableSNI,
	}
	if len(config.ALPN) > 0 {
		tlsOpt.ALPN = badoption.Listable[string](config.ALPN)
	}

	out := &option.TUICOutboundOptions{
		ServerOptions:     serverOptions(config),
		UUID:              config.UUID,
		Password:          config.Password,
		CongestionControl: config.TUICCongestionControl,
		UDPRelayMode:      config.TUICUDPRelayMode,
		Network:           option.NetworkList(networkName(config)),
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
			TLS: tlsOpt,
		},
	}

	return option.Outbound{
		Type:    C.TypeTUIC,
		Tag:     tag,
		Options: out,
	}, nil
}

func buildWireGuardEndpoint(config NodeConfig, tag string) (option.Endpoint, error) {
	if config.PrivateKey == "" {
		return option.Endpoint{}, domain.NewValidationError("missing_private_key", "wireguard private key is required")
	}

	var addrList []netip.Prefix
	for _, raw := range config.LocalAddress {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			if strings.Contains(raw, ":") {
				raw += "/128"
			} else {
				raw += "/32"
			}
		}
		if prefix, err := netip.ParsePrefix(raw); err == nil {
			addrList = append(addrList, prefix)
		}
	}
	if len(addrList) == 0 {
		addrList = []netip.Prefix{netip.MustParsePrefix("10.0.0.2/32")}
	}

	mtu := config.MTU
	if mtu == 0 {
		mtu = 1420
	}

	peer := option.WireGuardPeer{
		Address:      config.Server,
		Port:         uint16(config.Port),
		PublicKey:    config.PublicKey,
		PreSharedKey: config.PresharedKey,
		AllowedIPs:   []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
		Reserved:     config.Reserved,
	}

	return option.Endpoint{
		Type: C.TypeWireGuard,
		Tag:  tag,
		Options: &option.WireGuardEndpointOptions{
			PrivateKey: config.PrivateKey,
			Address:    badoption.Listable[netip.Prefix](addrList),
			MTU:        mtu,
			Peers:      []option.WireGuardPeer{peer},
		},
	}, nil
}

func buildSOCKSOutbound(config NodeConfig, tag string) option.Outbound {
	return option.Outbound{
		Type: C.TypeSOCKS,
		Tag:  tag,
		Options: &option.SOCKSOutboundOptions{
			ServerOptions: serverOptions(config),
			Version:       "5",
			Username:      config.Username,
			Password:      config.Password,
			Network:       option.NetworkList(networkName(config)),
		},
	}
}

func buildHTTPOutbound(config NodeConfig, tag string) option.Outbound {
	return option.Outbound{
		Type: C.TypeHTTP,
		Tag:  tag,
		Options: &option.HTTPOutboundOptions{
			ServerOptions: serverOptions(config),
			Username:      config.Username,
			Password:      config.Password,
		},
	}
}

func buildV2RayTransport(config NodeConfig, tlsEnabled bool) (*option.V2RayTransportOptions, error) {
	net := networkName(config)
	var headers badoption.HTTPHeader
	if len(config.Headers) > 0 {
		headers = make(badoption.HTTPHeader, len(config.Headers))
		for k, v := range config.Headers {
			headers[k] = badoption.Listable[string]{v}
		}
	}
	path := config.Path
	if path == "" && config.Transport != nil {
		path = config.Transport["path"]
	}
	host := ""
	if config.Headers != nil {
		host = config.Headers["Host"]
	}
	if host == "" && config.Transport != nil {
		host = config.Transport["host"]
	}

	switch net {
	case "http":
		var hostList badoption.Listable[string]
		if host != "" {
			hostList = badoption.Listable[string]{host}
		}
		return &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeHTTP,
			HTTPOptions: option.V2RayHTTPOptions{
				Host:    hostList,
				Path:    path,
				Headers: headers,
			},
		}, nil
	case "httpupgrade":
		return &option.V2RayTransportOptions{
			Type:               C.V2RayTransportTypeHTTPUpgrade,
			HTTPUpgradeOptions: option.V2RayHTTPUpgradeOptions{Host: host, Path: path, Headers: headers},
		}, nil
	case "ws":
		return &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeWebsocket,
			WebsocketOptions: option.V2RayWebsocketOptions{
				Path:    path,
				Headers: headers,
			},
		}, nil
	case "grpc":
		if host != "" {
			if !tlsEnabled || config.SNI == "" || !strings.EqualFold(host, config.SNI) {
				return nil, domain.NewValidationError(
					"unsupported_grpc_host",
					"grpc transport cannot override Host/authority independently of TLS SNI in sing-box",
				)
			}
		}
		serviceName := config.ServiceName
		if serviceName == "" && config.Transport != nil {
			serviceName = config.Transport["service_name"]
		}
		return &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeGRPC,
			GRPCOptions: option.V2RayGRPCOptions{
				ServiceName: serviceName,
			},
		}, nil
	default:
		return nil, nil
	}
}
