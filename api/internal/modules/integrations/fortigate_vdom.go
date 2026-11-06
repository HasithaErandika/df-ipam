package integrations

import (
	"context"
	"fmt"
	"time"
)

// fortiTable reads an optional table: a token limited to some tables still produces
// the rest, and the gap shows as a warning.
func fortiTable[T any](ctx context.Context, f *fortiGate, r *Result, code, vdom, path string, add func(T)) {
	items, err := fortiList[T](ctx, f, path, vdom)
	if err != nil {
		r.warn(code, vdomContext(vdom), err)
		return
	}
	for _, item := range items {
		add(item)
	}
}

func (f *fortiGate) collectVDOM(ctx context.Context, r *Result, dev *Device, vdom string, now time.Time) {
	context_ := vdomContext(vdom)
	fortiTable(ctx, f, r, "ADDRESSES_UNAVAILABLE", vdom, "/api/v2/cmdb/firewall/address", func(a fortiAddress) {
		r.AddressObjects = append(r.AddressObjects, addressObject(a, dev.ExternalID, vdom))
	})
	fortiTable(ctx, f, r, "GROUPS_UNAVAILABLE", vdom, "/api/v2/cmdb/firewall/addrgrp", func(g fortiGroup) {
		grp := AddressGroup{DeviceExternalID: dev.ExternalID, VDOM: vdom, Name: g.Name, Comment: g.Comment}
		for _, m := range g.Member {
			grp.Members = append(grp.Members, m.Name)
		}
		r.AddressGroups = append(r.AddressGroups, grp)
	})
	fortiTable(ctx, f, r, "ROUTES_UNAVAILABLE", vdom, "/api/v2/cmdb/router/static", func(s fortiRoute) {
		if _, p, ok := splitIPMask(s.Dst); ok {
			r.Routes = append(r.Routes, Route{Context: context_, DeviceExternalID: dev.ExternalID, VDOM: vdom, Destination: p.Masked().String(),
				NextHop: s.Gateway, Interface: s.Device, Type: "static"})
		}
	})
	fortiTable(ctx, f, r, "ARP_UNAVAILABLE", vdom, "/api/v2/monitor/network/arp", func(a fortiARP) {
		r.Observations = append(r.Observations, Observation{Source: SourceFortiGate, Context: context_, Address: a.IP, MAC: a.MAC, Alive: true, ObservedAt: now,
			Payload: map[string]any{"table": "arp", "interface": a.Interface, "vdom": vdom}})
	})
	fortiTable(ctx, f, r, "DHCP_SCOPES_UNAVAILABLE", vdom, "/api/v2/cmdb/system.dhcp/server", func(d fortiDHCPServer) {
		for _, ip := range d.IPRange {
			r.DHCPScopes = append(r.DHCPScopes, DHCPScope{Context: context_, DeviceExternalID: dev.ExternalID,
				Name: fmt.Sprintf("%s (%d)", d.Interface, d.ID), RangeStart: ip.Start, RangeEnd: ip.End})
		}
	})
	fortiTable(ctx, f, r, "DHCP_LEASES_UNAVAILABLE", vdom, "/api/v2/monitor/system/dhcp", func(l fortiLease) {
		r.Observations = append(r.Observations, Observation{Source: SourceDHCP, Context: context_, Address: l.IP, MAC: l.MAC, Hostname: l.Hostname,
			Alive: l.Status != "expired", ObservedAt: now, Payload: map[string]any{"interface": l.Interface, "status": l.Status, "vdom": vdom}})
	})
}

func addressObject(a fortiAddress, device, vdom string) AddressObject {
	o := AddressObject{DeviceExternalID: device, VDOM: vdom, Name: a.Name, Type: a.Type, Comment: a.Comment}
	switch a.Type {
	case "iprange":
		o.Value = a.StartIP + "-" + a.EndIP
	case "fqdn":
		o.Value = a.FQDN
	default:
		o.Value = a.Subnet
		if _, p, ok := splitIPMask(a.Subnet); ok {
			o.Network, o.Value = p.Masked().String(), p.Masked().String()
		}
	}
	return o
}
