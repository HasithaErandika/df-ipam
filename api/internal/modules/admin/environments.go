package admin

import (
	"encoding/json"
	"net/http"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type environmentDTO struct {
	ID              string `db:"id" json:"id"`
	Name            string `db:"name" json:"name"`
	Colour          string `db:"colour" json:"colour"`
	SortOrder       int    `db:"sort_order" json:"sortOrder"`
	DefaultSeverity string `db:"default_severity" json:"defaultSeverity"`
	DefaultMonitor  bool   `db:"default_monitor" json:"defaultMonitor"`
	DefaultCritical bool   `db:"default_critical" json:"defaultCritical"`
	InUse           int64  `db:"in_use" json:"inUse"`
}

func (h *Handler) environments(c *gin.Context) {
	out, err := db.All[environmentDTO](c, h.conn, `SELECT e.id::text AS id, e.name, e.colour, e.sort_order, e.default_severity,
		e.default_monitor, e.default_critical, (SELECT count(*) FROM ip_addresses i WHERE i.environment_id = e.id) AS in_use
		FROM environments e ORDER BY e.sort_order, lower(e.name)`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

type environmentInput struct {
	Name            string `json:"name"`
	Colour          string `json:"colour"`
	SortOrder       *int   `json:"sortOrder"`
	DefaultSeverity string `json:"defaultSeverity"`
	DefaultMonitor  bool   `json:"defaultMonitor"`
	DefaultCritical bool   `json:"defaultCritical"`
}

func (in *environmentInput) validate() error {
	var v validate.V
	v.Text("name", &in.Name, 40, true, "Name")
	v.Colour("colour", in.Colour)
	v.IntRange("sortOrder", in.SortOrder, 0, 10000, "Order")
	v.OneOf("defaultSeverity", in.DefaultSeverity, []string{"critical", "high", "medium", "low"}, "severity")
	if in.DefaultCritical && !in.DefaultMonitor {
		v.Add("defaultCritical", "Critical servers must be monitored. Turn on Monitor too.")
	}
	if in.SortOrder == nil {
		zero := 0
		in.SortOrder = &zero
	}
	return v.Err()
}

func (h *Handler) createEnvironment(c *gin.Context) { h.saveEnvironment(c, "") }

func (h *Handler) updateEnvironment(c *gin.Context) {
	if id, ok := httpx.PathID(c); ok {
		h.saveEnvironment(c, id)
	}
}

func (h *Handler) saveEnvironment(c *gin.Context, id string) {
	var in environmentInput
	if !httpx.Bind(c, &in) {
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	args := []any{in.Name, in.Colour, *in.SortOrder, in.DefaultSeverity, in.DefaultMonitor, in.DefaultCritical}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		if id == "" {
			if err := tx.QueryRow(c, `INSERT INTO environments (name, colour, sort_order, default_severity, default_monitor, default_critical)
				VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text`, args...).Scan(&id); err != nil {
				return err
			}
			return record(c, tx, "create", "environment", id, nil, in)
		}
		var old []byte
		if err := tx.QueryRow(c, "SELECT to_jsonb(e) FROM environments e WHERE id = $1::uuid FOR UPDATE", id).Scan(&old); err != nil {
			return err
		}
		if _, err := tx.Exec(c, `UPDATE environments SET name = $1, colour = $2, sort_order = $3, default_severity = $4, default_monitor = $5,
			default_critical = $6 WHERE id = $7::uuid`, append(args, id)...); err != nil {
			return err
		}
		return record(c, tx, "update", "environment", id, json.RawMessage(old), in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err, "An environment with this name already exists."))
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"id": id})
}

// deleteEnvironment removes one nobody uses. Segments that default to it lose
// the default; register entries and requests keep it from being deleted.
func (h *Handler) deleteEnvironment(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var old []byte
		if err := tx.QueryRow(c, "SELECT to_jsonb(e) FROM environments e WHERE id = $1::uuid FOR UPDATE", id).Scan(&old); err != nil {
			return err
		}
		if _, err := tx.Exec(c, "DELETE FROM environments WHERE id = $1::uuid", id); err != nil {
			if db.ErrorCode(err) == db.CodeForeignKey {
				return httpx.NewError(http.StatusConflict, "ENVIRONMENT_IN_USE", "Register entries or requests still use this environment. Move them first.")
			}
			return err
		}
		return record(c, tx, "delete", "environment", id, json.RawMessage(old), nil)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}
