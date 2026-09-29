package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/probe/singbox"
)

// Sentinel errors for node-level target resolution and SSRF policy rejection.
var (
	ErrTargetUnresolvable    = errors.New("target_unresolvable")
	ErrPrivateTargetRejected = errors.New("private_target_rejected")
)

// SafeNodeDialerOptions provides optional overrides for SafeNodeDialer.
type SafeNodeDialerOptions struct {
	// Resolver allows overriding the DNS resolver used for server verification.
	// If nil, fetch.DefaultPolicy().Resolver or net.DefaultResolver is used.
	Resolver fetch.Resolver
	// HTTPClientOptions configures singbox HTTPClient settings (timeouts, HTTP/2).
	HTTPClientOptions singbox.HTTPClientOptions
	// ClientFactory allows overriding or capturing singbox HTTP client creation.
	// If nil, singbox.NewHTTPClient is used.
	ClientFactory func(ctx context.Context, config singbox.NodeConfig, options singbox.HTTPClientOptions) (*http.Client, func() error, error)
}

// NewSafeNodeDialer returns a NodeDialer closure that reads plaintext credentials directly from domain.Node,
// verifies that the destination server is a public IP, disallows HTTP redirects, and establishes an ephemeral
// sing-box memory client.
//
// Domain destinations are resolved and validated here, then the sing-box server field is pinned
// to a validated IP so protocol outbounds cannot perform a second DNS lookup.
func NewSafeNodeDialer(opts ...SafeNodeDialerOptions) NodeDialer {
	var opt SafeNodeDialerOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	resolver := opt.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	return func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		if node.LogicalID == "" {
			return nil, nil, fmt.Errorf("%w: node logical ID is empty", ErrCredentialsUnavailable)
		}

		serverHost := strings.TrimSpace(node.Server)
		if serverHost == "" {
			return nil, nil, fmt.Errorf("%w: empty server in node", ErrCredentialsUnavailable)
		}
		if node.Port < 1 || node.Port > 65535 {
			return nil, nil, fmt.Errorf("%w: invalid port %d in node", ErrCredentialsUnavailable, node.Port)
		}

		// Reject insecure TLS flags and protocol options that add unverified entry points
		// or weaken the authenticated server identity.
		if domain.HasInsecureTransport(node.Credentials.Transport) {
			return nil, nil, fmt.Errorf("%w: insecure certificate verification requested", ErrCredentialsUnavailable)
		}
		if node.Protocol == domain.ProtocolHysteria2 && domain.ExtractHy2Ports(node.Credentials.Transport) != "" {
			return nil, nil, fmt.Errorf("%w: hysteria2 port hopping is unsupported", ErrCredentialsUnavailable)
		}
		if node.Protocol == domain.ProtocolTUIC && (node.Credentials.DisableSNI || domain.HasTUICDisableSNI(node.Credentials.Transport)) {
			return nil, nil, fmt.Errorf("%w: TUIC SNI disable is unsupported", ErrCredentialsUnavailable)
		}

		// Verify IP is public and pin domain destinations before sing-box dials.
		// Strip brackets for IPv6 literal: "[::1]" -> "::1"
		cleanHost := strings.Trim(serverHost, "[]")
		parsedIP := net.ParseIP(cleanHost)

		policy := fetch.DefaultPolicy()
		var targetServer string
		if parsedIP != nil {
			// Direct IP literal: must be public
			if err := policy.ValidateIP(parsedIP); err != nil {
				return nil, nil, fmt.Errorf("%w: server IP %s is not a public IP: %v", ErrPrivateTargetRejected, parsedIP.String(), err)
			}
			targetServer = cleanHost
		} else {
			// Resolve all IPs using injectable resolver and validate each with fetch.Policy.
			policy.Resolver = resolver
			ips, lookupErr := resolver.LookupIPAddr(ctx, cleanHost)
			if lookupErr != nil || len(ips) == 0 {
				return nil, nil, fmt.Errorf("%w: failed to resolve server domain %s", ErrTargetUnresolvable, cleanHost)
			}
			var chosenIP net.IP
			for _, ipAddr := range ips {
				if err := policy.ValidateIP(ipAddr.IP); err != nil {
					return nil, nil, fmt.Errorf("%w: domain %s resolved to non-public IP %s: %v", ErrPrivateTargetRejected, cleanHost, ipAddr.IP.String(), err)
				}
				if chosenIP == nil {
					chosenIP = ipAddr.IP
				}
			}

			if chosenIP == nil {
				return nil, nil, fmt.Errorf("%w: no valid public IP found for domain %s", ErrTargetUnresolvable, cleanHost)
			}
			targetServer = chosenIP.String()
		}

		// Build ephemeral sing-box config directly from domain.Node plaintext configuration
		norm := parser.NormalizedNode{
			Node:        node,
			Server:      serverHost,
			Port:        node.Port,
			Transport:   node.Credentials.Transport,
			Credentials: node.Credentials,
		}
		payload := &domain.NodeCredentialPayload{
			LogicalID:   node.LogicalID,
			Protocol:    node.Protocol,
			Server:      serverHost,
			Port:        node.Port,
			Credentials: node.Credentials,
		}

		cfg := singbox.NodeConfigFromPayload(norm, payload)
		cfg.LogicalID = node.LogicalID
		cfg.Server = targetServer
		cfg.Port = node.Port

		// Preserve TLS identity: if cfg.SNI is empty, retain original domain as SNI
		usesTLS := cfg.TLS || node.Protocol == domain.ProtocolTrojan || node.Protocol == domain.ProtocolHysteria2 || node.Protocol == domain.ProtocolTUIC
		if parsedIP == nil && usesTLS && cfg.SNI == "" {
			cfg.SNI = cleanHost
		}

		// Fail closed if skip cert verification is requested
		if cfg.SkipCertVerify {
			return nil, nil, fmt.Errorf("%w: strict certificate verification required for safe node dialer", ErrCredentialsUnavailable)
		}

		// Instantiate in-memory singbox client & enforce no-redirect policy
		clientFactory := opt.ClientFactory
		if clientFactory == nil {
			clientFactory = singbox.NewHTTPClient
		}
		httpOpts := opt.HTTPClientOptions
		if httpOpts.Resolver == nil {
			httpOpts.Resolver = resolver
		}
		client, cleanup, err := clientFactory(ctx, cfg, httpOpts)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: failed to create singbox client: %v", ErrCredentialsUnavailable, err)
		}

		// Enforce HTTP redirect prohibition
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		return client, cleanup, nil
	}
}
