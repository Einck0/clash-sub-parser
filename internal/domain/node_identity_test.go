package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestVerifiedNodeIdentity_AntiSpoofingAndTransportParity(t *testing.T) {
	transportYAML := map[string]string{
		"network":       "quic",
		"tls":           "true",
		"sni":           "hy2.example.com",
		"obfs":          "salamander",
		"obfs-password": "top-secret-obfs-password",
	}
	transportURI := map[string]string{
		"sni":           "hy2.example.com",
		"network":       "quic",
		"obfs":          "salamander",
		"tls":           "true",
		"obfs_password": "different-secret-password",
	}

	logicalID := domain.ComputeNodeLogicalID(domain.ProtocolHysteria2, "hy2.example.com", 8443, transportYAML)
	idYAML := domain.NewVerifiedNodeIdentity(logicalID, domain.ProtocolHysteria2, "HY2.EXAMPLE.COM", 8443, 1, transportYAML)
	idURI := domain.NewVerifiedNodeIdentity(logicalID, domain.ProtocolHysteria2, "hy2.example.com", 8443, 1, transportURI)

	if err := idYAML.ValidateNonEmpty(); err != nil {
		t.Fatalf("ValidateNonEmpty failed: %v", err)
	}
	if idYAML.TransportDigest != idURI.TransportDigest {
		t.Fatalf("expected identical TransportDigest across YAML/URI and secret rotation: %s vs %s", idYAML.TransportDigest, idURI.TransportDigest)
	}
	if !idYAML.MatchesEndpoint(domain.ProtocolHysteria2, "hy2.example.com", 8443) {
		t.Fatal("expected legitimate endpoint to match")
	}
	if idYAML.MatchesEndpoint(domain.ProtocolHysteria2, "evil.example.com", 8443) {
		t.Fatal("expected spoofed server with same logicalID/version to be rejected")
	}
	if idYAML.MatchesEndpoint(domain.ProtocolHysteria2, "hy2.example.com", 9999) {
		t.Fatal("expected spoofed port with same logicalID/version to be rejected")
	}

	// Missing binding / zero version must fail closed
	legacy := domain.VerifiedNodeIdentity{LogicalID: logicalID, Protocol: domain.ProtocolHysteria2}
	if err := legacy.ValidateNonEmpty(); err == nil {
		t.Fatal("expected legacy missing server/port/version binding to fail closed")
	}
}

func TestSafeNodeConnectionAndPatchValidation(t *testing.T) {
	payload := &domain.NodeCredentialPayload{
		LogicalID: "node_wg_test",
		Protocol:  domain.ProtocolWireGuard,
		Server:    "wg.example.com",
		Port:      51820,
		Version:   2,
		Credentials: domain.InboundProtocolCredential{
			PrivateKey:   "super-secret-wg-privkey",
			PublicKey:    "wg-peer-pubkey",
			PreSharedKey: "super-secret-wg-psk",
			LocalAddress: []string{"10.0.0.2/32"},
			Reserved:     []uint8{1, 2, 3},
			MTU:          1420,
			DNS:          []string{"1.1.1.1"},
		},
	}

	conn := domain.ProjectSafeNodeConnection(payload)
	if !conn.Available || !conn.HasPrivateKey || !conn.HasPreSharedKey {
		t.Fatalf("unexpected connection projection: %+v", conn)
	}
	rawJSON, _ := json.Marshal(conn)
	if strings.Contains(string(rawJSON), "super-secret-wg-privkey") || strings.Contains(string(rawJSON), "super-secret-wg-psk") {
		t.Fatalf("SafeNodeConnection leaked secret in JSON: %s", string(rawJSON))
	}

	id := domain.NewVerifiedNodeIdentity(payload.LogicalID, payload.Protocol, payload.Server, payload.Port, payload.Version, nil)

	// Valid secret rotation with matching CAS version
	validPatch := domain.NodeConnectionPatchRequest{
		ExpectedCredentialVersion: 2,
		LocalAddress:              []string{"10.0.0.2/32", "fd00::2/128"},
		PrivateKeyInput:           "rotated-privkey",
	}
	if err := validPatch.ValidateAgainstBaseline(id, payload); err != nil {
		t.Fatalf("expected valid patch to pass: %v", err)
	}

	// Stale CAS version rejected
	stalePatch := domain.NodeConnectionPatchRequest{ExpectedCredentialVersion: 1}
	if err := stalePatch.ValidateAgainstBaseline(id, payload); err == nil {
		t.Fatal("expected stale CAS version to be rejected")
	}

	// Mutating server under existing logical_id rejected
	evilServer := "evil.example.com"
	serverPatch := domain.NodeConnectionPatchRequest{
		ExpectedCredentialVersion: 2,
		Server:                    &evilServer,
	}
	if err := serverPatch.ValidateAgainstBaseline(id, payload); err == nil {
		t.Fatal("expected server mutation on existing logical_id to be rejected")
	}
}
