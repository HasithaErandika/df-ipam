package inventory

import (
	"net/http"
	"net/netip"
	"time"

	"dfipam/api/internal/ipam"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type segmentDTO struct {
	ID              string    `db:"id" json:"id"`
	Name            string    `db:"name" json:"name"`
	Kind            string    `db:"kind" json:"kind"`
	Floor           *int      `db:"floor" json:"floor"`
	VLAN            *int      `db:"vlan" json:"vlan"`
	Network         string    `db:"network" json:"network"`
	Gateway         *string   `db:"gateway" json:"gateway"`
	DHCPStart       *string   `db:"dhcp_start" json:"dhcpStart"`
	DHCPEnd         *string   `db:"dhcp_end" json:"dhcpEnd"`
	EnvironmentID   *string   `db:"default_environment_id" json:"defaultEnvironmentId"`
	EnvironmentName *string   `db:"environment_name" json:"environmentName"`
	Description     string    `db:"description" json:"description"`
	Registered      int64     `db:"registered" json:"registered"`
	Seen            int64     `db:"seen" json:"seen"`
	UsableSize      int64     `db:"-" json:"usableSize"`
	UpdatedAt       time.Time `db:"updated_at" json:"updatedAt"`
}

const segmentSelect = `SELECT s.id::text AS id, s.name, s.kind, s.floor, s.vlan, s.network::text AS network,
	host(s.gateway) AS gateway, host(s.dhcp_start) AS dhcp_start, host(s.dhcp_end) AS dhcp_end,
	s.default_environment_id::text AS default_environment_id, e.name AS environment_name, s.description,
	(SELECT count(*) FROM ip_addresses i WHERE i.segment_id = s.id) AS registered,
	(SELECT count(DISTINCT v.ip) FROM sightings v WHERE v.segment_id = s.id AND v.last_seen > now() - interval '24 hours') AS seen,
	s.updated_at
	FROM segments s LEFT JOIN environments e ON e.id = s.default_environment_id`

// segments lists every segment. There are few (one per floor and server network),
// and forms need all of them, so this list is not paged.
func (h *Handler) segments(c *gin.Context) {
	var w db.Where
	if kind := c.Query("kind"); kind == "dhcp" || kind == "server" || kind == "cloud" {
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

type segmentInput struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Floor         *int   `json:"floor"`
	VLAN          *int   `json:"vlan"`
	Network       string `json:"network"`
	Gateway       string `json:"gateway"`
	DHCPStart     string `json:"dhcpStart"`
	DHCPEnd       string `json:"dhcpEnd"`
	EnvironmentID string `json:"defaultEnvironmentId"`
	Description   string `json:"description"`
}

func (in *segmentInput) validate() error {
	var v validate.V
	v.Text("name", &in.Name, 80, true, "Name")
	v.OneOf("kind", in.Kind, []string{"dhcp", "server", "cloud"}, "segment type")
	v.IntRange("floor", in.Floor, -5, 200, "Floor")
	v.IntRange("vlan", in.VLAN, 1, 4094, "VLAN")
	v.Text("description", &in.Description, 500, false, "Description")
	p := v.CIDR("network", &in.Network)
	gw := v.IPv4("gateway", &in.Gateway, false)
	start := v.IPv4("dhcpStart", &in.DHCPStart, false)
	end := v.IPv4("dhcpEnd", &in.DHCPEnd, false)
	inside := func(field string, a netip.Addr) {
		if p.IsValid() && a.IsValid() && !p.Contains(a) {
			v.Add(field, "Must be inside "+p.String()+".")
		}
	}
	inside("gateway", gw)
	inside("dhcpStart", start)
	inside("dhcpEnd", end)
	switch {
	case in.Kind != "dhcp" && (in.DHCPStart != "" || in.DHCPEnd != ""):
		v.Add("dhcpStart", "Only floor segments have a DHCP pool.")
	case (in.DHCPStart == "") != (in.DHCPEnd == ""):
		v.Add("dhcpEnd", "Enter both the first and last pool address.")
	case start.IsValid() && end.IsValid() && end.Less(start):
		v.Add("dhcpEnd", "The last pool address must not be before the first.")
	case gw.IsValid() && start.IsValid() && end.IsValid() && !gw.Less(start) && !end.Less(gw):
		v.Add("gateway", "The gateway can't be inside the DHCP pool.")
	}
	v.UUID("defaultEnvironmentId", in.EnvironmentID, false, "an environment")
	if in.Kind == "dhcp" {
		in.EnvironmentID = ""
	}
	return v.Err()
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
	args := []any{in.Name, in.Kind, in.Floor, in.VLAN, in.Network, in.Gateway, in.DHCPStart, in.DHCPEnd, in.EnvironmentID, in.Description, actor(c)}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if id == "" {
			if err := tx.QueryRow(c, `INSERT INTO segments (name, kind, floor, vlan, network, gateway, dhcp_start, dhcp_end, default_environment_id, description, created_by, updated_by)
				VALUES ($1, $2, $3, $4, $5::cidr, NULLIF($6, '')::inet, NULLIF($7, '')::inet, NULLIF($8, '')::inet, NULLIF($9, '')::uuid, $10, $11::uuid, $11::uuid)
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
			description = $10, updated_by = $11::uuid WHERE id = $12::uuid`, append(args, id)...); err != nil {
			return err
		}
		return record(c, tx, "update", "segment", id, old, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "A segment with this name exists, or the network overlaps another segment."))
		return
	}
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
	c.Status(http.StatusNoContent)
}
