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
	"time"
)

// maxSweepBits is the agent's own ceiling on host bits per target: 4,096 addresses,
// matching the server's /20. A larger target is refused, never enumerated.
const maxSweepBits = 12

// Hosts lists the addresses of a frozen target. It refuses anything larger
// than maxSweepBits, so a bad job can't make the agent walk a huge range.
func Hosts(cidr string) ([]netip.Addr, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, err
	}
	p = p.Masked()
	bits := p.Addr().BitLen() - p.Bits()
	if bits > maxSweepBits {
		return nil, errors.New("target " + cidr + " is larger than the agent sweeps")
	}
	var out []netip.Addr
	a := p.Addr()
	for i := 0; i < 1<<bits && p.Contains(a); i++ {
		// Skip the IPv4 network and broadcast addresses on ordinary subnets.
		if !(a.Is4() && p.Bits() <= 30 && (i == 0 || i == 1<<bits-1)) && probeable(a) {
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

// ReverseDNS looks up PTR names. It asks the DNS servers, never the targets.
func ReverseDNS(ctx context.Context, hosts []netip.Addr, pps int) map[netip.Addr]string {
	lim := newLimiter(min(pps, maxDNSPerSecond))
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
	Hosts []nmapRunHost `xml:"host"`
}

type nmapRunHost struct {
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
}

// NmapHost is one host nmap reported up.
type NmapHost struct {
	Address, MAC, Vendor, Hostname string
}

// Neighbours reads the system's ARP table and IPv6 neighbour table. This is
// passive: it reads what the host already learned, usually from the sweep.
func Neighbours(ctx context.Context) map[netip.Addr]string {
	out := map[netip.Addr]string{}
	readARP(out)
	if raw, err := exec.CommandContext(ctx, "ip", "-6", "neigh", "show").Output(); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			readNeighbour(out, strings.Fields(line))
		}
	}
	return out
}

func readARP(out map[netip.Addr]string) {
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Scan() // header
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 4 || fields[3] == "00:00:00:00:00:00" {
			continue
		}
		if a, err := netip.ParseAddr(fields[0]); err == nil {
			out[a] = fields[3]
		}
	}
}

func readNeighbour(out map[netip.Addr]string, fields []string) {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "lladdr" {
			continue
		}
		if a, err := netip.ParseAddr(fields[0]); err == nil {
			out[a] = fields[i+1]
		}
	}
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
		"--max-parallelism", strconv.Itoa(max(cfg.MaxConcurrency, 1)),
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
		if nh := nmapHost(h); h.Status.State == "up" && nh.Address != "" {
			out = append(out, nh)
		}
	}
	return out, nil
}

func nmapHost(h nmapRunHost) NmapHost {
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
	return nh
}
