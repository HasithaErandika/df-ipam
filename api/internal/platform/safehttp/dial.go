package safehttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

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
