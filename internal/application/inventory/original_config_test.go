package inventory_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

type controlledOriginalFetcher struct {
	mu        sync.Mutex
	responses map[string]*fetch.Response
	errors    map[string]error
	onFetch   func(url string)
}

func newControlledOriginalFetcher() *controlledOriginalFetcher {
	return &controlledOriginalFetcher{
		responses: make(map[string]*fetch.Response),
		errors:    make(map[string]error),
	}
}

func (f *controlledOriginalFetcher) setResponse(url string, resp *fetch.Response) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[url] = resp
	delete(f.errors, url)
}

func (f *controlledOriginalFetcher) setError(url string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors[url] = err
	delete(f.responses, url)
}

func (f *controlledOriginalFetcher) Fetch(_ context.Context, opts fetch.Options) (*fetch.Response, error) {
	f.mu.Lock()
	onFetch := f.onFetch
	f.mu.Unlock()
	if onFetch != nil {
		onFetch(opts.URL)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.errors[opts.URL]; ok {
		return nil, err
	}
	if resp, ok := f.responses[opts.URL]; ok {
		return resp, nil
	}
	return nil, fmt.Errorf("unexpected fetch URL: %s", opts.URL)
}

func setupOriginalTestDB(t *testing.T) (*sql.DB, domain.SubscriptionRepository, *inventory.Service, *controlledOriginalFetcher) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "orig_inv_test.db")
	cfg := sqlite.DefaultConfig(dbPath)
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("setupOriginalTestDB: OpenAndMigrate failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	fetcher := newControlledOriginalFetcher()

	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher)
	return db, subRepo, svc, fetcher
}

func createOrigSub(t *testing.T, subRepo domain.SubscriptionRepository, id, name, url string) *domain.Subscription {
	t.Helper()
	sub := &domain.Subscription{
		ID:                 id,
		Name:               name,
		SourceURLSecretRef: url,
		Enabled:            true,
		Revision:           domain.MustNewUUIDv7(),
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  3600,
			TimeoutSeconds:   10,
			MaxResponseBytes: 15 * 1024 * 1024,
		},
		Config: domain.SubscriptionConfig{},
	}
	if err := subRepo.Create(context.Background(), sub); err != nil {
		t.Fatalf("createOrigSub failed: %v", err)
	}
	return sub
}

func TestInventoryService_GetOriginalConfig_Errors(t *testing.T) {
	_, subRepo, svc, _ := setupOriginalTestDB(t)
	ctx := context.Background()

	// 1. Empty subscription ID returns validation error
	_, err := svc.GetOriginalConfig(ctx, "")
	if err == nil {
		t.Fatalf("expected error for empty subscription ID, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != "missing_subscription_id" {
		t.Fatalf("expected missing_subscription_id, got: %v", err)
	}

	// 2. Non-existent subscription ID returns subscription_not_found
	_, err = svc.GetOriginalConfig(ctx, "sub-missing-id")
	if err == nil {
		t.Fatalf("expected error for non-existent subscription, got nil")
	}
	if !errors.As(err, &domErr) || domErr.Code != "subscription_not_found" {
		t.Fatalf("expected subscription_not_found, got: %v", err)
	}
	if domErr.Message != "subscription not found" {
		t.Fatalf("expected generic message %q, got %q", "subscription not found", domErr.Message)
	}

	// 3. Existing subscription without stored original config returns original_config_not_found
	sub := createOrigSub(t, subRepo, "sub-no-orig", "No Orig Sub", "https://example.com/no-orig")
	_, err = svc.GetOriginalConfig(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected error for missing original config, got nil")
	}
	if !errors.As(err, &domErr) || domErr.Code != "original_config_not_found" {
		t.Fatalf("expected original_config_not_found, got: %v", err)
	}
	if domErr.Message != "subscription original configuration not found" {
		t.Fatalf("expected generic message %q, got %q", "subscription original configuration not found", domErr.Message)
	}
}

func TestInventoryService_ReconcileSubscription_OriginalConfigPersistence(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-reconcile-success", "Success Sub", "https://example.com/sub-success")

	body := []byte("proxies:\n  - name: node-success-1\n    type: ss\n    server: 1.1.1.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: secret\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          body,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body)),
	})

	res, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ReconcileSubscription failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected outcome success, got %s", res.Outcome)
	}

	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, body) {
		t.Fatalf("expected body %q, got %q", string(body), string(orig.Body))
	}
	expectedDigest := fmt.Sprintf("%x", sha256.Sum256(body))
	if orig.ContentDigest != expectedDigest {
		t.Fatalf("expected digest %s, got %s", expectedDigest, orig.ContentDigest)
	}
	if orig.FetchID != res.FetchID {
		t.Fatalf("expected fetchID %s, got %s", res.FetchID, orig.FetchID)
	}
	if orig.ContentType != "application/yaml" {
		t.Fatalf("expected ContentType application/yaml, got %s", orig.ContentType)
	}
}

func TestInventoryService_ReconcileSubscription_PartialPersistence(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-reconcile-partial", "Partial Sub", "https://example.com/sub-partial")

	// 1 valid node + 1 invalid node (missing type/server)
	partialBody := []byte("proxies:\n  - name: valid-node\n    type: ss\n    server: 2.2.2.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: pass\n  - name: broken-node\n    invalid_field: true\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/yaml",
		Body:          partialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(partialBody)),
	})

	res, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ReconcileSubscription failed: %v", err)
	}
	if res.Outcome != domain.FetchOutcomePartial {
		t.Fatalf("expected outcome partial, got %s", res.Outcome)
	}

	// Must store complete un-truncated original body even for partial acceptance
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, partialBody) {
		t.Fatalf("expected complete partial body, got %q", string(orig.Body))
	}
}

func TestInventoryService_ReconcileSubscription_FailureRetainsOldCopy(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-fail-retain", "Fail Retain Sub", "https://example.com/sub-fail")

	initialBody := []byte("proxies:\n  - name: good-node\n    type: ss\n    server: 3.3.3.3\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Second fetch returns completely invalid syntax (parse error)
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          []byte("{{this is not yaml or base64 or anything valid node list}}"),
		ContentDigest: "bad-digest",
	})

	_, err = svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected parse failure error, got nil")
	}

	// Old copy MUST be retained
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, initialBody) {
		t.Fatalf("initial copy was not retained: got %q", string(orig.Body))
	}
}

func TestInventoryService_ReconcileSubscription_EmptyRetainsOldCopy(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-empty-retain", "Empty Retain Sub", "https://example.com/sub-empty")

	initialBody := []byte("proxies:\n  - name: good-node-2\n    type: ss\n    server: 4.4.4.4\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Second fetch returns valid YAML but with 0 proxies (empty subscription)
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          []byte("proxies: []\n"),
		ContentDigest: "empty-digest",
	})

	_, err = svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected empty subscription error, got nil")
	}

	// Old copy MUST be retained
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, initialBody) {
		t.Fatalf("initial copy was not retained: got %q", string(orig.Body))
	}
}

func TestInventoryService_ReconcileSubscription_LargeConfiguredBudget(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()
	sub := createOrigSub(t, subRepo, "sub-large-budget", "Large", "https://example.com/large")
	prefix := "proxies:\n  - name: large-node\n    type: ss\n    server: 6.6.6.6\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n# "
	body := append([]byte(prefix), bytes.Repeat([]byte("x"), 10*1024*1024+1-len(prefix))...)
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{StatusCode: 200, Body: body, ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body))})
	res, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil || res == nil || res.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("large configured response failed: result=%+v err=%v", res, err)
	}
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil || !bytes.Equal(orig.Body, body) {
		t.Fatalf("large original not retained: %v", err)
	}
}

func TestInventoryService_ReconcileSubscription_OverCapRetainsOldCopy(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-overcap", "Overcap Sub", "https://example.com/sub-overcap")

	initialBody := []byte("proxies:\n  - name: node-keep\n    type: ss\n    server: 5.5.5.5\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Exceed the configured 15 MiB response budget.
	prefix := "proxies:\n  - name: overcap-node\n    type: ss\n    server: 6.6.6.6\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n# padding: "
	padSize := 15*1024*1024 + 1 - len(prefix)
	overcapBody := append([]byte(prefix), bytes.Repeat([]byte("x"), padSize)...)

	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          overcapBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(overcapBody)),
	})

	resOver, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected validation error for overcap body, got nil")
	}
	if resOver == nil || resOver.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed result, got %+v", resOver)
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != "subscription_content_too_large" || domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected CategoryValidation subscription_content_too_large, got: %v", err)
	}

	// Old original config candidate must be retained (overcap candidate rejected)
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, initialBody) {
		t.Fatalf("old original config was overwritten by overcap body!")
	}
}

func TestInventoryService_ReconcileSubscription_Non2xxRetainsOldCopy(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-non2xx", "Non2xx Sub", "https://example.com/sub-non2xx")

	initialBody := []byte("proxies:\n  - name: good-node-3\n    type: ss\n    server: 7.7.7.7\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Fetcher returns HTTP 500 error
	fetcher.setError(sub.SourceURLSecretRef, errors.New("upstream server error HTTP 500"))

	_, err = svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected error on upstream HTTP failure, got nil")
	}

	// Old copy MUST be retained
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, initialBody) {
		t.Fatalf("initial copy was not retained: got %q", string(orig.Body))
	}
}

func TestInventoryService_ReconcileSubscription_CASRevisionConflict(t *testing.T) {
	db, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-cas-conflict", "CAS Sub", "https://example.com/sub-cas")

	initialBody := []byte("proxies:\n  - name: node-before-race\n    type: ss\n    server: 8.8.8.8\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	// Inject racing update in DB during Fetch operation
	fetcher.onFetch = func(url string) {
		// Simulate concurrent user editing subscription revision in DB
		_, execErr := db.Exec("UPDATE subscriptions SET revision = 'rev-racing-change' WHERE id = ?;", sub.ID)
		if execErr != nil {
			t.Logf("racing update failed: %v", execErr)
		}
	}

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected revision conflict error, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != "subscription_revision_conflict" {
		t.Fatalf("expected subscription_revision_conflict, got: %v", err)
	}

	// Because transaction aborted on CAS recheck, no original config was stored
	_, err = svc.GetOriginalConfig(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected original_config_not_found since transaction aborted, got nil")
	}
}

func TestInventoryService_ReconcileSubscription_ContextCanceled(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)

	sub := createOrigSub(t, subRepo, "sub-canceled", "Canceled Sub", "https://example.com/sub-canceled")

	body := []byte("proxies:\n  - name: node-canceled\n    type: ss\n    server: 9.9.9.9\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          body,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected error on canceled context, got nil")
	}

	// No original config committed
	_, err = svc.GetOriginalConfig(context.Background(), sub.ID)
	if err == nil {
		t.Fatalf("expected original_config_not_found after canceled context, got nil")
	}
}

// TestInventoryService_ReconcileSubscription_RepeatedSuccessKeepsExactlyOneLatest proves
// that repeated successful reconciliations keep exactly 1 config row with the latest bytes.
func TestInventoryService_ReconcileSubscription_RepeatedSuccessKeepsExactlyOneLatest(t *testing.T) {
	db, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-repeat-latest", "Repeat Latest Sub", "https://example.com/repeat-latest")

	body1 := []byte("proxies:\n  - name: node-v1\n    type: ss\n    server: 1.1.1.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: p1\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          body1,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body1)),
	})

	res1, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	orig1, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed after first reconcile: %v", err)
	}
	if !bytes.Equal(orig1.Body, body1) {
		t.Fatalf("expected body1, got %q", string(orig1.Body))
	}
	if orig1.FetchID != res1.FetchID {
		t.Fatalf("expected fetchID %s, got %s", res1.FetchID, orig1.FetchID)
	}

	// Reconcile second time with body 2
	body2 := []byte("proxies:\n  - name: node-v2\n    type: ss\n    server: 1.1.1.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: p2\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          body2,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body2)),
	})

	res2, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	orig2, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed after second reconcile: %v", err)
	}
	if !bytes.Equal(orig2.Body, body2) {
		t.Fatalf("expected body2, got %q", string(orig2.Body))
	}
	if orig2.FetchID != res2.FetchID {
		t.Fatalf("expected fetchID to update to res2 %s, got %s", res2.FetchID, orig2.FetchID)
	}

	// Verify exactly 1 config row exists in table for this subscription
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_original_configs WHERE subscription_id = ?", sub.ID).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row in subscription_original_configs, got %d", count)
	}
}

// Rejected candidates record failed fetches while retaining prior inventory and original bytes.
func TestInventoryService_ReconcileSubscription_OverCapAndNon2xxRetainOldAndUnchangedFactsCounts(t *testing.T) {
	db, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-counts-check", "Counts Check Sub", "https://example.com/sub-counts-check")

	initialBody := []byte("proxies:\n  - name: node-counts-initial\n    type: ss\n    server: 5.5.5.5\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	resInit, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}
	if resInit.Outcome != domain.FetchOutcomeSuccess {
		t.Fatalf("expected outcome success, got %s", resInit.Outcome)
	}

	getCounts := func() (fetches, nodes, payloads int) {
		t.Helper()
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_fetches WHERE subscription_id = ?", sub.ID).Scan(&fetches); err != nil {
			t.Fatalf("count fetches failed: %v", err)
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_sources WHERE subscription_id = ?", sub.ID).Scan(&nodes); err != nil {
			t.Fatalf("count nodes failed: %v", err)
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id = ?", sub.ID).Scan(&payloads); err != nil {
			t.Fatalf("count payloads failed: %v", err)
		}
		return
	}

	baseFetches, baseNodes, basePayloads := getCounts()

	// 1. Candidate exceeds the configured response budget.
	prefix := "proxies:\n  - name: overcap-node\n    type: ss\n    server: 6.6.6.6\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n# padding: "
	padSize := 15*1024*1024 + 1 - len(prefix)
	overcapBody := append([]byte(prefix), bytes.Repeat([]byte("x"), padSize)...)

	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          overcapBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(overcapBody)),
	})

	resOver, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected validation error for overcap body, got nil")
	}
	if resOver == nil || resOver.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed result, got %+v", resOver)
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != "subscription_content_too_large" || domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected CategoryValidation subscription_content_too_large, got: %v", err)
	}

	fetchesAfterOver, nodesAfterOver, payloadsAfterOver := getCounts()
	if fetchesAfterOver != baseFetches+1 || nodesAfterOver != baseNodes || payloadsAfterOver != basePayloads {
		t.Fatalf("counts mutated on overcap failure: fetches %d->%d, nodes %d->%d, payloads %d->%d",
			baseFetches, fetchesAfterOver, baseNodes, nodesAfterOver, basePayloads, payloadsAfterOver)
	}

	origAfterOver, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(origAfterOver.Body, initialBody) {
		t.Fatalf("original config was mutated by overcap body")
	}

	// 2. Explicit non-2xx Response must reject with validation error, leaving old copy and counts unchanged
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    500,
		ContentType:   "text/plain",
		Body:          []byte("Internal Server Error"),
		ContentDigest: "500-digest",
	})

	resNon2xx, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected validation error for non-2xx response, got nil")
	}
	if resNon2xx == nil || resNon2xx.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed result, got %+v", resNon2xx)
	}
	if !errors.As(err, &domErr) || domErr.Code != "invalid_fetch_status" || domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected CategoryValidation invalid_fetch_status, got: %v", err)
	}

	fetchesAfterNon2xx, nodesAfterNon2xx, payloadsAfterNon2xx := getCounts()
	if fetchesAfterNon2xx != baseFetches+2 || nodesAfterNon2xx != baseNodes || payloadsAfterNon2xx != basePayloads {
		t.Fatalf("counts mutated on non-2xx failure: fetches %d->%d, nodes %d->%d, payloads %d->%d",
			baseFetches, fetchesAfterNon2xx, baseNodes, nodesAfterNon2xx, basePayloads, payloadsAfterNon2xx)
	}

	origAfterNon2xx, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(origAfterNon2xx.Body, initialBody) {
		t.Fatalf("original config was mutated by non-2xx response")
	}

	// 3. Empty body candidate must reject with validation error, leaving old copy and counts unchanged
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/plain",
		Body:          []byte{},
		ContentDigest: "empty-digest",
	})

	resEmpty, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected validation error for empty body, got nil")
	}
	if resEmpty == nil || resEmpty.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed result, got %+v", resEmpty)
	}
	if !errors.As(err, &domErr) || domErr.Code != "empty_subscription_content" || domErr.Category != domain.CategoryValidation {
		t.Fatalf("expected CategoryValidation empty_subscription_content, got: %v", err)
	}

	fetchesAfterEmpty, nodesAfterEmpty, payloadsAfterEmpty := getCounts()
	if fetchesAfterEmpty != baseFetches+3 || nodesAfterEmpty != baseNodes || payloadsAfterEmpty != basePayloads {
		t.Fatalf("counts mutated on empty body failure: fetches %d->%d, nodes %d->%d, payloads %d->%d",
			baseFetches, fetchesAfterEmpty, baseNodes, nodesAfterEmpty, basePayloads, payloadsAfterEmpty)
	}

	// 4. Nil response must reject with internal error, leaving old copy and counts unchanged
	fetcher.setResponse(sub.SourceURLSecretRef, nil)

	resNil, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected internal error for nil response, got nil")
	}
	if resNil == nil || resNil.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("expected failed result, got %+v", resNil)
	}
	if !errors.As(err, &domErr) || domErr.Code != "nil_fetch_response" || domErr.Category != domain.CategoryInternal {
		t.Fatalf("expected CategoryInternal nil_fetch_response, got: %v", err)
	}

	fetchesAfterNil, nodesAfterNil, payloadsAfterNil := getCounts()
	if fetchesAfterNil != baseFetches+4 || nodesAfterNil != baseNodes || payloadsAfterNil != basePayloads {
		t.Fatalf("counts mutated on nil response failure: fetches %d->%d, nodes %d->%d, payloads %d->%d",
			baseFetches, fetchesAfterNil, baseNodes, nodesAfterNil, basePayloads, payloadsAfterNil)
	}
}

// TestInventoryService_ReconcileSubscription_PersistenceTriggerAbortRollsBackFacts proves
// that if persistence of the original config fails (simulated via an SQLite trigger raising ABORT),
// the entire convergence transaction rolls back: new fetch record, new payload, and new nodes
// are discarded, and the old original configuration body is retained.
func TestInventoryService_ReconcileSubscription_PersistenceTriggerAbortRollsBackFacts(t *testing.T) {
	db, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-trigger-abort", "Trigger Abort Sub", "https://example.com/sub-trigger")

	body1 := []byte("proxies:\n  - name: node-stable-1\n    type: ss\n    server: 7.7.7.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: p1\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          body1,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body1)),
	})

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Verify initial state: exactly 1 fetch, 1 node, 1 payload, initial body
	var initialFetches, initialNodes, initialPayloads int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_fetches WHERE subscription_id = ?", sub.ID).Scan(&initialFetches)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_sources WHERE subscription_id = ?", sub.ID).Scan(&initialNodes)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id = ?", sub.ID).Scan(&initialPayloads)
	if initialFetches != 1 || initialNodes != 1 || initialPayloads != 1 {
		t.Fatalf("expected initial counts (1, 1, 1), got (%d, %d, %d)", initialFetches, initialNodes, initialPayloads)
	}

	// Inject SQLite BEFORE UPDATE and BEFORE INSERT triggers on subscription_original_configs raising ABORT
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER trigger_abort_orig_update
		BEFORE UPDATE ON subscription_original_configs
		BEGIN
			SELECT RAISE(ABORT, 'injected failure in subscription_original_configs update');
		END;
		CREATE TRIGGER trigger_abort_orig_insert
		BEFORE INSERT ON subscription_original_configs
		BEGIN
			SELECT RAISE(ABORT, 'injected failure in subscription_original_configs insert');
		END;
	`)
	if err != nil {
		t.Fatalf("failed to create triggers: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS trigger_abort_orig_update;")
		_, _ = db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS trigger_abort_orig_insert;")
	}()

	// Reconcile with body2 containing new node
	body2 := []byte("proxies:\n  - name: node-candidate-2\n    type: ss\n    server: 7.7.7.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: p2\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          body2,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body2)),
	})

	res, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected reconcile failure due to trigger ABORT, got nil")
	}
	if !strings.Contains(err.Error(), "injected failure in subscription_original_configs") {
		t.Fatalf("expected trigger abort message in error, got: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result on transaction failure, got %+v", res)
	}

	// Verify all new facts were rolled back completely!
	var currentFetches, currentNodes, currentPayloads int
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_fetches WHERE subscription_id = ?", sub.ID).Scan(&currentFetches)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_sources WHERE subscription_id = ?", sub.ID).Scan(&currentNodes)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id = ?", sub.ID).Scan(&currentPayloads)

	if currentFetches != initialFetches {
		t.Fatalf("fetch row was not rolled back: had %d, now %d", initialFetches, currentFetches)
	}
	if currentNodes != initialNodes {
		t.Fatalf("node rows were not rolled back: had %d, now %d", initialNodes, currentNodes)
	}
	if currentPayloads != initialPayloads {
		t.Fatalf("payload row was not rolled back: had %d, now %d", initialPayloads, currentPayloads)
	}

	// Verify existing node is still node-stable-1
	var nodeName string
	_ = db.QueryRowContext(ctx, "SELECT n.display_name FROM nodes n JOIN node_sources ns ON ns.node_logical_id = n.logical_id WHERE ns.subscription_id = ? LIMIT 1", sub.ID).Scan(&nodeName)
	if nodeName != "node-stable-1" {
		t.Fatalf("expected node-stable-1, got %s", nodeName)
	}

	// Verify original config retained old body1
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, body1) {
		t.Fatalf("original config was corrupted: expected body1, got %q", string(orig.Body))
	}
}

// TestInventoryService_ReconcileSubscription_SourceURLEditDuringFetch_InvalidatesAndCASRejects
// proves that editing the subscription SourceURLSecretRef during fetch using repository.Update
// invalidates the old copy in the update transaction and causes the CAS recheck in ReconcileSubscription
// to reject the stale candidate.
func TestInventoryService_ReconcileSubscription_SourceURLEditDuringFetch_InvalidatesAndCASRejects(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)
	ctx := context.Background()

	sub := createOrigSub(t, subRepo, "sub-url-race", "URL Race Sub", "https://example.com/initial-url")

	initialBody := []byte("proxies:\n  - name: node-before-edit\n    type: ss\n    server: 8.8.8.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	_, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Verify initial config exists
	orig, err := svc.GetOriginalConfig(ctx, sub.ID)
	if err != nil || !bytes.Equal(orig.Body, initialBody) {
		t.Fatalf("expected initial config stored: %v", err)
	}

	staleBody := []byte("proxies:\n  - name: node-stale\n    type: ss\n    server: 8.8.8.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          staleBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(staleBody)),
	})

	// During fetch, user updates subscription source URL via repository.Update
	// This invalidates the old original config in the update transaction!
	fetcher.onFetch = func(url string) {
		currentSub, getErr := subRepo.GetByID(ctx, sub.ID)
		if getErr != nil {
			t.Errorf("failed to get subscription during fetch: %v", getErr)
			return
		}
		currentSub.SourceURLSecretRef = "https://example.com/new-url"
		currentSub.Revision = domain.MustNewUUIDv7()
		if updateErr := subRepo.Update(ctx, currentSub); updateErr != nil {
			t.Errorf("subRepo.Update failed during fetch: %v", updateErr)
		}
	}

	res, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected CAS revision conflict error, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != "subscription_revision_conflict" {
		t.Fatalf("expected subscription_revision_conflict, got: %v", err)
	}
	if res != nil {
		t.Fatalf("expected nil result on conflict, got %+v", res)
	}

	// The old original config was invalidated by the source URL change,
	// and the stale candidate was rejected by CAS, so GetOriginalConfig must return original_config_not_found!
	_, err = svc.GetOriginalConfig(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected original_config_not_found, got nil")
	}
	if !errors.As(err, &domErr) || domErr.Code != "original_config_not_found" {
		t.Fatalf("expected code original_config_not_found, got: %v", err)
	}
}

// TestInventoryService_ReconcileSubscription_CanceledDuringFetchedResponseRetainsOld
// proves that when context is canceled during fetched response delivery (in onFetch callback),
// the reconciliation fails cleanly and retains the previous original configuration.
func TestInventoryService_ReconcileSubscription_CanceledDuringFetchedResponseRetainsOld(t *testing.T) {
	_, subRepo, svc, fetcher := setupOriginalTestDB(t)

	sub := createOrigSub(t, subRepo, "sub-cancel-infetch", "Cancel in Fetch Sub", "https://example.com/sub-cancel")

	initialBody := []byte("proxies:\n  - name: node-keep-cancel\n    type: ss\n    server: 9.9.9.1\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          initialBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(initialBody)),
	})

	_, err := svc.ReconcileSubscription(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("initial reconcile failed: %v", err)
	}

	// Prepare second fetch response
	newBody := []byte("proxies:\n  - name: node-new-canceled\n    type: ss\n    server: 9.9.9.2\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n")
	fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{
		StatusCode:    200,
		ContentType:   "application/yaml",
		Body:          newBody,
		ContentDigest: fmt.Sprintf("%x", sha256.Sum256(newBody)),
	})

	// Cancel context *inside* onFetch when response is being returned
	ctx, cancel := context.WithCancel(context.Background())
	fetcher.onFetch = func(url string) {
		cancel()
	}

	res, err := svc.ReconcileSubscription(ctx, sub.ID)
	if err == nil {
		t.Fatalf("expected error on canceled context, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result on cancellation, got %+v", res)
	}

	// Old copy MUST be retained
	orig, err := svc.GetOriginalConfig(context.Background(), sub.ID)
	if err != nil {
		t.Fatalf("GetOriginalConfig failed: %v", err)
	}
	if !bytes.Equal(orig.Body, initialBody) {
		t.Fatalf("expected initialBody retained after cancellation, got %q", string(orig.Body))
	}
}
