package inventory

import (
	"net/http"
	"slices"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var registerHeader = []string{"ip", "segment", "status", "ipType", "environment", "assignedName", "platform", "services", "expectedMac", "owner", "department", "monitor", "critical", "checkPort", "health", "notes"}

func (h *Handler) exportRegister(c *gin.Context) {
	format, ok := exportFormat(c)
	if !ok {
		return
	}
	w, err := registerFilter(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := tabular.NewWriter(format, "IP register", registerHeader)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(c, `SELECT host(i.ip), s.name, i.status, i.ip_type, COALESCE(e.name, ''), i.assigned_name, i.platform, array_to_string(i.services, ' '), COALESCE(i.expected_mac::text, ''),
		i.owner, i.department, i.monitor::text, i.critical::text, COALESCE(i.check_port::text, ''), i.health, i.notes`+ipFrom+w.SQL()+" ORDER BY i.ip", w.Args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		r := make([]string, len(registerHeader))
		dest := make([]any, len(r))
		for i := range r {
			dest[i] = &r[i]
		}
		if err := rows.Scan(dest...); err != nil {
			httpx.Fail(c, err)
			return
		}
		_ = out.Write(r)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	sendTable(c, out, format, "ip-register")
}

// importRegister adds register entries from a CSV or Excel file. Each IP goes to the segment
// whose network holds it. Rows with problems, IPs outside every segment and IPs
// already registered are skipped and reported. One statement inserts the rest.
func (h *Handler) importRegister(c *gin.Context) {
	rows, ok := readTable(c, "ip")
	if !ok {
		return
	}
	result := importResult{Issues: []rowIssue{}}
	var cols [8][]string // ip, status, type, name, mac, owner, department, notes
	line := map[string]int{}
	for i, row := range rows {
		in := ipInput{SegmentID: "00000000-0000-0000-0000-000000000000", IP: row["ip"], Status: row["status"], Type: row["iptype"],
			Name: row["assignedname"], MAC: row["expectedmac"], Owner: row["owner"], Department: row["department"], Notes: row["notes"]}
		if err := in.validate(true); err != nil {
			result.Issues = append(result.Issues, rowIssue{i + 2, issueText(err)})
			continue
		}
		if _, dup := line[in.IP]; dup {
			result.Issues = append(result.Issues, rowIssue{i + 2, "This IP appears earlier in the file."})
			continue
		}
		line[in.IP] = i + 2
		for j, v := range []string{in.IP, in.Status, in.Type, in.Name, in.MAC, in.Owner, in.Department, in.Notes} {
			cols[j] = append(cols[j], v)
		}
	}
	uid := actor(c)
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		inserted, err := tx.Query(c, `INSERT INTO ip_addresses (segment_id, ip, status, ip_type, environment_id, assigned_name, expected_mac,
			owner, department, notes, created_by, updated_by)
			SELECT s.id, x.ip::inet, x.st, x.ty, s.default_environment_id, x.nm, NULLIF(x.mac, '')::macaddr, x.ow, x.dp, x.nt, $9::uuid, $9::uuid
			FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[]) AS x(ip, st, ty, nm, mac, ow, dp, nt)
			JOIN segments s ON x.ip::inet << s.network
			ON CONFLICT (ip) DO NOTHING RETURNING id::text, host(ip)`,
			cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6], cols[7], uid)
		if err != nil {
			return err
		}
		var entries []audit.Entry
		for inserted.Next() {
			var id, ip string
			if err := inserted.Scan(&id, &ip); err != nil {
				inserted.Close()
				return err
			}
			delete(line, ip)
			entries = append(entries, audit.Entry{ActorID: uid, Action: "import", ObjectType: "ip_address", ObjectID: id, New: map[string]string{"ip": ip}})
		}
		inserted.Close()
		if err := inserted.Err(); err != nil {
			return err
		}
		result.Imported = len(entries)
		return audit.WriteMany(c, tx, entries)
	})
	if err != nil {
		httpx.Fail(c, friendlyIP(err))
		return
	}
	for ip, n := range line {
		result.Issues = append(result.Issues, rowIssue{n, ip + " is already registered, or no segment contains it."})
	}
	result.Skipped = len(rows) - result.Imported
	slices.SortFunc(result.Issues, func(a, b rowIssue) int { return a.Row - b.Row })
	httpx.OK(c, http.StatusOK, result)
}
