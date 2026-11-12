package scanner

import (
	"context"
	"net/netip"
	"strings"
	"time"
)

// fullEvery is how often a device that keeps answering is fully identified
// again (mDNS, UPnP and, where the segment asks, ports). Those details rarely
// change, so in between a scan only pings, reads ARP and asks DNS and NetBIOS.
const fullEvery = 6 * time.Hour

// probePlan is what one scan of a segment asks each answering host.
type probePlan struct {
	seg         segment
	hostGateway string
	portSlots   chan struct{}
	full        func(ip string) bool // whether this host is due a full identification
}

// identify pings one host and, if it answers, asks it who it is: reverse DNS and
// NetBIOS every time; mDNS (name, then model), SSDP/UPnP (maker, model, kind)
// and common ports when it is due. Every question goes to that host only and
// changes nothing on it.
func identify(ctx context.Context, p probePlan, ip netip.Addr) (sighting, bool) {
	alive, ttl, err := pingTTL(ctx, ip, 700*time.Millisecond)
	if err != nil || !alive {
		return sighting{}, false
	}
	name, _ := reverseName(ctx, ip)
	gateway := name == "_gateway" || ip.String() == p.seg.gateway || ip.String() == p.hostGateway
	if strings.HasPrefix(name, "_") || name == "localhost" {
		name = "" // synthetic names from the local resolver, such as _gateway
	}
	mac, nbName, _ := netbiosStatus(ctx, ip) // also the MAC on routed networks, so every time
	if name == "" {
		name = nbName
	}
	source := "ping"
	if mac != "" || nbName != "" {
		source = "netbios"
	}
	out := sighting{ip: ip.String(), mac: mac, hostname: name, source: source, static: isStatic(p.seg, ip), gateway: gateway, os: osFromTTL(ttl)}
	if p.full(out.ip) {
		out.full = true
		deepIdentify(ctx, p, ip, &out)
	}
	return out, true
}

// deepIdentify adds what a device says about itself, and its open ports.
func deepIdentify(ctx context.Context, p probePlan, ip netip.Addr, out *sighting) {
	if out.hostname == "" {
		out.hostname = mdnsName(ctx, ip)
		out.model = mdnsModel(ctx, ip, out.hostname)
	}
	upnp := ssdpDescribe(ctx, ip)
	if out.hostname == "" {
		out.hostname = upnp.Name
	}
	if out.model == "" {
		out.model = upnp.Model
	}
	out.vendor, out.maker, out.kind = upnp.Describe(), upnp.Maker, upnp.Kind
	if p.seg.checkPorts {
		out.ports, out.portsChecked = openPorts(ctx, ip, p.portSlots), true
	}
}
