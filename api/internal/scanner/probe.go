package scanner

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// pinger sends echo requests over one ICMP socket and hands each reply to the
// probe waiting for it, matched by sequence number and sender. One socket per
// scan, instead of one per address, so raw sockets don't each receive every
// reply on the machine.
type pinger struct {
	conn    *icmp.PacketConn
	p4      *ipv4.PacketConn
	raw     bool // ip4:icmp: the echo ID is ours to check; on udp4 the kernel filters
	withTTL bool
	id      int
	mu      sync.Mutex
	seq     uint16
	waiting map[uint16]waiter
}

type waiter struct {
	ip    netip.Addr
	reply chan int // the reply's TTL
}

func newPinger() (*pinger, error) {
	conn, network, err := openICMP()
	if err != nil {
		return nil, err
	}
	p := &pinger{conn: conn, raw: network == "ip4:icmp", id: os.Getpid() & 0xffff, waiting: map[uint16]waiter{}}
	p.p4 = conn.IPv4PacketConn()
	p.withTTL = p.p4 != nil && p.p4.SetControlMessage(ipv4.FlagTTL, true) == nil
	go p.read()
	return p, nil
}

func (p *pinger) Close() { _ = p.conn.Close() }

// read hands replies to their waiters until the socket is closed.
func (p *pinger) read() {
	buf := make([]byte, 1500)
	for {
		n, ttl, peer, err := p.readOne(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		p.deliver(buf[:n], ttl, peer)
	}
}

// deliver passes an echo reply's TTL to the ping waiting for it, if any. A raw
// socket sees every process's replies, so it also checks the echo ID.
func (p *pinger) deliver(packet []byte, ttl int, peer net.Addr) {
	msg, err := icmp.ParseMessage(1, packet)
	if err != nil || msg.Type != ipv4.ICMPTypeEchoReply {
		return
	}
	echo, ok := msg.Body.(*icmp.Echo)
	if !ok || (p.raw && echo.ID != p.id) {
		return
	}
	p.mu.Lock()
	w, found := p.waiting[uint16(echo.Seq)]
	p.mu.Unlock()
	if found && peerIs(peer, w.ip) {
		select {
		case w.reply <- ttl:
		default:
		}
	}
}

func (p *pinger) readOne(buf []byte) (int, int, net.Addr, error) {
	if !p.withTTL {
		n, peer, err := p.conn.ReadFrom(buf)
		return n, 0, peer, err
	}
	n, cm, peer, err := p.p4.ReadFrom(buf)
	if cm == nil {
		return n, 0, peer, err
	}
	return n, cm.TTL, peer, err
}

// ping sends one echo and waits for its reply. It returns the reply's TTL, or 0
// when the system won't say; the starting TTL hints at the system (osFromTTL).
// An error means the request could not be sent, not that the host is down.
func (p *pinger) ping(ctx context.Context, ip netip.Addr, timeout time.Duration) (bool, int, error) {
	w := waiter{ip: ip, reply: make(chan int, 1)}
	p.mu.Lock()
	p.seq++
	seq := p.seq
	p.waiting[seq] = w
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.waiting, seq)
		p.mu.Unlock()
	}()
	packet, err := (&icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: p.id, Seq: int(seq), Data: []byte("df-ipam")}}).Marshal(nil)
	if err != nil {
		return false, 0, err
	}
	var dst net.Addr = &net.IPAddr{IP: ip.AsSlice()}
	if !p.raw {
		dst = &net.UDPAddr{IP: ip.AsSlice()}
	}
	if _, err := p.conn.WriteTo(packet, dst); err != nil {
		return false, 0, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ttl := <-w.reply:
		return true, ttl, nil
	case <-timer.C:
		return false, 0, nil
	case <-ctx.Done():
		return false, 0, nil
	}
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
