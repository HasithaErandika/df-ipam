package inventory

import (
	"strings"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platforms"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

var (
	ipTypes    = []string{"server", "printer", "network", "cctv", "access_point", "user_pc", "other"}
	ipStatuses = []string{"assigned", "reserved", "blocked"}
)

type ipDTO struct {
	ID               string     `db:"id" json:"id"`
	SegmentID        string     `db:"segment_id" json:"segmentId"`
	SegmentName      string     `db:"segment_name" json:"segmentName"`
	SegmentKind      string     `db:"segment_kind" json:"segmentKind"`
	IP               string     `db:"ip" json:"ip"`
	Status           string     `db:"status" json:"status"`
	Type             string     `db:"ip_type" json:"ipType"`
	EnvironmentID    *string    `db:"environment_id" json:"environmentId"`
	EnvironmentName  *string    `db:"environment_name" json:"environmentName"`
	EnvironmentColor *string    `db:"environment_colour" json:"environmentColour"`
	DeviceID         *string    `db:"device_id" json:"deviceId"`
	Name             string     `db:"assigned_name" json:"assignedName"`
	Platform         string     `db:"platform" json:"platform"`
	Services         []string   `db:"services" json:"services"`
	MAC              *string    `db:"expected_mac" json:"expectedMac"`
	Owner            string     `db:"owner" json:"owner"`
	Department       string     `db:"department" json:"department"`
	Monitor          bool       `db:"monitor" json:"monitor"`
	Critical         bool       `db:"critical" json:"critical"`
	Port             *int       `db:"check_port" json:"checkPort"`
	Notes            string     `db:"notes" json:"notes"`
	Health           string     `db:"health" json:"health"`
	HealthChangedAt  *time.Time `db:"health_changed_at" json:"healthChangedAt"`
	ResponseMs       *int32     `db:"response_ms" json:"responseMs"`
	LastSeen         *time.Time `db:"last_seen" json:"lastSeen"`
	SeenMAC          *string    `db:"seen_mac" json:"seenMac"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updatedAt"`
	CreatedAt        time.Time  `db:"created_at" json:"createdAt"`
	CheckFailures    int        `db:"server_check_failures" json:"checkFailures"`
	DeviceName       *string    `db:"device_name" json:"deviceName"`
	// What the scanner last learned at this address (evidence only).
	Hostname   string  `db:"hostname" json:"hostname"`
	NICVendor  string  `db:"nic_vendor" json:"nicVendor"`
	Maker      string  `db:"maker" json:"maker"`
	Model      string  `db:"model" json:"model"`
	DeviceKind string  `db:"device_kind" json:"deviceKind"`
	OSHint     string  `db:"os_hint" json:"osHint"`
	Vendor     string  `db:"vendor" json:"vendor"`
	Source     string  `db:"source" json:"source"`
	OpenPorts  []int32 `db:"open_ports" json:"openPorts"`
	OpenAlerts int64   `db:"open_alerts" json:"openAlerts"`
}

const ipFrom = ` FROM ip_addresses i JOIN segments s ON s.id = i.segment_id
	LEFT JOIN environments e ON e.id = i.environment_id
	LEFT JOIN devices dv ON dv.id = i.device_id
	LEFT JOIN LATERAL (SELECT last_seen, mac, hostname, nic_vendor, maker, model, device_kind, os_hint, vendor, source, open_ports
		FROM sightings WHERE ip = i.ip ORDER BY last_seen DESC LIMIT 1) v ON true`

const ipColumns = `SELECT i.id::text AS id, i.segment_id::text AS segment_id, s.name AS segment_name, s.kind AS segment_kind,
	host(i.ip) AS ip, i.status, i.ip_type, i.environment_id::text AS environment_id, e.name AS environment_name, i.platform, i.services,
	e.colour AS environment_colour, i.device_id::text AS device_id, i.assigned_name, i.expected_mac::text AS expected_mac,
	i.owner, i.department, i.monitor, i.critical, i.check_port, i.notes, i.health, i.health_changed_at, i.response_ms,
	v.last_seen, v.mac::text AS seen_mac, i.updated_at, i.created_at, i.server_check_failures, NULLIF(dv.name, '') AS device_name,
	COALESCE(v.hostname, '') AS hostname, COALESCE(v.nic_vendor, '') AS nic_vendor, COALESCE(v.maker, '') AS maker, COALESCE(v.model, '') AS model,
	COALESCE(v.device_kind, '') AS device_kind, COALESCE(v.os_hint, '') AS os_hint, COALESCE(v.vendor, '') AS vendor, COALESCE(v.source, '') AS source,
	COALESCE(v.open_ports, '{}') AS open_ports, (SELECT count(*) FROM alerts a WHERE a.ip = i.ip AND a.status <> 'closed') AS open_alerts`

var ipSorts = map[string]string{"ip": "i.ip", "name": "lower(i.assigned_name)", "health": "i.health", "segment": "lower(s.name)", "updated": "i.updated_at"}

// registerFilter reads the list filters. "view=servers" is the Servers screen:
// entries in server segments plus anything marked Monitor.
func registerFilter(c *gin.Context) (db.Where, error) {
	var w db.Where
	var v validate.V
	if id := c.Query("segmentId"); id != "" {
		v.UUID("segmentId", id, true, "a segment")
		w.Add("i.segment_id = ?::uuid", id)
	}
	if id := c.Query("environmentId"); id != "" {
		v.UUID("environmentId", id, true, "an environment")
		w.Add("i.environment_id = ?::uuid", id)
	}
	if s := c.Query("status"); s != "" {
		v.OneOf("status", s, ipStatuses, "status")
		w.Add("i.status = ?", s)
	}
	if hl := c.Query("health"); hl != "" {
		v.OneOf("health", hl, []string{"up", "down", "unknown"}, "health")
		w.Add("i.health = ?", hl)
	}
	if t := c.Query("ipType"); t != "" {
		v.OneOf("ipType", t, ipTypes, "IP type")
		w.Add("i.ip_type = ?", t)
	}
	if c.Query("view") == "servers" {
		w.Raw("(s.kind IN ('server', 'cloud') OR i.monitor)")
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(host(i.ip) ILIKE ? ESCAPE '\' OR i.assigned_name ILIKE ? ESCAPE '\' OR i.owner ILIKE ? ESCAPE '\'
			OR i.department ILIKE ? ESCAPE '\' OR i.expected_mac::text ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	return w, v.Err()
}

func (h *Handler) register(c *gin.Context) {
	w, err := registerFilter(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	page := httpx.ParsePage(c)
	order := httpx.Sort(c, ipSorts, "i.ip")
	items, total, err := db.ListPage[ipDTO](c, h.conn, ipColumns+ipFrom+w.SQL()+" ORDER BY "+order+", i.id", "SELECT count(*)"+ipFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

type ipInput struct {
	SegmentID     string   `json:"segmentId"`
	IP            string   `json:"ip"`
	Status        string   `json:"status"`
	Type          string   `json:"ipType"`
	EnvironmentID string   `json:"environmentId"`
	DeviceID      string   `json:"deviceId"`
	Name          string   `json:"assignedName"`
	Platform      string   `json:"platform"`
	Services      []string `json:"services"`
	MAC           string   `json:"expectedMac"`
	Owner         string   `json:"owner"`
	Department    string   `json:"department"`
	Monitor       bool     `json:"monitor"`
	Critical      bool     `json:"critical"`
	Port          *int     `json:"checkPort"`
	Notes         string   `json:"notes"`
}

func (in *ipInput) validate(create bool) error {
	var v validate.V
	if create {
		v.UUID("segmentId", in.SegmentID, true, "a segment")
		v.IPv4("ip", &in.IP, true)
	}
	if in.Status == "" {
		in.Status = "assigned"
	}
	if in.Type == "" {
		in.Type = "other"
	}
	v.OneOf("status", in.Status, ipStatuses, "status")
	v.OneOf("ipType", in.Type, ipTypes, "type")
	v.UUID("environmentId", in.EnvironmentID, false, "an environment")
	v.UUID("deviceId", in.DeviceID, false, "a device")
	v.Text("assignedName", &in.Name, 120, in.Status == "assigned", "Assigned to")
	v.MAC("expectedMac", &in.MAC, false)
	v.Text("owner", &in.Owner, 120, false, "Owner")
	v.Text("department", &in.Department, 120, false, "Department")
	v.IntRange("checkPort", in.Port, 1, 65535, "Check port")
	v.Text("notes", &in.Notes, 1000, false, "Notes")
	if !platforms.ValidPlatform(in.Platform) {
		v.Add("platform", "Choose a listed platform.")
	}
	services, unknown := platforms.CleanServices(in.Services)
	if unknown != "" {
		v.Add("services", "Choose listed services only.")
	}
	in.Services = services
	if in.Critical && !in.Monitor {
		v.Add("critical", "Turn on Monitor to mark an entry critical.")
	}
	return v.Err()
}
