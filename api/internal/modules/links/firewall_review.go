package links

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// suggestion is what the firewall says about a link: technical fields only.
// People add the business details (name, purpose, site, role) on review.
type suggestion struct {
	Name          string       `json:"name"`
	LinkType      string       `json:"linkType"`
	Interface     string       `json:"interface"`
	WANIP         string       `json:"wanIp"`
	GatewayIP     string       `json:"gatewayIp"`
	VLAN          *int         `json:"vlan"`
	DownMbps      *float64     `json:"downMbps"`
	UpMbps        *float64     `json:"upMbps"`
	DeviceText    string       `json:"deviceText"`
	VRF           string       `json:"vrf"`
	Routing       string       `json:"routing"`
	VPNTunnel     string       `json:"vpnTunnel"`
	VPNType       string       `json:"vpnType"`
	LocalPeer     string       `json:"localPeer"`
	RemotePeer    string       `json:"remotePeer"`
	RemoteSubnets []string     `json:"remoteSubnets"`
	LocalSegments []segmentRef `json:"localSegments"`
	Underlying    string       `json:"underlying"` // the key of the interface a tunnel runs over
}

// proposal is one interface or tunnel on the firewall, new or already recorded.
type proposal struct {
	Key        string     `json:"key"`
	Kind       string     `json:"kind"` // interface, ipsec, dialup
	Title      string     `json:"title"`
	Facts      []fact     `json:"facts"`
	Notes      []string   `json:"notes"`
	Suggestion suggestion `json:"suggestion"`
	Existing   *linkRef   `json:"existing"`
	Changes    []change   `json:"changes"`
}

type review struct {
	Firewall  string     `json:"firewall"`
	At        time.Time  `json:"at"`
	Proposals []proposal `json:"proposals"`
}

// firewallReview reads the firewall's WAN interfaces and IPsec tunnels and
// proposes links. Nothing is saved: people confirm what to add or update.
func (h *Handler) firewallReview(c *gin.Context) {
	client, ok := h.firewallClient(c)
	if !ok {
		return
	}
	r, err := buildReview(c, h.conn, client)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, r)
}

func unreachable(err error) error {
	return httpx.NewError(http.StatusBadGateway, "FIREWALL_UNREACHABLE", "The FortiGate didn't answer: "+err.Error()+".")
}

func buildReview(ctx context.Context, conn db.Conn, client *fortigate.Client) (review, error) {
	name, err := client.Status(ctx)
	if err != nil {
		return review{}, unreachable(err)
	}
	ifaces, err := client.InterfaceConfigs(ctx)
	if err != nil {
		return review{}, unreachable(err)
	}
	routes, err := client.Routes(ctx)
	if err != nil {
		return review{}, unreachable(err)
	}
	p1, p2, err := client.IPsec(ctx)
	if err != nil {
		return review{}, unreachable(err)
	}
	segs, err := db.All[segmentRef](ctx, conn, `SELECT id::text AS id, name, network::text AS network FROM segments`)
	if err != nil {
		return review{}, err
	}
	out := review{Firewall: name, At: time.Now(), Proposals: []proposal{}}
	ips := map[string]string{}
	for _, x := range ifaces {
		addr, _ := ipAndPrefix(x.IP)
		ips[x.Name] = addr
		if isWAN(x) {
			out.Proposals = append(out.Proposals, interfaceProposal(x, routes, name))
		}
	}
	for _, t := range p1 {
		out.Proposals = append(out.Proposals, tunnelProposal(t, p2, ips, segs, name))
	}
	return out, compare(ctx, conn, out.Proposals)
}

// isWAN keeps interfaces that carry a link: the WAN role, not tunnels or loopbacks.
func isWAN(x fortigate.IfaceConfig) bool {
	return strings.EqualFold(x.Role, "wan") && x.Type != "tunnel" && x.Type != "loopback"
}

func interfaceProposal(x fortigate.IfaceConfig, routes []fortigate.Route, firewall string) proposal {
	addr, prefix := ipAndPrefix(x.IP)
	s := suggestion{Name: firstOf(x.Alias, x.Desc, x.Name), LinkType: "ill", Interface: x.Name, DeviceText: firewall, RemoteSubnets: []string{}, LocalSegments: []segmentRef{}}
	if strings.EqualFold(x.Mode, "pppoe") {
		s.LinkType = "broadband"
	}
	if strings.EqualFold(x.Mode, "static") && addr != "" {
		s.WANIP = addr
	}
	if x.VLAN > 0 {
		v := x.VLAN
		s.VLAN = &v
	}
	if x.VDOM != "" && x.VDOM != "root" {
		s.VRF = x.VDOM
	}
	s.DownMbps, s.UpMbps = kbps(x.DownKbps), kbps(x.UpKbps)
	for _, r := range routes {
		if r.Device == x.Name && isDefault(r.Dst) && r.Status != "disable" && r.Gateway != "0.0.0.0" {
			s.GatewayIP, s.Routing = r.Gateway, "static"
		}
	}
	facts := []fact{{"Interface", x.Name}, {"Alias", x.Alias}, {"Addressing", x.Mode}, {"Address", strings.TrimSuffix(addr+prefix, "/")},
		{"Default gateway", s.GatewayIP}, {"VLAN", vlanText(x.VLAN, x.Parent)}, {"Estimated bandwidth", bwText(s.DownMbps, s.UpMbps)},
		{"Admin status", x.Status}, {"Description", x.Desc}}
	p := proposal{Key: "interface:" + x.Name, Kind: "interface", Title: s.Name, Facts: nonEmpty(facts), Notes: []string{}, Suggestion: s}
	if s.WANIP == "" && addr == "" {
		p.Notes = append(p.Notes, "The address comes from the provider by "+firstOf(x.Mode, "DHCP")+", so no WAN IP is filled in.")
	}
	return p
}

func kbps(n int) *float64 {
	if n <= 0 {
		return nil
	}
	v := float64(n) / 1000
	return &v
}

func vlanText(vlan int, parent string) string {
	if vlan <= 0 {
		return ""
	}
	return fmt.Sprintf("%d on %s", vlan, parent)
}

func bwText(down, up *float64) string {
	if down == nil && up == nil {
		return ""
	}
	f := func(p *float64) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprintf("%g Mbps", *p)
	}
	return f(down) + " down / " + f(up) + " up"
}

func nonEmpty(facts []fact) []fact {
	out := []fact{}
	for _, f := range facts {
		if strings.TrimSpace(f.Value) != "" {
			out = append(out, f)
		}
	}
	return out
}
