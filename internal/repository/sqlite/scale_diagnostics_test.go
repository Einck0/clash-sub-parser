package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

// TestScaleDiagnostics measures the real list predicate against synthetic retained
// history. Fixture construction is deliberately timed separately from reads.
func TestScaleDiagnostics(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stop()
	started := time.Now()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "scale.db"))
	db, err := OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	var mode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" || db.Stats().MaxOpenConnections != 1 {
		t.Fatalf("expected file WAL/default single connection: mode=%q stats=%+v err=%v", mode, db.Stats(), err)
	}
	t.Logf("stage=open_and_migrate elapsed=%s", time.Since(started))
	started = time.Now()
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
		nodes[i] = domain.Node{LogicalID: fmt.Sprintf("scale-%04d", i), Protocol: domain.ProtocolSS, DisplayName: fmt.Sprintf("node-%04d", i), Server: "192.0.2.1", Port: 10000 + i, Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic"}, Active: true, ConnectionRevision: 1, CreatedAt: now.Add(time.Duration(i) * time.Second), UpdatedAt: now}
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
	// Two additional retained sources, with four payloads each, distribute a
	// million historical rows. Every row is associated with a real node; old
	// notice evidence must not affect the current source's inventory.
	for source := 0; source < 2; source++ {
		subID := fmt.Sprintf("history-sub-%d", source)
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscriptions (id,name,source_url_secret_ref,enabled,revision,created_at,updated_at) VALUES (?,?,'synthetic',1,'1',?,?)`, subID, subID, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		for payload := 0; payload < 4; payload++ {
			id := fmt.Sprintf("history-%d-%d", source, payload)
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_fetches (id,subscription_id,started_at,outcome,nodes_valid) VALUES (?,?,?,'success',1000)`, id, subID, stamp); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_payloads (id,subscription_id,fetch_id,content_digest,body_blob,created_at) VALUES (?,?,?,?,X'00',?)`, id, subID, id, id, stamp); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.ExecContext(ctx, `WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<124999)
				INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at)
				SELECT ? || '-' || n,?,?,n,printf('old-%d',n),'old','ss',CASE WHEN n%1000=0 THEN 'notice' ELSE 'proxy' END,printf('scale-%04d',n%1000),? FROM seq`, id, id, subID, stamp); err != nil {
				t.Fatal(err)
			}
		}
		// A newer useful payload makes every historical payload noncurrent.
		id := subID + "-current"
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_fetches (id,subscription_id,started_at,outcome,nodes_valid) VALUES (?,?,?,'success',1)`, id, subID, now.Add(time.Hour).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_payloads (id,subscription_id,fetch_id,content_digest,body_blob,created_at) VALUES (?,?,?,?,X'00',?)`, id, subID, id, id, now.Add(time.Hour).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_sources (node_logical_id,subscription_id,last_seen_fetch_id) VALUES ('scale-0003',?,?)`, subID, id); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_entries (id,payload_id,subscription_id,ordinal,source_key,raw_name,protocol,entry_kind,node_logical_id,created_at) VALUES (?,?,?,0,'shared','shared','ss','notice','scale-0003',?)`, id, id, subID, stamp); err != nil {
			t.Fatal(err)
		}
	}
	// Explicit proxy rescue wins over another source's notice; explicit notice
	// hides an otherwise ordinary proxy. Exact IDs, not just counts, are checked.
	for _, override := range []struct{ anchor, kind string }{{"key-1", "notice"}, {"key-2", "proxy"}, {"key-3", "proxy"}} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_entry_overrides (subscription_id,override_anchor,user_kind_override,override_at) VALUES ('scale',?,?,?)`, override.anchor, override.kind, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE subscription_entries SET entry_kind='notice' WHERE id='current-2'`); err != nil {
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
			if err == nil && (total != 3 || len(items) != 3) {
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
	// List performs the production COUNT plus paginated SELECT on every page.
	// Unique created_at values make the coordinator's default ordering definite.
	var paginated []domain.Node
	for page := 1; page <= 11; page++ {
		pageFilter := filter
		pageFilter.Pagination = domain.Pagination{Page: page, PageSize: 100}
		before, started := db.Stats(), time.Now()
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		items, total, err := repo.List(readCtx, pageFilter)
		cancel()
		after := db.Stats()
		t.Logf("stage=list_count_page page=%d elapsed=%s pool_waits=%d pool_wait_duration=%s error=%v", page, time.Since(started), after.WaitCount-before.WaitCount, after.WaitDuration-before.WaitDuration, err)
		if err != nil || total != 998 {
			t.Fatalf("page %d total=%d error=%v", page, total, err)
		}
		wantLen := 100
		if page == 10 {
			wantLen = 98
		} else if page == 11 {
			wantLen = 0
		}
		if len(items) != wantLen {
			t.Fatalf("page %d length=%d want=%d", page, len(items), wantLen)
		}
		paginated = append(paginated, items...)
	}
	for i, node := range paginated {
		if want := fmt.Sprintf("scale-%04d", 999-i); node.LogicalID != want {
			t.Fatalf("paginated ID at %d=%s want=%s", i, node.LogicalID, want)
		}
	}
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
		if len(got) != len(paginated) {
			t.Fatalf("ListAll count=%d paginated=%d", len(got), len(paginated))
		}
		for j, node := range got {
			if node.LogicalID != paginated[j].LogicalID {
				t.Fatalf("ListAll/pagination mismatch at %d: %s vs %s", j, node.LogicalID, paginated[j].LogicalID)
			}
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
		if facts == nil || len(facts.Nodes) != 998 || len(facts.NodeSources) != 998 || len(facts.PrimaryNodeIDs) != 998 || facts.PrimarySubscriptionID != "scale" {
			t.Fatal("facts must contain 998 current proxies and their primary sources")
		}
		seen := make(map[string]bool)
		for _, node := range facts.Nodes {
			if node.LogicalID < "scale-0002" || node.LogicalID > "scale-0999" || seen[node.LogicalID] {
				t.Fatalf("unexpected or duplicate facts node %s", node.LogicalID)
			}
			seen[node.LogicalID] = true
			src := facts.NodeSources[node.LogicalID]
			wantSources := 1
			if node.LogicalID == "scale-0003" {
				wantSources = 3
			}
			if len(src) != wantSources {
				t.Fatalf("unexpected facts sources for %s: %+v", node.LogicalID, src)
			}
			foundPrimary := false
			for _, source := range src {
				if source.SubscriptionID == "scale" && source.LastSeenFetchID == "current" {
					foundPrimary = true
				}
			}
			if !foundPrimary {
				t.Fatalf("missing primary source for %s", node.LogicalID)
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	db, err = OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stage=reopen_and_migrate elapsed=%s", time.Since(started))
	items, total, err := NewNodeRepository(db).List(ctx, domain.NodeFilter{
		Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, ExcludeNotices: true,
		Pagination: domain.Pagination{Page: 10, PageSize: 100},
	})
	if err != nil || total != 998 || len(items) != 98 || items[97].LogicalID != "scale-0002" {
		t.Fatalf("reopened pagination: total=%d length=%d error=%v", total, len(items), err)
	}
}
