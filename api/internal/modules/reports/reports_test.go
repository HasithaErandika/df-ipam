package reports

import (
	"bytes"
	"encoding/csv"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"dfipam/api/internal/auth"

	"github.com/xuri/excelize/v2"
)

func TestSafeCellStopsFormulas(t *testing.T) {
	for in, want := range map[string]string{
		`=HYPERLINK("http://evil")`: `'=HYPERLINK("http://evil")`,
		"+1+1":                      "'+1+1",
		"-2+3":                      "'-2+3",
		"@SUM(A1)":                  "'@SUM(A1)",
		"\tcmd":                     "'\tcmd",
		"host-1.example.com":        "host-1.example.com",
		"10.0.0.1":                  "10.0.0.1",
		"":                          "",
	} {
		if got := safeCell(in); got != want {
			t.Errorf("%q: got %q", in, got)
		}
	}
}

func fixture() Definition {
	return Definition{Title: "Test", Columns: []string{"Host", "Address"}}
}

func TestCSV(t *testing.T) {
	r, err := newRenderer("csv", fixture(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.row([]string{"=evil()", "10.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	out, err := r.finish(true, 1)
	if err != nil {
		t.Fatal(err)
	}
	rd := csv.NewReader(bytes.NewReader(out))
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	if err != nil || len(rows) != 3 || rows[1][0] != "'=evil()" || !strings.Contains(rows[2][0], "Truncated") {
		t.Fatalf("%v %v", rows, err)
	}
}

func TestExcel(t *testing.T) {
	r, err := newRenderer("xlsx", fixture(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.row([]string{"=evil()", "2001:db8::1"}); err != nil {
		t.Fatal(err)
	}
	out, err := r.finish(false, 1)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := f.GetRows(sheetName)
	if err != nil || len(rows) != 2 || rows[0][0] != "Host" || rows[1][0] != "'=evil()" || rows[1][1] != "2001:db8::1" {
		t.Fatalf("%v %v", rows, err)
	}
	if formula, _ := f.GetCellFormula(sheetName, "A2"); formula != "" {
		t.Fatalf("stored as a formula: %s", formula)
	}
}

func TestPDF(t *testing.T) {
	r, err := newRenderer("pdf", fixture(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for range 200 { // enough to need more than one page
		if err := r.row([]string{strings.Repeat("very long hostname ", 40), "10.0.0.1"}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := r.finish(true, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
}

// Every statement must use exactly the five shared parameters, or the worker's
// call fails at run time on that report only.
func TestDefinitionsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range Definitions {
		if seen[d.Key] {
			t.Errorf("%s repeats", d.Key)
		}
		seen[d.Key] = true
		for n := 1; n <= 5; n++ {
			if !strings.Contains(d.sql, "$"+strconv.Itoa(n)+"::") {
				t.Errorf("%s doesn't use $%d with a type", d.Key, n)
			}
		}
		if regexp.MustCompile(`\$[6-9]`).MatchString(d.sql) {
			t.Errorf("%s uses an unknown parameter", d.Key)
		}
		if d.Permission == "" || len(d.Columns) == 0 || d.Title == "" {
			t.Errorf("%s is incomplete", d.Key)
		}
		// A report must never expose stored secrets.
		for _, secret := range []string{"credential", "ciphertext", "token_hash", "secret", "key_version"} {
			if strings.Contains(d.sql, secret) {
				t.Errorf("%s reads %s", d.Key, secret)
			}
		}
		if d.Scoped && !strings.Contains(d.sql, "= ANY($5") {
			t.Errorf("%s says it is scoped but ignores the scope", d.Key)
		}
	}
}

func TestAllowed(t *testing.T) {
	scoped := &auth.Principal{Permissions: map[string]bool{auth.ReportsExportScoped: true}}
	full := &auth.Principal{Permissions: map[string]bool{auth.ReportsExport: true}}
	auditor := &auth.Principal{Permissions: map[string]bool{auth.ReportsExport: true, auth.AuditView: true}}
	register, _ := definition("subnet_register")
	coverage, _ := definition("discovery_coverage")
	trail, _ := definition("audit_trail")
	if f, ok := allowed(scoped, register); f || !ok {
		t.Error("a scoped person should get a scoped register")
	}
	if _, ok := allowed(scoped, coverage); ok {
		t.Error("a scoped person reached an unscoped report")
	}
	if f, ok := allowed(full, register); !f || !ok {
		t.Error("full export should be unscoped")
	}
	if _, ok := allowed(full, trail); ok {
		t.Error("export rights alone reached the audit trail")
	}
	if _, ok := allowed(auditor, trail); !ok {
		t.Error("an auditor should reach the audit trail")
	}
}

func TestFilters(t *testing.T) {
	trail, _ := definition("audit_trail")
	register, _ := definition("subnet_register")
	for _, c := range []struct {
		f  Filters
		d  Definition
		ok bool
	}{
		{Filters{From: "2026-01-01", To: "2026-02-01"}, trail, true},
		{Filters{From: "2026-02-01", To: "2026-01-01"}, trail, false},
		{Filters{From: "not a date"}, trail, false},
		{Filters{Environment: "production"}, trail, false},
		{Filters{Environment: "nonsense"}, register, false},
		{Filters{RoutingDomainID: "x"}, register, false},
		{Filters{Environment: "uat"}, register, true},
	} {
		if err := c.f.validate(c.d); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.f, err)
		}
	}
}
