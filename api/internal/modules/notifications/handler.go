// Package notifications serves per-user inbox and email preferences.
package notifications

import (
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

type Handler struct{ conn db.Conn }

func New(conn db.Conn) *Handler { return &Handler{conn: conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/notifications", h.list, auth.InventoryView)
	g.Handle(http.MethodPost, "/notifications/:id/read", h.read, auth.InventoryView)
	g.Handle(http.MethodPost, "/notifications/read-all", h.readAll, auth.InventoryView)
	g.Handle(http.MethodGet, "/notification-preferences", h.preferences, auth.InventoryView)
	g.Handle(http.MethodPut, "/notification-preferences", h.updatePreference, auth.InventoryView)
}

type notification struct {
	ID, Kind, Message, Link, EmailStatus string
	IsRead                               bool
	CreatedAt                            time.Time
}

func (h *Handler) list(c *gin.Context) {
	uid := auth.Current(c).UserID
	rows, err := h.conn.Query(c, `SELECT id::text,kind,message,link,is_read,email_status,created_at FROM notifications WHERE user_id=$1::uuid ORDER BY created_at DESC LIMIT 100`, uid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	out := []notification{}
	for rows.Next() {
		var n notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Message, &n.Link, &n.IsRead, &n.EmailStatus, &n.CreatedAt); err != nil {
			httpx.Fail(c, err)
			return
		}
		out = append(out, n)
	}
	if rows.Err() != nil {
		httpx.Fail(c, rows.Err())
		return
	}
	var unread int
	if err := h.conn.QueryRow(c, "SELECT count(*) FROM notifications WHERE user_id=$1::uuid AND NOT is_read", uid).Scan(&unread); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, 200, gin.H{"items": out, "unreadCount": unread})
}

func (h *Handler) read(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	tag, err := h.conn.Exec(c, `UPDATE notifications SET is_read=true,read_at=COALESCE(read_at,now()) WHERE id=$1::uuid AND user_id=$2::uuid`, id, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	c.Status(204)
}
func (h *Handler) readAll(c *gin.Context) {
	_, err := h.conn.Exec(c, `UPDATE notifications SET is_read=true,read_at=now() WHERE user_id=$1::uuid AND NOT is_read`, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(204)
}

var preferenceKinds = []string{"alert", "server_up", "request", "request_decision", "reservation_expiry"}

func (h *Handler) preferences(c *gin.Context) {
	uid := auth.Current(c).UserID
	rows, err := h.conn.Query(c, `SELECT kind,email FROM notification_preferences WHERE user_id=$1::uuid ORDER BY kind`, uid)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	values := map[string]bool{}
	for rows.Next() {
		var k string
		var v bool
		if err := rows.Scan(&k, &v); err != nil {
			httpx.Fail(c, err)
			return
		}
		values[k] = v
	}
	out := []gin.H{}
	for _, k := range preferenceKinds {
		enabled, ok := values[k]
		if !ok {
			enabled = true
		}
		out = append(out, gin.H{"kind": k, "email": enabled})
	}
	httpx.OK(c, 200, out)
}

func (h *Handler) updatePreference(c *gin.Context) {
	var in struct {
		Kind  string `json:"kind"`
		Email bool   `json:"email"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	valid := false
	for _, k := range preferenceKinds {
		if in.Kind == k {
			valid = true
		}
	}
	if !valid {
		httpx.Fail(c, httpx.Validation("kind", "Choose a supported notification type."))
		return
	}
	_, err := h.conn.Exec(c, `INSERT INTO notification_preferences(user_id,kind,email) VALUES($1::uuid,$2,$3) ON CONFLICT(user_id,kind) DO UPDATE SET email=EXCLUDED.email`, auth.Current(c).UserID, in.Kind, in.Email)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(204)
}
