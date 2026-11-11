package discovery

import (
	"strings"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

// findingDTO is one IP and MAC the scanner saw. Register and device state come
// alongside, as separate fields: evidence never stands in for the register.
type findingDTO struct {
	ID             string    `db:"id" json:"id"`
	IP             string    `db:"ip" json:"ip"`
	MAC            *string   `db:"mac" json:"mac"`
	Hostname       string    `db:"hostname" json:"hostname"`
	Vendor         string    `db:"vendor" json:"vendor"`
	Online         bool      `db:"online" json:"online"`
	Source         string    `db:"source" json:"source"`
	IsStatic       bool      `db:"is_static" json:"isStatic"`
	IsPrivateMAC   bool      `db:"is_private_mac" json:"isPrivateMac"`
	IsGateway      bool      `db:"is_gateway" json:"isGateway"`
	FirstSeen      time.Time `db:"first_seen" json:"firstSeen"`
	LastSeen       time.Time `db:"last_seen" json:"lastSeen"`
	SegmentID      *string   `db:"segment_id" json:"segmentId"`
	SegmentName    *string   `db:"segment_name" json:"segmentName"`
	SegmentKind    *string   `db:"segment_kind" json:"segmentKind"`
	RegisterID     *string   `db:"register_id" json:"registerId"`
	RegisterStatus *string   `db:"register_status" json:"registerStatus"`
	AssignedName   *string   `db:"assigned_name" json:"assignedName"`
	DeviceID       *string   `db:"device_id" json:"deviceId"`
	DeviceStatus   *string   `db:"device_status" json:"deviceStatus"`
	DeviceName     *string   `db:"device_name" json:"deviceName"`
	Platform       string    `db:"platform" json:"platform"`
	OpenAlerts     int64     `db:"open_alerts" json:"openAlerts"`
}

const findingFrom = ` FROM sightings v
	LEFT JOIN segments s ON s.id = v.segment_id
	LEFT JOIN ip_addresses i ON i.ip = v.ip
	LEFT JOIN devices d ON d.mac = v.mac`

const findingColumns = `SELECT v.id::text AS id, host(v.ip) AS ip, v.mac::text AS mac, v.hostname, v.vendor, v.last_seen > now() - ` + recent + ` AS online, v.source, v.is_static, v.is_private_mac,
	COALESCE(v.is_gateway OR v.ip = s.gateway, v.is_gateway) AS is_gateway,
	v.first_seen, v.last_seen, v.segment_id::text AS segment_id, s.name AS segment_name, s.kind AS segment_kind,
	i.id::text AS register_id, i.status AS register_status, NULLIF(i.assigned_name, '') AS assigned_name,
	d.id::text AS device_id, d.status AS device_status, NULLIF(d.name, '') AS device_name,
	COALESCE(NULLIF(i.platform, ''), d.platform, '') AS platform,
	(SELECT count(*) FROM alerts a WHERE a.ip = v.ip AND a.status <> 'closed') AS open_alerts`

// recent is "answered within two scan intervals", the same window the alert rules use.
const recent = `make_interval(mins => GREATEST(10, 2 * COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), 10)))`

// Finding kinds a person can filter by. Each is a constant condition.
var kinds = map[string]string{
	"unregistered": "i.id IS NULL",
	"registered":   "i.id IS NOT NULL",
	"static":       "v.is_static AND s.kind = 'dhcp'",
	"online":       "v.last_seen > now() - " + recent,
	"offline":      "v.last_seen <= now() - " + recent,
	"dhcp":         "v.source = 'dhcp'",
	"vpn":          "v.source = 'vpn'",
	"unknown":      "v.mac IS NOT NULL AND NOT v.is_private_mac AND d.status IS DISTINCT FROM 'approved'",
	"rejected":     "d.status = 'rejected'",
	"private":      "v.is_private_mac",
	"gateway":      "(v.is_gateway OR v.ip IS NOT DISTINCT FROM s.gateway)",
	"alerting":     "EXISTS (SELECT 1 FROM alerts a WHERE a.ip = v.ip AND a.status <> 'closed')",
}

var windows = map[string]string{"1h": "1 hour", "24h": "24 hours", "7d": "7 days", "30d": "30 days"}

var findingSorts = map[string]string{"ip": "v.ip", "lastSeen": "v.last_seen", "firstSeen": "v.first_seen", "hostname": "lower(v.hostname)", "segment": "lower(s.name)"}

func findingFilter(c *gin.Context) (db.Where, error) {
	var w db.Where
	var v validate.V
	if id := c.Query("segmentId"); id != "" {
		v.UUID("segmentId", id, true, "a segment")
		w.Add("v.segment_id = ?::uuid", id)
	}
	if k := c.Query("kind"); k != "" {
		if cond, ok := kinds[k]; ok {
			w.Raw(cond)
		} else {
			v.Add("kind", "Choose a valid finding type.")
		}
	}
	if src := c.Query("source"); src != "" {
		v.OneOf("source", src, []string{"arp", "ping", "netbios", "snmp", "dns", "dhcp", "vpn"}, "source")
		w.Add("v.source = ?", src)
	}
	// "now" is what is connected: it answered within the last two scans. A device
	// that leaves drops out of this view; longer windows keep the history.
	switch seen := c.DefaultQuery("seen", "now"); seen {
	case "all":
	case "now":
		w.Raw("v.last_seen > now() - " + recent)
	default:
		span, ok := windows[seen]
		if !ok {
			v.Add("seen", "Choose a valid time window.")
		}
		w.Add("v.last_seen > now() - ?::interval", span)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(host(v.ip) ILIKE ? ESCAPE '\' OR v.mac::text ILIKE ? ESCAPE '\' OR v.hostname ILIKE ? ESCAPE '\'
			OR i.assigned_name ILIKE ? ESCAPE '\' OR d.name ILIKE ? ESCAPE '\' OR v.vendor ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	return w, v.Err()
}

func (h *Handler) findings(c *gin.Context) {
	w, err := findingFilter(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	page := httpx.ParsePage(c)
	order := httpx.Sort(c, findingSorts, "v.last_seen DESC")
	items, total, err := db.ListPage[findingDTO](c, h.conn, findingColumns+findingFrom+w.SQL()+" ORDER BY "+order+", v.ip, v.id",
		"SELECT count(*)"+findingFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}
