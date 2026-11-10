package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/probe/mihomo"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
)

// Entirely invented fixtures: documentation addresses and non-production
// credentials. These test format support, not the cause of any private rejection.
func TestSyntheticFormatAcceptanceAndPreservation(t *testing.T) {
	const uuid = "44444444-4444-4444-8444-444444444444"
	const publicKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	cases := []struct {
		name, yaml, uri, user, password string
		protocol                        domain.Protocol
		transport                       map[string]string
	}{
		{"HTTP anonymous", "{type: http, server: 192.0.2.101, port: 8080}", "http://192.0.2.101:8080", "", "", domain.ProtocolHTTP, map[string]string{"network": "tcp"}},
		{"HTTP username only", "{type: http, server: 192.0.2.102, port: 8080, username: invented-user}", "http://invented-user@192.0.2.102:8080", "invented-user", "", domain.ProtocolHTTP, map[string]string{"network": "tcp"}},
		{"HTTP authenticated TLS", "{type: http, server: 192.0.2.103, port: 8443, username: invented-user, password: invented-pass, tls: true, sni: http.example.org}", "https://invented-user:invented-pass@192.0.2.103:8443?sni=http.example.org", "invented-user", "invented-pass", domain.ProtocolHTTP, map[string]string{"tls": "true", "sni": "http.example.org"}},
		{"SOCKS anonymous", "{type: socks5, server: 192.0.2.104, port: 1080, udp: true}", "socks5://192.0.2.104:1080?udp=true", "", "", domain.ProtocolSocks5, map[string]string{"udp": "true"}},
		{"SOCKS authenticated", "{type: socks5, server: 192.0.2.105, port: 1080, username: invented-socks, password: invented-pass}", "socks5://invented-socks:invented-pass@192.0.2.105:1080", "invented-socks", "invented-pass", domain.ProtocolSocks5, map[string]string{"network": "tcp"}},
		{"AnyTLS", "{type: anytls, server: 192.0.2.106, port: 443, password: invented-anytls, sni: anytls.example.org, alpn: [h2, http/1.1], client-fingerprint: chrome, idle-session-check-interval: 30, idle-session-timeout: 60, min-idle-session: 2, disable-reuse: true}", "anytls://invented-anytls@192.0.2.106:443?sni=anytls.example.org&alpn=h2,http/1.1&fp=chrome&idle-session-check-interval=30&idle-session-timeout=60&min-idle-session=2&disable-reuse=true", "", "invented-anytls", domain.ProtocolAnyTLS, map[string]string{"tls": "true", "sni": "anytls.example.org", "alpn": "h2,http/1.1", "fp": "chrome", "idle-session-check-interval": "30", "idle-session-timeout": "60", "min-idle-session": "2", "disable-reuse": "true"}},
		{"VLESS Reality", "{type: vless, server: 192.0.2.107, port: 443, uuid: " + uuid + ", tls: true, flow: xtls-rprx-vision, client-fingerprint: chrome, reality-opts: {public-key: " + publicKey + ", short-id: 01ab}}", "vless://" + uuid + "@192.0.2.107:443?security=reality&flow=xtls-rprx-vision&fp=chrome&pbk=" + publicKey + "&sid=01ab", "", "", domain.ProtocolVLESS, map[string]string{"tls": "true", "flow": "xtls-rprx-vision", "fp": "chrome", "pbk": publicKey, "sid": "01ab"}},
		{"VLESS WebSocket", "{type: vless, server: 192.0.2.108, port: 443, uuid: " + uuid + ", tls: true, network: ws, ws-opts: {path: /invented-ws, headers: {Host: ws.example.org}}}", "vless://" + uuid + "@192.0.2.108:443?security=tls&type=ws&path=%2Finvented-ws&host=ws.example.org", "", "", domain.ProtocolVLESS, map[string]string{"tls": "true", "network": "ws", "path": "/invented-ws", "host": "ws.example.org"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, format := range []string{"yaml", "url", "base64_yaml", "base64_url"} {
				t.Run(format, func(t *testing.T) {
					body := tc.uri
					if strings.HasSuffix(format, "yaml") {
						body = "proxies:\n - " + tc.yaml + "\n"
					}
					if strings.HasPrefix(format, "base64_") {
						body = base64.StdEncoding.EncodeToString([]byte(body))
					}
					res, err := parser.Parse([]byte(body))
					if err != nil || len(res.Nodes) != 1 || res.Rejected != 0 || len(res.Diagnostics) != 0 || len(res.ReasonCounts) != 0 {
						t.Fatalf("acceptance: nodes=%d rejected=%d error=%v", len(res.Nodes), res.Rejected, err)
					}
					n := res.Nodes[0]
					if n.Node.Protocol != tc.protocol || n.Credentials.Username != tc.user || n.Credentials.Password != tc.password {
						t.Fatal("protocol or authentication changed")
					}
					if tc.protocol == domain.ProtocolVLESS && n.Credentials.UUID != uuid {
						t.Fatal("VLESS UUID changed")
					}
					for key, want := range tc.transport {
						if n.Credentials.Transport[key] != want || n.Node.Credentials.Transport[key] != want {
							t.Errorf("lost transport field %s", key)
						}
					}
					// Equivalent YAML/URI connection shapes must not split identity.
					yamlRes, err := parser.Parse([]byte("proxies:\n - " + tc.yaml))
					if err != nil || len(yamlRes.Nodes) != 1 || n.Node.LogicalID != yamlRes.Nodes[0].Node.LogicalID {
						t.Fatal("equivalent format changed connection identity")
					}
				})
			}
		})
	}
}

func TestSyntheticXHTTPECHPreservedAndTargetFailureIsNotParseFailure(t *testing.T) {
	const body = `proxies:
 - name: Invented XHTTP
   type: vless
   server: 198.51.100.110
   port: 443
   uuid: 55555555-5555-4555-8555-555555555555
   tls: true
   network: xhttp
   xhttp-opts:
     path: /invented-xhttp
     host: xhttp.example.org
     mode: auto
     headers: {X-Invented: invented-header}
     scMaxEachPostBytes: 1000000
   ech-opts: {enable: true, query-server-name: ech.example.org}
`
	res, err := parser.Parse([]byte(body))
	if err != nil || len(res.Nodes) != 1 || res.Rejected != 0 {
		t.Fatalf("XHTTP parse must succeed: nodes=%d rejected=%d error=%v", len(res.Nodes), res.Rejected, err)
	}
	n := res.Nodes[0].Node
	assertOptions := func(key string, want map[string]any) {
		t.Helper()
		var got map[string]any
		if err := json.Unmarshal([]byte(n.Credentials.Transport[key]), &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("lost %s: error=%v", key, err)
		}
	}
	assertOptions("xhttp-opts", map[string]any{"path": "/invented-xhttp", "host": "xhttp.example.org", "mode": "auto", "headers": map[string]any{"X-Invented": "invented-header"}, "scMaxEachPostBytes": float64(1000000)})
	assertOptions("ech-opts", map[string]any{"enable": true, "query-server-name": "ech.example.org"})
	mapping, err := mihomo.NodeToMapping(n)
	if err != nil || mapping["network"] != "xhttp" {
		t.Fatalf("Mihomo mapping lost transport: %v", err)
	}
	for _, key := range []string{"xhttp-opts", "ech-opts"} {
		// NodeToMapping deliberately uses json.Number. Compare JSON semantics,
		// not the Go numeric representation selected by each decoder.
		encoded, err := json.Marshal(mapping[key])
		if err != nil {
			t.Fatalf("encode mapped %s: %v", key, err)
		}
		var want, got map[string]any
		if err := json.Unmarshal([]byte(n.Credentials.Transport[key]), &want); err != nil {
			t.Fatalf("decode parsed %s: %v", key, err)
		}
		if err := json.Unmarshal(encoded, &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Mihomo mapping lost %s", key)
		}
	}
	// Use the actual parser output, not a hand-built credential envelope.
	snapshot := &resolver.ResolvedPolicySnapshot{SnapshotDigest: "synthetic-xhttp", CompilerVersion: "1.0.0", Nodes: []resolver.ResolvedNode{{LogicalID: n.LogicalID, DisplayName: n.DisplayName, Protocol: n.Protocol, Server: n.Server, Port: n.Port, Credentials: n.Credentials, Active: true}}}
	_, err = compiler.Compile(context.Background(), snapshot, domain.TargetSingBox)
	var capability *compiler.CapabilityError
	if !errors.As(err, &capability) || capability.Target != domain.TargetSingBox || capability.Feature != "xhttp" || capability.Location != "nodes[0].transport.network" {
		t.Fatalf("expected typed target capability rejection, not parser failure: %v", err)
	}
	if len(res.Diagnostics) != 0 || res.Rejected != 0 || snapshot.Nodes[0].Credentials.Transport["network"] != "xhttp" {
		t.Fatal("target rejection altered parser result or silently downgraded transport")
	}
	uri, err := parser.Parse([]byte("vless://55555555-5555-4555-8555-555555555555@198.51.100.110:443?security=tls&type=xhttp&path=%2Finvented-xhttp&host=xhttp.example.org&mode=auto"))
	if err != nil || len(uri.Nodes) != 1 || uri.Rejected != 0 {
		t.Fatalf("XHTTP URI rejected at parse boundary: %v", err)
	}
	for key, want := range map[string]string{"network": "xhttp", "tls": "true", "path": "/invented-xhttp", "host": "xhttp.example.org", "mode": "auto"} {
		if uri.Nodes[0].Credentials.Transport[key] != want {
			t.Errorf("XHTTP URI lost %s", key)
		}
	}
}

func TestSyntheticCorruptEntriesHaveSafeActionableReasons(t *testing.T) {
	const body = `proxies:
 - {type: http, server: 192.0.2.120, port: 8080}
 - {name: PRIVATE_NAME_SENTINEL, type: anytls, server: private-host.example.org, port: 443}
 - {name: PRIVATE_NAME_SENTINEL, type: socks5, server: private-host.example.org, port: 0, username: PRIVATE_USER_SENTINEL, password: PRIVATE_PASSWORD_SENTINEL}
 - {name: PRIVATE_NAME_SENTINEL, type: PRIVATE_PROTOCOL_SENTINEL, server: private-host.example.org, port: 443}
 - PRIVATE_SCALAR_SENTINEL
`
	res, err := parser.ExtractWithCredentials([]byte(body))
	want := []parser.Diagnostic{
		{Format: "yaml", Entry: 2, Protocol: "anytls", Reason: "missing_credential", Field: "password"},
		{Format: "yaml", Entry: 3, Protocol: "socks5", Reason: "invalid_endpoint", Field: "endpoint"},
		{Format: "yaml", Entry: 4, Reason: "unsupported_protocol", Field: "protocol"},
		{Format: "yaml", Entry: 5, Reason: "invalid_entry", Field: "entry"},
	}
	if err != nil || len(res.Items) != 1 || res.Rejected != 4 || !reflect.DeepEqual(res.Diagnostics, want) {
		t.Fatalf("wrong entry isolation or reason classification: diagnostics=%+v error=%v", res.Diagnostics, err)
	}
	wantCounts := []parser.ReasonCount{{Reason: "invalid_endpoint", Count: 1}, {Reason: "invalid_entry", Count: 1}, {Reason: "missing_credential", Count: 1}, {Reason: "unsupported_protocol", Count: 1}}
	if !reflect.DeepEqual(res.ReasonCounts, wantCounts) {
		t.Fatalf("wrong reason totals: %+v", res.ReasonCounts)
	}
	for _, private := range []string{"PRIVATE_", "private-host.example.org", "192.0.2.120", "proxies:"} {
		if strings.Contains(res.DiagnosticSummary(), private) {
			t.Fatal("diagnostic summary contains upstream content")
		}
	}
	// Without the good anchor the same finite reasons must remain available,
	// and the top-level error must also be safe.
	failed := strings.Replace(body, " - {type: http, server: 192.0.2.120, port: 8080}\n", "", 1)
	allFailed, err := parser.ExtractWithCredentials([]byte(failed))
	if err == nil || len(allFailed.Items) != 0 || allFailed.Rejected != 4 || !reflect.DeepEqual(allFailed.ReasonCounts, wantCounts) {
		t.Fatal("all-failed input lost rejection evidence")
	}
	if strings.Contains(allFailed.DiagnosticSummary()+err.Error(), "PRIVATE_") || strings.Contains(err.Error(), "private-host") {
		t.Fatal("all-failed error leaked input")
	}
}

// A partial refresh changes current membership, not ownership of shared nodes
// or historical storage. No production database or network is used here.
func TestSyntheticPartialFormatsDoNotPolluteCurrentMembership(t *testing.T) {
	const current = " - {name: current, type: http, server: 192.0.2.130, port: 8080}\n"
	const old = " - {name: old, type: socks5, server: 192.0.2.131, port: 1080}\n"
	const fresh = " - {name: fresh, type: anytls, server: 192.0.2.132, port: 443, password: invented-anytls}\n"
	for _, shared := range []bool{false, true} {
		name := "sole"
		if shared {
			name = "shared"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := openIntegrationDB(t)
			subs := sqlite.NewSubscriptionRepository(db)
			fetches := sqlite.NewSubscriptionFetchRepository(db)
			nodes := sqlite.NewNodeRepository(db)
			sources := sqlite.NewNodeSourceRepository(db)
			reconcile := func(id, body, digest string) *inventory.ReconcileResult {
				t.Helper()
				svc := inventory.NewService(db, subs, fetches, nodes, sources, fixtureFetcher{response: &fetch.Response{StatusCode: 200, Body: []byte(body), ContentDigest: digest}})
				res, err := svc.ReconcileSubscription(ctx, id)
				if err != nil || res == nil {
					t.Fatalf("reconcile: %v", err)
				}
				return res
			}
			for _, id := range []string{"primary", "other"} {
				now := domain.NowUTC()
				if err := subs.Create(ctx, &domain.Subscription{ID: id, Name: id, Enabled: true, SourceURLSecretRef: "https://fixture.invalid/" + id, Revision: domain.MustNewUUIDv7(), CreatedAt: now, UpdatedAt: now}); err != nil {
					t.Fatal(err)
				}
			}
			initial := reconcile("primary", "proxies:\n"+current+old, "synthetic-initial")
			if initial.Outcome != domain.FetchOutcomeSuccess || initial.NodesValid != 2 {
				t.Fatal("initial inventory not established")
			}
			if shared {
				if res := reconcile("other", "proxies:\n"+old, "synthetic-shared"); res.Outcome != domain.FetchOutcomeSuccess || res.NodesValid != 1 {
					t.Fatal("shared inventory not established")
				}
			}
			partial := reconcile("primary", "proxies:\n"+current+fresh+" - {type: anytls, server: 192.0.2.133, port: 443}\n", "synthetic-partial")
			if partial.Outcome != domain.FetchOutcomePartial || partial.NodesParsed != 3 || partial.NodesValid != 2 || !strings.Contains(partial.RedactedError, "missing_credential") {
				t.Fatalf("partial parse evidence missing: %+v", partial)
			}
			want := map[string]bool{"current": true, "fresh": true}
			if shared {
				want["old"] = true
			}
			visible, err := nodes.ListAll(ctx, domain.NodeFilter{Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, ExcludeNotices: true})
			if err != nil || len(visible) != len(want) {
				t.Fatalf("current membership count=%d want=%d error=%v", len(visible), len(want), err)
			}
			for _, n := range visible {
				if !want[n.DisplayName] {
					t.Fatal("stale or rejected node polluted current inventory")
				}
				delete(want, n.DisplayName)
			}
			if len(want) != 0 {
				t.Fatal("successful current or shared node missing")
			}
			ledger, err := nodes.ListAll(ctx, domain.NodeFilter{ActiveOnly: true})
			if err != nil || len(ledger) != 3 {
				t.Fatalf("partial refresh deleted history or stored rejected entry: count=%d error=%v", len(ledger), err)
			}
			for _, n := range ledger {
				if n.DisplayName != "old" {
					continue
				}
				var lastSeen string
				if err := db.QueryRowContext(ctx, "SELECT last_seen_fetch_id FROM node_sources WHERE subscription_id = ? AND node_logical_id = ?", "primary", n.LogicalID).Scan(&lastSeen); err != nil || lastSeen != initial.FetchID {
					t.Fatalf("old source history was deleted or falsely marked current: %v", err)
				}
			}
		})
	}
}
