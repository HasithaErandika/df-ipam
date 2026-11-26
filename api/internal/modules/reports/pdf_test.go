package reports

import (
	"strings"
	"testing"
	"time"
)

// A character is 1 unit wide here, so the expected lines are easy to read.
func chars(s string) float64 { return float64(len(s)) }

func TestWrapKeepsEverything(t *testing.T) {
	for _, tc := range []struct {
		text string
		w    float64
		want []string
	}{
		{"short", 10, []string{"short"}},
		{"Floor 3 Wi-Fi guest network", 12, []string{"Floor 3", "Wi-Fi guest", "network"}},
		{"fe80-0000-0000-abcd-ef01", 10, []string{"fe80-0000-", "0000-abcd-", "ef01"}},
		{"", 10, []string{""}},
	} {
		got := wrap(chars, tc.text, tc.w)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("wrap(%q, %v) = %q, want %q", tc.text, tc.w, got, tc.want)
		}
		if strings.ReplaceAll(strings.Join(got, ""), " ", "") != strings.ReplaceAll(tc.text, " ", "") {
			t.Errorf("wrap(%q) lost text: %q", tc.text, got)
		}
	}
}

// Narrow columns keep what they need; wide ones share the rest; the page is filled.
func TestColumnWidthsFillThePage(t *testing.T) {
	got := columnWidths([]float64{20, 300, 300, 18}, []float64{20, 30, 30, 18}, []float64{12, 12, 12, 12})
	if got[0] != 20 || got[3] != 18 {
		t.Errorf("narrow columns changed: %v", got)
	}
	sum := 0.0
	for _, w := range got {
		sum += w
	}
	if sum < pdfUsable-0.01 || sum > pdfUsable+0.01 {
		t.Errorf("widths add to %v, want %v", sum, pdfUsable)
	}
}

// A long value comes out whole across lines and pages, never cut with "...".
func TestPDFWithLongValues(t *testing.T) {
	r := Report{Title: "IP register", Description: "Every registered IP.", Columns: []string{"IP", "Name", "Owner", "Notes"}}
	p := newPDF(r, "Data as of 26 Nov 2026 08:00", time.Now())
	long := strings.Repeat("Very long assigned name for a core database cluster node ", 4)
	for i := 0; i < 300; i++ {
		_ = p.row([]string{"10.20.1.10", long, "Network team – Colombo", "ok"})
	}
	out, err := p.finish(false, maxRowsPDF)
	if err != nil || len(out) < 1000 || p.pdf.PageNo() < 2 {
		t.Fatalf("pdf: %d bytes, %d pages, %v", len(out), p.pdf.PageNo(), err)
	}
}
