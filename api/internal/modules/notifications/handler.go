package notifications

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

type Preference struct {
	EventType string `json:"eventType"`
	Channel   string `json:"channel"`
	IsEnabled bool   `json:"isEnabled"`
}

type Rule struct {
	EventType  string `json:"eventType"`
	Label      string `json:"label"`
	Recipients string `json:"recipients"`
	Severity   string `json:"severity"`
	IsEnabled  bool   `json:"isEnabled"`
}

var severities = map[string]bool{"info": true, "warning": true, "critical": true}

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

// preferences returns every event and channel, defaulting to on.
func (h *Handler) preferences(c *gin.Context) {
	rows, err := h.conn.Query(c.Request.Context(), `SELECT event_type, channel, is_enabled FROM notification_preferences
		WHERE user_id = $1::uuid`, auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	stored, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Preference, error) {
		var p Preference
		return p, r.Scan(&p.EventType, &p.Channel, &p.IsEnabled)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, MergePreferences(stored))
}

// MergePreferences lists every known event and channel, with stored choices
// applied and everything else on.
func MergePreferences(stored []Preference) []Preference {
	set := map[[2]string]bool{}
	for _, p := range stored {
		set[[2]string{p.EventType, p.Channel}] = p.IsEnabled
	}
	out := []Preference{}
	for _, e := range EventTypes {
		for _, ch := range Channels {
			enabled, ok := set[[2]string{e.Type, ch}]
			out = append(out, Preference{e.Type, ch, !ok || enabled})
		}
	}
	return out
}

func validChannel(ch string) bool {
	for _, c := range Channels {
		if c == ch {
			return true
		}
	}
	return false
}

func (h *Handler) setPreferences(c *gin.Context) {
	var req []Preference
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	for _, p := range req {
		if _, ok := knownType(p.EventType); !ok {
			httpx.Fail(c, httpx.Validation("eventType", "Unknown event type."))
			return
		}
		if !validChannel(p.Channel) {
			httpx.Fail(c, httpx.Validation("channel", "Choose in_app or email."))
			return
		}
	}
	me, ctx := auth.Current(c).UserID, c.Request.Context()
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		for _, p := range req {
			if _, err := tx.Exec(ctx, `INSERT INTO notification_preferences (user_id, event_type, channel, is_enabled)
				VALUES ($1::uuid, $2, $3, $4) ON CONFLICT (user_id, event_type, channel) DO UPDATE SET is_enabled = EXCLUDED.is_enabled`,
				me, p.EventType, p.Channel, p.IsEnabled); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: me, Action: "notification_preferences.update", ObjectType: "user", ObjectID: me, New: req})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.preferences(c)
}

func (h *Handler) loadRules(c *gin.Context, q Querier) ([]Rule, error) {
	rows, err := q.Query(c.Request.Context(), `SELECT event_type, severity, is_enabled FROM notification_rules`)
	if err != nil {
		return nil, err
	}
	stored, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Rule, error) {
		var rule Rule
		return rule, r.Scan(&rule.EventType, &rule.Severity, &rule.IsEnabled)
	})
	if err != nil {
		return nil, err
	}
	byType := map[string]Rule{}
	for _, r := range stored {
		byType[r.EventType] = r
	}
	out := []Rule{}
	for _, e := range EventTypes {
		r, ok := byType[e.Type]
		if !ok {
			r = Rule{EventType: e.Type, Severity: e.Severity, IsEnabled: true}
		}
		r.Label, r.Recipients = e.Label, e.Recipients
		out = append(out, r)
	}
	return out, nil
}

func (h *Handler) rules(c *gin.Context) {
	out, err := h.loadRules(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) setRule(c *gin.Context) {
	eventType := c.Param("eventType")
	def, ok := knownType(eventType)
	if !ok {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	var req struct {
		Severity  *string `json:"severity"`
		IsEnabled *bool   `json:"isEnabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if req.Severity != nil && !severities[*req.Severity] {
		httpx.Fail(c, httpx.Validation("severity", "Choose info, warning or critical."))
		return
	}
	actor, ctx := auth.Current(c).UserID, c.Request.Context()
	var out []Rule
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		old, err := h.loadRules(c, tx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO notification_rules (event_type, severity, is_enabled)
			VALUES ($1, COALESCE($2, $4), COALESCE($3, true))
			ON CONFLICT (event_type) DO UPDATE SET severity = COALESCE($2, notification_rules.severity),
			  is_enabled = COALESCE($3, notification_rules.is_enabled)`, eventType, req.Severity, req.IsEnabled, def.Severity); err != nil {
			return err
		}
		if out, err = h.loadRules(c, tx); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "notification_rule.update", ObjectType: "notification_rule",
			ObjectID: eventType, Old: pick(old, eventType), New: pick(out, eventType)})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, pick(out, eventType))
}

func pick(rules []Rule, eventType string) *Rule {
	for i := range rules {
		if rules[i].EventType == eventType {
			return &rules[i]
		}
	}
	return nil
}
