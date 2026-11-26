package reports

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
)

// The PDF layout, in millimetres on A4 landscape.
const (
	pdfMargin  = 10.0
	pdfUsable  = 277.0 // page width less margins
	pdfPadX    = 1.8
	pdfPadY    = 1.5
	pdfMinCol  = 16.0
	pdfMaxCol  = 110.0
	pdfMaxWord = 45.0 // a longer word may wrap
	pdfBottom  = 14.0 // room kept for the footer
	pdfBrandR  = 31   // the app's brand blue, #1f4fae
	pdfBrandG  = 79
	pdfBrandB  = 174
	pdfStripeR = 246 // every other row, #f6f8fb
	pdfStripeG = 248
	pdfStripeB = 251
)

// pdfRenderer collects the rows (a PDF holds at most maxRowsPDF), then lays
// them out: each column as wide as its content needs, long text wrapped onto
// more lines rather than cut, and the header repeated on every page.
type pdfRenderer struct {
	pdf       *fpdf.Fpdf
	translate func(string) string
	report    Report
	subtitle  string
	asOf      time.Time
	rows      [][]string
	widths    []float64
	size      float64 // font size in points: smaller for reports with many columns
}

// fontSizes are tried in turn until every column fits at its least width.
var fontSizes = []float64{7.5, 7, 6.5, 6}

// layout picks the largest font size at which the columns fit, and their widths.
func (p *pdfRenderer) layout() []float64 {
	var want, least, floor []float64
	for _, p.size = range fontSizes {
		want, least, floor = p.measure()
		total := 0.0
		for _, w := range least {
			total += w
		}
		if total <= pdfUsable {
			break
		}
	}
	return columnWidths(want, least, floor)
}

// lineH is one line of text at the chosen size.
func (p *pdfRenderer) lineH() float64 { return p.size * 0.48 }

func newPDF(r Report, subtitle string, asOf time.Time) *pdfRenderer {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.SetAutoPageBreak(false, pdfBottom)
	pdf.SetCreator("DF-Atlas", false)
	pdf.SetTitle(r.Title, false)
	pdf.AliasNbPages("")
	return &pdfRenderer{pdf: pdf, translate: pdf.UnicodeTranslatorFromDescriptor(""), report: r, subtitle: subtitle, asOf: asOf}
}

func (p *pdfRenderer) row(cells []string) error {
	out := make([]string, len(p.report.Columns))
	for i := range out {
		if i < len(cells) {
			out[i] = p.translate(cells[i])
		}
	}
	p.rows = append(p.rows, out)
	return nil
}

func (p *pdfRenderer) finish(truncated bool, limit int) ([]byte, error) {
	pdf := p.pdf
	p.widths = p.layout()
	pdf.SetHeaderFunc(p.header)
	pdf.SetFooterFunc(p.footer)
	pdf.AddPage()
	p.title(truncated, limit)
	p.tableHeader()
	if len(p.rows) == 0 {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.SetTextColor(111, 122, 140)
		pdf.CellFormat(pdfUsable, 12, "Nothing to report for this range.", "", 1, "C", false, 0, "")
	}
	for i, r := range p.rows {
		p.drawRow(r, i%2 == 1)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// title opens the first page: the report's name, what it covers, and how many rows.
func (p *pdfRenderer) title(truncated bool, limit int) {
	pdf := p.pdf
	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(19, 26, 38)
	pdf.CellFormat(pdfUsable, 8, p.translate(p.report.Title), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(77, 88, 106)
	if p.report.Description != "" {
		pdf.CellFormat(pdfUsable, 5, p.translate(p.report.Description), "", 1, "L", false, 0, "")
	}
	count := fmt.Sprintf("%d rows", len(p.rows))
	if len(p.rows) == 1 {
		count = "1 row"
	}
	pdf.CellFormat(pdfUsable, 5, p.translate(p.subtitle+" · "+count), "", 1, "L", false, 0, "")
	if truncated {
		pdf.SetTextColor(161, 92, 0)
		pdf.CellFormat(pdfUsable, 5, fmt.Sprintf("Only the first %d rows fit in a PDF. Download CSV or Excel, or narrow the range, for the rest.", limit), "", 1, "L", false, 0, "")
	}
	pdf.Ln(3)
}

// header is the band at the top of every page: the product, and the report.
func (p *pdfRenderer) header() {
	pdf := p.pdf
	pdf.SetFillColor(pdfBrandR, pdfBrandG, pdfBrandB)
	pdf.Rect(0, 0, 297, 3, "F")
	pdf.SetXY(pdfMargin, 6)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(pdfBrandR, pdfBrandG, pdfBrandB)
	pdf.CellFormat(30, 5, "DF-Atlas", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(111, 122, 140)
	pdf.CellFormat(pdfUsable-30, 5, p.translate("Dialog Finance · Network and IPAM · "+p.report.Title), "", 1, "R", false, 0, "")
	pdf.SetDrawColor(226, 230, 236)
	pdf.Line(pdfMargin, 12, pdfMargin+pdfUsable, 12)
	pdf.SetY(15)
	if pdf.PageNo() > 1 {
		p.tableHeader()
	}
}

func (p *pdfRenderer) footer() {
	pdf := p.pdf
	pdf.SetY(-10)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(111, 122, 140)
	pdf.CellFormat(pdfUsable/2, 4, p.translate("Confidential · Generated "+p.asOf.Format("2 Jan 2006, 15:04")+" · DF-Atlas"), "", 0, "L", false, 0, "")
	pdf.CellFormat(pdfUsable/2, 4, fmt.Sprintf("Page %d of {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
}
