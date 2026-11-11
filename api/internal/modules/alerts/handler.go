// Package alerts exposes the alert queue and acknowledgement action.
package alerts

import (
	"encoding/json"
	"net/http"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct{ conn db.Conn }

func New(conn db.Conn) *Handler { return &Handler{conn: conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/alerts", h.list, auth.InventoryView)
	g.Handle(http.MethodPost, "/alerts/:id/acknowledge", h.acknowledge, auth.AlertsManage)
}

type Alert struct {
	ID, Type, Severity, Status, Message, DedupeKey string
	SegmentID, IPAddressID, DeviceID, IP, MAC      *string
	OpenedAt, LastSeenAt                           time.Time
	AcknowledgedAt, ClosedAt                       *time.Time
}

func (a Alert) JSON() gin.H {
	return gin.H{"id": a.ID, "type": a.Type, "severity": a.Severity, "status": a.Status, "message": a.Message,
		"segmentId": a.SegmentID, "ipAddressId": a.IPAddressID, "deviceId": a.DeviceID, "ip": a.IP, "mac": a.MAC,
		"openedAt": a.OpenedAt, "lastSeenAt": a.LastSeenAt, "acknowledgedAt": a.AcknowledgedAt, "closedAt": a.ClosedAt}
}

func (h *Handler) list(c *gin.Context) {
	status := c.DefaultQuery("status", "open")
	if status != "open" && status != "acknowledged" && status != "closed" && status != "all" {
		httpx.Fail(c, httpx.Validation("status", "Choose open, acknowledged, closed or all."))
		return
	}
	rows, err := h.conn.Query(c, `SELECT id::text,alert_type,severity,status,message,segment_id::text,ip_address_id::text,device_id::text,ip::text,mac::text,opened_at,last_seen_at,acknowledged_at,closed_at,dedupe_key FROM alerts WHERE ($1='all' OR status=$1) ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 ELSE 4 END,opened_at DESC LIMIT 200`, status)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.Type, &a.Severity, &a.Status, &a.Message, &a.SegmentID, &a.IPAddressID, &a.DeviceID, &a.IP, &a.MAC, &a.OpenedAt, &a.LastSeenAt, &a.AcknowledgedAt, &a.ClosedAt, &a.DedupeKey); err != nil {
			httpx.Fail(c, err)
			return
		}
		out = append(out, a.JSON())
	}
	if rows.Err() != nil {
		httpx.Fail(c, rows.Err())
		return
	}
	httpx.OK(c, 200, out)
}

func (h *Handler) acknowledge(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	principal := auth.Current(c)
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		if err := tx.QueryRow(c, "SELECT to_jsonb(a) FROM alerts a WHERE id=$1::uuid FOR UPDATE", id).Scan(&old); err != nil {
			return err
		}
		if _, err := tx.Exec(c, `UPDATE alerts SET status='acknowledged',acknowledged_by=$2::uuid,acknowledged_at=now() WHERE id=$1::uuid AND status='open'`, id, principal.UserID); err != nil {
			return err
		}
		return audit.Write(c, tx, audit.Entry{ActorID: principal.UserID, Action: "acknowledge", ObjectType: "alert", ObjectID: id, Old: json.RawMessage(old)})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
