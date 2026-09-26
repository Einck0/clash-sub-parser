package domain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// PublicationCredentialBinding captures the canonical, version-stamped credential and endpoint
// identity of a single node bound into an immutable publication.
type PublicationCredentialBinding struct {
	LogicalID         string   `json:"logical_id"`
	Protocol          Protocol `json:"protocol"`
	Server            string   `json:"server"`
	Port              int      `json:"port"`
	CredentialVersion int      `json:"credential_version"`
	TransportDigest   string   `json:"transport_digest,omitempty"`
	IdentityVersion   int      `json:"identity_version,omitempty"`
}

// Validate checks that all required binding fields are present and valid.
func (b PublicationCredentialBinding) Validate() error {
	if strings.TrimSpace(b.LogicalID) == "" {
		return NewValidationError("invalid_publication_binding", "publication credential binding missing logical_id")
	}
	if !b.Protocol.IsValid() {
		return NewValidationError("invalid_publication_binding", "publication credential binding has invalid protocol")
	}
	if strings.TrimSpace(b.Server) == "" {
		return NewValidationError("invalid_publication_binding", "publication credential binding missing server")
	}
	if b.Port < 1 || b.Port > 65535 {
		return NewValidationError("invalid_publication_binding", fmt.Sprintf("publication credential binding port %d out of range", b.Port))
	}
	if b.CredentialVersion < 1 {
		return NewValidationError("invalid_publication_binding", fmt.Sprintf("publication credential binding credential_version %d must be >= 1", b.CredentialVersion))
	}
	return nil
}

// Normalize returns a canonicalized copy of the binding.
func (b PublicationCredentialBinding) Normalize() PublicationCredentialBinding {
	idVer := b.IdentityVersion
	if idVer <= 0 && strings.TrimSpace(b.TransportDigest) != "" {
		idVer = 1
	}
	return PublicationCredentialBinding{
		LogicalID:         strings.TrimSpace(b.LogicalID),
		Protocol:          b.Protocol,
		Server:            strings.ToLower(strings.TrimSpace(b.Server)),
		Port:              b.Port,
		CredentialVersion: b.CredentialVersion,
		TransportDigest:   strings.TrimSpace(b.TransportDigest),
		IdentityVersion:   idVer,
	}
}

// ToVerifiedNodeIdentity converts the publication credential binding to a VerifiedNodeIdentity.
func (b PublicationCredentialBinding) ToVerifiedNodeIdentity() VerifiedNodeIdentity {
	norm := b.Normalize()
	idVer := norm.IdentityVersion
	if idVer <= 0 {
		idVer = 1
	}
	return VerifiedNodeIdentity{
		LogicalID:       norm.LogicalID,
		Protocol:        norm.Protocol,
		Server:          norm.Server,
		Port:            norm.Port,
		Version:         norm.CredentialVersion,
		TransportDigest: norm.TransportDigest,
		IdentityVersion: idVer,
	}
}

// NewPublicationCredentialBindingFromPayload constructs a canonical binding from a verified credential payload.
func NewPublicationCredentialBindingFromPayload(payload *NodeCredentialPayload) (PublicationCredentialBinding, error) {
	if payload == nil {
		return PublicationCredentialBinding{}, NewValidationError("nil_credential_payload", "credential payload is nil")
	}
	identity := NewVerifiedNodeIdentity(
		payload.LogicalID,
		payload.Protocol,
		payload.Server,
		payload.Port,
		payload.Version,
		payload.Credentials.Transport,
	)
	if err := identity.ValidateNonEmpty(); err != nil {
		return PublicationCredentialBinding{}, err
	}
	if payload.Identity != nil {
		if err := payload.Identity.ValidateNonEmpty(); err != nil {
			return PublicationCredentialBinding{}, err
		}
		if payload.Identity.LogicalID != identity.LogicalID ||
			payload.Identity.Protocol != identity.Protocol ||
			payload.Identity.Version != identity.Version ||
			!payload.Identity.MatchesEndpoint(identity.Protocol, identity.Server, identity.Port) {
			return PublicationCredentialBinding{}, NewValidationError("identity_mismatch", "credential payload identity does not match payload fields")
		}
	}
	return PublicationCredentialBinding{
		LogicalID:         identity.LogicalID,
		Protocol:          identity.Protocol,
		Server:            identity.Server,
		Port:              identity.Port,
		CredentialVersion: identity.Version,
		TransportDigest:   identity.TransportDigest,
		IdentityVersion:   identity.IdentityVersion,
	}, nil
}

// ComputeCredentialBindingDigest normalizes, sorts, and JSON-marshals the bindings, returning
// the SHA-256 hex digest and the canonical JSON array representation.
func ComputeCredentialBindingDigest(bindings []PublicationCredentialBinding) (string, string, error) {
	normalized := make([]PublicationCredentialBinding, 0, len(bindings))
	for _, b := range bindings {
		norm := b.Normalize()
		if err := norm.Validate(); err != nil {
			return "", "", err
		}
		normalized = append(normalized, norm)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].LogicalID < normalized[j].LogicalID
	})
	for i := 1; i < len(normalized); i++ {
		if normalized[i].LogicalID == normalized[i-1].LogicalID {
			return "", "", NewValidationError("duplicate_publication_binding", fmt.Sprintf("duplicate node binding for %s", normalized[i].LogicalID))
		}
	}
	rawBytes, err := json.Marshal(normalized)
	if err != nil {
		return "", "", NewInternalError("binding_marshal_error", err.Error())
	}
	sum := sha256.Sum256(rawBytes)
	return hex.EncodeToString(sum[:]), string(rawBytes), nil
}

// ParseAndVerifyCredentialBindings parses persisted JSON bindings and verifies their digest.
func ParseAndVerifyCredentialBindings(rawJSON, expectedDigest string) ([]PublicationCredentialBinding, error) {
	trimmedJSON := strings.TrimSpace(rawJSON)
	trimmedDigest := strings.TrimSpace(expectedDigest)
	if trimmedJSON == "" || trimmedDigest == "" {
		return nil, NewValidationError("missing_publication_bindings", "publication is missing persisted credential bindings or digest")
	}
	var bindings []PublicationCredentialBinding
	if err := json.Unmarshal([]byte(trimmedJSON), &bindings); err != nil || bindings == nil {
		return nil, NewValidationError("invalid_publication_bindings_json", "persisted credential_bindings_json is malformed")
	}
	computedDigest, _, err := ComputeCredentialBindingDigest(bindings)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(computedDigest), []byte(trimmedDigest)) != 1 {
		return nil, NewValidationError("publication_binding_digest_mismatch", "persisted credential_binding_digest does not match credential_bindings_json")
	}
	return bindings, nil
}

// Publication represents a published configuration bundle export.
// It is immutable once compiled, but can be revoked by an administrator.
type Publication struct {
	ID                      string                         `json:"id"`
	RevisionID              string                         `json:"revision_id,omitempty"`
	Target                  CompilerTarget                 `json:"target"`
	SnapshotDigest          string                         `json:"snapshot_digest"`
	ContentDigest           string                         `json:"content_digest,omitempty"`
	CredentialBindingDigest string                         `json:"credential_binding_digest,omitempty"`
	CredentialBindingsJSON  string                         `json:"-"`
	CredentialBindings      []PublicationCredentialBinding `json:"credential_bindings,omitempty"`
	ContentType             string                         `json:"content_type,omitempty"`
	Filename                string                         `json:"filename,omitempty"`
	ArtifactKeyID           string                         `json:"-"`
	ArtifactNonce           []byte                         `json:"-"`
	ArtifactCiphertext      []byte                         `json:"-"`
	CompilerVersion         string                         `json:"compiler_version"`
	TokenHash               string                         `json:"token_hash"`
	State                   PublicationState               `json:"state"`
	CreatedAt               time.Time                      `json:"created_at"`
	RevokedAt               *time.Time                     `json:"revoked_at,omitempty"`
}

// IsActive returns whether the publication is active and serving requests.
func (p *Publication) IsActive() bool {
	return p.State == PublicationStateActive
}

// Revoke revokes the publication so that export endpoints will reject future access.
func (p *Publication) Revoke(now time.Time) error {
	if p.State == PublicationStateRevoked {
		return NewConflictError("already_revoked", fmt.Sprintf("publication %s is already revoked", p.ID))
	}
	utcNow := now.UTC()
	p.State = PublicationStateRevoked
	p.RevokedAt = &utcNow
	return nil
}

// PublicationArtifactAAD constructs the Associated Authenticated Data (AAD) binding
// publicationID + revisionID + target + snapshotDigest + credentialBindingDigest.
func PublicationArtifactAAD(publicationID, revisionID string, target CompilerTarget, snapshotDigest, credentialBindingDigest string) []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%s|%s",
		strings.TrimSpace(publicationID),
		strings.TrimSpace(revisionID),
		strings.TrimSpace(string(target)),
		strings.TrimSpace(snapshotDigest),
		strings.TrimSpace(credentialBindingDigest),
	))
}

// EncryptPublicationArtifact encrypts the compiled configuration artifact bytes using AES-GCM
// in the vault and populates ArtifactKeyID, ArtifactNonce, and ArtifactCiphertext on pub.
func (v *NodeCredentialVault) EncryptPublicationArtifact(pub *Publication, plaintext []byte) error {
	if v == nil {
		return NewValidationError("unsupported_target_capability", "publication artifact vault is nil (fail-closed)")
	}
	if pub == nil {
		return NewValidationError("nil_publication", "publication cannot be nil")
	}
	if strings.TrimSpace(pub.ID) == "" || !pub.Target.IsValid() || strings.TrimSpace(pub.SnapshotDigest) == "" || strings.TrimSpace(pub.CredentialBindingDigest) == "" {
		return NewValidationError("invalid_publication_aad_fields", "publication is missing required AAD fields for artifact encryption")
	}

	key, ok := v.keys[v.primaryKeyID]
	if !ok || len(key) != 32 {
		return NewSecurityError("key_not_found", "primary encryption key not found in vault")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return NewInternalError("aes_cipher_error", err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return NewInternalError("gcm_cipher_error", err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return NewInternalError("rand_nonce_error", err.Error())
	}

	aad := PublicationArtifactAAD(pub.ID, pub.RevisionID, pub.Target, pub.SnapshotDigest, pub.CredentialBindingDigest)
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	pub.ArtifactKeyID = v.primaryKeyID
	pub.ArtifactNonce = nonce
	pub.ArtifactCiphertext = ciphertext
	return nil
}

// DecryptPublicationArtifact authenticates and decrypts the persisted publication artifact ciphertext
// using the bound AAD (publicationID + revisionID + target + snapshotDigest + credentialBindingDigest)
// and verifies that the plaintext SHA-256 digest matches pub.ContentDigest.
func (v *NodeCredentialVault) DecryptPublicationArtifact(pub *Publication) ([]byte, error) {
	if v == nil {
		return nil, NewValidationError("publication_vault_unavailable", "vault is not configured to decrypt publication artifact")
	}
	if pub == nil {
		return nil, NewValidationError("nil_publication", "publication is nil")
	}
	if strings.TrimSpace(pub.ArtifactKeyID) == "" || len(pub.ArtifactNonce) == 0 || len(pub.ArtifactCiphertext) == 0 {
		return nil, NewValidationError("missing_publication_artifact", "publication is missing persisted encrypted artifact")
	}
	if strings.TrimSpace(pub.ContentDigest) == "" || strings.TrimSpace(pub.CredentialBindingDigest) == "" || strings.TrimSpace(pub.SnapshotDigest) == "" {
		return nil, NewValidationError("missing_publication_digests", "publication is missing required digests")
	}

	key, ok := v.keys[pub.ArtifactKeyID]
	if !ok || len(key) != 32 {
		return nil, NewValidationError("unknown_artifact_key_id", "decryption key for publication artifact not found in vault")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, NewInternalError("aes_cipher_error", err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, NewInternalError("gcm_cipher_error", err.Error())
	}

	if len(pub.ArtifactNonce) != gcm.NonceSize() {
		return nil, NewValidationError("invalid_artifact_nonce_size", "publication artifact nonce size is invalid")
	}

	aad := PublicationArtifactAAD(pub.ID, pub.RevisionID, pub.Target, pub.SnapshotDigest, pub.CredentialBindingDigest)
	plaintext, err := gcm.Open(nil, pub.ArtifactNonce, pub.ArtifactCiphertext, aad)
	if err != nil {
		return nil, NewValidationError("artifact_authentication_failed", "failed to authenticate or decrypt persisted publication artifact")
	}

	sum := sha256.Sum256(plaintext)
	actualDigest := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(actualDigest), []byte(strings.TrimSpace(pub.ContentDigest))) != 1 {
		return nil, NewValidationError("artifact_content_digest_mismatch", "decrypted publication artifact does not match persisted content_digest")
	}

	return plaintext, nil
}
