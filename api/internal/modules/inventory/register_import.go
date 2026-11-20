package inventory

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// importRow is one valid row, as the columns the insert takes.
type importRow struct {
	in                     ipInput
	env, services, monitor string
	critical, port         string
}

// importRegister adds register entries from a CSV or Excel file with the same
// columns as the export, so an export can be edited and imported back. Each IP
// goes to the segment whose network holds it; the segment column is ignored.
// With ?as=server, a blank type means Server, and blank Monitor and Critical
// take the environment's defaults. Rows with problems, and addresses the register
// refuses by hand too, are skipped and reported. One statement inserts the rest.
func (h *Handler) importRegister(c *gin.Context) {
	rows, ok := readTable(c, "ip")
	if !ok {
		return
	}
	lists, err := loadImportLists(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	asServer, dry := c.Query("as") == "server", isDryRun(c)
	result := importResult{DryRun: dry, Issues: []rowIssue{}, Preview: []previewRow{}}
	var valid []importRow
	line := map[string]int{}
	for i, row := range rows {
		x, problem := parseImportRow(row, lists, asServer)
		if problem == "" && line[x.in.IP] != 0 {
			problem = "This IP appears earlier in the file."
		}
		if problem != "" {
			result.Issues = append(result.Issues, rowIssue{i + 2, problem})
			continue
		}
		line[x.in.IP] = i + 2
		valid = append(valid, x)
	}
	err = importTx(c, h.conn, dry, func(tx pgx.Tx) error {
		names := map[string]string{}
		for _, x := range valid {
			names[x.in.IP] = x.in.Name
		}
		blocked, err := importBlocks(c, tx, valid)
		if err != nil {
			return err
		}
		for ip, why := range blocked {
			result.Issues = append(result.Issues, rowIssue{line[ip], why})
			delete(line, ip)
		}
		ips, err := insertImported(c, tx, withoutBlocked(valid, blocked), asServer)
		for _, ip := range ips {
			result.Preview = append(result.Preview, previewRow{Row: line[ip], Key: ip, Label: names[ip]})
			delete(line, ip)
		}
		result.Imported = len(ips)
		return err
	})
	if err != nil {
		httpx.Fail(c, friendlyIP(err))
		return
	}
	for ip, n := range line {
		result.Issues = append(result.Issues, rowIssue{n, ip + " was registered or reserved by someone else during the import."})
	}
	result.Skipped = len(rows) - result.Imported
	slices.SortFunc(result.Issues, func(a, b rowIssue) int { return a.Row - b.Row })
	slices.SortFunc(result.Preview, func(a, b previewRow) int { return a.Row - b.Row })
	if !dry {
		h.recheck(c)
	}
	httpx.OK(c, http.StatusOK, result)
}

// parseImportRow checks one row and returns it ready to insert, or why not.
func parseImportRow(row map[string]string, lists importLists, asServer bool) (importRow, string) {
	x := importRow{in: ipInput{SegmentID: "00000000-0000-0000-0000-000000000000", IP: row["ip"], Status: row["status"], Type: strings.ToLower(row["iptype"]),
		Name: row["assignedname"], MAC: row["expectedmac"], Owner: row["owner"], Department: row["department"], Notes: row["notes"],
		Platform: strings.ToLower(strings.TrimSpace(row["platform"])), Services: splitList(row["services"])}}
	if x.in.Type == "" && asServer {
		x.in.Type = "server"
	}
	if name := strings.ToLower(strings.TrimSpace(row["environment"])); name != "" {
		if x.env = lists.envs[name]; x.env == "" {
			return x, "Unknown environment " + row["environment"] + ". Add it in Settings first."
		}
	}
	var bad string
	if x.in.Department, bad = importDepartment(x.in.Department, lists.depts); bad != "" {
		return x, bad
	}
	x.monitor, bad = yesNo(row["monitor"], "monitor")
	if bad == "" {
		x.critical, bad = yesNo(row["critical"], "critical")
	}
	if bad != "" {
		return x, bad
	}
	x.port = strings.TrimSpace(row["checkport"])
	if x.port != "" {
		port, err := strconv.Atoi(x.port)
		if err != nil {
			return x, "Check port must be a number from 1 to 65535."
		}
		x.in.Port = &port
	}
	// Critical needs monitoring, so Critical with Monitor left blank turns it on.
	if x.critical == "true" && x.monitor == "" {
		x.monitor = "true"
	}
	// Other defaults are applied in SQL; validate what the row says.
	x.in.Monitor, x.in.Critical = x.monitor == "true", x.critical == "true"
	if err := x.in.validate(true); err != nil {
		return x, issueText(err)
	}
	x.services = strings.Join(x.in.Services, " ")
	return x, ""
}

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
			JOIN segments s ON x.ip::inet << s.network
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
