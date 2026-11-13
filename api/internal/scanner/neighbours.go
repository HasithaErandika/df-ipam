package scanner

import (
	"context"
	"net/netip"
	"sync"
)

// addNeighbours completes a sweep from the kernel's neighbour table, which
// pinging filled. Where the scanner shares the network with these hosts it
// knows their MACs, and the ARP MAC wins over NetBIOS: NetBIOS reports the
// first network card, which on a host with several is often not the one at
// this address. A host that answered ARP but nothing else (a firewall drops
// ping and every port) is still recorded, with its DNS name.
func addNeighbours(ctx context.Context, seg segment, hosts []netip.Addr, found []sighting, arp, own map[string]string) []sighting {
	seen := make(map[string]bool, len(found))
	for i := range found {
		seen[found[i].ip] = true
		mac, ok := arp[found[i].ip]
		if !ok {
			mac, ok = own[found[i].ip]
		}
		if ok {
			found[i].mac = mac
			if found[i].source == "ping" {
				found[i].source = "arp"
			}
		}
		setVendor(&found[i])
	}
	var silent []sighting
	for _, ip := range hosts {
		if mac, ok := arp[ip.String()]; ok && !seen[ip.String()] {
			silent = append(silent, sighting{ip: ip.String(), mac: mac, source: "arp", static: isStatic(seg, ip),
				gateway: ip.String() == seg.gateway})
		}
	}
	nameAll(ctx, silent)
	for i := range silent {
		setVendor(&silent[i])
	}
	return append(found, silent...)
}

// setVendor names the network card's maker from the MAC's first half. It is
// often not the device's maker (an Intel Wi-Fi card in an ASUS laptop), so it
// stands in for the device's maker only after the device had its say.
func setVendor(x *sighting) {
	x.nicVendor = macVendor(x.mac)
	if x.vendor == "" && x.full {
		x.vendor = x.nicVendor
	}
}

// nameAll looks up reverse DNS names, eight at a time.
func nameAll(ctx context.Context, items []sighting) {
	slots := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range items {
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			if ip, err := netip.ParseAddr(items[i].ip); err == nil {
				if name, _ := reverseName(ctx, ip); name != "" && name[0] != '_' && name != "localhost" {
					items[i].hostname = name
				}
			}
		}()
	}
	wg.Wait()
}
