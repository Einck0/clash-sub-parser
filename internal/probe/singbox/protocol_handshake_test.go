package singbox_test

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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
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

const testLoopbackMapperType = "test-loopback-mapper"

type socketRecord struct {
	Network     string
	Destination string
}

type loopbackMapperState struct {
	mu           sync.Mutex
	tcpMap       map[string]string
	udpMap       map[string]string
	authorized   []socketRecord
	unauthorized []socketRecord
}

func newLoopbackMapperState() *loopbackMapperState {
	return &loopbackMapperState{
		tcpMap: make(map[string]string),
		udpMap: make(map[string]string),
	}
}

func (s *loopbackMapperState) MapTCP(publicAddr, localLoopbackAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tcpMap[publicAddr] = localLoopbackAddr
}

func (s *loopbackMapperState) MapUDP(publicAddr, localLoopbackAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.udpMap[publicAddr] = localLoopbackAddr
}

func (s *loopbackMapperState) Authorized() []socketRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]socketRecord, len(s.authorized))
	copy(out, s.authorized)
	return out
}

func (s *loopbackMapperState) Unauthorized() []socketRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]socketRecord, len(s.unauthorized))
	copy(out, s.unauthorized)
	return out
}

type testLoopbackMapperOptions struct {
	State *loopbackMapperState
}

type testLoopbackMapperOutbound struct {
	outbound.Adapter
	state *loopbackMapperState
}

func (o *testLoopbackMapperOutbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
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
		o.state.authorized = append(o.state.authorized, socketRecord{Network: netKind, Destination: destStr})
	} else {
		o.state.unauthorized = append(o.state.unauthorized, socketRecord{Network: netKind, Destination: destStr})
	}
	o.state.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("unauthorized test socket dial: %s -> %s", netKind, destStr)
	}
	var d net.Dialer
	return d.DialContext(ctx, netKind, mapped)
}

func (o *testLoopbackMapperOutbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	destStr := destination.String()
	o.state.mu.Lock()
	mapped, ok := o.state.udpMap[destStr]
	if ok {
		o.state.authorized = append(o.state.authorized, socketRecord{Network: N.NetworkUDP, Destination: destStr})
	} else {
		o.state.unauthorized = append(o.state.unauthorized, socketRecord{Network: N.NetworkUDP, Destination: destStr})
	}
	o.state.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("unauthorized test packet listen: udp -> %s", destStr)
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
	return &mappedUDPConn{
		PacketConn: pc,
		publicDest: destination.UDPAddr(),
		localDest:  mappedAddr,
	}, nil
}

type mappedUDPConn struct {
	net.PacketConn
	publicDest *net.UDPAddr
	localDest  *net.UDPAddr
}

func (c *mappedUDPConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.PacketConn.WriteTo(p, c.localDest)
}

func (c *mappedUDPConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, _, err = c.PacketConn.ReadFrom(p)
	if err != nil {
		return n, nil, err
	}
	return n, c.publicDest, nil
}

var (
	testOutboundRegOnce sync.Once
	testOutboundReg     *outbound.Registry
)

func getTestOutboundRegistry() *outbound.Registry {
	testOutboundRegOnce.Do(func() {
		reg := include.OutboundRegistry()
		outbound.Register[testLoopbackMapperOptions](reg, testLoopbackMapperType, func(
			ctx context.Context,
			router adapter.Router,
			logger log.ContextLogger,
			tag string,
			options testLoopbackMapperOptions,
		) (adapter.Outbound, error) {
			if options.State == nil {
				return nil, fmt.Errorf("missing loopbackMapperState")
			}
			return &testLoopbackMapperOutbound{
				Adapter: outbound.NewAdapter(testLoopbackMapperType, tag, []string{N.NetworkTCP, N.NetworkUDP}, nil),
				state:   options.State,
			}, nil
		})
		outbound.Register[option.Hysteria2OutboundOptions](reg, C.TypeHysteria2, hysteria2.NewOutbound)
		outbound.Register[option.TUICOutboundOptions](reg, C.TypeTUIC, tuic.NewOutbound)
		testOutboundReg = reg
	})
	return testOutboundReg
}

type testCertBundle struct {
	CACertPEM     string
	ServerCertPEM string
	ServerKeyPEM  string
}

func generateTestCertBundle(t *testing.T, dnsNames ...string) *testCertBundle {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CSP Hermetic Test Root CA"},
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

	return &testCertBundle{
		CACertPEM:     caPEM,
		ServerCertPEM: srvCertPEM,
		ServerKeyPEM:  srvKeyPEM,
	}
}

func allocateFreeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free tcp port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func allocateFreeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free udp port: %v", err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func injectOutboundDetour(options *option.Options, detourTag string) error {
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

type staticPublicResolver struct {
	records map[string][]net.IPAddr
}

func (r *staticPublicResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ips, ok := r.records[host]; ok {
		return ips, nil
	}
	return nil, fmt.Errorf("host %q not found in staticPublicResolver", host)
}

func TestTrojanRealTLSHandshakeAnd204(t *testing.T) {
	const (
		nodeDomain       = "trojan.edge.example.test"
		pinnedNodeIP     = "93.184.216.34"
		pinnedNodePort   = 443
		trojanPassword   = "trojan-secret-204-pass"
		targetDomain     = "cp.cloudflare.com"
		pinnedTargetIP   = "93.184.216.35"
		pinnedTargetPort = 8080
	)

	// Verify both pinned node IP and target IP pass production fetch.DefaultPolicy() without any private exemption
	policy := fetch.DefaultPolicy()
	if err := policy.ValidateIP(net.ParseIP(pinnedNodeIP)); err != nil {
		t.Fatalf("pinnedNodeIP failed production policy: %v", err)
	}
	if err := policy.ValidateIP(net.ParseIP(pinnedTargetIP)); err != nil {
		t.Fatalf("pinnedTargetIP failed production policy: %v", err)
	}

	// 1. Start local httptest HTTP 204 target server
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

	// 2. Generate ephemeral CA & server SAN cert for trojan.edge.example.test
	certs := generateTestCertBundle(t, nodeDomain)

	// 3. Start server-side sing-box with real TrojanInboundOptions + TLS
	inboundPort := allocateFreeTCPPort(t)
	inboundLocalAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(inboundPort))
	listenAddr := badoption.Addr(netip.MustParseAddr("127.0.0.1"))

	serverTargetMapper := newLoopbackMapperState()
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
						{Name: "test-user", Password: trojanPassword},
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
				Type: testLoopbackMapperType,
				Tag:  "server-target-mapper",
				Options: &testLoopbackMapperOptions{
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
		Context: singbox.RuntimeContextWithOutboundRegistry(srvCtx, getTestOutboundRegistry()),
		Options: serverOptions,
	})
	if err != nil {
		t.Fatalf("create server sing-box: %v", err)
	}
	if err := srvBox.Start(); err != nil {
		t.Fatalf("start server sing-box: %v", err)
	}
	defer srvBox.Close()

	// 4. Configure client-side sing-box runtime with pinned public IP, preserved SNI, Insecure:false, and Detour
	clientNodeMapper := newLoopbackMapperState()
	expectedNodeAddr := net.JoinHostPort(pinnedNodeIP, strconv.Itoa(pinnedNodePort))
	clientNodeMapper.MapTCP(expectedNodeAddr, inboundLocalAddr)

	nodeCfg := singbox.NodeConfig{
		LogicalID: "trojan-real-204",
		Protocol:  domain.ProtocolTrojan,
		Server:    pinnedNodeIP,
		Port:      pinnedNodePort,
		Password:  trojanPassword,
		TLS:       true,
		SNI:       nodeDomain,
	}

	buildCfg := &singbox.BoxBuildConfig{
		Context: func(ctx context.Context) context.Context {
			return singbox.RuntimeContextWithOutboundRegistry(ctx, getTestOutboundRegistry())
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
				Type: testLoopbackMapperType,
				Tag:  "client-node-mapper",
				Options: &testLoopbackMapperOptions{
					State: clientNodeMapper,
				},
			})
			return nil
		},
	}

	rt, err := singbox.NewWithBoxConfig(context.Background(), nodeCfg, buildCfg)
	if err != nil {
		t.Fatalf("NewWithBoxConfig: %v", err)
	}
	defer rt.Close()

	resolver := &staticPublicResolver{
		records: map[string][]net.IPAddr{
			targetDomain: {{IP: net.ParseIP(pinnedTargetIP)}},
		},
	}
	httpClient := rt.HTTPClient(singbox.HTTPClientOptions{
		Resolver: resolver,
		Timeout:  5 * time.Second,
	})

	// 5. Perform real HTTP 204 probe through client sing-box -> TLS Trojan inbound -> target httptest
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

	// 6. Assert socket captures on both node-mapper and target-mapper
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
