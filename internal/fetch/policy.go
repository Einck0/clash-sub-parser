// Package fetch provides secure HTTP fetching for subscription sources with strict SSRF defense.
package fetch

import (
	"context"
	"fmt"
	"net"
	"net/url"
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
	AllowedSchemes []string
	MaxRedirects   int
	BlockedCIDRs   []*net.IPNet
	AllowedHosts   []string
	AllowPrivate   bool
	Resolver       Resolver
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
		AllowedSchemes: []string{"http", "https"},
		MaxRedirects:   5,
		BlockedCIDRs:   blockedNets,
		AllowedHosts:   nil,
		AllowPrivate:   false,
		Resolver:       net.DefaultResolver,
	}
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
	host := u.Hostname()
	if host == "" {
		return domain.NewValidationError("invalid_url", "URL is missing a host")
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
