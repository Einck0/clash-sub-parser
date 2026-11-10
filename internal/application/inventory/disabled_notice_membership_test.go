package inventory_test

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

// Disabled subscriptions must neither exclude an enabled shared proxy with a
// notice nor rescue an enabled notice with an explicit proxy override. Legacy
// all-assets classification and disabled source counts still retain that data.
func TestDisabledNoticeMembershipSharedConsumers(t *testing.T) {
	for _, tc := range []struct {
		name          string
		disableNotice bool
	}{
		{name: "disabled notice cannot exclude enabled proxy", disableNotice: true},
		{name: "disabled explicit proxy cannot rescue enabled notice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			db, subs, fetches, nodes, sources := setupTestEnv(t)
			f := newMockFetcher()
			svc := inventory.NewService(db, subs, fetches, nodes, sources, f)
			noticeSub, proxySub := domain.MustNewUUIDv7(), domain.MustNewUUIDv7()
			const noticeURL, proxyURL = "https://disabled-notice.invalid/", "https://enabled-proxy.invalid/"
			createTestSub(t, subs, noticeSub, "Notice source", noticeURL, true)
			createTestSub(t, subs, proxySub, "Proxy source", proxyURL, true)
			const body = "proxies:\n  - {name: Shared, type: ss, server: 192.0.2.21, port: 8388, cipher: aes-128-gcm, password: shared}\n"
			for _, source := range []struct{ id, url string }{{noticeSub, noticeURL}, {proxySub, proxyURL}} {
				f.setResponse(source.url, &fetch.Response{StatusCode: 200, Body: []byte(body), ContentDigest: "shared-" + source.id})
				result, err := svc.ReconcileSubscription(ctx, source.id)
				if err != nil || result == nil || result.Outcome != domain.FetchOutcomeSuccess {
					t.Fatalf("reconcile %s: result=%+v error=%v", source.id, result, err)
				}
			}
			entries := sqlite.NewSubscriptionEntryRepository(db)
			entryFor := func(id string) domain.SubscriptionEntry {
				t.Helper()
				items, err := entries.ListLatestBySubscription(ctx, id)
				if err != nil || len(items) != 1 || items[0].NodeLogicalID == nil {
					t.Fatalf("entries %s: %+v error=%v", id, items, err)
				}
				return items[0]
			}
			noticeEntry, proxyEntry := entryFor(noticeSub), entryFor(proxySub)
			if *noticeEntry.NodeLogicalID != *proxyEntry.NodeLogicalID {
				t.Fatal("fixture must share one logical node")
			}
			override := func(entry domain.SubscriptionEntry, kind domain.EntryKind) {
				t.Helper()
				if _, err := svc.OverrideEntryKind(ctx, entry.ID, &kind, "synthetic classification", "test"); err != nil {
					t.Fatal(err)
				}
			}
			override(noticeEntry, domain.EntryKindNotice)
			if !tc.disableNotice {
				override(proxyEntry, domain.EntryKindProxy)
			}
			disabledID, enabledID := proxySub, noticeSub
			beforeCount, afterCount := 1, 0
			if tc.disableNotice {
				disabledID, enabledID = noticeSub, proxySub
				beforeCount, afterCount = 0, 1
			}
			if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET is_primary = 1 WHERE id = ?", enabledID); err != nil {
				t.Fatal(err)
			}
			rev := &domain.ConfigurationRevision{ID: domain.MustNewUUIDv7(), ContentDigest: "disabled-notice-membership", State: domain.RevisionStateActive, CreatedAt: domain.NowUTC()}
			if err := sqlite.NewRevisionRepository(db).Create(ctx, rev); err != nil {
				t.Fatal(err)
			}
			loader := sqlite.NewConsistentFactsLoader(db)
			assertCurrent := func(stage string, want int) {
				t.Helper()
				for _, id := range []string{"", enabledID} {
					views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, SubscriptionID: id, Pagination: domain.Pagination{Page: 1, PageSize: 50}})
					if err != nil || total != want || len(views) != want {
						t.Errorf("%s enabled read model %s: count=%d total=%d want=%d error=%v", stage, id, len(views), total, want, err)
					}
				}
				sub, err := subs.GetByID(ctx, enabledID)
				if err != nil || sub == nil {
					t.Fatalf("%s enabled subscription: %v", stage, err)
				}
				if sub.SourceNodeCount != want || sub.NodeCount != want {
					t.Errorf("%s enabled counts: source=%d enabled=%d want=%d", stage, sub.SourceNodeCount, sub.NodeCount, want)
				}
				facts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetSingBox, domain.NowUTC())
				if err != nil || facts == nil {
					t.Fatalf("%s facts: %v", stage, err)
				}
				if len(facts.Nodes) != want || len(facts.PrimaryNodeIDs) != want {
					t.Errorf("%s publication facts: nodes=%d primary=%d want=%d", stage, len(facts.Nodes), len(facts.PrimaryNodeIDs), want)
				}
				for _, node := range facts.Nodes {
					if node.LogicalID != *proxyEntry.NodeLogicalID {
						t.Errorf("%s unexpected facts node %s", stage, node.LogicalID)
					}
					wantSources := 2
					if stage == "one disabled" {
						wantSources = 1
					}
					if len(facts.NodeSources[node.LogicalID]) != wantSources {
						t.Errorf("%s facts sources: count=%d want=%d", stage, len(facts.NodeSources[node.LogicalID]), wantSources)
					}
					for _, source := range facts.NodeSources[node.LogicalID] {
						sub, err := subs.GetByID(ctx, source.SubscriptionID)
						if err != nil || sub == nil || !sub.Enabled {
							t.Errorf("%s facts included disabled source %s: %v", stage, source.SubscriptionID, err)
						}
					}
				}
			}
			assertCurrent("both enabled", beforeCount)
			if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET enabled = 0 WHERE id = ?", disabledID); err != nil {
				t.Fatal(err)
			}
			assertCurrent("one disabled", afterCount)
			// Explicit all-assets exclusion keeps the legacy cross-subscription
			// override contract, unlike enabled inventory and publication facts.
			for _, scope := range []domain.NodeScope{domain.NodeScopeDefault, domain.NodeScopeAllAssets} {
				assets, err := nodes.ListAll(ctx, domain.NodeFilter{Scope: scope, ActiveOnly: true, ExcludeNotices: true})
				if err != nil || len(assets) != beforeCount {
					t.Errorf("legacy explicit classification %q: nodes=%d want=%d error=%v", scope, len(assets), beforeCount, err)
				}
			}
			disabled, err := subs.GetByID(ctx, disabledID)
			if err != nil || disabled == nil {
				t.Fatalf("disabled subscription: %v", err)
			}
			if disabled.SourceNodeCount != beforeCount || disabled.NodeCount != 0 {
				t.Errorf("disabled counts: source=%d enabled=%d want source=%d enabled=0", disabled.SourceNodeCount, disabled.NodeCount, beforeCount)
			}
			// Toggling enabled is not a destructive reconciliation or override clear.
			ledger, err := nodes.ListAll(ctx, domain.NodeFilter{Scope: domain.NodeScopeAllAssets, ActiveOnly: true})
			if err != nil || len(ledger) != 1 {
				t.Errorf("historical active node: count=%d want=1 error=%v", len(ledger), err)
			}
			if !tc.disableNotice {
				retainedProxy, err := entries.GetByID(ctx, proxyEntry.ID)
				if err != nil || retainedProxy == nil || retainedProxy.UserKindOverride == nil || *retainedProxy.UserKindOverride != domain.EntryKindProxy {
					t.Fatalf("proxy override lost: entry=%+v error=%v", retainedProxy, err)
				}
			}
			for _, entry := range []domain.SubscriptionEntry{noticeEntry, proxyEntry} {
				retained, err := entries.GetByID(ctx, entry.ID)
				if err != nil || retained == nil {
					t.Fatalf("retained entry %s missing: %v", entry.ID, err)
				}
				var count int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_sources WHERE subscription_id = ? AND node_logical_id = ?", entry.SubscriptionID, *entry.NodeLogicalID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("retained source %s: count=%d error=%v", entry.SubscriptionID, count, err)
				}
			}
			retainedNotice, err := entries.GetByID(ctx, noticeEntry.ID)
			if err != nil || retainedNotice == nil || retainedNotice.UserKindOverride == nil || *retainedNotice.UserKindOverride != domain.EntryKindNotice {
				t.Fatalf("notice override lost: entry=%+v error=%v", retainedNotice, err)
			}
			if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET enabled = 1 WHERE id = ?", disabledID); err != nil {
				t.Fatal(err)
			}
			assertCurrent("reenabled", beforeCount)
		})
	}
}
