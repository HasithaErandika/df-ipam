package links

import (
	"fmt"
	"strings"

	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
)

// readSheet reads the uploaded file. The header row is lower-cased so columns
// match whatever case the file uses.
func readSheet(c *gin.Context) ([][]string, bool) {
	file, err := c.FormFile("file")
	if err != nil {
		httpx.Fail(c, httpx.Validation("file", "Choose an Excel or CSV file to upload."))
		return nil, false
	}
	f, err := file.Open()
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	defer f.Close()
	records, err := tabular.Read(file.Filename, f, maxImportBytes)
	switch {
	case err != nil:
		httpx.Fail(c, httpx.Validation("file", "This file can't be read: "+err.Error()+"."))
	case len(records) < 2:
		httpx.Fail(c, httpx.Validation("file", "The file needs a header row and at least one data row."))
	case len(records) > maxImportRows+1:
		httpx.Fail(c, httpx.Validation("file", fmt.Sprintf("Import up to %d rows at a time.", maxImportRows)))
	default:
		for i, v := range records[0] {
			records[0][i] = strings.ToLower(strings.TrimSpace(v))
		}
		if !contains(records[0], "link name") {
			httpx.Fail(c, httpx.Validation("file", "The header row needs a Link Name column. Start from the import template."))
			return nil, false
		}
		return records, true
	}
	return nil, false
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
