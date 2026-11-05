// Package safehttp makes outbound requests to user-configured URLs safe from SSRF.
// See docs/security.md. Only https to an allowlisted host is permitted, every
// address is checked after DNS resolution, the connection goes to the checked
// address so rebinding can't swap it, and redirects are never followed.
package safehttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrBlocked marks a URL or address the policy refuses.
var ErrBlocked = errors.New("blocked by the outbound policy")

// Resolver is the part of net.Resolver the policy needs.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Policy says where integrations may connect.
type Policy struct {
	// Hosts are exact host names, or suffixes written as ".example.com".
	Hosts []string
	// PrivateRanges are the private ranges connections may reach. Private
	// addresses outside these are refused.
	PrivateRanges []netip.Prefix
	// RootCAs adds private certificate authorities. Nil uses the system store.
	RootCAs  *x509.CertPool
	Resolver Resolver
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.ToLower(strings.TrimSpace(item)); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// FromEnv reads INTEGRATION_ALLOWED_HOSTS, INTEGRATION_ALLOWED_PRIVATE_RANGES and
// INTEGRATION_CA_FILE. With no hosts set, every URL is refused.
func FromEnv() (Policy, error) {
	p := Policy{Hosts: splitList(os.Getenv("INTEGRATION_ALLOWED_HOSTS"))}
	for _, item := range splitList(os.Getenv("INTEGRATION_ALLOWED_PRIVATE_RANGES")) {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return p, fmt.Errorf("INTEGRATION_ALLOWED_PRIVATE_RANGES: %q is not a CIDR", item)
		}
		p.PrivateRanges = append(p.PrivateRanges, prefix.Masked())
	}
	if file := strings.TrimSpace(os.Getenv("INTEGRATION_CA_FILE")); file != "" {
		pem, err := os.ReadFile(file)
		if err != nil {
			return p, fmt.Errorf("INTEGRATION_CA_FILE: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return p, errors.New("INTEGRATION_CA_FILE holds no certificates")
		}
		p.RootCAs = pool
	}
	return p, nil
}

func (p Policy) hostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, allowed := range p.Hosts {
		if host == allowed || (strings.HasPrefix(allowed, ".") && strings.HasSuffix(host, allowed)) {
			return true
		}
	}
	return false
}

// CheckURL validates a configured URL without connecting.
func (p Policy) CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: the URL can't be read", ErrBlocked)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: only https is allowed", ErrBlocked)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: credentials don't belong in the URL", ErrBlocked)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w: the URL has no host", ErrBlocked)
	}
	if !p.hostAllowed(u.Hostname()) {
		return nil, fmt.Errorf("%w: %s is not on the allowlist", ErrBlocked, u.Hostname())
	}
	return u, nil
}

var (
	// Never reachable, whatever the allowlist says.
	alwaysBlocked = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT, and Alibaba's metadata address
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("169.254.0.0/16"), // link-local, including cloud metadata
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/128"),
		netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("64:ff9b::/96"), // NAT64 could reach any IPv4 address
		netip.MustParsePrefix("2002::/16"),    // 6to4, the same
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fec0::/10"),
		netip.MustParsePrefix("ff00::/8"),
		netip.MustParsePrefix("fd00:ec2::/32"), // AWS IPv6 metadata
	}
	private = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("fc00::/7"),
	}
)

func inAny(a netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// CheckAddr applies the address rules. IPv4-mapped IPv6 forms are normalised first.
func (p Policy) CheckAddr(a netip.Addr) error {
	a = a.Unmap().WithZone("")
	if !a.IsValid() || inAny(a, alwaysBlocked) {
		return fmt.Errorf("%w: %s is never reachable", ErrBlocked, a)
	}
	if inAny(a, private) && !inAny(a, p.PrivateRanges) {
		return fmt.Errorf("%w: private address %s is not on the allowlist", ErrBlocked, a)
	}
	return nil
}

// DialContext resolves the host, checks every address, then connects to a checked
// address. Refusing when any answer is bad stops a mixed DNS response.
func (p Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if !p.hostAllowed(host) {
		return nil, fmt.Errorf("%w: %s is not on the allowlist", ErrBlocked, host)
	}
	var addrs []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{literal}
	} else {
		var resolver Resolver = net.DefaultResolver
		if p.Resolver != nil {
			resolver = p.Resolver
		}
		if addrs, err = resolver.LookupNetIP(ctx, "ip", host); err != nil {
			return nil, err
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	for _, a := range addrs {
		if err := p.CheckAddr(a); err != nil {
			return nil, err
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	var last error
	for _, a := range addrs {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

// Transport returns a transport that uses the policy. TLS verification stays on,
// against the original host name.
func (p Policy) Transport() *http.Transport {
	return &http.Transport{
		DialContext:           p.DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: p.RootCAs},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       30 * time.Second,
		DisableCompression:    false,
	}
}

// Client returns an HTTP client that follows no redirects and refuses proxies.
func (p Policy) Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: p.Transport(),
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("%w: redirects are not followed", ErrBlocked)
		},
	}
}
