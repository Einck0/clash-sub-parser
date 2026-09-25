package inventory_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

type staticFetcher struct {
	body []byte
}

func (f *staticFetcher) Fetch(ctx context.Context, opts fetch.Options) (*fetch.Response, error) {
	return &fetch.Response{
		StatusCode:    200,
		Body:          f.body,
		ContentDigest: "test-content-digest-123",
	}, nil
}

func TestReconcileWithCredentialVaultEncryptsAndStores(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	key := make([]byte, 32)
	rand.Read(key)
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key})
	if err != nil {
		t.Fatal(err)
	}
	credRepo := sqlite.NewNodeCredentialRepository(db)

	yamlContent := []byte(`
proxies:
  - name: SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: my-secret-ss-password
`)

	fetcher := &staticFetcher{body: yamlContent}
	service := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault, credRepo),
	)

	sub := &domain.Subscription{
		ID:                 "sub-test-vault-1",
		Name:               "Vault Test Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}

	result, err := service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ReconcileSubscription failed: %v", err)
	}
	if result.NodesValid != 1 {
		t.Fatalf("expected 1 valid node, got %d", result.NodesValid)
	}

	// Verify node exists
	nodes, count, err := nodeRepo.List(ctx, domain.NodeFilter{})
	if err != nil || count != 1 {
		t.Fatalf("list nodes failed: count=%d, err=%v", count, err)
	}
	node := nodes[0]

	// Check DB row directly: plaintext password must NOT exist in the whole database
	var plainCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_credentials WHERE hex(ciphertext) LIKE '%6d792d736563726574%';").Scan(&plainCount)
	if err != nil {
		t.Fatal(err)
	}
	// "my-secret" in hex is 6d792d736563726574 - verify no plaintext leak in ciphertext
	if plainCount != 0 {
		t.Fatalf("plaintext leaked into ciphertext column!")
	}

	// Verify decrypted credential
	rec, err := credRepo.GetByLogicalID(ctx, node.LogicalID, 1)
	if err != nil {
		t.Fatalf("GetByLogicalID failed: %v", err)
	}
	if rec.KeyID != "k1" {
		t.Fatalf("KeyID mismatch: %s", rec.KeyID)
	}

	payload, err := vault.Decrypt(rec, domain.ProtocolSS)
	if err != nil {
		t.Fatalf("vault.Decrypt failed: %v", err)
	}
	if payload.Credentials.Password != "my-secret-ss-password" {
		t.Fatalf("decrypted password mismatch: %s", payload.Credentials.Password)
	}
	if payload.Credentials.Method != "aes-128-gcm" {
		t.Fatalf("decrypted method mismatch: %s", payload.Credentials.Method)
	}

	// Verify node row itself has credential_version == 1
	dbNode, err := nodeRepo.GetByLogicalID(ctx, node.LogicalID)
	if err != nil {
		t.Fatalf("GetByLogicalID failed: %v", err)
	}
	if dbNode.CredentialVersion != 1 {
		t.Fatalf("expected node.CredentialVersion == 1, got %d", dbNode.CredentialVersion)
	}
}

func TestReconcileOrphanDeactivationCleansCredentials(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	key := make([]byte, 32)
	rand.Read(key)
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key})
	if err != nil {
		t.Fatal(err)
	}
	credRepo := sqlite.NewNodeCredentialRepository(db)

	fetcher := &staticFetcher{body: []byte(`
proxies:
  - name: SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: my-secret-ss-password
`)}
	service := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault, credRepo),
	)

	sub := &domain.Subscription{
		ID:                 "sub-test-orphan",
		Name:               "Orphan Test Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}

	// 1st reconcile - node and credentials exist
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	nodes, _, _ := nodeRepo.List(ctx, domain.NodeFilter{})
	nodeID := nodes[0].LogicalID

	if _, err := credRepo.GetByLogicalID(ctx, nodeID, 1); err != nil {
		t.Fatalf("credential should exist: %v", err)
	}

	// 2nd reconcile - source now has a different node, previous node becomes orphan
	fetcher.body = []byte(`
proxies:
  - name: Other Node
    type: ss
    server: 198.51.100.2
    port: 8388
    cipher: aes-128-gcm
    password: other-secret-password
`)
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Previous node should now be deactivated
	deactivatedNode, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if deactivatedNode.Active {
		t.Fatalf("expected previous node to be deactivated")
	}

	// Credentials for deactivated node should be cleaned up
	if _, err := credRepo.GetByLogicalID(ctx, nodeID, 1); err == nil {
		t.Fatalf("expected credentials of orphan node to be deleted")
	}
}

func TestReconcileWithCorruptedCiphertextRollsBackAndFails(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	key := make([]byte, 32)
	rand.Read(key)
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key})
	if err != nil {
		t.Fatal(err)
	}
	credRepo := sqlite.NewNodeCredentialRepository(db)

	yamlContent := []byte(`
proxies:
  - name: SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: my-secret-ss-password
`)

	fetcher := &staticFetcher{body: yamlContent}
	service := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault, credRepo),
	)

	sub := &domain.Subscription{
		ID:                 "sub-test-corrupt-vault",
		Name:               "Corrupt Vault Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}

	// 1st reconcile succeeds
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	nodes, _, _ := nodeRepo.List(ctx, domain.NodeFilter{})
	nodeID := nodes[0].LogicalID

	// Corrupt the ciphertext in the database directly
	_, err = db.ExecContext(ctx, "UPDATE node_credentials SET ciphertext = x'deadbeef' WHERE logical_id = ? AND version = 1;", nodeID)
	if err != nil {
		t.Fatalf("failed to corrupt ciphertext: %v", err)
	}

	// 2nd reconcile with same or modified content - should fail closed and rollback
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected reconcile to fail due to corrupted ciphertext, but got nil error")
	}

	// Check that the error message is present and does not leak raw secrets
	errMsg := err.Error()
	if !contains(errMsg, "failed to decrypt existing credentials") {
		t.Fatalf("expected error to indicate decryption failure, got: %s", errMsg)
	}

	// Verify rollback: no version 2 row was created
	recV2, _ := credRepo.GetByLogicalID(ctx, nodeID, 2)
	if recV2 != nil {
		t.Fatalf("expected transaction rollback, but version 2 credential record was persisted")
	}

	// Verify subscription_fetches does not have a commit for this 2nd reconcile
	var fetchCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_fetches WHERE subscription_id = ?;", sub.ID).Scan(&fetchCount)
	if err != nil {
		t.Fatal(err)
	}
	if fetchCount != 1 {
		t.Fatalf("expected exactly 1 fetch record (from first run), got %d (transaction did not rollback)", fetchCount)
	}
}

func TestReconcileWithWrongKeyRollsBackAndFails(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	key1 := make([]byte, 32)
	rand.Read(key1)
	vault1, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key1})
	if err != nil {
		t.Fatal(err)
	}
	credRepo := sqlite.NewNodeCredentialRepository(db)

	yamlContent := []byte(`
proxies:
  - name: SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: my-secret-ss-password
`)

	fetcher := &staticFetcher{body: yamlContent}
	service1 := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault1, credRepo),
	)

	sub := &domain.Subscription{
		ID:                 "sub-test-wrong-key",
		Name:               "Wrong Key Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}

	// 1st reconcile with key1 succeeds
	_, err = service1.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	nodes, _, _ := nodeRepo.List(ctx, domain.NodeFilter{})
	nodeID := nodes[0].LogicalID

	// Create service2 with vault that does NOT have key1 (e.g. wrong key / rotation without old key)
	key2 := make([]byte, 32)
	rand.Read(key2)
	vault2, err := domain.NewNodeCredentialVault("k2", map[string][]byte{"k2": key2})
	if err != nil {
		t.Fatal(err)
	}
	service2 := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault2, credRepo),
	)

	// 2nd reconcile - should fail closed because key1 cannot be found or decrypted
	_, err = service2.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected reconcile to fail due to wrong key, but got nil error")
	}

	if !contains(err.Error(), "failed to decrypt existing credentials") {
		t.Fatalf("expected error to mention failed to decrypt, got: %s", err.Error())
	}

	// Verify rollback: no version 2 row was created
	recV2, _ := credRepo.GetByLogicalID(ctx, nodeID, 2)
	if recV2 != nil {
		t.Fatalf("expected transaction rollback, but version 2 credential record was persisted")
	}

	// Verify fetch count rolled back to 1
	var fetchCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_fetches WHERE subscription_id = ?;", sub.ID).Scan(&fetchCount)
	if err != nil {
		t.Fatal(err)
	}
	if fetchCount != 1 {
		t.Fatalf("expected exactly 1 fetch record, got %d", fetchCount)
	}
}

func TestCredentialVersionIncrementAndWatermarkReconcile(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	key := make([]byte, 32)
	rand.Read(key)
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key})
	if err != nil {
		t.Fatal(err)
	}
	credRepo := sqlite.NewNodeCredentialRepository(db)

	fetcher := &staticFetcher{}
	service := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault, credRepo),
	)

	sub := &domain.Subscription{
		ID:                 "sub-test-versioning",
		Name:               "Versioning Test Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}

	// 1. Initial refresh: node at v1
	fetcher.body = []byte(`
proxies:
  - name: SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: password-v1
`)
	res1, err := service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}
	if res1.NodesValid != 1 {
		t.Fatalf("expected 1 valid node, got %d", res1.NodesValid)
	}

	nodes, _, err := nodeRepo.List(ctx, domain.NodeFilter{})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	nodeID := nodes[0].LogicalID

	n1, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n1.CredentialVersion != 1 {
		t.Fatalf("expected initial CredentialVersion == 1, got %d", n1.CredentialVersion)
	}
	c1, err := credRepo.GetByLogicalID(ctx, nodeID, 1)
	if err != nil || c1 == nil {
		t.Fatalf("expected cred record v1 to exist: %v", err)
	}

	// 2. Re-refresh with same connection credentials but DIFFERENT display_name / subscription content
	// Single node credentials/server/port unchanged -> version MUST NOT increment, stays 1
	fetcher.body = []byte(`
proxies:
  - name: Renamed SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: password-v1
`)
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	n2, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n2.CredentialVersion != 1 {
		t.Fatalf("expected CredentialVersion to remain 1 after rename/content digest change, got %d", n2.CredentialVersion)
	}
	if n2.DisplayName != "Renamed SS Node" {
		t.Fatalf("expected DisplayName updated to Renamed SS Node, got %s", n2.DisplayName)
	}

	// 3. Password changed -> single node credential changed -> CredentialVersion increments to 2
	fetcher.body = []byte(`
proxies:
  - name: Renamed SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: password-v2
`)
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("third reconcile failed: %v", err)
	}

	n3, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n3.CredentialVersion != 2 {
		t.Fatalf("expected CredentialVersion to increment to 2, got %d", n3.CredentialVersion)
	}
	c2, err := credRepo.GetByLogicalID(ctx, nodeID, 2)
	if err != nil || c2 == nil {
		t.Fatalf("expected cred record v2 to exist: %v", err)
	}
	// Verify decrypted content of v2
	p2, err := vault.Decrypt(c2, domain.ProtocolSS)
	if err != nil || p2.Credentials.Password != "password-v2" {
		t.Fatalf("decrypted v2 payload mismatch: %+v, err: %v", p2, err)
	}

	// 4. Server/port changed: since logical_id is bound to (protocol, server, port), changing server or port
	// would produce a new node logical_id. Let's test cipher changed on same node:
	fetcher.body = []byte(`
proxies:
  - name: Renamed SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-256-gcm
    password: password-v2
`)
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("fourth reconcile failed: %v", err)
	}
	n4, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n4.CredentialVersion != 3 {
		t.Fatalf("expected CredentialVersion to increment to 3 after cipher change, got %d", n4.CredentialVersion)
	}

	// 5. Deactivation/Revocation and High-watermark test:
	// Subscription replaces node with another one -> previous node becomes orphan (deactivated)
	// and its credentials deleted.
	fetcher.body = []byte(`
proxies:
  - name: Brand New Node
    type: ss
    server: 198.51.100.99
    port: 8388
    cipher: aes-128-gcm
    password: pass
`)
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("fifth reconcile failed: %v", err)
	}

	nDeactivated, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if nDeactivated.Active {
		t.Fatalf("expected node to be deactivated")
	}
	// High watermark version in nodes table must be retained (version 3)
	if nDeactivated.CredentialVersion != 3 {
		t.Fatalf("expected deactivated node to retain high-watermark version 3, got %d", nDeactivated.CredentialVersion)
	}
	// Credentials in node_credentials table were cleaned up
	if _, err := credRepo.GetByLogicalID(ctx, nodeID, 3); err == nil {
		t.Fatalf("expected credentials of deactivated node to be cleaned up")
	}

	// 6. Node re-appears (authorized fresh re-fetch):
	// Must NOT reuse version 1, 2, or 3. Must increment past high watermark (to 4)!
	fetcher.body = []byte(`
proxies:
  - name: Revived SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: password-v4
`)
	_, err = service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("sixth reconcile failed: %v", err)
	}

	nRevived, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !nRevived.Active {
		t.Fatalf("expected revived node to be active")
	}
	if nRevived.CredentialVersion <= 3 {
		t.Fatalf("expected revived node CredentialVersion > 3 (high watermark non-reusable), got %d", nRevived.CredentialVersion)
	}
	if nRevived.CredentialVersion != 4 {
		t.Fatalf("expected revived node CredentialVersion == 4, got %d", nRevived.CredentialVersion)
	}
	cRevived, err := credRepo.GetByLogicalID(ctx, nodeID, 4)
	if err != nil || cRevived == nil {
		t.Fatalf("expected cred record v4 to exist: %v", err)
	}
}

func TestLegacyNodeDefaultZeroAndFreshAuthorizationReconcile(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	key := make([]byte, 32)
	rand.Read(key)
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key})
	if err != nil {
		t.Fatal(err)
	}
	credRepo := sqlite.NewNodeCredentialRepository(db)

	fetcher := &staticFetcher{}
	service := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault, credRepo),
	)

	nowStr := domain.NowUTC().Format(time.RFC3339)
	legacyNodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.1", 8388, map[string]string{"network": "tcp"})

	// Directly insert legacy node into database with credential_version = 0 (or default 0 from legacy schema)
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, 1, ?, ?);
	`, legacyNodeID, "ss", "Legacy Node Default Zero", "ref-legacy", nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to insert legacy node: %v", err)
	}

	// Verify legacy node reads as credential_version == 0
	legacyNode, err := nodeRepo.GetByLogicalID(ctx, legacyNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if legacyNode.CredentialVersion != 0 {
		t.Fatalf("expected legacy node CredentialVersion == 0, got %d", legacyNode.CredentialVersion)
	}
	// And no credentials exist for it
	if _, err := credRepo.GetByLogicalID(ctx, legacyNodeID, 1); err == nil {
		t.Fatalf("expected no credentials for legacy node")
	}

	// Now authorize fresh subscription refresh containing this node with credentials
	sub := &domain.Subscription{
		ID:                 "sub-test-legacy-upgrade",
		Name:               "Legacy Upgrade Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}

	fetcher.body = []byte(`
proxies:
  - name: Fresh Authorized Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: fresh-authorized-password
`)
	res, err := service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if res.NodesValid != 1 {
		t.Fatalf("expected 1 valid node, got %d", res.NodesValid)
	}

	// Legacy node should now be upgraded and bound to credential_version == 1
	upgradedNode, err := nodeRepo.GetByLogicalID(ctx, legacyNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if upgradedNode.CredentialVersion != 1 {
		t.Fatalf("expected upgraded node CredentialVersion == 1, got %d", upgradedNode.CredentialVersion)
	}

	// Encrypted credential version 1 must now exist and decrypt correctly
	credRec, err := credRepo.GetByLogicalID(ctx, legacyNodeID, 1)
	if err != nil || credRec == nil {
		t.Fatalf("expected cred record v1 to exist: %v", err)
	}
	payload, err := vault.Decrypt(credRec, domain.ProtocolSS)
	if err != nil {
		t.Fatalf("failed to decrypt upgraded credentials: %v", err)
	}
	if payload.Credentials.Password != "fresh-authorized-password" {
		t.Fatalf("expected decrypted password 'fresh-authorized-password', got %s", payload.Credentials.Password)
	}
}

func TestMultiSourceConsistentConfigSharesVersion(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	key := make([]byte, 32)
	rand.Read(key)
	vault, err := domain.NewNodeCredentialVault("k1", map[string][]byte{"k1": key})
	if err != nil {
		t.Fatal(err)
	}
	credRepo := sqlite.NewNodeCredentialRepository(db)

	fetcher := &mockFetcher{
		responses: make(map[string]*fetch.Response),
		errors:    make(map[string]error),
	}
	service := inventory.NewService(
		db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher,
		inventory.WithCredentialVault(vault, credRepo),
	)

	sub1ID := "sub-source-1"
	url1 := "http://example.com/sub1"
	sub1 := &domain.Subscription{
		ID:                 sub1ID,
		Name:               "Source 1",
		SourceURLSecretRef: url1,
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub1); err != nil {
		t.Fatal(err)
	}

	sub2ID := "sub-source-2"
	url2 := "http://example.com/sub2"
	sub2 := &domain.Subscription{
		ID:                 sub2ID,
		Name:               "Source 2",
		SourceURLSecretRef: url2,
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub2); err != nil {
		t.Fatal(err)
	}

	// Sub1 has Node Alpha with password "common-pass"
	fetcher.setResponse(url1, &fetch.Response{
		StatusCode:    200,
		Body: []byte(`
proxies:
  - name: Alpha Sub1
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: common-pass
`),
		ContentDigest: "digest-sub1",
	})

	// Sub2 also has Node Alpha with the SAME server, port, cipher, password, but different display name & digest
	fetcher.setResponse(url2, &fetch.Response{
		StatusCode:    200,
		Body: []byte(`
proxies:
  - name: Alpha Sub2 Different Name
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: common-pass
`),
		ContentDigest: "digest-sub2-different",
	})

	// Reconcile Sub1
	if _, err := service.ReconcileSubscription(ctx, sub1ID); err != nil {
		t.Fatalf("sub1 reconcile failed: %v", err)
	}

	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.1", 8388, map[string]string{"network": "tcp"})
	n1, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n1.CredentialVersion != 1 {
		t.Fatalf("expected version 1 after sub1, got %d", n1.CredentialVersion)
	}

	// Reconcile Sub2: both sources provide IDENTICAL connection configuration
	// Must share version 1 and NOT bump version!
	if _, err := service.ReconcileSubscription(ctx, sub2ID); err != nil {
		t.Fatalf("sub2 reconcile failed: %v", err)
	}

	n2, err := nodeRepo.GetByLogicalID(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if n2.CredentialVersion != 1 {
		t.Fatalf("expected version to remain 1 after consistent multi-source merge, got %d", n2.CredentialVersion)
	}

	// Provenance must show both sources
	sources, err := nodeSourceRepo.ListByNode(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources for merged node, got %d", len(sources))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > 0 && len(substr) > 0 && searchSubstr(s, substr)))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

