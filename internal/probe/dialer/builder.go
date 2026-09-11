package dialer

import (
	"fmt"
	"net/netip"
	"strings"

	"clash-sub-parser/internal/domain"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// NodeTag returns a stable unique identifier tag for sing-box outbound routing.
func NodeTag(node *domain.Node) string {
	if node == nil {
		return "proxy"
	}
	if node.LogicalID != "" {
		return node.LogicalID
	}
	if node.Name != "" {
		return node.Name
	}
	return fmt.Sprintf("node-%d", node.ID)
}

// BuildOptions constructs sing-box options for in-memory execution with no listening ports.
func BuildOptions(node *domain.Node) (option.Options, string, error) {
	if node == nil {
		return option.Options{}, "", ErrNilNode
	}

	tag := NodeTag(node)
	proto := node.CanonicalProtocol()

	opts := option.Options{
		Log: &option.LogOptions{
			Disabled: true,
		},
		Route: &option.RouteOptions{
			Final: tag,
		},
	}

	if proto == domain.ProtocolWireGuard {
		ep, err := BuildEndpoint(node)
		if err != nil {
			return option.Options{}, "", err
		}
		opts.Endpoints = []option.Endpoint{ep}
		return opts, tag, nil
	}

	out, err := BuildOutbound(node)
	if err != nil {
		return option.Options{}, "", err
	}
	opts.Outbounds = []option.Outbound{out}
	return opts, tag, nil
}

// BuildOutbound converts a domain.Node proxy configuration into a sing-box option.Outbound.
func BuildOutbound(node *domain.Node) (option.Outbound, error) {
	if node == nil {
		return option.Outbound{}, ErrNilNode
	}

	tag := NodeTag(node)
	proto := node.CanonicalProtocol()

	serverOpts := option.ServerOptions{
		Server:     node.Server,
		ServerPort: uint16(node.Port),
	}

	switch proto {
	case domain.ProtocolShadowsocks:
		ssOpts, err := node.GetShadowsocksOptions()
		if err != nil {
			return option.Outbound{}, fmt.Errorf("failed to parse shadowsocks options: %w", err)
		}
		return option.Outbound{
			Type: C.TypeShadowsocks,
			Tag:  tag,
			Options: &option.ShadowsocksOutboundOptions{
				ServerOptions: serverOpts,
				Method:        ssOpts.Cipher,
				Password:      ssOpts.Password,
				Plugin:        ssOpts.Plugin,
			},
		}, nil

	case domain.ProtocolVMess:
		vmOpts, err := node.GetVMessOptions()
		if err != nil {
			return option.Outbound{}, fmt.Errorf("failed to parse vmess options: %w", err)
		}
		sec := vmOpts.Cipher
		if sec == "" {
			sec = "auto"
		}
		outOpts := &option.VMessOutboundOptions{
			ServerOptions: serverOpts,
			UUID:          vmOpts.UUID,
			Security:      sec,
			AlterId:       vmOpts.AlterID,
		}
		if vmOpts.TLS {
			outOpts.TLS = &option.OutboundTLSOptions{
				Enabled:    true,
				ServerName: vmOpts.SNI,
				Insecure:   vmOpts.SkipCertVerify,
			}
		}
		if vmOpts.Network == "ws" && vmOpts.WSOptions != nil {
			outOpts.Transport = &option.V2RayTransportOptions{
				Type: C.V2RayTransportTypeWebsocket,
				WebsocketOptions: option.V2RayWebsocketOptions{
					Path:    vmOpts.WSOptions.Path,
					Headers: convertHTTPHeaders(vmOpts.WSOptions.Headers),
				},
			}
		} else if vmOpts.Network == "grpc" && vmOpts.GRPCOptions != nil {
			outOpts.Transport = &option.V2RayTransportOptions{
				Type: C.V2RayTransportTypeGRPC,
				GRPCOptions: option.V2RayGRPCOptions{
					ServiceName: vmOpts.GRPCOptions.ServiceName,
				},
			}
		}
		return option.Outbound{
			Type:    C.TypeVMess,
			Tag:     tag,
			Options: outOpts,
		}, nil

	case domain.ProtocolVLESS:
		vlOpts, err := node.GetVLESSOptions()
		if err != nil {
			return option.Outbound{}, fmt.Errorf("failed to parse vless options: %w", err)
		}
		outOpts := &option.VLESSOutboundOptions{
			ServerOptions: serverOpts,
			UUID:          vlOpts.UUID,
			Flow:          vlOpts.Flow,
		}
		if vlOpts.TLS || vlOpts.Reality != nil {
			tlsOpt := &option.OutboundTLSOptions{
				Enabled:    true,
				ServerName: vlOpts.SNI,
				Insecure:   vlOpts.SkipCertVerify,
			}
			if vlOpts.ClientFingerprint != "" {
				tlsOpt.UTLS = &option.OutboundUTLSOptions{
					Enabled:     true,
					Fingerprint: vlOpts.ClientFingerprint,
				}
			}
			if vlOpts.Reality != nil {
				tlsOpt.Reality = &option.OutboundRealityOptions{
					Enabled:   true,
					PublicKey: vlOpts.Reality.PublicKey,
					ShortID:   vlOpts.Reality.ShortID,
				}
				if tlsOpt.UTLS == nil {
					tlsOpt.UTLS = &option.OutboundUTLSOptions{
						Enabled:     true,
						Fingerprint: "chrome",
					}
				}
			}
			outOpts.TLS = tlsOpt
		}
		if vlOpts.Network == "ws" && vlOpts.WSOptions != nil {
			outOpts.Transport = &option.V2RayTransportOptions{
				Type: C.V2RayTransportTypeWebsocket,
				WebsocketOptions: option.V2RayWebsocketOptions{
					Path:    vlOpts.WSOptions.Path,
					Headers: convertHTTPHeaders(vlOpts.WSOptions.Headers),
				},
			}
		} else if vlOpts.Network == "grpc" && vlOpts.GRPCOptions != nil {
			outOpts.Transport = &option.V2RayTransportOptions{
				Type: C.V2RayTransportTypeGRPC,
				GRPCOptions: option.V2RayGRPCOptions{
					ServiceName: vlOpts.GRPCOptions.ServiceName,
				},
			}
		}
		return option.Outbound{
			Type:    C.TypeVLESS,
			Tag:     tag,
			Options: outOpts,
		}, nil

	case domain.ProtocolTrojan:
		trOpts, err := node.GetTrojanOptions()
		if err != nil {
			return option.Outbound{}, fmt.Errorf("failed to parse trojan options: %w", err)
		}
		outOpts := &option.TrojanOutboundOptions{
			ServerOptions: serverOpts,
			Password:      trOpts.Password,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
				TLS: &option.OutboundTLSOptions{
					Enabled:    true,
					ServerName: trOpts.SNI,
					Insecure:   trOpts.SkipCertVerify,
					ALPN:       badoption.Listable[string](trOpts.ALPN),
				},
			},
		}
		if trOpts.Network == "ws" && trOpts.WSOptions != nil {
			outOpts.Transport = &option.V2RayTransportOptions{
				Type: C.V2RayTransportTypeWebsocket,
				WebsocketOptions: option.V2RayWebsocketOptions{
					Path:    trOpts.WSOptions.Path,
					Headers: convertHTTPHeaders(trOpts.WSOptions.Headers),
				},
			}
		} else if trOpts.Network == "grpc" && trOpts.GRPCOptions != nil {
			outOpts.Transport = &option.V2RayTransportOptions{
				Type: C.V2RayTransportTypeGRPC,
				GRPCOptions: option.V2RayGRPCOptions{
					ServiceName: trOpts.GRPCOptions.ServiceName,
				},
			}
		}
		return option.Outbound{
			Type:    C.TypeTrojan,
			Tag:     tag,
			Options: outOpts,
		}, nil

	case domain.ProtocolHysteria2:
		hyOpts, err := node.GetHysteria2Options()
		if err != nil {
			return option.Outbound{}, fmt.Errorf("failed to parse hysteria2 options: %w", err)
		}
		outOpts := &option.Hysteria2OutboundOptions{
			ServerOptions: serverOpts,
			Password:      hyOpts.Password,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
				TLS: &option.OutboundTLSOptions{
					Enabled:    true,
					ServerName: hyOpts.SNI,
					Insecure:   hyOpts.SkipCertVerify,
					ALPN:       badoption.Listable[string](hyOpts.ALPN),
				},
			},
		}
		if hyOpts.Ports != "" {
			outOpts.ServerPorts = badoption.Listable[string]{hyOpts.Ports}
		}
		if hyOpts.Obfs != "" {
			outOpts.Obfs = &option.Hysteria2Obfs{
				Type:     hyOpts.Obfs,
				Password: hyOpts.ObfsPassword,
			}
		}
		return option.Outbound{
			Type:    C.TypeHysteria2,
			Tag:     tag,
			Options: outOpts,
		}, nil

	case domain.ProtocolTUIC:
		tuOpts, err := node.GetTUICOptions()
		if err != nil {
			return option.Outbound{}, fmt.Errorf("failed to parse tuic options: %w", err)
		}
		outOpts := &option.TUICOutboundOptions{
			ServerOptions:     serverOpts,
			UUID:              tuOpts.UUID,
			Password:          tuOpts.Password,
			CongestionControl: tuOpts.CongestionController,
			UDPRelayMode:      tuOpts.UDPRelayMode,
			OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{
				TLS: &option.OutboundTLSOptions{
					Enabled:    true,
					ServerName: tuOpts.SNI,
					Insecure:   tuOpts.SkipCertVerify,
					DisableSNI: tuOpts.DisableSNI,
					ALPN:       badoption.Listable[string](tuOpts.ALPN),
				},
			},
		}
		return option.Outbound{
			Type:    C.TypeTUIC,
			Tag:     tag,
			Options: outOpts,
		}, nil

	case domain.ProtocolSOCKS5:
		return option.Outbound{
			Type: C.TypeSOCKS,
			Tag:  tag,
			Options: &option.SOCKSOutboundOptions{
				ServerOptions: serverOpts,
				Version:       "5",
			},
		}, nil

	case domain.ProtocolHTTP:
		return option.Outbound{
			Type: C.TypeHTTP,
			Tag:  tag,
			Options: &option.HTTPOutboundOptions{
				ServerOptions: serverOpts,
			},
		}, nil

	default:
		return option.Outbound{}, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, proto)
	}
}

// BuildEndpoint converts a WireGuard node into a sing-box option.Endpoint.
func BuildEndpoint(node *domain.Node) (option.Endpoint, error) {
	if node == nil {
		return option.Endpoint{}, ErrNilNode
	}
	if node.CanonicalProtocol() != domain.ProtocolWireGuard {
		return option.Endpoint{}, fmt.Errorf("protocol %s is not an endpoint protocol", node.Protocol)
	}

	wgOpts, err := node.GetWireGuardOptions()
	if err != nil {
		return option.Endpoint{}, fmt.Errorf("failed to parse wireguard options: %w", err)
	}

	tag := NodeTag(node)
	var addrList []netip.Prefix
	if wgOpts.IP != "" {
		ipStr := wgOpts.IP
		if !strings.Contains(ipStr, "/") {
			if strings.Contains(ipStr, ":") {
				ipStr += "/128"
			} else {
				ipStr += "/32"
			}
		}
		if prefix, err := netip.ParsePrefix(ipStr); err == nil {
			addrList = append(addrList, prefix)
		}
	}

	var reserved []uint8
	for _, r := range wgOpts.Reserved {
		reserved = append(reserved, uint8(r))
	}

	peer := option.WireGuardPeer{
		Address:      node.Server,
		Port:         uint16(node.Port),
		PublicKey:    wgOpts.PublicKey,
		PreSharedKey: wgOpts.PresharedKey,
		AllowedIPs:   []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
		Reserved:     reserved,
	}

	mtu := uint32(wgOpts.MTU)
	if mtu == 0 {
		mtu = 1420
	}

	return option.Endpoint{
		Type: C.TypeWireGuard,
		Tag:  tag,
		Options: &option.WireGuardEndpointOptions{
			PrivateKey: wgOpts.PrivateKey,
			Address:    addrList,
			MTU:        mtu,
			Peers:      []option.WireGuardPeer{peer},
		},
	}, nil
}

func convertHTTPHeaders(headers map[string]string) badoption.HTTPHeader {
	if len(headers) == 0 {
		return nil
	}
	res := make(badoption.HTTPHeader, len(headers))
	for k, v := range headers {
		res[k] = badoption.Listable[string]{v}
	}
	return res
}
