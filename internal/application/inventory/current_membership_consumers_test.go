package inventory_test

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestCurrentMembershipSharedPartialConsumers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	f := newMockFetcher()
	svc := inventory.NewService(db, subs, fetches, nodes, sources, f)
	s, other := domain.MustNewUUIDv7(), domain.MustNewUUIDv7()
	const su, tu = "https://membership-s.invalid/", "https://membership-t.invalid/"
	createTestSub(t, subs, s, "S", su, true)
	createTestSub(t, subs, other, "T", tu, true)
	if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET is_primary = 1 WHERE id = ?", s); err != nil {
		t.Fatal(err)
	}
	const a = "  - {name: A, type: ss, server: 192.0.2.11, port: 8388, cipher: aes-128-gcm, password: a}\n"
	const b = "  - {name: B, type: ss, server: 192.0.2.12, port: 8388, cipher: aes-128-gcm, password: b}\n"
	const c = "  - {name: C, type: ss, server: 192.0.2.13, port: 8388, cipher: aes-128-gcm, password: c}\n"
	const bad = "  - {name: bad, type: ss, server: '', port: 0}\n"
	reconcile := func(id, url, body, digest string, outcome domain.FetchOutcome) *inventory.ReconcileResult {
		t.Helper()
		f.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte("proxies:\n" + body), ContentDigest: digest})
		result, err := svc.ReconcileSubscription(ctx, id)
		if result == nil || result.Outcome != outcome || (outcome != domain.FetchOutcomeFailed && err != nil) || (outcome == domain.FetchOutcomeFailed && err == nil) {
			t.Fatalf("reconcile %s: result=%+v error=%v", digest, result, err)
		}
		return result
	}
	first := reconcile(s, su, a+b, "membership-ab", domain.FetchOutcomeSuccess)
	reconcile(other, tu, b, "membership-b", domain.FetchOutcomeSuccess)
	partial := reconcile(s, su, a+c+bad, "membership-ac", domain.FetchOutcomePartial)
	rev := &domain.ConfigurationRevision{ID: "membership-rev", ContentDigest: "membership-rev", State: domain.RevisionStateActive, CreatedAt: domain.NowUTC()}
	if err := sqlite.NewRevisionRepository(db).Create(ctx, rev); err != nil {
		t.Fatal(err)
	}
	loader := sqlite.NewConsistentFactsLoader(db)
	assert := func(stage string, wantGlobal, wantS, wantT []string) {
		t.Helper()
		names := func(filter domain.NodeFilter) []string {
			t.Helper()
			got, err := nodes.ListAll(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			out := []string{}
			for _, n := range got {
				out = append(out, n.DisplayName)
			}
			sort.Strings(out)
			return out
		}
		check := func(label string, got, want []string) {
			t.Helper()
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s %s: got %v want %v", stage, label, got, want)
			}
		}
		base := domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true}
		check("global", names(base), wantGlobal)
		base.SubscriptionID = s
		check("S", names(base), wantS)
		base.SubscriptionID = other
		check("T", names(base), wantT)
		facts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetSingBox, domain.NowUTC())
		if err != nil {
			t.Fatal(err)
		}
		primary := []string{}
		membership := map[string][]string{}
		for _, n := range facts.Nodes {
			for _, src := range facts.NodeSources[n.LogicalID] {
				membership[src.SubscriptionID] = append(membership[src.SubscriptionID], n.DisplayName)
			}
			for _, id := range facts.PrimaryNodeIDs {
				if id == n.LogicalID {
					primary = append(primary, n.DisplayName)
				}
			}
		}
		sort.Strings(primary)
		check("primary", primary, wantS)
		for _, id := range []string{s, other} {
			sort.Strings(membership[id])
			if membership[id] == nil {
				membership[id] = []string{}
			}
		}
		check("facts S", membership[s], wantS)
		check("facts T", membership[other], wantT)
	}
	assert("partial", []string{"A", "B", "C"}, []string{"A", "C"}, []string{"B"})
	var stale string
	if err := db.QueryRowContext(ctx, `SELECT ns.last_seen_fetch_id FROM node_sources ns JOIN nodes n ON n.logical_id=ns.node_logical_id WHERE ns.subscription_id=? AND n.display_name='B'`, s).Scan(&stale); err != nil || stale != first.FetchID {
		t.Fatalf("historical B: %s %v", stale, err)
	}
	if partial.FetchID == first.FetchID {
		t.Fatal("partial fetch reused initial ID")
	}
	if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET enabled=0 WHERE id=?", other); err != nil {
		t.Fatal(err)
	}
	assert("disabled", []string{"A", "C"}, []string{"A", "C"}, []string{})
	if _, err := db.ExecContext(ctx, "UPDATE subscriptions SET enabled=1 WHERE id=?", other); err != nil {
		t.Fatal(err)
	}
	assert("reenabled", []string{"A", "B", "C"}, []string{"A", "C"}, []string{"B"})
	f.setResponse(su, &fetch.Response{StatusCode: 200, Body: []byte("corrupted invalid binary/garbage content %%%$$$"), ContentDigest: "membership-failed"})
	failed, err := svc.ReconcileSubscription(ctx, s)
	if err == nil || failed == nil || failed.Outcome != domain.FetchOutcomeFailed {
		t.Fatalf("failed reconcile: result=%+v error=%v", failed, err)
	}
	assert("failed", []string{"A", "B", "C"}, []string{"A", "C"}, []string{"B"})
	reconcile(s, su, b, "membership-later", domain.FetchOutcomeSuccess)
	assert("later", []string{"B"}, []string{"B"}, []string{"B"})
	all, err := nodes.ListAll(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	ledger := []string{}
	for _, n := range all {
		ledger = append(ledger, n.DisplayName)
	}
	sort.Strings(ledger)
	if !reflect.DeepEqual(ledger, []string{"A", "B", "C"}) {
		t.Fatalf("ledger lost historical nodes: %v", ledger)
	}
}
