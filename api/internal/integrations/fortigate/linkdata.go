package fortigate

import (
	"context"
	"encoding/json"
	"strings"
)

// What Network Links reads from the firewall: live counters (monitor) and the
// configured WAN interfaces, routes and IPsec tunnels (cmdb). GET only. The
// API token's admin profile needs read access to System, Network (router) and
// VPN. Field names are FortiOS's.

// IfaceStat is one interface's live state and byte counters.
type IfaceStat struct {
	Name     string `json:"name"`
	Alias    string `json:"alias"`
	IP       string `json:"ip"`
	Mask     int    `json:"mask"`
	Link     bool   `json:"link"`
	Speed    int    `json:"speed"` // Mbps
	TxBytes  uint64 `json:"tx_bytes"`
	RxBytes  uint64 `json:"rx_bytes"`
	TxErrors uint64 `json:"tx_errors"`
	RxErrors uint64 `json:"rx_errors"`
}

// TunnelStat is one IPsec tunnel's live state.
type TunnelStat struct {
	Name     string `json:"name"`
	Gateway  string `json:"rgwy"`
	InBytes  uint64 `json:"incoming_bytes"`
	OutBytes uint64 `json:"outgoing_bytes"`
	Comments string `json:"comments"`
	ProxyIDs []struct {
		Status string `json:"status"`
	} `json:"proxyid"`
}

// HealthMember is one SD-WAN member's measurements in one health check.
type HealthMember struct {
	Check, Member string
	Status        string  `json:"status"`
	Latency       float64 `json:"latency"`
	Jitter        float64 `json:"jitter"`
	Loss          float64 `json:"packet_loss"`
}

// IfaceConfig is a configured interface.
type IfaceConfig struct {
	Name     string `json:"name"`
	Alias    string `json:"alias"`
	Role     string `json:"role"`
	Type     string `json:"type"`
	Mode     string `json:"mode"` // static, dhcp, pppoe
	IP       string `json:"ip"`   // "203.0.113.2 255.255.255.252"
	VLAN     int    `json:"vlanid"`
	Parent   string `json:"interface"`
	Status   string `json:"status"`
	UpKbps   int    `json:"estimated-upstream-bandwidth"`
	DownKbps int    `json:"estimated-downstream-bandwidth"`
	VDOM     string `json:"vdom"`
	Desc     string `json:"description"`
}

// Phase1 is an IPsec tunnel's phase 1.
type Phase1 struct {
	Name      string `json:"name"`
	Interface string `json:"interface"`
	RemoteGW  string `json:"remote-gw"`
	Type      string `json:"type"` // static, dynamic (dial-up), ddns
	Comments  string `json:"comments"`
	ModeCfg   string `json:"mode-cfg"` // enable: dial-up users get addresses from StartIP to EndIP
	StartIP   string `json:"ipv4-start-ip"`
	EndIP     string `json:"ipv4-end-ip"`
}

// Phase2 is one phase 2 selector of a tunnel.
type Phase2 struct {
	Phase1  string `json:"phase1name"`
	SrcType string `json:"src-addr-type"`
	Src     string `json:"src-subnet"`
	DstType string `json:"dst-addr-type"`
	Dst     string `json:"dst-subnet"`
}

// Route is a static route.
type Route struct {
	Dst     string `json:"dst"`
	Gateway string `json:"gateway"`
	Device  string `json:"device"`
	Status  string `json:"status"`
}

// Resources are the firewall's current CPU and memory use (%) and sessions.
type Resources struct {
	CPU, Memory, Sessions *int
}

// Interfaces reads every interface's live counters, VLANs included.
func (c *Client) Interfaces(ctx context.Context) ([]IfaceStat, error) {
	var byName map[string]IfaceStat
	if err := c.get(ctx, "/api/v2/monitor/system/interface?include_vlan=true&include_aggregate=true", &byName); err != nil {
		return nil, err
	}
	out := make([]IfaceStat, 0, len(byName))
	for name, s := range byName {
		if s.Name == "" {
			s.Name = name
		}
		out = append(out, s)
	}
	return out, nil
}

// Tunnels reads the IPsec tunnels' live state.
func (c *Client) Tunnels(ctx context.Context) ([]TunnelStat, error) {
	var out []TunnelStat
	return out, c.get(ctx, "/api/v2/monitor/vpn/ipsec", &out)
}

// Health reads SD-WAN health checks. Firewalls without SD-WAN return none.
func (c *Client) Health(ctx context.Context) ([]HealthMember, error) {
	var raw map[string]map[string]HealthMember
	if err := c.get(ctx, "/api/v2/monitor/virtual-wan/health-check", &raw); err != nil {
		return nil, err
	}
	var out []HealthMember
	for check, members := range raw {
		for member, m := range members {
			m.Check, m.Member = check, member
			out = append(out, m)
		}
	}
	return out, nil
}

// Usage reads current CPU, memory and session counts.
func (c *Client) Usage(ctx context.Context) (Resources, error) {
	var raw map[string][]struct {
		Current *int `json:"current"`
	}
	if err := c.get(ctx, "/api/v2/monitor/system/resource/usage?resource=cpu,mem,session&interval=1-min", &raw); err != nil {
		return Resources{}, err
	}
	first := func(k string) *int {
		if len(raw[k]) > 0 {
			return raw[k][0].Current
		}
		return nil
	}
	return Resources{CPU: first("cpu"), Memory: first("mem"), Sessions: first("session")}, nil
}

// InterfaceConfigs reads the configured interfaces.
func (c *Client) InterfaceConfigs(ctx context.Context) ([]IfaceConfig, error) {
	var out []IfaceConfig
	return out, c.get(ctx, "/api/v2/cmdb/system/interface", &out)
}

// IPsec reads every tunnel's phase 1 and phase 2 settings.
func (c *Client) IPsec(ctx context.Context) ([]Phase1, []Phase2, error) {
	var p1 []Phase1
	var p2 []Phase2
	if err := c.get(ctx, "/api/v2/cmdb/vpn.ipsec/phase1-interface", &p1); err != nil {
		return nil, nil, err
	}
	return p1, p2, c.get(ctx, "/api/v2/cmdb/vpn.ipsec/phase2-interface", &p2)
}

// Routes reads the static routes.
func (c *Client) Routes(ctx context.Context) ([]Route, error) {
	var out []Route
	return out, c.get(ctx, "/api/v2/cmdb/router/static", &out)
}

// Up says whether any of the tunnel's phase 2 selectors is up, and how many are.
func (t TunnelStat) Up() (int, int) {
	up := 0
	for _, p := range t.ProxyIDs {
		if strings.EqualFold(p.Status, "up") {
			up++
		}
	}
	return up, len(t.ProxyIDs)
}

// UnmarshalJSON accepts the link flag as a boolean or "up".
func (s *IfaceStat) UnmarshalJSON(b []byte) error {
	type plain IfaceStat
	var raw struct {
		plain
		Link json.RawMessage `json:"link"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*s = IfaceStat(raw.plain)
	v := strings.Trim(string(raw.Link), `"`)
	s.Link = v == "true" || strings.EqualFold(v, "up")
	return nil
}
