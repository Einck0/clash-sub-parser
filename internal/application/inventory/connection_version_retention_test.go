package inventory_test

import (
	"context"
	"fmt"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

func TestConnectionVersionRetentionReconcileAndConnectionPatch(t *testing.T) {
	ctx := context.Background()
	db, subs, fetches, nodes, sources := setupTestEnv(t)
	fetcher := newMockFetcher()
	const subID, url = "bounded-runtime-sub", "https://synthetic.example/bounded"
	createTestSub(t, subs, subID, "Bounded runtime", url, true)
	svc := inventory.NewService(db, subs, fetches, nodes, sources, fetcher)
	var logicalID string
	for i := 0; i < 101; i++ {
		password := "synthetic-A"
		if i%2 == 1 {
			password = "synthetic-B"
		}
		body := fmt.Sprintf(`proxies:
  - name: Stable Name
    type: ss
    server: 192.0.2.1
    port: 8388
    cipher: aes-128-gcm
    password: %s
`, password)
		fetcher.setResponse(url, &fetch.Response{StatusCode: 200, Body: []byte(body), ContentDigest: fmt.Sprintf("synthetic-digest-%d", i)})
		if _, err := svc.ReconcileSubscription(ctx, subID); err != nil {
			t.Fatalf("refresh %d: %v", i, err)
		}
		current, err := nodes.ListAll(ctx, domain.NodeFilter{})
		if err != nil || len(current) != 1 {
			t.Fatalf("refresh %d nodes=%+v err=%v", i, current, err)
		}
		if i == 0 {
			logicalID = current[0].LogicalID
		}
		if current[0].LogicalID != logicalID || current[0].ConnectionRevision != int64(i+1) || current[0].Credentials.Password != password {
			t.Fatalf("refresh %d lost stable identity/monotonic revision/config: %+v", i, current[0])
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("refresh %d accumulated full versions=%d err=%v", i, count, err)
		}
	}
	// The separate user-edit transaction must have the same retention boundary.
	server := "192.0.2.2"
	detail, err := svc.UpdateNodeConnection(ctx, inventory.UpdateNodeConnectionCommand{LogicalID: logicalID, Patch: inventory.NodePatchRequest{Server: &server}, ActorKind: domain.ActorKindAdmin, RequestID: "bounded-patch"})
	if err != nil || detail.Node.ConnectionRevision != 102 || detail.Node.Server != server {
		t.Fatalf("connection patch: detail=%+v err=%v", detail, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_versions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("connection patch accumulated full versions=%d err=%v", count, err)
	}
	var origin, digest string
	if err := db.QueryRowContext(ctx, `SELECT p.origin_payload_id,e.content_digest FROM node_connection_provenance p JOIN subscription_payload_evidence e ON e.payload_id=p.origin_payload_id WHERE p.node_logical_id=? AND p.connection_revision=1`, logicalID).Scan(&origin, &digest); err != nil || origin == "" || digest != "synthetic-digest-0" {
		t.Fatalf("pruning full configurations lost original provenance: %s %s %v", origin, digest, err)
	}
}
