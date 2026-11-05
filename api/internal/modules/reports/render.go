package reports

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// Row limits per format. A report that reaches one says so: it is marked truncated
// and carries a notice, so nobody mistakes it for the whole register.
const (
	maxRowsSheet = 500_000
	maxRowsPDF   = 5_000
	maxFileBytes = 64 << 20
)

// Formats and their file details.
var formats = map[string]struct{ contentType, extension string }{
	"csv":  {"text/csv; charset=utf-8", "csv"},
	"xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"},
	"pdf":  {"application/pdf", "pdf"},
}

func limitFor(format string) int64 {
	if format == "pdf" {
		return maxRowsPDF
	}
	return maxRowsSheet
}

// safeCell stops a spreadsheet treating inventory text as a formula. Hostnames,
// descriptions and DHCP names come from the network, so anyone on it could plant one.
func safeCell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r|%", rune(v[0])) {
		return "'" + v
	}
	return v
}

// renderer receives rows one at a time.
type renderer interface {
	row(cells []string) error
	finish(truncated bool, limit int64) ([]byte, error)
}

func newRenderer(format string, d Definition, asOf time.Time) (renderer, error) {
	switch format {
	case "csv":
		return newCSV(d)
	case "xlsx":
		return newSheet(d)
	case "pdf":
		return newPDF(d, asOf), nil
	}
	return nil, errors.New("unknown format")
}

type csvRenderer struct {
	buf bytes.Buffer
	w   *csv.Writer
}

func newCSV(d Definition) (*csvRenderer, error) {
	r := &csvRenderer{}
	r.w = csv.NewWriter(&r.buf)
	return r, r.w.Write(d.Columns)
}

func (r *csvRenderer) row(cells []string) error {
	safe := make([]string, len(cells))
	for i, c := range cells {
		safe[i] = safeCell(c)
	}
	if r.buf.Len() > maxFileBytes {
		return errTooLarge
	}
	return r.w.Write(safe)
}

func (r *csvRenderer) finish(truncated bool, limit int64) ([]byte, error) {
	if truncated {
		if err := r.w.Write([]string{fmt.Sprintf("Truncated at %d rows. Narrow the filters to see the rest.", limit)}); err != nil {
			return nil, err
		}
	}
	r.w.Flush()
	return r.buf.Bytes(), r.w.Error()
}

var errTooLarge = errors.New("the report is too large: narrow the filters")

const sheetName = "Report"

type sheetRenderer struct {
	f    *excelize.File
	w    *excelize.StreamWriter
	line int
}

func newSheet(d Definition) (*sheetRenderer, error) {
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", sheetName); err != nil {
		return nil, err
	}
	w, err := f.NewStreamWriter(sheetName)
	if err != nil {
		return nil, err
	}
	r := &sheetRenderer{f, w, 0}
	return r, r.row(d.Columns)
}

func (r *sheetRenderer) row(cells []string) error {
	r.line++
	values := make([]any, len(cells))
	for i, c := range cells {
		values[i] = safeCell(c)
	}
	cell, err := excelize.CoordinatesToCellName(1, r.line)
	if err != nil {
		return err
	}
	return r.w.SetRow(cell, values)
}

func (r *sheetRenderer) finish(truncated bool, limit int64) ([]byte, error) {
	defer r.f.Close()
	if truncated {
		if err := r.row([]string{fmt.Sprintf("Truncated at %d rows. Narrow the filters to see the rest.", limit)}); err != nil {
			return nil, err
		}
	}
	if err := r.w.Flush(); err != nil {
		return nil, err
	}
	buf, err := r.f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
