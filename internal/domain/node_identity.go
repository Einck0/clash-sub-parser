package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// VerifiedNodeIdentity is the version-stamped, non-forgeable identity binding
// persisted alongside AEAD-encrypted credentials. It captures the canonical
// protocol/server/port plus non-secret transport material at the time of
// credential ingestion, independent of the logicalID hash.
//
// A trusted identity binding is ONLY produced by the parser+inventory pipeline
// during reconciliation — never by an untrusted payload alone.
type VerifiedNodeIdentity struct {
	LogicalID       string   `json:"logical_id"`
	Protocol        Protocol `json:"protocol"`
	Server          string   `json:"server"`
	Port            int      `json:"port"`
	Version         int      `json:"version"`
	TransportDigest string   `json:"transport_digest"` // SHA-256 of canonical non-secret transport
	IdentityVersion int      `json:"identity_version"` // algorithm version (1 = current ComputeNodeLogicalID)
}

// CanonicalTransportDigest computes a deterministic SHA-256 hex digest of sorted,
// lowercased, non-secret transport key=value pairs — the same material that
// participates in ComputeNodeLogicalID. Sensitive keys are excluded.
func CanonicalTransportDigest(transport map[string]string) string {
	if len(transport) == 0 {
		return sha256Hex("")
	}

	// Reuse the same sensitive-key exclusion as ComputeNodeLogicalID.
	sensitiveKeys := map[string]bool{
		"password": true, "secret": true, "token": true, "key": true,
		"private_key": true, "private-key": true,
		"public_key": true, "public-key": true,
		"preshared_key": true, "pre_shared_key": true, "pre-shared-key": true,
		"psk": true, "obfs-password": true, "obfs_password": true,
		"uuid": true, "auth": true, "username": true, "user": true,
	}

	var keys []string
	for k := range transport {
		lk := strings.ToLower(strings.TrimSpace(k))
		if !sensitiveKeys[lk] {
			keys = append(keys, lk)
		}
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		val := strings.TrimSpace(transport[k])
		parts = append(parts, fmt.Sprintf("%s=%s", k, val))
	}
	return sha256Hex(strings.Join(parts, "&"))
}

// NewVerifiedNodeIdentity constructs a VerifiedNodeIdentity from trusted parser output.
func NewVerifiedNodeIdentity(logicalID string, protocol Protocol, server string, port, credVersion int, transport map[string]string) VerifiedNodeIdentity {
	return VerifiedNodeIdentity{
		LogicalID:       logicalID,
		Protocol:        protocol,
		Server:          strings.ToLower(strings.TrimSpace(server)),
		Port:            port,
		Version:         credVersion,
		TransportDigest: CanonicalTransportDigest(transport),
		IdentityVersion: 1,
	}
}

// MatchesEndpoint returns true if the identity's server/port/protocol match the
// given candidate values. This is the core anti-spoofing check: a payload
// claiming the same logicalID+version but different server/port is rejected.
func (v VerifiedNodeIdentity) MatchesEndpoint(protocol Protocol, server string, port int) bool {
	return v.Protocol == protocol &&
		v.Server == strings.ToLower(strings.TrimSpace(server)) &&
		v.Port == port
}

// ValidateNonEmpty returns an error if required fields are missing or out of range.
func (v VerifiedNodeIdentity) ValidateNonEmpty() error {
	if strings.TrimSpace(v.LogicalID) == "" {
		return NewValidationError("identity_missing_logical_id", "verified identity requires logical_id")
	}
	if !v.Protocol.IsValid() {
		return NewValidationError("identity_invalid_protocol", "verified identity has invalid protocol")
	}
	if strings.TrimSpace(v.Server) == "" {
		return NewValidationError("identity_missing_server", "verified identity requires server")
	}
	if v.Port < 1 || v.Port > 65535 {
		return NewValidationError("identity_invalid_port", fmt.Sprintf("verified identity port %d out of range", v.Port))
	}
	if v.Version < 1 {
		return NewValidationError("identity_invalid_version", fmt.Sprintf("verified identity version %d must be >= 1", v.Version))
	}
	return nil
}

// ComputeIdentityHMAC produces an HMAC-SHA256 hex string binding the identity
// fields to a secret key. This is stored alongside the AEAD record for
// indexed lookups without exposing plaintext transport details.
func ComputeIdentityHMAC(key []byte, identity VerifiedNodeIdentity) string {
	canonical := fmt.Sprintf("v%d|%s|%s|%s|%d|%s|%d",
		identity.IdentityVersion,
		identity.LogicalID,
		strings.ToLower(string(identity.Protocol)),
		identity.Server,
		identity.Port,
		identity.TransportDigest,
		identity.Version,
	)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
