// Package search finds records across the inventory from one box, and keeps
// each person's saved filters.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const perGroup = 8

// Hit is one search result. Link is a path in the web app.
type Hit struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Link   string `json:"link"`
}

type Results struct {
	Query       string `json:"query"`
	Subnets     []Hit  `json:"subnets"`
	IPAddresses []Hit  `json:"ipAddresses"`
	Devices     []Hit  `json:"devices"`
	VLANs       []Hit  `json:"vlans"`
}

// Kind of query, decided once so each group runs one indexed query.
type Query struct {
	Text string
	Addr *netip.Addr
	VLAN *int
}

// Parse reads the search text. An IP address searches by containment and
// equality; a number from 1 to 4094 also matches VLAN IDs.
func Parse(raw string) (Query, error) {
	q := Query{Text: strings.TrimSpace(raw)}
	if len([]rune(q.Text)) < 2 {
		return q, httpx.Validation("q", "Type at least two characters.")
	}
	if len(q.Text) > 200 {
		return q, httpx.Validation("q", "Keep the search under 200 characters.")
	}
	if a, err := netip.ParseAddr(q.Text); err == nil && a.Zone() == "" {
		a = a.Unmap()
		q.Addr = &a
	}
	if n, err := strconv.Atoi(q.Text); err == nil && n >= 1 && n <= 4094 {
		q.VLAN = &n
	}
	return q, nil
}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/search", h.search, auth.InventoryView)
	g.Authenticated(http.MethodGet, "/saved-filters", h.listFilters)
	g.Authenticated(http.MethodPost, "/saved-filters", h.saveFilter)
	g.Authenticated(http.MethodDelete, "/saved-filters/:id", h.deleteFilter)
}

func collect(ctx context.Context, conn db.Conn, sql string, args ...any) ([]Hit, error) {
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Hit, error) {
		var h Hit
		return h, r.Scan(&h.Kind, &h.ID, &h.Title, &h.Detail, &h.Link)
	})
	if out == nil {
		out = []Hit{}
	}
	return out, err
}

func (h *Handler) search(c *gin.Context) {
	q, err := Parse(c.Query("q"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var addr *string
	if q.Addr != nil {
		s := q.Addr.String()
		addr = &s
	}
	res := Results{Query: q.Text}
	// Subnets: by name or CIDR text, or every active subnet holding the address.
	if res.Subnets, err = collect(ctx, h.conn, `
		SELECT 'subnet', s.id::text, s.cidr::text, s.name || ' · ' || rd.name, '/subnets?q=' || s.cidr::text
		FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id
		WHERE s.archived_at IS NULL AND (
		  ($2::text IS NOT NULL AND s.cidr >>= $2::text::inet)
		  OR ($2::text IS NULL AND (s.name ILIKE '%' || $1 || '%' OR s.cidr::text LIKE $1 || '%')))
		ORDER BY masklen(s.cidr) DESC, s.name LIMIT $3`, q.Text, addr, perGroup); err != nil {
		httpx.Fail(c, err)
		return
	}
	if res.IPAddresses, err = collect(ctx, h.conn, `
		SELECT 'ip_address', i.id::text, host(i.address),
		       concat_ws(' · ', NULLIF(i.hostname, ''), rd.name, replace(i.allocation_state, '_', ' ')),
		       '/ip-addresses?id=' || i.id::text
		FROM ip_addresses i JOIN routing_domains rd ON rd.id = i.routing_domain_id
		WHERE ($2::text IS NOT NULL AND i.address = $2::text::inet)
		   OR ($2::text IS NULL AND (lower(i.hostname) LIKE lower($1) || '%' OR lower(i.fqdn) LIKE lower($1) || '%'
		       OR i.mac::text = lower($1) OR host(i.address) LIKE $1 || '%'))
		ORDER BY i.address LIMIT $3`, q.Text, addr, perGroup); err != nil {
		httpx.Fail(c, err)
		return
	}
	if res.Devices, err = collect(ctx, h.conn, `
		SELECT 'device', d.id::text, d.name, concat_ws(' · ', NULLIF(d.device_type, ''), NULLIF(d.vendor, ''), host(d.management_ip)),
		       '/devices?id=' || d.id::text
		FROM devices d
		WHERE d.name ILIKE '%' || $1 || '%' OR d.hostname ILIKE '%' || $1 || '%' OR ($2::text IS NOT NULL AND d.management_ip = $2::text::inet)
		ORDER BY d.name LIMIT $3`, q.Text, addr, perGroup); err != nil {
		httpx.Fail(c, err)
		return
	}
	if res.VLANs, err = collect(ctx, h.conn, `
		SELECT 'vlan', v.id::text, v.vlan_id::text || ' ' || v.name, coalesce((SELECT name FROM sites WHERE id = v.site_id), 'No site'),
		       '/network?tab=vlans&q=' || v.name
		FROM vlans v
		WHERE ($2::int IS NOT NULL AND v.vlan_id = $2::int) OR v.name ILIKE '%' || $1 || '%'
		ORDER BY v.vlan_id LIMIT $3`, q.Text, q.VLAN, perGroup); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, res)
}

// Pages that can save filters. Filters are opaque to the API: each page stores
// its own query parameters and applies them through its normal allowlists.
var pages = map[string]bool{"subnets": true, "ip-addresses": true, "requests": true, "reservations": true, "allocations": true, "audit": true}

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
