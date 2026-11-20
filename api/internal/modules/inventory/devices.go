package inventory

import (
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platforms"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var deviceTypes = []string{"pc", "laptop", "printer", "phone", "access_point", "network", "server", "cctv", "other"}

type deviceDTO struct {
	ID          string     `db:"id" json:"id"`
	MAC         string     `db:"mac" json:"mac"`
	Name        string     `db:"name" json:"name"`
	Owner       string     `db:"owner" json:"owner"`
	Department  string     `db:"department" json:"department"`
	Type        string     `db:"device_type" json:"deviceType"`
	Platform    string     `db:"platform" json:"platform"`
	Status      string     `db:"status" json:"status"`
	Notes       string     `db:"notes" json:"notes"`
	FirstSeen   *time.Time `db:"first_seen" json:"firstSeen"`
	LastSeen    *time.Time `db:"last_seen" json:"lastSeen"`
	DecidedAt   *time.Time `db:"decided_at" json:"decidedAt"`
	LastIP      *string    `db:"last_ip" json:"lastIp"`
	SegmentID   *string    `db:"segment_id" json:"segmentId"`
	SegmentName *string    `db:"segment_name" json:"segmentName"`
	Hostname    *string    `db:"hostname" json:"hostname"`
	Registered  bool       `db:"registered" json:"registered"`
	Vendor      string     `db:"vendor" json:"vendor"`
	Online      bool       `db:"online" json:"online"`
	// What the scanner last learned about it (evidence, never edited here).
	Source       string  `db:"source" json:"source"`
	NICVendor    string  `db:"nic_vendor" json:"nicVendor"`
	Maker        string  `db:"maker" json:"maker"`
	Model        string  `db:"model" json:"model"`
	DeviceKind   string  `db:"device_kind" json:"deviceKind"`
	OSHint       string  `db:"os_hint" json:"osHint"`
	OpenPorts    []int32 `db:"open_ports" json:"openPorts"`
	IsPrivateMAC bool    `db:"is_private_mac" json:"isPrivateMac"`
	Addresses    int64   `db:"addresses" json:"addresses"`
	RegisteredIP *string `db:"registered_ip" json:"registeredIp"`
	OpenAlerts   int64   `db:"open_alerts" json:"openAlerts"`
}

const deviceFrom = ` FROM devices d
	LEFT JOIN LATERAL (SELECT ip, segment_id, hostname, vendor, source, nic_vendor, maker, model, device_kind, os_hint, open_ports, is_private_mac
		FROM sightings WHERE mac = d.mac ORDER BY last_seen DESC LIMIT 1) v ON true
	LEFT JOIN segments s ON s.id = v.segment_id`

var deviceSorts = map[string]string{"name": "lower(d.name)", "mac": "d.mac", "lastSeen": "d.last_seen", "status": "d.status"}

func (h *Handler) devices(c *gin.Context) {
	var w db.Where
	status := c.Query("status")
	if status != "" {
		var v validate.V
		v.OneOf("status", status, []string{"pending", "approved", "rejected"}, "status")
		if err := v.Err(); err != nil {
			httpx.Fail(c, err)
			return
		}
		w.Add("d.status = ?", status)
	}
	if t := c.Query("deviceType"); t != "" {
		w.Add("d.device_type = ?", t)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(d.mac::text ILIKE ? ESCAPE '\' OR d.name ILIKE ? ESCAPE '\' OR d.owner ILIKE ? ESCAPE '\'
			OR d.department ILIKE ? ESCAPE '\' OR host(v.ip) ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	page := httpx.ParsePage(c)
	order := httpx.Sort(c, deviceSorts, "CASE d.status WHEN 'pending' THEN 0 ELSE 1 END, d.last_seen DESC NULLS LAST, lower(d.name)")
	query := `SELECT d.id::text AS id, d.mac::text AS mac, d.name, d.owner, d.department, d.device_type, d.platform, d.status, d.notes,
		d.first_seen, d.last_seen, d.decided_at, host(v.ip) AS last_ip, v.segment_id::text AS segment_id, s.name AS segment_name,
		NULLIF(v.hostname, '') AS hostname, EXISTS (SELECT 1 FROM ip_addresses i WHERE i.ip = v.ip) AS registered, COALESCE(v.vendor, '') AS vendor,
		COALESCE(d.last_seen > now() - interval '30 minutes', false) AS online,
		COALESCE(v.source, '') AS source, COALESCE(v.nic_vendor, '') AS nic_vendor, COALESCE(v.maker, '') AS maker, COALESCE(v.model, '') AS model,
		COALESCE(v.device_kind, '') AS device_kind, COALESCE(v.os_hint, '') AS os_hint, COALESCE(v.open_ports, '{}') AS open_ports,
		COALESCE(v.is_private_mac, false) AS is_private_mac,
		(SELECT count(DISTINCT x.ip) FROM sightings x WHERE x.mac = d.mac) AS addresses,
		(SELECT host(r.ip) FROM ip_addresses r WHERE r.expected_mac = d.mac OR r.device_id = d.id ORDER BY r.ip LIMIT 1) AS registered_ip,
		(SELECT count(*) FROM alerts a WHERE a.mac = d.mac AND a.status <> 'closed') AS open_alerts` + deviceFrom + w.SQL() + " ORDER BY " + order + ", d.id"
	items, total, err := db.ListPage[deviceDTO](c, h.conn, query, "SELECT count(*)"+deviceFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

type deviceInput struct {
	MAC        string `json:"mac"`
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	Department string `json:"department"`
	Type       string `json:"deviceType"`
	Platform   string `json:"platform"`
	Notes      string `json:"notes"`
}

func (in *deviceInput) validate(withMAC bool) error {
	var v validate.V
	if withMAC {
		v.MAC("mac", &in.MAC, true)
	}
	v.Text("name", &in.Name, 120, true, "Name")
	v.Text("owner", &in.Owner, 120, false, "Owner")
	v.Text("department", &in.Department, 120, false, "Department")
	v.Text("notes", &in.Notes, 1000, false, "Notes")
	if in.Type == "" {
		in.Type = "other"
	}
	v.OneOf("deviceType", in.Type, deviceTypes, "device type")
	if !platforms.ValidPlatform(in.Platform) {
		v.Add("platform", "Choose a listed platform.")
	}
	return v.Err()
}

func (h *Handler) createDevice(c *gin.Context) {
	var in deviceInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(true); err != nil {
		httpx.Fail(c, err)
		return
	}
	var id string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if err := checkDepartment(c, tx, &in.Department); err != nil {
			return err
		}
		if err := tx.QueryRow(c, `INSERT INTO devices (mac, name, owner, department, device_type, platform, status, notes, decided_by, decided_at)
			VALUES ($1::macaddr, $2, $3, $4, $5, $8, 'approved', $6, $7::uuid, now()) RETURNING id::text`,
			in.MAC, in.Name, in.Owner, in.Department, in.Type, in.Notes, actor(c), in.Platform).Scan(&id); err != nil {
			return err
		}
		return record(c, tx, "create", "device", id, nil, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "A device with this MAC address already exists."))
		return
	}
	h.recheck(c)
	httpx.OK(c, http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateDevice(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in deviceInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(false); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		old, err := lockOld(c, tx, "devices", id)
		if err != nil {
			return err
		}
		if err := checkDepartment(c, tx, &in.Department); err != nil {
			return err
		}
		if _, err := tx.Exec(c, `UPDATE devices SET name = $2, owner = $3, department = $4, device_type = $5, notes = $6, platform = $7 WHERE id = $1::uuid`,
			id, in.Name, in.Owner, in.Department, in.Type, in.Notes, in.Platform); err != nil {
			return err
		}
		return record(c, tx, "update", "device", id, old, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	h.recheck(c)
	c.Status(http.StatusNoContent)
}
