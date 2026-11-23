package fortigate

import (
	"context"
	"encoding/json"
	"strings"
)

// The firewall reads behind Network Links; the shapes they return are in linktypes.go.

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
