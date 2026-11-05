package dbrules

import (
	"context"
	"testing"

	"dfipam/api/internal/modules/catalog"
)

// A text value can't go into a date, number or address column without a cast, so a
// field of the wrong kind fails on every save, with a 500 instead of a message.
func TestCatalogFieldsMatchTheirColumns(t *testing.T) {
	e := setup(t)
	allowed := map[catalog.Kind][]string{catalog.Text: {"text", "USER-DEFINED"}, catalog.UUID: {"uuid"}, catalog.Int: {"integer", "smallint", "bigint"},
		catalog.Bool: {"boolean"}, catalog.CIDR: {"cidr"}, catalog.Inet: {"inet"}, catalog.MAC: {"macaddr"}, catalog.Time: {"timestamp with time zone"}}
	for _, entity := range catalog.Entities {
		for _, f := range entity.Fields {
			var kind string
			if err := e.tx.QueryRow(context.Background(), `SELECT data_type FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`, entity.Table, f.Col).Scan(&kind); err != nil {
				t.Errorf("%s.%s: %v", entity.Table, f.Col, err)
				continue
			}
			ok := false
			for _, want := range allowed[f.Kind] {
				ok = ok || want == kind
			}
			if !ok {
				t.Errorf("%s.%s is %s, which field kind %d can't write", entity.Table, f.Col, kind, f.Kind)
			}
		}
	}
}
