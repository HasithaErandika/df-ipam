package inventory

import (
	"net/http"
	"net/netip"

	"dfipam/api/internal/ipam"
	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

// maxMapped is the largest segment drawn address by address (a /20).
const maxMapped = 4096

// Address states, most specific first. The register always wins over evidence.
const (
	stAssigned  = "assigned"
	stReserved  = "reserved"
	stBlocked   = "blocked"
	stGateway   = "gateway"
	stHeld      = "held"      // inside an active reservation, not registered
	stSeen      = "seen"      // in use on the network, not registered, outside the pool
	stDHCPUsed  = "dhcp_used" // in the pool and answering
	stDHCPFree  = "dhcp_free" // in the pool, not seen
	stAvailable = "available"
)

type run struct {
	Start string `json:"start"`
	End   string `json:"end"`
	State string `json:"state"`
	Count int    `json:"count"`
}

type mapRow struct {
	A     string `db:"a"`
	B     string `db:"b"`
	State string `db:"state"`
}

// addressMap describes every usable address in a segment by state: counts for
// charts, and runs of equal state for drawing the map.
func (h *Handler) addressMap(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var network string
	if err := h.conn.QueryRow(c, "SELECT network::text FROM segments WHERE id = $1::uuid", id).Scan(&network); err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	prefix, _ := netip.ParsePrefix(network)
	usable := netcalc.UsableRange(prefix)
	size := ipam.RangeSize(usable.First, usable.Last)
	free, err := freeRanges(c, h, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	info := netcalc.Calculate(prefix)
	if size > maxMapped {
		httpx.OK(c, http.StatusOK, gin.H{"usable": size, "mapped": false, "counts": gin.H{}, "runs": []run{}, "network": info, "available": free})
		return
	}
	// Lowest priority first: later rows overwrite earlier ones.
	rows, err := db.All[mapRow](c, h.conn, `
		SELECT host(dhcp_start) AS a, host(dhcp_end) AS b, 'dhcp_free' AS state FROM segments WHERE id = $1::uuid AND dhcp_start IS NOT NULL
		UNION ALL SELECT DISTINCT host(v.ip), host(v.ip), CASE WHEN s.dhcp_start IS NOT NULL AND v.ip BETWEEN s.dhcp_start AND s.dhcp_end THEN 'dhcp_used' ELSE 'seen' END
			FROM sightings v JOIN segments s ON s.id = v.segment_id WHERE v.segment_id = $1::uuid AND `+ipam.InUse+`
		UNION ALL SELECT host(start_ip), host(end_ip), 'held' FROM reservations WHERE segment_id = $1::uuid AND status = 'active'
		UNION ALL SELECT host(gateway), host(gateway), 'gateway' FROM segments WHERE id = $1::uuid AND gateway IS NOT NULL
		UNION ALL SELECT DISTINCT host(ip), host(ip), 'gateway' FROM sightings WHERE segment_id = $1::uuid AND is_gateway
		UNION ALL SELECT host(ip), host(ip), status FROM ip_addresses WHERE segment_id = $1::uuid`, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	states := make([]string, size)
	for i := range states {
		states[i] = stAvailable
	}
	rank := map[string]int{stAvailable: 0, stDHCPFree: 1, stDHCPUsed: 2, stSeen: 3, stHeld: 4, stGateway: 5, stBlocked: 6, stReserved: 6, stAssigned: 6}
	base := toUint(usable.First)
	for _, r := range rows {
		a, e1 := netip.ParseAddr(r.A)
		b, e2 := netip.ParseAddr(r.B)
		if e1 != nil || e2 != nil {
			continue
		}
		for n := max(toUint(a), base); n <= toUint(b) && int64(n-base) < size; n++ {
			if i := n - base; rank[r.State] >= rank[states[i]] {
				states[i] = r.State
			}
		}
	}
	counts := map[string]int{}
	var runs []run
	for i, st := range states {
		counts[st]++
		ip := fromUint(base + uint32(i)).String()
		if len(runs) > 0 && runs[len(runs)-1].State == st {
			runs[len(runs)-1].End, runs[len(runs)-1].Count = ip, runs[len(runs)-1].Count+1
			continue
		}
		runs = append(runs, run{Start: ip, End: ip, State: st, Count: 1})
	}
	httpx.OK(c, http.StatusOK, gin.H{"usable": size, "mapped": true, "first": usable.First.String(), "counts": counts, "runs": runs,
		"network": info, "available": free})
}

func toUint(a netip.Addr) uint32 {
	b := a.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func fromUint(n uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
}

// freeRange is one run of Available candidates.
type freeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Count int64  `json:"count"`
}

// maxFreeRanges bounds the list; a fragmented /16 could otherwise be very long.
const maxFreeRanges = 200

func freeRanges(c *gin.Context, h *Handler, id string) ([]freeRange, error) {
	ranges, err := ipam.Available(c, h.conn, id)
	if err != nil {
		return nil, err
	}
	out := []freeRange{}
	for _, r := range ranges {
		if len(out) == maxFreeRanges {
			break
		}
		out = append(out, freeRange{Start: r.First.String(), End: r.Last.String(), Count: ipam.RangeSize(r.First, r.Last)})
	}
	return out, nil
}
