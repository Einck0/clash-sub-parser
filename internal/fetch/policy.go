// Package fetch provides secure HTTP fetching for subscription sources with strict SSRF defense.
package fetch

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"clash-sub-parser/internal/domain"
)

var defaultBlockedCIDRs = []string{
	// IPv4 Special-Purpose and Private Ranges
	"0.0.0.0/8",          // Current network (RFC 1122)
	"10.0.0.0/8",         // Private network (RFC 1918)
	"100.64.0.0/10",      // Shared address space / CGNAT (RFC 6598)
	"127.0.0.0/8",        // Loopback (RFC 1122)
	"169.254.0.0/16",     // Link-local (RFC 3927)
	"172.16.0.0/12",      // Private network (RFC 1918)
	"192.0.0.0/24",       // IETF protocol assignments (RFC 6890)
	"192.0.2.0/24",       // TEST-NET-1 (RFC 5737)
	"192.88.99.0/24",     // 6to4 relay anycast (RFC 7526)
	"192.168.0.0/16",     // Private network (RFC 1918)
	"198.18.0.0/15",      // Network benchmark tests (RFC 2544)
	"198.51.100.0/24",    // TEST-NET-2 (RFC 5737)
	"203.0.113.0/24",     // TEST-NET-3 (RFC 5737)
	"224.0.0.0/4",        // Multicast (RFC 5771)
	"240.0.0.0/4",        // Reserved for future use (RFC 1112)
	"255.255.255.255/32", // Limited broadcast (RFC 919)

	// IPv6 Special-Purpose and Private Ranges
	"::/128",        // Unspecified (RFC 4291)
	"::1/128",       // Loopback (RFC 4291)
	"::ffff:0:0/96", // IPv4-mapped addresses (RFC 4291)
	"64:ff9b::/96",  // IPv4/IPv6 translation (RFC 6052)
	"100::/64",      // Discard prefix (RFC 6666)
	"2001::/23",     // IETF protocol assignments (RFC 2928)
	"2001:db8::/32", // Documentation (RFC 3849)
	"2002::/16",     // 6to4 (RFC 3056)
	"fc00::/7",      // Unique local address / ULA (RFC 4193)
	"fe80::/10",     // Link-local unicast (RFC 4291)
	"ff00::/8",      // Multicast (RFC 4291)
}

// Resolver defines the DNS lookup interface.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Policy defines the SSRF validation rules and blocked address spaces.
type Policy struct {
	AllowedSchemes    []string
	MaxRedirects      int
	BlockedCIDRs      []*net.IPNet
	AllowedHosts      []string
	AllowedProxyHosts []string
	AllowPrivate      bool
	Resolver          Resolver
}

// DefaultPolicy returns a production-grade SSRF policy with strict private and loopback filtering.
func DefaultPolicy() *Policy {
	blockedNets := make([]*net.IPNet, 0, len(defaultBlockedCIDRs))
	for _, cidr := range defaultBlockedCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil && ipNet != nil {
			blockedNets = append(blockedNets, ipNet)
		}
	}

	return &Policy{
		AllowedSchemes:    []string{"http", "https"},
		MaxRedirects:      5,
		BlockedCIDRs:      blockedNets,
		AllowedHosts:      nil,
		AllowedProxyHosts: nil,
		AllowPrivate:      false,
		Resolver:          net.DefaultResolver,
	}
}

// isObfuscatedIP checks if a host string is an obfuscated or non-standard IP address representation
// (e.g., octal, hex, dword/integer, short-IP like 127.1, or mixed formats).
func isObfuscatedIP(host string) bool {
	clean := strings.TrimSpace(host)
	clean = strings.TrimSuffix(clean, ".")
	if clean == "" {
		return false
	}

	// 1. All-numeric or hex integer (e.g. "2130706433", "0x7f000001")
	if _, err := strconv.ParseUint(clean, 10, 64); err == nil {
		return true
	}
	if strings.HasPrefix(strings.ToLower(clean), "0x") {
		if _, err := strconv.ParseUint(clean[2:], 16, 64); err == nil {
			return true
		}
	}

	// 2. Dotted formats
	if strings.Contains(clean, ".") {
		parts := strings.Split(clean, ".")
		allNumericOrHex := true
		for _, part := range parts {
			p := strings.ToLower(strings.TrimSpace(part))
			if p == "" {
				allNumericOrHex = false
				break
			}
			if strings.HasPrefix(p, "0x") {
				if _, err := strconv.ParseUint(p[2:], 16, 64); err != nil {
					allNumericOrHex = false
					break
				}
			} else {
				if _, err := strconv.ParseUint(p, 10, 64); err != nil {
					allNumericOrHex = false
					break
				}
			}
		}

		if allNumericOrHex {
			// If it has 1 to 3 numeric parts (short IP like 127.1, 10.1), it is obfuscated
			if len(parts) >= 1 && len(parts) <= 3 {
				return true
			}
			// If it has 4 parts, check if any octet has leading zero (octal) or hex prefix
			if len(parts) == 4 {
				for _, part := range parts {
					p := strings.ToLower(strings.TrimSpace(part))
					if strings.HasPrefix(p, "0x") {
						return true
					}
					// In decimal IPv4, "0" is valid, but "00", "01", "0177" are octal
					if len(p) > 1 && p[0] == '0' {
						return true
					}
				}
			}
		}
	}

	return false
}

// parseCanonicalProxyURL validates a raw proxy endpoint and returns its normalized form:
// "http://<canonical-host>:<canonical-port>".
// Only explicit "http://" scheme is allowed. Ports must be explicit without leading zeros.
// Obfuscated IPs, IPv4-mapped IPv6, IPv6 Zone IDs, trailing dots, userinfo, and paths are rejected.
func parseCanonicalProxyURL(raw string) (string, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", domain.NewValidationError("invalid_proxy_url", "empty proxy endpoint")
	}

	if !strings.HasPrefix(strings.ToLower(clean), "http://") {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("unsupported proxy scheme in %q, only explicit http:// is allowed", raw))
	}

	u, err := url.Parse(clean)
	if err != nil {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("malformed proxy URL %q: %v", raw, err))
	}

	if strings.ToLower(u.Scheme) != "http" {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("unsupported proxy scheme %q, only explicit http:// is allowed", u.Scheme))
	}

	if u.User != nil {
		return "", domain.NewValidationError("invalid_proxy_url", "userinfo in proxy endpoint is not allowed")
	}

	if u.Path != "" && u.Path != "/" {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("path %q in proxy endpoint is not allowed", u.Path))
	}

	if u.RawQuery != "" || u.Fragment != "" {
		return "", domain.NewValidationError("invalid_proxy_url", "query or fragment in proxy endpoint is not allowed")
	}

	rawHost := u.Hostname()
	if rawHost == "" {
		return "", domain.NewValidationError("invalid_proxy_url", "proxy endpoint missing host")
	}

	if strings.HasSuffix(rawHost, ".") {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("proxy host %q has a trailing dot", rawHost))
	}

	if strings.Contains(rawHost, "%") {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("proxy host %q contains IPv6 zone ID", rawHost))
	}

	if isObfuscatedIP(rawHost) {
		return "", domain.NewSecurityError("proxy_ssrf_blocked", fmt.Sprintf("proxy host %q is an obfuscated IP format", rawHost))
	}

	if strings.Contains(strings.ToLower(rawHost), "::ffff:") || strings.Contains(strings.ToLower(rawHost), ":ffff:") {
		return "", domain.NewSecurityError("proxy_ssrf_blocked", fmt.Sprintf("proxy host %q is an IPv4-mapped IPv6 address", rawHost))
	}

	if ip := net.ParseIP(rawHost); ip != nil {
		if ip.To4() != nil && strings.Contains(rawHost, ":") {
			return "", domain.NewSecurityError("proxy_ssrf_blocked", fmt.Sprintf("proxy host %q is an IPv4-mapped IPv6 address", rawHost))
		}
	}

	port := u.Port()
	if port == "" {
		return "", domain.NewValidationError("invalid_proxy_url", "proxy endpoint must explicitly specify a port")
	}

	if len(port) > 1 && port[0] == '0' {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("invalid proxy port %q: leading zeros are not allowed", port))
	}

	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return "", domain.NewValidationError("invalid_proxy_url", fmt.Sprintf("invalid proxy port %q", port))
	}

	var canonicalHost string
	if ip := net.ParseIP(rawHost); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			canonicalHost = ipv4.String()
		} else {
			// RFC 5952 canonical IPv6 wrapped in brackets
			canonicalHost = "[" + ip.String() + "]"
		}
	} else {
		canonicalHost = strings.ToLower(rawHost)
	}

	return fmt.Sprintf("http://%s:%d", canonicalHost, portNum), nil
}

// AddAllowedProxy parses, validates, and registers a trusted outbound proxy endpoint.
// Only explicit http:// scheme is permitted. Ports must be explicitly specified without leading zeros.
// Endpoints are normalized into canonical http://<canonical-host>:<canonical-port> format.
func (p *Policy) AddAllowedProxy(rawProxy string) error {
	canonical, err := parseCanonicalProxyURL(rawProxy)
	if err != nil {
		return err
	}

	for _, existing := range p.AllowedProxyHosts {
		if existing == canonical {
			return nil
		}
	}

	p.AllowedProxyHosts = append(p.AllowedProxyHosts, canonical)
	return nil
}

// IsHostAllowed checks if a host (or host:port) is explicitly exempted from private network restrictions.
func (p *Policy) IsHostAllowed(target string) bool {
	if p.AllowPrivate {
		return true
	}
	cleanTarget := strings.ToLower(strings.TrimSpace(target))
	for _, allowed := range p.AllowedHosts {
		if strings.EqualFold(cleanTarget, strings.ToLower(strings.TrimSpace(allowed))) {
			return true
		}
	}
	return false
}

// IsProxyHostAllowed checks if an address or URL is explicitly trusted as an outbound proxy relay.
// Only exact matches against canonical http://<canonical-host>:<canonical-port> are permitted.
func (p *Policy) IsProxyHostAllowed(target string) bool {
	if p.AllowPrivate {
		return true
	}
	cleanTarget := strings.TrimSpace(target)
	if cleanTarget == "" {
		return false
	}

	canonical, err := parseCanonicalProxyURL(cleanTarget)
	if err != nil {
		return false
	}

	for _, allowed := range p.AllowedProxyHosts {
		if allowed == canonical {
			return true
		}
	}

	return false
}

// ValidateProxy validates an outbound proxy endpoint.
// Explicitly allowed proxy endpoints (e.g. http://host.docker.internal:7890) are trusted as proxy relays.
// Any other proxy URL must have an explicit http:// scheme, explicit port without leading zeros, no userinfo,
// no obfuscated IP, and pass SSRF IP checks to ensure arbitrary user input cannot target private networks,
// loopback, or cloud metadata.
func (p *Policy) ValidateProxy(ctx context.Context, u *url.URL) error {
	if u == nil {
		return domain.NewValidationError("invalid_proxy_url", "proxy URL is nil")
	}

	canonical, err := parseCanonicalProxyURL(u.String())
	if err != nil {
		return err
	}

	// 1. Check if exactly matches an explicitly trusted proxy endpoint
	if p.IsProxyHostAllowed(canonical) {
		return nil
	}

	// 2. Untrusted proxy: validate target against standard SSRF policy (rejects loopback, private, link-local, cloud metadata)
	parsed, err := url.Parse(canonical)
	if err != nil {
		return domain.NewValidationError("invalid_proxy_url", err.Error())
	}
	host := parsed.Hostname()

	if ip := net.ParseIP(host); ip != nil {
		if err := p.ValidateIP(ip); err != nil {
			return domain.NewSecurityError("proxy_ssrf_blocked", fmt.Sprintf("proxy host %s blocked by SSRF policy: %v", host, err))
		}
		return nil
	}

	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return domain.NewSecurityError("proxy_dns_resolution_failed", fmt.Sprintf("failed to resolve proxy host %s: %v", host, err))
	}
	if len(ips) == 0 {
		return domain.NewSecurityError("proxy_dns_resolution_failed", fmt.Sprintf("no IP addresses resolved for proxy host %s", host))
	}

	for _, ipAddr := range ips {
		if err := p.ValidateIP(ipAddr.IP); err != nil {
			return domain.NewSecurityError("proxy_ssrf_blocked", fmt.Sprintf("proxy host %s resolved to blocked IP %s: %v", host, ipAddr.IP.String(), err))
		}
	}

	return nil
}

// ValidateIP checks whether an IP address is blocked by SSRF restrictions.
func (p *Policy) ValidateIP(ip net.IP) error {
	if ip == nil {
		return domain.NewSecurityError("invalid_ip", "nil IP address")
	}

	// Unwrap IPv4-mapped IPv6 address (e.g. ::ffff:127.0.0.1)
	checkIP := ip
	if ipv4 := ip.To4(); ipv4 != nil {
		checkIP = ipv4
	}

	// Standard Go built-in predicates
	if checkIP.IsLoopback() || checkIP.IsPrivate() || checkIP.IsLinkLocalUnicast() ||
		checkIP.IsLinkLocalMulticast() || checkIP.IsMulticast() || checkIP.IsUnspecified() {
		return domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target IP %s is blocked by SSRF policy", ip.String()))
	}

	// Check explicit blocked CIDR networks
	for _, block := range p.BlockedCIDRs {
		if block != nil {
			// If checkIP is an IPv4 address, only evaluate against IPv4 CIDRs to avoid
			// 16-byte representation inadvertently matching IPv6 CIDRs (like ::ffff:0:0/96,
			// whose block.Mask is 16 bytes with 96 ones, matching 4-byte checkIP via 0.0.0.0/0).
			if checkIP.To4() != nil {
				ones, bits := block.Mask.Size()
				// IPv4 CIDRs have bits == 32; skip IPv6 CIDRs (bits == 128)
				if bits == 32 && block.Contains(checkIP) {
					return domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target IP %s is blocked by SSRF policy (%s)", ip.String(), block.String()))
				}
				// Also handle IPv4-mapped IPv6 CIDRs (bits == 128, ones == 96) - do not match plain IPv4
				_ = ones
			} else {
				if block.Contains(checkIP) || block.Contains(ip) {
					return domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target IP %s is blocked by SSRF policy (%s)", ip.String(), block.String()))
				}
			}
		}
	}

	return nil
}

// ValidateTarget verifies the URL scheme and resolves all destination IPs against the SSRF policy.
func (p *Policy) ValidateTarget(ctx context.Context, u *url.URL) error {
	if u == nil {
		return domain.NewValidationError("invalid_url", "URL is nil")
	}

	// 1. Validate Scheme
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return domain.NewValidationError("missing_scheme", "URL is missing a scheme")
	}

	schemeAllowed := false
	for _, allowed := range p.AllowedSchemes {
		if strings.EqualFold(scheme, allowed) {
			schemeAllowed = true
			break
		}
	}
	if !schemeAllowed {
		return domain.NewSecurityError("unsupported_scheme", fmt.Sprintf("unsupported scheme %q, only http and https are allowed", u.Scheme))
	}

	// 2. Validate Hostname
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return domain.NewValidationError("invalid_url", "URL is missing a host")
	}

	if strings.HasSuffix(host, ".") {
		return domain.NewValidationError("invalid_url", fmt.Sprintf("target host %q has a trailing dot", host))
	}

	if isObfuscatedIP(host) {
		return domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target host %s is an obfuscated IP format", host))
	}

	if strings.Contains(strings.ToLower(host), "::ffff:") || strings.Contains(strings.ToLower(host), ":ffff:") {
		return domain.NewSecurityError("ssrf_blocked", fmt.Sprintf("target host %s is an IPv4-mapped IPv6 address", host))
	}

	if p.IsHostAllowed(host) || p.IsHostAllowed(u.Host) {
		return nil
	}

	// 3. Check IP literal
	if ip := net.ParseIP(host); ip != nil {
		return p.ValidateIP(ip)
	}

	// 4. Resolve DNS and validate every resolved IP
	resolver := p.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("failed to resolve host %s: %v", host, err))
	}
	if len(ips) == 0 {
		return domain.NewSecurityError("dns_resolution_failed", fmt.Sprintf("no IP addresses resolved for host %s", host))
	}

	for _, ipAddr := range ips {
		if err := p.ValidateIP(ipAddr.IP); err != nil {
			return err
		}
	}

	return nil
}
