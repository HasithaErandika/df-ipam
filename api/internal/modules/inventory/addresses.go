package inventory

import (
	"strings"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

// addressDTO is one address in a segment: what the register says and what the
// scanner saw, kept apart so each shows as its own badge.
type addressDTO struct {
	IP             string     `db:"ip" json:"ip"`
	RegisterID     *string    `db:"register_id" json:"registerId"`
	RegisterStatus *string    `db:"register_status" json:"registerStatus"`
	AssignedName   *string    `db:"assigned_name" json:"assignedName"`
	Owner          *string    `db:"owner" json:"owner"`
	MAC            *string    `db:"mac" json:"mac"`
	Hostname       *string    `db:"hostname" json:"hostname"`
	IsStatic       *bool      `db:"is_static" json:"isStatic"`
	IsPrivateMAC   *bool      `db:"is_private_mac" json:"isPrivateMac"`
	Source         *string    `db:"source" json:"source"`
	FirstSeen      *time.Time `db:"first_seen" json:"firstSeen"`
	LastSeen       *time.Time `db:"last_seen" json:"lastSeen"`
	DeviceID       *string    `db:"device_id" json:"deviceId"`
	DeviceStatus   *string    `db:"device_status" json:"deviceStatus"`
	DeviceName     *string    `db:"device_name" json:"deviceName"`
	Platform       string     `db:"platform" json:"platform"`
	Vendor         string     `db:"vendor" json:"vendor"`
	InPool         bool       `db:"in_pool" json:"inPool"`
}

const addressBase = `WITH seen AS (
	SELECT DISTINCT ON (ip) ip, mac, hostname, vendor, is_static, is_private_mac, source, first_seen, last_seen
	FROM sightings WHERE segment_id = $1::uuid ORDER BY ip, last_seen DESC
), merged AS (
	SELECT COALESCE(r.ip, v.ip) AS addr, host(COALESCE(r.ip, v.ip)) AS ip, r.id::text AS register_id, r.status AS register_status,
		r.assigned_name, r.owner, v.mac::text AS mac, NULLIF(v.hostname, '') AS hostname, v.is_static, v.is_private_mac, v.source,
		v.first_seen, v.last_seen, d.id::text AS device_id, d.status AS device_status, NULLIF(d.name, '') AS device_name, COALESCE(NULLIF(r.platform, ''), d.platform, '') AS platform, COALESCE(v.vendor, '') AS vendor,
		(s.dhcp_start IS NOT NULL AND COALESCE(r.ip, v.ip) BETWEEN s.dhcp_start AND s.dhcp_end) AS in_pool
	FROM (SELECT * FROM ip_addresses WHERE segment_id = $1::uuid) r
	FULL JOIN seen v ON v.ip = r.ip
	JOIN segments s ON s.id = $1::uuid
	LEFT JOIN devices d ON d.mac = v.mac
) `

const addressColumns = `SELECT ip, register_id, register_status, assigned_name, owner, mac, hostname, is_static, is_private_mac, source,
	first_seen, last_seen, device_id, device_status, device_name, platform, vendor, in_pool FROM merged`

// addresses lists every address in a segment that is registered or has been seen.
// state narrows it: unregistered (seen, not in the register), registered, static
// (seen outside the DHCP pool) or unknown (seen with a MAC nobody approved).
func (h *Handler) addresses(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	w := db.Where{Args: []any{id}}
	state := c.Query("state")
	var v validate.V
	if state != "" {
		v.OneOf("state", state, []string{"unregistered", "registered", "static", "unknown"}, "filter")
	}
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	switch state {
	case "unregistered":
		w.Raw("register_id IS NULL")
	case "registered":
		w.Raw("register_id IS NOT NULL")
	case "static":
		w.Raw("last_seen IS NOT NULL AND is_static")
	case "unknown":
		w.Add("mac IS NOT NULL AND NOT is_private_mac AND device_status IS DISTINCT FROM ?", "approved")
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(ip ILIKE ? ESCAPE '\' OR mac ILIKE ? ESCAPE '\' OR hostname ILIKE ? ESCAPE '\' OR assigned_name ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	where := w.SQL()
	page := httpx.ParsePage(c)
	items, total, err := db.ListPage[addressDTO](c, h.conn, addressBase+addressColumns+where+" ORDER BY addr",
		addressBase+"SELECT count(*) FROM merged"+where, w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}
