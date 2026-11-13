package scanner

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"time"
)

// commonPorts are the TCP ports checked on segments that ask for it: the usual
// remote access, web, file, mail, database and printer services. The list is
// fixed and short on purpose; this is a quick look, not a full port scan.
var commonPorts = []int{21, 22, 23, 25, 53, 80, 110, 135, 139, 143, 443, 445, 515, 631, 1433, 1521,
	3306, 3389, 5432, 5900, 5985, 6379, 8080, 8443, 9100, 27017}

// presencePorts are tried on an address that doesn't answer ping: file sharing
// and remote desktop (Windows), and SSH (Linux servers).
var presencePorts = []int{445, 3389, 22}

// portSlots is how many port checks may be in flight across a whole scan, so
// many answering devices at once can't turn into a burst of connections.
const portSlots = 16

// tcpEvery paces TCP connection attempts across a whole scan: at most 100 a
// second. Closed ports answer at once on a LAN, so a cap on connections in
// flight alone would still allow hundreds a second.
const tcpEvery = 10 * time.Millisecond

// dial tries one TCP port and reports whether the connection completed. A
// refusal is not counted: a router rejecting on a host's behalf looks the same.
func dial(ctx context.Context, ip netip.Addr, port int, tick <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-tick:
	}
	d := net.Dialer{Timeout: 600 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// answersTCP reports whether a host that ignores ping answers on a presence port.
func answersTCP(ctx context.Context, p probePlan, ip netip.Addr) bool {
	for _, port := range presencePorts {
		if dial(ctx, ip, port, p.tcpTick) {
			return true
		}
	}
	return false
}

// openPorts tries each common port, at most portSlots at a time across the scan
// and paced by tick. A connection that completes is closed straight away,
// without sending anything.
func openPorts(ctx context.Context, ip netip.Addr, slots chan struct{}, tick <-chan time.Time) []int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	open := []int{}
	for _, port := range commonPorts {
		select {
		case <-ctx.Done():
			wg.Wait()
			return open
		case slots <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			if dial(ctx, ip, port, tick) {
				mu.Lock()
				open = append(open, port)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	slices.Sort(open)
	return open
}

// portList writes ports as a Postgres array literal, such as {22,80,443}, or ""
// when the ports were not checked this time, so the last result is kept.
func portList(ports []int, checked bool) string {
	if !checked {
		return ""
	}
	b := []byte{'{'}
	for i, p := range ports {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, int64(p), 10)
	}
	return string(append(b, '}'))
}
