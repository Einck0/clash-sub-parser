package domain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	// DefaultKeyID is the default identifier for node credential encryption keys.
	DefaultKeyID = "k1"

	// EnvNodeCredentialMasterKey is the environment variable for the node credential master key (hex-encoded 32 bytes).
	EnvNodeCredentialMasterKey = "CSP_NODE_CREDENTIAL_KEY"
)

// InboundProtocolCredential contains protocol-specific credentials and optional transport details.
type InboundProtocolCredential struct {
	Password     string            `json:"password,omitempty"`
	UUID         string            `json:"uuid,omitempty"`
	Method       string            `json:"method,omitempty"`
	AlterID      int               `json:"alter_id,omitempty"`
	PrivateKey   string            `json:"private_key,omitempty"`
	PublicKey    string            `json:"public_key,omitempty"`
	PresharedKey string            `json:"preshared_key,omitempty"`
	Username     string            `json:"username,omitempty"`
	Transport    map[string]string `json:"transport,omitempty"`
}

// NodeCredentialPayload represents the plaintext structure encrypted in NodeCredentialRecord.
type NodeCredentialPayload struct {
	LogicalID   string                     `json:"logical_id"`
	Protocol    Protocol                   `json:"protocol"`
	Server      string                     `json:"server"`
	Port        int                        `json:"port"`
	Version     int                        `json:"version"`
	Digest      string                     `json:"digest"`
	Credentials InboundProtocolCredential  `json:"credentials"`
}

// NodeCredentialRecord represents the persisted AEAD encrypted credential entry.
type NodeCredentialRecord struct {
	LogicalID  string    `json:"logical_id"`
	Version    int       `json:"version"`
	KeyID      string    `json:"key_id"`
	Nonce      []byte    `json:"-"`
	Ciphertext []byte    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NodeCredentialVault provides authenticated encryption and decryption for node credentials.
type NodeCredentialVault struct {
	primaryKeyID string
	keys         map[string][]byte
}

// NewNodeCredentialVault creates a vault with a primary key ID and a set of 32-byte AES keys.
func NewNodeCredentialVault(primaryKeyID string, keys map[string][]byte) (*NodeCredentialVault, error) {
	if primaryKeyID == "" {
		primaryKeyID = DefaultKeyID
	}
	if len(keys) == 0 {
		return nil, NewSecurityError("missing_master_keys", "at least one master key must be provided")
	}
	primaryKey, exists := keys[primaryKeyID]
	if !exists || len(primaryKey) != 32 {
		return nil, NewSecurityError("invalid_primary_key", "primary key must be present and exactly 32 bytes")
	}
	for id, key := range keys {
		if len(key) != 32 {
			return nil, NewSecurityError("invalid_key_length", fmt.Sprintf("key %q must be exactly 32 bytes, got %d", id, len(key)))
		}
	}
	return &NodeCredentialVault{
		primaryKeyID: primaryKeyID,
		keys:         keys,
	}, nil
}

// NewNodeCredentialVaultFromEnv attempts to load the master key from the environment.
// If not configured, returns (nil, nil) indicating encryption is unconfigured.
func NewNodeCredentialVaultFromEnv() (*NodeCredentialVault, error) {
	envKey := strings.TrimSpace(os.Getenv(EnvNodeCredentialMasterKey))
	if envKey == "" {
		return nil, nil
	}
	keyBytes, err := hex.DecodeString(envKey)
	if err != nil || len(keyBytes) != 32 {
		return nil, NewSecurityError("invalid_env_key", "CSP_NODE_CREDENTIAL_KEY must be a 64-character hex string (32 bytes)")
	}
	return NewNodeCredentialVault(DefaultKeyID, map[string][]byte{
		DefaultKeyID: keyBytes,
	})
}

// Encrypt serializes and encrypts the payload using AES-GCM with associated data bound to (logical_id, version).
func (v *NodeCredentialVault) Encrypt(payload *NodeCredentialPayload) (*NodeCredentialRecord, error) {
	if v == nil {
		return nil, NewSecurityError("vault_nil", "node credential vault is nil (fail-closed)")
	}
	if payload == nil {
		return nil, NewValidationError("nil_payload", "payload cannot be nil")
	}
	if payload.LogicalID == "" {
		return nil, NewValidationError("missing_logical_id", "payload logical ID is required")
	}

	key, ok := v.keys[v.primaryKeyID]
	if !ok || len(key) != 32 {
		return nil, NewSecurityError("key_not_found", "primary encryption key not found in vault")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, NewInternalError("aes_cipher_error", err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, NewInternalError("gcm_cipher_error", err.Error())
	}

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, NewInternalError("marshal_error", err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, NewInternalError("rand_nonce_error", err.Error())
	}

	// Associated authenticated data (AAD) binds logical_id, version, and protocol
	aad := []byte(fmt.Sprintf("%s:%d:%s", payload.LogicalID, payload.Version, payload.Protocol))
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	now := NowUTC()
	return &NodeCredentialRecord{
		LogicalID:  payload.LogicalID,
		Version:    payload.Version,
		KeyID:      v.primaryKeyID,
		Nonce:      nonce,
		Ciphertext: ciphertext,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// Decrypt verifies and decrypts a NodeCredentialRecord into NodeCredentialPayload.
func (v *NodeCredentialVault) Decrypt(record *NodeCredentialRecord, expectedProtocol Protocol) (*NodeCredentialPayload, error) {
	if v == nil {
		return nil, NewSecurityError("vault_nil", "node credential vault is nil (fail-closed)")
	}
	if record == nil {
		return nil, NewValidationError("nil_record", "credential record is nil")
	}

	key, ok := v.keys[record.KeyID]
	if !ok {
		return nil, NewSecurityError("unknown_key_id", fmt.Sprintf("decryption key %q not found in vault", record.KeyID))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, NewInternalError("aes_cipher_error", err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, NewInternalError("gcm_cipher_error", err.Error())
	}

	if len(record.Nonce) != gcm.NonceSize() {
		return nil, NewSecurityError("invalid_nonce_size", "nonce size does not match cipher requirement")
	}

	aad := []byte(fmt.Sprintf("%s:%d:%s", record.LogicalID, record.Version, expectedProtocol))
	plaintext, err := gcm.Open(nil, record.Nonce, record.Ciphertext, aad)
	if err != nil {
		return nil, NewSecurityError("authentication_failed", "failed to authenticate or decrypt credential ciphertext")
	}

	var payload NodeCredentialPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, NewInternalError("unmarshal_error", err.Error())
	}
	if payload.LogicalID != record.LogicalID {
		return nil, NewSecurityError("logical_id_mismatch", "decrypted logical ID does not match record")
	}
	if payload.Version != record.Version {
		return nil, NewSecurityError("version_mismatch", "decrypted version does not match record")
	}

	return &payload, nil
}
