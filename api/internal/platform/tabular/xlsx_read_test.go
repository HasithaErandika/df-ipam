package tabular

import (
	"bytes"
	"errors"
	"testing"

	"github.com/xuri/excelize/v2"
)

// A cell far to the right makes Excel pad the row to 16,384 columns. Many
// such rows in a small file used to hold gigabytes; now the row is refused.
func TestWideRowIsRefused(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetCellValue("Sheet1", "A1", "ip"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "XFD2", "x"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if _, err := readXLSX(buf.Bytes()); !errors.Is(err, errTooManyCols) {
		t.Fatalf("want errTooManyCols, got %v", err)
	}
}

func TestTrimColumnsDropsEmptyPadding(t *testing.T) {
	cols := make([]string, MaxCols+10)
	cols[0] = "a"
	got, err := trimColumns(cols)
	if err != nil || len(got) != MaxCols || got[0] != "a" {
		t.Fatalf("got %d cols, err %v", len(got), err)
	}
}
