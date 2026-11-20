package links

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type change struct {
	Field string `json:"field"`
	Old   any    `json:"old"`
	New   any    `json:"new"`
}

type historyEntry struct {
	At      time.Time `json:"at"`
	Actor   string    `json:"actor"`
	Action  string    `json:"action"`
	Changes []change  `json:"changes"`
}

type auditRow struct {
	At     time.Time `db:"occurred_at"`
	Actor  *string   `db:"actor"`
	Action string    `db:"action"`
	Old    []byte    `db:"old_values"`
	New    []byte    `db:"new_values"`
}

// skipFields never count as a change people need to read.
var skipFields = []string{"id", "updated_at", "created_at", "created_by", "archived_by"}

// history lists one link's audit records, newest first, each as the fields it
// changed. Commercial fields are left out for people who may not see them.
func (h *Handler) history(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if !can(c, auth.LinksView) {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	rows, err := db.All[auditRow](c, h.conn, `SELECT a.occurred_at, COALESCE(u.email, a.actor_type) AS actor, a.action,
		convert_to(COALESCE(a.old_values::text, ''), 'UTF8') AS old_values, convert_to(COALESCE(a.new_values::text, ''), 'UTF8') AS new_values
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.object_type = 'network_link' AND a.object_id = $1 ORDER BY a.occurred_at DESC, a.id DESC LIMIT 200`, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	money := can(c, auth.LinksCommercialView)
	out := make([]historyEntry, 0, len(rows))
	for _, r := range rows {
		e := historyEntry{At: r.At, Action: r.Action, Changes: diff(r.Old, r.New, money)}
		if r.Actor != nil {
			e.Actor = *r.Actor
		}
		out = append(out, e)
	}
	httpx.OK(c, http.StatusOK, out)
}

// diff lists the fields whose value differs between two JSON objects.
func diff(oldRaw, newRaw []byte, money bool) []change {
	var old, now map[string]any
	_ = json.Unmarshal(oldRaw, &old)
	_ = json.Unmarshal(newRaw, &now)
	keys := map[string]bool{}
	for k := range old {
		keys[k] = true
	}
	for k := range now {
		keys[k] = true
	}
	out := []change{}
	for k := range keys {
		if slices.Contains(skipFields, k) || (!money && slices.Contains(commercialKeys, k)) {
			continue
		}
		o, n := old[k], now[k]
		if ob, _ := json.Marshal(o); string(ob) != mustJSON(n) {
			out = append(out, change{Field: k, Old: o, New: n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
