package inventory

import (
	"net/http"
	"net/netip"
	"time"

	"dfipam/api/internal/ipam"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type segmentDTO struct {
	ID              string     `db:"id" json:"id"`
	Name            string     `db:"name" json:"name"`
	Kind            string     `db:"kind" json:"kind"`
	Floor           *int       `db:"floor" json:"floor"`
	VLAN            *int       `db:"vlan" json:"vlan"`
	Network         string     `db:"network" json:"network"`
	Gateway         *string    `db:"gateway" json:"gateway"`
	DHCPStart       *string    `db:"dhcp_start" json:"dhcpStart"`
	DHCPEnd         *string    `db:"dhcp_end" json:"dhcpEnd"`
	EnvironmentID   *string    `db:"default_environment_id" json:"defaultEnvironmentId"`
	EnvironmentName *string    `db:"environment_name" json:"environmentName"`
	Description     string     `db:"description" json:"description"`
	AlertPrivateMAC bool       `db:"alert_private_macs" json:"alertPrivateMacs"`
	CheckPorts      bool       `db:"check_ports" json:"checkPorts"`
	Registered      int64      `db:"registered" json:"registered"`
	Seen            int64      `db:"seen" json:"seen"`
	Online          int64      `db:"online" json:"online"`
	PoolSize        int64      `db:"pool_size" json:"poolSize"`
	PoolUsed        int64      `db:"pool_used" json:"poolUsed"`
	Unknown         int64      `db:"unknown_devices" json:"unknownDevices"`
	OpenAlerts      int64      `db:"open_alerts" json:"openAlerts"`
	LastSeen        *time.Time `db:"last_seen" json:"lastSeen"`
	DetectedGateway *string    `db:"detected_gateway" json:"detectedGateway"`
	UsableSize      int64      `db:"-" json:"usableSize"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updatedAt"`
}

const segmentSelect = `SELECT s.id::text AS id, s.name, s.kind, s.floor, s.vlan, s.network::text AS network,
	host(s.gateway) AS gateway, host(s.dhcp_start) AS dhcp_start, host(s.dhcp_end) AS dhcp_end,
	s.default_environment_id::text AS default_environment_id, e.name AS environment_name, s.description, s.alert_private_macs, s.check_ports,
	COALESCE(reg.n, 0) AS registered, COALESCE(seen.seen, 0) AS seen, COALESCE(seen.online, 0) AS online,
	COALESCE(s.dhcp_end - s.dhcp_start + 1, 0) AS pool_size, COALESCE(seen.pool_used, 0) AS pool_used,
	COALESCE(seen.unknown_devices, 0) AS unknown_devices, COALESCE(al.n, 0) AS open_alerts,
	(SELECT max(v.last_seen) FROM sightings v WHERE v.segment_id = s.id) AS last_seen,
	(SELECT host(v.ip) FROM sightings v WHERE v.segment_id = s.id AND v.is_gateway ORDER BY v.last_seen DESC LIMIT 1) AS detected_gateway,
	s.updated_at
	FROM segments s LEFT JOIN environments e ON e.id = s.default_environment_id
	LEFT JOIN (SELECT segment_id, count(*) AS n FROM ip_addresses GROUP BY segment_id) reg ON reg.segment_id = s.id
	LEFT JOIN (SELECT segment_id, count(*) AS n FROM alerts WHERE status <> 'closed' GROUP BY segment_id) al ON al.segment_id = s.id
	LEFT JOIN (` + segmentActivity + `) seen ON seen.segment_id = s.id`

// segmentActivity counts what each segment's sightings show, in one pass over
// recent sightings instead of one query per count per segment.
const segmentActivity = `SELECT v.segment_id,
		count(DISTINCT v.ip) FILTER (WHERE v.last_seen > now() - interval '24 hours') AS seen,
		count(DISTINCT v.ip) FILTER (WHERE v.last_seen > w.since) AS online,
		count(DISTINCT v.ip) FILTER (WHERE v.last_seen > w.since AND v.ip BETWEEN sg.dhcp_start AND sg.dhcp_end) AS pool_used,
		count(DISTINCT v.mac) FILTER (WHERE v.last_seen > now() - interval '24 hours' AND v.mac IS NOT NULL AND NOT v.is_private_mac
			AND d.status IS DISTINCT FROM 'approved') AS unknown_devices
	FROM sightings v JOIN segments sg ON sg.id = v.segment_id LEFT JOIN devices d ON d.mac = v.mac
	CROSS JOIN (SELECT now() - ` + db.SeenWindow + ` AS since) w
	WHERE v.last_seen > LEAST(w.since, now() - interval '24 hours')
	GROUP BY v.segment_id`

// segments lists every segment. There are few (one per floor and server network),
// and forms need all of them, so this list is not paged.
func (h *Handler) segments(c *gin.Context) {
	var w db.Where
	if kind := c.Query("kind"); kind == "dhcp" || kind == "server" || kind == "cloud" || kind == "vpn" {
		w.Add("s.kind = ?", kind)
	}
	out, err := db.All[segmentDTO](c, h.conn, segmentSelect+w.SQL()+" ORDER BY s.kind, s.floor NULLS LAST, lower(s.name)", w.Args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	for i := range out {
		if p, err := netip.ParsePrefix(out[i].Network); err == nil {
			r := netcalc.UsableRange(p)
			out[i].UsableSize = ipam.RangeSize(r.First, r.Last)
		}
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) createSegment(c *gin.Context) { h.saveSegment(c, "") }

func (h *Handler) updateSegment(c *gin.Context) {
	if id, ok := httpx.PathID(c); ok {
		h.saveSegment(c, id)
	}
}

func (h *Handler) saveSegment(c *gin.Context, id string) {
	var in segmentInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	args := []any{in.Name, in.Kind, in.Floor, in.VLAN, in.Network, in.Gateway, in.DHCPStart, in.DHCPEnd, in.EnvironmentID, in.Description, actor(c), in.AlertPrivateMACs, in.CheckPorts}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if id == "" {
			if err := tx.QueryRow(c, `INSERT INTO segments (name, kind, floor, vlan, network, gateway, dhcp_start, dhcp_end, default_environment_id, description, created_by, updated_by, alert_private_macs, check_ports)
				VALUES ($1, $2, $3, $4, $5::cidr, NULLIF($6, '')::inet, NULLIF($7, '')::inet, NULLIF($8, '')::inet, NULLIF($9, '')::uuid, $10, $11::uuid, $11::uuid, $12, $13)
				RETURNING id::text`, args...).Scan(&id); err != nil {
				return err
			}
			return record(c, tx, "create", "segment", id, nil, in)
		}
		old, err := lockOld(c, tx, "segments", id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(c, `UPDATE segments SET name = $1, kind = $2, floor = $3, vlan = $4, network = $5::cidr, gateway = NULLIF($6, '')::inet,
			dhcp_start = NULLIF($7, '')::inet, dhcp_end = NULLIF($8, '')::inet, default_environment_id = NULLIF($9, '')::uuid,
			description = $10, updated_by = $11::uuid, alert_private_macs = $12, check_ports = $13 WHERE id = $14::uuid`, append(args, id)...); err != nil {
			return err
		}
		return record(c, tx, "update", "segment", id, old, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "A segment with this name exists, or the network overlaps another segment."))
		return
	}
	h.recheck(c)
	httpx.OK(c, http.StatusOK, gin.H{"id": id})
}

func (h *Handler) deleteSegment(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		old, err := lockOld(c, tx, "segments", id)
		if err != nil {
			return err
		}
		var used bool
		if err := tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM ip_addresses WHERE segment_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM reservations WHERE segment_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM ip_requests WHERE segment_id = $1::uuid)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return httpx.NewError(http.StatusConflict, "SEGMENT_IN_USE", "This segment still has register entries, reservations or requests. Remove them first.")
		}
		if _, err := tx.Exec(c, "DELETE FROM segments WHERE id = $1::uuid", id); err != nil {
			return err
		}
		return record(c, tx, "delete", "segment", id, old, nil)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	h.recheck(c)
	c.Status(http.StatusNoContent)
}
