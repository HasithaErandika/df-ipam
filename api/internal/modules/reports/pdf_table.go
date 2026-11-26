package reports

// measure is how wide each column would like to be (its header, or most of
// its values: the 95th percentile, so one very long value doesn't take the
// page) and the least it should get: enough for its usual longest word, so an
// IP, MAC or host name stays on one line.
func (p *pdfRenderer) measure() (want, least, floor []float64) {
	pdf := p.pdf
	n := len(p.report.Columns)
	want, least, floor = make([]float64, n), make([]float64, n), make([]float64, n)
	for i, h := range p.report.Columns {
		pdf.SetFont("Helvetica", "B", p.size)
		head := longestWord(pdf.GetStringWidth, p.translate(h))
		pdf.SetFont("Helvetica", "", p.size)
		full, word := make([]float64, 0, len(p.rows)), make([]float64, 0, len(p.rows))
		for _, r := range p.rows {
			full = append(full, pdf.GetStringWidth(r[i]))
			word = append(word, longestWord(pdf.GetStringWidth, r[i]))
		}
		pad := 2*pdfPadX + 0.5
		want[i] = min(max(head, p95(full))+pad, pdfMaxCol)
		floor[i] = max(head+pad, 10)
		least[i] = max(min(max(p95(word)+pad, pdfMinCol), pdfMaxWord, want[i]), floor[i])
		want[i] = max(want[i], least[i])
	}
	return want, least, floor
}

// columnWidths fills the page width. Every column gets its least; the room
// left goes to the columns that want more, the smallest needs met first, and
// any room still left is shared in proportion. If even the least doesn't fit,
// columns give up only what they have beyond their floor (the header's words).
func columnWidths(want, least, floor []float64) []float64 {
	out := append([]float64(nil), least...)
	left := pdfUsable
	for _, w := range least {
		left -= w
	}
	if left < 0 {
		return squeezed(out, floor)
	}
	open := map[int]bool{}
	for i := range want {
		if want[i] > out[i] {
			open[i] = true
		}
	}
	for len(open) > 0 && left > 0.01 {
		var done bool
		if left, done = share(out, want, open, left); done {
			return out
		}
	}
	return scaled(out, pdfUsable)
}

// share gives each open column an even share of left, or all it still needs
// if that is less. done means the shares ran out before every need was met.
func share(out, want []float64, open map[int]bool, left float64) (float64, bool) {
	each, met := left/float64(len(open)), false
	for i := range open {
		if need := want[i] - out[i]; need <= each {
			out[i], left, met = want[i], left-need, true
			delete(open, i)
		}
	}
	if met {
		return left, false
	}
	for i := range open {
		out[i] += each
	}
	return 0, true
}

// squeezed fits widths to the page by capping the widest columns first: a
// cap is found so that every column gets min(its width, the cap), but never
// less than its floor (the header's words). Short values such as an IP, MAC
// or serial then stay whole, and long names wrap. Floors shrink too only if
// they alone overflow.
func squeezed(w, floor []float64) []float64 {
	base := 0.0
	for _, f := range floor {
		base += f
	}
	if base > pdfUsable {
		return scaled(append([]float64(nil), floor...), pdfUsable)
	}
	capped := func(c float64) float64 {
		t := 0.0
		for i := range w {
			t += max(min(w[i], c), floor[i])
		}
		return t
	}
	lo, hi := 0.0, pdfUsable
	for range 40 {
		if mid := (lo + hi) / 2; capped(mid) > pdfUsable {
			hi = mid
		} else {
			lo = mid
		}
	}
	out := make([]float64, len(w))
	for i := range w {
		out[i] = max(min(w[i], lo), floor[i])
	}
	return scaled(out, pdfUsable)
}

// scaled stretches or shrinks widths in proportion so they add up to total.
func scaled(w []float64, total float64) []float64 {
	sum := 0.0
	for _, x := range w {
		sum += x
	}
	for i := range w {
		w[i] = w[i] * total / sum
	}
	return w
}

// tableHeader draws the column names on the brand colour, wrapped if needed.
func (p *pdfRenderer) tableHeader() {
	pdf := p.pdf
	pdf.SetFont("Helvetica", "B", p.size)
	cells := make([][]string, len(p.report.Columns))
	for i, h := range p.report.Columns {
		cells[i] = wrap(pdf.GetStringWidth, p.translate(h), p.widths[i]-2*pdfPadX)
	}
	pdf.SetFillColor(pdfBrandR, pdfBrandG, pdfBrandB)
	pdf.SetTextColor(255, 255, 255)
	p.drawCells(cells, true)
	pdf.SetFont("Helvetica", "", p.size)
}

// drawRow draws one record, every cell wrapped to its column, starting a new
// page first when the row wouldn't fit.
func (p *pdfRenderer) drawRow(r []string, stripe bool) {
	pdf := p.pdf
	pdf.SetFont("Helvetica", "", p.size)
	cells := make([][]string, len(r))
	for i, v := range r {
		cells[i] = wrap(pdf.GetStringWidth, v, p.widths[i]-2*pdfPadX)
	}
	if _, pageH := pdf.GetPageSize(); pdf.GetY()+p.rowHeight(cells) > pageH-pdfBottom {
		pdf.AddPage()
	}
	pdf.SetFillColor(pdfStripeR, pdfStripeG, pdfStripeB)
	pdf.SetTextColor(19, 26, 38)
	p.drawCells(cells, stripe)
}

func (p *pdfRenderer) rowHeight(cells [][]string) float64 {
	lines := 1
	for _, c := range cells {
		lines = max(lines, len(c))
	}
	return float64(lines)*p.lineH() + 2*pdfPadY
}

// drawCells draws one row of wrapped cells with a hairline under it.
func (p *pdfRenderer) drawCells(cells [][]string, fill bool) {
	pdf := p.pdf
	x, y, h := pdfMargin, pdf.GetY(), p.rowHeight(cells)
	if fill {
		pdf.Rect(x, y, pdfUsable, h, "F")
	}
	for i, lines := range cells {
		for j, line := range lines {
			pdf.SetXY(x+pdfPadX, y+pdfPadY+float64(j)*p.lineH())
			pdf.CellFormat(p.widths[i]-2*pdfPadX, p.lineH(), line, "", 0, "L", false, 0, "")
		}
		x += p.widths[i]
	}
	pdf.SetDrawColor(226, 230, 236)
	pdf.Line(pdfMargin, y+h, pdfMargin+pdfUsable, y+h)
	pdf.SetXY(pdfMargin, y+h)
}
