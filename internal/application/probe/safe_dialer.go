package probe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/probe/singbox"
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

func isTruthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

func hasInsecureTransport(transport map[string]string) bool {
	if transport == nil {
		return false
	}
	for k, v := range transport {
		kLower := strings.ToLower(strings.TrimSpace(k))
		if strings.Contains(kLower, "insecure") ||
			strings.Contains(kLower, "skip_cert") ||
			strings.Contains(kLower, "skip-cert") ||
			strings.Contains(kLower, "skipcert") {
			if isTruthy(v) {
				return true
			}
		}
	}
	return false
}

// NewSafeNodeDialer returns a NodeDialer closure that resolves credentials from repo and vault,
// enforces identity and server consistency, verifies that the destination server is a public IP,
// disallows HTTP redirects, and establishes an ephemeral sing-box memory client.
//
// Domain destinations are resolved and validated here, then the sing-box server field is pinned
// to a validated IP so protocol outbounds cannot perform a second DNS lookup.
func NewSafeNodeDialer(
	repo domain.NodeCredentialRepository,
	vault *domain.NodeCredentialVault,
	opts ...SafeNodeDialerOptions,
) NodeDialer {
	var opt SafeNodeDialerOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	resolver := opt.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	return func(ctx context.Context, node domain.Node) (*http.Client, func() error, error) {
		// 1. Dependency checks: fail-closed if repo or vault is nil
		if repo == nil || vault == nil {
			return nil, nil, fmt.Errorf("%w: missing credential repository or vault", ErrCredentialsUnavailable)
		}

		if node.LogicalID == "" {
			return nil, nil, fmt.Errorf("%w: node logical ID is empty", ErrCredentialsUnavailable)
		}
		if node.CredentialVersion <= 0 {
			return nil, nil, fmt.Errorf("%w: invalid credential version %d", ErrCredentialsUnavailable, node.CredentialVersion)
		}

		// 2. Fetch credentials for exact logical ID and version
		record, err := repo.GetByLogicalID(ctx, node.LogicalID, node.CredentialVersion)
		if err != nil {
			// Do not leak internal DB errors or secrets
			return nil, nil, fmt.Errorf("%w: failed to retrieve credentials: %v", ErrCredentialsUnavailable, err)
		}
		if record == nil {
			return nil, nil, fmt.Errorf("%w: no credential record found for logical ID %s version %d", ErrCredentialsUnavailable, node.LogicalID, node.CredentialVersion)
		}

		// Verify record logical ID and version binding
		if record.LogicalID != node.LogicalID {
			return nil, nil, fmt.Errorf("%w: record logical ID mismatch", ErrCredentialsUnavailable)
		}
		if record.Version != node.CredentialVersion {
			return nil, nil, fmt.Errorf("%w: record version mismatch", ErrCredentialsUnavailable)
		}

		// 3. Decrypt credentials using authenticated protocol
		payload, err := vault.Decrypt(record, node.Protocol)
		if err != nil {
			// Do not leak secret ciphertext, keys, or plaintext in error message
			return nil, nil, fmt.Errorf("%w: failed to decrypt credentials", ErrCredentialsUnavailable)
		}
		if payload == nil {
			return nil, nil, fmt.Errorf("%w: decrypted payload is nil", ErrCredentialsUnavailable)
		}

		// 4. Strict binding checks: logical ID, version, and protocol must match
		if payload.LogicalID != node.LogicalID {
			return nil, nil, fmt.Errorf("%w: payload logical ID mismatch", ErrCredentialsUnavailable)
		}
		if payload.Version != node.CredentialVersion || payload.Version != record.Version {
			return nil, nil, fmt.Errorf("%w: payload version does not match expected version", ErrCredentialsUnavailable)
		}
		if payload.Protocol != node.Protocol {
			return nil, nil, fmt.Errorf("%w: payload protocol mismatch", ErrCredentialsUnavailable)
		}

		serverHost := strings.TrimSpace(payload.Server)
		if serverHost == "" {
			return nil, nil, fmt.Errorf("%w: empty server in payload", ErrCredentialsUnavailable)
		}
		if payload.Port < 1 || payload.Port > 65535 {
			return nil, nil, fmt.Errorf("%w: invalid port %d in payload", ErrCredentialsUnavailable, payload.Port)
		}

		// Reject insecure TLS flags and protocol options that add unverified entry points
		// or weaken the authenticated server identity.
		if hasInsecureTransport(payload.Credentials.Transport) {
			return nil, nil, fmt.Errorf("%w: insecure certificate verification requested", ErrCredentialsUnavailable)
		}
		if node.Protocol == domain.ProtocolHysteria2 {
			for _, key := range []string{"ports", "server_ports", "hy2_ports"} {
				if strings.TrimSpace(payload.Credentials.Transport[key]) != "" {
					return nil, nil, fmt.Errorf("%w: hysteria2 port hopping is unsupported", ErrCredentialsUnavailable)
				}
			}
		}
		if node.Protocol == domain.ProtocolTUIC {
			for _, key := range []string{"disable_sni", "disable-sni", "tuic_disable_sni"} {
				if isTruthy(payload.Credentials.Transport[key]) {
					return nil, nil, fmt.Errorf("%w: TUIC SNI disable is unsupported", ErrCredentialsUnavailable)
				}
			}
		}

		// 5. Verify IP is public and pin domain destinations before sing-box dials.
		// Strip brackets for IPv6 literal: "[::1]" -> "::1"
		cleanHost := strings.Trim(serverHost, "[]")
		parsedIP := net.ParseIP(cleanHost)

		policy := fetch.DefaultPolicy()
		var targetServer string
		if parsedIP != nil {
			// Direct IP literal: must be public
			if err := policy.ValidateIP(parsedIP); err != nil {
				return nil, nil, fmt.Errorf("%w: server IP %s is not a public IP: %v", ErrCredentialsUnavailable, parsedIP.String(), err)
			}
			targetServer = cleanHost
		} else {
			// Resolve all IPs using injectable resolver and validate each with fetch.Policy.
			policy.Resolver = resolver
			ips, lookupErr := resolver.LookupIPAddr(ctx, cleanHost)
			if lookupErr != nil || len(ips) == 0 {
				return nil, nil, fmt.Errorf("%w: failed to resolve server domain %s", ErrCredentialsUnavailable, cleanHost)
			}
			var chosenIP net.IP
			for _, ipAddr := range ips {
				if err := policy.ValidateIP(ipAddr.IP); err != nil {
					return nil, nil, fmt.Errorf("%w: domain %s resolved to non-public IP %s: %v", ErrCredentialsUnavailable, cleanHost, ipAddr.IP.String(), err)
				}
				if chosenIP == nil {
					chosenIP = ipAddr.IP
				}
			}

			if chosenIP == nil {
				return nil, nil, fmt.Errorf("%w: no valid public IP found for domain %s", ErrCredentialsUnavailable, cleanHost)
			}
			targetServer = chosenIP.String()
		}

		// 6. Build ephemeral sing-box config
		norm := parser.NormalizedNode{
			Node:   node,
			Server: serverHost,
			Port:   payload.Port,
		}
		if payload.Credentials.Transport != nil {
			norm.Transport = payload.Credentials.Transport
		}

		cfg := singbox.NodeConfigFromPayload(norm, payload)
		// Guarantee logical ID and server match
		cfg.LogicalID = node.LogicalID
		// Pin socket dial destination to chosen public IP (or verified literal IP)
		cfg.Server = targetServer
		cfg.Port = payload.Port

		// Preserve TLS identity: if cfg.SNI is empty, retain original domain as SNI
		usesTLS := cfg.TLS || node.Protocol == domain.ProtocolTrojan || node.Protocol == domain.ProtocolHysteria2 || node.Protocol == domain.ProtocolTUIC
		if parsedIP == nil && usesTLS && cfg.SNI == "" {
			cfg.SNI = cleanHost
		}

		// Fail closed if skip cert verification is requested
		if cfg.SkipCertVerify {
			return nil, nil, fmt.Errorf("%w: strict certificate verification required for safe node dialer", ErrCredentialsUnavailable)
		}

		// 7. Instantiate in-memory singbox client & enforce no-redirect policy
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
