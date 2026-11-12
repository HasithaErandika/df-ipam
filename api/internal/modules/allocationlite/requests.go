package allocationlite

import (
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// maxOpenRequests stops one person flooding the approval queue.
const maxOpenRequests = 20

var (
	ipTypes          = []string{"server", "printer", "network", "cctv", "access_point", "user_pc", "other"}
	requestStatuses  = []string{"pending", "approved", "rejected", "cancelled"}
	errTooManyOpened = httpx.NewError(http.StatusTooManyRequests, "TOO_MANY_OPEN_REQUESTS", "You have 20 pending requests. Wait for decisions, or cancel some, before raising more.")
)

type requestDTO struct {
	ID              string     `db:"id" json:"id"`
	RequesterID     string     `db:"requester_id" json:"requesterId"`
	RequesterEmail  string     `db:"requester_email" json:"requesterEmail"`
	SegmentID       string     `db:"segment_id" json:"segmentId"`
	SegmentName     string     `db:"segment_name" json:"segmentName"`
	EnvironmentID   *string    `db:"environment_id" json:"environmentId"`
	EnvironmentName *string    `db:"environment_name" json:"environmentName"`
	Type            string     `db:"ip_type" json:"ipType"`
	Name            string     `db:"name" json:"name"`
	Purpose         string     `db:"purpose" json:"purpose"`
	NeededBy        *string    `db:"needed_by" json:"neededBy"`
	RequestedIP     *string    `db:"requested_ip" json:"requestedIp"`
	Status          string     `db:"status" json:"status"`
	DecisionReason  string     `db:"decision_reason" json:"decisionReason"`
	DecidedByEmail  *string    `db:"decided_by_email" json:"decidedByEmail"`
	DecidedAt       *time.Time `db:"decided_at" json:"decidedAt"`
	AssignedIP      *string    `db:"assigned_ip" json:"assignedIp"`
	CreatedAt       time.Time  `db:"created_at" json:"createdAt"`
}

const requestFrom = ` FROM ip_requests r JOIN users u ON u.id = r.requester_id JOIN segments s ON s.id = r.segment_id
	LEFT JOIN users dec ON dec.id = r.decided_by LEFT JOIN environments e ON e.id = r.environment_id
	LEFT JOIN ip_addresses a ON a.id = r.assigned_ip_id`

const requestColumns = `SELECT r.id::text AS id, r.requester_id::text AS requester_id, u.email AS requester_email, r.segment_id::text AS segment_id,
	s.name AS segment_name, r.environment_id::text AS environment_id, e.name AS environment_name, r.ip_type, r.name, r.purpose,
	r.needed_by::text AS needed_by, host(r.requested_ip) AS requested_ip, r.status, r.decision_reason, dec.email AS decided_by_email,
	r.decided_at, host(a.ip) AS assigned_ip, r.created_at`

// listRequests shows Admins every request and others only their own.
func (h *Handler) listRequests(c *gin.Context) {
	p := auth.Current(c)
	var w db.Where
	var v validate.V
	if !p.Has(auth.RequestsApprove) || c.Query("mine") == "true" {
		w.Add("r.requester_id = ?::uuid", p.UserID)
	}
	if st := c.Query("status"); st != "" && st != "all" {
		v.OneOf("status", st, requestStatuses, "status")
		w.Add("r.status = ?", st)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(r.name ILIKE ? ESCAPE '\' OR r.purpose ILIKE ? ESCAPE '\' OR u.email ILIKE ? ESCAPE '\' OR s.name ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	page := httpx.ParsePage(c)
	order := "CASE r.status WHEN 'pending' THEN 0 ELSE 1 END, r.created_at DESC"
	items, total, err := db.ListPage[requestDTO](c, h.conn, requestColumns+requestFrom+w.SQL()+" ORDER BY "+order+", r.id",
		"SELECT count(*)"+requestFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

type requestInput struct {
	SegmentID     string `json:"segmentId"`
	Type          string `json:"ipType"`
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Purpose       string `json:"purpose"`
	NeededBy      string `json:"neededBy"`
	RequestedIP   string `json:"requestedIp"`
}

func (in *requestInput) validate() error {
	var v validate.V
	v.UUID("segmentId", in.SegmentID, true, "a segment")
	if in.Type == "" {
		in.Type = "other"
	}
	v.OneOf("ipType", in.Type, ipTypes, "device type")
	v.UUID("environmentId", in.EnvironmentID, false, "an environment")
	v.Text("name", &in.Name, 120, true, "Device name")
	v.Text("purpose", &in.Purpose, 1000, true, "Purpose")
	v.Date("neededBy", &in.NeededBy, true, "Needed by")
	v.IPv4("requestedIp", &in.RequestedIP, false)
	return v.Err()
}

func (h *Handler) createRequest(c *gin.Context) {
	var in requestInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	uid := actor(c)
	var id string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var open int
		if err := tx.QueryRow(c, "SELECT count(*) FROM ip_requests WHERE requester_id = $1::uuid AND status = 'pending'", uid).Scan(&open); err != nil {
			return err
		}
		if open >= maxOpenRequests {
			return errTooManyOpened
		}
		if err := tx.QueryRow(c, `INSERT INTO ip_requests (requester_id, segment_id, ip_type, environment_id, name, purpose, needed_by, requested_ip)
			VALUES ($1::uuid, $2::uuid, $3, NULLIF($4, '')::uuid, $5, $6, NULLIF($7, '')::date, NULLIF($8, '')::inet) RETURNING id::text`,
			uid, in.SegmentID, in.Type, in.EnvironmentID, in.Name, in.Purpose, in.NeededBy, in.RequestedIP).Scan(&id); err != nil {
			return err
		}
		if err := auditChange(c, tx, "create", "ip_request", id, nil, in); err != nil {
			return err
		}
		return notify.Holders(c, tx, auth.RequestsApprove, "request", "New IP request from "+auth.Current(c).Email+": "+in.Name, "/requests", true)
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": id, "status": "pending"})
}

// cancelRequest lets the requester withdraw a pending request.
func (h *Handler) cancelRequest(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	uid := actor(c)
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		var owner, status string
		if err := tx.QueryRow(c, "SELECT to_jsonb(r), requester_id::text, status FROM ip_requests r WHERE id = $1::uuid FOR UPDATE", id).Scan(&old, &owner, &status); err != nil {
			return err
		}
		if owner != uid {
			return httpx.ErrNotFound
		}
		if status != "pending" {
			return errClosed
		}
		if _, err := tx.Exec(c, "UPDATE ip_requests SET status = 'cancelled' WHERE id = $1::uuid", id); err != nil {
			return err
		}
		return auditChange(c, tx, "cancel", "ip_request", id, rawJSON(old), map[string]string{"status": "cancelled"})
	})
	if err != nil {
		httpx.Fail(c, friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}
