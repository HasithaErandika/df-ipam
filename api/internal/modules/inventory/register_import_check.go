package inventory

import (
	"strings"

	"dfipam/api/internal/platform/db"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// importLists are the environments and departments an import row may name.
type importLists struct{ envs, depts map[string]string }

func loadImportLists(c *gin.Context, conn db.Conn) (importLists, error) {
	envs, err := environmentsByName(c, conn)
	if err != nil {
		return importLists{}, err
	}
	depts, err := departmentsByName(c, conn)
	return importLists{envs, depts}, err
}

// importBlocks says, in one query, why each row's address can't be registered:
// the same rules as registering by hand (ipam.RegisterIssue). Rows it names
// are left out of the insert and reported.
func importBlocks(c *gin.Context, tx pgx.Tx, valid []importRow) (map[string]string, error) {
	ips, types := make([]string, len(valid)), make([]string, len(valid))
	for i, x := range valid {
		ips[i], types[i] = x.in.IP, x.in.Type
	}
	rows, err := tx.Query(c, `SELECT x.ip, CASE
			WHEN s.id IS NULL THEN 'No segment contains this IP.'
			WHEN i.id IS NOT NULL THEN 'This IP is already in the register.'
			WHEN masklen(s.network) <= 30 AND x.ip::inet IN (set_masklen(s.network::inet, 32), set_masklen(broadcast(s.network), 32))
				THEN 'This is the segment''s network or broadcast address.'
			WHEN s.dhcp_start IS NOT NULL AND x.ip::inet BETWEEN s.dhcp_start AND s.dhcp_end
				THEN 'This IP is inside the DHCP pool, so it changes. Approve the device instead.'
			WHEN x.ip::inet = s.gateway AND x.ty <> 'network' THEN 'This is the segment''s gateway. Give it the type network.'
			WHEN EXISTS (SELECT 1 FROM reservations r WHERE r.status = 'active' AND x.ip::inet BETWEEN r.start_ip AND r.end_ip)
				THEN 'This IP is held by a reservation.'
		END
		FROM unnest($1::text[], $2::text[]) AS x(ip, ty)
		LEFT JOIN segments s ON x.ip::inet << s.network
		LEFT JOIN ip_addresses i ON i.ip = x.ip::inet`, ips, types)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var ip string
		var issue *string
		if err := rows.Scan(&ip, &issue); err != nil {
			return nil, err
		}
		if issue != nil {
			out[ip] = *issue
		}
	}
	return out, rows.Err()
}

// withoutBlocked keeps the rows importBlocks found nothing against.
func withoutBlocked(valid []importRow, blocked map[string]string) []importRow {
	var out []importRow
	for _, x := range valid {
		if blocked[x.in.IP] == "" {
			out = append(out, x)
		}
	}
	return out
}

// yesNo reads a yes/no cell as "true", "false", or "" when blank.
func yesNo(cell, field string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(cell)) {
	case "":
		return "", ""
	case "true", "yes", "y", "1":
		return "true", ""
	case "false", "no", "n", "0":
		return "false", ""
	}
	return "", "Write yes or no for " + field + "."
}

func splitList(cell string) []string {
	return strings.FieldsFunc(strings.ToLower(cell), func(r rune) bool { return r == ' ' || r == ',' || r == ';' || r == '|' })
}

func environmentsByName(c *gin.Context, conn db.Conn) (map[string]string, error) {
	rows, err := conn.Query(c, "SELECT lower(name), id::text FROM environments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			return nil, err
		}
		out[name] = id
	}
	return out, rows.Err()
}
