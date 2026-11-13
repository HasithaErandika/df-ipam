package allocationlite

import (
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/ipam"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// maxRange keeps a reservation to "a small range" (docs/product/register.md).
const maxRange = 256

type reservationDTO struct {
	ID          string     `db:"id" json:"id"`
	SegmentID   string     `db:"segment_id" json:"segmentId"`
	SegmentName string     `db:"segment_name" json:"segmentName"`
	StartIP     string     `db:"start_ip" json:"startIp"`
	EndIP       string     `db:"end_ip" json:"endIp"`
	Size        int64      `db:"size" json:"size"`
	Reason      string     `db:"reason" json:"reason"`
	OwnerID     *string    `db:"owner_id" json:"ownerId"`
	OwnerName   string     `db:"owner_name" json:"ownerName"`
	ExpiresAt   *time.Time `db:"expires_at" json:"expiresAt"`
	Status      string     `db:"status" json:"status"`
	CreatedBy   *string    `db:"created_by_email" json:"createdByEmail"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
}

const reservationFrom = ` FROM reservations r JOIN segments s ON s.id = r.segment_id LEFT JOIN users cb ON cb.id = r.created_by`

const reservationColumns = `SELECT r.id::text AS id, r.segment_id::text AS segment_id, s.name AS segment_name, host(r.start_ip) AS start_ip,
	host(r.end_ip) AS end_ip, (r.end_ip - r.start_ip) + 1 AS size, r.reason, r.owner_id::text AS owner_id, r.owner_name,
	r.expires_at, r.status, cb.email AS created_by_email, r.created_at`

// Views of the list. Each is a constant condition.
var reservationViews = map[string]string{
	"active":   "r.status = 'active'",
	"expiring": "r.status = 'active' AND r.expires_at IS NOT NULL AND r.expires_at <= now() + interval '7 days' AND r.expires_at > now()",
	"expired":  "r.status = 'active' AND r.expires_at <= now()",
	"closed":   "r.status IN ('converted', 'released')",
}

func (h *Handler) listReservations(c *gin.Context) {
	var w db.Where
	view := c.DefaultQuery("view", "active")
	if cond, ok := reservationViews[view]; ok {
		w.Raw(cond)
	} else if view != "all" {
		httpx.Fail(c, httpx.Validation("view", "Choose a valid view."))
		return
	}
	if id := c.Query("segmentId"); httpx.IsUUID(id) {
		w.Add("r.segment_id = ?::uuid", id)
	}
	if c.Query("mine") == "true" {
		w.Add("r.owner_id = ?::uuid", actor(c))
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(r.reason ILIKE ? ESCAPE '\' OR r.owner_name ILIKE ? ESCAPE '\' OR host(r.start_ip) ILIKE ? ESCAPE '\' OR s.name ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	page := httpx.ParsePage(c)
	items, total, err := db.ListPage[reservationDTO](c, h.conn, reservationColumns+reservationFrom+w.SQL()+" ORDER BY r.expires_at NULLS LAST, r.created_at DESC, r.id",
		"SELECT count(*)"+reservationFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

type reserveInput struct {
	SegmentID string `json:"segmentId"`
	StartIP   string `json:"startIp"`
	EndIP     string `json:"endIp"`
	Reason    string `json:"reason"`
	OwnerID   string `json:"ownerId"`
	OwnerName string `json:"ownerName"`
	ExpiresOn string `json:"expiresOn"`
}

func (in *reserveInput) validate(admin bool) error {
	var v validate.V
	v.UUID("segmentId", in.SegmentID, true, "a segment")
	first := v.IPv4("startIp", &in.StartIP, true)
	if strings.TrimSpace(in.EndIP) == "" {
		in.EndIP = in.StartIP
	}
	last := v.IPv4("endIp", &in.EndIP, true)
	if first.IsValid() && last.IsValid() {
		if last.Less(first) {
			v.Add("endIp", "The last IP must not be before the first.")
		} else if ipam.RangeSize(first, last) > maxRange {
			v.Add("endIp", "Reserve at most 256 addresses at a time.")
		}
	}
	v.Text("reason", &in.Reason, 300, true, "Reason")
	v.Text("ownerName", &in.OwnerName, 120, false, "Owner")
	v.UUID("ownerId", in.OwnerID, false, "an owner")
	if in.OwnerID != "" && !admin {
		v.Add("ownerId", "Only an Admin can reserve on behalf of someone else.")
	}
	v.Date("expiresOn", &in.ExpiresOn, true, "Expiry")
	return v.Err()
}

// createReservation holds a free range. It refuses one that overlaps a register
// entry, another active hold or an address seen in use recently.
func (h *Handler) createReservation(c *gin.Context) {
	var in reserveInput
	if !httpx.Bind(c, &in) {
		return
	}
	p := auth.Current(c)
	if err := in.validate(p.Has(auth.InventoryEdit)); err != nil {
		httpx.Fail(c, err)
		return
	}
	if in.OwnerID == "" {
		in.OwnerID = p.UserID
	}
	if in.OwnerName == "" {
		in.OwnerName = p.Email
	}
	var id string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(c, "SELECT 1 FROM segments WHERE id = $1::uuid FOR UPDATE", in.SegmentID); err != nil {
			return err
		}
		var busy bool
		if err := tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM reservations WHERE status = 'active' AND start_ip <= $2::inet AND end_ip >= $1::inet)
			OR EXISTS (SELECT 1 FROM ip_addresses WHERE ip BETWEEN $1::inet AND $2::inet)
			OR EXISTS (SELECT 1 FROM sightings WHERE ip BETWEEN $1::inet AND $2::inet AND last_seen > now() - $3::interval)`,
			in.StartIP, in.EndIP, ipam.SeenWindow).Scan(&busy); err != nil {
			return err
		}
		if busy {
			return httpx.NewError(http.StatusConflict, "RANGE_IN_USE", "The range includes an address that is registered, reserved or was seen in use in the last 24 hours.")
		}
		if err := tx.QueryRow(c, `INSERT INTO reservations (segment_id, start_ip, end_ip, reason, owner_id, owner_name, expires_at, created_by, updated_by)
			VALUES ($1::uuid, $2::inet, $3::inet, $4, $5::uuid, $6, NULLIF($7, '')::date + interval '1 day', $8::uuid, $8::uuid) RETURNING id::text`,
			in.SegmentID, in.StartIP, in.EndIP, in.Reason, in.OwnerID, in.OwnerName, in.ExpiresOn, p.UserID).Scan(&id); err != nil {
			return err
		}
		return auditChange(c, tx, "reserve", "reservation", id, nil, in)
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": id, "status": "active"})
}
