package reports

import (
	"fmt"

	"dfipam/api/internal/platform/tabular"
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
