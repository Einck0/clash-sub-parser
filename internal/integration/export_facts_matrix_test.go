package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

// Exercise the production facts path, rather than supplying a prebuilt snapshot.
func TestExportFactsMatrixAndImmutablePublication(t *testing.T) {
	ctx := context.Background()
	db := openIntegrationDB(t)
	subs := sqlite.NewSubscriptionRepository(db)
	fetches := sqlite.NewSubscriptionFetchRepository(db)
	nodes := sqlite.NewNodeRepository(db)
	sources := sqlite.NewNodeSourceRepository(db)
	now := domain.NowUTC()
	sub := &domain.Subscription{ID: "matrix-sub", Name: "matrix", SourceURLSecretRef: "https://fixture.invalid/matrix", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := subs.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}
	fixture := `proxies:
  - {name: SS, type: ss, server: 198.51.100.11, port: 8388, cipher: aes-256-gcm, password: matrix-ss}
  - name: VMess
    type: vmess
    server: 198.51.100.12
    port: 443
    uuid: 11111111-1111-4111-8111-111111111111
    alterId: 0
    cipher: auto
    tls: true
    network: ws
    ws-opts: {path: /matrix, headers: {Host: vmess.example.org}}
  - name: VLESS
    type: vless
    server: 198.51.100.13
    port: 443
    uuid: 22222222-2222-4222-8222-222222222222
    tls: true
    servername: vless.example.org
    flow: xtls-rprx-vision
    client-fingerprint: chrome
    reality-opts: {public-key: "jNXHt1yRo0vD5_1N6p2W3x4Y5z6A7b8C9d0E1f2G3h4", short-id: "01ab"}
  - {name: Trojan, type: trojan, server: 198.51.100.14, port: 443, password: matrix-trojan, sni: trojan.example.org}
  - {name: Hy2, type: hysteria2, server: 198.51.100.15, port: 8443, password: matrix-hy2, sni: hy2.example.org}
  - name: WG
    type: wireguard
    server: 198.51.100.16
    port: 51820
    ip: 10.0.0.2/32
    private-key: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
    public-key: "HyCEC7mK3/cd/2d+p4I5dfB3nBvV9uG1D2L8aF+p+A8="
  - {name: TUIC, type: tuic, server: 198.51.100.17, port: 8443, uuid: 33333333-3333-4333-8333-333333333333, password: matrix-tuic, congestion-controller: bbr, udp-relay-mode: native, alpn: [h3], sni: tuic.example.org}
`
	reconcile := func(body, digest string) {
		t.Helper()
		svc := inventory.NewService(db, subs, fetches, nodes, sources, fixtureFetcher{response: &fetch.Response{StatusCode: 200, ContentType: "text/yaml", ContentDigest: digest, Body: []byte(body)}})
		result, err := svc.ReconcileSubscription(ctx, sub.ID)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if result.Outcome != domain.FetchOutcomeSuccess || result.NodesValid != 7 {
			t.Fatalf("reconcile result: %+v", result)
		}
	}
	reconcile(fixture, "sha256:matrix-1")
	all, _, err := nodes.List(ctx, domain.NodeFilter{Pagination: domain.Pagination{Page: 1, PageSize: 20}})
	if err != nil || len(all) != 7 {
		t.Fatalf("nodes: %d, %v", len(all), err)
	}
	ids := map[domain.Protocol]string{}
	for _, n := range all {
		ids[n.Protocol] = n.LogicalID
	}
	revs := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{ID: "matrix-revision", ContentDigest: "sha256:matrix-revision", State: domain.RevisionStateActive, CreatedAt: now}
	if err := revs.Create(ctx, rev); err != nil {
		t.Fatal(err)
	}
	if err := revs.SetActive(ctx, rev.ID); err != nil {
		t.Fatal(err)
	}
	// Mihomo consumes the routing graph, unlike the other pure-node targets.
	// With no effective route it attempts primary-subscription fallback, but this
	// fixture deliberately has no primary subscription. Persist a real group and
	// MATCH rule so the matrix exercises normal compilation from database facts.
	policy := sqlite.NewPolicyRepository(db)
	group := &domain.NodeGroup{ID: domain.MustNewUUIDv7(), Name: "Matrix Select", GroupType: domain.GroupTypeSelect, CreatedAt: now, UpdatedAt: now}
	members := make([]domain.MemberSource, 0, len(all))
	for _, n := range all {
		nodeID := n.LogicalID
		members = append(members, domain.MemberSource{Operation: domain.OperationAdd, Kind: domain.SourceKindManualNode, NodeLogicalID: &nodeID})
	}
	if _, err := policy.SaveGroupWithSources(ctx, group, 0, members); err != nil {
		t.Fatalf("create matrix group: %v", err)
	}
	if err := policy.CreatePolicyRule(ctx, &domain.PolicyRule{ID: domain.MustNewUUIDv7(), RevisionID: rev.ID, TargetGroupID: group.ID, Expression: "MATCH", Enabled: true}); err != nil {
		t.Fatalf("create matrix route: %v", err)
	}
	pubs := sqlite.NewPublicationRepository(db)
	svc := publication.NewService(pubs, nil, publication.WithPolicyRepository(policy), publication.WithRevisionRepository(revs), publication.WithNodeRepository(nodes), publication.WithPayloadRefRepository(sqlite.NewPublicationPayloadRefRepository(db)), publication.WithConsistentFactsLoader(sqlite.NewConsistentFactsLoader(db)))
	assertPublicationOK := func(t *testing.T, operation string, err error) {
		t.Helper()
		if err == nil {
			return
		}
		var preflight *publication.PreflightError
		if errors.As(err, &preflight) {
			t.Fatalf("%s: %v; preflight: %+v", operation, err, preflight.Result)
		}
		var capability *publication.StrictCapabilityError
		if errors.As(err, &capability) {
			t.Fatalf("%s: %v; capabilities: %+v", operation, err, capability.Diagnostics)
		}
		t.Fatalf("%s: %v", operation, err)
	}
	cases := []struct {
		target   domain.CompilerTarget
		count    int
		excluded []domain.Protocol
		contains []string
	}{
		{domain.TargetMihomo, 7, nil, []string{"198.51.100.16", "matrix-ss", "/matrix", "MATCH,Matrix Select"}},
		{domain.TargetSingBox, 7, nil, []string{"198.51.100.16", "matrix-ss", "/matrix"}},
		{domain.TargetSurge, 6, []domain.Protocol{domain.ProtocolVLESS}, []string{"matrix-ss", "198.51.100.16"}},
		{domain.TargetQuantumultX, 3, []domain.Protocol{domain.ProtocolVLESS, domain.ProtocolHysteria2, domain.ProtocolWireGuard, domain.ProtocolTUIC}, []string{"matrix-ss", "/matrix"}},
	}
	var draft string
	var initial []byte
	for _, tc := range cases {
		t.Run(string(tc.target), func(t *testing.T) {
			res, err := svc.Preview(ctx, publication.PreviewQuery{Target: tc.target, RevisionID: rev.ID, CompatMode: "compatible"})
			assertPublicationOK(t, "preview", err)
			if res.Manifest.NodeCount != tc.count || res.Manifest.ExcludedCount != len(tc.excluded) {
				t.Fatalf("manifest: %+v", res.Manifest)
			}
			expected := map[string]bool{}
			for _, p := range tc.excluded {
				expected[ids[p]] = true
			}
			for _, e := range res.Manifest.Excluded {
				if !expected[e.NodeID] {
					t.Errorf("unexpected excluded node: %+v", e)
				}
				delete(expected, e.NodeID)
			}
			if len(expected) != 0 {
				t.Errorf("missing exclusions: %v", expected)
			}
			for _, part := range tc.contains {
				if !strings.Contains(string(res.Content), part) {
					t.Errorf("content missing %q", part)
				}
			}
			if tc.target == domain.TargetSingBox {
				draft = res.SnapshotID
				initial = append([]byte(nil), res.Content...)
			}
		})
	}
	if _, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSurge, RevisionID: rev.ID, CompatMode: "strict"}); err == nil {
		t.Error("strict Surge preview accepted VLESS")
	}
	if draft == "" {
		t.Fatal("missing sing-box draft")
	}
	published, err := svc.Publish(ctx, publication.PublishCommand{Target: domain.TargetSingBox, SnapshotID: draft, ActorKind: domain.ActorKindAdmin, RequestID: "matrix-publish"})
	assertPublicationOK(t, "publish", err)
	reconcile(strings.Replace(fixture, "matrix-ss", "matrix-ss-updated", 1), "sha256:matrix-2")
	artifact, err := svc.ResolveAndServeAuthorized(ctx, published.Publication.ID, func(string) bool { return true })
	if err != nil {
		t.Fatalf("serve publication: %v", err)
	}
	if string(artifact.Content) != string(initial) {
		t.Error("published bytes changed after refresh")
	}
	refreshed, err := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetSingBox, RevisionID: rev.ID, CompatMode: "compatible"})
	assertPublicationOK(t, "refreshed preview", err)
	if !strings.Contains(string(refreshed.Content), "matrix-ss-updated") {
		t.Error("refreshed facts absent from preview")
	}
}
