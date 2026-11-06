package imports

import (
	"encoding/csv"
	"errors"
	"io"
	"strings"
)

func delimiter(text string) rune {
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(strings.TrimLeft(t, `"`), "#") {
			if strings.Count(t, ";") > strings.Count(t, ",") {
				return ';'
			}
			break
		}
	}
	return ','
}

func readCSV(text string) ([]sheetRow, error) {
	text = strings.TrimPrefix(text, "\uFEFF") // Excel's "CSV UTF-8" starts with this
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes = delimiter(text), -1, true
	var grid [][]string
	for {
		cells, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, badTemplate("This CSV can't be read: %s.", err)
		}
		// The reader skips blank lines, so keep row numbers true to the file.
		line, _ := r.FieldPos(0)
		for len(grid) < line-1 {
			grid = append(grid, nil)
		}
		grid = append(grid, cells)
	}
	if len(grid) == 0 || len(grid[0]) == 0 || !strings.HasPrefix(strings.TrimSpace(grid[0][0]), csvMarker) {
		return nil, badTemplate("This CSV doesn't come from a DF-IPAM template. Download one and keep its first line.")
	}
	s := sheetByKey(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(grid[0][0]), csvMarker)))
	if s == nil {
		return nil, badTemplate("This template's first line names a type DF-IPAM doesn't know.")
	}
	return sheetRows(s, grid)
}
