package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"clash-sub-parser/internal/domain"
)

// areInboundCredentialsEqual checks semantic equality between two InboundProtocolCredentials.
// JSON key ordering, transport map iteration ordering, or formatting do NOT affect this comparison.
func areInboundCredentialsEqual(a, b domain.InboundProtocolCredential) bool {
	if a.Password != b.Password ||
		a.UUID != b.UUID ||
		a.Method != b.Method ||
		a.AlterID != b.AlterID ||
		a.PrivateKey != b.PrivateKey ||
		a.PublicKey != b.PublicKey ||
		a.EffectivePreSharedKey() != b.EffectivePreSharedKey() ||
		a.MTU != b.MTU ||
		a.CongestionControl != b.CongestionControl ||
		a.UDPRelayMode != b.UDPRelayMode ||
		a.SNI != b.SNI ||
		a.DisableSNI != b.DisableSNI ||
		a.Username != b.Username {
		return false
	}

	if !slices.Equal(a.LocalAddress, b.LocalAddress) {
		return false
	}
	if !slices.Equal(a.Reserved, b.Reserved) {
		return false
	}
	if !slices.Equal(a.DNS, b.DNS) {
		return false
	}
	if !slices.Equal(a.ALPN, b.ALPN) {
		return false
	}

	// Compare transport maps semantically
	if len(a.Transport) != len(b.Transport) {
		return false
	}
	for k, vA := range a.Transport {
		vB, exists := b.Transport[k]
		if !exists || vA != vB {
			return false
		}
	}

	return true
}

// areConnectionParametersEqual returns true if server, port, protocol, and credentials are semantically identical.
func areConnectionParametersEqual(
	protocolA, protocolB domain.Protocol,
	serverA, serverB string,
	portA, portB int,
	credsA, credsB domain.InboundProtocolCredential,
) bool {
	if protocolA != protocolB {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(serverA), strings.TrimSpace(serverB)) {
		return false
	}
	if portA != portB {
		return false
	}
	return areInboundCredentialsEqual(credsA, credsB)
}

// computeScopedLogicalID derives an isolated node logical ID bound to a specific subscription
// to prevent cross-subscription credential merging or ID collision.
func computeScopedLogicalID(subscriptionID string, baseLogicalID string) string {
	canonical := subscriptionID + "|" + baseLogicalID
	sum := sha256.Sum256([]byte(canonical))
	return "node_" + hex.EncodeToString(sum[:16])
}
