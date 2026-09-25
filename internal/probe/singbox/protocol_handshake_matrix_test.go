package singbox_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/probe/singbox"

	box "github.com/sagernet/sing-box"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

const (
	matrixNodeIP   = "93.184.216.34"
	matrixTargetIP = "93.184.216.35"
	matrixDomain   = "probe-matrix.example.test"
	matrixTarget   = "target-matrix.example.test"
)

type matrixCase struct {
	name      string
	protocol  domain.Protocol
	tls       bool
	udp       bool
	transport string
}

func TestProtocolMatrixRealHandshake204(t *testing.T) {
	cases := []matrixCase{
		{name: "ss", protocol: domain.ProtocolSS},
		{name: "trojan", protocol: domain.ProtocolTrojan, tls: true},
		{name: "vless", protocol: domain.ProtocolVLESS, tls: true},
		{name: "vmess", protocol: domain.ProtocolVMess, tls: true},
		{name: "hysteria2", protocol: domain.ProtocolHysteria2, tls: true, udp: true},
		{name: "tuic", protocol: domain.ProtocolTUIC, tls: true, udp: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runMatrixHandshake(t, tc) })
	}
}

func TestTransportMatrixRealHandshake204(t *testing.T) {
	for _, transport := range []string{"ws", "http", "httpupgrade", "grpc"} {
		for _, protocol := range []domain.Protocol{domain.ProtocolTrojan, domain.ProtocolVLESS, domain.ProtocolVMess} {
			t.Run(string(protocol)+"/"+transport, func(t *testing.T) {
				runMatrixHandshake(t, matrixCase{name: string(protocol) + "-" + transport, protocol: protocol, tls: true, transport: transport})
			})
		}
	}
}

func matrixNode(tc matrixCase) singbox.NodeConfig {
	cfg := singbox.NodeConfig{LogicalID: "matrix-" + tc.name, Protocol: tc.protocol, Server: matrixNodeIP, Port: 443, TLS: tc.tls, SNI: matrixDomain, Password: "matrix-secret", UUID: "00000000-0000-0000-0000-000000000001", Method: "aes-128-gcm", Network: "tcp", ServiceName: "matrix"}
	if tc.udp {
		cfg.Network = "udp"
	}
	if tc.transport != "" {
		cfg.Network = tc.transport
		cfg.Path = "/matrix"
		cfg.Headers = map[string]string{"Host": matrixDomain}
		cfg.Transport = map[string]string{"network": tc.transport, "path": cfg.Path, "host": matrixDomain}
		if tc.transport == "grpc" {
			cfg.Headers["Host"] = matrixDomain
		}
	}
	return cfg
}

func runMatrixHandshake(t *testing.T, tc matrixCase) {
	t.Helper()
	if err := fetch.DefaultPolicy().ValidateIP(net.ParseIP(matrixNodeIP)); err != nil {
		t.Fatal(err)
	}
	if err := fetch.DefaultPolicy().ValidateIP(net.ParseIP(matrixTargetIP)); err != nil {
		t.Fatal(err)
	}
	var targetHits atomic.Int64
	var observedHost atomic.Value
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/generate_204" {
			http.NotFound(w, r)
			return
		}
		observedHost.Store(r.Host)
		targetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	certs := generateTestCertBundle(t, matrixDomain)
	inboundPort := allocateFreeTCPPort(t)
	if tc.udp {
		inboundPort = allocateFreeUDPPort(t)
	}
	localNode := net.JoinHostPort("127.0.0.1", strconv.Itoa(inboundPort))
	listenAddr := badoption.Addr(netip.MustParseAddr("127.0.0.1"))
	serverMap := newLoopbackMapperState()
	targetAddr := net.JoinHostPort(matrixTargetIP, "8080")
	serverMap.MapTCP(targetAddr, target.Listener.Addr().String())
	serverMap.MapUDP(targetAddr, target.Listener.Addr().String())
	clientMap := newLoopbackMapperState()
	nodeAddr := net.JoinHostPort(matrixNodeIP, "443")
	if tc.udp {
		nodeAddr = net.JoinHostPort(matrixNodeIP, strconv.Itoa(inboundPort))
		clientMap.MapUDP(nodeAddr, localNode)
	} else {
		clientMap.MapTCP(nodeAddr, localNode)
	}

	inboundType := map[domain.Protocol]string{domain.ProtocolSS: C.TypeShadowsocks, domain.ProtocolTrojan: C.TypeTrojan, domain.ProtocolVLESS: C.TypeVLESS, domain.ProtocolVMess: C.TypeVMess, domain.ProtocolHysteria2: C.TypeHysteria2, domain.ProtocolTUIC: C.TypeTUIC}[tc.protocol]
	inOpt := any(nil)
	switch tc.protocol {
	case domain.ProtocolSS:
		inOpt = &option.ShadowsocksInboundOptions{ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)}, Method: "aes-128-gcm", Password: "matrix-secret"}
	case domain.ProtocolTrojan:
		to, _, _ := singbox.BuildOptions(singbox.NodeConfig{LogicalID: "x", Protocol: domain.ProtocolTrojan, Server: matrixNodeIP, Port: 443, Password: "x", TLS: true, SNI: matrixDomain, Network: tc.transport, Path: "/matrix", Headers: map[string]string{"Host": matrixDomain}, ServiceName: "matrix"})
		transport := to.Outbounds[0].Options.(*option.TrojanOutboundOptions).Transport
		inOpt = &option.TrojanInboundOptions{ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)}, Users: []option.TrojanUser{{Name: "matrix", Password: "matrix-secret"}}, InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{TLS: &option.InboundTLSOptions{Enabled: true, Certificate: badoption.Listable[string]{certs.ServerCertPEM}, Key: badoption.Listable[string]{certs.ServerKeyPEM}}}, Transport: transport}
	case domain.ProtocolVLESS:
		out, _, _ := singbox.BuildOptions(matrixNode(tc))
		transport := out.Outbounds[0].Options.(*option.VLESSOutboundOptions).Transport
		inOpt = &option.VLESSInboundOptions{ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)}, Users: []option.VLESSUser{{Name: "matrix", UUID: "00000000-0000-0000-0000-000000000001"}}, InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{TLS: &option.InboundTLSOptions{Enabled: true, Certificate: badoption.Listable[string]{certs.ServerCertPEM}, Key: badoption.Listable[string]{certs.ServerKeyPEM}}}, Transport: transport}
	case domain.ProtocolVMess:
		out, _, _ := singbox.BuildOptions(matrixNode(tc))
		transport := out.Outbounds[0].Options.(*option.VMessOutboundOptions).Transport
		inOpt = &option.VMessInboundOptions{ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)}, Users: []option.VMessUser{{Name: "matrix", UUID: "00000000-0000-0000-0000-000000000001"}}, InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{TLS: &option.InboundTLSOptions{Enabled: true, Certificate: badoption.Listable[string]{certs.ServerCertPEM}, Key: badoption.Listable[string]{certs.ServerKeyPEM}}}, Transport: transport}
	case domain.ProtocolHysteria2:
		inOpt = &option.Hysteria2InboundOptions{ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)}, Users: []option.Hysteria2User{{Name: "matrix", Password: "matrix-secret"}}, InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{TLS: &option.InboundTLSOptions{Enabled: true, Certificate: badoption.Listable[string]{certs.ServerCertPEM}, Key: badoption.Listable[string]{certs.ServerKeyPEM}}}}
	case domain.ProtocolTUIC:
		inOpt = &option.TUICInboundOptions{ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)}, Users: []option.TUICUser{{Name: "matrix", UUID: "00000000-0000-0000-0000-000000000001", Password: "matrix-secret"}}, InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{TLS: &option.InboundTLSOptions{Enabled: true, Certificate: badoption.Listable[string]{certs.ServerCertPEM}, Key: badoption.Listable[string]{certs.ServerKeyPEM}}}}
	}
	inbound := option.Inbound{Type: inboundType, Tag: "matrix-in", Options: inOpt}
	serverOpts := option.Options{Log: &option.LogOptions{Disabled: true}, Inbounds: []option.Inbound{inbound}, Outbounds: []option.Outbound{{Type: testLoopbackMapperType, Tag: "target-map", Options: &testLoopbackMapperOptions{State: serverMap}}}, Route: &option.RouteOptions{Final: "target-map"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := box.New(box.Options{Context: singbox.RuntimeContextWithOutboundRegistry(ctx, getTestOutboundRegistry()), Options: serverOpts})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err = srv.Start(); err != nil {
		t.Fatalf("start inbound: %v", err)
	}
	defer srv.Close()

	cfg := matrixNode(tc)
	if tc.udp {
		cfg.Port = inboundPort
	} else {
		cfg.Port = 443
	}
	clientBuild := &singbox.BoxBuildConfig{Context: func(c context.Context) context.Context {
		return singbox.RuntimeContextWithOutboundRegistry(c, getTestOutboundRegistry())
	}, Mutate: func(opts *option.Options, tag string) error {
		if tc.tls {
			opts.Certificate = &option.CertificateOptions{Store: C.CertificateStoreNone, Certificate: badoption.Listable[string]{certs.CACertPEM}}
		}
		if err := injectOutboundDetour(opts, "node-map"); err != nil {
			return err
		}
		opts.Outbounds = append(opts.Outbounds, option.Outbound{Type: testLoopbackMapperType, Tag: "node-map", Options: &testLoopbackMapperOptions{State: clientMap}})
		return nil
	}}
	// Set server to public IP/port from fixture; TCP and UDP are separate records.
	if tc.udp {
		cfg.Port = inboundPort
	}
	rt, err := singbox.NewWithBoxConfig(context.Background(), cfg, clientBuild)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer rt.Close()
	resolver := &staticPublicResolver{records: map[string][]net.IPAddr{matrixTarget: {{IP: net.ParseIP(matrixTargetIP)}}}}
	resp, err := rt.HTTPClient(singbox.HTTPClientOptions{Resolver: resolver, Timeout: 8 * time.Second}).Get("http://" + matrixTarget + ":8080/generate_204")
	if err != nil {
		t.Fatalf("HTTP 204 probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || targetHits.Load() != 1 {
		t.Fatalf("status=%d targetHits=%d", resp.StatusCode, targetHits.Load())
	}
	gotHost, _ := observedHost.Load().(string)
	if gotHost != net.JoinHostPort(matrixTarget, "8080") {
		t.Fatalf("target Host=%q", gotHost)
	}
	if got := clientMap.Unauthorized(); len(got) != 0 {
		t.Fatalf("unauthorized node socket: %+v", got)
	}
	if len(clientMap.Authorized()) == 0 {
		t.Fatal("no node socket captured")
	}
	if len(serverMap.Authorized()) == 0 {
		t.Fatal("no target socket captured")
	}
	if got := clientMap.Authorized(); len(got) != 1 || got[0].Destination != nodeAddr {
		t.Fatalf("node socket records=%+v, want exactly %s", got, nodeAddr)
	}
	if got := serverMap.Authorized(); len(got) != 1 || got[0].Destination != targetAddr {
		t.Fatalf("target socket records=%+v, want exactly %s", got, targetAddr)
	}
}

func TestTransportMatrixRealHandshake204_PostBodyBeforeResponseAndBoundedCancel(t *testing.T) {
	expectedBody := bytes.Repeat([]byte("csp-h2-grpc-late-conn-post-body-"), 1024) // 32 KiB deterministic POST body

	for _, transport := range []string{"http", "grpc"} {
		for _, protocol := range []domain.Protocol{domain.ProtocolTrojan, domain.ProtocolVLESS, domain.ProtocolVMess} {
			tc := matrixCase{
				name:      string(protocol) + "-" + transport + "-post-sync",
				protocol:  protocol,
				tls:       true,
				transport: transport,
			}
			t.Run(string(protocol)+"/"+transport+"/post_body_before_204", func(t *testing.T) {
				runMatrixPostBodyBeforeResponseAndCancel(t, tc, expectedBody, false)
			})
			t.Run(string(protocol)+"/"+transport+"/bounded_cancel_waiting_response", func(t *testing.T) {
				runMatrixPostBodyBeforeResponseAndCancel(t, tc, expectedBody, true)
			})
		}
	}
}

func runMatrixPostBodyBeforeResponseAndCancel(t *testing.T, tc matrixCase, expectedBody []byte, cancelAfterBody bool) {
	t.Helper()
	if err := fetch.DefaultPolicy().ValidateIP(net.ParseIP(matrixNodeIP)); err != nil {
		t.Fatal(err)
	}
	if err := fetch.DefaultPolicy().ValidateIP(net.ParseIP(matrixTargetIP)); err != nil {
		t.Fatal(err)
	}

	var (
		fullBodyRead atomic.Int64
		targetHits   atomic.Int64
		observedHost atomic.Value
		bodyRecvOnce sync.Once
		bodyRecvCh   = make(chan struct{})
		releaseSrvCh = make(chan struct{})
	)
	defer close(releaseSrvCh)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/generate_204" || r.Method != http.MethodPost {
			http.Error(w, "method or path mismatch", http.StatusBadRequest)
			return
		}
		buf := make([]byte, len(expectedBody))
		if _, err := io.ReadFull(r.Body, buf); err != nil {
			http.Error(w, "failed to read full post body", http.StatusBadRequest)
			return
		}
		if !bytes.Equal(buf, expectedBody) {
			http.Error(w, "post body content mismatch", http.StatusBadRequest)
			return
		}
		fullBodyRead.Add(1)
		observedHost.Store(r.Host)
		bodyRecvOnce.Do(func() {
			close(bodyRecvCh)
		})

		if cancelAfterBody {
			select {
			case <-releaseSrvCh:
			case <-r.Context().Done():
			}
			return
		}

		targetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	certs := generateTestCertBundle(t, matrixDomain)
	inboundPort := allocateFreeTCPPort(t)
	localNode := net.JoinHostPort("127.0.0.1", strconv.Itoa(inboundPort))
	listenAddr := badoption.Addr(netip.MustParseAddr("127.0.0.1"))

	serverMap := newLoopbackMapperState()
	targetAddr := net.JoinHostPort(matrixTargetIP, "8080")
	serverMap.MapTCP(targetAddr, target.Listener.Addr().String())

	clientMap := newLoopbackMapperState()
	nodeAddr := net.JoinHostPort(matrixNodeIP, "443")
	clientMap.MapTCP(nodeAddr, localNode)

	inboundType := map[domain.Protocol]string{
		domain.ProtocolTrojan: C.TypeTrojan,
		domain.ProtocolVLESS:  C.TypeVLESS,
		domain.ProtocolVMess:  C.TypeVMess,
	}[tc.protocol]

	var inOpt any
	switch tc.protocol {
	case domain.ProtocolTrojan:
		to, _, _ := singbox.BuildOptions(singbox.NodeConfig{
			LogicalID:   "x",
			Protocol:    domain.ProtocolTrojan,
			Server:      matrixNodeIP,
			Port:        443,
			Password:    "x",
			TLS:         true,
			SNI:         matrixDomain,
			Network:     tc.transport,
			Path:        "/matrix",
			Headers:     map[string]string{"Host": matrixDomain},
			ServiceName: "matrix",
		})
		transport := to.Outbounds[0].Options.(*option.TrojanOutboundOptions).Transport
		inOpt = &option.TrojanInboundOptions{
			ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
			Users:         []option.TrojanUser{{Name: "matrix", Password: "matrix-secret"}},
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &option.InboundTLSOptions{
					Enabled:     true,
					Certificate: badoption.Listable[string]{certs.ServerCertPEM},
					Key:         badoption.Listable[string]{certs.ServerKeyPEM},
				},
			},
			Transport: transport,
		}
	case domain.ProtocolVLESS:
		out, _, _ := singbox.BuildOptions(matrixNode(tc))
		transport := out.Outbounds[0].Options.(*option.VLESSOutboundOptions).Transport
		inOpt = &option.VLESSInboundOptions{
			ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
			Users:         []option.VLESSUser{{Name: "matrix", UUID: "00000000-0000-0000-0000-000000000001"}},
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &option.InboundTLSOptions{
					Enabled:     true,
					Certificate: badoption.Listable[string]{certs.ServerCertPEM},
					Key:         badoption.Listable[string]{certs.ServerKeyPEM},
				},
			},
			Transport: transport,
		}
	case domain.ProtocolVMess:
		out, _, _ := singbox.BuildOptions(matrixNode(tc))
		transport := out.Outbounds[0].Options.(*option.VMessOutboundOptions).Transport
		inOpt = &option.VMessInboundOptions{
			ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
			Users:         []option.VMessUser{{Name: "matrix", UUID: "00000000-0000-0000-0000-000000000001"}},
			InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
				TLS: &option.InboundTLSOptions{
					Enabled:     true,
					Certificate: badoption.Listable[string]{certs.ServerCertPEM},
					Key:         badoption.Listable[string]{certs.ServerKeyPEM},
				},
			},
			Transport: transport,
		}
	}

	serverOpts := option.Options{
		Log:      &option.LogOptions{Disabled: true},
		Inbounds: []option.Inbound{{Type: inboundType, Tag: "matrix-in", Options: inOpt}},
		Outbounds: []option.Outbound{{
			Type:    testLoopbackMapperType,
			Tag:     "target-map",
			Options: &testLoopbackMapperOptions{State: serverMap},
		}},
		Route: &option.RouteOptions{Final: "target-map"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := box.New(box.Options{
		Context: singbox.RuntimeContextWithOutboundRegistry(ctx, getTestOutboundRegistry()),
		Options: serverOpts,
	})
	if err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("start inbound: %v", err)
	}
	defer srv.Close()

	cfg := matrixNode(tc)
	cfg.Port = 443
	clientBuild := &singbox.BoxBuildConfig{
		Context: func(c context.Context) context.Context {
			return singbox.RuntimeContextWithOutboundRegistry(c, getTestOutboundRegistry())
		},
		Mutate: func(opts *option.Options, tag string) error {
			opts.Certificate = &option.CertificateOptions{
				Store:       C.CertificateStoreNone,
				Certificate: badoption.Listable[string]{certs.CACertPEM},
			}
			if err := injectOutboundDetour(opts, "node-map"); err != nil {
				return err
			}
			opts.Outbounds = append(opts.Outbounds, option.Outbound{
				Type:    testLoopbackMapperType,
				Tag:     "node-map",
				Options: &testLoopbackMapperOptions{State: clientMap},
			})
			return nil
		},
	}
	rt, err := singbox.NewWithBoxConfig(context.Background(), cfg, clientBuild)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer rt.Close()

	resolver := &staticPublicResolver{
		records: map[string][]net.IPAddr{
			matrixTarget: {{IP: net.ParseIP(matrixTargetIP)}},
		},
	}
	httpClient := rt.HTTPClient(singbox.HTTPClientOptions{
		Resolver: resolver,
		Timeout:  8 * time.Second,
	})

	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, "http://"+matrixTarget+":8080/generate_204", bytes.NewReader(expectedBody))
	if err != nil {
		t.Fatalf("new POST request: %v", err)
	}

	if cancelAfterBody {
		var cancelAt atomic.Int64
		go func() {
			select {
			case <-bodyRecvCh:
				cancelAt.Store(time.Now().UnixNano())
				reqCancel()
			case <-reqCtx.Done():
			}
		}()

		resp, doErr := httpClient.Do(req)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if doErr == nil {
			t.Fatal("expected POST request to fail after context cancellation while waiting for server response")
		}
		if !errors.Is(doErr, context.Canceled) {
			t.Fatalf("expected context.Canceled error, got: %v", doErr)
		}
		cNano := cancelAt.Load()
		if cNano == 0 {
			t.Fatal("request cancelled before target server received full POST body")
		}
		elapsed := time.Since(time.Unix(0, cNano))
		if elapsed > 2*time.Second {
			t.Fatalf("unbounded exit after context cancel: took %v (> 2s)", elapsed)
		}
		if fullBodyRead.Load() != 1 {
			t.Fatalf("expected fullBodyRead=1 before cancel, got %d", fullBodyRead.Load())
		}
		return
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP POST 204 probe failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent || fullBodyRead.Load() != 1 || targetHits.Load() != 1 {
		t.Fatalf("status=%d fullBodyRead=%d targetHits=%d", resp.StatusCode, fullBodyRead.Load(), targetHits.Load())
	}
	gotHost, _ := observedHost.Load().(string)
	if gotHost != net.JoinHostPort(matrixTarget, "8080") {
		t.Fatalf("target Host=%q, want %s:8080", gotHost, matrixTarget)
	}
	if got := clientMap.Unauthorized(); len(got) != 0 {
		t.Fatalf("unauthorized node socket: %+v", got)
	}
	if got := serverMap.Unauthorized(); len(got) != 0 {
		t.Fatalf("unauthorized target socket: %+v", got)
	}
	if got := clientMap.Authorized(); len(got) != 1 || got[0].Destination != nodeAddr {
		t.Fatalf("node socket records=%+v, want exactly %s", got, nodeAddr)
	}
	if got := serverMap.Authorized(); len(got) != 1 || got[0].Destination != targetAddr {
		t.Fatalf("target socket records=%+v, want exactly %s", got, targetAddr)
	}
}
