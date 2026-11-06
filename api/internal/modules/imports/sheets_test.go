package imports

import (
	"strings"
	"testing"

	"dfipam/api/internal/modules/catalog"
)

// A freshly downloaded template must read back cleanly, with its examples skipped.
func TestTemplatesReadBack(t *testing.T) {
	data, err := templateWorkbook()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readWorkbook(data)
	if err != nil || len(rows) != 0 {
		t.Fatalf("workbook: %d rows, %v", len(rows), err)
	}
	for i := range sheets {
		text, err := templateCSV(&sheets[i])
		if err != nil {
			t.Fatal(err)
		}
		rows, err := readCSV(string(text))
		if err != nil || len(rows) != 0 {
			t.Fatalf("%s: %d rows, %v", sheets[i].Key, len(rows), err)
		}
	}
}

// Excel adds a byte order mark, pads rows and may use semicolons. All must still work.
func TestCSVSavedByExcel(t *testing.T) {
	text := "\uFEFF\"# dfipam-template: teams\";;\r\nname *;email;description\r\n\r\nNetOps;;\r\n;;\r\n"
	rows, err := readCSV(text)
	if err != nil || len(rows) != 1 || rows[0].fields["name"] != "NetOps" || rows[0].number != 4 {
		t.Fatalf("rows %+v, %v", rows, err)
	}
}

func TestUnknownColumnIsRefused(t *testing.T) {
	_, err := readCSV(csvMarker + " teams\nname,emial\nNetOps,a@b.c\n")
	if err == nil || !strings.Contains(err.Error(), "emial") {
		t.Fatalf("got %v", err)
	}
}

// Sheets must keep to what the catalog accepts and the index can find.
func TestSheetsMatchWhatTheyWriteTo(t *testing.T) {
	for i := range sheets {
		s := &sheets[i]
		if aliasQueries[s.Key] == "" || s.column(s.KeyCol) == nil {
			t.Errorf("%s: no name query or key column", s.Key)
		}
		if s.Parent != "" && s.column(s.Parent).Ref == "" {
			t.Errorf("%s: parent %s isn't a reference", s.Key, s.Parent)
		}
		e, inCatalog := catalog.ByPath(s.Entity)
		if s.Entity != "" && !inCatalog {
			t.Errorf("%s: no catalog entity %s", s.Key, s.Entity)
		}
		for _, c := range s.Columns {
			if c.Ref != "" && aliasQueries[c.Ref] == "" {
				t.Errorf("%s.%s: unknown reference %s", s.Key, c.Name, c.Ref)
			}
			if inCatalog && c.Field != "" && !hasField(e, c.Field) {
				t.Errorf("%s.%s: catalog has no field %s", s.Key, c.Name, c.Field)
			}
		}
	}
}

func hasField(e catalog.Entity, name string) bool {
	for _, f := range e.Fields {
		if f.JSON == name {
			return true
		}
	}
	return false
}
