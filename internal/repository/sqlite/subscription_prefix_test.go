package sqlite_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestLegacyPrefixFallbackUsesPreviousSourceOrder(t *testing.T) {
	ctx := context.Background()
	db, subs := newSubscriptionsTestDB(t)
	now := domain.NowUTC()
	for _, source := range []struct{ id, prefix string }{{"a", "A "}, {"b", "B "}} {
		if err := subs.Create(ctx, &domain.Subscription{ID: source.id, Name: source.id, SourceURLSecretRef: "https://fixture.example", Prefix: source.prefix, Enabled: true, Revision: source.id, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	nodes := sqlite.NewNodeRepository(db)
	if err := nodes.UpsertBatch(ctx, []domain.Node{{LogicalID: "legacy", Protocol: domain.ProtocolSS, DisplayName: "A B [natural] Name", Server: "198.51.100.1", Port: 8388, Active: true, CreatedAt: now, UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO node_sources(node_logical_id, subscription_id, last_seen_fetch_id) VALUES ('legacy','a',''), ('legacy','b','')`); err != nil {
		t.Fatal(err)
	}
	b, _ := subs.GetByID(ctx, "b")
	b.Prefix = "C "
	if err := subs.Update(ctx, b); err != nil {
		t.Fatal(err)
	}
	node, err := nodes.GetByLogicalID(ctx, "legacy")
	if err != nil || node.DisplayName != "A C [natural] Name" {
		t.Fatalf("fallback stripped wrong labels: %v %v", node, err)
	}
	if err := subs.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	node, err = nodes.GetByLogicalID(ctx, "legacy")
	if err != nil || node.DisplayName != "C [natural] Name" || !node.Active {
		t.Fatalf("delete damaged shared legacy label: %v %v", node, err)
	}
}
