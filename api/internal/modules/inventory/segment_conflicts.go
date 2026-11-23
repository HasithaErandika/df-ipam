package inventory

import (
	"fmt"
	"net/http"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// segmentConflicts keeps an edited segment true to the register's rules: a DHCP
// pool address is never registered, and the gateway is registered only as type
// network. Moving the pool or gateway onto existing entries is refused, the same
// as registering those entries by hand would be.
func segmentConflicts(c *gin.Context, tx pgx.Tx, id string, in segmentInput) error {
	var inPool int
	var atGateway string
	err := tx.QueryRow(c, `SELECT
			(SELECT count(*) FROM ip_addresses a WHERE a.segment_id = $1::uuid AND NULLIF($2, '') IS NOT NULL
				AND a.ip BETWEEN $2::inet AND $3::inet),
			COALESCE((SELECT host(a.ip) FROM ip_addresses a WHERE a.segment_id = $1::uuid AND NULLIF($4, '') IS NOT NULL
				AND a.ip = $4::inet AND a.ip_type <> 'network'), '')`,
		id, in.DHCPStart, in.DHCPEnd, in.Gateway).Scan(&inPool, &atGateway)
	if err != nil {
		return err
	}
	if inPool > 0 {
		return httpx.Validation("dhcpStart", fmt.Sprintf("%d register %s inside this DHCP pool. Move or remove %s, or choose another pool.",
			inPool, plural(inPool, "entry is", "entries are"), plural(inPool, "it", "them")))
	}
	if atGateway != "" {
		return httpx.Validation("gateway", atGateway+" is in the register with a type other than network. Change its type to network first.")
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// segmentUsedBy says what still points at a segment, so delete can refuse it
// with a reason people can act on.
func segmentUsedBy(c *gin.Context, tx pgx.Tx, id string) error {
	var register, links bool
	if err := tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM ip_addresses WHERE segment_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM reservations WHERE segment_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM ip_requests WHERE segment_id = $1::uuid),
		EXISTS (SELECT 1 FROM network_links WHERE wan_segment_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM network_link_segments WHERE segment_id = $1::uuid)`, id).Scan(&register, &links); err != nil {
		return err
	}
	if register {
		return httpx.NewError(http.StatusConflict, "SEGMENT_IN_USE", "This segment still has register entries, reservations or requests. Remove them first.")
	}
	if links {
		return httpx.NewError(http.StatusConflict, "SEGMENT_IN_USE", "Network links still use this segment as their WAN subnet or one of their segments. Change those links first.")
	}
	return nil
}
