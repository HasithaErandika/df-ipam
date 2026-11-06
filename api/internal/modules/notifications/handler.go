package notifications

import (
	"encoding/json"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Notification is one in-app alert for the signed-in user.
type Notification struct {
	ID         string     `json:"id"`
	EventType  string     `json:"eventType"`
	Severity   string     `json:"severity"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Link       string     `json:"link"`
	ObjectType *string    `json:"objectType"`
	ObjectID   *string    `json:"objectId"`
	CreatedAt  time.Time  `json:"createdAt"`
	ReadAt     *time.Time `json:"readAt"`
}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

// Register adds the routes. Everyone may read their own notifications and set
// their own preferences; rules are administration.
func (h *Handler) Register(g *auth.Guard) {
	g.Authenticated(http.MethodGet, "/notifications", h.list)
	g.Authenticated(http.MethodGet, "/notifications/unread-count", h.unreadCount)
	g.Authenticated(http.MethodPost, "/notifications/read-all", h.readAll)
	g.Authenticated(http.MethodPost, "/notifications/:id/read", h.read)
	g.Authenticated(http.MethodGet, "/notifications/preferences", h.preferences)
	g.Authenticated(http.MethodPut, "/notifications/preferences", h.setPreferences)
	g.Handle(http.MethodGet, "/notifications/rules", h.rules, auth.UsersManage)
	g.Handle(http.MethodPut, "/notifications/rules/:eventType", h.setRule, auth.UsersManage)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	me, ctx := auth.Current(c).UserID, c.Request.Context()
	unread := c.Query("unread") == "true"
	const where = ` FROM notifications WHERE recipient_id = $1::uuid AND channel = 'in_app' AND ($2 = false OR read_at IS NULL)`
	var total int64
	if err := h.conn.QueryRow(ctx, "SELECT count(*)"+where, me, unread).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(ctx, `SELECT id::text, event_type, severity, payload, object_type, object_id, created_at, read_at`+where+
		` ORDER BY created_at DESC, id LIMIT $3 OFFSET $4`, me, unread, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Notification, error) {
		var n Notification
		var raw []byte
		if err := r.Scan(&n.ID, &n.EventType, &n.Severity, &raw, &n.ObjectType, &n.ObjectID, &n.CreatedAt, &n.ReadAt); err != nil {
			return n, err
		}
		var pl Payload
		_ = json.Unmarshal(raw, &pl)
		n.Title, n.Body, n.Link = pl.Title, pl.Body, pl.Link
		return n, nil
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) unreadCount(c *gin.Context) {
	var n int64
	err := h.conn.QueryRow(c.Request.Context(), `SELECT count(*) FROM notifications
		WHERE recipient_id = $1::uuid AND channel = 'in_app' AND read_at IS NULL`, auth.Current(c).UserID).Scan(&n)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"unread": n})
}

// read marks one of the caller's own notifications. Someone else's is not found.
func (h *Handler) read(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	tag, err := h.conn.Exec(c.Request.Context(), `UPDATE notifications SET read_at = COALESCE(read_at, now())
		WHERE id = $1::uuid AND recipient_id = $2::uuid AND channel = 'in_app'`, id, auth.Current(c).UserID)
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
	_, err := h.conn.Exec(c.Request.Context(), `UPDATE notifications SET read_at = now()
		WHERE recipient_id = $1::uuid AND channel = 'in_app' AND read_at IS NULL`, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
