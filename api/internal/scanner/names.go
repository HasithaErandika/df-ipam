package scanner

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// mdnsName asks one host for its own name with a unicast mDNS query (RFC 6762,
// section 6.7). Sent from an ordinary port, the host answers straight back to
// the asker, so this works through NAT and sends no multicast. iPhones, Macs,
// many Android phones, printers and Linux machines with Avahi answer it.
func mdnsName(parent context.Context, ip netip.Addr) string {
	b := ip.As4()
	return parsePTR(mdnsAsk(parent, ip, fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.", b[3], b[2], b[1], b[0]), dnsmessage.TypePTR))
}

// mdnsAsk sends one unicast mDNS question to the host and returns its raw answer.
func mdnsAsk(parent context.Context, ip netip.Addr, question string, typ dnsmessage.Type) []byte {
	ctx, cancel := context.WithTimeout(parent, 700*time.Millisecond)
	defer cancel()
	name, err := dnsmessage.NewName(question)
	if err != nil {
		return nil
	}
	query, err := (&dnsmessage.Message{
		Header:    dnsmessage.Header{ID: uint16(time.Now().UnixNano())},
		Questions: []dnsmessage.Question{{Name: name, Type: typ, Class: dnsmessage.ClassINET}},
	}).Pack()
	if err != nil {
		return nil
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp4", net.JoinHostPort(ip.String(), "5353"))
	if err != nil {
		return nil
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(query); err != nil {
		return nil
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return nil
	}
	return buf[:n]
}

// parsePTR returns the first PTR answer, without the trailing dot or ".local".
func parsePTR(packet []byte) string {
	var p dnsmessage.Parser
	if _, err := p.Start(packet); err != nil {
		return ""
	}
	if err := p.SkipAllQuestions(); err != nil {
		return ""
	}
	for {
		h, err := p.AnswerHeader()
		if err != nil {
			return ""
		}
		if h.Type != dnsmessage.TypePTR {
			if err := p.SkipAnswer(); err != nil {
				return ""
			}
			continue
		}
		r, err := p.PTRResource()
		if err != nil {
			return ""
		}
		name := strings.TrimSuffix(r.PTR.String(), ".")
		return strings.TrimSuffix(name, ".local")
	}
}

// arpTable reads the kernel's neighbour table. It only holds MACs when the
// scanner shares a network with the hosts: on a trunk port, or with host
// networking. Behind Docker's NAT it holds only the containers, which no
// segment contains, so nothing is wrongly attributed.
func arpTable() map[string]string {
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	s.Scan() // header
	for s.Scan() {
		fields := strings.Fields(s.Text())
		// IP, HW type, flags, MAC, mask, device. Flag 0x0 is an incomplete entry.
		if len(fields) < 6 || fields[2] == "0x0" || fields[3] == "00:00:00:00:00:00" {
			continue
		}
		out[fields[0]] = strings.ToLower(fields[3])
	}
	return out
}

// defaultGateway is the gateway of the default route the scanner uses, from
// /proc/net/route. With host networking it is the site's router; inside a
// Docker bridge it is Docker's own gateway, which no segment contains.
func defaultGateway() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		var v uint32
		if _, err := fmt.Sscanf(fields[2], "%08X", &v); err != nil || v == 0 {
			continue
		}
		// The kernel writes the address in host (little-endian) byte order.
		return netip.AddrFrom4([4]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}).String()
	}
	return ""
}

// ownMACs maps this machine's own IPv4 addresses to their MACs. The kernel's
// ARP table never lists the machine itself, so the DF-IPAM server would
// otherwise appear without one.
func ownMACs() map[string]string {
	out := map[string]string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if len(iface.HardwareAddr) != 6 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().Is4() {
				out[p.Addr().String()] = iface.HardwareAddr.String()
			}
		}
	}
	return out
}
