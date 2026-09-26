package inventory_test

import (
	"context"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

type staticFetcher struct {
	body []byte
}

func (f *staticFetcher) Fetch(ctx context.Context, opts fetch.Options) (*fetch.Response, error) {
	return &fetch.Response{
		StatusCode:    200,
		Body:          f.body,
		ContentDigest: "test-content-digest-123",
	}, nil
}

func TestReconcilePlaintextPersistsDirectlyToNodesTable(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	yamlContent := []byte(`
proxies:
  - name: SS Node
    type: ss
    server: 198.51.100.1
    port: 8388
    cipher: aes-128-gcm
    password: my-secret-ss-password
  - name: WG Edge
    type: wireguard
    server: 198.51.100.10
    port: 51820
    ip: 10.0.0.2/32
    public-key: peer-pub-key-base64
    private-key: client-priv-key-base64
    pre-shared-key: psk-base64
    mtu: 1420
  - name: TUIC Edge
    type: tuic
    server: 198.51.100.20
    port: 8443
    uuid: 00000000-0000-0000-0000-000000000001
    password: tuic-password
    congestion-controller: bbr
    udp-relay-mode: native
    sni: tuic.example.com
`)

	fetcher := &staticFetcher{body: yamlContent}
	service := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher)

	sub := &domain.Subscription{
		ID:                 "sub-test-plaintext-1",
		Name:               "Plaintext Test Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}

	result, err := service.ReconcileSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ReconcileSubscription failed: %v", err)
	}
	if result.NodesValid != 3 {
		t.Fatalf("expected 3 valid nodes, got %d", result.NodesValid)
	}

	nodes, count, err := nodeRepo.List(ctx, domain.NodeFilter{})
	if err != nil || count != 3 {
		t.Fatalf("list nodes failed: count=%d, err=%v", count, err)
	}

	byProto := make(map[domain.Protocol]domain.Node, len(nodes))
	for _, n := range nodes {
		byProto[n.Protocol] = n
	}

	ssNode := byProto[domain.ProtocolSS]
	if ssNode.Server != "198.51.100.1" || ssNode.Port != 8388 || ssNode.Credentials.Password != "my-secret-ss-password" || ssNode.Credentials.Method != "aes-128-gcm" {
		t.Fatalf("unexpected SS node plaintext fields: %+v", ssNode)
	}

	wgNode := byProto[domain.ProtocolWireGuard]
	if wgNode.Server != "198.51.100.10" || wgNode.Port != 51820 || wgNode.Credentials.PrivateKey != "client-priv-key-base64" || wgNode.Credentials.PublicKey != "peer-pub-key-base64" || wgNode.Credentials.MTU != 1420 {
		t.Fatalf("unexpected WG node plaintext fields: %+v", wgNode)
	}

	tuicNode := byProto[domain.ProtocolTUIC]
	if tuicNode.Server != "198.51.100.20" || tuicNode.Port != 8443 || tuicNode.Credentials.UUID != "00000000-0000-0000-0000-000000000001" || tuicNode.Credentials.Password != "tuic-password" {
		t.Fatalf("unexpected TUIC node plaintext fields: %+v", tuicNode)
	}
}

func TestUpdateNodeConnectionPlaintextDirectEdit(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo := setupTestEnv(t)
	ctx := context.Background()

	yamlContent := []byte(`
proxies:
  - name: WG Edge
    type: wireguard
    server: 198.51.100.10
    port: 51820
    ip: 10.0.0.2/32
    public-key: peer-pub-key-base64
    private-key: client-priv-key-base64
    pre-shared-key: psk-base64
    mtu: 1420
`)

	fetcher := &staticFetcher{body: yamlContent}
	service := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, nodeSourceRepo, fetcher)

	sub := &domain.Subscription{
		ID:                 "sub-test-edit-1",
		Name:               "Edit Test Sub",
		SourceURLSecretRef: "http://example.com/sub",
		Enabled:            true,
		Revision:           "rev-1",
		CreatedAt:          domain.NowUTC(),
		UpdatedAt:          domain.NowUTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReconcileSubscription(ctx, sub.ID); err != nil {
		t.Fatal(err)
	}

	nodes, _, _ := nodeRepo.List(ctx, domain.NodeFilter{})
	wgID := nodes[0].LogicalID

	newName := "WG Edge Updated"
	newServer := "203.0.113.55"
	newPort := 51821
	newMTU := 1380
	newPrivKey := "updated-priv-key-plaintext"
	detail, err := service.UpdateNodeConnection(ctx, inventory.UpdateNodeConnectionCommand{
		LogicalID: wgID,
		Patch: inventory.NodePatchRequest{
			DisplayName:  &newName,
			Server:       &newServer,
			Port:         &newPort,
			LocalAddress: []string{"10.0.0.99/32"},
			MTU:          &newMTU,
			PrivateKey:   &newPrivKey,
		},
		RequestID: "req-test-edit",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("UpdateNodeConnection failed: %v", err)
	}
	if detail.Node.DisplayName != newName || detail.Node.Server != newServer || detail.Node.Port != newPort {
		t.Fatalf("expected updated display_name/server/port, got %+v", detail.Node)
	}
	if detail.Node.Credentials.MTU != 1380 || detail.Node.Credentials.PrivateKey != newPrivKey || len(detail.Node.Credentials.LocalAddress) != 1 || detail.Node.Credentials.LocalAddress[0] != "10.0.0.99/32" {
		t.Fatalf("expected updated credentials, got %+v", detail.Node.Credentials)
	}
}
