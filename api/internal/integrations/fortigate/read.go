package fortigate

import (
	"context"
	"net/netip"
	"strings"
)

// Entry is one IP the firewall knows about, as evidence for sightings.
type Entry struct {
	IP, MAC, Hostname, Vendor, Source string
}

type arpEntry struct {
	IP  string `json:"ip"`
	MAC string `json:"mac"`
}

type lease struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Hostname string `json:"hostname"`
	VCI      string `json:"vci"` // DHCP vendor class, such as "MSFT 5.0" or "android-dhcp-14"
	Status   string `json:"status"`
}

type sslSession struct {
	User        string `json:"user_name"`
	RemoteHost  string `json:"remote_host"`
	Subsessions []struct {
		AIP string `json:"aip"`
	} `json:"subsessions"`
}

type ipsecTunnel struct {
	Name     string `json:"name"`
	Gateway  string `json:"rgwy"`
	User     string `json:"xauth_user"`
	ProxyIDs []struct {
		Status string `json:"status"`
		Dst    []struct {
			Subnet string `json:"subnet"`
		} `json:"proxy_dst"`
	} `json:"proxyid"`
}

// Counts says how much each source returned in the last read.
type Counts struct {
	ARP  int `json:"arp"`
	DHCP int `json:"dhcp"`
	VPN  int `json:"vpn"`
}

// ReadAll collects ARP entries, DHCP leases and VPN sessions. ARP and DHCP are
// required; VPN endpoints are optional, since not every firewall runs both kinds.
func (c *Client) ReadAll(ctx context.Context) ([]Entry, Counts, error) {
	var counts Counts
	var arp []arpEntry
	if err := c.get(ctx, "/api/v2/monitor/system/arp", &arp); err != nil {
		return nil, counts, err
	}
	var leases []lease
	if err := c.get(ctx, "/api/v2/monitor/system/dhcp", &leases); err != nil {
		return nil, counts, err
	}
	var ssl []sslSession
	_ = c.get(ctx, "/api/v2/monitor/vpn/ssl", &ssl)
	var ipsec []ipsecTunnel
	_ = c.get(ctx, "/api/v2/monitor/vpn/ipsec", &ipsec)

	var out []Entry
	for _, a := range arp {
		if ok4(a.IP) {
			out = append(out, Entry{IP: a.IP, MAC: a.MAC, Source: "arp"})
			counts.ARP++
		}
	}
	for _, l := range leases {
		if ok4(l.IP) && (l.Status == "" || l.Status == "leased") {
			out = append(out, Entry{IP: l.IP, MAC: l.MAC, Hostname: l.Hostname, Vendor: l.VCI, Source: "dhcp"})
			counts.DHCP++
		}
	}
	for _, s := range ssl {
		for _, sub := range s.Subsessions {
			if ok4(sub.AIP) {
				out = append(out, Entry{IP: sub.AIP, Hostname: vpnName("SSL VPN", s.User, s.RemoteHost), Source: "vpn"})
				counts.VPN++
			}
		}
	}
	for _, t := range ipsec {
		for _, p := range t.ProxyIDs {
			for _, d := range p.Dst {
				if ip := singleHost(d.Subnet); ip != "" && p.Status == "up" {
					user := t.User
					if user == "" {
						user = t.Name
					}
					out = append(out, Entry{IP: ip, Hostname: vpnName("IPsec", user, t.Gateway), Source: "vpn"})
					counts.VPN++
				}
			}
		}
	}
	return out, counts, nil
}

func ok4(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4() && !a.IsUnspecified()
}

func vpnName(kind, user, from string) string {
	name := kind
	if user != "" {
		name += ": " + user
	}
	if from != "" {
		name += " from " + from
	}
	return name
}

// singleHost reads "10.0.0.5-10.0.0.5" or "10.0.0.5/32" as one address.
func singleHost(subnet string) string {
	a, b, found := strings.Cut(subnet, "-")
	if found && a == b && ok4(a) {
		return a
	}
	if p, err := netip.ParsePrefix(subnet); err == nil && p.Bits() == 32 && p.Addr().Is4() {
		return p.Addr().String()
	}
	return ""
}
