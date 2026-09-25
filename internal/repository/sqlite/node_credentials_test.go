package sqlite_test

import (
	"context"
	"crypto/rand"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestNodeCredentialRepositoryLifecycle(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	nodeRepo := sqlite.NewNodeRepository(db)
	credRepo := sqlite.NewNodeCredentialRepository(db)

	nodeID := "0191e4a0-0000-7000-8000-000000000001"
	node := domain.Node{
		LogicalID:                 nodeID,
		Protocol:                  domain.ProtocolSS,
		DisplayName:               "SS Node",
		NormalizedConfigSecretRef: "secret_ref_1",
		Active:                    true,
		CreatedAt:                 domain.NowUTC(),
		UpdatedAt:                 domain.NowUTC(),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatalf("upsert node: %v", err)
	}

	nonce := make([]byte, 12)
	rand.Read(nonce)
	ciphertext := []byte("encrypted-secret-payload-v1")

	recordV1 := &domain.NodeCredentialRecord{
		LogicalID:  nodeID,
		Version:    1,
		KeyID:      "k1",
		Nonce:      nonce,
		Ciphertext: ciphertext,
		CreatedAt:  domain.NowUTC(),
		UpdatedAt:  domain.NowUTC(),
	}

	// 1. Upsert V1
	if err := credRepo.Upsert(ctx, recordV1); err != nil {
		t.Fatalf("upsert credential: %v", err)
	}

	// 2. Query V1
	gotV1, err := credRepo.GetByLogicalID(ctx, nodeID, 1)
	if err != nil {
		t.Fatalf("get credential v1: %v", err)
	}
	if gotV1.KeyID != "k1" || string(gotV1.Ciphertext) != "encrypted-secret-payload-v1" {
		t.Fatalf("unexpected record v1: %+v", gotV1)
	}

	// 3. Upsert V2
	recordV2 := &domain.NodeCredentialRecord{
		LogicalID:  nodeID,
		Version:    2,
		KeyID:      "k2",
		Nonce:      nonce,
		Ciphertext: []byte("encrypted-secret-payload-v2"),
		CreatedAt:  domain.NowUTC(),
		UpdatedAt:  domain.NowUTC(),
	}
	if err := credRepo.Upsert(ctx, recordV2); err != nil {
		t.Fatalf("upsert credential v2: %v", err)
	}

	// 4. Query Latest should return V2
	latest, err := credRepo.GetLatestByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatalf("get latest credential: %v", err)
	}
	if latest.Version != 2 || latest.KeyID != "k2" || string(latest.Ciphertext) != "encrypted-secret-payload-v2" {
		t.Fatalf("unexpected latest record: %+v", latest)
	}

	// 5. Query non-existent returns not found
	if _, err := credRepo.GetByLogicalID(ctx, "nonexistent", 1); err == nil {
		t.Fatalf("expected not found for nonexistent node")
	}

	// 6. Delete credentials
	if err := credRepo.DeleteByLogicalID(ctx, nodeID); err != nil {
		t.Fatalf("delete credentials: %v", err)
	}
	if _, err := credRepo.GetLatestByLogicalID(ctx, nodeID); err == nil {
		t.Fatalf("expected not found after deletion")
	}
}

func TestNodeCredentialCascadeOnNodeDelete(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	nodeRepo := sqlite.NewNodeRepository(db)
	credRepo := sqlite.NewNodeCredentialRepository(db)

	nodeID := "0191e4a0-0000-7000-8000-000000000002"
	node := domain.Node{
		LogicalID:                 nodeID,
		Protocol:                  domain.ProtocolVMess,
		DisplayName:               "VMess Node",
		NormalizedConfigSecretRef: "secret_ref_2",
		Active:                    true,
		CreatedAt:                 domain.NowUTC(),
		UpdatedAt:                 domain.NowUTC(),
	}
	if err := nodeRepo.UpsertBatch(ctx, []domain.Node{node}); err != nil {
		t.Fatalf("upsert node: %v", err)
	}

	record := &domain.NodeCredentialRecord{
		LogicalID:  nodeID,
		Version:    1,
		KeyID:      "k1",
		Nonce:      make([]byte, 12),
		Ciphertext: []byte("encrypted"),
		CreatedAt:  domain.NowUTC(),
		UpdatedAt:  domain.NowUTC(),
	}
	if err := credRepo.Upsert(ctx, record); err != nil {
		t.Fatalf("upsert credential: %v", err)
	}

	// Delete node directly from DB
	if _, err := db.ExecContext(ctx, "DELETE FROM nodes WHERE logical_id = ?;", nodeID); err != nil {
		t.Fatalf("delete node: %v", err)
	}

	// Credentials should cascade delete
	if _, err := credRepo.GetByLogicalID(ctx, nodeID, 1); err == nil {
		t.Fatalf("expected credential to be deleted via cascade")
	}
}
