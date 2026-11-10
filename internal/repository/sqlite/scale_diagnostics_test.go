package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

// TestScaleDiagnostics measures the real list predicate against synthetic retained
// history. Fixture construction is deliberately timed separately from reads.
func TestScaleDiagnostics(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stop()
	started := time.Now()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stamp := now.Format(time.RFC3339)
	for _, query := range []string{
		`INSERT INTO configuration_revisions (id,content_digest,state,created_at) VALUES ('scale-rev','synthetic-scale','active','2026-01-01T00:00:00Z')`,
		`INSERT INTO subscriptions (id,name,source_url_secret_ref,enabled,is_primary,revision,created_at,updated_at) VALUES ('scale','scale','synthetic',1,1,'1','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
		`INSERT INTO subscription_fetches (id,subscription_id,started_at,outcome,nodes_valid) VALUES ('old','scale','2025-01-01T00:00:00Z','success',1000), ('current','scale','2026-01-01T00:00:00Z','success',1000)`,
		`INSERT INTO subscription_payloads (id,subscription_id,fetch_id,content_digest,body_blob,pinned,created_at) VALUES ('old-p','scale','old','old',X'00',0,'2025-01-01T00:00:00Z'), ('current-p','scale','current','current',X'00',1,'2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	nodes := make([]domain.Node, 1000)
	for i := range nodes {
		nodes[i] = domain.Node{LogicalID: fmt.Sprintf("scale-%04d", i), Protocol: domain.ProtocolSS, DisplayName: fmt.Sprintf("node-%04d", i), Server: "192.0.2.1", Port: 10000 + i, Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic"}, Active: true, ConnectionRevision: 1, CreatedAt: now, UpdatedAt: now}
	}
	repo := NewNodeRepository(db)
	if err := repo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := range nodes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id,subscription_id,last_seen_fetch_id) VALUES (?,'scale','current')`, nodes[i].LogicalID); err != nil {
			t.Fatal(err)
		}
		kind := "proxy"
		if i == 0 {
			kind = "notice"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at) VALUES (?,'current-p','scale',?,?,?,'ss',?,?,?)`, fmt.Sprintf("current-%d", i), i, fmt.Sprintf("key-%d", i), nodes[i].DisplayName, kind, nodes[i].LogicalID, stamp); err != nil {
			t.Fatal(err)
		}
	}
	// Recursive INSERT produces bounded synthetic historical rows in one transaction.
	if _, err := tx.ExecContext(ctx, `WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<999999) INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,created_at) SELECT printf('history-%d',n),'old-p','scale',n,printf('old-%d',n),'old','ss','proxy',? FROM seq`, stamp); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture_setup=%s nodes=%d historical_entries=%d", time.Since(started), len(nodes), 1000000)

	rows, err := db.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT n.logical_id FROM nodes n WHERE n.logical_id IN (`+enabledSubscriptionsInventorySubquery+`) AND n.logical_id NOT IN (`+confirmedNoticeExclusionSubquery+`) AND n.active=1`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		t.Logf("plan id=%d parent=%d %s", id, parent, detail)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	subs := NewSubscriptionRepository(db)
	for _, stage := range []string{"get", "list"} {
		before := db.Stats()
		started := time.Now()
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		var err error
		if stage == "get" {
			var sub *domain.Subscription
			sub, err = subs.GetByID(readCtx, "scale")
			if err == nil && (sub == nil || sub.ID != "scale") {
				t.Fatal("unexpected subscription from get")
			}
		} else {
			var items []domain.Subscription
			var total int
			items, total, err = subs.List(readCtx, domain.SubscriptionFilter{})
			if err == nil && (total != 1 || len(items) != 1 || items[0].ID != "scale") {
				t.Fatalf("unexpected subscription list: total=%d items=%v", total, items)
			}
		}
		cancel()
		after := db.Stats()
		t.Logf("stage=%s elapsed=%s pool_waits=%d pool_wait_duration=%s error=%v", stage, time.Since(started), after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration, err)
		if err != nil {
			t.Fatal(err)
		}
	}
	filter := domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, ExcludeNotices: true}
	for i := 0; i < 2; i++ {
		before := db.Stats()
		readStart := time.Now()
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		got, err := repo.ListAll(readCtx, filter)
		cancel()
		after := db.Stats()
		t.Logf("list_iteration=%d elapsed=%s pool_waits=%d pool_wait_duration=%s error=%v", i, time.Since(readStart), after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration, err)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 999 {
			t.Fatalf("expected 999 current proxy nodes, got %d", len(got))
		}
	}
	// Exercise the actual publication facts consumer, not just an equivalent
	// list predicate. A primary subscription forces source loading as well.
	loader := NewConsistentFactsLoader(db)
	for i := 0; i < 2; i++ {
		before := db.Stats()
		readStart := time.Now()
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		facts, err := loader.LoadConsistentFacts(readCtx, "scale-rev", domain.TargetSingBox, now)
		cancel()
		after := db.Stats()
		t.Logf("facts_iteration=%d elapsed=%s pool_waits=%d pool_wait_duration=%s error=%v", i, time.Since(readStart), after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration, err)
		if err != nil {
			t.Fatal(err)
		}
		if facts == nil || len(facts.Nodes) != 999 || len(facts.NodeSources) != 999 || len(facts.PrimaryNodeIDs) != 999 || facts.PrimarySubscriptionID != "scale" {
			t.Fatal("facts must contain 999 current proxies and their primary sources")
		}
		for _, node := range facts.Nodes {
			if node.LogicalID == nodes[0].LogicalID {
				t.Fatal("confirmed notice leaked into publication facts")
			}
			src := facts.NodeSources[node.LogicalID]
			if len(src) != 1 || src[0].SubscriptionID != "scale" || src[0].LastSeenFetchID != "current" {
				t.Fatalf("unexpected facts source for %s: %+v", node.LogicalID, src)
			}
			if node.Credentials.Method != "aes-128-gcm" || node.Credentials.Password != "synthetic" {
				t.Fatalf("facts credentials missing for %s", node.LogicalID)
			}
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.ListAll(cancelled, filter); err == nil {
		t.Fatal("cancelled list unexpectedly succeeded")
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("database unavailable after cancellation: %v", err)
	}
}
