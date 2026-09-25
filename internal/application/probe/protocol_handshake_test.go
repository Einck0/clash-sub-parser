package probe_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/probe/singbox"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/tuic"
	"github.com/sagernet/sing/common/json/badoption"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const testAppLoopbackMapperType = "test-app-loopback-mapper"

type appSocketRecord struct {
	Network     string
	Destination string
}

type appLoopbackMapperState struct {
	mu           sync.Mutex
	tcpMap       map[string]string
	udpMap       map[string]string
	authorized   []appSocketRecord
	unauthorized []appSocketRecord
}

func newAppLoopbackMapperState() *appLoopbackMapperState {
	return &appLoopbackMapperState{
		tcpMap: make(map[string]string),
		udpMap: make(map[string]string),
	}
}

func (s *appLoopbackMapperState) MapTCP(publicAddr, localLoopbackAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tcpMap[publicAddr] = localLoopbackAddr
}

func (s *appLoopbackMapperState) MapUDP(publicAddr, localLoopbackAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.udpMap[publicAddr] = localLoopbackAddr
}

func (s *appLoopbackMapperState) Authorized() []appSocketRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]appSocketRecord, len(s.authorized))
	copy(out, s.authorized)
	return out
}

func (s *appLoopbackMapperState) Unauthorized() []appSocketRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]appSocketRecord, len(s.unauthorized))
	copy(out, s.unauthorized)
	return out
}

type testAppLoopbackMapperOptions struct {
	State *appLoopbackMapperState
}

type testAppLoopbackMapperOutbound struct {
	outbound.Adapter
	state *appLoopbackMapperState
}

func (o *testAppLoopbackMapperOutbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	netKind := N.NetworkName(network)
	destStr := destination.String()

	o.state.mu.Lock()
	var mapped string
	var ok bool
	if netKind == N.NetworkUDP {
		mapped, ok = o.state.udpMap[destStr]
	} else {
		mapped, ok = o.state.tcpMap[destStr]
	}
	if ok {
		o.state.authorized = append(o.state.authorized, appSocketRecord{Network: netKind, Destination: destStr})
	} else {
		o.state.unauthorized = append(o.state.unauthorized, appSocketRecord{Network: netKind, Destination: destStr})
	}
	o.state.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("unauthorized app test socket dial: %s -> %s", netKind, destStr)
	}
	var d net.Dialer
	return d.DialContext(ctx, netKind, mapped)
}

func (o *testAppLoopbackMapperOutbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	destStr := destination.String()
	o.state.mu.Lock()
	mapped, ok := o.state.udpMap[destStr]
	if ok {
		o.state.authorized = append(o.state.authorized, appSocketRecord{Network: N.NetworkUDP, Destination: destStr})
	} else {
		o.state.unauthorized = append(o.state.unauthorized, appSocketRecord{Network: N.NetworkUDP, Destination: destStr})
	}
	o.state.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("unauthorized app test packet listen: udp -> %s", destStr)
	}
	mappedAddr := M.ParseSocksaddr(mapped).UDPAddr()
	if mappedAddr == nil {
		return nil, fmt.Errorf("invalid mapped UDP address: %s", mapped)
	}
	var lc net.ListenConfig
	pc, err := lc.ListenPacket(ctx, "udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return &mappedAppUDPConn{
		PacketConn: pc,
		publicDest: destination.UDPAddr(),
		localDest:  mappedAddr,
	}, nil
}

type mappedAppUDPConn struct {
	net.PacketConn
	publicDest *net.UDPAddr
	localDest  *net.UDPAddr
}

func (c *mappedAppUDPConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.PacketConn.WriteTo(p, c.localDest)
}

func (c *mappedAppUDPConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, _, err = c.PacketConn.ReadFrom(p)
	if err != nil {
		return n, nil, err
	}
	return n, c.publicDest, nil
}

var (
	testAppOutboundRegOnce sync.Once
	testAppOutboundReg     *outbound.Registry
)

func getTestAppOutboundRegistry() *outbound.Registry {
	testAppOutboundRegOnce.Do(func() {
		reg := include.OutboundRegistry()
		outbound.Register[testAppLoopbackMapperOptions](reg, testAppLoopbackMapperType, func(
			ctx context.Context,
			router adapter.Router,
			logger log.ContextLogger,
			tag string,
			options testAppLoopbackMapperOptions,
		) (adapter.Outbound, error) {
			if options.State == nil {
				return nil, fmt.Errorf("missing appLoopbackMapperState")
			}
			return &testAppLoopbackMapperOutbound{
				Adapter: outbound.NewAdapter(testAppLoopbackMapperType, tag, []string{N.NetworkTCP, N.NetworkUDP}, nil),
				state:   options.State,
			}, nil
		})
		outbound.Register[option.Hysteria2OutboundOptions](reg, C.TypeHysteria2, hysteria2.NewOutbound)
		outbound.Register[option.TUICOutboundOptions](reg, C.TypeTUIC, tuic.NewOutbound)
		testAppOutboundReg = reg
	})
	return testAppOutboundReg
}

type testAppCertBundle struct {
	CACertPEM     string
	ServerCertPEM string
	ServerKeyPEM  string
}

func generateTestAppCertBundle(t *testing.T, dnsNames ...string) testAppCertBundle {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CSP Hermetic App Root CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	caPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))

	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}
	srvCertPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvDER}))
	srvKeyDER, err := x509.MarshalECPrivateKey(srvKey)
	if err != nil {
		t.Fatalf("marshal server key: %v", err)
	}
	srvKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: srvKeyDER}))

	return testAppCertBundle{
		CACertPEM:     caPEM,
		ServerCertPEM: srvCertPEM,
		ServerKeyPEM:  srvKeyPEM,
	}
}

func allocateFreeTCPPortApp(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free tcp port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

type staticAppResolver struct {
	records map[string][]net.IPAddr
}

func (r *staticAppResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ips, ok := r.records[host]; ok {
		return ips, nil
	}
	return nil, fmt.Errorf("host %q not found in staticAppResolver", host)
}

func TestSafeNodeDialerTrojanRealTLSHandshakeTo204(t *testing.T) {
	const (
		nodeLogicalID    = "node-trojan-safe-dialer-204"
		nodeDomain       = "trojan.safe.example.test"
		pinnedNodeIP     = "93.184.216.34"
		pinnedNodePort   = 443
		trojanPassword   = "trojan-safe-dialer-204-pwd"
		targetDomain     = "cp.cloudflare.com"
		pinnedTargetIP   = "93.184.216.35"
		pinnedTargetPort = 8080
	)

	// Verify public IPs adhere strictly to production policy
	policy := fetch.DefaultPolicy()
	if err := policy.ValidateIP(net.ParseIP(pinnedNodeIP)); err != nil {
		t.Fatalf("pinnedNodeIP failed production policy: %v", err)
	}
	if err := policy.ValidateIP(net.ParseIP(pinnedTargetIP)); err != nil {
		t.Fatalf("pinnedTargetIP failed production policy: %v", err)
	}

	// 1. Local httptest HTTP 204 server
	var targetHits atomic.Int64
	var observedTargetHost atomic.Value
	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/generate_204" {
			targetHits.Add(1)
			observedTargetHost.Store(r.Host)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer targetSrv.Close()
	targetLocalAddr := targetSrv.Listener.Addr().String()

	// 2. Ephemeral CA and server certificate for trojan.safe.example.test
	certs := generateTestAppCertBundle(t, nodeDomain)

	// 3. Start server sing-box instance with real TrojanInboundOptions
	inboundPort := allocateFreeTCPPortApp(t)
	inboundLocalAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(inboundPort))
	listenAddr := badoption.Addr(netip.MustParseAddr("127.0.0.1"))

	serverTargetMapper := newAppLoopbackMapperState()
	expectedTargetAddr := net.JoinHostPort(pinnedTargetIP, strconv.Itoa(pinnedTargetPort))
	serverTargetMapper.MapTCP(expectedTargetAddr, targetLocalAddr)

	serverOptions := option.Options{
		Log: &option.LogOptions{Disabled: true},
		Inbounds: []option.Inbound{
			{
				Type: C.TypeTrojan,
				Tag:  "trojan-in",
				Options: &option.TrojanInboundOptions{
					ListenOptions: option.ListenOptions{
						Listen:     &listenAddr,
						ListenPort: uint16(inboundPort),
					},
					Users: []option.TrojanUser{
						{Name: "app-user", Password: trojanPassword},
					},
					InboundTLSOptionsContainer: option.InboundTLSOptionsContainer{
						TLS: &option.InboundTLSOptions{
							Enabled:     true,
							ServerName:  nodeDomain,
							Certificate: badoption.Listable[string]{certs.ServerCertPEM},
							Key:         badoption.Listable[string]{certs.ServerKeyPEM},
						},
					},
				},
			},
		},
		Outbounds: []option.Outbound{
			{
				Type: testAppLoopbackMapperType,
				Tag:  "server-target-mapper",
				Options: &testAppLoopbackMapperOptions{
					State: serverTargetMapper,
				},
			},
		},
		Route: &option.RouteOptions{
			Final: "server-target-mapper",
		},
	}

	srvCtx, srvCancel := context.WithCancel(context.Background())
	defer srvCancel()
	srvBox, err := box.New(box.Options{
		Context: singbox.RuntimeContextWithOutboundRegistry(srvCtx, getTestAppOutboundRegistry()),
		Options: serverOptions,
	})
	if err != nil {
		t.Fatalf("create server sing-box: %v", err)
	}
	if err := srvBox.Start(); err != nil {
		t.Fatalf("start server sing-box: %v", err)
	}
	defer srvBox.Close()

	// 4. Setup repo and vault with Trojan credentials
	repo := newMemoryCredRepo()
	vault := createTestVault(t)
	ctx := context.Background()

	payload := &domain.NodeCredentialPayload{
		LogicalID: nodeLogicalID,
		Version:   1,
		Protocol:  domain.ProtocolTrojan,
		Server:    nodeDomain,
		Port:      pinnedNodePort,
		Credentials: domain.InboundProtocolCredential{
			Password: trojanPassword,
		},
	}
	rec, err := vault.Encrypt(payload)
	if err != nil {
		t.Fatalf("vault.Encrypt: %v", err)
	}
	if err := repo.Upsert(ctx, rec); err != nil {
		t.Fatalf("repo.Upsert: %v", err)
	}

	// 5. Build SafeNodeDialer with IP pinning, test CA trust, and client loopback mapper
	clientNodeMapper := newAppLoopbackMapperState()
	expectedNodeAddr := net.JoinHostPort(pinnedNodeIP, strconv.Itoa(pinnedNodePort))
	clientNodeMapper.MapTCP(expectedNodeAddr, inboundLocalAddr)

	dialerResolver := &staticAppResolver{
		records: map[string][]net.IPAddr{
			nodeDomain:   {{IP: net.ParseIP(pinnedNodeIP)}},
			targetDomain: {{IP: net.ParseIP(pinnedTargetIP)}},
		},
	}

	clientFactory := func(fCtx context.Context, config singbox.NodeConfig, opts singbox.HTTPClientOptions) (*http.Client, func() error, error) {
		buildCfg := &singbox.BoxBuildConfig{
			Context: func(c context.Context) context.Context {
				return singbox.RuntimeContextWithOutboundRegistry(c, getTestAppOutboundRegistry())
			},
			Mutate: func(options *option.Options, tag string) error {
				options.Certificate = &option.CertificateOptions{
					Store:       C.CertificateStoreNone,
					Certificate: badoption.Listable[string]{certs.CACertPEM},
				}
				trojanOut, ok := options.Outbounds[0].Options.(*option.TrojanOutboundOptions)
				if !ok {
					return fmt.Errorf("expected *option.TrojanOutboundOptions, got %T", options.Outbounds[0].Options)
				}
				if trojanOut.TLS == nil || trojanOut.TLS.Insecure || trojanOut.TLS.ServerName != nodeDomain {
					return fmt.Errorf("unexpected TLS options: %+v", trojanOut.TLS)
				}
				trojanOut.DialerOptions.Detour = "client-node-mapper"
				options.Outbounds = append(options.Outbounds, option.Outbound{
					Type: testAppLoopbackMapperType,
					Tag:  "client-node-mapper",
					Options: &testAppLoopbackMapperOptions{
						State: clientNodeMapper,
					},
				})
				return nil
			},
		}
		rt, err := singbox.NewWithBoxConfig(fCtx, config, buildCfg)
		if err != nil {
			return nil, nil, err
		}
		// Ensure httpClient uses our static resolver to resolve the target destination to pinnedTargetIP
		opts.Resolver = dialerResolver
		return rt.HTTPClient(opts), rt.Close, nil
	}

	dialer := probe.NewSafeNodeDialer(repo, vault, probe.SafeNodeDialerOptions{
		Resolver:      dialerResolver,
		ClientFactory: clientFactory,
	})

	node := domain.Node{
		LogicalID:         nodeLogicalID,
		Protocol:          domain.ProtocolTrojan,
		CredentialVersion: 1,
	}

	httpClient, cleanup, err := dialer(ctx, node)
	if err != nil {
		t.Fatalf("dialer(node) failed: %v", err)
	}
	defer cleanup()

	// 6. Perform HTTP 204 probe
	targetURL := fmt.Sprintf("http://%s:%d/generate_204", targetDomain, pinnedTargetPort)
	resp, err := httpClient.Get(targetURL)
	if err != nil {
		t.Fatalf("httpClient.Get(%q) failed: %v", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected HTTP 204 No Content, got %d", resp.StatusCode)
	}
	if targetHits.Load() != 1 {
		t.Fatalf("expected targetHits=1, got %d", targetHits.Load())
	}
	if gotHost, _ := observedTargetHost.Load().(string); gotHost != net.JoinHostPort(targetDomain, strconv.Itoa(pinnedTargetPort)) {
		t.Fatalf("expected target Host %q, got %q", net.JoinHostPort(targetDomain, strconv.Itoa(pinnedTargetPort)), gotHost)
	}

	// 7. Verify socket pinning and zero leaks
	nodeSockets := clientNodeMapper.Authorized()
	if len(nodeSockets) != 1 || nodeSockets[0].Network != "tcp" || nodeSockets[0].Destination != expectedNodeAddr {
		t.Fatalf("unexpected client node sockets: %+v (want tcp -> %s)", nodeSockets, expectedNodeAddr)
	}
	if len(clientNodeMapper.Unauthorized()) != 0 {
		t.Fatalf("unexpected unauthorized client sockets: %+v", clientNodeMapper.Unauthorized())
	}

	targetSockets := serverTargetMapper.Authorized()
	if len(targetSockets) != 1 || targetSockets[0].Network != "tcp" || targetSockets[0].Destination != expectedTargetAddr {
		t.Fatalf("unexpected server target sockets: %+v (want tcp -> %s)", targetSockets, expectedTargetAddr)
	}
	if len(serverTargetMapper.Unauthorized()) != 0 {
		t.Fatalf("unexpected unauthorized server sockets: %+v", serverTargetMapper.Unauthorized())
	}
}

func allocateFreeUDPPortApp(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free udp port: %v", err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func injectAppOutboundDetour(options *option.Options, detourTag string) error {
	if len(options.Outbounds) == 0 {
		return fmt.Errorf("no outbound configured")
	}
	switch out := options.Outbounds[0].Options.(type) {
	case *option.ShadowsocksOutboundOptions:
		out.DialerOptions.Detour = detourTag
	case *option.TrojanOutboundOptions:
		out.DialerOptions.Detour = detourTag
	case *option.VLESSOutboundOptions:
		out.DialerOptions.Detour = detourTag
	case *option.VMessOutboundOptions:
		out.DialerOptions.Detour = detourTag
	case *option.Hysteria2OutboundOptions:
		out.DialerOptions.Detour = detourTag
	case *option.TUICOutboundOptions:
		out.DialerOptions.Detour = detourTag
	default:
		return fmt.Errorf("unsupported outbound options type for detour: %T", out)
	}
	return nil
}

func executeDefaultRunnerBaseline(
	t *testing.T,
	ctx context.Context,
	node domain.Node,
	dialer probe.NodeDialer,
) (error, []domain.ProbeObservation, domain.ProbeRunState) {
	t.Helper()
	nodesRepo := newMemoryNodes()
	if err := nodesRepo.UpsertBatch(context.Background(), []domain.Node{node}); err != nil {
		t.Fatalf("nodesRepo.UpsertBatch: %v", err)
	}
	obsRepo := newMemoryObservations()
	runsRepo := newMemoryRuns()
	sched, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatalf("queue.NewScheduler: %v", err)
	}
	defer sched.Close()

	runner := probe.NewDefaultRunner(
		nodesRepo,
		obsRepo,
		sched,
		runsRepo,
		probe.WithNodeDialer(dialer),
		probe.WithRunBudget(probe.RunBudget{
			MaxTasks:         16,
			MaxResponseBytes: 1 << 20,
			TaskTimeout:      2 * time.Second,
		}),
	)

	now := time.Now().UTC()
	runObj := &domain.ProbeRun{
		ID:             "run-" + node.LogicalID,
		ActorScope:     "test-actor",
		IdempotencyKey: "idem-" + node.LogicalID,
		State:          domain.ProbeRunStateQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
		DeadlineAt:     now.Add(10 * time.Second),
	}
	if err := runsRepo.Create(context.Background(), runObj); err != nil {
		t.Fatalf("runsRepo.Create: %v", err)
	}

	runErr := runner.Run(ctx, runObj, []string{node.LogicalID}, []domain.ProbeKind{domain.ProbeKindBaseline})
	obsList, _ := obsRepo.ListByRun(context.Background(), runObj.ID)
	persistedRun, _ := runsRepo.GetByID(context.Background(), runObj.ID)
	state := runObj.State
	if persistedRun != nil {
		state = persistedRun.State
	}
	return runErr, obsList, state
}

func TestSafeNodeDialerAndDefaultRunnerSecurityMatrixAndUDPHandshake(t *testing.T) {
	const (
		nodeDomain     = "edge.safe.example.test"
		pinnedNodeIP   = "93.184.216.34"
		targetDomain   = "cp.cloudflare.com"
		pinnedTargetIP = "93.184.216.35"
		serverPassword = "correct-server-secret-pass"
		serverUUID     = "00000000-0000-0000-0000-000000000001"
	)

	type matrixScenario struct {
		name               string
		protocol           domain.Protocol
		udp                bool
		serverSAN          string
		untrustedClientCA  bool
		inboundPassword    string
		inboundUUID        string
		inboundMethod      string
		inboundTransport   string
		inboundHost        string
		inboundPath        string
		credServer         string
		credPassword       string
		credUUID           string
		credMethod         string
		credTransport      map[string]string
		nodeVersion        int
		nodeDNSRecords     []net.IPAddr
		targetDNSRecords   []net.IPAddr
		rebindSecondLookup bool
		targetRedirectTo   string
		cancelRunContext   bool
		wantAvailable      bool
		wantNodeDialed     bool
	}

	scenarios := []matrixScenario{
		// 1. Positive controls (TCP TLS + UDP/QUIC Hy2 & TUIC) through SafeNodeDialer + DefaultRunner -> HTTP 204
		{
			name:          "positive_trojan_tcp_tls_204",
			protocol:      domain.ProtocolTrojan,
			credPassword:  serverPassword,
			credTransport: map[string]string{"tls": "true"},
			wantAvailable: true,
		},
		{
			name:          "positive_hysteria2_udp_quic_pin_204",
			protocol:      domain.ProtocolHysteria2,
			udp:           true,
			credPassword:  serverPassword,
			credTransport: map[string]string{"tls": "true", "network": "udp"},
			wantAvailable: true,
		},
		{
			name:          "positive_tuic_udp_quic_pin_204",
			protocol:      domain.ProtocolTUIC,
			udp:           true,
			credPassword:  serverPassword,
			credUUID:      serverUUID,
			credTransport: map[string]string{"tls": "true", "network": "udp"},
			wantAvailable: true,
		},

		// 2. Real inbound wrong password / UUID / cipher method across all 6 protocols
		{
			name:           "negative_ss_wrong_password",
			protocol:       domain.ProtocolSS,
			inboundMethod:  "aes-128-gcm",
			credPassword:   "wrong-client-secret-pass",
			credMethod:     "aes-128-gcm",
			wantNodeDialed: true,
		},
		{
			name:           "negative_ss_wrong_cipher_method",
			protocol:       domain.ProtocolSS,
			inboundMethod:  "aes-128-gcm",
			credPassword:   serverPassword,
			credMethod:     "chacha20-ietf-poly1305",
			wantNodeDialed: true,
		},
		{
			name:           "negative_trojan_wrong_password",
			protocol:       domain.ProtocolTrojan,
			credPassword:   "wrong-trojan-secret-pass",
			credTransport:  map[string]string{"tls": "true"},
			wantNodeDialed: true,
		},
		{
			name:           "negative_vless_wrong_uuid",
			protocol:       domain.ProtocolVLESS,
			credUUID:       "00000000-0000-0000-0000-000000000099",
			credTransport:  map[string]string{"tls": "true"},
			wantNodeDialed: true,
		},
		{
			name:           "negative_vmess_wrong_uuid",
			protocol:       domain.ProtocolVMess,
			credUUID:       "00000000-0000-0000-0000-000000000099",
			credTransport:  map[string]string{"tls": "true"},
			wantNodeDialed: true,
		},
		{
			name:           "negative_hysteria2_wrong_password",
			protocol:       domain.ProtocolHysteria2,
			udp:            true,
			credPassword:   "wrong-hy2-secret-pass",
			credTransport:  map[string]string{"tls": "true", "network": "udp"},
			wantNodeDialed: true,
		},
		{
			name:           "negative_tuic_wrong_credentials",
			protocol:       domain.ProtocolTUIC,
			udp:            true,
			credPassword:   "wrong-tuic-secret-pass",
			credUUID:       "00000000-0000-0000-0000-000000000099",
			credTransport:  map[string]string{"tls": "true", "network": "udp"},
			wantNodeDialed: true,
		},

		// 3. Real inbound TLS verification negative cases (wrong SAN, untrusted CA, explicit SNI mismatch)
		{
			name:           "negative_tls_wrong_san",
			protocol:       domain.ProtocolTrojan,
			serverSAN:      "wrong-san.example.test",
			credPassword:   serverPassword,
			credTransport:  map[string]string{"tls": "true"},
			wantNodeDialed: true,
		},
		{
			name:              "negative_tls_untrusted_ca",
			protocol:          domain.ProtocolTrojan,
			untrustedClientCA: true,
			credPassword:      serverPassword,
			credTransport:     map[string]string{"tls": "true"},
			wantNodeDialed:    true,
		},
		{
			name:           "negative_tls_explicit_sni_mismatch",
			protocol:       domain.ProtocolTrojan,
			credPassword:   serverPassword,
			credTransport:  map[string]string{"tls": "true", "sni": "mismatch-sni.example.test"},
			wantNodeDialed: true,
		},

		// 4. Transport explicit Host positive controls + Host/path/authority mismatch & gRPC fail-closed
		{
			name:             "positive_vless_ws_explicit_host_204",
			protocol:         domain.ProtocolVLESS,
			inboundTransport: "ws",
			inboundHost:      "custom-ws-host.example.test",
			credUUID:         serverUUID,
			credTransport:    map[string]string{"tls": "true", "network": "ws", "path": "/matrix", "host": "custom-ws-host.example.test"},
			wantAvailable:    true,
		},
		{
			name:             "positive_vless_http2_explicit_host_204",
			protocol:         domain.ProtocolVLESS,
			inboundTransport: "http",
			inboundHost:      "custom-h2-host.example.test",
			credUUID:         serverUUID,
			credTransport:    map[string]string{"tls": "true", "network": "http", "path": "/matrix", "host": "custom-h2-host.example.test"},
			wantAvailable:    true,
		},
		{
			name:             "positive_vless_httpupgrade_explicit_host_204",
			protocol:         domain.ProtocolVLESS,
			inboundTransport: "httpupgrade",
			inboundHost:      "custom-up-host.example.test",
			credUUID:         serverUUID,
			credTransport:    map[string]string{"tls": "true", "network": "httpupgrade", "path": "/matrix", "host": "custom-up-host.example.test"},
			wantAvailable:    true,
		},
		{
			name:             "negative_ws_path_and_host_mismatch",
			protocol:         domain.ProtocolVLESS,
			inboundTransport: "ws",
			inboundHost:      "expected-ws.example.test",
			inboundPath:      "/expected-matrix-ws",
			credUUID:         serverUUID,
			credTransport:    map[string]string{"tls": "true", "network": "ws", "path": "/wrong-matrix-ws", "host": "wrong-ws.example.test"},
			wantNodeDialed:   true,
		},
		{
			name:             "negative_http2_authority_mismatch",
			protocol:         domain.ProtocolVLESS,
			inboundTransport: "http",
			inboundHost:      "expected-h2.example.test",
			credUUID:         serverUUID,
			credTransport:    map[string]string{"tls": "true", "network": "http", "path": "/matrix", "host": "wrong-h2.example.test"},
			wantNodeDialed:   true,
		},
		{
			name:             "negative_httpupgrade_host_mismatch",
			protocol:         domain.ProtocolVLESS,
			inboundTransport: "httpupgrade",
			inboundHost:      "expected-up.example.test",
			credUUID:         serverUUID,
			credTransport:    map[string]string{"tls": "true", "network": "httpupgrade", "path": "/matrix", "host": "wrong-up.example.test"},
			wantNodeDialed:   true,
		},
		{
			name:           "negative_grpc_independent_host_fail_closed",
			protocol:       domain.ProtocolVLESS,
			credUUID:       serverUUID,
			credTransport:  map[string]string{"tls": "true", "network": "grpc", "service_name": "matrix", "sni": nodeDomain, "host": "independent-grpc-host.example.test"},
			wantNodeDialed: false,
		},

		// 5. Node & target SSRF, mixed DNS, dual-lookup DNS rebinding, HTTP 302 redirect
		{
			name:           "negative_node_mixed_public_private_dns",
			protocol:       domain.ProtocolTrojan,
			credPassword:   serverPassword,
			nodeDNSRecords: []net.IPAddr{{IP: net.ParseIP(pinnedNodeIP)}, {IP: net.ParseIP("10.0.0.8")}},
			wantNodeDialed: false,
		},
		{
			name:           "negative_node_private_ip_literal",
			protocol:       domain.ProtocolTrojan,
			credServer:     "192.168.1.25",
			credPassword:   serverPassword,
			wantNodeDialed: false,
		},
		{
			name:           "negative_node_metadata_dns",
			protocol:       domain.ProtocolTrojan,
			credPassword:   serverPassword,
			nodeDNSRecords: []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}},
			wantNodeDialed: false,
		},
		{
			name:           "negative_node_rfc6598_dns",
			protocol:       domain.ProtocolTrojan,
			credPassword:   serverPassword,
			nodeDNSRecords: []net.IPAddr{{IP: net.ParseIP("100.64.0.1")}},
			wantNodeDialed: false,
		},
		{
			name:           "negative_node_ipv4_mapped_ipv6",
			protocol:       domain.ProtocolTrojan,
			credPassword:   serverPassword,
			nodeDNSRecords: []net.IPAddr{{IP: net.ParseIP("::ffff:10.0.0.1")}},
			wantNodeDialed: false,
		},
		{
			name:               "negative_dns_rebinding_second_lookup_private",
			protocol:           domain.ProtocolTrojan,
			credPassword:       serverPassword,
			rebindSecondLookup: true,
			wantNodeDialed:     false,
		},
		{
			name:             "negative_target_private_ip_blocked",
			protocol:         domain.ProtocolTrojan,
			credPassword:     serverPassword,
			credTransport:    map[string]string{"tls": "true"},
			targetDNSRecords: []net.IPAddr{{IP: net.ParseIP("10.0.0.42")}},
			wantNodeDialed:   false,
		},
		{
			name:             "negative_target_302_redirect_to_private_not_followed",
			protocol:         domain.ProtocolTrojan,
			credPassword:     serverPassword,
			credTransport:    map[string]string{"tls": "true"},
			targetRedirectTo: "http://169.254.169.254/latest/meta-data/",
			wantNodeDialed:   true,
		},

		// 6. Hysteria2 / TUIC / TLS dangerous options, revoked version, cancelled context
		{
			name:           "negative_hy2_port_hopping_rejected",
			protocol:       domain.ProtocolHysteria2,
			udp:            true,
			credPassword:   serverPassword,
			credTransport:  map[string]string{"server_ports": "443,8443"},
			wantNodeDialed: false,
		},
		{
			name:           "negative_tuic_disable_sni_rejected",
			protocol:       domain.ProtocolTUIC,
			udp:            true,
			credPassword:   serverPassword,
			credUUID:       serverUUID,
			credTransport:  map[string]string{"disable_sni": "true"},
			wantNodeDialed: false,
		},
		{
			name:           "negative_skip_cert_verify_rejected",
			protocol:       domain.ProtocolTrojan,
			credPassword:   serverPassword,
			credTransport:  map[string]string{"skip_cert_verify": "true"},
			wantNodeDialed: false,
		},
		{
			name:           "negative_revoked_credential_version",
			protocol:       domain.ProtocolTrojan,
			credPassword:   serverPassword,
			nodeVersion:    2,
			wantNodeDialed: false,
		},
		{
			name:             "negative_context_cancelled",
			protocol:         domain.ProtocolTrojan,
			credPassword:     serverPassword,
			cancelRunContext: true,
			wantNodeDialed:   false,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			var (
				targetHits       atomic.Int64
				redirectTrapHits atomic.Int64
				observedHost     atomic.Value
			)

			redirectTrap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectTrapHits.Add(1)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer redirectTrap.Close()

			targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if sc.targetRedirectTo != "" {
					http.Redirect(w, r, sc.targetRedirectTo, http.StatusFound)
					return
				}
				if r.URL.Path == "/generate_204" {
					targetHits.Add(1)
					observedHost.Store(r.Host)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer targetSrv.Close()

			certSAN := nodeDomain
			if sc.serverSAN != "" {
				certSAN = sc.serverSAN
			}
			serverCerts := generateTestAppCertBundle(t, certSAN)
			clientTrustCA := serverCerts.CACertPEM
			if sc.untrustedClientCA {
				otherCA := generateTestAppCertBundle(t, nodeDomain)
				clientTrustCA = otherCA.CACertPEM
			}

			inboundPort := allocateFreeTCPPortApp(t)
			if sc.udp {
				inboundPort = allocateFreeUDPPortApp(t)
			}
			inboundLocalAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(inboundPort))
			listenAddr := badoption.Addr(netip.MustParseAddr("127.0.0.1"))

			serverTargetMapper := newAppLoopbackMapperState()
			expectedTargetAddr := net.JoinHostPort(pinnedTargetIP, "80")
			serverTargetMapper.MapTCP(expectedTargetAddr, targetSrv.Listener.Addr().String())

			inPwd := serverPassword
			if sc.inboundPassword != "" {
				inPwd = sc.inboundPassword
			}
			inUUID := serverUUID
			if sc.inboundUUID != "" {
				inUUID = sc.inboundUUID
			}
			inMethod := "aes-128-gcm"
			if sc.inboundMethod != "" {
				inMethod = sc.inboundMethod
			}

			var v2rayTransport *option.V2RayTransportOptions
			if sc.inboundTransport != "" {
				host := nodeDomain
				if sc.inboundHost != "" {
					host = sc.inboundHost
				}
				path := "/matrix"
				if sc.inboundPath != "" {
					path = sc.inboundPath
				}
				switch sc.inboundTransport {
				case "ws":
					v2rayTransport = &option.V2RayTransportOptions{
						Type: C.V2RayTransportTypeWebsocket,
						WebsocketOptions: option.V2RayWebsocketOptions{
							Path:    path,
							Headers: badoption.HTTPHeader{"Host": badoption.Listable[string]{host}},
						},
					}
				case "http":
					v2rayTransport = &option.V2RayTransportOptions{
						Type: C.V2RayTransportTypeHTTP,
						HTTPOptions: option.V2RayHTTPOptions{
							Host: badoption.Listable[string]{host},
							Path: path,
						},
					}
				case "httpupgrade":
					v2rayTransport = &option.V2RayTransportOptions{
						Type: C.V2RayTransportTypeHTTPUpgrade,
						HTTPUpgradeOptions: option.V2RayHTTPUpgradeOptions{
							Host: host,
							Path: path,
						},
					}
				}
			}

			var inboundType string
			var inOpt any
			tlsContainer := option.InboundTLSOptionsContainer{
				TLS: &option.InboundTLSOptions{
					Enabled:     true,
					ServerName:  certSAN,
					Certificate: badoption.Listable[string]{serverCerts.ServerCertPEM},
					Key:         badoption.Listable[string]{serverCerts.ServerKeyPEM},
				},
			}
			switch sc.protocol {
			case domain.ProtocolSS:
				inboundType = C.TypeShadowsocks
				inOpt = &option.ShadowsocksInboundOptions{
					ListenOptions: option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
					Method:        inMethod,
					Password:      inPwd,
				}
			case domain.ProtocolTrojan:
				inboundType = C.TypeTrojan
				inOpt = &option.TrojanInboundOptions{
					ListenOptions:              option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
					Users:                      []option.TrojanUser{{Name: "u", Password: inPwd}},
					InboundTLSOptionsContainer: tlsContainer,
					Transport:                  v2rayTransport,
				}
			case domain.ProtocolVLESS:
				inboundType = C.TypeVLESS
				inOpt = &option.VLESSInboundOptions{
					ListenOptions:              option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
					Users:                      []option.VLESSUser{{Name: "u", UUID: inUUID}},
					InboundTLSOptionsContainer: tlsContainer,
					Transport:                  v2rayTransport,
				}
			case domain.ProtocolVMess:
				inboundType = C.TypeVMess
				inOpt = &option.VMessInboundOptions{
					ListenOptions:              option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
					Users:                      []option.VMessUser{{Name: "u", UUID: inUUID}},
					InboundTLSOptionsContainer: tlsContainer,
					Transport:                  v2rayTransport,
				}
			case domain.ProtocolHysteria2:
				inboundType = C.TypeHysteria2
				inOpt = &option.Hysteria2InboundOptions{
					ListenOptions:              option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
					Users:                      []option.Hysteria2User{{Name: "u", Password: inPwd}},
					InboundTLSOptionsContainer: tlsContainer,
				}
			case domain.ProtocolTUIC:
				inboundType = C.TypeTUIC
				inOpt = &option.TUICInboundOptions{
					ListenOptions:              option.ListenOptions{Listen: &listenAddr, ListenPort: uint16(inboundPort)},
					Users:                      []option.TUICUser{{Name: "u", UUID: inUUID, Password: inPwd}},
					InboundTLSOptionsContainer: tlsContainer,
				}
			}

			srvOpts := option.Options{
				Log:      &option.LogOptions{Disabled: true},
				Inbounds: []option.Inbound{{Type: inboundType, Tag: "in", Options: inOpt}},
				Outbounds: []option.Outbound{{
					Type:    testAppLoopbackMapperType,
					Tag:     "server-target-mapper",
					Options: &testAppLoopbackMapperOptions{State: serverTargetMapper},
				}},
				Route: &option.RouteOptions{Final: "server-target-mapper"},
			}
			srvCtx, srvCancel := context.WithCancel(context.Background())
			defer srvCancel()
			srvBox, err := box.New(box.Options{
				Context: singbox.RuntimeContextWithOutboundRegistry(srvCtx, getTestAppOutboundRegistry()),
				Options: srvOpts,
			})
			if err != nil {
				t.Fatalf("create server box: %v", err)
			}
			if err := srvBox.Start(); err != nil {
				t.Fatalf("start server box: %v", err)
			}
			defer srvBox.Close()

			nodePort := 443
			if sc.udp {
				nodePort = inboundPort
			}
			expectedNodeAddr := net.JoinHostPort(pinnedNodeIP, strconv.Itoa(nodePort))
			clientNodeMapper := newAppLoopbackMapperState()
			if sc.udp {
				clientNodeMapper.MapUDP(expectedNodeAddr, inboundLocalAddr)
			} else {
				clientNodeMapper.MapTCP(expectedNodeAddr, inboundLocalAddr)
			}

			repo := newMemoryCredRepo()
			vault := createTestVault(t)
			logicalID := "node-" + sc.name
			serverHost := nodeDomain
			if sc.credServer != "" {
				serverHost = sc.credServer
			}

			payload := &domain.NodeCredentialPayload{
				LogicalID: logicalID,
				Version:   1,
				Protocol:  sc.protocol,
				Server:    serverHost,
				Port:      nodePort,
				Credentials: domain.InboundProtocolCredential{
					Password:  sc.credPassword,
					UUID:      sc.credUUID,
					Method:    sc.credMethod,
					Transport: sc.credTransport,
				},
			}
			rec, err := vault.Encrypt(payload)
			if err != nil {
				t.Fatalf("vault.Encrypt: %v", err)
			}
			if err := repo.Upsert(context.Background(), rec); err != nil {
				t.Fatalf("repo.Upsert: %v", err)
			}

			var nodeLookupCalls atomic.Int32
			resolver := staticAppResolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
				if host == nodeDomain {
					callNum := nodeLookupCalls.Add(1)
					if sc.rebindSecondLookup {
						if callNum == 1 {
							return []net.IPAddr{{IP: net.ParseIP(pinnedNodeIP)}}, nil
						}
						return []net.IPAddr{{IP: net.ParseIP("10.0.0.99")}}, nil
					}
					if len(sc.nodeDNSRecords) > 0 {
						return sc.nodeDNSRecords, nil
					}
					return []net.IPAddr{{IP: net.ParseIP(pinnedNodeIP)}}, nil
				}
				if host == targetDomain {
					if len(sc.targetDNSRecords) > 0 {
						return sc.targetDNSRecords, nil
					}
					return []net.IPAddr{{IP: net.ParseIP(pinnedTargetIP)}}, nil
				}
				return nil, fmt.Errorf("unexpected lookup host %q", host)
			})

			clientFactory := func(fCtx context.Context, config singbox.NodeConfig, opts singbox.HTTPClientOptions) (*http.Client, func() error, error) {
				if config.Server != pinnedNodeIP {
					return nil, nil, fmt.Errorf("expected pinned server IP %s, got %s", pinnedNodeIP, config.Server)
				}
				buildCfg := &singbox.BoxBuildConfig{
					Context: func(c context.Context) context.Context {
						return singbox.RuntimeContextWithOutboundRegistry(c, getTestAppOutboundRegistry())
					},
					Mutate: func(options *option.Options, tag string) error {
						options.Certificate = &option.CertificateOptions{
							Store:       C.CertificateStoreNone,
							Certificate: badoption.Listable[string]{clientTrustCA},
						}
						if err := injectAppOutboundDetour(options, "client-node-mapper"); err != nil {
							return err
						}
						options.Outbounds = append(options.Outbounds, option.Outbound{
							Type:    testAppLoopbackMapperType,
							Tag:     "client-node-mapper",
							Options: &testAppLoopbackMapperOptions{State: clientNodeMapper},
						})
						return nil
					},
				}
				rt, err := singbox.NewWithBoxConfig(fCtx, config, buildCfg)
				if err != nil {
					return nil, nil, err
				}
				opts.Resolver = resolver
				opts.Timeout = 1500 * time.Millisecond
				opts.TLSHandshakeTimeout = 1200 * time.Millisecond
				return rt.HTTPClient(opts), rt.Close, nil
			}

			dialer := probe.NewSafeNodeDialer(repo, vault, probe.SafeNodeDialerOptions{
				Resolver:      resolver,
				ClientFactory: clientFactory,
			})

			credVer := 1
			if sc.nodeVersion != 0 {
				credVer = sc.nodeVersion
			}
			node := domain.Node{
				LogicalID:         logicalID,
				DisplayName:       sc.name,
				Protocol:          sc.protocol,
				CredentialVersion: credVer,
				Active:            true,
			}

			if sc.rebindSecondLookup {
				// 1st lookup returns public IP (pinned in NodeConfig; sing-box never re-resolves nodeDomain)
				firstClient, firstCleanup, firstErr := dialer(context.Background(), node)
				if firstErr != nil {
					t.Fatalf("first dial before rebind failed: %v", firstErr)
				}
				_ = firstCleanup()
				_ = firstClient
				if nodeLookupCalls.Load() != 1 {
					t.Fatalf("expected exactly 1 DNS lookup during first dial (pinned IP), got %d", nodeLookupCalls.Load())
				}
			}

			runCtx := context.Background()
			if sc.cancelRunContext {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithCancel(runCtx)
				cancel()
			}

			runErr, observations, runState := executeDefaultRunnerBaseline(t, runCtx, node, dialer)

			if len(clientNodeMapper.Unauthorized()) != 0 {
				t.Fatalf("unauthorized client node sockets: %+v", clientNodeMapper.Unauthorized())
			}
			if len(serverTargetMapper.Unauthorized()) != 0 {
				t.Fatalf("unauthorized server target sockets: %+v", serverTargetMapper.Unauthorized())
			}
			if redirectTrapHits.Load() != 0 {
				t.Fatalf("redirect trap server was hit %d times", redirectTrapHits.Load())
			}

			if sc.wantAvailable {
				if runErr != nil {
					t.Fatalf("expected positive scenario to succeed, got runErr=%v", runErr)
				}
				if len(observations) != 1 || observations[0].Verdict != domain.VerdictAvailable {
					t.Fatalf("expected 1 VerdictAvailable observation, got %+v", observations)
				}
				if targetHits.Load() != 1 {
					t.Fatalf("expected targetHits=1, got %d", targetHits.Load())
				}
				if gotHost, _ := observedHost.Load().(string); gotHost != targetDomain {
					t.Fatalf("expected target Host %q, got %q", targetDomain, gotHost)
				}
				wantNet := "tcp"
				if sc.udp {
					wantNet = "udp"
				}
				nodeSocks := clientNodeMapper.Authorized()
				if len(nodeSocks) != 1 || nodeSocks[0].Network != wantNet || nodeSocks[0].Destination != expectedNodeAddr {
					t.Fatalf("unexpected client node sockets: %+v (want %s -> %s)", nodeSocks, wantNet, expectedNodeAddr)
				}
				targetSocks := serverTargetMapper.Authorized()
				if len(targetSocks) != 1 || targetSocks[0].Network != "tcp" || targetSocks[0].Destination != expectedTargetAddr {
					t.Fatalf("unexpected server target sockets: %+v (want tcp -> %s)", targetSocks, expectedTargetAddr)
				}
				return
			}

			// Negative scenario assertions: DefaultRunner MUST NEVER emit VerdictAvailable
			for _, obs := range observations {
				if obs.Verdict == domain.VerdictAvailable {
					t.Fatalf("SECURITY VIOLATION: negative scenario %q produced VerdictAvailable: %+v", sc.name, obs)
				}
				if sc.credPassword != "" && strings.Contains(obs.RedactedSummary, sc.credPassword) {
					t.Fatalf("secret password leaked in observation summary: %s", obs.RedactedSummary)
				}
				if sc.credUUID != "" && strings.Contains(obs.RedactedSummary, sc.credUUID) {
					t.Fatalf("secret UUID leaked in observation summary: %s", obs.RedactedSummary)
				}
			}
			if runErr != nil {
				if sc.credPassword != "" && strings.Contains(runErr.Error(), sc.credPassword) {
					t.Fatalf("secret password leaked in error: %v", runErr)
				}
				if sc.credUUID != "" && strings.Contains(runErr.Error(), sc.credUUID) {
					t.Fatalf("secret UUID leaked in error: %v", runErr)
				}
			}
			if targetHits.Load() != 0 {
				t.Fatalf("negative scenario %q reached HTTP 204 target (%d hits)", sc.name, targetHits.Load())
			}
			if sc.targetRedirectTo == "" && len(serverTargetMapper.Authorized()) != 0 {
				t.Fatalf("negative scenario %q dialed target socket: %+v", sc.name, serverTargetMapper.Authorized())
			}
			if sc.wantNodeDialed {
				if len(clientNodeMapper.Authorized()) == 0 {
					t.Fatalf("expected negative scenario %q to dial pinned node inbound socket, got 0 sockets", sc.name)
				}
			} else {
				if len(clientNodeMapper.Authorized()) != 0 {
					t.Fatalf("expected negative scenario %q to fail before dialing node socket, got %+v", sc.name, clientNodeMapper.Authorized())
				}
			}
			if sc.cancelRunContext && runState != domain.ProbeRunStateCancelled {
				t.Fatalf("expected cancelled run state, got %s", runState)
			}
		})
	}
}
