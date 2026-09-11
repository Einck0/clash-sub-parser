package domain

// ProtocolType represents supported proxy protocols in CSP.
type ProtocolType string

const (
	ProtocolShadowsocks ProtocolType = "ss"
	ProtocolVMess       ProtocolType = "vmess"
	ProtocolVLESS       ProtocolType = "vless"
	ProtocolTrojan      ProtocolType = "trojan"
	ProtocolHysteria2   ProtocolType = "hysteria2"
	ProtocolTUIC        ProtocolType = "tuic"
	ProtocolWireGuard   ProtocolType = "wireguard"
	ProtocolSOCKS5      ProtocolType = "socks5"
	ProtocolHTTP        ProtocolType = "http"
)

// LifecycleState represents lifecycle state of a proxy node.
type LifecycleState string

const (
	LifecycleActive     LifecycleState = "active"
	LifecycleInactive   LifecycleState = "inactive"
	LifecycleQuarantine LifecycleState = "quarantine"
	LifecycleDraining   LifecycleState = "draining"
)

// WSOptions represents WebSocket transport configuration.
type WSOptions struct {
	Path    string            `json:"path,omitempty" yaml:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// GRPCOptions represents gRPC transport configuration.
type GRPCOptions struct {
	ServiceName string `json:"service-name,omitempty" yaml:"service-name,omitempty"`
}

// RealityOptions represents VLESS Reality security options.
type RealityOptions struct {
	PublicKey string `json:"public-key" yaml:"public-key"`
	ShortID   string `json:"short-id,omitempty" yaml:"short-id,omitempty"`
	SpiderX   string `json:"spider-x,omitempty" yaml:"spider-x,omitempty"`
}

// ShadowsocksOptions contains connection parameters for Shadowsocks proxies.
type ShadowsocksOptions struct {
	Cipher     string         `json:"cipher" yaml:"cipher"`
	Password   string         `json:"password" yaml:"password"`
	Plugin     string         `json:"plugin,omitempty" yaml:"plugin,omitempty"`
	PluginOpts map[string]any `json:"plugin-opts,omitempty" yaml:"plugin-opts,omitempty"`
	UDP        bool           `json:"udp,omitempty" yaml:"udp,omitempty"`
}

// VMessOptions contains connection parameters for VMess proxies.
type VMessOptions struct {
	UUID           string       `json:"uuid" yaml:"uuid"`
	AlterID        int          `json:"alterId,omitempty" yaml:"alterId,omitempty"`
	Cipher         string       `json:"cipher,omitempty" yaml:"cipher,omitempty"`
	Network        string       `json:"network,omitempty" yaml:"network,omitempty"`
	TLS            bool         `json:"tls,omitempty" yaml:"tls,omitempty"`
	SNI            string       `json:"servername,omitempty" yaml:"servername,omitempty"`
	SkipCertVerify bool         `json:"skip-cert-verify,omitempty" yaml:"skip-cert-verify,omitempty"`
	WSOptions      *WSOptions   `json:"ws-opts,omitempty" yaml:"ws-opts,omitempty"`
	GRPCOptions    *GRPCOptions `json:"grpc-opts,omitempty" yaml:"grpc-opts,omitempty"`
	UDP            bool         `json:"udp,omitempty" yaml:"udp,omitempty"`
}

// VLESSOptions contains connection parameters for VLESS proxies (including Reality and uTLS).
type VLESSOptions struct {
	UUID              string          `json:"uuid" yaml:"uuid"`
	Flow              string          `json:"flow,omitempty" yaml:"flow,omitempty"`
	Network           string          `json:"network,omitempty" yaml:"network,omitempty"`
	TLS               bool            `json:"tls,omitempty" yaml:"tls,omitempty"`
	SNI               string          `json:"servername,omitempty" yaml:"servername,omitempty"`
	SkipCertVerify    bool            `json:"skip-cert-verify,omitempty" yaml:"skip-cert-verify,omitempty"`
	Reality           *RealityOptions `json:"reality-opts,omitempty" yaml:"reality-opts,omitempty"`
	ClientFingerprint string          `json:"client-fingerprint,omitempty" yaml:"client-fingerprint,omitempty"`
	WSOptions         *WSOptions      `json:"ws-opts,omitempty" yaml:"ws-opts,omitempty"`
	GRPCOptions       *GRPCOptions    `json:"grpc-opts,omitempty" yaml:"grpc-opts,omitempty"`
	UDP               bool            `json:"udp,omitempty" yaml:"udp,omitempty"`
}

// TrojanOptions contains connection parameters for Trojan proxies.
type TrojanOptions struct {
	Password       string       `json:"password" yaml:"password"`
	SNI            string       `json:"sni,omitempty" yaml:"sni,omitempty"`
	ALPN           []string     `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	SkipCertVerify bool         `json:"skip-cert-verify,omitempty" yaml:"skip-cert-verify,omitempty"`
	Network        string       `json:"network,omitempty" yaml:"network,omitempty"`
	WSOptions      *WSOptions   `json:"ws-opts,omitempty" yaml:"ws-opts,omitempty"`
	GRPCOptions    *GRPCOptions `json:"grpc-opts,omitempty" yaml:"grpc-opts,omitempty"`
	UDP            bool         `json:"udp,omitempty" yaml:"udp,omitempty"`
}

// Hysteria2Options contains connection parameters for Hysteria 2 proxies.
type Hysteria2Options struct {
	Password       string   `json:"password" yaml:"password"`
	Ports          string   `json:"ports,omitempty" yaml:"ports,omitempty"`
	SNI            string   `json:"sni,omitempty" yaml:"sni,omitempty"`
	SkipCertVerify bool     `json:"skip-cert-verify,omitempty" yaml:"skip-cert-verify,omitempty"`
	ALPN           []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	Obfs           string   `json:"obfs,omitempty" yaml:"obfs,omitempty"`
	ObfsPassword   string   `json:"obfs-password,omitempty" yaml:"obfs-password,omitempty"`
	Up             string   `json:"up,omitempty" yaml:"up,omitempty"`
	Down           string   `json:"down,omitempty" yaml:"down,omitempty"`
}

// TUICOptions contains connection parameters for TUIC proxies.
type TUICOptions struct {
	UUID                 string   `json:"uuid" yaml:"uuid"`
	Password             string   `json:"password" yaml:"password"`
	CongestionController string   `json:"congestion_controller,omitempty" yaml:"congestion_controller,omitempty"`
	UDPRelayMode         string   `json:"udp_relay_mode,omitempty" yaml:"udp_relay_mode,omitempty"`
	SNI                  string   `json:"sni,omitempty" yaml:"sni,omitempty"`
	ALPN                 []string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	SkipCertVerify       bool     `json:"skip-cert-verify,omitempty" yaml:"skip-cert-verify,omitempty"`
	DisableSNI           bool     `json:"disable_sni,omitempty" yaml:"disable_sni,omitempty"`
}

// WireGuardOptions contains connection parameters for WireGuard proxies.
type WireGuardOptions struct {
	PrivateKey   string   `json:"private-key" yaml:"private-key"`
	PublicKey    string   `json:"public-key,omitempty" yaml:"public-key,omitempty"`
	IP           string   `json:"ip,omitempty" yaml:"ip,omitempty"`
	IPv6         string   `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	MTU          int      `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Reserved     []int    `json:"reserved,omitempty" yaml:"reserved,omitempty"`
	PresharedKey string   `json:"preshared-key,omitempty" yaml:"preshared-key,omitempty"`
	UDP          bool     `json:"udp,omitempty" yaml:"udp,omitempty"`
}
