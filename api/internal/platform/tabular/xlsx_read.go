package tabular

import (
	"bytes"
	"errors"

	"github.com/xuri/excelize/v2"
)

// An .xlsx file is a zip: a few megabytes can unpack to gigabytes. These caps
// keep one upload from filling the API's memory or its in-memory /tmp.
const (
	maxUnzipBytes    = 64 << 20
	maxUnzipXMLBytes = 16 << 20
	// MaxRows is the most rows read from one sheet. Imports set lower limits
	// of their own; this only stops a sheet with millions of empty rows.
	MaxRows = 20000
)

var errTooManyRows = errors.New("the sheet has too many rows")

// readXLSX reads the first sheet row by row, within the caps above.
func readXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: maxUnzipBytes, UnzipXMLSizeLimit: maxUnzipXMLBytes})
	if err != nil {
		return nil, errors.New("the file is not a valid Excel workbook, or it unpacks too large")
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("the workbook has no sheets")
	}
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, errors.New("the first sheet can't be read")
	}
	defer rows.Close()
	out := [][]string{}
	for rows.Next() {
		if len(out) >= MaxRows {
			return nil, errTooManyRows
		}
		cols, err := rows.Columns()
		if err != nil {
			return nil, errors.New("the first sheet can't be read")
		}
		out = append(out, cols)
	}
	return out, rows.Error()
}
