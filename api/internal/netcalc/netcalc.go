// Package netcalc does IPv4 and IPv6 subnet arithmetic. Counts are big integers
// because an IPv6 prefix can hold 2^128 addresses.
package netcalc

import (
	"math/big"
	"net/netip"
)

// Info describes a prefix. Fields that don't apply are empty.
type Info struct {
	Family      int    `json:"family"`
	CIDR        string `json:"cidr"`
	Network     string `json:"network"`
	PrefixLen   int    `json:"prefixLength"`
	Mask        string `json:"mask,omitempty"`
	Broadcast   string `json:"broadcast,omitempty"`
	FirstUsable string `json:"firstUsable"`
	LastUsable  string `json:"lastUsable"`
	Total       string `json:"totalAddresses"`
	Usable      string `json:"usableAddresses"`
}

func lastAddress(p netip.Prefix) netip.Addr {
	b := p.Addr().AsSlice()
	for i := p.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 1 << (7 - i%8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

func mask(bits int) string {
	b := make([]byte, 4)
	for i := 0; i < bits; i++ {
		b[i/8] |= 1 << (7 - i%8)
	}
	return netip.AddrFrom4([4]byte(b)).String()
}

// Calculate returns the details of a canonical prefix.
func Calculate(p netip.Prefix) Info {
	p = p.Masked()
	network, last := p.Addr(), lastAddress(p)
	total := new(big.Int).Lsh(big.NewInt(1), uint(network.BitLen()-p.Bits()))
	usable := new(big.Int).Set(total)
	info := Info{CIDR: p.String(), Network: network.String(), PrefixLen: p.Bits(),
		FirstUsable: network.String(), LastUsable: last.String()}

	if network.Is4() {
		info.Family = 4
		info.Mask = mask(p.Bits())
		// /31 (RFC 3021) and /32 have no network or broadcast address.
		if p.Bits() <= 30 {
			info.Broadcast = last.String()
			info.FirstUsable = network.Next().String()
			info.LastUsable = last.Prev().String()
			usable.Sub(usable, big.NewInt(2))
		}
	} else {
		info.Family = 6 // IPv6 has no broadcast; /127 follows RFC 6164
	}
	info.Total, info.Usable = total.String(), usable.String()
	return info
}

// UtilisationPercent returns used/usable as a percentage with two decimals.
func UtilisationPercent(used *big.Int, usable string) string {
	u, ok := new(big.Rat).SetString(usable)
	if !ok || u.Sign() == 0 {
		return "0.00"
	}
	pct := new(big.Rat).SetInt(new(big.Int).Mul(used, big.NewInt(100)))
	return pct.Quo(pct, u).FloatString(2)
}
