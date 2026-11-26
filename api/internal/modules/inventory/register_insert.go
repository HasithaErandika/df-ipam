package inventory

import (
	"dfipam/api/internal/audit"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// insertImported inserts the valid rows in one statement, audits each, and
// returns the IPs that went in.
func insertImported(c *gin.Context, tx pgx.Tx, valid []importRow, asServer bool) ([]string, error) {
	var cols [14][]string
	for _, x := range valid {
		for j, v := range []string{x.in.IP, x.in.Status, x.in.Type, x.in.Name, x.in.MAC, x.in.Owner, x.in.Department, x.in.Notes,
			x.env, x.in.Platform, x.services, x.monitor, x.critical, x.port} {
			cols[j] = append(cols[j], v)
		}
	}
	uid := actor(c)
	inserted, err := tx.Query(c, `WITH x AS (
			SELECT x.*, s.id AS segment_id, COALESCE(NULLIF(x.env, '')::uuid, s.default_environment_id) AS env_id
			FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[],
				$9::text[], $10::text[], $11::text[], $12::text[], $13::text[], $14::text[]) AS x(ip, st, ty, nm, mac, ow, dp, nt, env, pf, sv, mon, crit, port)
			JOIN segments s ON x.ip::inet <<= s.network
		), y AS (
			SELECT x.*, COALESCE(NULLIF(x.mon, '')::boolean, $16 AND COALESCE(e.default_monitor, false)) AS monitor,
				COALESCE(NULLIF(x.crit, '')::boolean, $16 AND COALESCE(e.default_critical, false)) AS critical
			FROM x LEFT JOIN environments e ON e.id = x.env_id
		)
		INSERT INTO ip_addresses (segment_id, ip, status, ip_type, environment_id, assigned_name, expected_mac, owner, department, notes,
			platform, services, monitor, critical, check_port, created_by, updated_by)
		SELECT segment_id, ip::inet, st, ty, env_id, nm, NULLIF(mac, '')::macaddr, ow, dp, nt, pf, COALESCE(string_to_array(NULLIF(sv, ''), ' '), '{}'),
			monitor, critical AND monitor, NULLIF(port, '')::integer, $15::uuid, $15::uuid FROM y
		WHERE NOT EXISTS (SELECT 1 FROM reservations r WHERE r.status = 'active' AND y.ip::inet BETWEEN r.start_ip AND r.end_ip)
		ON CONFLICT (ip) DO NOTHING RETURNING id::text, host(ip)`,
		cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6], cols[7], cols[8], cols[9], cols[10], cols[11], cols[12], cols[13], uid, asServer)
	if err != nil {
		return nil, err
	}
	var entries []audit.Entry
	var ips []string
	for inserted.Next() {
		var id, ip string
		if err := inserted.Scan(&id, &ip); err != nil {
			inserted.Close()
			return nil, err
		}
		ips = append(ips, ip)
		entries = append(entries, audit.Entry{ActorID: uid, Action: "import", ObjectType: "ip_address", ObjectID: id, New: map[string]string{"ip": ip}})
	}
	inserted.Close()
	if err := inserted.Err(); err != nil {
		return nil, err
	}
	return ips, audit.WriteMany(c, tx, entries)
}
