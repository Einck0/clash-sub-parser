package dialer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"clash-sub-parser/internal/domain"
	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/x/list"
	"github.com/sagernet/sing/service"
)

var (
	baseContextOnce sync.Once
	baseContext     context.Context
	registryMu      sync.Mutex
)

// getBaseContext returns a sing-box context with shared registries and a headless platform interface.
func getBaseContext() context.Context {
	registryMu.Lock()
	defer registryMu.Unlock()
	baseContextOnce.Do(func() {
		ctx := include.Context(context.Background())
		ctx = service.ContextWith[adapter.PlatformInterface](ctx, &headlessPlatform{})
		baseContext = ctx
	})
	return baseContext
}

type nodeDialer struct {
	box       *box.Box
	outbound  adapter.Outbound
	tag       string
	closed    atomic.Bool
	closeOnce sync.Once
}

// NewDialer creates a pure in-memory Dialer pipeline backed by sing-box outbounds.
func NewDialer(ctx context.Context, node *domain.Node) (Dialer, error) {
	if node == nil {
		return nil, ErrNilNode
	}

	opts, tag, err := BuildOptions(node)
	if err != nil {
		return nil, err
	}

	boxCtx := getBaseContext()
	boxInstance, err := box.New(box.Options{
		Context: boxCtx,
		Options: opts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create sing-box instance: %w", err)
	}

	if err := boxInstance.Start(); err != nil {
		_ = boxInstance.Close()
		return nil, fmt.Errorf("failed to start sing-box instance: %w", err)
	}

	var ob adapter.Outbound
	if o, ok := boxInstance.Outbound().Outbound(tag); ok {
		ob = o
	} else if def := boxInstance.Outbound().Default(); def != nil {
		ob = def
	} else if ep, ok := boxInstance.Endpoint().Get(tag); ok {
		ob = ep
	}

	if ob == nil {
		_ = boxInstance.Close()
		return nil, ErrMissingOutbound
	}

	return &nodeDialer{
		box:      boxInstance,
		outbound: ob,
		tag:      tag,
	}, nil
}

// DialContext connects to destination address through the proxy outbound with context.
func (d *nodeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.closed.Load() {
		return nil, ErrDialerClosed
	}
	dest := M.ParseSocksaddr(addr)
	return d.outbound.DialContext(ctx, network, dest)
}

// Dial connects to destination address through the proxy outbound.
func (d *nodeDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// ListenPacket creates an in-memory packet connection for UDP communications through the outbound.
func (d *nodeDialer) ListenPacket(ctx context.Context, destination string) (net.PacketConn, error) {
	if d.closed.Load() {
		return nil, ErrDialerClosed
	}
	dest := M.ParseSocksaddr(destination)
	return d.outbound.ListenPacket(ctx, dest)
}

// Outbound returns the underlying sing-box adapter.Outbound.
func (d *nodeDialer) Outbound() adapter.Outbound {
	return d.outbound
}

// Close shuts down the sing-box runtime and releases in-memory resources and active connections.
func (d *nodeDialer) Close() error {
	var err error
	d.closeOnce.Do(func() {
		d.closed.Store(true)
		if d.box != nil {
			err = d.box.Close()
		}
	})
	return err
}

// HTTPClient constructs an http.Client strictly bound to this dialer, isolating egress from environment proxies.
func (d *nodeDialer) HTTPClient(opts HTTPClientOptions) *http.Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tlsHandshakeTimeout := opts.TLSHandshakeTimeout
	if tlsHandshakeTimeout <= 0 {
		tlsHandshakeTimeout = 5 * time.Second
	}
	idleConnTimeout := opts.IdleConnTimeout
	if idleConnTimeout <= 0 {
		idleConnTimeout = 30 * time.Second
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           d.DialContext,
			ForceAttemptHTTP2:     opts.ForceAttemptHTTP2,
			MaxIdleConns:          10,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: 1 * time.Second,
			Proxy:                 nil, // Explicitly disable system/environment proxy (HTTP_PROXY / ALL_PROXY)
		},
	}
}

// headlessPlatform implements adapter.PlatformInterface to run sing-box in a headless server environment.
// It bypasses netlink interface change monitors, avoiding background OS thread races during concurrent probes.
type headlessPlatform struct{}

func (p *headlessPlatform) Initialize(networkManager adapter.NetworkManager) error { return nil }
func (p *headlessPlatform) UsePlatformAutoDetectInterfaceControl() bool            { return false }
func (p *headlessPlatform) AutoDetectInterfaceControl(fd int) error                { return nil }
func (p *headlessPlatform) UsePlatformInterface() bool                             { return false }
func (p *headlessPlatform) OpenInterface(options *tun.Options, platformOptions option.TunPlatformOptions) (tun.Tun, error) {
	return nil, os.ErrInvalid
}
func (p *headlessPlatform) ProcessPlatformOptions(options option.TunPlatformOptions) error { return nil }
func (p *headlessPlatform) UsePlatformDefaultInterfaceMonitor() bool                      { return true }
func (p *headlessPlatform) CreateDefaultInterfaceMonitor(logger logger.Logger) tun.DefaultInterfaceMonitor {
	return &headlessInterfaceMonitor{}
}
func (p *headlessPlatform) UsePlatformNetworkInterfaces() bool { return false }
func (p *headlessPlatform) NetworkInterfaces() ([]adapter.NetworkInterface, error) {
	return nil, os.ErrInvalid
}
func (p *headlessPlatform) UnderNetworkExtension() bool                    { return false }
func (p *headlessPlatform) NetworkExtensionIncludeAllNetworks() bool       { return false }
func (p *headlessPlatform) ClearDNSCache()                                 {}
func (p *headlessPlatform) RequestPermissionForWIFIState() error           { return nil }
func (p *headlessPlatform) ReadWIFIState(ctx context.Context) adapter.WIFIState {
	return adapter.WIFIState{}
}
func (p *headlessPlatform) UsePlatformConnectionOwnerFinder() bool { return false }
func (p *headlessPlatform) FindConnectionOwner(request *adapter.FindConnectionOwnerRequest) (*adapter.ConnectionOwner, error) {
	return nil, os.ErrInvalid
}
func (p *headlessPlatform) UsePlatformWIFIMonitor() bool                                   { return false }
func (p *headlessPlatform) UsePlatformNotification() bool                                  { return false }
func (p *headlessPlatform) SendNotification(notification *adapter.Notification) error       { return nil }
func (p *headlessPlatform) CancelNotification(identifier string, typeID int32) error      { return nil }
func (p *headlessPlatform) MyInterfaceAddress() []netip.Addr                               { return nil }
func (p *headlessPlatform) UsePlatformNeighborResolver() bool                              { return false }
func (p *headlessPlatform) StartNeighborMonitor(listener adapter.NeighborUpdateListener) error {
	return os.ErrInvalid
}
func (p *headlessPlatform) CloseNeighborMonitor(listener adapter.NeighborUpdateListener) error {
	return nil
}
func (p *headlessPlatform) UsePlatformShell() bool                                              { return false }
func (p *headlessPlatform) CheckPlatformShell() error                                           { return nil }
func (p *headlessPlatform) OpenShellSession(user *adapter.PlatformUser, command string, env []string, term string, rows int32, cols int32) (adapter.ShellSession, error) {
	return nil, os.ErrInvalid
}
func (p *headlessPlatform) LookupUser(username string) (*adapter.PlatformUser, error) {
	return nil, os.ErrInvalid
}
func (p *headlessPlatform) LookupSFTPServer() (string, error) {
	return "", os.ErrInvalid
}
func (p *headlessPlatform) ReadSystemSSHHostKey() ([]byte, error) {
	return nil, os.ErrInvalid
}
func (p *headlessPlatform) TailscaleHostname() string {
	return ""
}
func (p *headlessPlatform) UsePlatformBridge() bool {
	return false
}
func (p *headlessPlatform) CreateBridge(options adapter.BridgeOptions) (adapter.BridgeSession, error) {
	return nil, os.ErrInvalid
}

type headlessInterfaceMonitor struct{}

func (m *headlessInterfaceMonitor) Start() error                                { return nil }
func (m *headlessInterfaceMonitor) Close() error                                { return nil }
func (m *headlessInterfaceMonitor) DefaultInterface() *control.Interface        { return nil }
func (m *headlessInterfaceMonitor) OverrideAndroidVPN() bool                    { return false }
func (m *headlessInterfaceMonitor) AndroidVPNEnabled() bool                     { return false }
func (m *headlessInterfaceMonitor) RegisterCallback(callback tun.DefaultInterfaceUpdateCallback) *list.Element[tun.DefaultInterfaceUpdateCallback] {
	return nil
}
func (m *headlessInterfaceMonitor) UnregisterCallback(element *list.Element[tun.DefaultInterfaceUpdateCallback]) {
}
func (m *headlessInterfaceMonitor) RegisterMyInterface(interfaceName string) {}
func (m *headlessInterfaceMonitor) MyInterfaces() []string                  { return nil }
