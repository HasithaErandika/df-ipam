package tabular

import (
	"bytes"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// sizeRows is how many rows set the column widths. The stream writer needs
// widths before the first row, so these rows wait in memory; the rest stream.
const sizeRows = 500

// sheetWriter streams one Excel sheet: a frozen, coloured header and each
// column as wide as its content (from the header and the first rows), within limits.
type sheetWriter struct {
	f       *excelize.File
	sw      *excelize.StreamWriter
	header  []string
	pending [][]string
	row     int
	started bool
}

func newSheet(name string, header []string) (*sheetWriter, error) {
	f := excelize.NewFile()
	if name == "" {
		name = "Data"
	}
	if len(name) > 31 {
		name = name[:31]
	}
	if err := f.SetSheetName("Sheet1", name); err != nil {
		return nil, err
	}
	sw, err := f.NewStreamWriter(name)
	if err != nil {
		return nil, err
	}
	return &sheetWriter{f: f, sw: sw, header: header}, nil
}

func (s *sheetWriter) Write(row []string) error {
	if !s.started {
		s.pending = append(s.pending, row)
		if len(s.pending) < sizeRows {
			return nil
		}
		return s.start()
	}
	return s.put(row)
}

// start sets the widths from what is known so far, writes the header and the
// rows held back, and streams from then on.
func (s *sheetWriter) start() error {
	s.started = true
	for i, w := range widths(s.header, s.pending) {
		if err := s.sw.SetColWidth(i+1, i+1, w); err != nil {
			return err
		}
	}
	_ = s.sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	head, _ := s.f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F4FAE"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	cells := make([]any, len(s.header))
	for i, h := range s.header {
		cells[i] = excelize.Cell{Value: h, StyleID: head}
	}
	if err := s.sw.SetRow("A1", cells, excelize.RowOpts{Height: 30}); err != nil {
		return err
	}
	s.row = 1
	for _, r := range s.pending {
		if err := s.put(r); err != nil {
			return err
		}
	}
	s.pending = nil
	return nil
}

func (s *sheetWriter) put(row []string) error {
	s.row++
	cells := make([]any, len(row))
	for i, v := range row {
		cells[i] = v // written as text: an IP or MAC must never become a number or date
	}
	cell, _ := excelize.CoordinatesToCellName(1, s.row)
	return s.sw.SetRow(cell, cells)
}

func (s *sheetWriter) Bytes() ([]byte, error) {
	if !s.started {
		if err := s.start(); err != nil {
			return nil, err
		}
	}
	if err := s.sw.Flush(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := s.f.Write(&buf); err != nil {
		return nil, err
	}
	_ = s.f.Close()
	return buf.Bytes(), nil
}

// widths is each column's width in characters: its longest value or header,
// at least 8 and at most 60 (longer text stays in the cell, shown on select).
func widths(header []string, rows [][]string) []float64 {
	out := make([]float64, len(header))
	for i, h := range header {
		n := utf8.RuneCountInString(h)
		for _, r := range rows {
			if i < len(r) {
				n = max(n, utf8.RuneCountInString(r[i]))
			}
		}
		out[i] = float64(min(max(n+2, 8), 60))
	}
	return out
}
