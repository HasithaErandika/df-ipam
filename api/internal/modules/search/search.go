// Package search finds inventory records of every kind from one query. An
// address or CIDR is matched by containment, so it finds the subnet holding it.
package search

import (
	"context"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Result is one match. Type says which screen opens it.
type Result struct {
	Type              string  `json:"type"`
	ID                string  `json:"id"`
	Title             string  `json:"title"`
	Subtitle          string  `json:"subtitle"`
	RoutingDomainName *string `json:"routingDomainName"`
}

const perType = 6

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/search", h.search, auth.InventoryView)
}

// Query is a parsed search term.
type Query struct {
	Text   string
	Addr   *netip.Addr
	Prefix *netip.Prefix
	Number *int
}

// Parse works out what kind of term was entered.
func Parse(q string) Query {
	out := Query{Text: strings.TrimSpace(q)}
	if a, err := netip.ParseAddr(out.Text); err == nil {
		a = a.Unmap()
		out.Addr = &a
	} else if p, err := netip.ParsePrefix(out.Text); err == nil {
		p = p.Masked()
		out.Prefix = &p
	}
	if n, err := strconv.Atoi(out.Text); err == nil && n >= 1 && n <= 4094 {
		out.Number = &n
	}
	return out
}

type finder struct {
	kind string
	sql  string
	args []any
}

// finders builds one bounded query per record type. Every value is a parameter.
func finders(q Query) []finder {
	text := q.Text
	var out []finder
	add := func(kind, sql string, args ...any) { out = append(out, finder{kind, sql, args}) }

	switch {
	case q.Addr != nil:
		add("subnet", `SELECT s.id::text, s.name, s.cidr::text, rd.name FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id
			WHERE s.archived_at IS NULL AND s.cidr >>= $1::text::inet ORDER BY masklen(s.cidr) DESC, s.id`, q.Addr.String())
		add("ip_address", `SELECT i.id::text, host(i.address), COALESCE(NULLIF(i.hostname, ''), s.name), rd.name
			FROM ip_addresses i JOIN subnets s ON s.id = i.subnet_id JOIN routing_domains rd ON rd.id = i.routing_domain_id
			WHERE i.address = $1::text::inet ORDER BY rd.name, i.id`, q.Addr.String())
		add("network_block", `SELECT b.id::text, b.name, b.cidr::text, rd.name FROM network_blocks b JOIN routing_domains rd ON rd.id = b.routing_domain_id
			WHERE b.cidr >>= $1::text::inet ORDER BY masklen(b.cidr) DESC, b.id`, q.Addr.String())
		add("device", `SELECT d.id::text, d.name, host(d.management_ip), NULL::text FROM devices d
			WHERE d.management_ip = $1::text::inet ORDER BY d.name, d.id`, q.Addr.String())
		add("dns_record", `SELECT r.id::text, r.name, r.record_type || ' ' || r.value, rd.name FROM dns_records r LEFT JOIN routing_domains rd ON rd.id = r.routing_domain_id
			WHERE r.value = $1 ORDER BY r.name, r.id`, q.Addr.String())
	case q.Prefix != nil:
		add("subnet", `SELECT s.id::text, s.name, s.cidr::text, rd.name FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id
			WHERE s.archived_at IS NULL AND s.cidr && $1::text::cidr ORDER BY s.cidr, s.id`, q.Prefix.String())
		add("network_block", `SELECT b.id::text, b.name, b.cidr::text, rd.name FROM network_blocks b JOIN routing_domains rd ON rd.id = b.routing_domain_id
			WHERE b.cidr && $1::text::cidr ORDER BY b.cidr, b.id`, q.Prefix.String())
	default:
		like := "%" + escapeLike(text) + "%"
		prefix := escapeLike(text) + "%"
		add("subnet", `SELECT s.id::text, s.name, s.cidr::text, rd.name FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id
			WHERE s.archived_at IS NULL AND (s.name ILIKE $1 OR s.cidr::text LIKE $2) ORDER BY s.name, s.id`, like, prefix)
		add("ip_address", `SELECT i.id::text, host(i.address), COALESCE(NULLIF(i.hostname, ''), s.name), rd.name
			FROM ip_addresses i JOIN subnets s ON s.id = i.subnet_id JOIN routing_domains rd ON rd.id = i.routing_domain_id
			WHERE host(i.address) LIKE $2 OR i.hostname ILIKE $1 OR i.fqdn ILIKE $1 OR i.mac::text ILIKE $1 ORDER BY i.address, i.id`, like, prefix)
		add("network_block", `SELECT b.id::text, b.name, b.cidr::text, rd.name FROM network_blocks b JOIN routing_domains rd ON rd.id = b.routing_domain_id
			WHERE b.name ILIKE $1 OR b.cidr::text LIKE $2 ORDER BY b.name, b.id`, like, prefix)
		add("site", `SELECT t.id::text, t.name, t.code, NULL::text FROM sites t WHERE t.name ILIKE $1 OR t.code ILIKE $1 ORDER BY t.name, t.id`, like)
		add("vdom", `SELECT v.id::text, v.name, COALESCE(d.name, ''), rd.name FROM vdoms v LEFT JOIN devices d ON d.id = v.device_id
			LEFT JOIN routing_domains rd ON rd.id = v.routing_domain_id WHERE v.name ILIKE $1 ORDER BY v.name, v.id`, like)
		add("device", `SELECT d.id::text, d.name, COALESCE(host(d.management_ip), d.hostname), NULL::text FROM devices d
			WHERE d.name ILIKE $1 OR d.hostname ILIKE $1 ORDER BY d.name, d.id`, like)
		add("dns_record", `SELECT r.id::text, r.name, r.record_type || ' ' || r.value, rd.name FROM dns_records r LEFT JOIN routing_domains rd ON rd.id = r.routing_domain_id
			WHERE r.name ILIKE $1 OR r.value ILIKE $1 ORDER BY r.name, r.id`, like)
	}
	if q.Number != nil {
		add("vlan", `SELECT l.id::text, l.vlan_id::text || CASE WHEN l.name = '' THEN '' ELSE ' ' || l.name END, COALESCE(st.name, ''), NULL::text
			FROM vlans l LEFT JOIN sites st ON st.id = l.site_id WHERE l.vlan_id = $1 ORDER BY st.name, l.id`, *q.Number)
	} else if q.Addr == nil && q.Prefix == nil {
		add("vlan", `SELECT l.id::text, l.vlan_id::text || ' ' || l.name, COALESCE(st.name, ''), NULL::text
			FROM vlans l LEFT JOIN sites st ON st.id = l.site_id WHERE l.name ILIKE $1 ORDER BY l.vlan_id, l.id`, "%"+escapeLike(text)+"%")
	}
	return out
}

// escapeLike stops % and _ in the term acting as wildcards.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

func (h *Handler) search(c *gin.Context) {
	q := Parse(c.Query("q"))
	if len(q.Text) < 2 || len(q.Text) > 100 {
		httpx.Fail(c, httpx.Validation("q", "Enter 2 to 100 characters."))
		return
	}
	results, err := h.run(c.Request.Context(), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, results)
}

func (h *Handler) run(ctx context.Context, q Query) ([]Result, error) {
	out := []Result{}
	for _, f := range finders(q) {
		rows, err := h.conn.Query(ctx, f.sql+" LIMIT "+strconv.Itoa(perType), f.args...)
		if err != nil {
			return nil, err
		}
		found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Result, error) {
			res := Result{Type: f.kind}
			return res, r.Scan(&res.ID, &res.Title, &res.Subtitle, &res.RoutingDomainName)
		})
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}
