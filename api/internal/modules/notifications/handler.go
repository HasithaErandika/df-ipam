// Package notifications serves each person's inbox and email preferences.
package notifications

import (
	"net/http"
	"slices"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type Handler struct{ conn db.Conn }

func New(conn db.Conn) *Handler { return &Handler{conn: conn} }

// Every signed-in person with a role has an inbox, whatever the role.
var anyRole = []string{auth.InventoryView, auth.IPRequest, auth.SystemManage}

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/notifications", h.list, anyRole...)
	g.Handle(http.MethodGet, "/notifications/unread-count", h.unread, anyRole...)
	g.Handle(http.MethodPost, "/notifications/:id/read", h.read, anyRole...)
	g.Handle(http.MethodPost, "/notifications/read-all", h.readAll, anyRole...)
	g.Handle(http.MethodGet, "/notification-preferences", h.preferences, anyRole...)
	g.Handle(http.MethodPut, "/notification-preferences", h.updatePreference, anyRole...)
}

type notificationDTO struct {
	ID          string    `db:"id" json:"id"`
	Kind        string    `db:"kind" json:"kind"`
	Message     string    `db:"message" json:"message"`
	Link        string    `db:"link" json:"link"`
	IsRead      bool      `db:"is_read" json:"isRead"`
	EmailStatus string    `db:"email_status" json:"emailStatus"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
}

func (h *Handler) list(c *gin.Context) {
	w := db.Where{}
	w.Add("user_id = ?::uuid", auth.Current(c).UserID)
	if c.Query("unread") == "true" {
		w.Raw("NOT is_read")
	}
	page := httpx.ParsePage(c)
	items, total, err := db.ListPage[notificationDTO](c, h.conn, `SELECT id::text AS id, kind, message, link, is_read, email_status, created_at
		FROM notifications`+w.SQL()+" ORDER BY created_at DESC, id", "SELECT count(*) FROM notifications"+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

func (h *Handler) unread(c *gin.Context) {
	var n int64
	if err := h.conn.QueryRow(c, "SELECT count(*) FROM notifications WHERE user_id = $1::uuid AND NOT is_read", auth.Current(c).UserID).Scan(&n); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"count": n})
}

func (h *Handler) read(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	tag, err := h.conn.Exec(c, `UPDATE notifications SET is_read = true, read_at = COALESCE(read_at, now()) WHERE id = $1::uuid AND user_id = $2::uuid`,
		id, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) readAll(c *gin.Context) {
	if _, err := h.conn.Exec(c, `UPDATE notifications SET is_read = true, read_at = now() WHERE user_id = $1::uuid AND NOT is_read`, auth.Current(c).UserID); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type preference struct {
	Kind  string `json:"kind"`
	Email bool   `json:"email"`
}

func (h *Handler) preferences(c *gin.Context) {
	rows, err := h.conn.Query(c, `SELECT kind, email FROM notification_preferences WHERE user_id = $1::uuid`, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	values := map[string]bool{}
	for rows.Next() {
		var p preference
		if err := rows.Scan(&p.Kind, &p.Email); err != nil {
			httpx.Fail(c, err)
			return
		}
		values[p.Kind] = p.Email
	}
	out := make([]preference, 0, len(notify.Kinds))
	for _, k := range notify.Kinds {
		on, set := values[k]
		out = append(out, preference{Kind: k, Email: on || !set})
	}
	var enabled bool
	_ = h.conn.QueryRow(c, `SELECT COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'email.enabled'), false)`).Scan(&enabled)
	httpx.OK(c, http.StatusOK, gin.H{"items": out, "emailEnabled": enabled})
}

func (h *Handler) updatePreference(c *gin.Context) {
	var in preference
	if !httpx.Bind(c, &in) {
		return
	}
	if !slices.Contains(notify.Kinds, in.Kind) {
		httpx.Fail(c, httpx.Validation("kind", "Choose a supported notification type."))
		return
	}
	if _, err := h.conn.Exec(c, `INSERT INTO notification_preferences (user_id, kind, email) VALUES ($1::uuid, $2, $3)
		ON CONFLICT (user_id, kind) DO UPDATE SET email = EXCLUDED.email`, auth.Current(c).UserID, in.Kind, in.Email); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
