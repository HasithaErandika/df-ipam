package links

import (
	"context"
	"time"

	"dfipam/api/internal/integrations/fortigate"
)

// take reads the firewall once. Interfaces are required; tunnels, SD-WAN,
// roles and load are optional, and a part that fails is named in Problems.
func (s *sampler) take(ctx context.Context, client *fortigate.Client) (liveView, error) {
	name, err := client.Status(ctx)
	if err != nil {
		return liveView{}, err
	}
	stats, err := client.Interfaces(ctx)
	if err != nil {
		return liveView{}, err
	}
	now := time.Now()
	v := liveView{Firewall: name, At: now, Interfaces: []liveIface{}, Tunnels: []liveTunnel{}, Health: []liveHealth{}, VPNUsers: []fortigate.VPNUser{}, Problems: []string{}}
	seconds := now.Sub(s.prevAt).Seconds()
	counters := map[string][2]uint64{}
	roles := map[string]string{}
	if cfg, err := client.InterfaceConfigs(ctx); err == nil {
		for _, c := range cfg {
			roles[c.Name] = c.Role
		}
	} else {
		v.Problems = append(v.Problems, "Interface roles: "+err.Error())
	}
	for _, x := range stats {
		key := "if:" + x.Name
		counters[key] = [2]uint64{x.RxBytes, x.TxBytes}
		i := liveIface{Name: x.Name, Alias: x.Alias, IP: x.IP, Role: roles[x.Name], Up: x.Link, SpeedMbps: x.Speed,
			RxBytes: x.RxBytes, TxBytes: x.TxBytes, Errors: x.RxErrors + x.TxErrors}
		if p, ok := s.prev[key]; ok {
			i.RxBps, i.TxBps = rate(x.RxBytes, p[0], seconds), rate(x.TxBytes, p[1], seconds)
		}
		v.Interfaces = append(v.Interfaces, i)
	}
	s.tunnels(ctx, client, &v, counters, seconds)
	s.extras(ctx, client, &v)
	sortByName(v.Interfaces, func(x liveIface) string { return x.Name })
	sortByName(v.Tunnels, func(x liveTunnel) string { return x.Name })
	s.prev, s.prevAt = counters, now
	return v, nil
}

func (s *sampler) tunnels(ctx context.Context, client *fortigate.Client, v *liveView, counters map[string][2]uint64, seconds float64) {
	tunnels, err := client.Tunnels(ctx)
	if err != nil {
		v.Problems = append(v.Problems, "IPsec tunnels: "+err.Error())
		return
	}
	for _, t := range tunnels {
		key := "vpn:" + t.Name
		counters[key] = [2]uint64{t.InBytes, t.OutBytes}
		up, all := t.Up()
		lt := liveTunnel{Name: t.Name, Gateway: t.Gateway, PhaseUp: up, Phases: all, Comments: t.Comments}
		if p, ok := s.prev[key]; ok {
			lt.RxBps, lt.TxBps = rate(t.InBytes, p[0], seconds), rate(t.OutBytes, p[1], seconds)
		}
		v.Tunnels = append(v.Tunnels, lt)
	}
}

// extras adds SD-WAN health and the firewall's load.
func (s *sampler) extras(ctx context.Context, client *fortigate.Client, v *liveView) {
	if health, err := client.Health(ctx); err == nil {
		for _, m := range health {
			v.Health = append(v.Health, liveHealth{Check: m.Check, Member: m.Member, Status: m.Status, LatencyMs: m.Latency, JitterMs: m.Jitter, LossPct: m.Loss})
		}
		sortByName(v.Health, func(x liveHealth) string { return x.Check + " " + x.Member })
	} else {
		v.Problems = append(v.Problems, "SD-WAN health: "+err.Error())
	}
	if users, err := client.VPNUsers(ctx); err == nil {
		v.VPNUsers = users
	} else {
		v.Problems = append(v.Problems, "VPN users: "+err.Error())
	}
	if use, err := client.Usage(ctx); err == nil {
		v.CPU, v.Memory, v.Sessions = use.CPU, use.Memory, use.Sessions
	} else {
		v.Problems = append(v.Problems, "CPU and memory: "+err.Error())
	}
}
