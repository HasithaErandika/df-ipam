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
	conn, network, err := openICMP()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	message := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: int(time.Now().UnixNano() & 0xffff), Data: []byte("df-ipam")}}
	packet, err := message.Marshal(nil)
	if err != nil {
		return false, err
	}
	var dst net.Addr = &net.IPAddr{IP: ip.AsSlice()}
	if network == "udp4" {
		dst = &net.UDPAddr{IP: ip.AsSlice()}
	}
	if _, err := conn.WriteTo(packet, dst); err != nil {
		return false, nil
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetReadDeadline(deadline)
	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return false, nil
		}
		msg, err := icmp.ParseMessage(1, buf[:n])
		if err == nil && msg.Type == ipv4.ICMPTypeEchoReply && peerIs(peer, ip) {
			return true, nil
		}
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
