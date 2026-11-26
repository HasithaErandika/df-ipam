package firewall

import (
	"context"
	"net/netip"
	"strings"

	"dfipam/api/internal/integrations/fortigate"
)

type segmentRecord struct {
	ID        string `db:"id" json:"id"`
	Name      string `db:"name" json:"name"`
	Kind      string `db:"kind" json:"kind"`
	Network   string `db:"network" json:"network"`
	Gateway   string `db:"gateway" json:"gateway"`
	VLAN      *int32 `db:"vlan" json:"vlan"`
	DHCPStart string `db:"dhcp_start" json:"dhcpStart"`
	DHCPEnd   string `db:"dhcp_end" json:"dhcpEnd"`
}

// netProposal is one firewall interface network, as a segment.
type netProposal struct {
	Key       string         `json:"key"`
	Interface string         `json:"interface"`
	Alias     string         `json:"alias"`
	Role      string         `json:"role"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"` // dhcp when the firewall serves DHCP on it, else server
	Network   string         `json:"network"`
	Gateway   string         `json:"gateway"`
	VLAN      *int           `json:"vlan"`
	DHCPStart string         `json:"dhcpStart"`
	DHCPEnd   string         `json:"dhcpEnd"`
	CanAdd    bool           `json:"canAdd"`
	Notes     []string       `json:"notes"`
	Existing  *segmentRecord `json:"existing"`
	Changes   []change       `json:"changes"`
}

// prefix reads FortiOS's "192.168.10.1 255.255.255.0" as the address and its network.
func prefix(v string) (netip.Addr, netip.Prefix, bool) {
	parts := strings.Fields(v)
	if len(parts) != 2 {
		return netip.Addr{}, netip.Prefix{}, false
	}
	a, err1 := netip.ParseAddr(parts[0])
	m, err2 := netip.ParseAddr(parts[1])
	if err1 != nil || err2 != nil || !a.Is4() || a.IsUnspecified() {
		return netip.Addr{}, netip.Prefix{}, false
	}
	bits := 0
	for _, b := range m.As4() {
		for ; b != 0; b <<= 1 {
			bits++
		}
	}
	p, err := a.Prefix(bits)
	return a, p, err == nil
}

// networks proposes a segment for each LAN, DMZ and VLAN interface with a
// static address. WAN interfaces are Network Links, not segments.
func (h *Handler) networks(ctx context.Context, client *fortigate.Client, out *inventory, dhcp []fortigate.DHCPServer) error {
	ifaces, err := client.InterfaceConfigs(ctx)
	if err != nil {
		return err
	}
	pools := map[string]fortigate.DHCPServer{}
	for _, d := range dhcp {
		if d.Status != "disable" {
			pools[d.Interface] = d
		}
	}
	for _, x := range ifaces {
		addr, p, ok := prefix(x.IP)
		if !ok || strings.EqualFold(x.Role, "wan") || x.Type == "tunnel" || x.Type == "loopback" || !strings.EqualFold(firstOf(x.Mode, "static"), "static") {
			continue
		}
		n := netProposal{Key: "net:" + x.Name, Interface: x.Name, Alias: x.Alias, Role: x.Role, Name: firstOf(x.Alias, x.Name), Kind: "server",
			Network: p.String(), Gateway: addr.String(), CanAdd: true, Notes: []string{}, Changes: []change{}}
		if x.VLAN > 0 {
			v := x.VLAN
			n.VLAN = &v
		}
		if d, ok := pools[x.Name]; ok && len(d.Ranges) > 0 {
			n.Kind, n.DHCPStart, n.DHCPEnd = "dhcp", d.Ranges[0].Start, d.Ranges[0].End
		}
		match(&n, p, out.Segments)
		out.Networks = append(out.Networks, n)
	}
	return nil
}

// match finds the segment with this network, or one that overlaps it, which
// stops it being added.
func match(n *netProposal, p netip.Prefix, segs []segmentRecord) {
	if p.Bits() < 16 || p.Bits() > 30 {
		n.CanAdd = false
		n.Notes = append(n.Notes, "DF-Atlas segments are /16 to /30, so "+p.String()+" can't be added.")
	}
	for i := range segs {
		s := segs[i]
		other, err := netip.ParsePrefix(s.Network)
		if err != nil || !other.Overlaps(p) {
			continue
		}
		if other == p {
			n.Existing, n.CanAdd = &s, false
			n.Changes = segmentChanges(*n, s)
			return
		}
		n.CanAdd = false
		n.Notes = append(n.Notes, "Overlaps the segment "+s.Name+" ("+s.Network+"), so it can't be added.")
	}
}

func segmentChanges(n netProposal, s segmentRecord) []change {
	out := []change{}
	if n.Gateway != s.Gateway {
		out = append(out, change{"gateway", s.Gateway, n.Gateway})
	}
	var vlan *int
	if s.VLAN != nil {
		v := int(*s.VLAN)
		vlan = &v
	}
	if n.VLAN != nil && (vlan == nil || *vlan != *n.VLAN) {
		out = append(out, change{"vlan", vlan, n.VLAN})
	}
	if n.DHCPStart != "" && (n.DHCPStart != s.DHCPStart || n.DHCPEnd != s.DHCPEnd) && s.Kind == "dhcp" {
		out = append(out, change{"dhcpPool", s.DHCPStart + " – " + s.DHCPEnd, n.DHCPStart + " – " + n.DHCPEnd})
	}
	return out
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
