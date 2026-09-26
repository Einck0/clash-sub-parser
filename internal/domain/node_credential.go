package domain

import "strings"

// InboundProtocolCredential contains protocol-specific credentials and optional transport details.
type InboundProtocolCredential struct {
	Password     string   `json:"password,omitempty"`
	UUID         string   `json:"uuid,omitempty"`
	Method       string   `json:"method,omitempty"`
	AlterID      int      `json:"alter_id,omitempty"`
	PrivateKey   string   `json:"private_key,omitempty"`
	PublicKey    string   `json:"public_key,omitempty"`
	PresharedKey string   `json:"preshared_key,omitempty"`
	PreSharedKey string   `json:"pre_shared_key,omitempty"`
	LocalAddress []string `json:"local_address,omitempty"`
	Reserved     []uint8  `json:"reserved,omitempty"`
	MTU          int      `json:"mtu,omitempty"`
	DNS          []string `json:"dns,omitempty"`

	CongestionControl string   `json:"congestion_control,omitempty"`
	UDPRelayMode      string   `json:"udp_relay_mode,omitempty"`
	ALPN              []string `json:"alpn,omitempty"`
	SNI               string   `json:"sni,omitempty"`
	DisableSNI        bool     `json:"disable_sni,omitempty"`

	Username  string            `json:"username,omitempty"`
	Transport map[string]string `json:"transport,omitempty"`
}

// EffectivePreSharedKey returns the WireGuard pre-shared key regardless of which field alias was populated.
func (c InboundProtocolCredential) EffectivePreSharedKey() string {
	if s := strings.TrimSpace(c.PreSharedKey); s != "" {
		return s
	}
	return strings.TrimSpace(c.PresharedKey)
}

// IsTruthy evaluates whether a string represents a boolean true flag ("true", "1", "yes", "on").
func IsTruthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

// HasInsecureTransport checks whether a transport map requests disabling TLS certificate verification.
func HasInsecureTransport(m map[string]string) bool {
	if m == nil {
		return false
	}
	for k, v := range m {
		kLower := strings.ToLower(strings.TrimSpace(k))
		if strings.Contains(kLower, "insecure") ||
			strings.Contains(kLower, "skip_cert") ||
			strings.Contains(kLower, "skip-cert") ||
			strings.Contains(kLower, "skipcert") {
			if IsTruthy(v) {
				return true
			}
		}
	}
	return false
}

// ExtractHy2Ports returns the Hysteria2 port-hopping specification if present in a transport map.
func ExtractHy2Ports(m map[string]string) string {
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

// HasTUICDisableSNI checks whether a transport map requests disabling TUIC SNI.
func HasTUICDisableSNI(m map[string]string) bool {
	if m == nil {
		return false
	}
	for _, k := range []string{"disable_sni", "disable-sni", "tuic_disable_sni"} {
		if IsTruthy(m[k]) {
			return true
		}
	}
	return false
}

// NodeCredentialPayload represents a plaintext node endpoint and protocol credential bundle.
type NodeCredentialPayload struct {
	LogicalID   string                    `json:"logical_id"`
	Protocol    Protocol                  `json:"protocol"`
	Server      string                    `json:"server"`
	Port        int                       `json:"port"`
	Version     int                       `json:"version,omitempty"`
	Digest      string                    `json:"digest,omitempty"`
	Credentials InboundProtocolCredential `json:"credentials"`
}
