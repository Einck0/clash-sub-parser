package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Internal management keys that must be stripped during client export.
var internalManagementKeys = map[string]struct{}{
	"id":                  {},
	"logical_id":          {},
	"sub_id":              {},
	"source":              {},
	"source_id":           {},
	"source_logical_id":   {},
	"subscription_id":     {},
	"internal_id":         {},
	"status":              {},
	"latency":             {},
	"latency_ms":          {},
	"speed":               {},
	"speed_mbps":          {},
	"checked_at":          {},
	"original_order":      {},
	"lifecycle_state":     {},
	"created_at":          {},
	"updated_at":          {},
	"payload_fingerprint": {},
	"admin_token":         {},
	"management_token":    {},
	"upstream_token":      {},
	"secret_key":          {},
	"token":               {},
	"auth_token":          {},
	"raw_link":            {},
	"fetch_url":           {},
}

// Node represents a proxy node with normalized connection parameters.
type Node struct {
	ID                 int64          `json:"id" yaml:"id"`
	LogicalID          string         `json:"logical_id" yaml:"logical_id"`
	Name               string         `json:"name" yaml:"name"`
	Protocol           ProtocolType   `json:"protocol" yaml:"protocol"`
	Server             string         `json:"server" yaml:"server"`
	Port               int            `json:"port" yaml:"port"`
	NormalizedPayload  map[string]any `json:"normalized_payload" yaml:"normalized_payload"`
	PayloadFingerprint string         `json:"payload_fingerprint" yaml:"payload_fingerprint"`
	LifecycleState     LifecycleState `json:"lifecycle_state" yaml:"lifecycle_state"`
	CreatedAt          time.Time      `json:"created_at" yaml:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at" yaml:"updated_at"`
}

// Validate ensures node attributes and protocol-specific fields are valid.
func (n *Node) Validate() error {
	if strings.TrimSpace(n.Name) == "" {
		return ErrMissingName
	}
	if strings.TrimSpace(n.Server) == "" {
		return ErrMissingServer
	}
	if n.Port < 1 || n.Port > 65535 {
		return ErrInvalidPort
	}

	normProto := n.CanonicalProtocol()
	switch normProto {
	case ProtocolShadowsocks, ProtocolVMess, ProtocolVLESS, ProtocolTrojan,
		ProtocolHysteria2, ProtocolTUIC, ProtocolWireGuard, ProtocolSOCKS5, ProtocolHTTP:
		// Valid known protocols
	default:
		return ErrUnsupportedProtocol
	}

	return nil
}

// CanonicalProtocol normalizes protocol aliases (e.g. ss -> shadowsocks, hy2 -> hysteria2).
func (n *Node) CanonicalProtocol() ProtocolType {
	p := strings.ToLower(strings.TrimSpace(string(n.Protocol)))
	switch p {
	case "ss", "shadowsocks":
		return ProtocolShadowsocks
	case "vmess":
		return ProtocolVMess
	case "vless":
		return ProtocolVLESS
	case "trojan":
		return ProtocolTrojan
	case "hy2", "hysteria2":
		return ProtocolHysteria2
	case "tuic":
		return ProtocolTUIC
	case "wg", "wireguard":
		return ProtocolWireGuard
	case "socks", "socks5":
		return ProtocolSOCKS5
	case "http", "https":
		return ProtocolHTTP
	default:
		return ProtocolType(p)
	}
}

// ComputeFingerprint generates a versioned SHA-256 fingerprint from the node's normalized payload.
func (n *Node) ComputeFingerprint() string {
	payload := n.NormalizedPayload
	if payload == nil {
		payload = make(map[string]any)
	}

	// Always ensure server, port, type are present
	clean := make(map[string]any)
	for k, v := range payload {
		if _, forbidden := internalManagementKeys[k]; !forbidden && v != nil {
			clean[k] = v
		}
	}
	clean["type"] = string(n.CanonicalProtocol())
	clean["server"] = strings.TrimSpace(n.Server)
	clean["port"] = n.Port

	// Deterministic JSON serialization
	b, err := json.Marshal(clean)
	if err != nil {
		b = []byte(fmt.Sprintf("%s:%s:%d", clean["type"], clean["server"], clean["port"]))
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("v1:%s", hex.EncodeToString(sum[:]))
}

// SetProtocolOptions serializes typed protocol options into NormalizedPayload.
func (n *Node) SetProtocolOptions(opts any) error {
	if opts == nil {
		return ErrNilOptions
	}

	data, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("failed to marshal protocol options: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to unmarshal into payload map: %w", err)
	}

	if n.NormalizedPayload == nil {
		n.NormalizedPayload = make(map[string]any)
	}

	for k, v := range m {
		if v != nil {
			n.NormalizedPayload[k] = v
		}
	}

	n.NormalizedPayload["type"] = string(n.CanonicalProtocol())
	n.NormalizedPayload["server"] = n.Server
	n.NormalizedPayload["port"] = n.Port

	n.PayloadFingerprint = n.ComputeFingerprint()
	return nil
}

// GetShadowsocksOptions deserializes NormalizedPayload into ShadowsocksOptions.
func (n *Node) GetShadowsocksOptions() (*ShadowsocksOptions, error) {
	var opts ShadowsocksOptions
	if err := n.unmarshalPayload(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// GetVMessOptions deserializes NormalizedPayload into VMessOptions.
func (n *Node) GetVMessOptions() (*VMessOptions, error) {
	var opts VMessOptions
	if err := n.unmarshalPayload(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// GetVLESSOptions deserializes NormalizedPayload into VLESSOptions.
func (n *Node) GetVLESSOptions() (*VLESSOptions, error) {
	var opts VLESSOptions
	if err := n.unmarshalPayload(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// GetTrojanOptions deserializes NormalizedPayload into TrojanOptions.
func (n *Node) GetTrojanOptions() (*TrojanOptions, error) {
	var opts TrojanOptions
	if err := n.unmarshalPayload(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// GetHysteria2Options deserializes NormalizedPayload into Hysteria2Options.
func (n *Node) GetHysteria2Options() (*Hysteria2Options, error) {
	var opts Hysteria2Options
	if err := n.unmarshalPayload(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// GetTUICOptions deserializes NormalizedPayload into TUICOptions.
func (n *Node) GetTUICOptions() (*TUICOptions, error) {
	var opts TUICOptions
	if err := n.unmarshalPayload(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

// GetWireGuardOptions deserializes NormalizedPayload into WireGuardOptions.
func (n *Node) GetWireGuardOptions() (*WireGuardOptions, error) {
	var opts WireGuardOptions
	if err := n.unmarshalPayload(&opts); err != nil {
		return nil, err
	}
	return &opts, nil
}

func (n *Node) unmarshalPayload(target any) error {
	if n.NormalizedPayload == nil {
		return ErrMissingRequiredField
	}
	data, err := json.Marshal(n.NormalizedPayload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// SanitizeForExport removes all internal database management keys, retaining proxy fields.
func (n *Node) SanitizeForExport() map[string]any {
	out := make(map[string]any)

	out["name"] = n.Name
	out["type"] = string(n.CanonicalProtocol())
	out["server"] = n.Server
	out["port"] = n.Port

	if n.NormalizedPayload != nil {
		for k, v := range n.NormalizedPayload {
			if _, forbidden := internalManagementKeys[k]; forbidden {
				continue
			}
			out[k] = v
		}
	}

	return out
}

// Clone creates a deep copy of the Node.
func (n *Node) Clone() *Node {
	clone := *n
	if n.NormalizedPayload != nil {
		clone.NormalizedPayload = make(map[string]any, len(n.NormalizedPayload))
		for k, v := range n.NormalizedPayload {
			clone.NormalizedPayload[k] = v
		}
	}
	return &clone
}

// PayloadKeysSorted returns all keys in NormalizedPayload in sorted order.
func (n *Node) PayloadKeysSorted() []string {
	keys := make([]string, 0, len(n.NormalizedPayload))
	for k := range n.NormalizedPayload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
