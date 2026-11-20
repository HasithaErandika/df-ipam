package firewall

import (
	"context"
	"net/netip"
	"sort"
	"strings"

	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/platform/db"
)

// hostSource is where the firewall names an address: an address object, a
// virtual IP or a DHCP reservation.
type hostSource struct {
	Kind    string `json:"kind"` // address, vip, reserved
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

// hostProposal is one address the firewall names, as a register entry.
type hostProposal struct {
	IP          string         `json:"ip"`
	Name        string         `json:"name"`
	MAC         string         `json:"mac"`
	Sources     []hostSource   `json:"sources"`
	SegmentID   string         `json:"segmentId"`
	SegmentName string         `json:"segmentName"`
	SegmentKind string         `json:"segmentKind"`
	IPType      string         `json:"ipType"`
	Notes       []string       `json:"notes"`
	Registered  *registeredRef `json:"registered"`
}

type registeredRef struct {
	ID   string `db:"id" json:"id"`
	IP   string `db:"ip" json:"ip"`
	Name string `db:"assigned_name" json:"name"`
}

// hosts gathers the addresses the firewall names, one entry per IP.
type hosts map[string]*hostProposal

func (h hosts) add(ip string, src hostSource, mac string) {
	a, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !a.Is4() || a.IsUnspecified() {
		return
	}
	key := a.String()
	if h[key] == nil {
		h[key] = &hostProposal{IP: key, Sources: []hostSource{}, Notes: []string{}}
	}
	p := h[key]
	p.Sources = append(p.Sources, src)
	p.Name, p.MAC = firstOf(p.Name, src.Name), firstOf(p.MAC, mac)
}

// servers proposes register entries for single-host address objects, the
// inside addresses of virtual IPs and DHCP reservations.
func (h *Handler) servers(ctx context.Context, client *fortigate.Client, out *inventory, addrs []fortigate.Address, dhcp []fortigate.DHCPServer) error {
	found := hosts{}
	found.fromAddresses(addrs)
	if vips, err := client.VIPs(ctx); err == nil {
		found.fromVIPs(vips)
	} else {
		out.Problems = append(out.Problems, "Virtual IPs: "+err.Error())
	}
	for _, d := range dhcp {
		for _, r := range d.Reserved {
			found.add(r.IP, hostSource{"reserved", firstOf(r.Description, "DHCP reservation"), r.Description}, r.MAC)
		}
	}
	return h.placeHosts(ctx, out, found)
}

func (h hosts) fromAddresses(addrs []fortigate.Address) {
	for _, a := range addrs {
		if ip, p, ok := prefix(a.Subnet); ok && p.Bits() == 32 && (a.Type == "" || a.Type == "ipmask") {
			h.add(ip.String(), hostSource{"address", a.Name, a.Comment}, "")
		}
	}
}

func (h hosts) fromVIPs(vips []fortigate.VIP) {
	for _, v := range vips {
		for _, m := range v.MappedIP {
			// One inside address ("10.0.0.5" or "10.0.0.5-10.0.0.5"), not a range.
			if parts := strings.Split(m.Range, "-"); len(parts) == 1 || parts[0] == parts[1] {
				h.add(parts[0], hostSource{"vip", v.Name, firstOf(v.Comment, "Published on "+v.ExtIP)}, "")
			}
		}
	}
}

// placeHosts puts each address in its segment and marks those already registered.
func (h *Handler) placeHosts(ctx context.Context, out *inventory, found hosts) error {
	ips := make([]string, 0, len(found))
	for ip := range found {
		ips = append(ips, ip)
	}
	regs, err := db.All[registeredRef](ctx, h.conn, `SELECT id::text AS id, host(ip) AS ip, assigned_name FROM ip_addresses WHERE ip = ANY($1::inet[])`, ips)
	if err != nil {
		return err
	}
	for i := range regs {
		if p := found[regs[i].IP]; p != nil {
			p.Registered = &regs[i]
		}
	}
	for _, p := range found {
		place(p, out.Segments)
		out.Servers = append(out.Servers, *p)
	}
	sort.Slice(out.Servers, func(i, j int) bool {
		return netip.MustParseAddr(out.Servers[i].IP).Less(netip.MustParseAddr(out.Servers[j].IP))
	})
	return nil
}

// place finds the address's segment and the register type that fits it.
func place(p *hostProposal, segs []segmentRecord) {
	a := netip.MustParseAddr(p.IP)
	for _, s := range segs {
		if n, err := netip.ParsePrefix(s.Network); err == nil && n.Contains(a) {
			p.SegmentID, p.SegmentName, p.SegmentKind = s.ID, s.Name, s.Kind
			break
		}
	}
	p.IPType = "other"
	if p.SegmentKind == "server" || p.SegmentKind == "cloud" {
		p.IPType = "server"
	}
	if p.SegmentID == "" && p.Registered == nil {
		p.Notes = append(p.Notes, "Not inside any segment: add its network first (Networks tab or Network setup).")
	}
}
