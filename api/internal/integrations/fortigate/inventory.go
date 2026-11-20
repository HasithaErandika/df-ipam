package fortigate

import (
	"context"
	"strings"
)

// What the firewall knows that can become networks, devices and register
// entries in DF-IPAM, after people review it. GET only.

// DHCPServer is one DHCP server on an interface, with its pool and reservations.
type DHCPServer struct {
	Interface string `json:"interface"`
	Gateway   string `json:"default-gateway"`
	Netmask   string `json:"netmask"`
	Status    string `json:"status"`
	Ranges    []struct {
		Start string `json:"start-ip"`
		End   string `json:"end-ip"`
	} `json:"ip-range"`
	Reserved []struct {
		IP          string `json:"ip"`
		MAC         string `json:"mac"`
		Description string `json:"description"`
	} `json:"reserved-address"`
}

// Address is a firewall address object. A single host ("x 255.255.255.255")
// usually names a server.
type Address struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // ipmask, iprange, fqdn…
	Subnet    string `json:"subnet"`
	Comment   string `json:"comment"`
	Interface string `json:"associated-interface"`
	StartIP   string `json:"start-ip"` // iprange
	EndIP     string `json:"end-ip"`
}

// VIP publishes an internal server on an outside address.
type VIP struct {
	Name     string `json:"name"`
	ExtIP    string `json:"extip"`
	ExtIntf  string `json:"extintf"`
	Comment  string `json:"comment"`
	MappedIP []struct {
		Range string `json:"range"`
	} `json:"mappedip"`
}

// Detected is a device the firewall identified itself: name, system and kind.
type Detected struct {
	MAC       string `json:"mac"`
	IP        string `json:"ipv4_address"`
	Hostname  string `json:"hostname"`
	OS        string `json:"os_name"`
	OSVersion string `json:"os_version"`
	Vendor    string `json:"hardware_vendor"`
	Type      string `json:"hardware_type"`
	Interface string `json:"detected_interface"`
	Online    bool   `json:"is_online"`
}

// DHCPServers reads the firewall's DHCP servers.
func (c *Client) DHCPServers(ctx context.Context) ([]DHCPServer, error) {
	var out []DHCPServer
	return out, c.get(ctx, "/api/v2/cmdb/system.dhcp/server", &out)
}

// Addresses reads the firewall's address objects.
func (c *Client) Addresses(ctx context.Context) ([]Address, error) {
	var out []Address
	return out, c.get(ctx, "/api/v2/cmdb/firewall/address", &out)
}

// VIPs reads the firewall's virtual IPs.
func (c *Client) VIPs(ctx context.Context) ([]VIP, error) {
	var out []VIP
	return out, c.get(ctx, "/api/v2/cmdb/firewall/vip", &out)
}

// Devices reads the firewall's device identification list. FortiOS 7 answers
// on device/query; older versions on device/select.
func (c *Client) Devices(ctx context.Context) ([]Detected, error) {
	var out []Detected
	if err := c.get(ctx, "/api/v2/monitor/user/device/query", &out); err == nil {
		return out, nil
	}
	return out, c.get(ctx, "/api/v2/monitor/user/device/select", &out)
}

// Known reads the ARP table and DHCP leases, as Discovery does.
func (c *Client) Known(ctx context.Context) ([]Entry, error) {
	entries, _, err := c.ReadAll(ctx)
	return entries, err
}

// VPNUser is one person connected over VPN right now.
type VPNUser struct {
	User     string `json:"user"`
	Kind     string `json:"kind"` // ssl, ipsec
	Tunnel   string `json:"tunnel"`
	From     string `json:"from"`
	TunnelIP string `json:"tunnelIp"`
	Since    string `json:"since"`
}

// VPNUsers reads the SSL VPN sessions and dial-up IPsec tunnels with a user.
// Either may be missing on a firewall that doesn't run that kind of VPN.
func (c *Client) VPNUsers(ctx context.Context) ([]VPNUser, error) {
	var ssl []struct {
		sslSession
		LastLogin string `json:"last_login_time"`
	}
	sslErr := c.get(ctx, "/api/v2/monitor/vpn/ssl", &ssl)
	var ipsec []ipsecTunnel
	ipsecErr := c.get(ctx, "/api/v2/monitor/vpn/ipsec", &ipsec)
	if sslErr != nil && ipsecErr != nil {
		return nil, sslErr
	}
	out := []VPNUser{}
	for _, s := range ssl {
		u := VPNUser{User: s.User, Kind: "ssl", From: s.RemoteHost, Since: s.LastLogin}
		if len(s.Subsessions) > 0 {
			u.TunnelIP = s.Subsessions[0].AIP
		}
		out = append(out, u)
	}
	for _, t := range ipsec {
		if t.User == "" {
			continue
		}
		u := VPNUser{User: t.User, Kind: "ipsec", Tunnel: t.Name, From: t.Gateway}
		if len(t.ProxyIDs) > 0 && len(t.ProxyIDs[0].Dst) > 0 {
			u.TunnelIP = strings.SplitN(t.ProxyIDs[0].Dst[0].Subnet, "-", 2)[0]
		}
		out = append(out, u)
	}
	return out, nil
}

// SSLPools names the address objects SSL VPN users get their addresses from.
func (c *Client) SSLPools(ctx context.Context) ([]string, error) {
	var settings struct {
		Pools []struct {
			Name string `json:"name"`
		} `json:"tunnel-ip-pools"`
	}
	if err := c.get(ctx, "/api/v2/cmdb/vpn.ssl/settings", &settings); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(settings.Pools))
	for _, p := range settings.Pools {
		out = append(out, p.Name)
	}
	return out, nil
}
