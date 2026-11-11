package allocationlite

import (
	"math/big"
	"net/netip"

	"dfipam/api/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

func (h *Handler) dashboard(c *gin.Context) {
	rows, err := h.conn.Query(c, `SELECT s.id::text,s.name,s.floor,s.vlan,s.network::text,
		count(DISTINCT i.ip) FILTER(WHERE i.status='assigned'),
		count(DISTINCT o.ip) FILTER(WHERE o.last_seen>now()-interval '10 minutes'),
		count(DISTINCT o.ip) FILTER(WHERE o.last_seen>now()-interval '10 minutes' AND s.dhcp_start IS NOT NULL AND o.ip BETWEEN s.dhcp_start AND s.dhcp_end),
		s.dhcp_start::text,s.dhcp_end::text
		FROM segments s LEFT JOIN ip_addresses i ON i.segment_id=s.id LEFT JOIN sightings o ON o.segment_id=s.id
		WHERE s.kind='dhcp' GROUP BY s.id ORDER BY s.floor NULLS LAST,s.name`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	floors := []gin.H{}
	for rows.Next() {
		var id, name, network string
		var floor, vlan *int
		var assigned, seen, used int64
		var start, end *string
		if err := rows.Scan(&id, &name, &floor, &vlan, &network, &assigned, &seen, &used, &start, &end); err != nil {
			rows.Close()
			httpx.Fail(c, err)
			return
		}
		poolSize := int64(0)
		if start != nil && end != nil {
			first, e1 := netip.ParseAddr(*start)
			last, e2 := netip.ParseAddr(*end)
			if e1 == nil && e2 == nil {
				poolSize = addressNumber(last) - addressNumber(first) + 1
			}
		}
		floors = append(floors, gin.H{"id": id, "name": name, "floor": floor, "vlan": vlan, "network": network, "assigned": assigned, "seen": seen, "poolUsed": used, "poolSize": poolSize})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		httpx.Fail(c, err)
		return
	}
	rows.Close()
	alerts := map[string]int64{}
	alertRows, err := h.conn.Query(c, "SELECT severity,count(*) FROM alerts WHERE status<>'closed' GROUP BY severity")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	for alertRows.Next() {
		var key string
		var count int64
		if err := alertRows.Scan(&key, &count); err != nil {
			alertRows.Close()
			httpx.Fail(c, err)
			return
		}
		alerts[key] = count
	}
	if err := alertRows.Err(); err != nil {
		alertRows.Close()
		httpx.Fail(c, err)
		return
	}
	alertRows.Close()
	servers := []gin.H{}
	serverRows, err := h.conn.Query(c, `SELECT COALESCE(e.name,'Unassigned'),COALESCE(e.colour,'#64748b'),count(i.id) FILTER(WHERE i.monitor AND i.health='down') FROM ip_addresses i JOIN segments s ON s.id=i.segment_id LEFT JOIN environments e ON e.id=COALESCE(i.environment_id,s.default_environment_id) WHERE s.kind='server' GROUP BY e.name,e.colour ORDER BY e.name`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	for serverRows.Next() {
		var name, colour string
		var down int64
		if err := serverRows.Scan(&name, &colour, &down); err != nil {
			serverRows.Close()
			httpx.Fail(c, err)
			return
		}
		servers = append(servers, gin.H{"environment": name, "colour": colour, "down": down})
	}
	if err := serverRows.Err(); err != nil {
		serverRows.Close()
		httpx.Fail(c, err)
		return
	}
	serverRows.Close()
	httpx.OK(c, 200, gin.H{"floors": floors, "alerts": alerts, "serversDown": servers})
}

func addressNumber(addr netip.Addr) int64 {
	value := new(big.Int).SetBytes(addr.AsSlice())
	return value.Int64()
}
