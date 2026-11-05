package ipaddresses

import (
	"dfipam/api/internal/platform/httpx"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var sorts = map[string]string{"address": "i.address", "hostname": "i.hostname", "updatedAt": "i.updated_at"}

// listQuery is the WHERE clause of a list, built from the request.
type listQuery struct {
	where []string
	args  []any
}

func (q *listQuery) add(clause string, v any) {
	q.args = append(q.args, v)
	q.where = append(q.where, fmt.Sprintf(clause, len(q.args)))
}

func (q listQuery) clause() string {
	if len(q.where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(q.where, " AND ")
}

func parseListQuery(c *gin.Context) (listQuery, error) {
	var q listQuery
	if text := strings.TrimSpace(c.Query("q")); text != "" {
		q.add(`(host(i.address) ILIKE '%%' || $%[1]d || '%%' OR i.hostname ILIKE '%%' || $%[1]d || '%%'
			OR i.fqdn ILIKE '%%' || $%[1]d || '%%' OR i.mac::text ILIKE '%%' || $%[1]d || '%%')`, text)
	}
	for param, column := range map[string]string{"subnetId": "i.subnet_id", "routingDomainId": "i.routing_domain_id",
		"deviceId": "i.device_id", "ownerId": "i.owner_id", "siteId": "s.site_id", "businessServiceId": "i.business_service_id"} {
		v := c.Query(param)
		if v == "" {
			continue
		}
		if !httpx.IsUUID(v) {
			return q, httpx.Validation(param, "Must be a valid ID.")
		}
		q.add(column+" = $%d::uuid", v)
	}
	for param, column := range map[string]string{"allocationState": "i.allocation_state", "observationState": "i.observation_state", "environment": "s.environment"} {
		if v := c.Query(param); v != "" {
			q.add(column+" = $%d", v)
		}
	}
	return q, nil
}

func listOrder(c *gin.Context) string {
	sort := c.Query("sort")
	direction := ""
	if strings.HasPrefix(sort, "-") {
		sort, direction = sort[1:], " DESC"
	}
	if col, ok := sorts[sort]; ok {
		return col + direction
	}
	return "i.address"
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	q, err := parseListQuery(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var total int64
	if err := h.conn.QueryRow(ctx, "SELECT count(*)"+fromAddress+q.clause(), q.args...).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	args := append(q.args, p.Size, p.Offset())
	rows, err := h.conn.Query(ctx, fmt.Sprintf("%s%s%s ORDER BY %s, i.id LIMIT $%d OFFSET $%d", selectAddress, fromAddress, q.clause(), listOrder(c), len(args)-1, len(args)), args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, scan)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}
