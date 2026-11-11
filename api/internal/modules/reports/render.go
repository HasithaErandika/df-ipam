package reports

import (
	"bytes"
	"fmt"
	"time"

	"dfipam/api/internal/platform/tabular"

	"github.com/go-pdf/fpdf"
)

// Row limits per format. A report that reaches one says so, so nobody mistakes a
// cut-off file for the whole register.
const (
	maxRowsTable = 200_000
	maxRowsPDF   = 5_000
)

type renderer interface {
	row(cells []string) error
	finish(truncated bool, limit int) ([]byte, error)
}

// tableRenderer writes CSV or Excel through the shared tabular package.
type tableRenderer struct{ w tabular.Writer }

func newTable(format string, r Report) (*tableRenderer, error) {
	w, err := tabular.NewWriter(format, r.Title, r.Columns)
	return &tableRenderer{w: w}, err
}

func (t *tableRenderer) row(cells []string) error { return t.w.Write(cells) }

func (t *tableRenderer) finish(truncated bool, limit int) ([]byte, error) {
	if truncated {
		_ = t.w.Write([]string{fmt.Sprintf("Truncated at %d rows. Narrow the date range to see the rest.", limit)})
	}
	return t.w.Bytes()
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

func newPDF(r Report, subtitle string, asOf time.Time) *pdfRenderer {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 12)
	pdf.SetCreator("DF-IPAM", false)
	pdf.SetTitle(r.Title, false)
	translate := pdf.UnicodeTranslatorFromDescriptor("")
	out := &pdfRenderer{pdf: pdf, translate: translate, columns: r.Columns}
	for range r.Columns {
		out.widths = append(out.widths, pdfUsable/float64(len(r.Columns)))
	}
	first := true
	pdf.SetHeaderFunc(func() {
		if first {
			pdf.SetFont("Helvetica", "B", 13)
			pdf.SetTextColor(23, 48, 95)
			pdf.CellFormat(0, 7, translate("DF-IPAM · "+r.Title), "", 1, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 8)
			pdf.SetTextColor(90, 100, 115)
			pdf.CellFormat(0, 5, translate(subtitle), "", 1, "L", false, 0, "")
			pdf.Ln(2)
			first = false
		}
		pdf.SetTextColor(20, 20, 20)
		pdf.SetFont("Helvetica", "B", 7.5)
		pdf.SetFillColor(232, 238, 248)
		for i, c := range out.columns {
			pdf.CellFormat(out.widths[i], pdfLine+1, out.fit(c, out.widths[i]), "1", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 7)
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("Helvetica", "", 7)
		pdf.CellFormat(0, 4, translate(fmt.Sprintf("DF-IPAM | %s | Generated %s | Confidential | Page %d",
			r.Title, asOf.Format("2006-01-02 15:04 MST"), pdf.PageNo())), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	return out
}

// fit shortens text so a cell never wraps into the next column.
func (p *pdfRenderer) fit(text string, width float64) string {
	text = p.translate(text)
	limit := width - 1.5
	if p.pdf.GetStringWidth(text) <= limit {
		return text
	}
	for len(text) > 0 && p.pdf.GetStringWidth(text+"...") > limit {
		text = text[:len(text)-1]
	}
	return text + "..."
}

func (p *pdfRenderer) row(cells []string) error {
	for i, c := range cells {
		p.pdf.CellFormat(p.widths[i], pdfLine, p.fit(c, p.widths[i]), "1", 0, "L", false, 0, "")
	}
	p.pdf.Ln(-1)
	return p.pdf.Error()
}

func (p *pdfRenderer) finish(truncated bool, limit int) ([]byte, error) {
	if truncated {
		p.pdf.Ln(2)
		p.pdf.SetFont("Helvetica", "B", 8)
		p.pdf.CellFormat(0, 6, fmt.Sprintf("Truncated at %d rows. Download CSV, or narrow the date range, to see the rest.", limit), "", 1, "L", false, 0, "")
	}
	var buf bytes.Buffer
	if err := p.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
