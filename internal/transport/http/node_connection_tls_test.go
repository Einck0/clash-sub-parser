package http_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
	transporthttp "clash-sub-parser/internal/transport/http"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badoption"
	"gopkg.in/yaml.v3"
)

type tlsReadbackFetcher struct{}

func (tlsReadbackFetcher) Fetch(context.Context, fetch.Options) (*fetch.Response, error) {
	body := []byte(`proxies:
  - name: Imported Trojan TLS
    type: trojan
    server: trojan.synthetic.invalid
    port: 443
    password: synthetic-trojan-password
    sni: old.synthetic.invalid
    alpn: [h2]
  - name: Imported TUIC TLS
    type: tuic
    server: tuic.synthetic.invalid
    port: 443
    uuid: 11111111-2222-4333-8444-555555555555
    password: synthetic-tuic-password
    sni: old.synthetic.invalid
    alpn: [h2]
`)
	return &fetch.Response{StatusCode: http.StatusOK, Body: body, ContentDigest: fmt.Sprintf("sha256:%x", sha256.Sum256(body))}, nil
}

func TestNodeConnectionTLSHTTPImportedReadback(t *testing.T) {
	for _, protocol := range []domain.Protocol{domain.ProtocolTrojan, domain.ProtocolTUIC} {
		t.Run(string(protocol), func(t *testing.T) {
			ctx := context.Background()
			db := newCleanSQLiteDB(t)
			nodes := sqlite.NewNodeRepository(db)
			subs := sqlite.NewSubscriptionRepository(db)
			service := inventory.NewService(db, subs, sqlite.NewSubscriptionFetchRepository(db), nodes, sqlite.NewNodeSourceRepository(db), tlsReadbackFetcher{})
			now := domain.NowUTC()
			sub := &domain.Subscription{
				ID: domain.MustNewUUIDv7(), Name: "Synthetic TLS import", SourceURLSecretRef: "https://subscription.synthetic.invalid/tls",
				Enabled: true, Revision: "rev-1", CreatedAt: now, UpdatedAt: now,
				RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 3600, TimeoutSeconds: 30, MaxResponseBytes: 1024 * 1024},
			}
			if err := subs.Create(ctx, sub); err != nil {
				t.Fatal(err)
			}
			result, err := service.ReconcileSubscription(ctx, sub.ID)
			if err != nil || result.NodesValid != 2 {
				t.Fatalf("import result=%+v err=%v", result, err)
			}
			imported, total, err := nodes.List(ctx, domain.NodeFilter{})
			if err != nil || total != 2 {
				t.Fatalf("imported nodes=%d err=%v", total, err)
			}
			var node domain.Node
			for _, candidate := range imported {
				if candidate.Protocol == protocol {
					node = candidate
				}
			}
			if node.LogicalID == "" || node.Credentials.Transport["sni"] != "old.synthetic.invalid" || node.Credentials.Transport["alpn"] != "h2" {
				t.Fatalf("parser did not retain imported TLS transport: %+v", node)
			}
			assertTLSReadbackExport(t, node, map[domain.CompilerTarget]string{
				domain.TargetMihomo: "old.synthetic.invalid", domain.TargetSingBox: "old.synthetic.invalid",
			}, []string{"h2"})
			cfg := newTestRouterConfig(true)
			cfg.InventoryService = service
			router := transporthttp.NewRouter(cfg)
			path := "/api/v1/nodes/" + node.LogicalID
			request := func(method, body string, status int) *httptest.ResponseRecorder {
				t.Helper()
				url := path
				if method == http.MethodPatch {
					url += "/connection"
				}
				req := httptest.NewRequest(method, url, strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+testAdminToken)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code != status {
					t.Fatalf("%s status=%d want=%d body=%s", method, rec.Code, status, rec.Body.String())
				}
				return rec
			}
			decode := func(rec *httptest.ResponseRecorder) inventory.NodeView {
				t.Helper()
				var response nodeDetailResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				return response.Data.Node
			}
			initial := decode(request(http.MethodGet, "", http.StatusOK))
			unchanged := decode(request(http.MethodPatch, `{"display_name":"Renamed TLS node"}`, http.StatusOK))
			if unchanged.ConnectionRevision != initial.ConnectionRevision || !reflect.DeepEqual(unchanged.Credentials, initial.Credentials) {
				t.Fatalf("name-only edit changed connection: before=%+v after=%+v", initial, unchanged)
			}
			for _, field := range []string{"password", "uuid"} {
				if field == "uuid" && protocol != domain.ProtocolTUIC {
					continue
				}
				rejected := request(http.MethodPatch, fmt.Sprintf(`{"%s":"","sni":"","alpn":[]}`, field), http.StatusUnprocessableEntity)
				if !strings.Contains(rejected.Body.String(), "invalid_"+field) {
					t.Fatalf("unexpected validation branch: %s", rejected.Body.String())
				}
			}
			afterRejected := decode(request(http.MethodGet, "", http.StatusOK))
			if afterRejected.ConnectionRevision != initial.ConnectionRevision || !reflect.DeepEqual(afterRejected.Credentials, initial.Credentials) {
				t.Fatal("rejected required-credential edit changed the connection")
			}

			assertCleared := func(rec *httptest.ResponseRecorder, revision int64) {
				t.Helper()
				view := decode(rec)
				if view.ConnectionRevision != revision || view.Credentials == nil {
					t.Fatalf("unexpected cleared revision/credentials: %+v", view)
				}
				wire, err := json.Marshal(view.Credentials)
				if err != nil {
					t.Fatal(err)
				}
				assertTLSReadbackJSONCleared(t, wire)
				var raw struct {
					Data struct {
						Node struct {
							Credentials json.RawMessage `json:"credentials"`
						} `json:"node"`
					} `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
					t.Fatal(err)
				}
				assertTLSReadbackJSONCleared(t, raw.Data.Node.Credentials)
				stored, err := nodes.GetByLogicalID(ctx, node.LogicalID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.ConnectionRevision != revision || stored.Credentials.Password != node.Credentials.Password || stored.Credentials.UUID != node.Credentials.UUID {
					t.Fatalf("persisted identity/credentials changed: %+v", stored)
				}
				var configJSON, effectiveJSON string
				if err := db.QueryRowContext(ctx, `SELECT n.config_json,v.effective_config_json FROM nodes n JOIN node_connection_versions v ON v.node_logical_id=n.logical_id AND v.connection_revision=n.connection_revision WHERE n.logical_id=?`, node.LogicalID).Scan(&configJSON, &effectiveJSON); err != nil {
					t.Fatal(err)
				}
				assertTLSReadbackJSONCleared(t, []byte(configJSON))
				var effective struct {
					Credentials json.RawMessage `json:"credentials"`
				}
				if err := json.Unmarshal([]byte(effectiveJSON), &effective); err != nil {
					t.Fatal(err)
				}
				assertTLSReadbackJSONCleared(t, effective.Credentials)
				assertTLSReadbackExport(t, *stored, map[domain.CompilerTarget]string{
					domain.TargetMihomo: "", domain.TargetSingBox: stored.Server,
				}, nil)
			}
			clearBody := `{"sni":"","alpn":[]}`
			revision := initial.ConnectionRevision + 1
			assertCleared(request(http.MethodPatch, clearBody, http.StatusOK), revision)
			assertCleared(request(http.MethodGet, "", http.StatusOK), revision)
			assertCleared(request(http.MethodPatch, clearBody, http.StatusOK), revision)

			// Transport replacement in the same request cannot keep aliases that defeat the typed edit.
			aliasedTransport := `"transport":{"network":"` + node.Credentials.Transport["network"] + `","tls":"true","sni":"stale.synthetic.invalid","servername":"stale.synthetic.invalid","serverName":"stale.synthetic.invalid","peer":"stale.synthetic.invalid","alpn":"h2"}`
			replaced := decode(request(http.MethodPatch, `{"sni":" new.synthetic.invalid ","alpn":[" h3 ",""],`+aliasedTransport+`}`, http.StatusOK))
			if replaced.ConnectionRevision != revision+1 || replaced.Credentials.SNI != "new.synthetic.invalid" || !reflect.DeepEqual(replaced.Credentials.ALPN, []string{"h3"}) || replaced.Credentials.Transport["sni"] != "new.synthetic.invalid" || replaced.Credentials.Transport["alpn"] != "h3" {
				t.Fatalf("typed edit not synchronized with transport: %+v", replaced)
			}
			for _, alias := range []string{"servername", "serverName", "peer"} {
				if _, present := replaced.Credentials.Transport[alias]; present {
					t.Fatalf("stale SNI alias retained: %s", alias)
				}
			}
			replacedNode, err := nodes.GetByLogicalID(ctx, node.LogicalID)
			if err != nil {
				t.Fatal(err)
			}
			assertTLSReadbackExport(t, *replacedNode, map[domain.CompilerTarget]string{
				domain.TargetMihomo: "new.synthetic.invalid", domain.TargetSingBox: "new.synthetic.invalid",
			}, []string{"h3"})
			assertCleared(request(http.MethodPatch, `{"sni":"","alpn":[],`+aliasedTransport+`}`, http.StatusOK), revision+2)
			assertCleared(request(http.MethodGet, "", http.StatusOK), revision+2)
		})
	}
}

func assertTLSReadbackJSONCleared(t *testing.T, data []byte) {
	t.Helper()
	var credentials map[string]json.RawMessage
	if err := json.Unmarshal(data, &credentials); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sni", "alpn"} {
		if _, present := credentials[key]; present {
			t.Fatalf("empty typed %s must be omitted on the real wire: %s", key, data)
		}
	}
	var transport map[string]string
	if err := json.Unmarshal(credentials["transport"], &transport); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sni", "servername", "serverName", "peer", "alpn"} {
		if _, present := transport[key]; present {
			t.Fatalf("cleared transport alias %s survives: %s", key, data)
		}
	}
	if transport["network"] == "" || transport["tls"] != "true" {
		t.Fatalf("clear removed unrelated transport: %s", data)
	}
}

func assertTLSReadbackExport(t *testing.T, node domain.Node, expectedSNI map[domain.CompilerTarget]string, alpn []string) {
	t.Helper()
	snapshot := &resolver.ResolvedPolicySnapshot{SnapshotDigest: "synthetic-tls-readback", Nodes: []resolver.ResolvedNode{{
		LogicalID: node.LogicalID, DisplayName: node.DisplayName, Protocol: node.Protocol, Server: node.Server, Port: node.Port,
		Credentials: node.Credentials, Active: true, ConnectionRevision: node.ConnectionRevision,
	}}}
	for _, target := range []domain.CompilerTarget{domain.TargetMihomo, domain.TargetSingBox} {
		result, err := compiler.Compile(context.Background(), snapshot, target)
		if err != nil {
			t.Fatalf("render %s after edit: %v", target, err)
		}
		var gotSNI string
		var gotALPN []string
		var insecure, disableSNI bool
		if target == domain.TargetMihomo {
			var config struct {
				Proxies []struct {
					SNI            string   `yaml:"sni"`
					ALPN           []string `yaml:"alpn"`
					SkipCertVerify bool     `yaml:"skip-cert-verify"`
					DisableSNI     bool     `yaml:"disable-sni"`
				} `yaml:"proxies"`
			}
			if err := yaml.Unmarshal(result.Content, &config); err != nil || len(config.Proxies) != 1 {
				t.Fatalf("decode rendered Mihomo: err=%v content=%s", err, result.Content)
			}
			gotSNI, gotALPN = config.Proxies[0].SNI, config.Proxies[0].ALPN
			insecure, disableSNI = config.Proxies[0].SkipCertVerify, config.Proxies[0].DisableSNI
		} else {
			var config struct {
				Outbounds []struct {
					Tag string `json:"tag"`
					TLS struct {
						ServerName string                     `json:"server_name"`
						ALPN       badoption.Listable[string] `json:"alpn"`
						Insecure   bool                       `json:"insecure"`
						DisableSNI bool                       `json:"disable_sni"`
					} `json:"tls"`
				} `json:"outbounds"`
			}
			if err := singjson.UnmarshalContext(context.Background(), result.Content, &config); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, outbound := range config.Outbounds {
				if outbound.Tag == node.DisplayName {
					found = true
					gotSNI, gotALPN = outbound.TLS.ServerName, []string(outbound.TLS.ALPN)
					insecure, disableSNI = outbound.TLS.Insecure, outbound.TLS.DisableSNI
				}
			}
			if !found {
				t.Fatal("render omitted the edited node")
			}
		}
		wantSNI, present := expectedSNI[target]
		if !present || gotSNI != wantSNI || !reflect.DeepEqual(gotALPN, alpn) {
			t.Fatalf("%s effective TLS=%q/%v want=%q/%v", target, gotSNI, gotALPN, wantSNI, alpn)
		}
		if insecure || disableSNI {
			t.Fatalf("%s custom override edit disabled SNI or certificate verification", target)
		}
	}
}
