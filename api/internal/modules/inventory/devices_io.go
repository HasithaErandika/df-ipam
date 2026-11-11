package inventory

import (
	"fmt"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) exportDevices(c *gin.Context) {
	format, ok := exportFormat(c)
	if !ok {
		return
	}
	w, err := tabular.NewWriter(format, "Devices", []string{"mac", "name", "owner", "department", "deviceType", "platform", "status", "notes", "firstSeen", "lastSeen"})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(c, `SELECT mac::text, name, owner, department, device_type, platform, status, notes,
		COALESCE(to_char(first_seen, 'YYYY-MM-DD HH24:MI'), ''), COALESCE(to_char(last_seen, 'YYYY-MM-DD HH24:MI'), '') FROM devices ORDER BY mac`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		r := make([]string, 10)
		if err := rows.Scan(&r[0], &r[1], &r[2], &r[3], &r[4], &r[5], &r[6], &r[7], &r[8], &r[9]); err != nil {
			httpx.Fail(c, err)
			return
		}
		_ = w.Write(r)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	sendTable(c, w, format, "devices")
}

// importDevices adds approved devices from a CSV or Excel file. Rows with problems are skipped
// and reported; MACs already known are left as they are. One statement inserts
// every valid row.
func (h *Handler) importDevices(c *gin.Context) {
	rows, ok := readTable(c, "mac")
	if !ok {
		return
	}
	result := importResult{Issues: []rowIssue{}}
	var batch [6][]string // mac, name, owner, department, type, platform
	seen := map[string]bool{}
	for i, row := range rows {
		in := deviceInput{MAC: row["mac"], Name: row["name"], Owner: row["owner"], Department: row["department"], Type: row["devicetype"], Platform: row["platform"], Notes: row["notes"]}
		if in.Name == "" {
			in.Name = in.MAC
		}
		if err := in.validate(true); err != nil {
			result.Issues = append(result.Issues, rowIssue{i + 2, issueText(err)})
			continue
		}
		if seen[in.MAC] {
			result.Issues = append(result.Issues, rowIssue{i + 2, "This MAC appears earlier in the file."})
			continue
		}
		seen[in.MAC] = true
		for j, v := range []string{in.MAC, in.Name, in.Owner, in.Department, in.Type, in.Platform} {
			batch[j] = append(batch[j], v)
		}
	}
	uid := actor(c)
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		inserted, err := tx.Query(c, `INSERT INTO devices (mac, name, owner, department, device_type, platform, status, decided_by, decided_at)
			SELECT m::macaddr, n, o, d, t, p, 'approved', $7::uuid, now() FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[]) AS x(m, n, o, d, t, p)
			ON CONFLICT (mac) DO NOTHING RETURNING id::text, mac::text`, batch[0], batch[1], batch[2], batch[3], batch[4], batch[5], uid)
		if err != nil {
			return err
		}
		var entries []audit.Entry
		for inserted.Next() {
			var id, mac string
			if err := inserted.Scan(&id, &mac); err != nil {
				inserted.Close()
				return err
			}
			entries = append(entries, audit.Entry{ActorID: uid, Action: "import", ObjectType: "device", ObjectID: id, New: map[string]string{"mac": mac}})
		}
		inserted.Close()
		if err := inserted.Err(); err != nil {
			return err
		}
		result.Imported = len(entries)
		return audit.WriteMany(c, tx, entries)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	result.Skipped = len(rows) - result.Imported
	if already := len(batch[0]) - result.Imported; already > 0 {
		result.Issues = append(result.Issues, rowIssue{0, fmt.Sprintf("%d MAC addresses were already known and were left unchanged.", already)})
	}
	h.recheck(c)
	httpx.OK(c, http.StatusOK, result)
}

// issueText flattens validation problems into one line for an import report.
func issueText(err error) string {
	if e, ok := err.(*httpx.Error); ok && len(e.Details) > 0 {
		return e.Details[0].Field + ": " + e.Details[0].Issue
	}
	return err.Error()
}
