package links

import (
	"net/netip"
	"strconv"
	"strings"

	"dfipam/api/internal/validate"
)

// splitAddress reads an IPv4 address written the ways network diagrams write
// it: "10.10.40.253", "10.10.40.253/24", "10.10.40.253 /24" or
// "122.255.4.226 255.255.255.248". bits is -1 when no prefix or mask is given.
func splitAddress(value string) (netip.Addr, int, bool) {
	value = strings.TrimSpace(value)
	var addr, prefix string
	if i := strings.IndexByte(value, '/'); i >= 0 {
		addr, prefix = strings.TrimSpace(value[:i]), strings.TrimSpace(value[i+1:])
	} else if parts := strings.Fields(value); len(parts) == 2 {
		addr, prefix = parts[0], parts[1]
	} else {
		addr = value
	}
	a, err := netip.ParseAddr(addr)
	if err != nil || !a.Is4() {
		return netip.Addr{}, 0, false
	}
	if prefix == "" {
		return a, -1, !strings.Contains(value, "/")
	}
	bits, ok := prefixBits(prefix)
	return a, bits, ok
}

// prefixBits reads "24" or "255.255.255.0". A mask must be contiguous.
func prefixBits(p string) (int, bool) {
	if n, err := strconv.Atoi(p); err == nil {
		return n, n >= 0 && n <= 32
	}
	m, err := netip.ParseAddr(p)
	if err != nil || !m.Is4() {
		return 0, false
	}
	b := m.As4()
	mask := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	bits := 0
	for mask&0x80000000 != 0 {
		bits, mask = bits+1, mask<<1
	}
	return bits, mask == 0
}

// hostAddress checks a single IPv4 address. A prefix or mask written after it
// (an interface address such as "10.10.40.253 /24") is accepted and dropped:
// the network belongs in the WAN subnet.
func hostAddress(v *validate.V, field string, value *string, label string) {
	*value = strings.TrimSpace(*value)
	if *value == "" {
		return
	}
	a, _, ok := splitAddress(*value)
	if !ok {
		v.Add(field, label+" must be an IPv4 address such as 10.10.40.253, optionally with its prefix (/24) or mask.")
		return
	}
	*value = a.String()
}

// ipOrNetwork accepts an IPv4 address or network, with a prefix or a dotted
// mask, and returns it in canonical form.
func ipOrNetwork(v *validate.V, field, value, label string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	a, bits, ok := splitAddress(value)
	if !ok {
		v.Add(field, label+" must be an IPv4 address or network such as 203.0.113.0/29.")
		return value
	}
	if bits < 0 {
		bits = 32
	}
	p := netip.PrefixFrom(a, bits)
	if p.Masked() != p {
		v.Add(field, label+": use the network address "+p.Masked().String()+".")
	}
	return p.String()
}

// wanNetwork is the network a WAN IP written with its prefix sits in
// ("10.10.40.253 /24" gives 10.10.40.0/24), or "" when no prefix is given.
func wanNetwork(value string) string {
	a, bits, ok := splitAddress(value)
	if !ok || bits < 0 {
		return ""
	}
	return netip.PrefixFrom(a, bits).Masked().String()
}

// extraWANIPs checks a link's further WAN addresses. Each keeps its subnet's
// prefix ("122.255.35.1/32", "10.10.40.253/24"); one without a prefix is a /32.
// The main WAN IP and repeats are left out.
func extraWANIPs(v *validate.V, list []string, main string) []string {
	if len(list) > 16 {
		v.Add("extraWanIps", "List at most 16 more WAN addresses.")
	}
	out, seen := []string{}, map[string]bool{main: true}
	for _, s := range list {
		if strings.TrimSpace(s) == "" {
			continue
		}
		a, bits, ok := splitAddress(s)
		if !ok {
			v.Add("extraWanIps", "Each WAN address must be an IPv4 address such as 122.255.35.1, optionally with its prefix (/32) or mask.")
			continue
		}
		if bits < 0 {
			bits = 32
		}
		if !seen[a.String()] {
			seen[a.String()] = true
			out = append(out, netip.PrefixFrom(a, bits).String())
		}
	}
	return out
}
