package links

import (
	"context"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/platform/db"
)

// ipAndPrefix reads FortiOS's "203.0.113.2 255.255.255.252" as the address and
// "/30". An unset address ("0.0.0.0 0.0.0.0") gives nothing.
func ipAndPrefix(v string) (string, string) {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return "", ""
	}
	a, err := netip.ParseAddr(parts[0])
	if err != nil || !a.Is4() || a.IsUnspecified() {
		return "", ""
	}
	if len(parts) < 2 {
		return a.String(), ""
	}
	m, err := netip.ParseAddr(parts[1])
	if err != nil {
		return a.String(), ""
	}
	bits := 0
	for _, b := range m.As4() {
		bits += strings.Count(strconv.FormatUint(uint64(b), 2), "1")
	}
	return a.String(), "/" + strconv.Itoa(bits)
}

// network reads "10.20.0.0 255.255.0.0" as 10.20.0.0/16.
func network(v string) string {
	a, p := ipAndPrefix(v)
	if a == "" || p == "" {
		return ""
	}
	pre, err := netip.ParsePrefix(a + p)
	if err != nil {
		return ""
	}
	return pre.Masked().String()
}

func isDefault(dst string) bool { return dst == "" || strings.HasPrefix(dst, "0.0.0.0 0.0.0.0") }

func tunnelProposal(t fortigate.Phase1, p2 []fortigate.Phase2, ips map[string]string, segs []segmentRef, firewall string) proposal {
	kind, linkType, label := "ipsec", "site_vpn", "Site-to-site"
	if t.Type == "dynamic" {
		kind, linkType, label = "dialup", "remote_vpn", "Dial-up (remote access)"
	}
	s := suggestion{Name: t.Name, LinkType: linkType, Interface: t.Name, VPNTunnel: t.Name, VPNType: "ipsec", LocalPeer: ips[t.Interface],
		DeviceText: firewall, Underlying: "interface:" + t.Interface, RemoteSubnets: []string{}, LocalSegments: []segmentRef{}}
	if t.Type != "dynamic" && t.RemoteGW != "0.0.0.0" {
		s.RemotePeer = t.RemoteGW
	}
	var notes, locals []string
	for _, x := range p2 {
		if x.Phase1 == t.Name {
			locals, notes = selectors(x, segs, &s, locals, notes)
		}
	}
	facts := nonEmpty([]fact{{"Tunnel", t.Name}, {"Kind", label}, {"Over interface", t.Interface}, {"Remote gateway", s.RemotePeer},
		{"Local subnets", strings.Join(locals, ", ")}, {"Remote subnets", strings.Join(s.RemoteSubnets, ", ")}, {"Comments", t.Comments}})
	slices.Sort(notes)
	return proposal{Key: kind + ":" + t.Name, Kind: kind, Title: t.Name, Facts: facts, Notes: slices.Compact(notes), Suggestion: s}
}

func isSubnet(kind string) bool { return kind == "" || kind == "subnet" }

// selectors takes one phase 2's local and remote subnets. Local ones that are
// segments become local subnets; named addresses are noted for people to add.
func selectors(x fortigate.Phase2, segs []segmentRef, s *suggestion, locals, notes []string) ([]string, []string) {
	if src := network(x.Src); src != "" && isSubnet(x.SrcType) {
		locals = append(locals, src)
		if i := slices.IndexFunc(segs, func(sg segmentRef) bool { return sg.Network == src }); i >= 0 {
			s.LocalSegments = append(s.LocalSegments, segs[i])
		} else {
			notes = append(notes, "Local subnet "+src+" isn't a segment in the register, so it isn't linked.")
		}
	} else if !isSubnet(x.SrcType) {
		notes = append(notes, "A phase 2 uses a named local address: add it by hand if needed.")
	}
	dst := network(x.Dst)
	switch {
	case dst != "" && isSubnet(x.DstType) && !slices.Contains(s.RemoteSubnets, dst):
		s.RemoteSubnets = append(s.RemoteSubnets, dst)
	case !isSubnet(x.DstType):
		notes = append(notes, "A phase 2 uses a named remote address: add its subnets by hand.")
	}
	return locals, notes
}

// compare finds the link already recorded for each proposal, by interface,
// WAN IP or tunnel name, and lists where the firewall now differs.
func compare(ctx context.Context, conn db.Conn, props []proposal) error {
	links, err := db.All[linkDTO](ctx, conn, linkColumns+linkFrom+" WHERE l.archived_at IS NULL ORDER BY l.code")
	if err != nil {
		return err
	}
	for i := range props {
		p := &props[i]
		p.Changes = []change{}
		j := slices.IndexFunc(links, func(l linkDTO) bool { return matches(p, l) })
		if j < 0 {
			continue
		}
		l := links[j]
		p.Existing = &linkRef{ID: l.ID, Code: l.Code, Name: l.Name, DownMbps: l.DownMbps, UpMbps: l.UpMbps}
		p.Changes = differences(p.Suggestion, l)
	}
	return nil
}

func matches(p *proposal, l linkDTO) bool {
	name := strings.ToLower(p.Suggestion.Interface)
	if p.Kind == "interface" {
		if l.VPNTunnel != "" {
			return false
		}
		return strings.ToLower(l.Interface) == name || (p.Suggestion.WANIP != "" && str(l.WANIP) == p.Suggestion.WANIP)
	}
	return strings.ToLower(l.VPNTunnel) == name || strings.ToLower(l.Interface) == name
}

// updatable are the fields the firewall knows and a confirmed update may change.
var updatable = []string{"interface", "wanIp", "gatewayIp", "vlan", "downMbps", "upMbps", "vpnTunnel", "localPeer", "remotePeer", "remoteSubnets"}

func differences(s suggestion, l linkDTO) []change {
	out := []change{}
	add := func(field string, now, before any, has bool) {
		if has && mustJSON(now) != mustJSON(before) {
			out = append(out, change{Field: field, Old: before, New: now})
		}
	}
	add("interface", s.Interface, l.Interface, s.Interface != "")
	add("wanIp", s.WANIP, str(l.WANIP), s.WANIP != "")
	add("gatewayIp", s.GatewayIP, str(l.GatewayIP), s.GatewayIP != "")
	add("vlan", s.VLAN, intOf(l.VLAN), s.VLAN != nil)
	add("downMbps", s.DownMbps, l.DownMbps, s.DownMbps != nil)
	add("upMbps", s.UpMbps, l.UpMbps, s.UpMbps != nil)
	add("vpnTunnel", s.VPNTunnel, l.VPNTunnel, s.VPNTunnel != "")
	add("localPeer", s.LocalPeer, l.LocalPeer, s.LocalPeer != "")
	add("remotePeer", s.RemotePeer, l.RemotePeer, s.RemotePeer != "")
	have := slices.Clone(l.RemoteSubnets)
	want := slices.Clone(s.RemoteSubnets)
	slices.Sort(have)
	slices.Sort(want)
	add("remoteSubnets", want, have, len(want) > 0)
	return out
}
