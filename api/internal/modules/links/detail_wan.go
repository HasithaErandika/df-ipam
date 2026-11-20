package links

import (
	"time"

	"dfipam/api/internal/platform/db"

	"github.com/gin-gonic/gin"
)

// wanAddress is one of a link's WAN addresses with the subnet it is in: the
// chosen WAN subnet for the main address, else the segment holding it, else
// the network its stored prefix gives. Register entries are named when there.
type wanAddress struct {
	IP           *string `db:"ip" json:"ip"` // empty when only the WAN subnet is known
	Main         bool    `db:"main" json:"main"`
	Network      *string `db:"network" json:"network"`
	SegmentID    *string `db:"segment_id" json:"segmentId"`
	SegmentName  *string `db:"segment_name" json:"segmentName"`
	RegisterID   *string `db:"register_id" json:"registerId"`
	RegisterName *string `db:"register_name" json:"registerName"`
}

// linkDevice is a connected device with where the scan last saw it.
type linkDevice struct {
	ID       string     `db:"id" json:"id"`
	Name     string     `db:"name" json:"name"`
	MAC      string     `db:"mac" json:"mac"`
	Type     string     `db:"device_type" json:"deviceType"`
	Status   string     `db:"status" json:"status"`
	LastIP   *string    `db:"last_ip" json:"lastIp"`
	LastSeen *time.Time `db:"last_seen" json:"lastSeen"`
}

const wanAddressesSQL = `WITH a AS (
	SELECT 0 AS n, l.wan_ip AS ip, ws.id AS ws_id FROM network_links l LEFT JOIN segments ws ON ws.id = l.wan_segment_id
		WHERE l.id = $1::uuid AND (l.wan_ip IS NOT NULL OR l.wan_segment_id IS NOT NULL)
	UNION ALL
	SELECT u.n, u.e, NULL FROM network_links l, unnest(l.extra_wan_ips) WITH ORDINALITY u(e, n) WHERE l.id = $1::uuid)
SELECT host(a.ip) AS ip, a.n = 0 AS main, COALESCE(s.network::text, CASE WHEN masklen(a.ip) < 32 OR a.n > 0 THEN network(a.ip)::text END) AS network,
	s.id::text AS segment_id, s.name AS segment_name, r.id::text AS register_id, NULLIF(r.assigned_name, '') AS register_name
FROM a
LEFT JOIN LATERAL (SELECT id, name, network FROM segments
	WHERE id = a.ws_id OR (a.ws_id IS NULL AND network >>= set_masklen(a.ip, 32)) ORDER BY id = a.ws_id DESC LIMIT 1) s ON true
LEFT JOIN ip_addresses r ON r.ip = set_masklen(a.ip, 32)
ORDER BY a.n`

const linkDevicesSQL = `SELECT d.id::text AS id, COALESCE(NULLIF(d.name, ''), d.mac::text) AS name, d.mac::text AS mac, d.device_type, d.status,
	host(s.ip) AS last_ip, s.last_seen
FROM network_links l, unnest(l.device_ids) WITH ORDINALITY u(id, n)
JOIN devices d ON d.id = u.id
LEFT JOIN LATERAL (SELECT ip, last_seen FROM sightings WHERE mac = d.mac ORDER BY last_seen DESC LIMIT 1) s ON true
WHERE l.id = $1::uuid ORDER BY u.n`

// attachWAN adds the link's WAN addresses by subnet and its connected devices.
func (h *Handler) attachWAN(c *gin.Context, id string, out gin.H) error {
	wan, err := db.All[wanAddress](c, h.conn, wanAddressesSQL, id)
	if err != nil {
		return err
	}
	devices, err := db.All[linkDevice](c, h.conn, linkDevicesSQL, id)
	if err != nil {
		return err
	}
	out["wanAddresses"], out["devices"] = wan, devices
	return nil
}
