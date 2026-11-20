package discovery

import (
	"fmt"
	"strings"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

const findingFrom = ` FROM sightings v
	LEFT JOIN segments s ON s.id = v.segment_id
	LEFT JOIN ip_addresses i ON i.ip = v.ip
	LEFT JOIN devices d ON d.mac = v.mac`

const findingColumns = `SELECT v.id::text AS id, host(v.ip) AS ip, v.mac::text AS mac, v.hostname, v.vendor, v.last_seen > now() - ` + recent + ` AS online, v.source, v.is_static, v.is_private_mac,
	COALESCE(v.is_gateway OR v.ip = s.gateway, v.is_gateway) AS is_gateway,
	v.first_seen, v.last_seen, v.segment_id::text AS segment_id, s.name AS segment_name, s.kind AS segment_kind,
	i.id::text AS register_id, i.status AS register_status, NULLIF(i.assigned_name, '') AS assigned_name,
	d.id::text AS device_id, d.status AS device_status, NULLIF(d.name, '') AS device_name,
	COALESCE(NULLIF(i.owner, ''), NULLIF(d.owner, '')) AS owner,
	v.nic_vendor, v.maker, v.model, v.device_kind, v.os_hint, v.open_ports, COALESCE(s.check_ports, false) AS ports_checked,
	COALESCE(` + known + `, false) AS known,
	COALESCE(NULLIF(i.platform, ''), d.platform, '') AS platform,
	v.intune_user, v.intune_user_name, v.intune_device, v.intune_seen_at, v.intune_serial, v.intune_maker, v.intune_model, v.intune_os,
	v.intune_compliance, v.intune_encrypted, v.intune_last_sync,
	v.entra_user, v.entra_user_name, v.entra_device, v.entra_join, v.entra_last_sign_in, v.entra_seen_at, v.workgroup, v.windows_domain, v.windows_fqdn, v.os_version,
	v.vpn_user, v.vpn_kind, v.vpn_tunnel, v.vpn_remote`

// findingPage adds, to one page of rows that is already sorted and cut, the
// device's own register entry elsewhere and the open alert count: looked up for
// the rows shown only, not for every row that matches.
const findingPage = `SELECT page.*, home.ip AS home_ip, home.segment AS home_segment, COALESCE(al.n, 0) AS open_alerts FROM page
	LEFT JOIN LATERAL (SELECT host(r.ip) AS ip, rs.name AS segment FROM ip_addresses r JOIN segments rs ON rs.id = r.segment_id
		WHERE page.register_id IS NULL AND r.ip <> page.ip::inet
		  AND (r.expected_mac = page.mac::macaddr OR r.device_id = page.device_id::uuid) ORDER BY r.ip LIMIT 1) home ON true
	LEFT JOIN LATERAL (SELECT count(*) AS n FROM alerts a WHERE a.ip = page.ip::inet AND a.status <> 'closed') al ON true
	ORDER BY page.rn`

// recent is "answered within two scan intervals", the same window the alert rules use.
const recent = db.SeenWindow

// Finding kinds a person can filter by. Each is a constant condition.
var kinds = map[string]string{
	"unregistered": "i.id IS NULL",
	"registered":   "i.id IS NOT NULL",
	"static":       "v.is_static AND s.kind = 'dhcp'",
	"online":       "v.last_seen > now() - " + recent,
	"offline":      "v.last_seen <= now() - " + recent,
	"dhcp":         "v.source = 'dhcp'",
	"vpn":          "v.source = 'vpn'",
	"unknown":      undecided,
	"known":        known,
	"rejected":     "d.status = 'rejected'",
	"private":      "v.is_private_mac",
	"gateway":      "(v.is_gateway OR v.ip IS NOT DISTINCT FROM s.gateway)",
	"alerting":     alerting,
	"needs":        "(" + alerting + " OR " + undecided + ")",
	"new":          "v.first_seen > now() - " + newWindow,
}

// alerting, undecided, known and newWindow are shared with the summary counts.
// Known: registered here; or an approved device on a DHCP pool address, which
// is never registered as its address changes; or an approved device registered
// at another address, such as a laptop that moved to another Wi-Fi network; or
// a VPN session the FortiGate signed a user in to.
const (
	known = "((i.id IS NOT NULL AND (v.mac IS NULL OR d.status = 'approved')) OR (i.id IS NULL AND d.status = 'approved' AND (" + dhcpClient + " OR EXISTS (" + homeWhere + ")))" +
		" OR (v.source = 'vpn' AND v.vpn_user <> ''))"
	// dhcpClient is an address the DHCP server leases: in a DHCP segment, inside its pool.
	dhcpClient = "(s.kind = 'dhcp' AND NOT v.is_static)"
	// homeWhere finds the device's own register entry, by its MAC or its device.
	homeWhere = "SELECT 1 FROM ip_addresses r WHERE r.ip <> v.ip AND (r.expected_mac = v.mac OR r.device_id = d.id)"
	alerting  = "EXISTS (SELECT 1 FROM alerts a WHERE a.ip = v.ip AND a.status <> 'closed')"
	undecided = "(v.mac IS NOT NULL AND (NOT v.is_private_mac OR d.id IS NOT NULL) AND d.status IS DISTINCT FROM 'approved')"
	newWindow = "interval '1 hour'"
)

var windows = map[string]string{"1h": "1 hour", "24h": "24 hours", "7d": "7 days", "30d": "30 days"}

var findingSorts = map[string]string{"ip": "v.ip", "lastSeen": "v.last_seen", "firstSeen": "v.first_seen", "hostname": "lower(v.hostname)", "segment": "lower(s.name)"}

func findingFilter(c *gin.Context) (db.Where, error) {
	var w db.Where
	var v validate.V
	if id := c.Query("id"); id != "" {
		v.UUID("id", id, true, "a finding")
		w.Add("v.id = ?::uuid", id)
	}
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
	seenFilter(c.DefaultQuery("seen", "now"), &w, &v)
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(host(v.ip) ILIKE ? ESCAPE '\' OR v.mac::text ILIKE ? ESCAPE '\' OR v.hostname ILIKE ? ESCAPE '\'
			OR i.assigned_name ILIKE ? ESCAPE '\' OR d.name ILIKE ? ESCAPE '\' OR v.vendor ILIKE ? ESCAPE '\'
			OR v.model ILIKE ? ESCAPE '\' OR v.maker ILIKE ? ESCAPE '\'
			OR v.intune_user ILIKE ? ESCAPE '\' OR v.intune_user_name ILIKE ? ESCAPE '\' OR v.intune_serial ILIKE ? ESCAPE '\'
			OR v.entra_user ILIKE ? ESCAPE '\' OR v.entra_user_name ILIKE ? ESCAPE '\'
			OR v.workgroup ILIKE ? ESCAPE '\' OR v.windows_domain ILIKE ? ESCAPE '\' OR v.vpn_user ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	return w, v.Err()
}

// seenFilter limits to a time window. "now" is what is connected: it answered
// within the last two scans. A device that leaves drops out of this view; longer
// windows keep the history. Problems are added to v.
func seenFilter(seen string, w *db.Where, v *validate.V) {
	switch seen {
	case "all":
	case "now":
		w.Raw("v.last_seen > now() - " + recent)
	default:
		span, ok := windows[seen]
		if !ok {
			v.Add("seen", "Choose a valid time window.")
			return
		}
		w.Add("v.last_seen > now() - ?::interval", span)
	}
}

func (h *Handler) findings(c *gin.Context) {
	w, err := findingFilter(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	page := httpx.ParsePage(c)
	order := httpx.Sort(c, findingSorts, "v.last_seen DESC") + ", v.ip, v.id"
	var total int64
	if err := h.conn.QueryRow(c, "SELECT count(*)"+findingFrom+w.SQL(), w.Args...).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	n := len(w.Args)
	query := fmt.Sprintf("WITH page AS (%s, row_number() OVER (ORDER BY %s) AS rn%s%s ORDER BY %s LIMIT $%d OFFSET $%d) %s",
		findingColumns, order, findingFrom, w.SQL(), order, n+1, n+2, findingPage)
	items, err := db.All[findingDTO](c, h.conn, query, append(append([]any{}, w.Args...), page.Size, page.Offset())...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}
