package reports

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
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

type pdfRenderer struct {
	pdf       *fpdf.Fpdf
	translate func(string) string
	widths    []float64
	columns   []string
}

const (
	pdfUsable = 277.0 // A4 landscape less margins, in millimetres
	pdfLine   = 5.0
)

func newPDF(d Definition, asOf time.Time) *pdfRenderer {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 12)
	pdf.SetCreator("DF-IPAM", false)
	pdf.SetTitle(d.Title, false)
	translate := pdf.UnicodeTranslatorFromDescriptor("")
	r := &pdfRenderer{pdf: pdf, translate: translate, columns: d.Columns}
	width := pdfUsable / float64(len(d.Columns))
	for range d.Columns {
		r.widths = append(r.widths, width)
	}
	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(232, 238, 248)
		for i, c := range r.columns {
			pdf.CellFormat(r.widths[i], pdfLine+1, r.fit(c, r.widths[i]), "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 7)
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "", 7)
		pdf.CellFormat(0, 4, translate(fmt.Sprintf("DF-IPAM | %s | Data as of %s | Confidential | Page %d", d.Title, asOf.UTC().Format("2006-01-02 15:04 UTC"), pdf.PageNo())), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	return r
}

// fit shortens text so a cell never wraps or spills into the next column.
func (r *pdfRenderer) fit(text string, width float64) string {
	text = r.translate(text)
	limit := width - 1.5
	if r.pdf.GetStringWidth(text) <= limit {
		return text
	}
	for len(text) > 0 && r.pdf.GetStringWidth(text+"...") > limit {
		text = text[:len(text)-1]
	}
	return text + "..."
}

func (r *pdfRenderer) row(cells []string) error {
	for i, c := range cells {
		r.pdf.CellFormat(r.widths[i], pdfLine, r.fit(c, r.widths[i]), "1", 0, "L", false, 0, "")
	}
	r.pdf.Ln(-1)
	return r.pdf.Error()
}

func (r *pdfRenderer) finish(truncated bool, limit int64) ([]byte, error) {
	if truncated {
		r.pdf.Ln(2)
		r.pdf.SetFont("Helvetica", "B", 8)
		r.pdf.CellFormat(0, 6, fmt.Sprintf("Truncated at %d rows. Use Excel or CSV, or narrow the filters, to see the rest.", limit), "", 1, "L", false, 0, "")
	}
	var buf bytes.Buffer
	if err := r.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
