package notifications

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Preference struct {
	EventType string `json:"eventType"`
	Label     string `json:"label,omitempty"` // filled in on reads
	Channel   string `json:"channel"`
	IsEnabled bool   `json:"isEnabled"`
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
			out = append(out, Preference{EventType: e.Type, Label: e.Label, Channel: ch, IsEnabled: !ok || enabled})
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
		if err := savePreferences(ctx, tx, me, req); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: me, Action: "notification_preferences.update", ObjectType: "user", ObjectID: me, New: req})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.preferences(c)
}

// savePreferences stores them in one statement. The last entry for a key wins, as
// one INSERT can't update a row twice.
func savePreferences(ctx context.Context, tx pgx.Tx, user string, list []Preference) error {
	index := map[[2]string]int{}
	var events, channels []string
	var enabled []bool
	for _, p := range list {
		key := [2]string{p.EventType, p.Channel}
		if i, ok := index[key]; ok {
			enabled[i] = p.IsEnabled
			continue
		}
		index[key] = len(events)
		events, channels, enabled = append(events, p.EventType), append(channels, p.Channel), append(enabled, p.IsEnabled)
	}
	if len(events) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO notification_preferences (user_id, event_type, channel, is_enabled)
		SELECT $1::uuid, e, c, v FROM unnest($2::text[], $3::text[], $4::boolean[]) AS t(e, c, v)
		ON CONFLICT (user_id, event_type, channel) DO UPDATE SET is_enabled = EXCLUDED.is_enabled`, user, events, channels, enabled)
	return err
}
