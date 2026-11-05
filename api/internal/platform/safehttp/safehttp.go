// Package safehttp makes outbound requests to user-configured URLs safe from SSRF.
// See docs/security.md. Only https to an allowlisted host is permitted, every
// address is checked after DNS resolution, the connection goes to the checked
// address so rebinding can't swap it, and redirects are never followed.
package safehttp

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strings"
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
