package inventory_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

// A partial refresh replaces the subscription's membership with its accepted
// subset. Shared and explicitly confirmed manual nodes remain available.
func TestCurrentBehavior_PartialRefreshInventoryFactsAndPreview(t *testing.T) {
	const nodeA = "  - {name: A, type: ss, server: 192.0.2.11, port: 8388, cipher: aes-128-gcm, password: synthetic-a}\n"
	const nodeB = "  - {name: B, type: ss, server: 192.0.2.12, port: 8388, cipher: aes-128-gcm, password: synthetic-b}\n"
	const nodeC = "  - {name: C, type: ss, server: 192.0.2.13, port: 8388, cipher: aes-128-gcm, password: synthetic-c}\n"
	const rejected = "  - {name: Rejected, type: ss, server: '', port: 0}\n"

	for _, sharedB := range []bool{false, true} {
		name := "sole_subscription"
		if sharedB {
			name = "shared_B"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			db, subs, fetches, nodes, sources := setupTestEnv(t)
			fetcher := newMockFetcher()
			svc := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
			id := domain.MustNewUUIDv7()
			url := "https://partial-current.invalid/feed"
			createTestSub(t, subs, id, "Current behavior", url, true)
			fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte("proxies:\n" + nodeA + nodeB), ContentDigest: "synthetic-full-ab"})
			initial, err := svc.ReconcileSubscription(ctx, id)
			if err != nil || initial == nil || initial.Outcome != domain.FetchOutcomeSuccess || initial.NodesValid != 2 {
				t.Fatalf("full A/B: result=%+v error=%v", initial, err)
			}
			full, err := nodes.ListAll(ctx, domain.NodeFilter{ActiveOnly: true})
			if err != nil || len(full) != 2 {
				t.Fatalf("initial ledger: nodes=%d error=%v", len(full), err)
			}
			ids := map[string]string{}
			for _, n := range full {
				ids[n.DisplayName] = n.LogicalID
			}
			if ids["A"] == "" || ids["B"] == "" {
				t.Fatalf("initial names missing: %v", ids)
			}

			now := time.Now().UTC()
			manual := domain.Node{
				LogicalID:   "manual-partial-consumer",
				Protocol:    domain.ProtocolSS,
				DisplayName: "Manual",
				Server:      "192.0.2.14",
				Port:        8388,
				Credentials: domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic-manual"},
				Active:      true,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := nodes.UpsertBatch(ctx, []domain.Node{manual}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO node_source_history(id,node_logical_id,source_identity,source_label,relation_state,cause,evidence_kind,evidence_key,created_at) VALUES ('manual-partial-fixture',?,'manual','manual','verified','manual_confirmed','fixture','manual-partial-fixture',?)`, manual.LogicalID, now); err != nil {
				t.Fatal(err)
			}
			ids["Manual"] = manual.LogicalID

			var otherID, otherFetchID string
			if sharedB {
				otherID = domain.MustNewUUIDv7()
				otherURL := "https://shared-b.invalid/feed"
				createTestSub(t, subs, otherID, "Shared B", otherURL, true)
				fetcher.setResponse(otherURL, &fetch.Response{StatusCode: 200, Body: []byte("proxies:\n" + nodeB), ContentDigest: "synthetic-shared-b"})
				res, err := svc.ReconcileSubscription(ctx, otherID)
				if err != nil || res == nil || res.Outcome != domain.FetchOutcomeSuccess {
					t.Fatalf("shared B: result=%+v error=%v", res, err)
				}
				otherFetchID = res.FetchID
			}

			revisions := sqlite.NewRevisionRepository(db)
			rev := &domain.ConfigurationRevision{ID: "rev-current-partial", ContentDigest: "synthetic-current-partial", State: domain.RevisionStateActive, CreatedAt: domain.NowUTC()}
			if err := revisions.Create(ctx, rev); err != nil {
				t.Fatal(err)
			}
			loader := sqlite.NewConsistentFactsLoader(db)
			pubs := sqlite.NewPublicationRepository(db)
			pubSvc := publication.NewService(pubs, nil, publication.WithConsistentFactsLoader(loader))

			// Check real consumers before and after the partial refresh. No supplied
			// Snapshot bypass: Preview must load facts and run its artifact pipeline.
			assertConsumers := func(stage string, wantNames []string) {
				t.Helper()
				wantIDs := make([]string, 0, len(wantNames))
				wantServers := make([]string, 0, len(wantNames))
				servers := map[string]string{"A": "192.0.2.11", "B": "192.0.2.12", "C": "192.0.2.13", "Manual": "192.0.2.14"}
				for _, name := range wantNames {
					wantIDs = append(wantIDs, ids[name])
					wantServers = append(wantServers, servers[name])
				}
				sort.Strings(wantIDs)
				sort.Strings(wantServers)
				assertIDs := func(consumer string, got []string) {
					t.Helper()
					sort.Strings(got)
					if !reflect.DeepEqual(got, wantIDs) {
						t.Fatalf("%s %s IDs=%v want=%v", stage, consumer, got, wantIDs)
					}
				}
				inventoryNodes, err := nodes.ListAll(ctx, domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, ExcludeNotices: true})
				if err != nil {
					t.Fatal(err)
				}
				var inventoryIDs []string
				for _, n := range inventoryNodes {
					inventoryIDs = append(inventoryIDs, n.LogicalID)
				}
				assertIDs("enabled inventory", inventoryIDs)
				facts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetSingBox, domain.NowUTC())
				if err != nil || facts == nil {
					t.Fatalf("%s facts: %v", stage, err)
				}
				var factIDs []string
				for _, n := range facts.Nodes {
					factIDs = append(factIDs, n.LogicalID)
				}
				assertIDs("consistent facts", factIDs)
				preview, err := pubSvc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSingBox, RevisionID: rev.ID})
				if err != nil || preview == nil {
					t.Fatalf("%s real preview: %v", stage, err)
				}
				var manifestIDs []string
				for _, n := range preview.Manifest.Included {
					manifestIDs = append(manifestIDs, n.NodeID)
				}
				assertIDs("preview manifest", manifestIDs)
				if preview.Manifest.NodeCount != len(wantIDs) || preview.Manifest.ExcludedCount != 0 {
					t.Fatalf("%s manifest: %+v", stage, preview.Manifest)
				}
				var rendered struct {
					Outbounds []struct {
						Server string `json:"server"`
					} `json:"outbounds"`
				}
				if err := json.Unmarshal(preview.Content, &rendered); err != nil {
					t.Fatal(err)
				}
				var gotServers []string
				for _, outbound := range rendered.Outbounds {
					if outbound.Server != "" {
						gotServers = append(gotServers, outbound.Server)
					}
				}
				sort.Strings(gotServers)
				if !reflect.DeepEqual(gotServers, wantServers) {
					t.Fatalf("%s rendered servers=%v want=%v", stage, gotServers, wantServers)
				}
				draft, err := pubs.GetByID(ctx, preview.SnapshotID)
				if err != nil || draft == nil || draft.State != domain.PublicationStateDraft || string(draft.Content) != string(preview.Content) {
					t.Fatalf("%s persisted preview mismatch: %v", stage, err)
				}
				t.Logf("stage=%s enabled_inventory=%d facts=%d preview=%d rendered=%v", stage, len(inventoryIDs), len(factIDs), preview.Manifest.NodeCount, gotServers)
			}
			assertConsumers("full", []string{"A", "B", "Manual"})

			fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte("proxies:\n" + nodeA + nodeC + rejected), ContentDigest: "synthetic-partial-ac"})
			partial, err := svc.ReconcileSubscription(ctx, id)
			if err != nil || partial == nil || partial.Outcome != domain.FetchOutcomePartial || partial.NodesValid != 2 || partial.RedactedError == "" {
				t.Fatalf("partial A/C/rejected: result=%+v error=%v", partial, err)
			}
			ledger, err := nodes.ListAll(ctx, domain.NodeFilter{ActiveOnly: true})
			if err != nil || len(ledger) != 4 {
				t.Fatalf("current active ledger should retain A/B/C and manual: count=%d error=%v", len(ledger), err)
			}
			for _, n := range ledger {
				if oldID := ids[n.DisplayName]; oldID != "" && oldID != n.LogicalID {
					t.Fatalf("%s identity changed", n.DisplayName)
				}
				ids[n.DisplayName] = n.LogicalID
			}
			if ids["C"] == "" {
				t.Fatal("new C missing from ledger")
			}
			rows, err := db.QueryContext(ctx, "SELECT node_logical_id, last_seen_fetch_id FROM node_sources WHERE subscription_id = ?", id)
			if err != nil {
				t.Fatal(err)
			}
			gotSources := map[string]string{}
			for rows.Next() {
				var nodeID, fetchID string
				if err := rows.Scan(&nodeID, &fetchID); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				gotSources[nodeID] = fetchID
			}
			rowsErr := rows.Err()
			rows.Close()
			if rowsErr != nil {
				t.Fatal(rowsErr)
			}
			// Superseded stale-association contract: partial now removes only this
			// source's omitted membership while retaining B as a permanent asset.
			wantSources := map[string]string{ids["A"]: partial.FetchID, ids["C"]: partial.FetchID}
			if !reflect.DeepEqual(gotSources, wantSources) {
				t.Fatalf("current partial source fetch IDs=%v want=%v", gotSources, wantSources)
			}
			wantNames := []string{"A", "C", "Manual"}
			if sharedB {
				var sharedFetch string
				if err := db.QueryRowContext(ctx, "SELECT last_seen_fetch_id FROM node_sources WHERE node_logical_id = ? AND subscription_id = ?", ids["B"], otherID).Scan(&sharedFetch); err != nil || sharedFetch != otherFetchID {
					t.Fatalf("shared B association changed: fetch=%s error=%v", sharedFetch, err)
				}
				wantNames = append(wantNames, "B")
			}
			assertConsumers("partial", wantNames)
			if sharedB {
				if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET enabled = 0 WHERE id = ?", otherID); err != nil {
					t.Fatal(err)
				}
				assertConsumers("shared subscription disabled", []string{"A", "C", "Manual"})
			}
		})
	}
}
