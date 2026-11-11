package scanner

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func ping(ctx context.Context, ip netip.Addr, timeout time.Duration) (bool, error) {
	alive, _, err := pingTTL(ctx, ip, timeout)
	return alive, err
}

// pingTTL also returns the TTL of the reply, or 0 when the system won't say. The
// starting TTL hints at the operating system (see osFromTTL).
func pingTTL(ctx context.Context, ip netip.Addr, timeout time.Duration) (bool, int, error) {
	conn, network, err := openICMP()
	if err != nil {
		return false, 0, err
	}
	defer conn.Close()
	message := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: int(time.Now().UnixNano() & 0xffff), Data: []byte("df-ipam")}}
	packet, err := message.Marshal(nil)
	if err != nil {
		return false, 0, err
	}
	var dst net.Addr = &net.IPAddr{IP: ip.AsSlice()}
	if network == "udp4" {
		dst = &net.UDPAddr{IP: ip.AsSlice()}
	}
	if _, err := conn.WriteTo(packet, dst); err != nil {
		return false, 0, nil
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetReadDeadline(deadline)
	p4 := conn.IPv4PacketConn()
	withTTL := p4 != nil && p4.SetControlMessage(ipv4.FlagTTL, true) == nil
	buf := make([]byte, 1500)
	for {
		n, ttl, peer, err := readReply(conn, p4, withTTL, buf)
		if err != nil {
			return false, 0, nil
		}
		msg, err := icmp.ParseMessage(1, buf[:n])
		if err == nil && msg.Type == ipv4.ICMPTypeEchoReply && peerIs(peer, ip) {
			return true, ttl, nil
		}
	}
}

func readReply(conn *icmp.PacketConn, p4 *ipv4.PacketConn, withTTL bool, buf []byte) (int, int, net.Addr, error) {
	if !withTTL {
		n, peer, err := conn.ReadFrom(buf)
		return n, 0, peer, err
	}
	n, cm, peer, err := p4.ReadFrom(buf)
	if cm == nil {
		return n, 0, peer, err
	}
	return n, cm.TTL, peer, err
}

// osFromTTL names the likely system family from a reply's TTL. Systems start at
// 64 (Linux, Android, Apple), 128 (Windows) or 255 (network gear and printers),
// and each router on the way takes one off. Only a hint: it can be changed.
func osFromTTL(ttl int) string {
	switch {
	case ttl <= 0:
		return ""
	case ttl <= 64:
		return "Linux, Android or Apple (TTL 64)"
	case ttl <= 128:
		return "Windows (TTL 128)"
	default:
		return "Network device or printer (TTL 255)"
	}
}

func openICMP() (*icmp.PacketConn, string, error) {
	if conn, err := icmp.ListenPacket("udp4", ""); err == nil {
		return conn, "udp4", nil
	}
	conn, err := icmp.ListenPacket("ip4:icmp", "")
	if err != nil {
		return nil, "", errors.New("ICMP unavailable: grant NET_RAW to the backend container or enable ping sockets")
	}
	return conn, "ip4:icmp", nil
}

func peerIs(peer net.Addr, want netip.Addr) bool {
	host, _, _ := net.SplitHostPort(peer.String())
	if host == "" {
		host = peer.String()
	}
	got, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return err == nil && got.Unmap() == want.Unmap()
}

func reverseName(parent context.Context, ip netip.Addr) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 600*time.Millisecond)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip.String())
	if err != nil || len(names) == 0 {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSpace(names[0]), "."), nil
}
