package agent

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// maxSweep is the agent's own ceiling on addresses per target, matching the
// server's /20. A larger target is refused, never enumerated.
const maxSweep = 4096

// Hosts lists the addresses of a frozen target. It refuses anything larger
// than maxSweep, so a bad job can't make the agent walk a huge range.
func Hosts(cidr string) ([]netip.Addr, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}
	p = p.Masked()
	bits := p.Addr().BitLen() - p.Bits()
	if bits > 12 {
		return nil, errors.New("target " + cidr + " is larger than the agent sweeps")
	}
	var out []netip.Addr
	a := p.Addr()
	for i := 0; i < 1<<bits && p.Contains(a); i++ {
		// Skip the IPv4 network and broadcast addresses on ordinary subnets.
		if !(a.Is4() && p.Bits() <= 30 && (i == 0 || i == 1<<bits-1)) {
			out = append(out, a)
		}
		a = a.Next()
	}
	return out, nil
}

// limiter paces probes to the policy's packets per second.
type limiter struct{ tick *time.Ticker }

func newLimiter(pps int) *limiter {
	if pps < 1 {
		pps = 1
	}
	return &limiter{time.NewTicker(time.Second / time.Duration(pps))}
}

func (l *limiter) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.tick.C:
		return nil
	}
}

func (l *limiter) stop() { l.tick.Stop() }

// Ping sends ICMP echo requests to each address, within the rate and
// concurrency limits, and returns the ones that answered.
func Ping(ctx context.Context, hosts []netip.Addr, cfg JobConfig) ([]netip.Addr, error) {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	lim := newLimiter(cfg.MaxPPS)
	defer lim.stop()
	conc := cfg.MaxConcurrency
	if conc < 1 {
		conc = 1
	}
	sem := make(chan struct{}, conc)
	var mu sync.Mutex
	var alive []netip.Addr
	var wg sync.WaitGroup
	var firstErr error
	for i, h := range hosts {
		if err := lim.wait(ctx); err != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(seq int, h netip.Addr) {
			defer wg.Done()
			defer func() { <-sem }()
			for attempt := 0; attempt <= cfg.Retries; attempt++ {
				ok, err := echo(h, seq, timeout)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				if ok {
					mu.Lock()
					alive = append(alive, h)
					mu.Unlock()
					return
				}
			}
		}(i, h)
	}
	wg.Wait()
	if len(alive) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return alive, nil
}

// echo sends one echo request using an unprivileged ICMP socket where the
// system allows it, and a raw socket otherwise.
func echo(a netip.Addr, seq int, timeout time.Duration) (bool, error) {
	network, proto, reply := "udp4", 1, icmp.Type(ipv4.ICMPTypeEchoReply)
	var typ icmp.Type = ipv4.ICMPTypeEcho
	if a.Is6() {
		network, proto, reply, typ = "udp6", 58, ipv6.ICMPTypeEchoReply, ipv6.ICMPTypeEchoRequest
	}
	conn, err := icmp.ListenPacket(network, "")
	if err != nil {
		raw := "ip4:icmp"
		if a.Is6() {
			raw = "ip6:ipv6-icmp"
		}
		if conn, err = icmp.ListenPacket(raw, ""); err != nil {
			return false, errors.New("ICMP needs CAP_NET_RAW or net.ipv4.ping_group_range for this user")
		}
		network = raw
	}
	defer conn.Close()
	msg := icmp.Message{Type: typ, Body: &icmp.Echo{ID: os.Getpid() & 0xffff, Seq: seq & 0xffff, Data: []byte("df-ipam")}}
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
	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			return false, nil
		}
		m, err := icmp.ParseMessage(proto, buf[:n])
		if err != nil || m.Type != reply {
			continue
		}
		host, _, _ := net.SplitHostPort(peer.String())
		if host == "" {
			host = peer.String()
		}
		if p, err := netip.ParseAddr(host); err == nil && p.Unmap() == a {
			return true, nil
		}
	}
}

// Neighbours reads the system's ARP table and IPv6 neighbour table. This is
// passive: it reads what the host already learned, usually from the sweep.
func Neighbours(ctx context.Context) map[netip.Addr]string {
	out := map[netip.Addr]string{}
	if f, err := os.Open("/proc/net/arp"); err == nil {
		s := bufio.NewScanner(f)
		s.Scan() // header
		for s.Scan() {
			fields := strings.Fields(s.Text())
			if len(fields) >= 4 && fields[3] != "00:00:00:00:00:00" {
				if a, err := netip.ParseAddr(fields[0]); err == nil {
					out[a] = fields[3]
				}
			}
		}
		f.Close()
	}
	if raw, err := exec.CommandContext(ctx, "ip", "-6", "neigh", "show").Output(); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			for i := 0; i+1 < len(fields); i++ {
				if fields[i] == "lladdr" {
					if a, err := netip.ParseAddr(fields[0]); err == nil {
						out[a] = fields[i+1]
					}
				}
			}
		}
	}
	return out
}

// ReverseDNS looks up PTR names. It asks the DNS servers, never the targets.
func ReverseDNS(ctx context.Context, hosts []netip.Addr, pps int) map[netip.Addr]string {
	lim := newLimiter(pps)
	defer lim.stop()
	out := map[netip.Addr]string{}
	for _, h := range hosts {
		if lim.wait(ctx) != nil {
			break
		}
		lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		names, err := net.DefaultResolver.LookupAddr(lctx, h.String())
		cancel()
		if err == nil && len(names) > 0 {
			out[h] = strings.TrimSuffix(names[0], ".")
		}
	}
	return out
}

type nmapRun struct {
	Hosts []struct {
		Status struct {
			State string `xml:"state,attr"`
		} `xml:"status"`
		Addresses []struct {
			Addr     string `xml:"addr,attr"`
			AddrType string `xml:"addrtype,attr"`
			Vendor   string `xml:"vendor,attr"`
		} `xml:"address"`
		Hostnames struct {
			Names []struct {
				Name string `xml:"name,attr"`
			} `xml:"hostname"`
		} `xml:"hostnames"`
	} `xml:"host"`
}

// NmapHost is one host nmap reported up.
type NmapHost struct {
	Address, MAC, Vendor, Hostname string
}

// Nmap runs host discovery only (-sn): no port scan, no OS detection, at the
// policy's rate. It is skipped when nmap isn't installed.
func Nmap(ctx context.Context, cidr string, cfg JobConfig) ([]NmapHost, error) {
	path, err := exec.LookPath("nmap")
	if err != nil {
		return nil, errors.New("nmap is not installed on this agent")
	}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	args := []string{"-sn", "-n", "--max-rate", strconv.Itoa(cfg.MaxPPS), "--max-retries", strconv.Itoa(cfg.Retries),
		"--host-timeout", strconv.Itoa(int(timeout.Milliseconds())*4) + "ms", "-oX", "-"}
	if strings.Contains(cidr, ":") {
		args = append(args, "-6")
	}
	raw, err := exec.CommandContext(ctx, path, append(args, cidr)...).Output()
	if err != nil {
		return nil, err
	}
	var run nmapRun
	if err := xml.Unmarshal(raw, &run); err != nil {
		return nil, err
	}
	var out []NmapHost
	for _, h := range run.Hosts {
		if h.Status.State != "up" {
			continue
		}
		var nh NmapHost
		for _, a := range h.Addresses {
			switch a.AddrType {
			case "ipv4", "ipv6":
				nh.Address = a.Addr
			case "mac":
				nh.MAC, nh.Vendor = a.Addr, a.Vendor
			}
		}
		if len(h.Hostnames.Names) > 0 {
			nh.Hostname = h.Hostnames.Names[0].Name
		}
		if nh.Address != "" {
			out = append(out, nh)
		}
	}
	return out, nil
}
