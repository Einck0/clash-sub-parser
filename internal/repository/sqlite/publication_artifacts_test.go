package sqlite_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestMigration000009_PublicationArtifactsPersistenceAndLegacyPreservation(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	applied, err := runner.AppliedVersions(ctx)
	if err != nil {
		t.Fatalf("failed to query applied versions: %v", err)
	}
	if _, ok := applied[9]; !ok {
		t.Fatalf("expected migration version 9 (000009_publication_artifacts.sql) to be applied, got %+v", applied)
	}

	pubRepo := sqlite.NewPublicationRepository(db)
	now := time.Now().UTC().Truncate(time.Second)

	// 1. Simulate legacy pre-000009 publication row (missing artifact and binding columns)
	legacyID := "0191e4a0-0000-7000-8000-000000000901"
	legacyTokenHash := "hash-legacy-mihomo-000009"
	_, err = db.ExecContext(ctx, `
		INSERT INTO publications (id, target, snapshot_digest, compiler_version, token_hash, state, created_at)
		VALUES (?, 'mihomo', 'sha256:legacy-snap', '1.0.0', ?, 'active', ?);
	`, legacyID, legacyTokenHash, now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to insert legacy publication row: %v", err)
	}

	legacyPub, err := pubRepo.GetByID(ctx, legacyID)
	if err != nil {
		t.Fatalf("GetByID failed for legacy publication: %v", err)
	}
	if legacyPub.ContentDigest != "" || legacyPub.CredentialBindingDigest != "" || legacyPub.CredentialBindingsJSON != "" || len(legacyPub.ArtifactCiphertext) != 0 {
		t.Fatalf("expected legacy publication row to have empty binding/artifact fields without auto-backfill, got %+v", legacyPub)
	}

	// 2. Persist modern publication with AEAD-encrypted artifact and credential bindings
	masterKey := []byte("01234567890123456789012345678901")
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": masterKey})
	if err != nil {
		t.Fatalf("failed to create test vault: %v", err)
	}

	nodeLogicalID := domain.ComputeNodeLogicalID(domain.ProtocolTrojan, "edge1.example.com", 443, nil)
	bindings := []domain.PublicationCredentialBinding{
		{
			LogicalID:         nodeLogicalID,
			Protocol:          domain.ProtocolTrojan,
			Server:            "edge1.example.com",
			Port:              443,
			CredentialVersion: 2,
			TransportDigest:   domain.CanonicalTransportDigest(nil),
			IdentityVersion:   1,
		},
	}
	credBindingDigest, credBindingsJSON, err := domain.ComputeCredentialBindingDigest(bindings)
	if err != nil {
		t.Fatalf("ComputeCredentialBindingDigest failed: %v", err)
	}

	artifactPlaintext := []byte("proxies:\n  - name: edge1\n    type: trojan\n    server: edge1.example.com\n    port: 443\n    password: secret-password\n")
	contentSum := sha256.Sum256(artifactPlaintext)
	contentDigest := hex.EncodeToString(contentSum[:])

	modernID := "0191e4a0-0000-7000-8000-000000000902"
	modernRevID := "0191e4a0-0000-7000-8000-000000000999"
	modernTokenHash := "hash-modern-mihomo-000009"
	modernPub := &domain.Publication{
		ID:                      modernID,
		RevisionID:              modernRevID,
		Target:                  domain.TargetMihomo,
		SnapshotDigest:          "sha256:modern-snap-000009",
		ContentDigest:           contentDigest,
		CredentialBindingDigest: credBindingDigest,
		CredentialBindingsJSON:  credBindingsJSON,
		CredentialBindings:      bindings,
		ContentType:             "application/x-yaml",
		Filename:                "mihomo.yaml",
		CompilerVersion:         "1.0.0",
		TokenHash:               modernTokenHash,
		State:                   domain.PublicationStateActive,
		CreatedAt:               now,
	}
	if err := vault.EncryptPublicationArtifact(modernPub, artifactPlaintext); err != nil {
		t.Fatalf("EncryptPublicationArtifact failed: %v", err)
	}
	if bytes.Contains(modernPub.ArtifactCiphertext, []byte("secret-password")) {
		t.Fatal("ArtifactCiphertext must not contain plaintext secrets")
	}

	if err := pubRepo.Create(ctx, modernPub); err != nil {
		t.Fatalf("Create modern publication failed: %v", err)
	}

	// 3. Round-trip via GetByID and GetByTokenHash
	loadedByID, err := pubRepo.GetByID(ctx, modernID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	loadedByToken, err := pubRepo.GetByTokenHash(ctx, modernTokenHash)
	if err != nil {
		t.Fatalf("GetByTokenHash failed: %v", err)
	}

	for _, loaded := range []*domain.Publication{loadedByID, loadedByToken} {
		if loaded.RevisionID != modernRevID {
			t.Fatalf("expected RevisionID %s, got %s", modernRevID, loaded.RevisionID)
		}
		if loaded.ContentDigest != contentDigest {
			t.Fatalf("expected ContentDigest %s, got %s", contentDigest, loaded.ContentDigest)
		}
		if loaded.CredentialBindingDigest != credBindingDigest {
			t.Fatalf("expected CredentialBindingDigest %s, got %s", credBindingDigest, loaded.CredentialBindingDigest)
		}
		if loaded.CredentialBindingsJSON != credBindingsJSON {
			t.Fatalf("expected CredentialBindingsJSON %s, got %s", credBindingsJSON, loaded.CredentialBindingsJSON)
		}
		if len(loaded.CredentialBindings) != 1 || loaded.CredentialBindings[0].LogicalID != nodeLogicalID || loaded.CredentialBindings[0].CredentialVersion != 2 {
			t.Fatalf("unexpected loaded CredentialBindings: %+v", loaded.CredentialBindings)
		}
		if loaded.ContentType != "application/x-yaml" || loaded.Filename != "mihomo.yaml" {
			t.Fatalf("unexpected content_type/filename: %s / %s", loaded.ContentType, loaded.Filename)
		}
		if loaded.ArtifactKeyID != "k1" || len(loaded.ArtifactNonce) == 0 || len(loaded.ArtifactCiphertext) == 0 {
			t.Fatalf("missing persisted encrypted artifact fields: key=%s nonce=%d cipher=%d", loaded.ArtifactKeyID, len(loaded.ArtifactNonce), len(loaded.ArtifactCiphertext))
		}

		decrypted, err := vault.DecryptPublicationArtifact(loaded)
		if err != nil {
			t.Fatalf("DecryptPublicationArtifact failed: %v", err)
		}
		if !bytes.Equal(decrypted, artifactPlaintext) {
			t.Fatalf("decrypted artifact mismatch")
		}
	}

	// 4. Verify AAD binding tampering detection (publicationID, revisionID, target, snapshotDigest, credentialBindingDigest, ciphertext)
	t.Run("TamperDetection", func(t *testing.T) {
		tamperCases := []struct {
			name   string
			mutate func(p *domain.Publication)
		}{
			{
				name:   "tamper_publication_id",
				mutate: func(p *domain.Publication) { p.ID = "0191e4a0-0000-7000-8000-000000000998" },
			},
			{
				name:   "tamper_revision_id",
				mutate: func(p *domain.Publication) { p.RevisionID = "0191e4a0-0000-7000-8000-000000000888" },
			},
			{
				name:   "tamper_target",
				mutate: func(p *domain.Publication) { p.Target = domain.TargetSingBox },
			},
			{
				name:   "tamper_snapshot_digest",
				mutate: func(p *domain.Publication) { p.SnapshotDigest = "sha256:tampered-snap" },
			},
			{
				name:   "tamper_credential_binding_digest",
				mutate: func(p *domain.Publication) { p.CredentialBindingDigest = strings.Repeat("a", 64) },
			},
			{
				name:   "tamper_content_digest",
				mutate: func(p *domain.Publication) { p.ContentDigest = strings.Repeat("b", 64) },
			},
			{
				name:   "tamper_ciphertext_byte",
				mutate: func(p *domain.Publication) { p.ArtifactCiphertext[0] ^= 0xff },
			},
		}

		for _, tc := range tamperCases {
			t.Run(tc.name, func(t *testing.T) {
				cloned := *loadedByID
				cloned.ArtifactNonce = append([]byte(nil), loadedByID.ArtifactNonce...)
				cloned.ArtifactCiphertext = append([]byte(nil), loadedByID.ArtifactCiphertext...)
				tc.mutate(&cloned)
				if _, err := vault.DecryptPublicationArtifact(&cloned); err == nil {
					t.Fatalf("expected DecryptPublicationArtifact to fail closed on %s", tc.name)
				}
			})
		}
	})
}
