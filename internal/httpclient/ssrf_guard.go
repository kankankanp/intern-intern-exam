package httpclient

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var (
	ErrInvalidScheme = errors.New("ssrf: invalid scheme, only http/https allowed")
	ErrPrivateIP     = errors.New("ssrf: private IP address not allowed")
	ErrLoopback      = errors.New("ssrf: loopback address not allowed")
	ErrLinkLocal     = errors.New("ssrf: link-local address not allowed")
	ErrInvalidHost   = errors.New("ssrf: invalid host")
)

// SSRFGuard validates URLs and IPs to prevent SSRF attacks
type SSRFGuard struct {
	blockedRanges []*net.IPNet
}

// NewSSRFGuard creates a new SSRF guard
func NewSSRFGuard() *SSRFGuard {
	blockedCIDRs := []string{
		"0.0.0.0/8",       // Current network
		"10.0.0.0/8",      // Private (Class A)
		"100.64.0.0/10",   // Carrier-grade NAT
		"127.0.0.0/8",     // Loopback
		"169.254.0.0/16",  // Link-local
		"172.16.0.0/12",   // Private (Class B)
		"192.0.0.0/24",    // IETF Protocol Assignments
		"192.0.2.0/24",    // TEST-NET-1
		"192.168.0.0/16",  // Private (Class C)
		"198.18.0.0/15",   // Benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"224.0.0.0/4",     // Multicast
		"240.0.0.0/4",     // Reserved
		"255.255.255.255/32", // Broadcast
		// IPv6
		"::/128",     // Unspecified
		"::1/128",    // Loopback
		"fc00::/7",   // Unique local
		"fe80::/10",  // Link-local
		"ff00::/8",   // Multicast
	}

	var blockedRanges []*net.IPNet
	for _, cidr := range blockedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			blockedRanges = append(blockedRanges, network)
		}
	}

	return &SSRFGuard{
		blockedRanges: blockedRanges,
	}
}

// ValidateURL validates a URL for SSRF protection
func (g *SSRFGuard) ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	// Layer 1: Scheme validation
	if err := g.validateScheme(u.Scheme); err != nil {
		return err
	}

	// Layer 2: Host validation
	if err := g.validateHost(u.Host); err != nil {
		return err
	}

	return nil
}

// ValidateIP validates an IP address for SSRF protection
func (g *SSRFGuard) ValidateIP(ip net.IP) error {
	if ip == nil {
		return ErrInvalidHost
	}

	// Loopback
	if ip.IsLoopback() {
		return ErrLoopback
	}

	// Private
	if ip.IsPrivate() {
		return ErrPrivateIP
	}

	// Link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return ErrLinkLocal
	}

	// Unspecified (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return ErrPrivateIP
	}

	// Check against blocked ranges
	for _, network := range g.blockedRanges {
		if network.Contains(ip) {
			return ErrPrivateIP
		}
	}

	return nil
}

func (g *SSRFGuard) validateScheme(scheme string) error {
	scheme = strings.ToLower(scheme)
	if scheme != "http" && scheme != "https" {
		return ErrInvalidScheme
	}
	return nil
}

func (g *SSRFGuard) validateHost(host string) error {
	// Split host and port
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	// Empty host
	if hostname == "" {
		return ErrInvalidHost
	}

	// Localhost variations
	if g.isLocalhost(hostname) {
		return ErrLoopback
	}

	// Direct IP address
	if ip := net.ParseIP(hostname); ip != nil {
		return g.ValidateIP(ip)
	}

	return nil
}

func (g *SSRFGuard) isLocalhost(hostname string) bool {
	hostname = strings.ToLower(hostname)
	localNames := []string{
		"localhost",
		"localhost.localdomain",
		"127.0.0.1",
		"::1",
		"[::1]",
	}
	for _, name := range localNames {
		if hostname == name {
			return true
		}
	}
	return false
}
