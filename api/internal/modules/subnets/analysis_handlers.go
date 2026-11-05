package subnets

import (
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/httpx"
	"math/big"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) utilisation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := h.svc.Analyse(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) freeRanges(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ranges, free, count, err := h.svc.FreeRanges(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"ranges": ranges, "totalFree": free, "rangeCount": count, "truncated": count > len(ranges)})
}

func (h *Handler) addresses(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	offset, valid := new(big.Int).SetString(c.DefaultQuery("offset", "0"), 10)
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "256"))
	if !valid || err != nil || limit < 1 || limit > maxGridPage {
		httpx.Fail(c, httpx.Validation("limit", "Use a whole-number offset and a limit from 1 to 1024."))
		return
	}
	cells, total, err := h.svc.Grid(c.Request.Context(), id, offset, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": cells, "meta": gin.H{"offset": offset.String(), "limit": limit, "total": total}})
}

func (h *Handler) blockFreeSpace(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := h.svc.BlockFreeSpace(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) split(c *gin.Context) {
	var req struct {
		CIDR      string `json:"cidr"`
		NewPrefix int    `json:"newPrefix"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	p, err := ParseCIDR(req.CIDR)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	parts, total, err := netcalc.Split(p, req.NewPrefix, maxListed)
	if err != nil {
		httpx.Fail(c, httpx.Validation("newPrefix", err.Error()))
		return
	}
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = part.String()
	}
	httpx.OK(c, http.StatusOK, gin.H{"subnets": out, "total": total, "truncated": total != strconv.Itoa(len(out))})
}

func (h *Handler) summarise(c *gin.Context) {
	var req struct {
		CIDRs []string `json:"cidrs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if len(req.CIDRs) == 0 || len(req.CIDRs) > 5000 {
		httpx.Fail(c, httpx.Validation("cidrs", "Enter between 1 and 5,000 CIDRs."))
		return
	}
	prefixes := make([]netip.Prefix, len(req.CIDRs))
	for i, value := range req.CIDRs {
		p, err := ParseCIDR(value)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		prefixes[i] = p
	}
	out := []string{}
	for _, p := range netcalc.Summarise(prefixes) {
		out = append(out, p.String())
	}
	httpx.OK(c, http.StatusOK, gin.H{"summary": out})
}

// locate finds the most specific subnet for an address in each routing domain,
// because the same private address can exist in several.
func (h *Handler) locate(c *gin.Context) {
	a, err := netip.ParseAddr(strings.TrimSpace(c.Query("address")))
	if err != nil {
		httpx.Fail(c, httpx.Validation("address", "Enter a valid IP address."))
		return
	}
	rows, err := h.svc.pool.Query(c.Request.Context(), `
		SELECT DISTINCT ON (s.routing_domain_id) s.id::text, s.name, s.cidr::text, s.routing_domain_id::text, rd.name
		FROM subnets s JOIN routing_domains rd ON rd.id = s.routing_domain_id
		WHERE s.archived_at IS NULL AND s.cidr >>= $1::text::inet
		ORDER BY s.routing_domain_id, masklen(s.cidr) DESC`, a.String())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type match struct {
		SubnetID          string `json:"subnetId"`
		SubnetName        string `json:"subnetName"`
		CIDR              string `json:"cidr"`
		RoutingDomainID   string `json:"routingDomainId"`
		RoutingDomainName string `json:"routingDomainName"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (match, error) {
		var m match
		return m, r.Scan(&m.SubnetID, &m.SubnetName, &m.CIDR, &m.RoutingDomainID, &m.RoutingDomainName)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out == nil {
		out = []match{}
	}
	httpx.OK(c, http.StatusOK, out)
}
