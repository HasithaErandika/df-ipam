package links

import (
	"context"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// seenDevice is a device the scan saw on one of a link's addresses. It is
// evidence: people choose whether it becomes one of the link's devices.
type seenDevice struct {
	ID       string    `db:"id" json:"id"`
	Name     string    `db:"name" json:"name"`
	MAC      string    `db:"mac" json:"mac"`
	Status   string    `db:"status" json:"status"`
	IP       string    `db:"ip" json:"ip"`
	LastSeen time.Time `db:"last_seen" json:"lastSeen"`
}

// seenWindow is how recent a sighting must be to count.
const seenWindow = "30 days"

// devicesSeenAt lists the devices sighted on these addresses in the last 30
// days, the latest sighting of each.
func devicesSeenAt(ctx context.Context, q db.Conn, ips []string) ([]seenDevice, error) {
	return db.All[seenDevice](ctx, q, `SELECT * FROM (
		SELECT DISTINCT ON (d.id) d.id::text AS id, COALESCE(NULLIF(d.name, ''), d.mac::text) AS name, d.mac::text AS mac, d.status,
			host(s.ip) AS ip, s.last_seen
		FROM sightings s JOIN devices d ON d.mac = s.mac
		WHERE s.ip = ANY($1::inet[]) AND s.last_seen > now() - $2::interval
		ORDER BY d.id, s.last_seen DESC) x ORDER BY ip, name`, ips, seenWindow)
}

// seenDevices answers the link form: which devices the scan saw on the
// addresses typed so far (?ip=…, up to 20).
func (h *Handler) seenDevices(c *gin.Context) {
	var ips []string
	for _, s := range c.QueryArray("ip") {
		if a, err := netip.ParseAddr(s); err == nil && a.Is4() && !slices.Contains(ips, a.String()) {
			ips = append(ips, a.String())
		}
	}
	if len(ips) > 20 {
		httpx.Fail(c, httpx.Validation("ip", "Ask about at most 20 addresses."))
		return
	}
	out, err := devicesSeenAt(c, h.conn, ips)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// checkDevices refuses devices newly added to the link unless they are approved.
// Devices the link already had stay, whatever their status now.
func checkDevices(ctx context.Context, tx pgx.Tx, ids, had []string) error {
	var added []string
	for _, id := range ids {
		if !slices.Contains(had, id) {
			added = append(added, id)
		}
	}
	if len(added) == 0 {
		return nil
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM devices WHERE id = ANY($1::uuid[]) AND status = 'approved'`, added).Scan(&n); err != nil {
		return err
	}
	if n != len(added) {
		return httpx.Validation("deviceIds", "Choose approved devices. Approve a device in Devices first.")
	}
	return nil
}

// linkAddresses are the addresses a link's devices answer on: the WAN IPs and the gateway.
func linkAddresses(l *linkDTO) []string {
	var out []string
	for _, p := range []*string{l.WANIP, l.GatewayIP} {
		if p != nil && *p != "" {
			out = append(out, *p)
		}
	}
	for _, x := range l.ExtraWANIPs {
		if p, err := netip.ParsePrefix(x); err == nil {
			out = append(out, p.Addr().String())
		}
	}
	return out
}

// setDevices reads an import cell of device names or MACs separated by ;.
// Known devices are linked; any other name is kept as text.
func setDevices(in *linkInput, v string, x *resolver) {
	in.DeviceIDs, in.DeviceText = []string{}, ""
	var other []string
	for _, name := range split(v) {
		if id := x.find("devices", name); id != "" {
			in.DeviceIDs = append(in.DeviceIDs, id)
		} else {
			other = append(other, name)
		}
	}
	in.DeviceText = strings.Join(other, "; ")
}
