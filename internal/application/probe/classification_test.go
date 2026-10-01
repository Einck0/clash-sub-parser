package probe_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/probe/singbox"
)

func TestRunnerReasonCategoriesHaveNoSecretLeakageAndAssociateRevision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer tlsSrv.Close()
	_, _ = tlsSrv.Client().Get(tlsSrv.URL)
	secret := "secret-value-never-log"
	host := "hidden-host-name.example"
	for _, tc := range []struct {
		name   string
		node   domain.Node
		dial   probe.NodeDialer
		reason string
	}{
		{name: "missing_uuid", node: domain.Node{LogicalID: "1", Protocol: domain.ProtocolVLESS, Server: "93.184.216.34", Port: 443, Active: true, ConnectionRevision: 2}, reason: "credentials_unavailable"},
		{name: "unsafe_tls", node: domain.Node{LogicalID: "2", Protocol: domain.ProtocolHysteria2, Server: "93.184.216.34", Port: 443, Active: true, ConnectionRevision: 3, Credentials: domain.InboundProtocolCredential{Password: secret, Transport: map[string]string{"skip_cert_verify": "true"}}}, reason: "unsafe_tls_rejected"},
		{name: "private_target", node: domain.Node{LogicalID: "3", Protocol: domain.ProtocolSS, Server: "127.0.0.1", Port: 443, Active: true, ConnectionRevision: 4, Credentials: domain.InboundProtocolCredential{Password: secret}}, reason: "private_target_rejected"},
		{name: "client_build_failed", node: domain.Node{LogicalID: "4", Protocol: domain.ProtocolVLESS, Server: "93.184.216.34", Port: 443, Active: true, ConnectionRevision: 5, Credentials: domain.InboundProtocolCredential{UUID: secret}}, dial: probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{ClientFactory: func(context.Context, singbox.NodeConfig, singbox.HTTPClientOptions) (*http.Client, func() error, error) {
			return nil, nil, errors.New(secret + host)
		}}), reason: "client_build_failed"},
		{name: "dns_error", node: domain.Node{LogicalID: "5", Protocol: domain.ProtocolSS, Server: host, Port: 443, Active: true, ConnectionRevision: 6, Credentials: domain.InboundProtocolCredential{Password: secret}}, dial: func(context.Context, domain.Node) (*http.Client, func() error, error) {
			return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, &net.DNSError{Err: secret, Name: host} })}, nil, nil
		}, reason: "dns_error"},
		{name: "timeout", node: domain.Node{LogicalID: "6", Protocol: domain.ProtocolSS, Server: host, Port: 443, Active: true, ConnectionRevision: 7, Credentials: domain.InboundProtocolCredential{Password: secret}}, dial: func(context.Context, domain.Node) (*http.Client, func() error, error) {
			return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}, nil, nil
		}, reason: "timeout"},
		{name: "transport_error", node: domain.Node{LogicalID: "7", Protocol: domain.ProtocolSS, Server: host, Port: 443, Active: true, ConnectionRevision: 8, Credentials: domain.InboundProtocolCredential{Password: secret}}, dial: func(context.Context, domain.Node) (*http.Client, func() error, error) {
			return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New(secret + host) })}, nil, nil
		}, reason: "transport_error"},
		{name: "local_loopback_fixture", node: domain.Node{LogicalID: "8", Protocol: domain.ProtocolVLESS, Server: "93.184.216.34", Port: 443, Active: true, ConnectionRevision: 9, Credentials: domain.InboundProtocolCredential{UUID: secret, Transport: map[string]string{"flow": "xtls-rprx-vision", "pbk": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "sni": "example.org"}}}, dial: probe.NewSafeNodeDialer(probe.SafeNodeDialerOptions{ClientFactory: func(_ context.Context, cfg singbox.NodeConfig, _ singbox.HTTPClientOptions) (*http.Client, func() error, error) {
			if _, _, err := singbox.BuildOptions(cfg); err != nil {
				return nil, nil, err
			}
			return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				proxyReq, _ := http.NewRequestWithContext(req.Context(), http.MethodGet, srv.URL, nil)
				return srv.Client().Do(proxyReq)
			})}, nil, nil
		}}), reason: "contract_matched"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs := newMemoryRuns()
			obsRepo := newMemoryObservations()
			nodes := newMemoryNodes()
			nodes.items[tc.node.LogicalID] = tc.node
			sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
			if err != nil {
				t.Fatal(err)
			}
			defer sched.Close()
			dial := tc.dial
			if dial == nil {
				dial = probe.NewSafeNodeDialer()
			}
			runner := probe.NewDefaultRunner(nodes, obsRepo, sched, runs, probe.WithNodeDialer(dial))
			run := &domain.ProbeRun{ID: "run-" + tc.name, IdempotencyKey: tc.name, ActorScope: "admin", State: domain.ProbeRunStateQueued, DeadlineAt: time.Now().Add(time.Hour)}
			if err := runs.Create(context.Background(), run); err != nil {
				t.Fatal(err)
			}
			_ = runner.Run(context.Background(), run, []string{tc.node.LogicalID}, []domain.ProbeKind{domain.ProbeKindBaseline})
			items, err := obsRepo.ListByRun(context.Background(), run.ID)
			if err != nil || len(items) != 1 {
				t.Fatalf("observations %+v %v", items, err)
			}
			got := items[0]
			if !strings.Contains(got.RedactedSummary, "reason="+tc.reason) || strings.Contains(got.RedactedSummary, secret) || strings.Contains(got.RedactedSummary, host) {
				t.Fatalf("unexpected summary %q for %s", got.RedactedSummary, tc.reason)
			}
			if got.ConnectionRevision == nil || *got.ConnectionRevision != tc.node.ConnectionRevision {
				t.Fatalf("observation revision %+v want %d", got.ConnectionRevision, tc.node.ConnectionRevision)
			}
			if tc.reason == "transport_error" {
				if status, _ := domain.EvaluateBaselineHealth(&got, tc.node.ConnectionRevision, time.Now().UTC(), time.Hour); status != domain.BaselineUnknown {
					t.Fatalf("ambiguous transport_error must remain unknown, got %s", status)
				}
			}
		})
	}
}
