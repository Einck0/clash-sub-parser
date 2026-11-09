package sqlite_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func createTestSubscription(t *testing.T, db *sql.DB, subRepo domain.SubscriptionRepository, id, name, url string) *domain.Subscription {
	t.Helper()
	sub := &domain.Subscription{
		ID:                 id,
		Name:               name,
		SourceURLSecretRef: url,
		Enabled:            true,
		Revision:           "rev-init-1",
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds: 3600,
		},
		Config: domain.SubscriptionConfig{},
	}
	if err := subRepo.Create(context.Background(), sub); err != nil {
		t.Fatalf("failed to create test subscription %s: %v", id, err)
	}
	return sub
}

func TestSubscriptionOriginalRepository_NotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	// 1. Non-existent subscription
	_, err := origRepo.GetBySubscriptionID(ctx, "sub-non-existent")
	if err == nil {
		t.Fatalf("expected error for non-existent subscription, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) {
		t.Fatalf("expected *domain.DomainError, got %T: %v", err, err)
	}
	if domErr.Code != "subscription_not_found" {
		t.Fatalf("expected code subscription_not_found, got %q", domErr.Code)
	}
	if domErr.Category != domain.CategoryNotFound {
		t.Fatalf("expected CategoryNotFound, got %q", domErr.Category)
	}
	if domErr.Message != "subscription not found" {
		t.Fatalf("expected generic message %q, got %q", "subscription not found", domErr.Message)
	}

	// 2. Existing subscription without original config
	createTestSubscription(t, db, subRepo, "sub-exists-no-config", "No Config Sub", "https://example.com/no-config")
	_, err = origRepo.GetBySubscriptionID(ctx, "sub-exists-no-config")
	if err == nil {
		t.Fatalf("expected error for subscription with no original config, got nil")
	}
	if !errors.As(err, &domErr) {
		t.Fatalf("expected *domain.DomainError, got %T: %v", err, err)
	}
	if domErr.Code != "original_config_not_found" {
		t.Fatalf("expected code original_config_not_found, got %q", domErr.Code)
	}
	if domErr.Category != domain.CategoryNotFound {
		t.Fatalf("expected CategoryNotFound, got %q", domErr.Category)
	}
	if domErr.Message != "subscription original configuration not found" {
		t.Fatalf("expected generic message %q, got %q", "subscription original configuration not found", domErr.Message)
	}
}

func TestSubscriptionOriginalRepository_UpsertAndGet(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-upsert-1", "Upsert Sub", "https://example.com/upsert")

	rawBody := []byte("proxies:\n  - name: node-alpha\n    type: ss\n    server: 1.2.3.4\n    port: 8388\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(rawBody))
	fetchedAt := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	fetchID := "fetch-test-123"

	origCfg := &domain.SubscriptionOriginalConfig{
		SubscriptionID: sub.ID,
		ContentDigest:  digest,
		Body:           rawBody,
		FetchedAt:      fetchedAt,
		ContentType:    "application/yaml",
		FetchID:        fetchID,
	}

	err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, origCfg, sub.Revision)
	})
	if err != nil {
		t.Fatalf("UpsertOriginalConfigTx failed: %v", err)
	}

	got, err := origRepo.GetBySubscriptionID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetBySubscriptionID failed: %v", err)
	}
	if got.SubscriptionID != sub.ID {
		t.Errorf("SubscriptionID mismatch: expected %q, got %q", sub.ID, got.SubscriptionID)
	}
	if got.ContentDigest != digest {
		t.Errorf("ContentDigest mismatch: expected %q, got %q", digest, got.ContentDigest)
	}
	if !bytes.Equal(got.Body, rawBody) {
		t.Errorf("Body mismatch: expected %q, got %q", string(rawBody), string(got.Body))
	}
	if got.FetchID != fetchID {
		t.Errorf("FetchID mismatch: expected %q, got %q", fetchID, got.FetchID)
	}
	if got.ContentType != "application/yaml" {
		t.Errorf("ContentType mismatch: expected application/yaml, got %q", got.ContentType)
	}
	if got.FetchedAt.Unix() != fetchedAt.Unix() {
		t.Errorf("FetchedAt mismatch: expected %v, got %v", fetchedAt, got.FetchedAt)
	}

	// Secret-bearing JSON redaction check: Body and ContentType must NOT leak in JSON serialization
	jsonBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(jsonBytes)
	if bytes.Contains(jsonBytes, rawBody) || bytes.Contains(jsonBytes, []byte("node-alpha")) {
		t.Fatalf("raw body leaked in JSON serialization: %s", jsonStr)
	}
	if bytes.Contains(jsonBytes, []byte("application/yaml")) {
		t.Fatalf("raw content_type leaked in JSON serialization: %s", jsonStr)
	}
}

func TestSubscriptionOriginalRepository_OneRowReplacement(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-replace-1", "Replace Sub", "https://example.com/replace")

	body1 := []byte("proxies:\n  - name: version-1\n")
	orig1 := &domain.SubscriptionOriginalConfig{
		SubscriptionID: sub.ID,
		ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(body1)),
		Body:           body1,
		FetchedAt:      time.Now().Add(-1 * time.Hour),
		ContentType:    "text/plain",
		FetchID:        "fetch-1",
	}
	err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, orig1, "rev-1")
	})
	if err != nil {
		t.Fatalf("initial upsert failed: %v", err)
	}

	body2 := []byte("proxies:\n  - name: version-2\n")
	orig2 := &domain.SubscriptionOriginalConfig{
		SubscriptionID: sub.ID,
		ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(body2)),
		Body:           body2,
		FetchedAt:      time.Now(),
		ContentType:    "text/plain",
		FetchID:        "fetch-2",
	}
	err = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, orig2, "rev-2")
	})
	if err != nil {
		t.Fatalf("replacement upsert failed: %v", err)
	}

	// Verify only body2 is present
	got, err := origRepo.GetBySubscriptionID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetBySubscriptionID failed: %v", err)
	}
	if !bytes.Equal(got.Body, body2) {
		t.Fatalf("expected body2, got %q", string(got.Body))
	}
	if got.FetchID != "fetch-2" {
		t.Fatalf("expected fetch-2, got %q", got.FetchID)
	}

	// Verify exactly 1 row exists in table for this subscription
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_original_configs WHERE subscription_id = ?;", sub.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row in subscription_original_configs, got %d", count)
	}
}

func TestSubscriptionOriginalRepository_CrossSubscriptionIsolation(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	subA := createTestSubscription(t, db, subRepo, "sub-alpha", "Sub A", "https://example.com/a")
	subB := createTestSubscription(t, db, subRepo, "sub-beta", "Sub B", "https://example.com/b")

	bodyA := []byte("proxies:\n  - name: node-from-sub-A\n")
	bodyB := []byte("proxies:\n  - name: node-from-sub-B\n")

	_ = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: subA.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(bodyA)),
			Body:           bodyA,
			FetchedAt:      time.Now(),
			ContentType:    "application/x-yaml",
			FetchID:        "fetch-a",
		}, "rev-a")
	})
	_ = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: subB.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(bodyB)),
			Body:           bodyB,
			FetchedAt:      time.Now(),
			ContentType:    "application/x-yaml",
			FetchID:        "fetch-b",
		}, "rev-b")
	})

	gotA, err := origRepo.GetBySubscriptionID(ctx, subA.ID)
	if err != nil || !bytes.Equal(gotA.Body, bodyA) {
		t.Fatalf("failed to isolate subA config: err=%v, body=%q", err, string(gotA.Body))
	}

	gotB, err := origRepo.GetBySubscriptionID(ctx, subB.ID)
	if err != nil || !bytes.Equal(gotB.Body, bodyB) {
		t.Fatalf("failed to isolate subB config: err=%v, body=%q", err, string(gotB.Body))
	}
}

func TestSubscriptionOriginalRepository_CascadeOnSubscriptionDelete(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-cascade-del", "Cascade Del", "https://example.com/del")
	body := []byte("proxies:\n  - name: cascade-node\n")

	_ = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(body)),
			Body:           body,
			FetchedAt:      time.Now(),
			ContentType:    "text/plain",
			FetchID:        "fetch-del",
		}, sub.Revision)
	})

	// Delete subscription via repository
	if err := subRepo.Delete(ctx, sub.ID); err != nil {
		t.Fatalf("subRepo.Delete failed: %v", err)
	}

	// Getter must report subscription_not_found
	_, err := origRepo.GetBySubscriptionID(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected error after subscription deletion, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != "subscription_not_found" {
		t.Fatalf("expected subscription_not_found, got: %v", err)
	}

	// Verify direct row count is 0
	var count int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_original_configs WHERE subscription_id = ?;", sub.ID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 rows in subscription_original_configs after cascade, got %d", count)
	}
}

func TestSubscriptionOriginalRepository_InvalidationOnSourceURLChange(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-url-change", "URL Sub", "https://example.com/initial-url")
	body := []byte("proxies:\n  - name: initial-node\n")

	_ = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(body)),
			Body:           body,
			FetchedAt:      time.Now(),
			ContentType:    "text/plain",
			FetchID:        "fetch-init",
		}, sub.Revision)
	})

	// Change SourceURLSecretRef
	sub.SourceURLSecretRef = "https://example.com/new-url"
	sub.Revision = "rev-url-updated"
	if err := subRepo.Update(ctx, sub); err != nil {
		t.Fatalf("subRepo.Update failed: %v", err)
	}

	// Stored original config must be invalidated (deleted)
	_, err := origRepo.GetBySubscriptionID(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected original_config_not_found after source URL change, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != "original_config_not_found" {
		t.Fatalf("expected original_config_not_found, got: %v", err)
	}
}

func TestSubscriptionOriginalRepository_RetentionOnNonSourceURLChange(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-keep-config", "Original Name", "https://example.com/keep-url")
	body := []byte("proxies:\n  - name: keep-node\n")

	_ = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(body)),
			Body:           body,
			FetchedAt:      time.Now(),
			ContentType:    "text/plain",
			FetchID:        "fetch-keep",
		}, sub.Revision)
	})

	// Update only name, prefix, and enabled flag (disable)
	sub.Name = "Renamed Subscription"
	sub.Prefix = "PRE_"
	sub.Enabled = false
	sub.Revision = "rev-non-url-updated"
	if err := subRepo.Update(ctx, sub); err != nil {
		t.Fatalf("subRepo.Update failed: %v", err)
	}

	// Stored original config must remain intact!
	got, err := origRepo.GetBySubscriptionID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("expected original config to be retained, got err: %v", err)
	}
	if !bytes.Equal(got.Body, body) {
		t.Fatalf("expected retained body, got %q", string(got.Body))
	}
}

func TestSubscriptionOriginalRepository_HistoryPruningIndependence(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-prune-indep", "Prune Sub", "https://example.com/prune")

	// Insert old fetches and payloads
	oldTime := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	fetchID := "fetch-to-be-pruned"
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'dig-old', 1, 1);
	`, fetchID, sub.ID, oldTime)
	if err != nil {
		t.Fatalf("failed to insert test fetch: %v", err)
	}

	payloadID := "payload-to-be-pruned"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, http_status, headers_json, pinned, created_at)
		VALUES (?, ?, ?, 'dig-old', X'', 200, '{}', 0, ?);
	`, payloadID, sub.ID, fetchID, oldTime)
	if err != nil {
		t.Fatalf("failed to insert test payload: %v", err)
	}

	// Two newer useful payloads make the old payload eligible for pruning.
	for i := 0; i < 2; i++ {
		newFetch, newPayload := fmt.Sprintf("fetch-new-%d", i), fmt.Sprintf("payload-new-%d", i)
		newTime := time.Now().Add(time.Duration(-i) * time.Minute).UTC().Format(time.RFC3339)
		if _, err := db.ExecContext(ctx, `INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid) VALUES (?, ?, ?, 'success', 'new', 1, 1)`, newFetch, sub.ID, newTime); err != nil {
			t.Fatalf("insert newer fetch: %v", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO subscription_payloads (id, subscription_id, fetch_id, content_digest, body_blob, http_status, headers_json, pinned, created_at) VALUES (?, ?, ?, 'new', X'', 200, '{}', 0, ?)`, newPayload, sub.ID, newFetch, newTime); err != nil {
			t.Fatalf("insert newer payload: %v", err)
		}
	}

	// Store original config pointing to fetchID
	body := []byte("proxies:\n  - name: independent-copy\n")
	if err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(body)),
			Body:           body,
			FetchedAt:      time.Now(),
			ContentType:    "text/plain",
			FetchID:        fetchID,
		}, sub.Revision)
	}); err != nil {
		t.Fatalf("store original: %v", err)
	}

	// Run retention pruning and verify the old payload actually disappeared.
	olderThan := time.Now().Add(-1 * time.Hour)
	if _, err := sqlite.PruneSubscriptionRetention(ctx, db, sub.ID, 50, &olderThan); err != nil {
		t.Fatalf("prune: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE id = ?", payloadID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("old payload was not pruned: count=%d err=%v", count, err)
	}

	// Original config must remain intact even if referenced fetch/payload was affected
	got, err := origRepo.GetBySubscriptionID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("expected original config to survive retention pruning, got err: %v", err)
	}
	if !bytes.Equal(got.Body, body) {
		t.Fatalf("body changed or corrupted: %q", string(got.Body))
	}
}

func TestSubscriptionOriginalRepository_SizeConstraints(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-size-checks", "Size Sub", "https://example.com/size")

	// 1. Zero-byte body violates CHECK (length(body_blob) >= 1)
	err := sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Body:           []byte{},
			FetchedAt:      time.Now(),
			ContentType:    "text/plain",
			FetchID:        "fetch-zero",
		}, sub.Revision)
	})
	if err == nil {
		t.Fatalf("expected error inserting 0-byte body due to CHECK constraint, got nil")
	}

	// 2. Exact 10 MiB (10485760 bytes) body succeeds
	maxBody := make([]byte, 10485760)
	maxBody[0] = 'a'
	maxBody[len(maxBody)-1] = 'z'
	err = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(maxBody)),
			Body:           maxBody,
			FetchedAt:      time.Now(),
			ContentType:    "application/octet-stream",
			FetchID:        "fetch-max",
		}, sub.Revision)
	})
	if err != nil {
		t.Fatalf("expected 10MiB body to succeed, got %v", err)
	}

	// 3. Storage accepts bodies above the default fetch budget.
	overBody := make([]byte, 10485761)
	err = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  fmt.Sprintf("%x", sha256.Sum256(overBody)),
			Body:           overBody,
			FetchedAt:      time.Now(),
			ContentType:    "application/octet-stream",
			FetchID:        "fetch-over",
		}, sub.Revision)
	})
	if err != nil {
		t.Fatalf("expected >10MiB body to succeed, got %v", err)
	}
}

func TestSubscriptionOriginalRepository_MissingTableReturnsError(t *testing.T) {
	db, _ := setupTestDB(t)
	subRepo := sqlite.NewSubscriptionRepository(db)
	origRepo := sqlite.NewSubscriptionOriginalRepository(db)
	ctx := context.Background()

	sub := createTestSubscription(t, db, subRepo, "sub-missing-table", "Missing Table Sub", "https://example.com/missing-table")

	// Drop subscription_original_configs table to simulate missing migration
	_, err := db.ExecContext(ctx, "DROP TABLE subscription_original_configs;")
	if err != nil {
		t.Fatalf("failed to drop table: %v", err)
	}

	// 1. GetBySubscriptionID must return an error and NOT report original_config_not_found
	_, err = origRepo.GetBySubscriptionID(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected error from GetBySubscriptionID on missing table, got nil")
	}
	var domErr *domain.DomainError
	if errors.As(err, &domErr) && domErr.Code == "original_config_not_found" {
		t.Fatalf("missing table must NOT be reported as original_config_not_found")
	}

	// 2. UpsertOriginalConfigTx must return an error and NOT swallow
	err = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		return sqlite.UpsertOriginalConfigTx(ctx, tx, &domain.SubscriptionOriginalConfig{
			SubscriptionID: sub.ID,
			ContentDigest:  "digest",
			Body:           []byte("proxies: []"),
			FetchedAt:      time.Now(),
		}, sub.Revision)
	})
	if err == nil {
		t.Fatalf("expected error from UpsertOriginalConfigTx on missing table, got nil")
	}

	// 3. DeleteBySubscriptionID must return an error and NOT swallow
	err = origRepo.DeleteBySubscriptionID(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected error from DeleteBySubscriptionID on missing table, got nil")
	}

	// 4. Invalidation on source URL change must fail transaction if table is missing
	sub.SourceURLSecretRef = "https://example.com/new-url"
	err = subRepo.Update(ctx, sub)
	if err == nil {
		t.Fatalf("expected subRepo.Update to fail when invalidation encounters missing table, got nil")
	}
}
