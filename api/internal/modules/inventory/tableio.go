package inventory

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	maxImportBytes = 5 << 20
	maxImportRows  = 5000
)

// readTable reads an uploaded CSV or Excel file whose header row names the
// columns. Each row comes back as a map from lower-case header to trimmed value.
func readTable(c *gin.Context, required ...string) ([]map[string]string, bool) {
	records, err := readRecords(c)
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	header := make([]string, len(records[0]))
	for i, h := range records[0] {
		header[i] = strings.ToLower(strings.TrimSpace(h))
	}
	for _, name := range required {
		if !contains(header, name) {
			httpx.Fail(c, httpx.Validation("file", "The header row needs a "+name+" column."))
			return nil, false
		}
	}
	return tableRows(header, records[1:]), true
}

// readRecords reads the uploaded file and checks it has a header and a
// reasonable number of rows.
func readRecords(c *gin.Context) ([][]string, error) {
	file, err := c.FormFile("file")
	if err != nil {
		return nil, httpx.Validation("file", "Choose a CSV or Excel file to upload.")
	}
	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	records, err := tabular.Read(file.Filename, f, maxImportBytes)
	if err != nil {
		return nil, httpx.Validation("file", "This file can't be read: "+err.Error()+".")
	}
	if len(records) < 2 {
		return nil, httpx.Validation("file", "The file needs a header row and at least one data row.")
	}
	if len(records) > maxImportRows+1 {
		return nil, httpx.Validation("file", fmt.Sprintf("Import up to %d rows at a time.", maxImportRows))
	}
	return records, nil
}

// tableRows maps each record by header, dropping blank rows: spreadsheets
// often end with empty rows.
func tableRows(header []string, records [][]string) []map[string]string {
	rows := make([]map[string]string, 0, len(records))
	for _, rec := range records {
		row := map[string]string{}
		blank := true
		for i, v := range rec {
			if i < len(header) {
				row[header[i]] = strings.TrimSpace(v)
				blank = blank && row[header[i]] == ""
			}
		}
		if !blank {
			rows = append(rows, row)
		}
	}
	return rows
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// rowIssue is one rejected import row, numbered as a spreadsheet shows it.
type rowIssue struct {
	Row   int    `json:"row"`
	Issue string `json:"issue"`
}

// importResult says what an import did, or would do on a dry run. Rows with
// problems are skipped and listed; Preview lists the rows that go in.
type importResult struct {
	DryRun   bool         `json:"dryRun"`
	Imported int          `json:"imported"`
	Skipped  int          `json:"skipped"`
	Issues   []rowIssue   `json:"issues"`
	Preview  []previewRow `json:"preview"`
}

// previewRow is one row an import adds: its line in the file, its key (an IP or
// MAC) and a label people recognise.
type previewRow struct {
	Row   int    `json:"row"`
	Key   string `json:"key"`
	Label string `json:"label"`
}

// errDryRun rolls back an import that was only previewed.
var errDryRun = errors.New("dry run")

// isDryRun reads ?dryRun=true: run the import, report it, and keep nothing.
func isDryRun(c *gin.Context) bool { return c.Query("dryRun") == "true" }

// importTx runs an import in one transaction. On a dry run it does all the
// work, so the report is exact, then rolls it back.
func importTx(c *gin.Context, conn db.Conn, dry bool, fn func(pgx.Tx) error) error {
	err := db.InTx(c, conn, func(tx pgx.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		if dry {
			return errDryRun
		}
		return nil
	})
	if errors.Is(err, errDryRun) {
		return nil
	}
	return err
}

// exportFormat reads ?format=csv|txt|xlsx, defaulting to CSV.
func exportFormat(c *gin.Context) (string, bool) {
	f := c.DefaultQuery("format", "csv")
	if _, ok := tabular.Formats[f]; !ok {
		httpx.Fail(c, httpx.Validation("format", "Choose CSV, text or Excel."))
		return "", false
	}
	return f, true
}

// sendTable finishes an export as a download.
func (h *Handler) sendTable(c *gin.Context, w tabular.Writer, format, name string) {
	data, err := w.Bytes()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := audit.Export(c, h.conn, actor(c), name, format, c.Request.URL.Query()); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+"."+format+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, tabular.Formats[format], data)
}
