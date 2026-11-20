package links

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

// Firewall gives a client for the FortiGate connected in System > FortiGate.
type Firewall interface {
	Connect(ctx context.Context) (*fortigate.Client, error)
}

// liveGap is the shortest time between two reads of the firewall, however
// many people watch: everyone shares the last sample.
const liveGap = 10 * time.Second

// sampler keeps the last view and byte counters, to work out rates.
type sampler struct {
	mu     sync.Mutex
	last   *liveView
	prev   map[string][2]uint64
	prevAt time.Time
}

var errNoFirewall = httpx.NewError(http.StatusConflict, "FIREWALL_NOT_CONNECTED", "Connect a FortiGate in System > FortiGate first: its address and a read-only API token.")

func (h *Handler) firewallClient(c *gin.Context) (*fortigate.Client, bool) {
	if h.firewall == nil {
		httpx.Fail(c, errNoFirewall)
		return nil, false
	}
	client, err := h.firewall.Connect(c)
	if err != nil {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "FIREWALL_NOT_CONNECTED", "The FortiGate isn't connected: "+err.Error()+"."))
		return nil, false
	}
	return client, true
}

// live returns the firewall's interfaces, tunnels, SD-WAN health and load,
// each matched to the link recorded for it. Nothing is stored.
func (h *Handler) live(c *gin.Context) {
	client, ok := h.firewallClient(c)
	if !ok {
		return
	}
	view, err := h.sample.read(c, client)
	if err != nil {
		httpx.Fail(c, httpx.NewError(http.StatusBadGateway, "FIREWALL_UNREACHABLE", "The FortiGate didn't answer: "+err.Error()+"."))
		return
	}
	// Only people who may see links learn which link each row belongs to.
	if can(c, auth.LinksView) {
		if err := matchLive(c, h.conn, &view); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	httpx.OK(c, http.StatusOK, view)
}

// read gives the shared sample, reading the firewall again only when it is
// older than liveGap. The first read takes two samples two seconds apart, so
// rates show at once.
func (s *sampler) read(ctx context.Context, client *fortigate.Client) (liveView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last != nil && time.Since(s.last.At) < liveGap {
		return copyView(*s.last), nil
	}
	if s.last == nil || time.Since(s.prevAt) > 5*time.Minute {
		if _, err := s.take(ctx, client); err != nil {
			return liveView{}, err
		}
		time.Sleep(2 * time.Second)
	}
	v, err := s.take(ctx, client)
	if err != nil {
		return liveView{}, err
	}
	s.last = &v
	return copyView(v), nil
}

// copyView gives each request its own slices, as matching fills them in.
func copyView(v liveView) liveView {
	v.Interfaces = append([]liveIface(nil), v.Interfaces...)
	v.Tunnels = append([]liveTunnel(nil), v.Tunnels...)
	v.VPNUsers = append([]fortigate.VPNUser(nil), v.VPNUsers...)
	return v
}

func rate(now, before uint64, seconds float64) *float64 {
	if seconds <= 0 || now < before { // a counter reset or a reboot
		return nil
	}
	r := float64(now-before) * 8 / seconds
	return &r
}

func sortByName[T any](list []T, name func(T) string) {
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(name(list[i])) < strings.ToLower(name(list[j])) })
}

// matchLive names the link recorded for each interface (by interface name or
// WAN IP) and tunnel (by tunnel name).
func matchLive(ctx context.Context, conn db.Conn, v *liveView) error {
	rows, err := conn.Query(ctx, `SELECT l.id::text, l.code, l.name, lower(l.interface), COALESCE(host(l.wan_ip), ''), lower(l.vpn_tunnel),
		l.down_mbps::float8, l.up_mbps::float8 FROM network_links l WHERE l.archived_at IS NULL ORDER BY l.code`)
	if err != nil {
		return err
	}
	defer rows.Close()
	byIface, byIP, byTunnel := map[string]*linkRef{}, map[string]*linkRef{}, map[string]*linkRef{}
	for rows.Next() {
		var r linkRef
		var iface, ip, tunnel string
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &iface, &ip, &tunnel, &r.DownMbps, &r.UpMbps); err != nil {
			return err
		}
		ref := r
		keep(byIface, iface, &ref)
		keep(byIP, ip, &ref)
		keep(byTunnel, tunnel, &ref)
	}
	for i := range v.Interfaces {
		x := &v.Interfaces[i]
		if x.Link = byIface[strings.ToLower(x.Name)]; x.Link == nil && x.IP != "" {
			x.Link = byIP[x.IP]
		}
	}
	for i := range v.Tunnels {
		x := &v.Tunnels[i]
		if x.Link = byTunnel[strings.ToLower(x.Name)]; x.Link == nil {
			x.Link = byIface[strings.ToLower(x.Name)]
		}
	}
	return rows.Err()
}

// keep records the first link with a key: the lowest Link ID wins.
func keep(m map[string]*linkRef, key string, r *linkRef) {
	if _, ok := m[key]; key != "" && !ok {
		m[key] = r
	}
}
