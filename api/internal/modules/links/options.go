package links

import (
	"net/http"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type option struct {
	ID     string `db:"id" json:"id"`
	Label  string `db:"label" json:"label"`
	Detail string `db:"detail" json:"detail"`
}

// options lists what the link form picks from: people, subnets, devices and
// other links. Providers come from /link-providers.
func (h *Handler) options(c *gin.Context) {
	queries := map[string]string{
		"officers": `SELECT id::text AS id, COALESCE(NULLIF(display_name, ''), email) AS label, email AS detail
			FROM users WHERE is_active ORDER BY lower(COALESCE(NULLIF(display_name, ''), email)) LIMIT 2000`,
		"segments": `SELECT id::text AS id, name AS label, network::text AS detail FROM segments ORDER BY network`,
		"devices": `SELECT id::text AS id, COALESCE(NULLIF(name, ''), mac::text) AS label, mac::text AS detail
			FROM devices WHERE status = 'approved' ORDER BY lower(COALESCE(NULLIF(name, ''), mac::text)) LIMIT 5000`,
		"links": `SELECT id::text AS id, code || ' · ' || name AS label, link_type AS detail FROM network_links
			WHERE archived_at IS NULL ORDER BY code`,
		"registerIps": `SELECT host(ip) AS id, host(ip) || CASE WHEN assigned_name <> '' THEN ' · ' || assigned_name ELSE '' END AS label,
			segment_id::text AS detail FROM ip_addresses ORDER BY ip LIMIT 20000`,
	}
	out := gin.H{}
	for key, q := range queries {
		rows, err := db.All[option](c, h.conn, q)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out[key] = rows
	}
	httpx.OK(c, http.StatusOK, out)
}
