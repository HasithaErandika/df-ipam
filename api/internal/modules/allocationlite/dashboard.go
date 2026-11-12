package allocationlite

import (
	"net/http"
	"net/netip"
	"time"

	"dfipam/api/internal/ipam"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type floorDTO struct {
	ID         string  `db:"id" json:"id"`
	Name       string  `db:"name" json:"name"`
	Floor      *int    `db:"floor" json:"floor"`
	VLAN       *int    `db:"vlan" json:"vlan"`
	Network    string  `db:"network" json:"network"`
	Registered int64   `db:"registered" json:"registered"`
	Seen       int64   `db:"seen" json:"seen"`
	PoolUsed   int64   `db:"pool_used" json:"poolUsed"`
	Static     int64   `db:"static" json:"static"`
	DHCPStart  *string `db:"dhcp_start" json:"-"`
	DHCPEnd    *string `db:"dhcp_end" json:"-"`
	PoolSize   int64   `db:"-" json:"poolSize"`
}

type environmentHealth struct {
	Environment string `db:"environment" json:"environment"`
	Colour      string `db:"colour" json:"colour"`
	Total       int64  `db:"total" json:"total"`
	Up          int64  `db:"up" json:"up"`
	Down        int64  `db:"down" json:"down"`
	Unknown     int64  `db:"unknown" json:"unknown"`
}

// recentAlert is one open alert for the dashboard's short list.
type recentAlert struct {
	ID       string    `db:"id" json:"id"`
	Type     string    `db:"alert_type" json:"type"`
	Severity string    `db:"severity" json:"severity"`
	Message  string    `db:"message" json:"message"`
	IP       *string   `db:"ip" json:"ip"`
	Segment  *string   `db:"segment" json:"segment"`
	Opened   time.Time `db:"opened_at" json:"openedAt"`
}

const recentAlertsSQL = `SELECT a.id::text AS id, a.alert_type, a.severity, a.message, host(a.ip) AS ip, s.name AS segment, a.opened_at
	FROM alerts a LEFT JOIN segments s ON s.id = a.segment_id WHERE a.status <> 'closed'
	ORDER BY array_position(ARRAY['critical','high','medium','low'], a.severity), a.opened_at DESC LIMIT 6`

type severityCount struct {
	Severity string `db:"severity" json:"severity"`
	Count    int64  `db:"count" json:"count"`
}

// "Seen" means answered within the last two scan intervals, at least ten minutes.
const recentSeen = `make_interval(mins => GREATEST(10, 2 * COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), 10)))`

const floorsSQL = `SELECT s.id::text AS id, s.name, s.floor, s.vlan, s.network::text AS network,
	(SELECT count(*) FROM ip_addresses i WHERE i.segment_id = s.id) AS registered,
	count(DISTINCT v.ip) AS seen,
	count(DISTINCT v.ip) FILTER (WHERE s.dhcp_start IS NOT NULL AND v.ip BETWEEN s.dhcp_start AND s.dhcp_end) AS pool_used,
	count(DISTINCT v.ip) FILTER (WHERE v.is_static) AS static,
	host(s.dhcp_start) AS dhcp_start, host(s.dhcp_end) AS dhcp_end
	FROM segments s LEFT JOIN sightings v ON v.segment_id = s.id AND v.last_seen > now() - ` + recentSeen + `
	WHERE s.kind = 'dhcp' GROUP BY s.id ORDER BY s.floor NULLS LAST, lower(s.name)`

const serversSQL = `SELECT COALESCE(e.name, 'No environment') AS environment, COALESCE(e.colour, '#64748b') AS colour,
	count(*) AS total, count(*) FILTER (WHERE i.health = 'up') AS up, count(*) FILTER (WHERE i.health = 'down') AS down,
	count(*) FILTER (WHERE i.health = 'unknown') AS unknown
	FROM ip_addresses i JOIN segments s ON s.id = i.segment_id LEFT JOIN environments e ON e.id = COALESCE(i.environment_id, s.default_environment_id)
	WHERE i.monitor GROUP BY e.name, e.colour, e.sort_order ORDER BY e.sort_order NULLS LAST, e.name`

func (h *Handler) dashboard(c *gin.Context) {
	floors, err := db.All[floorDTO](c, h.conn, floorsSQL)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	for i, f := range floors {
		if f.DHCPStart != nil && f.DHCPEnd != nil {
			first, e1 := netip.ParseAddr(*f.DHCPStart)
			last, e2 := netip.ParseAddr(*f.DHCPEnd)
			if e1 == nil && e2 == nil {
				floors[i].PoolSize = ipam.RangeSize(first, last)
			}
		}
	}
	alerts, err := db.All[severityCount](c, h.conn, `SELECT severity, count(*) AS count FROM alerts WHERE status <> 'closed' GROUP BY severity`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	servers, err := db.All[environmentHealth](c, h.conn, serversSQL)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	recent, err := db.All[recentAlert](c, h.conn, recentAlertsSQL)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var totals struct {
		PendingDevices  int64 `json:"pendingDevices"`
		PendingRequests int64 `json:"pendingRequests"`
		ExpiringHolds   int64 `json:"expiringReservations"`
		Registered      int64 `json:"registered"`
		Assigned        int64 `json:"assigned"`
		Reserved        int64 `json:"reserved"`
		Blocked         int64 `json:"blocked"`
		Held            int64 `json:"held"`
		OnlineDevices   int64 `json:"online"`
		UnknownDevices  int64 `json:"unknownDevices"`
		Approved        int64 `json:"approvedDevices"`
		Rejected        int64 `json:"rejectedDevices"`
		NewLastHour     int64 `json:"newLastHour"`
	}
	if err := h.conn.QueryRow(c, `SELECT (SELECT count(*) FROM devices WHERE status = 'pending'),
		(SELECT count(*) FROM ip_requests WHERE status = 'pending'),
		(SELECT count(*) FROM reservations WHERE status = 'active' AND expires_at <= now() + interval '7 days'),
		(SELECT count(*) FROM ip_addresses),
		(SELECT count(*) FROM ip_addresses WHERE status = 'assigned'), (SELECT count(*) FROM ip_addresses WHERE status = 'reserved'),
		(SELECT count(*) FROM ip_addresses WHERE status = 'blocked'),
		(SELECT COALESCE(sum(end_ip - start_ip + 1), 0) FROM reservations WHERE status = 'active'),
		(SELECT count(DISTINCT ip) FROM sightings WHERE last_seen > now() - `+recentSeen+`),
		(SELECT count(*) FROM devices WHERE status <> 'approved'),
		(SELECT count(*) FROM devices WHERE status = 'approved'), (SELECT count(*) FROM devices WHERE status = 'rejected'),
		(SELECT count(DISTINCT ip) FROM sightings WHERE first_seen > now() - interval '1 hour')`).Scan(&totals.PendingDevices, &totals.PendingRequests, &totals.ExpiringHolds,
		&totals.Registered, &totals.Assigned, &totals.Reserved, &totals.Blocked, &totals.Held, &totals.OnlineDevices, &totals.UnknownDevices,
		&totals.Approved, &totals.Rejected, &totals.NewLastHour); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"floors": floors, "alerts": alerts, "servers": servers, "totals": totals, "recentAlerts": recent})
}
