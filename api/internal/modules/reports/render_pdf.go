package reports

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
)

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
