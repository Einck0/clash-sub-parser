package inventory_test

import (
	"context"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

func TestPrefixUpdateAtomicRollbackAndStaleRevision(t *testing.T) {
	for _, failure := range []string{"subscription", "node", "stale"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			db, subs, fetches, nodes, sources := setupTestEnv(t)
			fetcher := newMockFetcher()
			url := "https://atomic.example/feed"
			fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(clashYAMLSharedNode), ContentDigest: "atomic"})
			svc := subscription.NewService(subs, nil)
			created, err := svc.Create(ctx, subscription.CreateSubscriptionCommand{Name: "Atomic", SourceURLSecretRef: url, Prefix: "A ", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			inv := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
			if _, err := inv.ReconcileSubscription(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
			before, err := nodes.ListAll(ctx, domain.NodeFilter{})
			if err != nil || len(before) != 1 {
				t.Fatalf("nodes: %v %v", before, err)
			}
			revision := created.Revision
			switch failure {
			case "subscription":
				_, err = db.Exec(`CREATE TRIGGER fail_subscription BEFORE UPDATE ON subscriptions BEGIN SELECT RAISE(ABORT, 'subscription write rejected'); END`)
			case "node":
				_, err = db.Exec(`CREATE TRIGGER fail_name BEFORE UPDATE OF display_name ON nodes BEGIN SELECT RAISE(ABORT, 'node write rejected'); END`)
			case "stale":
				revision = "stale"
			}
			if err != nil {
				t.Fatal(err)
			}
			prefix := "B "
			if _, err := svc.Update(ctx, subscription.UpdateSubscriptionCommand{ID: created.ID, Revision: revision, Prefix: &prefix}); err == nil {
				t.Fatal("expected save failure")
			}
			afterSub, err := subs.GetByID(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			after, err := nodes.GetByLogicalID(ctx, before[0].LogicalID)
			if err != nil {
				t.Fatal(err)
			}
			if afterSub.Prefix != "A " || afterSub.Revision != created.Revision || after.DisplayName != before[0].DisplayName || after.ConnectionRevision != before[0].ConnectionRevision {
				t.Fatalf("failed save mutated source/node: prefix=%q rev=%s name=%q conn=%d", afterSub.Prefix, afterSub.Revision, after.DisplayName, after.ConnectionRevision)
			}
		})
	}
}

func TestSharedPrefixStableAcrossImportSaveRefreshAndDelete(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	fetcher := newMockFetcher()
	inv := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
	now := domain.NowUTC()
	for _, id := range []string{"source-b", "source-a"} {
		prefix, raw := "B ", "Second"
		if id == "source-a" {
			prefix, raw = "A ", "First"
		}
		url := "https://" + id + ".example/feed"
		body := strings.Replace(clashYAMLSharedNode, "Shared Node Alpha", raw, 1)
		fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(body), ContentDigest: id})
		if err := subs.Create(ctx, &domain.Subscription{ID: id, Name: id, SourceURLSecretRef: url, Prefix: prefix, Enabled: true, Revision: id, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if _, err := inv.ReconcileSubscription(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	list, err := nodes.ListAll(ctx, domain.NodeFilter{})
	if err != nil || len(list) != 1 {
		t.Fatalf("shared node identity split: %v %v", list, err)
	}
	id, revision := list[0].LogicalID, list[0].ConnectionRevision
	// Seed a valid existing connection head/version; prefix-only writes must preserve both.
	if _, err := db.Exec(`INSERT INTO node_connection_versions(node_logical_id, connection_revision, effective_config_json, config_fingerprint, created_at) VALUES (?, ?, '{}', 'fixture-connection', ?)`, id, revision, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO node_connection_heads(logical_id, connection_revision, updated_at) VALUES (?, ?, ?)`, id, revision, now); err != nil {
		t.Fatal(err)
	}
	var versionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM node_connection_versions WHERE node_logical_id = ?`, id).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	assertName := func(want string) {
		t.Helper()
		n, err := nodes.GetByLogicalID(ctx, id)
		if err != nil || n.DisplayName != want || n.ConnectionRevision != revision {
			t.Fatalf("want name=%q rev=%d, got %+v err=%v", want, revision, n, err)
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM node_connection_versions WHERE node_logical_id = ?`, id).Scan(&count); err != nil || count != versionCount {
			t.Fatalf("connection churn count=%d err=%v", count, err)
		}
		var head int64
		if err := db.QueryRow(`SELECT connection_revision FROM node_connection_heads WHERE logical_id = ?`, id).Scan(&head); err != nil || head != revision {
			t.Fatalf("head changed: %d %v", head, err)
		}
	}
	assertName("A B First")
	a, _ := subs.GetByID(ctx, "source-a")
	a.Prefix = "B "
	if err := subs.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	assertName("B First") // identical labels deduplicated
	a.Prefix = ""
	if err := subs.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	assertName("B First") // only this owner's label removed
	if _, err := inv.ReconcileSubscription(ctx, "source-b"); err != nil {
		t.Fatal(err)
	}
	assertName("B First") // refresh order never wins
	var entryID, raw, key string
	if err := db.QueryRow(`SELECT id, raw_name, source_key FROM subscription_entries WHERE subscription_id = 'source-a' ORDER BY rowid DESC LIMIT 1`).Scan(&entryID, &raw, &key); err != nil {
		t.Fatal(err)
	}
	if raw != "First" || !strings.HasSuffix(key, "|First") {
		t.Fatalf("synthetic prefix contaminated raw anchors: %q %q", raw, key)
	}
	kind := domain.EntryKindProxy
	if _, err := inv.OverrideEntryKind(ctx, entryID, &kind, "keep raw anchor", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.ReconcileSubscription(ctx, "source-a"); err != nil {
		t.Fatal(err)
	}
	var overrideKind, anchor, reason string
	if err := db.QueryRow(`SELECT user_kind_override, override_anchor, override_reason FROM subscription_entries WHERE subscription_id = 'source-a' ORDER BY rowid DESC LIMIT 1`).Scan(&overrideKind, &anchor, &reason); err != nil {
		t.Fatal(err)
	}
	if overrideKind != "proxy" || anchor != key || reason != "keep raw anchor" {
		t.Fatalf("raw classification override corrupted: %q %q %q", overrideKind, anchor, reason)
	}
	assertName("B First")
	// Latest raw entry from the first source supplies the base; another owner's label
	// appearing naturally in that base must not be stripped.
	fetcher.setResponse("https://source-a.example/feed", &fetch.Response{StatusCode: 200, Body: []byte(strings.Replace(clashYAMLSharedNode, "Shared Node Alpha", "B First latest", 1)), ContentDigest: "latest-a"})
	if _, err := inv.ReconcileSubscription(ctx, "source-a"); err != nil {
		t.Fatal(err)
	}
	assertName("B B First latest")
	// A user display-name override always outranks source labels, including a newly shared source.
	if _, err := db.Exec(`INSERT INTO node_overrides(node_logical_id, field_path, override_value_json, created_at, updated_at) VALUES (?, 'display_name', '"My chosen name"', ?, ?)`, id, now, now); err != nil {
		t.Fatal(err)
	}
	a.Prefix = "C "
	if err := subs.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	assertName("My chosen name")
	if _, err := inv.ReconcileSubscription(ctx, "source-b"); err != nil {
		t.Fatal(err)
	}
	assertName("My chosen name")
	if _, err := db.Exec(`DELETE FROM node_overrides WHERE node_logical_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if err := subs.Delete(ctx, "source-a"); err != nil {
		t.Fatal(err)
	}
	assertName("B Second")
	if err := subs.Delete(ctx, "source-b"); err != nil {
		t.Fatal(err)
	}
	n, _ := nodes.GetByLogicalID(ctx, id)
	if n.Active {
		t.Fatal("subscription-only orphan remains active after deletion")
	}
}

func TestPrunedNodesAreNotManualButConfirmedRecoveryIs(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	fetcher := newMockFetcher()
	url := "https://prune.example/feed"
	now := domain.NowUTC()
	sub := &domain.Subscription{ID: "prune-source", Name: "Prune", SourceURLSecretRef: url, Enabled: true, Revision: "rev", CreatedAt: now, UpdatedAt: now}
	if err := subs.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(clashYAMLSharedNode), ContentDigest: "before"})
	inv := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
	if _, err := inv.ReconcileSubscription(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := nodes.ListAll(ctx, domain.NodeFilter{})
	prunedID := before[0].LogicalID
	// Use a distinct name and endpoint so this is a new node, not an in-place source update.
	fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(strings.ReplaceAll(strings.ReplaceAll(clashYAMLSharedNode, "Shared Node Alpha", "Replacement"), "198.51.100.1", "198.51.100.99")), ContentDigest: "after"})
	if _, err := inv.ReconcileSubscription(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	pruned, _ := nodes.GetByLogicalID(ctx, prunedID)
	if !pruned.Active {
		t.Fatal("refresh deactivated retained asset")
	}
	manual := domain.Node{LogicalID: "manual", Protocol: domain.ProtocolWireGuard, DisplayName: "Manual WARP", Server: "warp.example", Port: 2408, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := nodes.UpsertBatch(ctx, []domain.Node{manual}); err != nil {
		t.Fatal(err)
	}
	assertScope := func(wantPruned bool) {
		t.Helper()
		list, err := nodes.ListAll(ctx, domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		found := make(map[string]bool)
		for _, n := range list {
			found[n.LogicalID] = true
		}
		if !found["manual"] || found[prunedID] != wantPruned {
			t.Fatalf("wrong manual/pruned eligibility: %v", found)
		}
	}
	assertScope(false)
	// Entries alone must prevent the no-source branch from re-admitting subscription assets.
	if _, err := db.Exec(`DELETE FROM node_source_history WHERE node_logical_id = ?`, prunedID); err != nil {
		t.Fatal(err)
	}
	assertScope(false)
	// History alone is also origin evidence after raw entry cleanup.
	if _, err := db.Exec(`DELETE FROM subscription_entries WHERE node_logical_id = ?`, prunedID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO node_source_history(id,node_logical_id,source_identity,source_label,relation_state,cause,evidence_kind,evidence_key,created_at) VALUES ('history-only',?,'sub:prune-source','old subscription','verified','refresh_removed','fixture','history-only',?)`, prunedID, now); err != nil {
		t.Fatal(err)
	}
	assertScope(false)
	_, err := db.Exec(`INSERT INTO node_source_history(id,node_logical_id,source_identity,source_label,relation_state,cause,evidence_kind,evidence_key,created_at) VALUES ('recovery',?,'manual','recovered','verified','manual_confirmed','fixture','recovery',?)`, prunedID, now)
	if err != nil {
		t.Fatal(err)
	}
	assertScope(true)
	if err := subs.Delete(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	assertScope(true)
	for _, id := range []string{"manual", prunedID} {
		n, err := nodes.GetByLogicalID(ctx, id)
		if err != nil || !n.Active {
			t.Fatalf("manual recovery damaged: %s %v", id, err)
		}
	}
}
