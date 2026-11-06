// Package discovery runs controlled, approved discovery. Every job's scope is
// frozen when it is created: the policy's allowlist bounds it, exclusions and
// blackouts always win, sensitive networks get passive methods only, and the
// emergency stop halts everything. Results feed evidence and observation
// state only: discovery never changes allocation state or ownership.
package discovery

import (
	"net/netip"

	"dfipam/api/internal/netcalc"
)

// Methods an agent can run. Passive methods send nothing to the targets.
var Methods = map[string]bool{"icmp": true, "arp": true, "ndp": true, "dns": true, "snmp": true, "nmap": true}

var passive = map[string]bool{"dns": true}

// SourceName maps a method to its observation source.
var SourceName = map[string]string{"icmp": "ICMP", "arp": "ARP", "ndp": "ARP", "dns": "DNS", "snmp": "SNMP", "nmap": "Nmap"}

// Sweep limits: an active sweep never covers more than this many addresses
// per target. Larger IPv4 subnets must be split; IPv6 subnets are probed only
// at their recorded addresses, never enumerated.
const (
	MaxIPv4Bits     = 20 // a /20 holds 4,096 addresses
	MaxIPv6Bits     = 120
	MaxKnownTargets = 1024
	MaxTargets      = 512
	MaxJobAddresses = 65536 // about 22 minutes at the default 50 packets a second
)

// Target is one frozen piece of a job's scope.
type Target struct {
	RoutingDomainID string `json:"routingDomainId"`
	CIDR            string `json:"cidr"`
}

// Exclusion is a range discovery must never touch.
type Exclusion struct {
	RoutingDomainID string // empty means every routing domain
	CIDR            netip.Prefix
}

// Probeable refuses networks that reach many hosts at once, or that never belong on
// a sweep. A sweep of multicast space is amplified by every listener.
func Probeable(p netip.Prefix) bool {
	a := p.Masked().Addr()
	return !a.IsMulticast() && !a.IsLoopback() && !a.IsUnspecified() && !a.IsLinkLocalUnicast() && !a.IsLinkLocalMulticast()
}

// addressesIn counts a target that has already passed TooLarge, so it stays small.
func addressesIn(cidr string) int {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return 0
	}
	return 1 << (p.Addr().BitLen() - p.Bits())
}

// Allowed reports whether the whole prefix sits inside the allowlist.
func Allowed(p netip.Prefix, allowlist []netip.Prefix) bool {
	for _, a := range allowlist {
		if a.Bits() <= p.Bits() && a.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// Subtract removes excluded ranges from a prefix and returns what is left as
// prefixes. Exclusions always win over allowlists.
func Subtract(p netip.Prefix, routingDomainID string, exclusions []Exclusion) []netip.Prefix {
	var used []netcalc.Range
	for _, e := range exclusions {
		if (e.RoutingDomainID == "" || e.RoutingDomainID == routingDomainID) && e.CIDR.Addr().BitLen() == p.Addr().BitLen() && e.CIDR.Overlaps(p) {
			used = append(used, netcalc.PrefixRange(e.CIDR))
		}
	}
	if len(used) == 0 {
		return []netip.Prefix{p}
	}
	var out []netip.Prefix
	for _, g := range netcalc.Gaps(netcalc.PrefixRange(p), used) {
		out = append(out, netcalc.Prefixes(g, MaxTargets)...)
	}
	return out
}

// TooLarge reports a prefix an active sweep may not cover whole.
func TooLarge(p netip.Prefix) bool {
	if p.Addr().Is4() {
		return p.Bits() < MaxIPv4Bits
	}
	return p.Bits() < MaxIPv6Bits
}

// FilterMethods keeps the policy's methods a target may receive. Sensitive
// targets get passive methods only, unless the policy itself is an approved
// sensitive policy.
func FilterMethods(methods []string, sensitiveTarget, sensitivePolicy bool) []string {
	var out []string
	for _, m := range methods {
		if !Methods[m] {
			continue
		}
		if sensitiveTarget && !sensitivePolicy && !passive[m] {
			continue
		}
		out = append(out, m)
	}
	return out
}
