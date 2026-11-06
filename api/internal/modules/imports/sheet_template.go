package imports

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

const (
	xlsxType     = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	workbookName = "dfipam-import-template.xlsx"
)

type templateColumn struct {
	Name     string   `json:"name"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	RefersTo string   `json:"refersTo,omitempty"`
}

type templateInfo struct {
	Key     string           `json:"key"`
	Title   string           `json:"title"`
	Columns []templateColumn `json:"columns"`
}

func (h *Handler) templates(c *gin.Context) {
	out := make([]templateInfo, len(sheets))
	for i := range sheets {
		s := &sheets[i]
		out[i] = templateInfo{Key: s.Key, Title: s.Title, Columns: make([]templateColumn, len(s.Columns))}
		for j, col := range s.Columns {
			out[i].Columns[j] = templateColumn{Name: col.Name, Required: col.Required, Options: col.Enum}
			if col.Ref != "" {
				out[i].Columns[j].RefersTo = refLabel(col.Ref)
			}
		}
	}
	httpx.OK(c, http.StatusOK, out)
}

func headers(s *sheet) []string {
	out := make([]string, len(s.Columns))
	for i, c := range s.Columns {
		out[i] = c.Name
		if c.Required {
			out[i] += " *"
		}
	}
	return out
}

// exampleRow is ignored on upload: rows starting with # carry no data.
func exampleRow(s *sheet) []string {
	out := make([]string, len(s.Columns))
	for i, c := range s.Columns {
		out[i] = c.Example
	}
	out[0] = "# " + out[0]
	return out
}

func templateCSV(s *sheet) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{csvMarker + " " + s.Key})
	_ = w.Write(headers(s))
	_ = w.Write(exampleRow(s))
	w.Flush()
	return b.Bytes(), w.Error()
}

var readMe = []string{
	"DF-IPAM import template",
	"",
	"1. Fill in the sheets you need and leave the others empty. Sheets run parents first.",
	"2. Columns marked * are required. Don't rename, or add, columns.",
	"3. The row starting with # is an example and is skipped. Add your rows below it.",
	"4. Refer to other records by name or code. They can be in the inventory or on an earlier sheet.",
	"5. If a name is used twice, write it with its parent, such as DF/Default.",
	"6. Upload the file. Every row is checked and nothing changes until someone else approves.",
	"",
	"An import only adds records. A row that already exists is skipped, never changed.",
}

func writeSheetTab(f *excelize.File, s *sheet) error {
	if _, err := f.NewSheet(s.Title); err != nil {
		return err
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E6EEFC"}}})
	if err != nil {
		return err
	}
	for i, values := range [][]string{headers(s), exampleRow(s)} {
		row := make([]any, len(values))
		for j, v := range values {
			row[j] = v
		}
		if err := f.SetSheetRow(s.Title, fmt.Sprintf("A%d", i+1), &row); err != nil {
			return err
		}
	}
	last, err := excelize.ColumnNumberToName(len(s.Columns))
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(s.Title, "A1", last+"1", bold); err != nil {
		return err
	}
	if err := f.SetColWidth(s.Title, "A", last, 22); err != nil {
		return err
	}
	if err := f.SetPanes(s.Title, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	return addDropdowns(f, s)
}

func addDropdowns(f *excelize.File, s *sheet) error {
	for i, c := range s.Columns {
		if c.Enum == nil {
			continue
		}
		name, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		dv := excelize.NewDataValidation(true)
		dv.Sqref = fmt.Sprintf("%s3:%s%d", name, name, maxSheetRows)
		if err := dv.SetDropList(c.Enum); err != nil {
			return err
		}
		if err := f.AddDataValidation(s.Title, dv); err != nil {
			return err
		}
	}
	return nil
}

func templateWorkbook() ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", readMeTitle); err != nil {
		return nil, err
	}
	for i, line := range readMe {
		if err := f.SetCellStr(readMeTitle, fmt.Sprintf("A%d", i+1), line); err != nil {
			return nil, err
		}
	}
	if err := f.SetColWidth(readMeTitle, "A", "A", 100); err != nil {
		return nil, err
	}
	for i := range sheets {
		if err := writeSheetTab(f, &sheets[i]); err != nil {
			return nil, err
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (h *Handler) template(c *gin.Context) {
	key := c.Param("key")
	if key == "workbook" {
		data, err := templateWorkbook()
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		c.Header("Content-Disposition", `attachment; filename="`+workbookName+`"`)
		c.Data(http.StatusOK, xlsxType, data)
		return
	}
	s := sheetByKey(key)
	if s == nil {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	data, err := templateCSV(s)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="dfipam-`+strings.ReplaceAll(s.Key, "_", "-")+`.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}
