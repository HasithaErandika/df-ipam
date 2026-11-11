// Package inventory implements the Lite segment, device and IP register API.
package inventory

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn: conn} }

func (h *Handler) Register(g *auth.Guard) {
	read, edit := auth.InventoryView, auth.InventoryEdit
	for _, r := range []struct {
		method, path string
		fn           gin.HandlerFunc
		permission   string
	}{
		{http.MethodGet, "/segments", h.segments, read}, {http.MethodPost, "/segments", h.createSegment, edit},
		{http.MethodPut, "/segments/:id", h.updateSegment, edit}, {http.MethodDelete, "/segments/:id", h.deleteSegment, edit},
		{http.MethodGet, "/devices", h.devices, read}, {http.MethodPost, "/devices", h.createDevice, edit},
		{http.MethodPut, "/devices/:id", h.updateDevice, edit}, {http.MethodPost, "/devices/:id/decision", h.decideDevice, edit},
		{http.MethodGet, "/devices/export.csv", h.exportDevices, edit}, {http.MethodPost, "/devices/import.csv", h.importDevices, edit},
		{http.MethodGet, "/ip-addresses", h.ips, read}, {http.MethodPost, "/ip-addresses", h.createIP, edit},
		{http.MethodPut, "/ip-addresses/:id", h.updateIP, edit}, {http.MethodDelete, "/ip-addresses/:id", h.releaseIP, edit},
		{http.MethodGet, "/ip-addresses/available-candidate", h.candidate, edit},
	} {
		g.Handle(r.method, r.path, r.fn, r.permission)
	}
}

type segment struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Kind                 string  `json:"kind"`
	Floor                *int    `json:"floor"`
	VLAN                 *int    `json:"vlan"`
	Network              string  `json:"network"`
	Gateway              *string `json:"gateway"`
	DHCPStart            *string `json:"dhcpStart"`
	DHCPEnd              *string `json:"dhcpEnd"`
	DefaultEnvironmentID *string `json:"defaultEnvironmentId"`
	Description          string  `json:"description"`
}

func actor(c *gin.Context) string                 { return auth.Current(c).UserID }
func fail(c *gin.Context, err error)              { httpx.Fail(c, err) }
func invalid(c *gin.Context, field, issue string) { fail(c, httpx.Validation(field, issue)) }
func id(c *gin.Context) (string, bool)            { return httpx.PathID(c) }

func (h *Handler) segments(c *gin.Context) {
	rows, err := h.conn.Query(c, `SELECT id,name,kind,floor,vlan,network::text,gateway::text,dhcp_start::text,dhcp_end::text,default_environment_id,description FROM segments ORDER BY kind,floor NULLS LAST,name`)
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	out := []segment{}
	for rows.Next() {
		var x segment
		if err := rows.Scan(&x.ID, &x.Name, &x.Kind, &x.Floor, &x.VLAN, &x.Network, &x.Gateway, &x.DHCPStart, &x.DHCPEnd, &x.DefaultEnvironmentID, &x.Description); err != nil {
			fail(c, err)
			return
		}
		out = append(out, x)
	}
	if rows.Err() != nil {
		fail(c, rows.Err())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func segmentBody(c *gin.Context) (segment, bool) {
	var x segment
	if c.ShouldBindJSON(&x) != nil {
		fail(c, httpx.ErrBadRequest)
		return x, false
	}
	x.Name = strings.TrimSpace(x.Name)
	if x.Name == "" {
		invalid(c, "name", "Enter a segment name.")
		return x, false
	}
	if x.Kind != "dhcp" && x.Kind != "server" {
		invalid(c, "kind", "Choose dhcp or server.")
		return x, false
	}
	if x.Network == "" {
		invalid(c, "network", "Enter an IPv4 network in CIDR notation.")
		return x, false
	}
	return x, true
}

func (h *Handler) saveSegment(c *gin.Context, update bool) {
	x, ok := segmentBody(c)
	if !ok {
		return
	}
	uid := actor(c)
	objectID := ""
	if update {
		objectID, ok = id(c)
		if !ok {
			return
		}
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old any
		if update {
			var b []byte
			if e := tx.QueryRow(c, "SELECT to_jsonb(s) FROM segments s WHERE id=$1::uuid FOR UPDATE", objectID).Scan(&b); e != nil {
				return e
			}
			old = json.RawMessage(b)
			_, e := tx.Exec(c, `UPDATE segments SET name=$2,kind=$3,floor=$4,vlan=$5,network=$6::cidr,gateway=$7::inet,dhcp_start=$8::inet,dhcp_end=$9::inet,default_environment_id=$10::uuid,description=$11,updated_by=$12::uuid WHERE id=$1::uuid`, objectID, x.Name, x.Kind, x.Floor, x.VLAN, x.Network, x.Gateway, x.DHCPStart, x.DHCPEnd, x.DefaultEnvironmentID, x.Description, uid)
			if e != nil {
				return e
			}
		} else if e := tx.QueryRow(c, `INSERT INTO segments(name,kind,floor,vlan,network,gateway,dhcp_start,dhcp_end,default_environment_id,description,created_by,updated_by) VALUES($1,$2,$3,$4,$5::cidr,$6::inet,$7::inet,$8::inet,$9::uuid,$10,$11::uuid,$11::uuid) RETURNING id`, x.Name, x.Kind, x.Floor, x.VLAN, x.Network, x.Gateway, x.DHCPStart, x.DHCPEnd, x.DefaultEnvironmentID, x.Description, uid).Scan(&objectID); e != nil {
			return e
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: map[bool]string{true: "update", false: "create"}[update], ObjectType: "segment", ObjectID: objectID, Old: old, New: x})
	})
	if err != nil {
		fail(c, conflict(err))
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"id": objectID})
}

func (h *Handler) createSegment(c *gin.Context) { h.saveSegment(c, false) }
func (h *Handler) updateSegment(c *gin.Context) { h.saveSegment(c, true) }
func (h *Handler) deleteSegment(c *gin.Context) {
	key, ok := id(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		if e := tx.QueryRow(c, "SELECT to_jsonb(s) FROM segments s WHERE id=$1::uuid FOR UPDATE", key).Scan(&old); e != nil {
			return e
		}
		if _, e := tx.Exec(c, "DELETE FROM segments WHERE id=$1::uuid", key); e != nil {
			return e
		}
		return audit.Write(c, tx, audit.Entry{ActorID: actor(c), Action: "delete", ObjectType: "segment", ObjectID: key, Old: json.RawMessage(old)})
	})
	if err != nil {
		fail(c, conflict(err))
		return
	}
	c.Status(http.StatusNoContent)
}

type device struct {
	ID         string     `json:"id"`
	MAC        string     `json:"mac"`
	Name       string     `json:"name"`
	Owner      string     `json:"owner"`
	Department string     `json:"department"`
	Type       string     `json:"deviceType"`
	Status     string     `json:"status"`
	Notes      string     `json:"notes"`
	FirstSeen  *time.Time `json:"firstSeen"`
	LastSeen   *time.Time `json:"lastSeen"`
}

func (h *Handler) devices(c *gin.Context) {
	status := c.Query("status")
	if status != "" && status != "pending" && status != "approved" && status != "rejected" {
		invalid(c, "status", "Choose pending, approved or rejected.")
		return
	}
	rows, e := h.conn.Query(c, `SELECT id,mac::text,name,owner,department,device_type,status,notes,first_seen,last_seen FROM devices WHERE ($1='' OR status=$1) ORDER BY status,name,mac`, status)
	if e != nil {
		fail(c, e)
		return
	}
	defer rows.Close()
	out := []device{}
	for rows.Next() {
		var x device
		if e := rows.Scan(&x.ID, &x.MAC, &x.Name, &x.Owner, &x.Department, &x.Type, &x.Status, &x.Notes, &x.FirstSeen, &x.LastSeen); e != nil {
			fail(c, e)
			return
		}
		out = append(out, x)
	}
	if rows.Err() != nil {
		fail(c, rows.Err())
		return
	}
	c.JSON(200, gin.H{"data": out})
}
func (h *Handler) createDevice(c *gin.Context) {
	var x device
	if c.ShouldBindJSON(&x) != nil {
		fail(c, httpx.ErrBadRequest)
		return
	}
	x.MAC = strings.ToLower(strings.TrimSpace(x.MAC))
	x.Name = strings.TrimSpace(x.Name)
	if x.MAC == "" {
		invalid(c, "mac", "Enter a MAC address.")
		return
	}
	uid := actor(c)
	var key string
	e := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if z := tx.QueryRow(c, `INSERT INTO devices(mac,name,owner,department,device_type,status,notes,decided_by,decided_at) VALUES($1::macaddr,$2,$3,$4,$5,'approved',$6,$7::uuid,now()) RETURNING id`, x.MAC, x.Name, x.Owner, x.Department, defaultType(x.Type), x.Notes, uid).Scan(&key); z != nil {
			return z
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "create", ObjectType: "device", ObjectID: key, New: x})
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	httpx.OK(c, 201, gin.H{"id": key})
}
func defaultType(s string) string {
	if s == "" {
		return "other"
	}
	return s
}
func (h *Handler) updateDevice(c *gin.Context) {
	key, ok := id(c)
	if !ok {
		return
	}
	var x device
	if c.ShouldBindJSON(&x) != nil {
		fail(c, httpx.ErrBadRequest)
		return
	}
	uid := actor(c)
	e := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		if z := tx.QueryRow(c, "SELECT to_jsonb(d) FROM devices d WHERE id=$1::uuid FOR UPDATE", key).Scan(&old); z != nil {
			return z
		}
		if _, z := tx.Exec(c, `UPDATE devices SET name=$2,owner=$3,department=$4,device_type=$5,notes=$6 WHERE id=$1::uuid`, key, x.Name, x.Owner, x.Department, defaultType(x.Type), x.Notes); z != nil {
			return z
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "update", ObjectType: "device", ObjectID: key, Old: json.RawMessage(old), New: x})
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	c.Status(204)
}
func (h *Handler) decideDevice(c *gin.Context) {
	key, ok := id(c)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if c.ShouldBindJSON(&in) != nil || (in.Status != "approved" && in.Status != "rejected") {
		invalid(c, "status", "Choose approved or rejected.")
		return
	}
	uid := actor(c)
	e := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		if z := tx.QueryRow(c, "SELECT to_jsonb(d) FROM devices d WHERE id=$1::uuid FOR UPDATE", key).Scan(&old); z != nil {
			return z
		}
		if _, z := tx.Exec(c, "UPDATE devices SET status=$2,decided_by=$3::uuid,decided_at=now() WHERE id=$1::uuid", key, in.Status, uid); z != nil {
			return z
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: in.Status, ObjectType: "device", ObjectID: key, Old: json.RawMessage(old), New: in})
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	c.Status(204)
}

type ipEntry struct {
	ID         string  `json:"id"`
	SegmentID  string  `json:"segmentId"`
	IP         string  `json:"ip"`
	Status     string  `json:"status"`
	Type       string  `json:"ipType"`
	Name       string  `json:"assignedName"`
	MAC        *string `json:"expectedMac"`
	Owner      string  `json:"owner"`
	Department string  `json:"department"`
	Monitor    bool    `json:"monitor"`
	Critical   bool    `json:"critical"`
	Port       *int    `json:"checkPort"`
	Notes      string  `json:"notes"`
	Health     string  `json:"health"`
}

func (h *Handler) ips(c *gin.Context) {
	rows, e := h.conn.Query(c, `SELECT id,segment_id,ip::text,status,ip_type,assigned_name,expected_mac::text,owner,department,monitor,critical,check_port,notes,health FROM ip_addresses WHERE ($1='' OR segment_id=$1::uuid) ORDER BY ip`, c.Query("segmentId"))
	if e != nil {
		fail(c, e)
		return
	}
	defer rows.Close()
	out := []ipEntry{}
	for rows.Next() {
		var x ipEntry
		if e = rows.Scan(&x.ID, &x.SegmentID, &x.IP, &x.Status, &x.Type, &x.Name, &x.MAC, &x.Owner, &x.Department, &x.Monitor, &x.Critical, &x.Port, &x.Notes, &x.Health); e != nil {
			fail(c, e)
			return
		}
		out = append(out, x)
	}
	if rows.Err() != nil {
		fail(c, rows.Err())
		return
	}
	c.JSON(200, gin.H{"data": out})
}
func (h *Handler) createIP(c *gin.Context) {
	var x ipEntry
	if c.ShouldBindJSON(&x) != nil {
		fail(c, httpx.ErrBadRequest)
		return
	}
	if x.SegmentID == "" || x.IP == "" {
		invalid(c, "ip", "Choose a segment and enter an IP address.")
		return
	}
	if x.Status == "" {
		x.Status = "assigned"
	}
	if x.Type == "" {
		x.Type = "other"
	}
	uid := actor(c)
	var key string
	e := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		z := tx.QueryRow(c, `INSERT INTO ip_addresses(segment_id,ip,status,ip_type,assigned_name,expected_mac,owner,department,monitor,critical,check_port,notes,created_by,updated_by) VALUES($1::uuid,$2::inet,$3,$4,$5,$6::macaddr,$7,$8,$9,$10,$11,$12,$13::uuid,$13::uuid) RETURNING id`, x.SegmentID, x.IP, x.Status, x.Type, x.Name, x.MAC, x.Owner, x.Department, x.Monitor, x.Critical, x.Port, x.Notes, uid).Scan(&key)
		if z != nil {
			return z
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "create", ObjectType: "ip_address", ObjectID: key, New: x})
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	httpx.OK(c, 201, gin.H{"id": key})
}
func (h *Handler) updateIP(c *gin.Context) {
	key, ok := id(c)
	if !ok {
		return
	}
	var x ipEntry
	if c.ShouldBindJSON(&x) != nil {
		fail(c, httpx.ErrBadRequest)
		return
	}
	uid := actor(c)
	e := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		if z := tx.QueryRow(c, "SELECT to_jsonb(i) FROM ip_addresses i WHERE id=$1::uuid FOR UPDATE", key).Scan(&old); z != nil {
			return z
		}
		_, z := tx.Exec(c, `UPDATE ip_addresses SET status=$2,ip_type=$3,assigned_name=$4,expected_mac=$5::macaddr,owner=$6,department=$7,monitor=$8,critical=$9,check_port=$10,notes=$11,updated_by=$12::uuid WHERE id=$1::uuid`, key, x.Status, x.Type, x.Name, x.MAC, x.Owner, x.Department, x.Monitor, x.Critical, x.Port, x.Notes, uid)
		if z != nil {
			return z
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "update", ObjectType: "ip_address", ObjectID: key, Old: json.RawMessage(old), New: x})
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	c.Status(204)
}
func (h *Handler) releaseIP(c *gin.Context) {
	key, ok := id(c)
	if !ok {
		return
	}
	uid := actor(c)
	e := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		if z := tx.QueryRow(c, "SELECT to_jsonb(i) FROM ip_addresses i WHERE id=$1::uuid FOR UPDATE", key).Scan(&old); z != nil {
			return z
		}
		if _, z := tx.Exec(c, "DELETE FROM ip_addresses WHERE id=$1::uuid", key); z != nil {
			return z
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "release", ObjectType: "ip_address", ObjectID: key, Old: json.RawMessage(old)})
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	c.Status(204)
}

func (h *Handler) reserve(c *gin.Context) {
	var x struct{ SegmentID, StartIP, EndIP, Reason, OwnerName, ExpiresAt string }
	if c.ShouldBindJSON(&x) != nil {
		fail(c, httpx.ErrBadRequest)
		return
	}
	if x.SegmentID == "" || x.StartIP == "" || x.Reason == "" {
		invalid(c, "reason", "Segment, IP range and reason are required.")
		return
	}
	uid := actor(c)
	var key string
	e := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		z := tx.QueryRow(c, `INSERT INTO reservations(segment_id,start_ip,end_ip,reason,owner_name,expires_at,created_by,updated_by) VALUES($1::uuid,$2::inet,$3::inet,$4,$5,NULLIF($6,'')::timestamptz,$7::uuid,$7::uuid) RETURNING id`, x.SegmentID, x.StartIP, x.EndIP, x.Reason, x.OwnerName, x.ExpiresAt, uid).Scan(&key)
		if z != nil {
			return z
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "reserve", ObjectType: "reservation", ObjectID: key, New: x})
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	httpx.OK(c, 201, gin.H{"id": key})
}
func (h *Handler) candidate(c *gin.Context) {
	var cidr string
	if e := h.conn.QueryRow(c, "SELECT network::text FROM segments WHERE id=$1::uuid", c.Query("segmentId")).Scan(&cidr); e != nil {
		fail(c, httpx.ErrNotFound)
		return
	}
	prefix, e := netip.ParsePrefix(cidr)
	if e != nil {
		fail(c, e)
		return
	}
	usable := netcalc.UsableRange(prefix)
	rows, e := h.conn.Query(c, `SELECT ip::text,ip::text FROM ip_addresses WHERE segment_id=$1::uuid UNION ALL SELECT start_ip::text,end_ip::text FROM reservations WHERE segment_id=$1::uuid AND status='active' ORDER BY 1`, c.Query("segmentId"))
	if e != nil {
		fail(c, e)
		return
	}
	defer rows.Close()
	var used []netcalc.Range
	for rows.Next() {
		var a, b string
		if e = rows.Scan(&a, &b); e != nil {
			fail(c, e)
			return
		}
		first, x := netip.ParseAddr(a)
		last, y := netip.ParseAddr(b)
		if x == nil && y == nil {
			used = append(used, netcalc.Range{First: first, Last: last})
		}
	}
	for _, gap := range netcalc.Gaps(usable, used) {
		if gap.First.IsValid() {
			httpx.OK(c, 200, gin.H{"ip": gap.First.String(), "label": "Available candidate"})
			return
		}
	}
	fail(c, httpx.ErrNotFound)
}

func (h *Handler) exportDevices(c *gin.Context) {
	rows, e := h.conn.Query(c, "SELECT mac::text,name,owner,department,device_type,status,notes FROM devices ORDER BY mac")
	if e != nil {
		fail(c, e)
		return
	}
	defer rows.Close()
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", `attachment; filename="devices.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"mac", "name", "owner", "department", "deviceType", "status", "notes"})
	for rows.Next() {
		var r [7]string
		if e = rows.Scan(&r[0], &r[1], &r[2], &r[3], &r[4], &r[5], &r[6]); e != nil {
			fail(c, e)
			return
		}
		_ = w.Write(r[:])
	}
	w.Flush()
}
func (h *Handler) importDevices(c *gin.Context) {
	file, e := c.FormFile("file")
	if e != nil {
		fail(c, httpx.ErrBadRequest)
		return
	}
	if file.Size > 2<<20 {
		invalid(c, "file", "CSV must be smaller than 2 MB.")
		return
	}
	f, e := file.Open()
	if e != nil {
		fail(c, e)
		return
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, e := r.ReadAll()
	if e != nil || len(records) < 1 {
		invalid(c, "file", "Upload a valid CSV file.")
		return
	}
	if len(records) > 5001 {
		invalid(c, "file", "Import up to 5000 devices at a time.")
		return
	}
	if strings.ToLower(strings.TrimSpace(records[0][0])) != "mac" {
		invalid(c, "file", "The first column must be mac.")
		return
	}
	uid := actor(c)
	count := 0
	e = db.InTx(c, h.conn, func(tx pgx.Tx) error {
		entries := []audit.Entry{}
		for n, row := range records[1:] {
			if len(row) < 1 || strings.TrimSpace(row[0]) == "" {
				continue
			}
			name, owner, dept, typ := "", "", "", "other"
			if len(row) > 1 {
				name = row[1]
			}
			if len(row) > 2 {
				owner = row[2]
			}
			if len(row) > 3 {
				dept = row[3]
			}
			if len(row) > 4 && row[4] != "" {
				typ = row[4]
			}
			var key string
			if z := tx.QueryRow(c, `INSERT INTO devices(mac,name,owner,department,device_type,status,decided_by,decided_at) VALUES($1::macaddr,$2,$3,$4,$5,'approved',$6::uuid,now()) ON CONFLICT(mac) DO NOTHING RETURNING id`, strings.TrimSpace(row[0]), name, owner, dept, typ, uid).Scan(&key); z == nil {
				count++
				entries = append(entries, audit.Entry{ActorID: uid, Action: "import", ObjectType: "device", ObjectID: key, New: map[string]any{"row": n + 2, "mac": row[0]}})
			} else if z != pgx.ErrNoRows {
				return z
			}
		}
		return audit.WriteMany(c, tx, entries)
	})
	if e != nil {
		fail(c, conflict(e))
		return
	}
	httpx.OK(c, 200, gin.H{"imported": count})
}

func conflict(err error) error {
	switch db.ErrorCode(err) {
	case db.CodeUnique, db.CodeExclusion:
		return httpx.NewError(409, "CONFLICT", "That value is already in use or overlaps an existing segment.")
	case db.CodeCheck, db.CodeBadValue:
		return httpx.NewError(422, "VALIDATION_FAILED", "The values do not fit the selected segment or allowed format.")
	}
	return err
}
