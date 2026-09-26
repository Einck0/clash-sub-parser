package domain

import (
	"fmt"
	"net/netip"
	"strings"
)

// SafeNodeConnection is the API-safe, non-secret connection projection returned
// on admin-authenticated GET /api/v1/nodes/{logical_id} inside node.connection.
// Private keys, pre-shared keys, and passwords are NEVER serialized; only
// presence booleans from the verified credential record are exposed.
type SafeNodeConnection struct {
	Available         bool     `json:"available"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
	Server            string   `json:"server,omitempty"`
	Port              int      `json:"port,omitempty"`
	LocalAddress      []string `json:"local_address,omitempty"`
	PublicKey         string   `json:"public_key,omitempty"`
	Reserved          []uint8  `json:"reserved,omitempty"`
	MTU               int      `json:"mtu,omitempty"`
	DNS               []string `json:"dns,omitempty"`
	UUID              string   `json:"uuid,omitempty"`
	CongestionControl string   `json:"congestion_control,omitempty"`
	UDPRelayMode      string   `json:"udp_relay_mode,omitempty"`
	ALPN              []string `json:"alpn,omitempty"`
	SNI               string   `json:"sni,omitempty"`
	DisableSNI        bool     `json:"disable_sni,omitempty"`
	Method            string   `json:"method,omitempty"`
	Flow              string   `json:"flow,omitempty"`
	RealityPublicKey  string   `json:"reality_public_key,omitempty"`
	RealityShortID    string   `json:"reality_short_id,omitempty"`
	ClientFingerprint string   `json:"client_fingerprint,omitempty"`
	Up                string   `json:"up,omitempty"`
	Down              string   `json:"down,omitempty"`
	Obfs              string   `json:"obfs,omitempty"`
	HasPrivateKey     bool     `json:"has_private_key"`
	HasPreSharedKey   bool     `json:"has_pre_shared_key"`
	HasPassword       bool     `json:"has_password"`
}

// UnavailableNodeConnection returns a fail-closed connection projection with no fabricated defaults.
func UnavailableNodeConnection(reason string) SafeNodeConnection {
	if strings.TrimSpace(reason) == "" {
		reason = "credential_unavailable"
	}
	return SafeNodeConnection{
		Available:         false,
		UnavailableReason: reason,
	}
}

// ProjectSafeNodeConnection projects non-secret connection metadata from a verified payload.
func ProjectSafeNodeConnection(payload *NodeCredentialPayload) SafeNodeConnection {
	if payload == nil || strings.TrimSpace(payload.Server) == "" || payload.Port < 1 || payload.Port > 65535 {
		return UnavailableNodeConnection("credential_unavailable")
	}
	c := payload.Credentials
	t := c.Transport
	alpn := append([]string(nil), c.ALPN...)
	if len(alpn) == 0 && t != nil && strings.TrimSpace(t["alpn"]) != "" {
		for _, part := range strings.Split(t["alpn"], ",") {
			if p := strings.TrimSpace(part); p != "" {
				alpn = append(alpn, p)
			}
		}
	}
	sni := strings.TrimSpace(c.SNI)
	if sni == "" && t != nil {
		sni = strings.TrimSpace(t["sni"])
	}
	return SafeNodeConnection{
		Available:         true,
		Server:            strings.TrimSpace(payload.Server),
		Port:              payload.Port,
		LocalAddress:      append([]string(nil), c.LocalAddress...),
		PublicKey:         strings.TrimSpace(c.PublicKey),
		Reserved:          append([]uint8(nil), c.Reserved...),
		MTU:               c.MTU,
		DNS:               append([]string(nil), c.DNS...),
		UUID:              strings.TrimSpace(c.UUID),
		CongestionControl: strings.TrimSpace(c.CongestionControl),
		UDPRelayMode:      strings.TrimSpace(c.UDPRelayMode),
		ALPN:              alpn,
		SNI:               sni,
		DisableSNI:        c.DisableSNI || HasTUICDisableSNI(t),
		Method:            strings.TrimSpace(c.Method),
		Flow:              strings.TrimSpace(t["flow"]),
		RealityPublicKey:  strings.TrimSpace(t["pbk"]),
		RealityShortID:    strings.TrimSpace(t["sid"]),
		ClientFingerprint: strings.TrimSpace(t["fp"]),
		Up:                strings.TrimSpace(t["up"]),
		Down:              strings.TrimSpace(t["down"]),
		Obfs:              strings.TrimSpace(t["obfs"]),
		HasPrivateKey:     strings.TrimSpace(c.PrivateKey) != "",
		HasPreSharedKey:   c.EffectivePreSharedKey() != "",
		HasPassword:       strings.TrimSpace(c.Password) != "",
	}
}

// NodeConnectionPatchRequest is the frozen request DTO for PATCH /api/v1/nodes/{logical_id}/connection.
// Secret inputs are write-only; empty strings preserve existing secrets.
// Endpoint or identity-transport changes that alter logical_id are rejected (409 identity_mutation_forbidden).
type NodeConnectionPatchRequest struct {
	ExpectedCredentialVersion int      `json:"expected_credential_version"`
	DisplayName               *string  `json:"display_name,omitempty"`
	Server                    *string  `json:"server,omitempty"`
	Port                      *int     `json:"port,omitempty"`
	LocalAddress              []string `json:"local_address,omitempty"`
	PublicKey                 *string  `json:"public_key,omitempty"`
	Reserved                  []uint8  `json:"reserved,omitempty"`
	MTU                       *int     `json:"mtu,omitempty"`
	DNS                       []string `json:"dns,omitempty"`
	UUID                      *string  `json:"uuid,omitempty"`
	CongestionControl         *string  `json:"congestion_control,omitempty"`
	UDPRelayMode              *string  `json:"udp_relay_mode,omitempty"`
	ALPN                      []string `json:"alpn,omitempty"`
	SNI                       *string  `json:"sni,omitempty"`
	DisableSNI                *bool    `json:"disable_sni,omitempty"`
	Method                    *string  `json:"method,omitempty"`
	PrivateKeyInput           string   `json:"private_key_input,omitempty"`
	PreSharedKeyInput         string   `json:"pre_shared_key_input,omitempty"`
	PasswordInput             string   `json:"password_input,omitempty"`
}

// ValidateAgainstBaseline validates the patch request against the current verified identity and payload.
func (req NodeConnectionPatchRequest) ValidateAgainstBaseline(identity VerifiedNodeIdentity, current *NodeCredentialPayload) error {
	if current == nil {
		return NewNotFoundError("node_credential_not_found", "node credentials are unavailable")
	}
	if req.ExpectedCredentialVersion < 1 {
		return NewValidationError("missing_expected_credential_version", "expected_credential_version must be >= 1")
	}
	if req.ExpectedCredentialVersion != current.Version {
		return NewConflictError("credential_version_conflict", fmt.Sprintf("expected credential version %d does not match current version %d", req.ExpectedCredentialVersion, current.Version))
	}
	if req.Server != nil && strings.ToLower(strings.TrimSpace(*req.Server)) != strings.ToLower(strings.TrimSpace(identity.Server)) {
		return NewConflictError("identity_mutation_forbidden", "changing server alters node logical identity; update subscription source and reconcile instead")
	}
	if req.Port != nil && *req.Port != identity.Port {
		return NewConflictError("identity_mutation_forbidden", "changing port alters node logical identity; update subscription source and reconcile instead")
	}
	if req.SNI != nil && strings.TrimSpace(*req.SNI) != strings.TrimSpace(current.Credentials.SNI) &&
		strings.TrimSpace(*req.SNI) != strings.TrimSpace(current.Credentials.Transport["sni"]) {
		return NewConflictError("identity_mutation_forbidden", "changing transport SNI alters node logical identity; update subscription source and reconcile instead")
	}
	if req.DisableSNI != nil && *req.DisableSNI != (current.Credentials.DisableSNI || HasTUICDisableSNI(current.Credentials.Transport)) {
		return NewConflictError("identity_mutation_forbidden", "changing transport disable_sni alters node logical identity; update subscription source and reconcile instead")
	}
	if len(req.ALPN) > 0 && strings.Join(req.ALPN, ",") != strings.Join(current.Credentials.ALPN, ",") &&
		strings.Join(req.ALPN, ",") != strings.TrimSpace(current.Credentials.Transport["alpn"]) {
		return NewConflictError("identity_mutation_forbidden", "changing transport ALPN alters node logical identity; update subscription source and reconcile instead")
	}
	if identity.Protocol == ProtocolWireGuard {
		for _, cidr := range req.LocalAddress {
			if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
				return NewValidationError("invalid_local_address", "WireGuard local_address must be valid CIDR")
			}
		}
		if len(req.Reserved) > 0 && len(req.Reserved) != 3 {
			return NewValidationError("invalid_reserved", "WireGuard reserved must contain exactly 3 bytes")
		}
		if req.MTU != nil && (*req.MTU < 576 || *req.MTU > 9000) {
			return NewValidationError("invalid_mtu", "WireGuard MTU must be between 576 and 9000")
		}
	}
	return nil
}
