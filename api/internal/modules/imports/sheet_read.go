package imports

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"dfipam/api/internal/platform/httpx"

	"github.com/xuri/excelize/v2"
)

const (
	templateFormat = "template"
	csvMarker      = "# dfipam-template:"
	readMeTitle    = "Read me"
	maxSheetRows   = 10000
	// A small file can unpack to something huge, so unpacking has its own limits.
	maxUnzipped, maxUnzippedXML = 64 << 20, 32 << 20
)

// sheetRow is one data row of a template file.
type sheetRow struct {
	sheet  *sheet
	line   int // position across the whole file
	number int // row number as the person sees it
	raw    string
	fields map[string]string
}

func badTemplate(format string, args ...any) error {
	return httpx.NewError(http.StatusUnprocessableEntity, "IMPORT_BAD_TEMPLATE", fmt.Sprintf(format, args...))
}

func headerName(s string) string {
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), "*"))
	return strings.ReplaceAll(strings.ToLower(s), " ", "_")
}

// readHeader maps each position to its column. An unknown column is refused rather
// than ignored, so a typing mistake can't quietly lose a whole column of data.
func readHeader(s *sheet, cells []string) ([]string, error) {
	names := make([]string, len(cells))
	seen := map[string]bool{}
	for i, cell := range cells {
		name := headerName(cell)
		if name == "" {
			continue
		}
		if s.column(name) == nil {
			return nil, badTemplate("%s has a column this template doesn't know: %q.", s.Title, cell)
		}
		names[i], seen[name] = name, true
	}
	for _, c := range s.Columns {
		if c.Required && !seen[c.Name] {
			return nil, badTemplate("%s is missing the required column %q.", s.Title, c.Name)
		}
	}
	return names, nil
}

func isBlank(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func isNote(cells []string) bool {
	return len(cells) > 0 && strings.HasPrefix(strings.TrimSpace(cells[0]), "#")
}

// sheetRows turns a grid into rows. Blank rows and rows starting with # carry no data.
func sheetRows(s *sheet, grid [][]string) ([]sheetRow, error) {
	var names []string
	var out []sheetRow
	for i, cells := range grid {
		if isBlank(cells) || isNote(cells) {
			continue
		}
		if names == nil {
			var err error
			if names, err = readHeader(s, cells); err != nil {
				return nil, err
			}
			continue
		}
		fields := map[string]string{}
		for j, cell := range cells {
			if j < len(names) && names[j] != "" && strings.TrimSpace(cell) != "" {
				fields[names[j]] = strings.TrimSpace(cell)
			}
		}
		out = append(out, sheetRow{sheet: s, number: i + 1, raw: strings.Join(cells, ","), fields: fields})
	}
	return out, nil
}

func readWorkbook(data []byte) ([]sheetRow, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: maxUnzipped, UnzipXMLSizeLimit: maxUnzippedXML})
	if err != nil {
		return nil, badTemplate("This workbook can't be read. Save it as .xlsx and try again.")
	}
	defer func() { _ = f.Close() }()
	var out []sheetRow
	for _, name := range f.GetSheetList() {
		if name == readMeTitle {
			continue
		}
		s := sheetByTitle(name)
		if s == nil {
			return nil, badTemplate("This workbook has a sheet DF-IPAM doesn't know: %q.", name)
		}
		grid, err := f.GetRows(name)
		if err != nil {
			return nil, badTemplate("Sheet %q can't be read.", name)
		}
		rows, err := sheetRows(s, grid)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

// readTemplate reads a file and numbers its rows, parents first.
func readTemplate(fileName string, data []byte) ([]*sheetRow, error) {
	var rows []sheetRow
	var err error
	if strings.HasSuffix(strings.ToLower(fileName), ".xlsx") {
		rows, err = readWorkbook(data)
	} else {
		rows, err = readCSV(string(data))
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, badTemplate("This file has no rows to import. Rows starting with # are examples and are skipped.")
	}
	if len(rows) > maxSheetRows {
		return nil, badTemplate("This file has %d rows. Split it into files of at most %d.", len(rows), maxSheetRows)
	}
	return orderRows(rows), nil
}

func orderRows(rows []sheetRow) []*sheetRow {
	out := make([]*sheetRow, 0, len(rows))
	for i := range sheets {
		for j := range rows {
			if rows[j].sheet == &sheets[i] {
				out = append(out, &rows[j])
			}
		}
	}
	for i, r := range out {
		r.line = i + 1
	}
	return out
}
