package singbox

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/badhttp"
	"github.com/sagernet/sing-box/common/listener"
	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/trojan"
	"github.com/sagernet/sing-box/protocol/tuic"
	"github.com/sagernet/sing-box/protocol/vless"
	"github.com/sagernet/sing-box/protocol/vmess"
	"github.com/sagernet/sing-box/transport/v2rayhttp"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/json/badoption"
	"github.com/sagernet/sing/common/logger"
	N "github.com/sagernet/sing/common/network"
	aTLS "github.com/sagernet/sing/common/tls"
	"github.com/sagernet/sing/common/x/list"
	"github.com/sagernet/sing/service"
)

var (
	registryOnce sync.Once
	inboundReg   adapter.InboundRegistry
	outboundReg  adapter.OutboundRegistry
	endpointReg  adapter.EndpointRegistry
	dnsReg       adapter.DNSTransportRegistry
	serviceReg   adapter.ServiceRegistry
	certReg      adapter.CertificateProviderRegistry
)

func initRegistries() {
	registryOnce.Do(func() {
		inReg := include.InboundRegistry()
		hysteria2.RegisterInbound(inReg)
		tuic.RegisterInbound(inReg)
		registerSafeHTTPUpgradeInbounds(inReg)
		inboundReg = inReg

		outReg := include.OutboundRegistry()
		hysteria2.RegisterOutbound(outReg)
		tuic.RegisterOutbound(outReg)
		outboundReg = outReg

		endpointReg = include.EndpointRegistry()
		dnsReg = include.DNSTransportRegistry()
		serviceReg = include.ServiceRegistry()
		certReg = include.CertificateProviderRegistry()
	})
}

// runtimeContext attaches sing-box registries and a headless platform interface to ctx.
// Registry creation is serialized via sync.Once to avoid data races in sing-box's package-level QUIC stubs.
func runtimeContext(ctx context.Context) context.Context {
	return RuntimeContextWithOutboundRegistry(ctx, nil)
}

// RuntimeContextWithOutboundRegistry attaches standard sing-box registries and the headless platform
// interface to ctx, optionally overriding the OutboundRegistry for isolated test fixtures.
func RuntimeContextWithOutboundRegistry(ctx context.Context, customOutboundReg adapter.OutboundRegistry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	initRegistries()
	oReg := outboundReg
	if customOutboundReg != nil {
		oReg = customOutboundReg
	}
	ctx = box.Context(ctx, inboundReg, oReg, endpointReg, dnsReg, serviceReg, certReg)
	ctx = service.ContextWith[adapter.PlatformInterface](ctx, &headlessPlatform{})
	return ctx
}

// headlessPlatform implements adapter.PlatformInterface to run sing-box in headless mode.
// This prevents background OS netlink monitors and thread races during concurrent probes.
type headlessPlatform struct{}

func (*headlessPlatform) Initialize(adapter.NetworkManager) error     { return nil }
func (*headlessPlatform) UsePlatformAutoDetectInterfaceControl() bool { return false }
func (*headlessPlatform) AutoDetectInterfaceControl(int) error        { return nil }
func (*headlessPlatform) UsePlatformInterface() bool                  { return false }
func (*headlessPlatform) OpenInterface(*tun.Options, option.TunPlatformOptions) (tun.Tun, error) {
	return nil, os.ErrInvalid
}
func (*headlessPlatform) ProcessPlatformOptions(option.TunPlatformOptions) error { return nil }
func (*headlessPlatform) UsePlatformDefaultInterfaceMonitor() bool               { return true }
func (*headlessPlatform) CreateDefaultInterfaceMonitor(logger.Logger) tun.DefaultInterfaceMonitor {
	return &headlessMonitor{}
}
func (*headlessPlatform) UsePlatformNetworkInterfaces() bool { return false }
func (*headlessPlatform) NetworkInterfaces() ([]adapter.NetworkInterface, error) {
	return nil, os.ErrInvalid
}
func (*headlessPlatform) UnderNetworkExtension() bool                     { return false }
func (*headlessPlatform) NetworkExtensionIncludeAllNetworks() bool        { return false }
func (*headlessPlatform) ClearDNSCache()                                  {}
func (*headlessPlatform) RequestPermissionForWIFIState() error            { return nil }
func (*headlessPlatform) ReadWIFIState(context.Context) adapter.WIFIState { return adapter.WIFIState{} }
func (*headlessPlatform) UsePlatformConnectionOwnerFinder() bool          { return false }
func (*headlessPlatform) FindConnectionOwner(*adapter.FindConnectionOwnerRequest) (*adapter.ConnectionOwner, error) {
	return nil, os.ErrInvalid
}
func (*headlessPlatform) UsePlatformWIFIMonitor() bool                 { return false }
func (*headlessPlatform) UsePlatformNotification() bool                { return false }
func (*headlessPlatform) SendNotification(*adapter.Notification) error { return nil }
func (*headlessPlatform) CancelNotification(string, int32) error       { return nil }
func (*headlessPlatform) MyInterfaceAddress() []netip.Addr             { return nil }
func (*headlessPlatform) UsePlatformNeighborResolver() bool            { return false }
func (*headlessPlatform) StartNeighborMonitor(adapter.NeighborUpdateListener) error {
	return os.ErrInvalid
}
func (*headlessPlatform) CloseNeighborMonitor(adapter.NeighborUpdateListener) error { return nil }
func (*headlessPlatform) UsePlatformShell() bool                                    { return false }
func (*headlessPlatform) CheckPlatformShell() error                                 { return nil }
func (*headlessPlatform) OpenShellSession(*adapter.PlatformUser, string, []string, string, int32, int32) (adapter.ShellSession, error) {
	return nil, os.ErrInvalid
}
func (*headlessPlatform) LookupUser(string) (*adapter.PlatformUser, error) { return nil, os.ErrInvalid }
func (*headlessPlatform) LookupSFTPServer() (string, error)                { return "", os.ErrInvalid }
func (*headlessPlatform) ReadSystemSSHHostKey() ([]byte, error)            { return nil, os.ErrInvalid }
func (*headlessPlatform) TailscaleHostname() string                        { return "" }
func (*headlessPlatform) UsePlatformBridge() bool                          { return false }
func (*headlessPlatform) CreateBridge(adapter.BridgeOptions) (adapter.BridgeSession, error) {
	return nil, os.ErrInvalid
}
func (*headlessPlatform) UsePlatformAutoRedirect() bool { return false }
func (*headlessPlatform) CreateAutoRedirect(adapter.AutoRedirectOptions) (adapter.AutoRedirectSession, error) {
	return nil, os.ErrInvalid
}

// headlessMonitor implements tun.DefaultInterfaceMonitor without spawning OS netlink watchers.
type headlessMonitor struct{}

func (*headlessMonitor) Start() error                         { return nil }
func (*headlessMonitor) Close() error                         { return nil }
func (*headlessMonitor) DefaultInterface() *control.Interface { return nil }
func (*headlessMonitor) OverrideAndroidVPN() bool             { return false }
func (*headlessMonitor) AndroidVPNEnabled() bool              { return false }
func (*headlessMonitor) RegisterCallback(tun.DefaultInterfaceUpdateCallback) *list.Element[tun.DefaultInterfaceUpdateCallback] {
	return nil
}
func (*headlessMonitor) UnregisterCallback(*list.Element[tun.DefaultInterfaceUpdateCallback]) {}
func (*headlessMonitor) RegisterMyInterface(string)                                           {}
func (*headlessMonitor) MyInterfaces() []string                                               { return nil }

// registerSafeHTTPUpgradeInbounds wraps Trojan/VLESS/VMess inbound constructors when transport=="httpupgrade"
// so that the HTTP 101 Switching Protocols server hijacks net.Conn BEFORE flushing 101 headers.
// In upstream sing-box v2rayhttpupgrade/server.go, WriteHeader(101)+FlushError() runs before hijacker.Hijack(),
// allowing net/http's connReader.backgroundRead to consume the first byte of the post-upgrade frame into
// hijacker's bufio.Reader (which upstream discards), causing intermittent EOF on loopback.
func registerSafeHTTPUpgradeInbounds(reg *inbound.Registry) {
	inbound.Register[option.TrojanInboundOptions](reg, C.TypeTrojan, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TrojanInboundOptions) (adapter.Inbound, error) {
		if options.Transport != nil && options.Transport.Type == C.V2RayTransportTypeHTTPUpgrade {
			innerOpts := options
			innerOpts.TLS = nil
			innerOpts.Transport = nil
			loopback := badoption.Addr(netip.MustParseAddr("127.0.0.1"))
			innerOpts.ListenOptions = option.ListenOptions{Listen: &loopback, ListenPort: 0}
			inner, err := trojan.NewInbound(ctx, router, logger, tag, innerOpts)
			if err != nil {
				return nil, err
			}
			return newSafeHTTPUpgradeInbound(ctx, logger, inner, options.ListenOptions, options.TLS, options.Transport.HTTPUpgradeOptions)
		}
		return trojan.NewInbound(ctx, router, logger, tag, options)
	})
	inbound.Register[option.VLESSInboundOptions](reg, C.TypeVLESS, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VLESSInboundOptions) (adapter.Inbound, error) {
		if options.Transport != nil && options.Transport.Type == C.V2RayTransportTypeHTTPUpgrade {
			innerOpts := options
			innerOpts.TLS = nil
			innerOpts.Transport = nil
			loopback := badoption.Addr(netip.MustParseAddr("127.0.0.1"))
			innerOpts.ListenOptions = option.ListenOptions{Listen: &loopback, ListenPort: 0}
			inner, err := vless.NewInbound(ctx, router, logger, tag, innerOpts)
			if err != nil {
				return nil, err
			}
			return newSafeHTTPUpgradeInbound(ctx, logger, inner, options.ListenOptions, options.TLS, options.Transport.HTTPUpgradeOptions)
		}
		return vless.NewInbound(ctx, router, logger, tag, options)
	})
	inbound.Register[option.VMessInboundOptions](reg, C.TypeVMess, func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VMessInboundOptions) (adapter.Inbound, error) {
		if options.Transport != nil && options.Transport.Type == C.V2RayTransportTypeHTTPUpgrade {
			innerOpts := options
			innerOpts.TLS = nil
			innerOpts.Transport = nil
			loopback := badoption.Addr(netip.MustParseAddr("127.0.0.1"))
			innerOpts.ListenOptions = option.ListenOptions{Listen: &loopback, ListenPort: 0}
			inner, err := vmess.NewInbound(ctx, router, logger, tag, innerOpts)
			if err != nil {
				return nil, err
			}
			return newSafeHTTPUpgradeInbound(ctx, logger, inner, options.ListenOptions, options.TLS, options.Transport.HTTPUpgradeOptions)
		}
		return vmess.NewInbound(ctx, router, logger, tag, options)
	})
}

type safeHTTPUpgradeInbound struct {
	adapter.Inbound
	injectable adapter.TCPInjectableInbound
	listener   *listener.Listener
	tlsConfig  tls.ServerConfig
	httpServer *http.Server
	host       string
	path       string
}

func newSafeHTTPUpgradeInbound(
	ctx context.Context,
	logger log.ContextLogger,
	inner adapter.Inbound,
	listenOpts option.ListenOptions,
	tlsOpts *option.InboundTLSOptions,
	upgradeOpts option.V2RayHTTPUpgradeOptions,
) (adapter.Inbound, error) {
	injectable, ok := inner.(adapter.TCPInjectableInbound)
	if !ok {
		return nil, os.ErrInvalid
	}
	var tlsConfig tls.ServerConfig
	if tlsOpts != nil {
		var err error
		tlsConfig, err = tls.NewServerWithOptions(tls.ServerOptions{
			Context: ctx,
			Logger:  logger,
			Options: common.PtrValueOrDefault(tlsOpts),
		})
		if err != nil {
			return nil, err
		}
	}
	path := upgradeOpts.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	w := &safeHTTPUpgradeInbound{
		Inbound:    inner,
		injectable: injectable,
		tlsConfig:  tlsConfig,
		host:       upgradeOpts.Host,
		path:       path,
	}
	w.httpServer = &http.Server{
		Handler:           w,
		ReadHeaderTimeout: C.TCPTimeout,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return log.ContextWithNewID(ctx)
		},
		TLSNextProto: make(map[string]func(*http.Server, *tls.STDConn, http.Handler)),
	}
	w.listener = listener.New(listener.Options{
		Context: ctx,
		Logger:  logger,
		Network: []string{N.NetworkTCP},
		Listen:  listenOpts,
	})
	return w, nil
}

func (w *safeHTTPUpgradeInbound) Start(stage adapter.StartStage) error {
	if err := w.Inbound.Start(stage); err != nil {
		return err
	}
	if stage != adapter.StartStateStart {
		return nil
	}
	if w.tlsConfig != nil {
		if err := w.tlsConfig.Start(); err != nil {
			return err
		}
	}
	tcpListener, err := w.listener.ListenTCP()
	if err != nil {
		return err
	}
	if w.tlsConfig != nil {
		if len(w.tlsConfig.NextProtos()) == 0 {
			w.tlsConfig.SetNextProtos([]string{"http/1.1"})
		}
		tcpListener = aTLS.NewListener(tcpListener, w.tlsConfig)
	}
	go func() {
		_ = w.httpServer.Serve(tcpListener)
	}()
	return nil
}

func (w *safeHTTPUpgradeInbound) Close() error {
	return common.Close(
		w.httpServer,
		w.listener,
		w.tlsConfig,
		w.Inbound,
	)
}

func (w *safeHTTPUpgradeInbound) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if len(w.host) > 0 && request.Host != w.host {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	if request.URL.Path != w.path ||
		request.Method != http.MethodGet ||
		!strings.EqualFold(request.Header.Get("Connection"), "upgrade") ||
		!strings.EqualFold(request.Header.Get("Upgrade"), "websocket") ||
		request.Header.Get("Sec-WebSocket-Key") != "" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	hijacker, canHijack := writer.(http.Hijacker)
	if !canHijack {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	// Hijack BEFORE writing 101 Switching Protocols so net/http's backgroundRead is aborted
	// before the client sends the first post-upgrade protocol frame.
	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	const switchingProtocolsResp = "HTTP/1.1 101 Switching Protocols\r\nConnection: upgrade\r\nUpgrade: websocket\r\n\r\n"
	if _, err := bufrw.WriteString(switchingProtocolsResp); err != nil {
		_ = conn.Close()
		return
	}
	if err := bufrw.Flush(); err != nil {
		_ = conn.Close()
		return
	}
	if buffered := bufrw.Reader.Buffered(); buffered > 0 {
		buffer := buf.NewSize(buffered)
		if _, err := buffer.ReadFullFrom(bufrw.Reader, buffered); err != nil {
			buffer.Release()
			_ = conn.Close()
			return
		}
		conn = bufio.NewCachedConn(conn, buffer)
	}
	var metadata adapter.InboundContext
	metadata.Source = badhttp.SourceAddress(request)
	w.injectable.NewConnection(v2rayhttp.DupContext(request.Context()), conn, metadata, nil)
}
