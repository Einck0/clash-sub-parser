package domain_test

import (
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

func TestUUIDv7GenerationAndValidation(t *testing.T) {
	id, err := domain.NewUUIDv7()
	if err != nil {
		t.Fatalf("failed to generate UUIDv7: %v", err)
	}

	if len(id) != 36 {
		t.Fatalf("expected UUIDv7 length 36, got %d (%s)", len(id), id)
	}

	if !domain.IsValidUUIDv7(id) {
		t.Fatalf("generated ID %s should pass IsValidUUIDv7", id)
	}

	if err := domain.ValidateUUIDv7(id); err != nil {
		t.Fatalf("ValidateUUIDv7 failed for valid ID %s: %v", id, err)
	}

	// Verify version byte is 7 (14th char, index 14).
	if id[14] != '7' {
		t.Fatalf("expected version 7 at index 14, got %c in %s", id[14], id)
	}

	// Verify variant bits (index 19 must be 8, 9, a, or b).
	variantChar := id[19]
	if variantChar != '8' && variantChar != '9' && variantChar != 'a' && variantChar != 'b' &&
		variantChar != 'A' && variantChar != 'B' {
		t.Fatalf("expected variant [89ab] at index 19, got %c in %s", variantChar, id)
	}
}

func TestUUIDv7TimestampExtraction(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	id := domain.MustNewUUIDv7()
	after := time.Now().UTC().Add(time.Second)

	extractedTime, err := domain.ExtractTimeFromUUIDv7(id)
	if err != nil {
		t.Fatalf("failed to extract time from UUIDv7 %s: %v", id, err)
	}

	if extractedTime.Before(before) || extractedTime.After(after) {
		t.Fatalf("extracted time %v should be between %v and %v", extractedTime, before, after)
	}

	if extractedTime.Location() != time.UTC {
		t.Fatalf("extracted time must be UTC, got %v", extractedTime.Location())
	}
}

func TestUUIDv7InvalidInputs(t *testing.T) {
	invalidCases := []string{
		"",
		"not-a-uuid",
		"00000000-0000-0000-0000-000000000000",  // version 0
		"018e3a2b-4c5d-4123-8abc-def012345678",  // version 4
		"018e3a2b-4c5d-7123-0abc-def012345678",  // wrong variant
		"018e3a2b4c5d71238abcdef012345678",      // missing hyphens
		"018e3a2b-4c5d-7123-8abc-def01234567g",  // invalid hex char 'g'
		"018e3a2b-4c5d-7123-8abc-def0123456789", // too long
		"018e3a2b-4c5d-7123-8abc-def01234567",   // too short
	}

	for _, tc := range invalidCases {
		if domain.IsValidUUIDv7(tc) {
			t.Errorf("IsValidUUIDv7(%q) expected false, got true", tc)
		}
		if err := domain.ValidateUUIDv7(tc); err == nil {
			t.Errorf("ValidateUUIDv7(%q) expected error, got nil", tc)
		}
	}
}

func TestNodeLogicalIDComputation(t *testing.T) {
	params1 := map[string]string{
		"network":  "ws",
		"sni":      "example.com",
		"path":     "/graphql",
		"password": "secret-password-123", // secret should be ignored/excluded from identity
	}

	params2 := map[string]string{
		"path":     "/graphql",
		"sni":      "example.com",
		"network":  "ws",
		"password": "different-password-456", // same transport params, different secret
	}

	id1 := domain.ComputeNodeLogicalID(domain.ProtocolVMess, "hk01.node.com", 443, params1)
	id2 := domain.ComputeNodeLogicalID(domain.ProtocolVMess, "HK01.NODE.COM", 443, params2)

	if id1 == "" {
		t.Fatal("computed logical ID must not be empty")
	}

	// Should be case-insensitive on host and stable regardless of map iteration order or secrets.
	if id1 != id2 {
		t.Fatalf("expected identical logical ID across case and secrets, got %s vs %s", id1, id2)
	}

	if !domain.IsValidLogicalID(id1) {
		t.Fatalf("IsValidLogicalID failed for %s", id1)
	}

	// Ensure port difference changes logical ID.
	idDiffPort := domain.ComputeNodeLogicalID(domain.ProtocolVMess, "hk01.node.com", 8443, params1)
	if id1 == idDiffPort {
		t.Fatalf("different port must yield different logical ID, got same %s", id1)
	}

	// Ensure protocol difference changes logical ID.
	idDiffProtocol := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "hk01.node.com", 443, params1)
	if id1 == idDiffProtocol {
		t.Fatalf("different protocol must yield different logical ID, got same %s", id1)
	}

	// Ensure secrets never leak into logical ID string.
	if strings.Contains(id1, "secret") || strings.Contains(id1, "password") {
		t.Fatalf("logical ID must never contain secret plaintext: %s", id1)
	}
}

func TestComputeConnectionLogicalID(t *testing.T) {
	baseTransport := map[string]string{
		"network": "ws",
		"sni":     "example.com",
		"path":    "/ws",
		"tls":     "true",
	}
	baseCreds := domain.InboundProtocolCredential{
		Username: "synthetic-user-1",
		Password: "synthetic-password-A",
		UUID:     "11111111-1111-1111-1111-111111111111",
	}

	baseID := domain.ComputeConnectionLogicalID(domain.ProtocolHTTP, "proxy.example.com", 8080, baseTransport, baseCreds)
	if !domain.IsValidLogicalID(baseID) {
		t.Fatalf("invalid baseID: %s", baseID)
	}

	// 1. Map ordering parity: different map key insertion order must yield the EXACT SAME ID
	shuffledTransport := map[string]string{
		"tls":     "true",
		"path":    "/ws",
		"sni":     "example.com",
		"network": "ws",
	}
	shuffledID := domain.ComputeConnectionLogicalID(domain.ProtocolHTTP, "PROXY.EXAMPLE.COM", 8080, shuffledTransport, baseCreds)
	if baseID != shuffledID {
		t.Fatalf("map order or host casing must produce identical logical ID: %s != %s", baseID, shuffledID)
	}

	// 2. Different username -> distinct ID
	userDiffCreds := baseCreds
	userDiffCreds.Username = "synthetic-user-2"
	idUserDiff := domain.ComputeConnectionLogicalID(domain.ProtocolHTTP, "proxy.example.com", 8080, baseTransport, userDiffCreds)
	if baseID == idUserDiff {
		t.Fatalf("different username must yield distinct logical ID: %s", baseID)
	}

	// 3. Different password -> distinct ID
	passDiffCreds := baseCreds
	passDiffCreds.Password = "synthetic-password-B"
	idPassDiff := domain.ComputeConnectionLogicalID(domain.ProtocolHTTP, "proxy.example.com", 8080, baseTransport, passDiffCreds)
	if baseID == idPassDiff {
		t.Fatalf("different password must yield distinct logical ID: %s", baseID)
	}

	// 4. Different UUID -> distinct ID
	uuidDiffCreds := baseCreds
	uuidDiffCreds.UUID = "22222222-2222-2222-2222-222222222222"
	idUUIDDiff := domain.ComputeConnectionLogicalID(domain.ProtocolVLESS, "proxy.example.com", 8080, baseTransport, uuidDiffCreds)
	baseVLESS := domain.ComputeConnectionLogicalID(domain.ProtocolVLESS, "proxy.example.com", 8080, baseTransport, baseCreds)
	if baseVLESS == idUUIDDiff {
		t.Fatalf("different UUID must yield distinct logical ID: %s", baseVLESS)
	}

	// 5. Different WireGuard key -> distinct ID
	wgCreds1 := domain.InboundProtocolCredential{
		PrivateKey:   "priv-key-1",
		PublicKey:    "pub-key-1",
		LocalAddress: []string{"10.0.0.2/32"},
	}
	wgCreds2 := domain.InboundProtocolCredential{
		PrivateKey:   "priv-key-2",
		PublicKey:    "pub-key-1",
		LocalAddress: []string{"10.0.0.2/32"},
	}
	idWG1 := domain.ComputeConnectionLogicalID(domain.ProtocolWireGuard, "wg.example.com", 51820, nil, wgCreds1)
	idWG2 := domain.ComputeConnectionLogicalID(domain.ProtocolWireGuard, "wg.example.com", 51820, nil, wgCreds2)
	if idWG1 == idWG2 {
		t.Fatalf("different WireGuard private key must yield distinct logical ID: %s", idWG1)
	}

	// 6. Different TLS / transport param / header -> distinct ID
	tlsDiffTransport := map[string]string{
		"network": "ws",
		"sni":     "example.com",
		"path":    "/ws",
		"tls":     "false",
	}
	idTLSDiff := domain.ComputeConnectionLogicalID(domain.ProtocolHTTP, "proxy.example.com", 8080, tlsDiffTransport, baseCreds)
	if baseID == idTLSDiff {
		t.Fatalf("different TLS flag must yield distinct logical ID: %s", baseID)
	}

	headerDiffTransport := map[string]string{
		"network": "ws",
		"sni":     "example.com",
		"path":    "/ws",
		"tls":     "true",
		"headers": `{"X-Custom":"header-value"}`,
	}
	idHeaderDiff := domain.ComputeConnectionLogicalID(domain.ProtocolHTTP, "proxy.example.com", 8080, headerDiffTransport, baseCreds)
	if baseID == idHeaderDiff {
		t.Fatalf("different headers must yield distinct logical ID: %s", baseID)
	}

	// 7. Verify no auth_digest or plain secrets leak into ID
	if strings.Contains(baseID, "auth_digest") || strings.Contains(baseID, "synthetic") {
		t.Fatalf("logical ID must not contain auth_digest or secret substrings: %s", baseID)
	}
}
