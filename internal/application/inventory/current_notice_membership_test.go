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

// Retained source references must not turn historical notice/proxy overrides
// into current classifications in another subscription's inventory or export.
func TestCurrentNoticeMembershipSharedPartialConsumers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	f := newMockFetcher()
	svc := inventory.NewService(db, subs, fetches, nodes, sources, f)
	s, other := domain.MustNewUUIDv7(), domain.MustNewUUIDv7()
	const su, tu = "https://notice-s.invalid/", "https://notice-t.invalid/"
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
	reconcile(s, su, a+b, "notice-initial-ab", domain.FetchOutcomeSuccess)
	reconcile(other, tu, b, "notice-other-b", domain.FetchOutcomeSuccess)
	entries := sqlite.NewSubscriptionEntryRepository(db)
	findB := func(id string) domain.SubscriptionEntry {
		t.Helper()
		latest, err := entries.ListLatestBySubscription(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range latest {
			if entry.RawName == "B" {
				return entry
			}
		}
		t.Fatal("B entry missing")
		return domain.SubscriptionEntry{}
	}
	sb, tb := findB(s), findB(other)
	if sb.NodeLogicalID == nil || tb.NodeLogicalID == nil || *sb.NodeLogicalID != *tb.NodeLogicalID {
		t.Fatalf("B must be shared: S=%+v T=%+v", sb, tb)
	}
	override := func(entryID string, kind domain.EntryKind) {
		t.Helper()
		updated, err := svc.OverrideEntryKind(ctx, entryID, &kind, "synthetic classification", "test")
		if err != nil || updated == nil || updated.UserKindOverride == nil || *updated.UserKindOverride != kind {
			t.Fatalf("override %s: entry=%+v error=%v", kind, updated, err)
		}
	}
	override(sb.ID, domain.EntryKindNotice)

	rev := &domain.ConfigurationRevision{ID: "notice-membership-rev", ContentDigest: "notice-membership-rev", State: domain.RevisionStateActive, CreatedAt: domain.NowUTC()}
	if err := sqlite.NewRevisionRepository(db).Create(ctx, rev); err != nil {
		t.Fatal(err)
	}
	// A persisted dynamic group exercises actual facts -> resolver -> compiler
	// consumption, with valid routing even when B is classified as notice.
	policy := sqlite.NewPolicyRepository(db)
	now := domain.NowUTC()
	group := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "Current notice members", GroupType: domain.GroupTypeSelect, CreatedAt: now, UpdatedAt: now}
	pattern := ".*"
	if _, err := policy.SaveGroupWithSources(ctx, group, 0, []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindRegex, Pattern: &pattern}}); err != nil {
		t.Fatal(err)
	}
	if err := policy.CreatePolicyRule(ctx, &domain.PolicyRule{ID: domain.MustNewUUIDv7(), RevisionID: rev.ID, TargetGroupID: group.ID, Expression: "MATCH", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	loader := sqlite.NewConsistentFactsLoader(db)
	pubs := sqlite.NewPublicationRepository(db)
	pub := publication.NewService(pubs, nil, publication.WithConsistentFactsLoader(loader))
	assert := func(stage string, wantGlobal, wantS, wantT []string) {
		t.Helper()
		check := func(label string, got, want []string) {
			t.Helper()
			if got == nil {
				got = []string{}
			}
			if want == nil {
				want = []string{}
			}
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s %s: got %v want %v", stage, label, got, want)
			}
		}
		for _, scope := range []struct {
			id   string
			want []string
		}{{"", wantGlobal}, {s, wantS}, {other, wantT}} {
			views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, SubscriptionID: scope.id, Pagination: domain.Pagination{Page: 1, PageSize: 50}})
			if err != nil {
				t.Fatal(err)
			}
			names := []string{}
			for _, view := range views {
				names = append(names, view.DisplayName)
			}
			check("read model "+scope.id, names, scope.want)
			if total != len(scope.want) {
				t.Fatalf("%s read model total=%d want=%d", stage, total, len(scope.want))
			}
			if scope.id != "" {
				sub, err := subs.GetByID(ctx, scope.id)
				if err != nil {
					t.Fatal(err)
				}
				if sub.SourceNodeCount != len(scope.want) || sub.NodeCount != len(scope.want) {
					t.Fatalf("%s subscription %s source=%d enabled=%d want=%d", stage, scope.id, sub.SourceNodeCount, sub.NodeCount, len(scope.want))
				}
			}
		}
		facts, err := loader.LoadConsistentFacts(ctx, rev.ID, domain.TargetSingBox, domain.NowUTC())
		if err != nil || facts == nil {
			t.Fatalf("%s facts: %v", stage, err)
		}
		namesByID := map[string]string{}
		factNames, primary := []string{}, []string{}
		membership := map[string][]string{}
		for _, node := range facts.Nodes {
			namesByID[node.LogicalID] = node.DisplayName
			factNames = append(factNames, node.DisplayName)
			for _, source := range facts.NodeSources[node.LogicalID] {
				membership[source.SubscriptionID] = append(membership[source.SubscriptionID], node.DisplayName)
			}
		}
		for _, id := range facts.PrimaryNodeIDs {
			primary = append(primary, namesByID[id])
		}
		check("facts nodes", factNames, wantGlobal)
		check("facts S", membership[s], wantS)
		check("facts T", membership[other], wantT)
		check("primary", primary, wantS)
		preview, err := pub.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSingBox, RevisionID: rev.ID})
		if err != nil || preview == nil {
			t.Fatalf("%s preview: %v", stage, err)
		}
		included := []string{}
		for _, node := range preview.Manifest.Included {
			included = append(included, namesByID[node.NodeID])
		}
		check("preview manifest", included, wantGlobal)
		if preview.Manifest.NodeCount != len(wantGlobal) || preview.Manifest.ExcludedCount != 0 {
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
		servers, wantServers := []string{}, []string{}
		for _, outbound := range rendered.Outbounds {
			if outbound.Server != "" {
				servers = append(servers, outbound.Server)
			}
		}
		serverByName := map[string]string{"A": "192.0.2.11", "B": "192.0.2.12", "C": "192.0.2.13"}
		for _, name := range wantGlobal {
			wantServers = append(wantServers, serverByName[name])
		}
		check("rendered servers", servers, wantServers)
		draft, err := pubs.GetByID(ctx, preview.SnapshotID)
		if err != nil || draft == nil || draft.State != domain.PublicationStateDraft || string(draft.Content) != string(preview.Content) {
			t.Fatalf("%s persisted preview mismatch: %v", stage, err)
		}
	}

	// Current notice still excludes B, even though T has an ordinary proxy.
	assert("current notice", []string{"A"}, []string{"A"}, nil)
	partial := reconcile(s, su, a+c+bad, "notice-partial-ac", domain.FetchOutcomePartial)
	if partial.NodesValid != 2 || partial.RedactedError == "" {
		t.Fatalf("partial must report two valid nodes and a rejection: %+v", partial)
	}
	assert("stale notice must not exclude shared B", []string{"A", "B", "C"}, []string{"A", "C"}, []string{"B"})
	if _, err := sqlite.PruneSubscriptionRetention(ctx, db, s, 20, nil); err != nil {
		t.Fatal(err)
	}
	var staleFetch string
	if err := db.QueryRowContext(ctx, "SELECT last_seen_fetch_id FROM node_sources WHERE subscription_id=? AND node_logical_id=?", s, *sb.NodeLogicalID).Scan(&staleFetch); err == nil {
		t.Fatal("partial retained omitted current association")
	}
	// The old parsed entry is no longer the durable intent consumer.
	var kind string
	if err := db.QueryRowContext(ctx,`SELECT user_kind_override FROM subscription_entry_overrides WHERE subscription_id=? AND raw_name='B'`,s).Scan(&kind);err!=nil || kind!="notice" {t.Fatalf("durable notice intent lost: %s %v",kind,err)}
	var payloadCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads WHERE id=?", sb.PayloadID).Scan(&payloadCount); err != nil || payloadCount != 0 {
		t.Fatalf("historical parsed payload retained: count=%d error=%v", payloadCount, err)
	}
	reconcile(s, su, bad, "notice-failed-s", domain.FetchOutcomeFailed)
	reconcile(other, tu, bad, "notice-failed-t", domain.FetchOutcomeFailed)
	assert("total failures retain last useful snapshots", []string{"A", "B", "C"}, []string{"A", "C"}, []string{"B"})

	// Reappearing B regains S's durable notice override; a current explicit
	// proxy override in T still wins. Neither precedence rule is weakened.
	reconcile(s, su, a+b, "notice-reappears", domain.FetchOutcomeSuccess)
	assert("reappeared current notice", []string{"A"}, []string{"A"}, nil)
	override(tb.ID, domain.EntryKindProxy)
	// Reconciled notices do not refresh their own source membership; T is
	// still the current supplier of B, even though its explicit proxy wins.
	assert("current explicit proxy wins", []string{"A", "B"}, []string{"A"}, []string{"B"})

	// Restore a current S source, then classify it as notice without another
	// reconcile. This isolates classification from membership in the inverse
	// regression below: B would otherwise disappear solely due to staleness.
	currentSB := findB(s)
	if _, err := svc.OverrideEntryKind(ctx, currentSB.ID, nil, "clear synthetic notice", "test"); err != nil {
		t.Fatal(err)
	}
	reconcile(s, su, a+b, "notice-current-source-ab", domain.FetchOutcomeSuccess)
	currentSB = findB(s)
	override(currentSB.ID, domain.EntryKindNotice)
	assert("both current with explicit proxy", []string{"A", "B"}, []string{"A", "B"}, []string{"B"})

	// The inverse regression: old explicit proxy must not neutralize a
	// current notice after its subscription's successful subset drops B.
	reconcile(other, tu, c+bad, "notice-other-partial-c", domain.FetchOutcomePartial)
	assert("stale explicit proxy cannot win", []string{"A", "C"}, []string{"A"}, []string{"C"})
	if err := db.QueryRowContext(ctx, "SELECT last_seen_fetch_id FROM node_sources WHERE subscription_id=? AND node_logical_id=?", other, *tb.NodeLogicalID).Scan(&staleFetch); err == nil {
		t.Fatal("partial retained omitted proxy association")
	}
	ledger, err := nodes.ListAll(ctx, domain.NodeFilter{ActiveOnly: true})
	if err != nil || len(ledger) != 3 {
		t.Fatalf("historical nodes must remain active: count=%d error=%v", len(ledger), err)
	}
}
