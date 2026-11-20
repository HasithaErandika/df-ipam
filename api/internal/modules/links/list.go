package links

import (
	"net/netip"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

var linkSorts = map[string]string{
	"code": "l.code", "name": "lower(l.name)", "type": "l.link_type", "provider": "lower(p.name)", "site": "lower(l.source_site)",
	"destination": "lower(l.destination)", "bandwidth": "l.down_mbps", "role": "l.link_role", "rental": "l.monthly_rental",
	"status": "l.status", "criticality": "CASE l.criticality WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END",
	"contractEnd": "l.contract_end", "updated": "l.updated_at",
}

// commercialSorts can only be used by people who may see the values.
var commercialSorts = map[string]bool{"rental": true, "contractEnd": true}

// linkFilter reads the list filters. Every condition is a constant; values are bound.
func linkFilter(c *gin.Context, set Settings) (db.Where, error) {
	var w db.Where
	var v validate.V
	switch c.DefaultQuery("archived", "no") {
	case "only":
		w.Raw("l.archived_at IS NOT NULL")
	case "all":
	default:
		w.Raw("l.archived_at IS NULL")
	}
	for param, column := range map[string]string{"linkType": "l.link_type", "site": "l.source_site", "providerId": "l.provider_id::text"} {
		if s := strings.TrimSpace(c.Query(param)); s != "" {
			v.Text(param, &s, 120, false, "Filter")
			w.Add(column+" = ?", s)
		}
	}
	if s := c.Query("status"); s != "" {
		v.OneOf("status", s, statuses, "status")
		w.Add("l.status = ?", s)
	}
	if s := c.Query("criticality"); s != "" {
		v.OneOf("criticality", s, criticalities, "criticality")
		w.Add("l.criticality = ?", s)
	}
	if s := c.Query("vpn"); s == "true" {
		w.Add("l.link_type = ANY(?::text[])", set.vpnKeys())
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(l.name ILIKE ? ESCAPE '\' OR l.code ILIKE ? ESCAPE '\' OR l.circuit_id ILIKE ? ESCAPE '\' OR p.name ILIKE ? ESCAPE '\'
			OR l.destination ILIKE ? ESCAPE '\' OR l.vpn_tunnel ILIKE ? ESCAPE '\' OR host(l.wan_ip) ILIKE ? ESCAPE '\' OR array_to_string(l.extra_wan_ips, ' ') ILIKE ? ESCAPE '\'
			OR host(l.gateway_ip) ILIKE ? ESCAPE '\' OR l.public_ips::text ILIKE ? ESCAPE '\' OR l.remote_peer ILIKE ? ESCAPE '\'
			OR l.local_peer ILIKE ? ESCAPE '\' OR array_to_string(l.remote_subnets, ' ') ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	if s := c.Query("segmentId"); s != "" {
		v.UUID("segmentId", s, true, "a subnet")
		w.Add(`(l.wan_segment_id = ?::uuid OR EXISTS (SELECT 1 FROM network_link_segments x WHERE x.link_id = l.id AND x.segment_id = ?::uuid)
			OR EXISTS (SELECT 1 FROM segments sg WHERE sg.id = ?::uuid AND (sg.network >>= l.wan_ip OR sg.network >>= l.gateway_ip
				OR EXISTS (SELECT 1 FROM unnest(l.extra_wan_ips) e WHERE sg.network >>= set_masklen(e, 32)))))`, s)
	}
	if s := c.Query("ip"); s != "" {
		if a, err := netip.ParseAddr(s); err != nil || !a.Is4() {
			v.Add("ip", "Enter an IPv4 address.")
		}
		w.Add(`(l.wan_ip = ?::inet OR l.gateway_ip = ?::inet OR l.public_ips >>= ?::inet
			OR EXISTS (SELECT 1 FROM unnest(l.extra_wan_ips) e WHERE set_masklen(e, 32) = ?::inet))`, s)
	}
	return w, v.Err()
}

// vpnKeys lists the link types that are VPNs.
func (s Settings) vpnKeys() []string {
	keys := []string{}
	for _, t := range s.Types {
		if t.VPN {
			keys = append(keys, t.Key)
		}
	}
	return keys
}

func (h *Handler) list(c *gin.Context) {
	s, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	w, err := linkFilter(c, s)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	money := can(c, auth.LinksCommercialView)
	sorts := linkSorts
	if !money {
		sorts = map[string]string{}
		for k, v := range linkSorts {
			if !commercialSorts[k] {
				sorts[k] = v
			}
		}
	}
	page := httpx.ParsePage(c)
	order := httpx.Sort(c, sorts, "l.code")
	items, total, err := db.ListPage[linkDTO](c, h.conn, linkColumns+linkFrom+w.SQL()+" ORDER BY "+order+", l.id",
		"SELECT count(*)"+linkFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if !money {
		for i := range items {
			items[i].hideCommercial()
		}
	}
	httpx.List(c, items, page, total)
}
