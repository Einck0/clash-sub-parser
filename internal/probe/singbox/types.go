// Package singbox provides an isolated in-memory sing-box runtime adapter for probe tasks.
// It ensures that all network traffic for a probed node is strictly routed through the
// node's outbound without leaking or inheriting host environment proxies.
package singbox

import (
	"context"
	"net"
	"net/http"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/parser"
	"github.com/sagernet/sing-box/adapter"
)

var (
	// ErrNilNode is returned when a nil NodeConfig is passed.
	ErrNilNode = domain.NewValidationError("nil_node_config", "node config is nil")

	// ErrInvalidPort is returned when the port is outside 1-65535.
	ErrInvalidPort = domain.NewValidationError("invalid_port", "node port must be between 1 and 65535")

	// ErrMissingServer is returned when the server host is empty.
	ErrMissingServer = domain.NewValidationError("missing_server", "node server address is required")

	// ErrUnsupportedProto is returned when a protocol is not supported by sing-box adapter.
	ErrUnsupportedProto = domain.NewValidationError("unsupported_protocol", "unsupported protocol for sing-box runtime")

	// ErrMissingOutbound is returned when an outbound was not found in the sing-box instance.
	ErrMissingOutbound = domain.NewInternalError("missing_outbound", "sing-box outbound was not found in runtime")

	// ErrRuntimeClosed is returned when operations are attempted on a closed runtime.
	ErrRuntimeClosed = domain.NewConflictError("runtime_closed", "sing-box runtime is closed")
)

// NodeConfig represents the runtime-only configuration for a probed node.
// Plaintext secrets are accepted only in-memory to build ephemeral outbounds and are never persisted.
type NodeConfig struct {
	LogicalID   string          `json:"logical_id"`
	DisplayName string          `json:"display_name"`
	Protocol    domain.Protocol `json:"protocol"`
	Server      string          `json:"server"`
	Port        int             `json:"port"`

	// Credentials
	Method       string `json:"method,omitempty"` // cipher method for SS / security for VMess
	Password     string `json:"password,omitempty"`
	UUID         string `json:"uuid,omitempty"`
	AlterID      int    `json:"alter_id,omitempty"`
	PrivateKey   string `json:"private_key,omitempty"`
	PublicKey    string `json:"public_key,omitempty"`
	PresharedKey string `json:"preshared_key,omitempty"`

	// Transport and TLS
	Network           string            `json:"network,omitempty"` // "tcp", "ws", "grpc", "quic", "wireguard"
	TLS               bool              `json:"tls,omitempty"`
	SNI               string            `json:"sni,omitempty"`
	ALPN              []string          `json:"alpn,omitempty"`
	SkipCertVerify    bool              `json:"skip_cert_verify,omitempty"`
	ClientFingerprint string            `json:"client_fingerprint,omitempty"`
	Transport         map[string]string `json:"transport,omitempty"`

	// WebSocket options
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`

	// gRPC options
	ServiceName string `json:"service_name,omitempty"`

	// Reality options
	RealityPublicKey string `json:"reality_public_key,omitempty"`
	RealityShortID   string `json:"reality_short_id,omitempty"`

	// Hysteria2 options
	Hy2Ports        string `json:"hy2_ports,omitempty"`
	Hy2Obfs         string `json:"hy2_obfs,omitempty"`
	Hy2ObfsPassword string `json:"hy2_obfs_password,omitempty"`

	// TUIC options
	TUICCongestionControl string `json:"tuic_congestion_control,omitempty"`
	TUICUDPRelayMode      string `json:"tuic_udp_relay_mode,omitempty"`
	TUICDisableSNI        bool   `json:"tuic_disable_sni,omitempty"`

	// WireGuard options
	LocalAddress []string `json:"local_address,omitempty"`
	Reserved     []uint8  `json:"reserved,omitempty"`
	MTU          uint32   `json:"mtu,omitempty"`

	// SOCKS5 / HTTP options
	Username string `json:"username,omitempty"`
}

// HTTPClientOptions configures timeouts and behavior for an HTTP client bound to a sing-box runtime.
type HTTPClientOptions struct {
	// Resolver overrides destination DNS resolution before routing the approved IP via the outbound.
	Resolver            fetch.Resolver
	Timeout             time.Duration
	TLSHandshakeTimeout time.Duration
	IdleConnTimeout     time.Duration
	ForceAttemptHTTP2   bool
}

// Adapter defines the public contract for an isolated in-memory sing-box runtime.
type Adapter interface {
	LogicalID() string
	Protocol() domain.Protocol
	Tag() string
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	ListenPacket(ctx context.Context, destination string) (net.PacketConn, error)
	HTTPClient(options HTTPClientOptions) *http.Client
	Close() error
	IsClosed() bool
	Outbound() adapter.Outbound
}

// NodeConfigFromNormalized builds a NodeConfig from a parsed NormalizedNode and credentials.
func NodeConfigFromNormalized(norm parser.NormalizedNode, secrets ...string) NodeConfig {
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
		cfg.TLS = norm.Transport["tls"] == "true"
		cfg.SNI = norm.Transport["sni"]
		cfg.Path = norm.Transport["path"]
		if host, ok := norm.Transport["host"]; ok {
			cfg.Headers = map[string]string{"Host": host}
		}
		cfg.ServiceName = norm.Transport["service_name"]
		if alpnStr, ok := norm.Transport["alpn"]; ok && alpnStr != "" {
			cfg.ALPN = []string{alpnStr}
		}
	}

	if len(secrets) > 0 {
		switch norm.Node.Protocol {
		case domain.ProtocolSS:
			cfg.Password = secrets[0]
			if len(secrets) > 1 {
				cfg.Method = secrets[1]
			}
		case domain.ProtocolVMess, domain.ProtocolVLESS:
			cfg.UUID = secrets[0]
		case domain.ProtocolTrojan, domain.ProtocolHysteria2:
			cfg.Password = secrets[0]
		case domain.ProtocolTUIC:
			cfg.UUID = secrets[0]
			if len(secrets) > 1 {
				cfg.Password = secrets[1]
			}
		case domain.ProtocolWireGuard:
			cfg.PrivateKey = secrets[0]
			if len(secrets) > 1 {
				cfg.PublicKey = secrets[1]
			}
		}
	}

	return cfg
}
