package search

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type SavedFilter struct {
	ID        string            `json:"id"`
	Page      string            `json:"page"`
	Name      string            `json:"name"`
	Filters   map[string]string `json:"filters"`
	CreatedAt *time.Time        `json:"createdAt,omitempty"`
}

func (h *Handler) listFilters(c *gin.Context) {
	page := c.Query("page")
	if !pages[page] {
		httpx.Fail(c, httpx.Validation("page", "Unknown page."))
		return
	}
	rows, err := h.conn.Query(c.Request.Context(), `SELECT id::text, page, name, filters FROM saved_filters
		WHERE user_id = $1::uuid AND page = $2 ORDER BY name`, auth.Current(c).UserID, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SavedFilter, error) {
		var f SavedFilter
		var raw []byte
		if err := r.Scan(&f.ID, &f.Page, &f.Name, &raw); err != nil {
			return f, err
		}
		return f, json.Unmarshal(raw, &f.Filters)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, httpx.Page{Number: 1, Size: len(out)}, int64(len(out)))
}

// ValidateFilter checks a saved filter's shape. Values are plain strings, so
// they can only ever become the page's own query parameters.
func ValidateFilter(f *SavedFilter) error {
	f.Name = strings.TrimSpace(f.Name)
	if !pages[f.Page] {
		return httpx.Validation("page", "Unknown page.")
	}
	if f.Name == "" || len(f.Name) > 80 {
		return httpx.Validation("name", "Name the filter, in at most 80 characters.")
	}
	if len(f.Filters) == 0 || len(f.Filters) > 20 {
		return httpx.Validation("filters", "Save between 1 and 20 filter values.")
	}
	for k, v := range f.Filters {
		if len(k) > 40 || len(v) > 200 {
			return httpx.Validation("filters", "A filter value is too long.")
		}
	}
	return nil
}

// saveFilter stores a filter, replacing one of the same name on the same page.
func (h *Handler) saveFilter(c *gin.Context) {
	var f SavedFilter
	if err := c.ShouldBindJSON(&f); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if err := ValidateFilter(&f); err != nil {
		httpx.Fail(c, err)
		return
	}
	raw, _ := json.Marshal(f.Filters)
	me, ctx := auth.Current(c).UserID, c.Request.Context()
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO saved_filters (user_id, page, name, filters) VALUES ($1::uuid, $2, $3, $4::jsonb)
			ON CONFLICT (user_id, page, name) DO UPDATE SET filters = EXCLUDED.filters RETURNING id::text`,
			me, f.Page, f.Name, raw).Scan(&f.ID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: me, Action: "saved_filter.save", ObjectType: "saved_filter", ObjectID: f.ID, New: f})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, f)
}

func (h *Handler) deleteFilter(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	me, ctx := auth.Current(c).UserID, c.Request.Context()
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM saved_filters WHERE id = $1::uuid AND user_id = $2::uuid`, id, me)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.ErrNotFound
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: me, Action: "saved_filter.delete", ObjectType: "saved_filter", ObjectID: id})
	})
	if err != nil && !errors.Is(err, httpx.ErrNotFound) {
		httpx.Fail(c, err)
		return
	}
	if err != nil {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	c.Status(http.StatusNoContent)
}
