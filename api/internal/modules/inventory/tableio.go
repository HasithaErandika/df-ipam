package inventory

import (
	"fmt"
	"net/http"
	"strings"

	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
)

const (
	maxImportBytes = 5 << 20
	maxImportRows  = 5000
)

// readTable reads an uploaded CSV or Excel file whose header row names the
// columns. Each row comes back as a map from lower-case header to trimmed value.
func readTable(c *gin.Context, required ...string) ([]map[string]string, bool) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.Validation("file", "Choose a CSV or Excel file to upload."))
		return nil, false
	}
	f, err := file.Open()
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	defer f.Close()
	records, err := tabular.Read(file.Filename, f, maxImportBytes)
	if err != nil {
		httpx.Fail(c, httpx.Validation("file", "This file can't be read: "+err.Error()+"."))
		return nil, false
	}
	if len(records) < 2 {
		httpx.Fail(c, httpx.Validation("file", "The file needs a header row and at least one data row."))
		return nil, false
	}
	if len(records) > maxImportRows+1 {
		httpx.Fail(c, httpx.Validation("file", fmt.Sprintf("Import up to %d rows at a time.", maxImportRows)))
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
	rows := make([]map[string]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		row := map[string]string{}
		blank := true
		for i, v := range rec {
			if i < len(header) {
				row[header[i]] = strings.TrimSpace(v)
				blank = blank && row[header[i]] == ""
			}
		}
		if !blank { // spreadsheets often end with empty rows
			rows = append(rows, row)
		}
	}
	return rows, true
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

// importResult says what an import did. Rows with problems are skipped and listed.
type importResult struct {
	Imported int        `json:"imported"`
	Skipped  int        `json:"skipped"`
	Issues   []rowIssue `json:"issues"`
}

// exportFormat reads ?format=csv|xlsx, defaulting to CSV.
func exportFormat(c *gin.Context) (string, bool) {
	f := c.DefaultQuery("format", "csv")
	if _, ok := tabular.Formats[f]; !ok {
		httpx.Fail(c, httpx.Validation("format", "Choose CSV or Excel."))
		return "", false
	}
	return f, true
}

// sendTable finishes an export as a download.
func sendTable(c *gin.Context, w tabular.Writer, format, name string) {
	data, err := w.Bytes()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+"."+format+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, tabular.Formats[format], data)
}
