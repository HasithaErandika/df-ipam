package scanner

import (
	"context"
	"log/slog"
	"net/netip"
	"slices"
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
	pace        *pace // one turn per address, shared across the scanner
	portSlots   chan struct{}
	tcpTick     <-chan time.Time // paces every TCP connection attempt in the scan
	due         func(ip string) bool
	portsDue    func(ip string) bool
	done        func(ip string, ports bool)
	sendErrors  *atomic.Int64
	firstError  *atomic.Value
	// An address that doesn't answer ping is asked again (NetBIOS, then a few
	// TCP ports) only on server segments, where something was seen in the last
	// 30 days or is registered (known), or on the hourly full sweep (silentAll).
	// Otherwise an empty address costs one ping.
	known     map[string]bool
	silentAll bool
}

// askSilent reports whether an address that ignored ping is worth more questions.
func (p probePlan) askSilent(ip string) bool {
	return p.silentAll || p.seg.kind == "server" || p.known[ip]
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
	var nb nbInfo
	nbDone := false
	if !alive && !p.askSilent(ip.String()) {
		return sighting{}, false
	}
	if !alive {
		// Windows blocks ping by default, and so do many servers. NetBIOS and a
		// few TCP ports find them. Only a completed connection counts: a router
		// can refuse on an absent host's behalf (see dial).
		nb, _ = netbiosStatus(ctx, ip)
		nbDone = true
		alive = nb.answered() || answersTCP(ctx, p, ip)
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
		nb, _ = netbiosStatus(ctx, ip) // also the MAC on routed networks, so every time
	}
	if name == "" {
		name = nb.name
	}
	source := "ping"
	if nb.answered() {
		source = "netbios"
	}
	out := sighting{ip: ip.String(), mac: nb.mac, hostname: name, source: source, static: isStatic(p.seg, ip), gateway: gateway,
		os: osFromTTL(ttl), workgroup: nb.group}
	if p.due(out.ip) {
		out.full = true
		deepIdentify(ctx, p, ip, &out)
		if ctx.Err() == nil {
			p.done(out.ip, out.portsChecked) // only a finished identification waits fullEvery
		}
	}
	return out, true
}

// slowAfter is how many pings may fail to send before the scan halves its rate:
// the network, or this host's buffers, can't keep up.
const slowAfter = 5

func (p probePlan) sendFailed(err error) {
	n := p.sendErrors.Add(1)
	if n == 1 {
		p.firstError.Store(err.Error())
	}
	if n == slowAfter && p.pace.slowDown() {
		slog.Warn("pings are failing to send; halving the scan rate until the next scan", "segment", p.seg.name, "error", err.Error())
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
	if p.seg.checkPorts && p.portsDue(out.ip) {
		out.ports, out.portsChecked = openPorts(ctx, ip, p.portSlots, p.tcpTick), true
	}
	// Only hosts that look like Windows (they answered NetBIOS, or have file
	// sharing open) are asked for their Windows details, paced like any connection.
	if out.source == "netbios" || slices.Contains(out.ports, 445) {
		select {
		case <-ctx.Done():
			return
		case <-p.tcpTick:
		}
		out.windows = smbInfo(ctx, ip)
		if out.hostname == "" {
			out.hostname = out.windows.fqdn
		}
	}
}
