package domain_test

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"clash-sub-parser/internal/domain"
)

func TestNodeCredentialVaultEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key})
	if err != nil {
		t.Fatalf("NewNodeCredentialVault: %v", err)
	}

	payload := &domain.NodeCredentialPayload{
		LogicalID: "test-node-12345",
		Protocol:  domain.ProtocolSS,
		Server:    "1.2.3.4",
		Port:      8388,
		Version:   1,
		Digest:    "digest-abc",
		Credentials: domain.InboundProtocolCredential{
			Password: "super-secret-password",
			Method:   "aes-128-gcm",
		},
	}

	record, err := vault.Encrypt(payload)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if record.KeyID != "k1" {
		t.Fatalf("expected key ID k1, got %s", record.KeyID)
	}
	if len(record.Nonce) == 0 || len(record.Ciphertext) == 0 {
		t.Fatalf("expected non-empty nonce and ciphertext")
	}

	decrypted, err := vault.Decrypt(record, domain.ProtocolSS)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted.LogicalID != payload.LogicalID {
		t.Fatalf("logical ID mismatch: %s != %s", decrypted.LogicalID, payload.LogicalID)
	}
	if decrypted.Credentials.Password != "super-secret-password" {
		t.Fatalf("password mismatch: %s", decrypted.Credentials.Password)
	}

	// Mismatched protocol should fail authentication (AAD check)
	if _, err := vault.Decrypt(record, domain.ProtocolVMess); err == nil {
		t.Fatalf("expected decryption with mismatched protocol to fail")
	}

	// Tampered ciphertext should fail
	record.Ciphertext[0] ^= 0xff
	if _, err := vault.Decrypt(record, domain.ProtocolSS); err == nil {
		t.Fatalf("expected decryption of tampered ciphertext to fail")
	}
}

func TestNodeCredentialVaultKeyRotation(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	rand.Read(k1)
	rand.Read(k2)

	// Vault with primary k1
	v1, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": k1})
	if err != nil {
		t.Fatal(err)
	}

	payload := &domain.NodeCredentialPayload{
		LogicalID: "test-node-rotate",
		Protocol:  domain.ProtocolTrojan,
		Version:   2,
		Credentials: domain.InboundProtocolCredential{
			Password: "trojan-pass",
		},
	}
	rec, err := v1.Encrypt(payload)
	if err != nil {
		t.Fatal(err)
	}

	// Dual key window vault: primary k2, can still read k1
	v2, err := domain.NewNodeCredentialVault("k2", map[string][]byte{"k1": k1, "k2": k2})
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := v2.Decrypt(rec, domain.ProtocolTrojan)
	if err != nil {
		t.Fatalf("dual key vault failed to decrypt k1 record: %v", err)
	}
	if decrypted.Credentials.Password != "trojan-pass" {
		t.Fatalf("password mismatch: %s", decrypted.Credentials.Password)
	}

	// Re-encrypt with k2
	rec2, err := v2.Encrypt(decrypted)
	if err != nil {
		t.Fatal(err)
	}
	if rec2.KeyID != "k2" {
		t.Fatalf("expected rec2 KeyID k2, got %s", rec2.KeyID)
	}

	// Vault without k1 should decrypt rec2 successfully
	v3, err := domain.NewNodeCredentialVault("k2", map[string][]byte{"k2": k2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v3.Decrypt(rec2, domain.ProtocolTrojan); err != nil {
		t.Fatalf("v3 failed to decrypt rec2: %v", err)
	}
	// v3 cannot decrypt rec (k1)
	if _, err := v3.Decrypt(rec, domain.ProtocolTrojan); err == nil {
		t.Fatalf("expected v3 without k1 to fail decrypting rec")
	}
}

func TestNodeCredentialVaultFromEnv(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	hexKey := hex.EncodeToString(key)

	t.Setenv(domain.EnvNodeCredentialMasterKey, hexKey)
	vault, err := domain.NewNodeCredentialVaultFromEnv()
	if err != nil {
		t.Fatalf("NewNodeCredentialVaultFromEnv: %v", err)
	}
	if vault == nil {
		t.Fatalf("expected non-nil vault")
	}

	t.Setenv(domain.EnvNodeCredentialMasterKey, "")
	emptyVault, err := domain.NewNodeCredentialVaultFromEnv()
	if err != nil {
		t.Fatalf("expected nil err for empty env: %v", err)
	}
	if emptyVault != nil {
		t.Fatalf("expected nil vault for empty env")
	}
}
