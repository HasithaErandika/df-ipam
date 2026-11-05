package agent

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
	"golang.org/x/net/ipv6"
)

// pingResults collects what concurrent probes find.
type pingResults struct {
	mu       sync.Mutex
	alive    []netip.Addr
	firstErr error
}

func (r *pingResults) record(h netip.Addr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alive = append(r.alive, h)
}

func (r *pingResults) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.firstErr == nil {
		r.firstErr = err
	}
}

// probe tries one address, up to the policy's retries. Every retry is a packet too,
// so each waits for the limiter: dead addresses must not multiply the rate.
func (r *pingResults) probe(ctx context.Context, lim *limiter, h netip.Addr, seq, retries int, timeout time.Duration) {
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 && lim.wait(ctx) != nil {
			return
		}
		ok, err := echo(h, seq, timeout)
		if err != nil {
			r.fail(err)
			return
		}
		if ok {
			r.record(h)
			return
		}
	}
}

// Ping sends ICMP echo requests to each address, within the rate and
// concurrency limits, and returns the ones that answered.
func Ping(ctx context.Context, hosts []netip.Addr, cfg JobConfig) ([]netip.Addr, error) {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	lim := newLimiter(cfg.MaxPPS)
	defer lim.stop()
	sem := make(chan struct{}, max(cfg.MaxConcurrency, 1))
	var results pingResults
	var wg sync.WaitGroup
	for i, h := range hosts {
		if err := lim.wait(ctx); err != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results.probe(ctx, lim, h, i, cfg.Retries, timeout)
		}()
	}
	wg.Wait()
	if len(results.alive) == 0 && results.firstErr != nil {
		return nil, results.firstErr
	}
	return results.alive, nil
}

// icmpFlavour is what differs between IPv4 and IPv6 echo.
type icmpFlavour struct {
	network, raw string
	proto        int
	request      icmp.Type
	reply        icmp.Type
}

func flavourOf(a netip.Addr) icmpFlavour {
	if a.Is6() {
		return icmpFlavour{"udp6", "ip6:ipv6-icmp", 58, ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply}
	}
	return icmpFlavour{"udp4", "ip4:icmp", 1, ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply}
}

// openICMP prefers an unprivileged ICMP socket, where the system allows one.
func openICMP(f icmpFlavour) (*icmp.PacketConn, string, error) {
	if conn, err := icmp.ListenPacket(f.network, ""); err == nil {
		return conn, f.network, nil
	}
	conn, err := icmp.ListenPacket(f.raw, "")
	if err != nil {
		return nil, "", errors.New("ICMP needs CAP_NET_RAW or net.ipv4.ping_group_range for this user")
	}
	return conn, f.raw, nil
}

// echo sends one echo request and waits for the matching reply.
func echo(a netip.Addr, seq int, timeout time.Duration) (bool, error) {
	f := flavourOf(a)
	conn, network, err := openICMP(f)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	msg := icmp.Message{Type: f.request, Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: seq & 0xffff, Data: []byte("df-ipam")}}
	b, err := msg.Marshal(nil)
	if err != nil {
		return false, err
	}
	var dst net.Addr = &net.IPAddr{IP: a.AsSlice()}
	if strings.HasPrefix(network, "udp") {
		dst = &net.UDPAddr{IP: a.AsSlice()}
	}
	if _, err := conn.WriteTo(b, dst); err != nil {
		return false, nil
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	return awaitReply(conn, a, f), nil
}

func awaitReply(conn *icmp.PacketConn, a netip.Addr, f icmpFlavour) bool {
	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return false
		}
		m, err := icmp.ParseMessage(f.proto, buf[:n])
		if err == nil && m.Type == f.reply && peerIs(peer, a) {
			return true
		}
	}
}

func peerIs(peer net.Addr, a netip.Addr) bool {
	host, _, _ := net.SplitHostPort(peer.String())
	if host == "" {
		host = peer.String()
	}
	p, err := netip.ParseAddr(host)
	return err == nil && p.Unmap() == a
}
