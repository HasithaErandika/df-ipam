package firewall

import (
	"context"
	"net/netip"

	"dfipam/api/internal/integrations/fortigate"
)

// covering is the smallest network holding every address from start to end,
// as a VPN client pool segment needs one network.
func covering(start, end string) (netip.Prefix, bool) {
	a, err1 := netip.ParseAddr(start)
	b, err2 := netip.ParseAddr(end)
	if err1 != nil || err2 != nil || !a.Is4() || !b.Is4() {
		return netip.Prefix{}, false
	}
	for bits := 32; bits >= 0; bits-- {
		p, _ := a.Prefix(bits)
		if p.Contains(b) {
			return p, true
		}
	}
	return netip.Prefix{}, false
}

// vpnPools proposes a VPN client pool segment for each SSL VPN tunnel pool
// and each dial-up IPsec tunnel that hands out addresses.
func (h *Handler) vpnPools(ctx context.Context, client *fortigate.Client, out *inventory, addrs []fortigate.Address) {
	byName := map[string]fortigate.Address{}
	for _, a := range addrs {
		byName[a.Name] = a
	}
	if names, err := client.SSLPools(ctx); err == nil {
		for _, name := range names {
			if p, span, ok := poolNetwork(byName[name]); ok {
				addPool(out, "vpn:ssl:"+name, "SSL VPN · "+name, "ssl.root", p, span)
			}
		}
	} else {
		out.Problems = append(out.Problems, "SSL VPN settings: "+err.Error())
	}
	if p1, _, err := client.IPsec(ctx); err == nil {
		dialupPools(out, p1)
	}
}

// dialupPools adds the address range each dial-up IPsec tunnel hands out.
func dialupPools(out *inventory, p1 []fortigate.Phase1) {
	for _, t := range p1 {
		if t.Type != "dynamic" || t.ModeCfg != "enable" {
			continue
		}
		if p, ok := covering(t.StartIP, t.EndIP); ok {
			addPool(out, "vpn:ipsec:"+t.Name, "IPsec dial-up · "+t.Name, t.Name, p, t.StartIP+" – "+t.EndIP)
		}
	}
}

func addPool(out *inventory, key, name, iface string, p netip.Prefix, span string) {
	n := netProposal{Key: key, Interface: iface, Name: name, Kind: "vpn", Network: p.String(), CanAdd: true, Notes: []string{}, Changes: []change{}}
	if span != "" && p.String() != span {
		n.Notes = append(n.Notes, "Addresses "+span+" are handed out; the segment is the network that holds them.")
	}
	match(&n, p, out.Segments)
	out.Networks = append(out.Networks, n)
}

// poolNetwork is the network of an SSL VPN pool's address object: an address
// range, or a subnet.
func poolNetwork(a fortigate.Address) (netip.Prefix, string, bool) {
	if a.Type == "iprange" {
		p, ok := covering(a.StartIP, a.EndIP)
		return p, a.StartIP + " – " + a.EndIP, ok
	}
	_, p, ok := prefix(a.Subnet)
	return p, "", ok
}
