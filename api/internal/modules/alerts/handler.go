// Package alerts exposes the alert queue and acknowledgement. Alerts close by
// themselves when the cause is gone; people acknowledge them, nothing more.
package alerts

import (
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct{ conn db.Conn }

func New(conn db.Conn) *Handler { return &Handler{conn: conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/alerts", h.list, auth.InventoryView)
	g.Handle(http.MethodGet, "/alerts/summary", h.summary, auth.InventoryView)
	g.Handle(http.MethodPost, "/alerts/:id/acknowledge", h.acknowledge, auth.AlertsManage)
}

var (
	types      = []string{"unknown_device", "unregistered_static", "register_misuse", "unknown_server_ip", "ip_conflict", "server_down", "server_mac_changed", "rejected_device", "private_mac_device"}
	severities = []string{"critical", "high", "medium", "low"}
)

type alertDTO struct {
	ID             string     `db:"id" json:"id"`
	Type           string     `db:"alert_type" json:"type"`
	Severity       string     `db:"severity" json:"severity"`
	Status         string     `db:"status" json:"status"`
	Message        string     `db:"message" json:"message"`
	SegmentID      *string    `db:"segment_id" json:"segmentId"`
	SegmentName    *string    `db:"segment_name" json:"segmentName"`
	IPAddressID    *string    `db:"ip_address_id" json:"ipAddressId"`
	DeviceID       *string    `db:"device_id" json:"deviceId"`
	IP             *string    `db:"ip" json:"ip"`
	MAC            *string    `db:"mac" json:"mac"`
	OpenedAt       time.Time  `db:"opened_at" json:"openedAt"`
	LastSeenAt     time.Time  `db:"last_seen_at" json:"lastSeenAt"`
	AcknowledgedBy *string    `db:"acknowledged_by" json:"acknowledgedBy"`
	AcknowledgedAt *time.Time `db:"acknowledged_at" json:"acknowledgedAt"`
	ClosedAt       *time.Time `db:"closed_at" json:"closedAt"`
}

const alertFrom = ` FROM alerts a LEFT JOIN segments s ON s.id = a.segment_id LEFT JOIN users u ON u.id = a.acknowledged_by`

const alertColumns = `SELECT a.id::text AS id, a.alert_type, a.severity, a.status, a.message, a.segment_id::text AS segment_id,
	s.name AS segment_name, a.ip_address_id::text AS ip_address_id, a.device_id::text AS device_id, host(a.ip) AS ip,
	a.mac::text AS mac, a.opened_at, a.last_seen_at, u.email AS acknowledged_by, a.acknowledged_at, a.closed_at`

const severityOrder = "CASE a.severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END"

func (h *Handler) list(c *gin.Context) {
	var w db.Where
	var v validate.V
	switch status := c.DefaultQuery("status", "active"); status {
	case "active":
		w.Raw("a.status <> 'closed'")
	case "all":
	default:
		v.OneOf("status", status, []string{"open", "acknowledged", "closed"}, "status")
		w.Add("a.status = ?", status)
	}
	if s := c.Query("severity"); s != "" {
		v.OneOf("severity", s, severities, "severity")
		w.Add("a.severity = ?", s)
	}
	if t := c.Query("type"); t != "" {
		v.OneOf("type", t, types, "alert type")
		w.Add("a.alert_type = ?", t)
	}
	if id := c.Query("segmentId"); id != "" {
		v.UUID("segmentId", id, true, "a segment")
		w.Add("a.segment_id = ?::uuid", id)
	}
	if from := c.Query("from"); from != "" {
		v.Date("from", &from, false, "From")
		w.Add("a.opened_at >= ?::date", from)
	}
	if to := c.Query("to"); to != "" {
		v.Date("to", &to, false, "To")
		w.Add("a.opened_at < ?::date + 1", to)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(a.message ILIKE ? ESCAPE '\' OR host(a.ip) ILIKE ? ESCAPE '\' OR a.mac::text ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	page := httpx.ParsePage(c)
	order := "CASE a.status WHEN 'open' THEN 0 WHEN 'acknowledged' THEN 1 ELSE 2 END, " + severityOrder + ", a.opened_at DESC, a.id"
	items, total, err := db.ListPage[alertDTO](c, h.conn, alertColumns+alertFrom+w.SQL()+" ORDER BY "+order, "SELECT count(*)"+alertFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

// summary counts unresolved alerts by severity, for badges.
func (h *Handler) summary(c *gin.Context) {
	out := map[string]int64{"critical": 0, "high": 0, "medium": 0, "low": 0, "open": 0}
	rows, err := h.conn.Query(c, `SELECT severity, count(*), count(*) FILTER (WHERE status = 'open') FROM alerts WHERE status <> 'closed' GROUP BY severity`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var sev string
		var n, open int64
		if err := rows.Scan(&sev, &n, &open); err != nil {
			httpx.Fail(c, err)
			return
		}
		out[sev] = n
		out["open"] += open
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) acknowledge(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	uid := auth.Current(c).UserID
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(c, "SELECT status FROM alerts WHERE id = $1::uuid FOR UPDATE", id).Scan(&status); err != nil {
			return err
		}
		if status != "open" {
			return httpx.NewError(http.StatusConflict, "ALERT_NOT_OPEN", "This alert is already "+status+".")
		}
		if _, err := tx.Exec(c, `UPDATE alerts SET status = 'acknowledged', acknowledged_by = $2::uuid, acknowledged_at = now() WHERE id = $1::uuid`, id, uid); err != nil {
			return err
		}
		return audit.Write(c, tx, audit.Entry{ActorID: uid, Action: "acknowledge", ObjectType: "alert", ObjectID: id,
			Old: map[string]string{"status": status}, New: map[string]string{"status": "acknowledged"}})
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}
