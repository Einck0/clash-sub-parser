package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository"
)

func setupTestDB(t *testing.T) *repository.SQLiteDB {
	t.Helper()
	// Use unique in-memory database with shared cache for connection pool testing
	dbName := fmt.Sprintf("file:test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	opts := repository.Options{
		Path:         dbName,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		BusyTimeout:  30 * time.Second,
	}

	db, err := repository.NewSQLiteDB(opts)
	if err != nil {
		t.Fatalf("failed to create sqlite db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.InitSchema(ctx); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func TestSQLiteDB_PragmasAndConnection(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Verify foreign_keys
	var fk int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&fk); err != nil {
		t.Fatalf("failed to query foreign_keys pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("expected foreign_keys = 1, got %d", fk)
	}

	// Verify busy_timeout
	var busyTimeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatalf("failed to query busy_timeout pragma: %v", err)
	}
	if busyTimeout < 30000 {
		t.Errorf("expected busy_timeout >= 30000, got %d", busyTimeout)
	}
}

func TestSubscriptionRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := db.NewSubscriptionRepository()
	ctx := context.Background()

	// 1. Initial count should be 0
	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}

	// 2. Create subscription
	sub := &domain.Subscription{
		Name:                 "MainSub",
		URL:                  "https://example.com/sub",
		UpdateInterval:       86400,
		IsPrimary:            true,
		Enabled:              true,
		NodePrefix:           "[HK]",
		FilterRegex:          []string{"^HK.*", "^US.*"},
		FilterMediaUnlock:    []string{"netflix", "youtube"},
		IncludeNodeNames:     []string{"NodeA", "NodeB"},
		ExcludeNodeNames:     []string{"Expired"},
		NodeRenames:          map[string]string{"Old": "New"},
		FetchComments:        []string{"Fetched OK"},
		SubscriptionUserinfo: "upload=100; download=200; total=1000",
	}

	if err := repo.Create(ctx, sub); err != nil {
		t.Fatalf("create subscription failed: %v", err)
	}
	if sub.ID == 0 {
		t.Fatalf("expected sub.ID to be assigned, got 0")
	}

	// 3. GetByID
	fetched, err := repo.GetByID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
	if fetched.Name != sub.Name || fetched.URL != sub.URL || !fetched.IsPrimary || !fetched.Enabled {
		t.Errorf("unexpected fetched subscription: %+v", fetched)
	}
	if len(fetched.FilterRegex) != 2 || fetched.FilterRegex[0] != "^HK.*" {
		t.Errorf("unexpected FilterRegex: %+v", fetched.FilterRegex)
	}
	if fetched.NodeRenames["Old"] != "New" {
		t.Errorf("unexpected NodeRenames: %+v", fetched.NodeRenames)
	}

	// 4. GetByName
	fetchedByName, err := repo.GetByName(ctx, "MainSub")
	if err != nil {
		t.Fatalf("get by name failed: %v", err)
	}
	if fetchedByName.ID != sub.ID {
		t.Errorf("expected ID %d, got %d", sub.ID, fetchedByName.ID)
	}

	// 5. Update
	now := time.Now().UTC()
	sub.URL = "https://example.com/sub_v2"
	sub.LastFetchedAt = &now
	sub.FetchFailedCount = 1
	sub.LastFetchError = "temporary timeout"
	if err := repo.Update(ctx, sub); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	updated, err := repo.GetByID(ctx, sub.ID)
	if err != nil {
		t.Fatalf("get updated sub failed: %v", err)
	}
	if updated.URL != "https://example.com/sub_v2" || updated.FetchFailedCount != 1 || updated.LastFetchError != "temporary timeout" {
		t.Errorf("unexpected updated fields: %+v", updated)
	}

	// 6. List
	list, err := repo.List(ctx, false)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected list length 1, got %d", len(list))
	}

	// 7. Delete
	if err := repo.Delete(ctx, sub.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = repo.GetByID(ctx, sub.ID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}
}

func TestNodeRepository_CRUD_And_BatchUpsert(t *testing.T) {
	db := setupTestDB(t)
	repo := db.NewNodeRepository()
	ctx := context.Background()

	// 1. Create a node
	node := &domain.Node{
		Name:     "HK-Node-01",
		Protocol: domain.ProtocolVLESS,
		Server:   "hk01.example.com",
		Port:     443,
		NormalizedPayload: map[string]any{
			"uuid": "550e8400-e29b-41d4-a716-446655440000",
			"tls":  true,
		},
		LifecycleState: domain.LifecycleActive,
	}

	if err := repo.Create(ctx, node); err != nil {
		t.Fatalf("create node failed: %v", err)
	}
	if node.ID == 0 {
		t.Fatalf("expected node.ID to be assigned")
	}
	if node.LogicalID == "" {
		t.Fatalf("expected node.LogicalID to be generated")
	}
	if node.PayloadFingerprint == "" {
		t.Fatalf("expected node.PayloadFingerprint to be computed")
	}

	// 2. GetByID and GetByLogicalID and GetByFingerprint
	byID, err := repo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
	if byID.Name != node.Name || byID.Server != node.Server || byID.Port != 443 {
		t.Errorf("unexpected node: %+v", byID)
	}

	byLogical, err := repo.GetByLogicalID(ctx, node.LogicalID)
	if err != nil {
		t.Fatalf("get by logical id failed: %v", err)
	}
	if byLogical.ID != node.ID {
		t.Errorf("expected ID %d, got %d", node.ID, byLogical.ID)
	}

	byFP, err := repo.GetByFingerprint(ctx, node.PayloadFingerprint)
	if err != nil {
		t.Fatalf("get by fingerprint failed: %v", err)
	}
	if byFP.ID != node.ID {
		t.Errorf("expected ID %d, got %d", node.ID, byFP.ID)
	}

	// 3. BatchUpsert: update existing + insert new
	newNode := &domain.Node{
		Name:     "JP-Node-01",
		Protocol: domain.ProtocolShadowsocks,
		Server:   "jp01.example.com",
		Port:     8388,
		NormalizedPayload: map[string]any{
			"password": "secret",
			"cipher":   "aes-128-gcm",
		},
		LifecycleState: domain.LifecycleActive,
	}

	updatedNode1 := &domain.Node{
		LogicalID:          node.LogicalID,
		PayloadFingerprint: node.PayloadFingerprint,
		Name:               "HK-Node-01-Renamed",
		Protocol:           domain.ProtocolVLESS,
		Server:             "hk01.example.com",
		Port:               443,
		NormalizedPayload: map[string]any{
			"uuid": "550e8400-e29b-41d4-a716-446655440000",
			"tls":  true,
		},
		LifecycleState: domain.LifecycleActive,
	}

	affected, err := repo.BatchUpsert(ctx, []*domain.Node{updatedNode1, newNode})
	if err != nil {
		t.Fatalf("batch upsert failed: %v", err)
	}
	if affected != 2 {
		t.Errorf("expected 2 affected nodes, got %d", affected)
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 nodes in database, got %d", count)
	}

	// Check updated node name
	refetched, err := repo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("refetch failed: %v", err)
	}
	if refetched.Name != "HK-Node-01-Renamed" {
		t.Errorf("expected name 'HK-Node-01-Renamed', got %q", refetched.Name)
	}

	// 4. List by LifecycleState
	activeList, err := repo.List(ctx, domain.LifecycleActive)
	if err != nil {
		t.Fatalf("list active nodes failed: %v", err)
	}
	if len(activeList) != 2 {
		t.Errorf("expected 2 active nodes, got %d", len(activeList))
	}

	// 5. Delete
	if err := repo.Delete(ctx, node.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	countAfter, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count after delete failed: %v", err)
	}
	if countAfter != 1 {
		t.Errorf("expected 1 node after delete, got %d", countAfter)
	}
}

func TestNodeGroupRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := db.NewNodeGroupRepository()
	ctx := context.Background()

	group := &domain.NodeGroup{
		Name:      "Auto-HK",
		Kind:      domain.GroupKindAuto,
		GroupType: domain.GroupTypeURLTest,
		SortOrder: 10,
		RegexRules: []string{"HK"},
		IncludeNodes: []string{"HK-01", "HK-02"},
		IncludeGroupIDs: []int64{1, 2},
		IncludeEntries: []domain.GroupIncludeEntry{
			{Type: "regex", Value: "HK.*"},
		},
		AddFallback: true,
		URLTestConfig: domain.URLTestConfig{
			URL:       "https://cp.cloudflare.com/generate_204",
			Interval:  300,
			Tolerance: 50,
		},
	}

	if err := repo.Create(ctx, group); err != nil {
		t.Fatalf("create node group failed: %v", err)
	}
	if group.ID == 0 {
		t.Fatalf("expected assigned group.ID")
	}

	fetched, err := repo.GetByID(ctx, group.ID)
	if err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
	if fetched.Name != "Auto-HK" || fetched.GroupType != domain.GroupTypeURLTest {
		t.Errorf("unexpected fetched group: %+v", fetched)
	}
	if fetched.URLTestConfig.Tolerance != 50 {
		t.Errorf("unexpected URLTestConfig tolerance: %d", fetched.URLTestConfig.Tolerance)
	}

	fetchedByName, err := repo.GetByName(ctx, "Auto-HK")
	if err != nil {
		t.Fatalf("get by name failed: %v", err)
	}
	if fetchedByName.ID != group.ID {
		t.Errorf("expected ID %d, got %d", group.ID, fetchedByName.ID)
	}

	// Update sort order
	group.SortOrder = 20
	if err := repo.Update(ctx, group); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 || list[0].SortOrder != 20 {
		t.Errorf("unexpected list: %+v", list)
	}

	// Delete
	if err := repo.Delete(ctx, group.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	count, _ := repo.Count(ctx)
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestRuleRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := db.NewRuleRepository()
	ctx := context.Background()

	rule1 := &domain.Rule{
		Name:      "Google",
		Category:  "Streaming",
		Type:      domain.RuleTypeDomainSuffix,
		Value:     "google.com",
		Proxy:     "Auto-Proxy",
		Options:   []string{"no-resolve"},
		SortOrder: 5,
		Enabled:   true,
	}
	rule2 := &domain.Rule{
		Name:      "Direct-LAN",
		Category:  "Direct",
		Type:      domain.RuleTypeIPCIDR,
		Value:     "192.168.0.0/16",
		Proxy:     "DIRECT",
		SortOrder: 1,
		Enabled:   false,
	}

	if err := repo.Create(ctx, rule1); err != nil {
		t.Fatalf("create rule1 failed: %v", err)
	}
	if err := repo.Create(ctx, rule2); err != nil {
		t.Fatalf("create rule2 failed: %v", err)
	}

	// List enabled only
	enabledRules, err := repo.List(ctx, true)
	if err != nil {
		t.Fatalf("list enabled failed: %v", err)
	}
	if len(enabledRules) != 1 || enabledRules[0].Name != "Google" {
		t.Errorf("unexpected enabled rules: %+v", enabledRules)
	}

	// List all
	allRules, err := repo.List(ctx, false)
	if err != nil {
		t.Fatalf("list all failed: %v", err)
	}
	if len(allRules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(allRules))
	}
	// Verify sort order: rule2 (sort_order=1) should come before rule1 (sort_order=5)
	if allRules[0].Name != "Direct-LAN" || allRules[1].Name != "Google" {
		t.Errorf("rules not sorted properly: first=%s, second=%s", allRules[0].Name, allRules[1].Name)
	}

	// ListByCategory
	streamingRules, err := repo.ListByCategory(ctx, "Streaming")
	if err != nil {
		t.Fatalf("list by category failed: %v", err)
	}
	if len(streamingRules) != 1 || streamingRules[0].Name != "Google" {
		t.Errorf("unexpected category rules: %+v", streamingRules)
	}
}

func TestProbeRepository_CRUD_And_BatchUpsert(t *testing.T) {
	db := setupTestDB(t)
	repo := db.NewProbeRepository()
	ctx := context.Background()

	latency := int64(45)
	speed := float64(128.5)
	asn := int64(13335)

	res := &domain.NodeProbeResult{
		NodeKey:      "hk-01|vless|1.1.1.1:443",
		Name:         "HK-01",
		Server:       "1.1.1.1",
		Port:         443,
		Type:         "vless",
		Status:       domain.ProbeStatusOK,
		LatencyMs:    &latency,
		SpeedMbps:    &speed,
		IP:           "1.1.1.1",
		Country:      "HK",
		ASN:          &asn,
		Organization: "Cloudflare",
		Media: domain.MediaUnlockInfo{
			Netflix: "HK",
			YouTube: "Yes",
		},
		CheckedAt: time.Now().Unix(),
	}

	if err := repo.Upsert(ctx, res); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	fetched, err := repo.GetByNodeKey(ctx, res.NodeKey)
	if err != nil {
		t.Fatalf("get by node key failed: %v", err)
	}
	if fetched.Status != domain.ProbeStatusOK || *fetched.LatencyMs != 45 || fetched.Media.Netflix != "HK" {
		t.Errorf("unexpected fetched probe result: %+v", fetched)
	}

	// Update via Upsert
	newLatency := int64(30)
	res.LatencyMs = &newLatency
	res.Status = domain.ProbeStatusOK
	if err := repo.Upsert(ctx, res); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	updated, err := repo.GetByNodeKey(ctx, res.NodeKey)
	if err != nil {
		t.Fatalf("get updated failed: %v", err)
	}
	if *updated.LatencyMs != 30 {
		t.Errorf("expected updated latency 30, got %d", *updated.LatencyMs)
	}

	// Batch Upsert
	res2 := &domain.NodeProbeResult{
		NodeKey:   "jp-01|ss|2.2.2.2:8388",
		Name:      "JP-01",
		Server:    "2.2.2.2",
		Port:      8388,
		Type:      "ss",
		Status:    domain.ProbeStatusOK,
		CheckedAt: time.Now().Unix(),
	}

	affected, err := repo.BatchUpsert(ctx, []*domain.NodeProbeResult{res, res2})
	if err != nil {
		t.Fatalf("batch upsert failed: %v", err)
	}
	if affected != 2 {
		t.Errorf("expected 2 affected, got %d", affected)
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 probe results, got %d", count)
	}

	// Pagination
	paged, err := repo.List(ctx, 1, 0)
	if err != nil {
		t.Fatalf("list page failed: %v", err)
	}
	if len(paged) != 1 {
		t.Errorf("expected 1 item on page, got %d", len(paged))
	}
}

func TestConfigRepositories(t *testing.T) {
	db := setupTestDB(t)
	genRepo := db.NewGenerateConfigRepository()
	dnsRepo := db.NewDNSConfigRepository()
	ctx := context.Background()

	// GenerateConfig default
	cfg, err := genRepo.Get(ctx)
	if err != nil {
		t.Fatalf("get generate config failed: %v", err)
	}
	if !cfg.Enabled || !cfg.Subscriptions {
		t.Errorf("unexpected default generate config: %+v", cfg)
	}

	cfg.ExcludeNodeProxies = false
	if err := genRepo.Update(ctx, cfg); err != nil {
		t.Fatalf("update generate config failed: %v", err)
	}

	cfgUpdated, err := genRepo.Get(ctx)
	if err != nil {
		t.Fatalf("get updated generate config failed: %v", err)
	}
	if cfgUpdated.ExcludeNodeProxies != false {
		t.Errorf("expected ExcludeNodeProxies false, got true")
	}

	// DNSConfig default
	dnsCfg, err := dnsRepo.Get(ctx)
	if err != nil {
		t.Fatalf("get dns config failed: %v", err)
	}
	if !dnsCfg.Enabled {
		t.Errorf("expected dns enabled by default")
	}

	dnsCfg.RawYAML = "listen: 0.0.0.0:53\nenhanced-mode: fake-ip"
	if err := dnsRepo.Update(ctx, dnsCfg); err != nil {
		t.Fatalf("update dns config failed: %v", err)
	}

	dnsUpdated, err := dnsRepo.Get(ctx)
	if err != nil {
		t.Fatalf("get updated dns config failed: %v", err)
	}
	if dnsUpdated.RawYAML != dnsCfg.RawYAML {
		t.Errorf("unexpected raw yaml: %q", dnsUpdated.RawYAML)
	}
}

func TestConcurrency_ReadWriteStress(t *testing.T) {
	db := setupTestDB(t)
	nodeRepo := db.NewNodeRepository()
	ctx := context.Background()

	var wg sync.WaitGroup
	workers := 20
	opsPerWorker := 30

	errCh := make(chan error, workers*opsPerWorker*2)

	for w := 0; w < workers; w++ {
		workerID := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				node := &domain.Node{
					Name:     fmt.Sprintf("node-w%d-%d", workerID, i),
					Protocol: domain.ProtocolShadowsocks,
					Server:   fmt.Sprintf("%d.%d.example.com", workerID, i),
					Port:     1000 + i,
					NormalizedPayload: map[string]any{
						"password": fmt.Sprintf("pass-%d", i),
						"cipher":   "aes-128-gcm",
					},
					LifecycleState: domain.LifecycleActive,
				}

				if err := nodeRepo.Create(ctx, node); err != nil {
					errCh <- fmt.Errorf("worker %d insert %d: %w", workerID, i, err)
					return
				}

				// Interleaved read
				if _, err := nodeRepo.GetByID(ctx, node.ID); err != nil {
					errCh <- fmt.Errorf("worker %d read %d: %w", workerID, i, err)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrency error: %v", err)
	}

	count, err := nodeRepo.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	expectedCount := int64(workers * opsPerWorker)
	if count != expectedCount {
		t.Errorf("expected count %d, got %d", expectedCount, count)
	}
}

func TestTransactionRollback(t *testing.T) {
	db := setupTestDB(t)
	nodeRepo := db.NewNodeRepository()
	ctx := context.Background()

	// 1. Initial count
	initialCount, err := nodeRepo.Count(ctx)
	if err != nil {
		t.Fatalf("initial count failed: %v", err)
	}

	// 2. Batch with an invalid node (violates NOT NULL or unique constraint)
	validNode1 := &domain.Node{
		Name:     "Valid1",
		Protocol: domain.ProtocolVLESS,
		Server:   "valid1.com",
		Port:     443,
		NormalizedPayload: map[string]any{
			"uuid": "u1",
		},
		LifecycleState: domain.LifecycleActive,
	}
	invalidNode := &domain.Node{
		Name:     "", // Empty name or invalid
		Protocol: "",
		Server:   "",
		Port:     0,
	}

	// Attempt batch upsert with invalid node
	_, err = nodeRepo.BatchUpsert(ctx, []*domain.Node{validNode1, invalidNode})
	if err == nil {
		t.Fatalf("expected batch upsert with invalid node to fail, but it succeeded")
	}

	// 3. Count must be unchanged because of atomic transaction rollback
	countAfter, err := nodeRepo.Count(ctx)
	if err != nil {
		t.Fatalf("count after rollback failed: %v", err)
	}
	if countAfter != initialCount {
		t.Fatalf("expected count %d after rollback, got %d (transaction did not rollback properly!)", initialCount, countAfter)
	}
}

func TestErrorCases(t *testing.T) {
	db := setupTestDB(t)
	repos := db.Repositories()
	ctx := context.Background()

	// 1. ErrNilEntity on all repos
	if err := repos.Subscriptions.Create(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on sub create, got %v", err)
	}
	if err := repos.Subscriptions.Update(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on sub update, got %v", err)
	}
	if err := repos.Nodes.Create(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on node create, got %v", err)
	}
	if err := repos.Nodes.Update(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on node update, got %v", err)
	}
	if err := repos.NodeGroups.Create(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on group create, got %v", err)
	}
	if err := repos.NodeGroups.Update(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on group update, got %v", err)
	}
	if err := repos.Rules.Create(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on rule create, got %v", err)
	}
	if err := repos.Rules.Update(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on rule update, got %v", err)
	}
	if err := repos.Probes.Upsert(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on probe upsert, got %v", err)
	}
	if err := repos.GenerateConfig.Update(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on gen config update, got %v", err)
	}
	if err := repos.DNSConfig.Update(ctx, nil); !errors.Is(err, repository.ErrNilEntity) {
		t.Errorf("expected ErrNilEntity on dns config update, got %v", err)
	}

	// 2. ErrNotFound on non-existent records
	if _, err := repos.Subscriptions.GetByID(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for sub 999999, got %v", err)
	}
	if _, err := repos.Subscriptions.GetByName(ctx, "nonexistent"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for sub name, got %v", err)
	}
	if err := repos.Subscriptions.Delete(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound on sub delete, got %v", err)
	}

	if _, err := repos.Nodes.GetByID(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for node 999999, got %v", err)
	}
	if _, err := repos.Nodes.GetByLogicalID(ctx, "nonexistent"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for node logical id, got %v", err)
	}
	if _, err := repos.Nodes.GetByFingerprint(ctx, "nonexistent"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for node fp, got %v", err)
	}
	if err := repos.Nodes.Delete(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound on node delete, got %v", err)
	}

	if _, err := repos.NodeGroups.GetByID(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for group 999999, got %v", err)
	}
	if _, err := repos.NodeGroups.GetByName(ctx, "nonexistent"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for group name, got %v", err)
	}
	if err := repos.NodeGroups.Delete(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound on group delete, got %v", err)
	}

	if _, err := repos.Rules.GetByID(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for rule 999999, got %v", err)
	}
	if err := repos.Rules.Delete(ctx, 999999); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound on rule delete, got %v", err)
	}

	if _, err := repos.Probes.GetByNodeKey(ctx, "nonexistent"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound for probe key, got %v", err)
	}

	// 3. ErrDuplicate on unique constraint
	sub1 := &domain.Subscription{
		Name: "UniqueSub",
		URL:  "https://example.com/sub1",
	}
	if err := repos.Subscriptions.Create(ctx, sub1); err != nil {
		t.Fatalf("create sub1: %v", err)
	}
	sub2 := &domain.Subscription{
		Name: "UniqueSub", // duplicate name
		URL:  "https://example.com/sub2",
	}
	if err := repos.Subscriptions.Create(ctx, sub2); !errors.Is(err, repository.ErrDuplicate) {
		t.Errorf("expected ErrDuplicate on duplicate sub name, got %v", err)
	}

	group1 := &domain.NodeGroup{
		Name:      "UniqueGroup",
		Kind:      domain.GroupKindManual,
		GroupType: domain.GroupTypeSelect,
	}
	if err := repos.NodeGroups.Create(ctx, group1); err != nil {
		t.Fatalf("create group1: %v", err)
	}
	group2 := &domain.NodeGroup{
		Name:      "UniqueGroup", // duplicate name
		Kind:      domain.GroupKindManual,
		GroupType: domain.GroupTypeSelect,
	}
	if err := repos.NodeGroups.Create(ctx, group2); !errors.Is(err, repository.ErrDuplicate) {
		t.Errorf("expected ErrDuplicate on duplicate group name, got %v", err)
	}
}

func TestRealBackupDB_Compatibility(t *testing.T) {
	// Dynamically locate the rehearsal database in backups/ without hardcoding absolute paths
	candidates := []string{
		filepath.Join("..", "..", "backups", "clash_sub_parser_pre_go_rewrite.db"),
		filepath.Join("backups", "clash_sub_parser_pre_go_rewrite.db"),
	}

	var foundPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			foundPath = p
			break
		}
	}

	if foundPath == "" {
		t.Skip("rehearsal database backups/clash_sub_parser_pre_go_rewrite.db not found, skipping real db test")
	}

	absPath, err := filepath.Abs(foundPath)
	if err != nil {
		t.Fatalf("resolve abs path: %v", err)
	}

	// Open strictly with ReadOnly = true
	opts := repository.Options{
		Path:         fmt.Sprintf("file:%s", filepath.ToSlash(absPath)),
		ReadOnly:     true,
		MaxOpenConns: 5,
		MaxIdleConns: 2,
		BusyTimeout:  30 * time.Second,
	}

	db, err := repository.NewSQLiteDB(opts)
	if err != nil {
		t.Fatalf("failed to open real rehearsal db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repos := db.Repositories()

	// 1. Verify Subscriptions count (target: 8)
	subCount, err := repos.Subscriptions.Count(ctx)
	if err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if subCount != 8 {
		t.Errorf("expected 8 subscriptions in rehearsal DB, got %d", subCount)
	}

	subs, err := repos.Subscriptions.List(ctx, false)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 8 {
		t.Errorf("expected 8 subscriptions list, got %d", len(subs))
	}

	// 2. Verify Nodes count (target: 7,236)
	nodeCount, err := repos.Nodes.Count(ctx)
	if err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if nodeCount != 7236 {
		t.Errorf("expected 7236 nodes in rehearsal DB, got %d", nodeCount)
	}

	// 3. Verify NodeGroups count (target: 29)
	groupCount, err := repos.NodeGroups.Count(ctx)
	if err != nil {
		t.Fatalf("count node groups: %v", err)
	}
	if groupCount != 29 {
		t.Errorf("expected 29 node groups in rehearsal DB, got %d", groupCount)
	}

	groups, err := repos.NodeGroups.List(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 29 {
		t.Errorf("expected 29 groups list, got %d", len(groups))
	}

	// 4. Verify Rules count (target: 470)
	ruleCount, err := repos.Rules.Count(ctx)
	if err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if ruleCount != 470 {
		t.Errorf("expected 470 rules in rehearsal DB, got %d", ruleCount)
	}

	rules, err := repos.Rules.List(ctx, false)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 470 {
		t.Errorf("expected 470 rules list, got %d", len(rules))
	}

	// 5. Verify ProbeResults count (target: 7,297)
	probeCount, err := repos.Probes.Count(ctx)
	if err != nil {
		t.Fatalf("count probe results: %v", err)
	}
	if probeCount != 7297 {
		t.Errorf("expected 7297 probe results in rehearsal DB, got %d", probeCount)
	}

	// Sample 5 probe results
	pagedProbes, err := repos.Probes.List(ctx, 5, 0)
	if err != nil {
		t.Fatalf("sample probe results: %v", err)
	}
	if len(pagedProbes) != 5 {
		t.Errorf("expected 5 sampled probes, got %d", len(pagedProbes))
	}

	// 6. Verify GenerateConfig
	genCfg, err := repos.GenerateConfig.Get(ctx)
	if err != nil {
		t.Fatalf("get generate config: %v", err)
	}
	if !genCfg.Enabled {
		t.Errorf("expected generate config enabled")
	}

	// 7. Verify DNSConfig
	dnsCfg, err := repos.DNSConfig.Get(ctx)
	if err != nil {
		t.Fatalf("get dns config: %v", err)
	}
	if !dnsCfg.Enabled {
		t.Errorf("expected dns config enabled")
	}

	t.Logf("Rehearsal DB Compatibility: 8 subs, 7236 nodes, 29 groups, 470 rules, 7297 probes verified 100%% successfully")
}
