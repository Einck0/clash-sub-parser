package inventory_test

import (
	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"context"
	"testing"
)

func TestReconcile_PartialSharedConnectionChangePreservesOldAssociation(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	f := newMockFetcher()
	svc := inventory.NewService(db, subs, fetches, nodes, sources, f)
	a, b := domain.MustNewUUIDv7(), domain.MustNewUUIDv7()
	createTestSub(t, subs, a, "A", "https://a.example/feed", true)
	createTestSub(t, subs, b, "B", "https://b.example/feed", true)
	initial := []byte("proxies:\n  - {name: Shared, type: ss, server: old.example, port: 8388, cipher: aes-128-gcm, password: fixture}\n")
	for _, pair := range [][2]string{{a, "https://a.example/feed"}, {b, "https://b.example/feed"}} {
		f.setResponse(pair[1], &fetch.Response{StatusCode: 200, Body: initial, ContentDigest: "initial"})
		if _, err := svc.ReconcileSubscription(ctx, pair[0]); err != nil {
			t.Fatal(err)
		}
	}
	var oldID string
	if err := db.QueryRow("SELECT logical_id FROM nodes").Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	f.setResponse("https://a.example/feed", &fetch.Response{StatusCode: 200, Body: []byte("proxies:\n  - {name: Shared, type: ss, server: new.example, port: 8388, cipher: aes-128-gcm, password: fixture}\n  - {name: Broken, type: ss, server: '', port: 0}\n"), ContentDigest: "partial"})
	result, err := svc.ReconcileSubscription(ctx, a)
	if err != nil || result.Outcome != domain.FetchOutcomePartial {
		t.Fatalf("partial: %+v %v", result, err)
	}
	var oldOwners int
	if err := db.QueryRow("SELECT COUNT(*) FROM node_sources WHERE node_logical_id=?", oldID).Scan(&oldOwners); err != nil {
		t.Fatal(err)
	}
	if oldOwners != 2 {
		t.Fatalf("partial copy-on-write deleted old association: owners=%d", oldOwners)
	}
	if result.RedactedError == "" {
		t.Fatal("partial reason missing")
	}
}
