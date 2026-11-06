package notifications

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Rule struct {
	EventType  string `json:"eventType"`
	Label      string `json:"label"`
	Recipients string `json:"recipients"`
	Severity   string `json:"severity"`
	IsEnabled  bool   `json:"isEnabled"`
}

var severities = map[string]bool{"info": true, "warning": true, "critical": true}

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
