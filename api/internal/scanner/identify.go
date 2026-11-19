package scanner

import (
	"context"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"
)

// fullEvery is how often a device that keeps answering is fully identified
// again (mDNS, UPnP and, where the segment asks, ports). Those details rarely
// change, so in between a scan only pings, reads ARP and asks DNS and NetBIOS.
const fullEvery = 6 * time.Hour

// probePlan is what one scan of a segment asks each address.
type probePlan struct {
	seg         segment
	hostGateway string
	pinger      *pinger
	portSlots   chan struct{}
	tcpTick     <-chan time.Time // paces every TCP connection attempt in the scan
	due         func(ip string) bool
	done        func(ip string)
	sendErrors  *atomic.Int64
	firstError  *atomic.Value
}

// identify finds out whether one address is in use and, if so, asks it who it
// is: reverse DNS and NetBIOS every time; mDNS (name, then model), SSDP/UPnP
// (maker, model, kind) and common ports when it is due. Every question goes to
// that host only and changes nothing on it.
func identify(ctx context.Context, p probePlan, ip netip.Addr) (sighting, bool) {
	alive, ttl, err := p.pinger.ping(ctx, ip, 700*time.Millisecond)
	if err != nil {
		p.sendFailed(err)
	}
	var mac, nbName string
	nbDone := false
	if !alive {
		// Windows blocks ping by default, and so do many servers. NetBIOS and a
		// few TCP ports find them. Only a completed connection counts: a router
		// can refuse on an absent host's behalf (see dial).
		mac, nbName, _ = netbiosStatus(ctx, ip)
		nbDone = true
		alive = mac != "" || nbName != "" || answersTCP(ctx, p, ip)
	}
	if !alive {
		return sighting{}, false
	}
	name, _ := reverseName(ctx, ip)
	gateway := name == "_gateway" || ip.String() == p.seg.gateway || ip.String() == p.hostGateway
	if strings.HasPrefix(name, "_") || name == "localhost" {
		name = "" // synthetic names from the local resolver, such as _gateway
	}
	if !nbDone {
		mac, nbName, _ = netbiosStatus(ctx, ip) // also the MAC on routed networks, so every time
	}
	if name == "" {
		name = nbName
	}
	source := "ping"
	if mac != "" || nbName != "" {
		source = "netbios"
	}
	out := sighting{ip: ip.String(), mac: mac, hostname: name, source: source, static: isStatic(p.seg, ip), gateway: gateway, os: osFromTTL(ttl)}
	if p.due(out.ip) {
		out.full = true
		deepIdentify(ctx, p, ip, &out)
		if ctx.Err() == nil {
			p.done(out.ip) // only a finished identification waits fullEvery
		}
	}
	return out, true
}

func (p probePlan) sendFailed(err error) {
	if p.sendErrors.Add(1) == 1 {
		p.firstError.Store(err.Error())
	}
}

// deepIdentify adds what a device says about itself, and its open ports.
func deepIdentify(ctx context.Context, p probePlan, ip netip.Addr, out *sighting) {
	// Asked even when DNS gave a name: the mDNS name is what the device-info
	// record (an Apple device's model) is published under.
	if local := mdnsName(ctx, ip); local != "" {
		if out.hostname == "" {
			out.hostname = local
		}
		out.model = mdnsModel(ctx, ip, local)
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
		out.ports, out.portsChecked = openPorts(ctx, ip, p.portSlots, p.tcpTick), true
	}
}
