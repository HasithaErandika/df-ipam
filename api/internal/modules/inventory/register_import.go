package inventory

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

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
	valid, line := parseImportRows(rows, lists, asServer, &result)
	err = importTx(c, h.conn, dry, func(tx pgx.Tx) error {
		return applyImport(c, tx, valid, line, asServer, &result)
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

// parseImportRows checks every row and reports the ones with problems. It
// returns the rest, and the file row of each IP among them.
func parseImportRows(rows []map[string]string, lists importLists, asServer bool, result *importResult) ([]importRow, map[string]int) {
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
	return valid, line
}

// applyImport reports the rows the register refuses and inserts the rest.
// Each IP it deals with leaves line, so what remains went to someone else.
func applyImport(c *gin.Context, tx pgx.Tx, valid []importRow, line map[string]int, asServer bool, result *importResult) error {
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
