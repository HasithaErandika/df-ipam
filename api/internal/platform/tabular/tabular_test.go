package tabular

import (
	"bytes"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	for _, format := range []string{"csv", "txt", "xlsx"} {
		w, err := NewWriter(format, "Devices", []string{"mac", "name"})
		if err != nil {
			t.Fatal(err)
		}
		_ = w.Write([]string{"00:11:22:33:44:55", "=cmd()"})
		_ = w.Write([]string{"00:11:22:33:44:66", "10.0.0.1"})
		_ = w.Write([]string{"00:11:22:33:44:77", "two\tcolumns\nand lines"})
		data, err := w.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		rows, err := Read("devices."+format, bytes.NewReader(data), 1<<20)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if len(rows) != 4 || rows[0][0] != "mac" || rows[2][1] != "10.0.0.1" || len(rows[3]) != 2 {
			t.Fatalf("%s: got %v", format, rows)
		}
		if format == "txt" && rows[3][1] != "two columns and lines" {
			t.Fatalf("a tab or line break in a value split the row: %q", rows[3])
		}
		if format != "xlsx" && rows[1][1] != "'=cmd()" {
			t.Fatalf("formula was not neutralised: %q", rows[1][1])
		}
	}
}

func TestRefusesOtherFiles(t *testing.T) {
	if _, err := Read("evil.exe", bytes.NewReader([]byte("x")), 10); err == nil {
		t.Fatal("accepted an .exe")
	}
	if _, err := Read("big.csv", bytes.NewReader(make([]byte, 11)), 10); err == nil {
		t.Fatal("accepted a file over the limit")
	}
	if _, err := Read("fake.xlsx", bytes.NewReader([]byte("not a zip")), 100); err == nil {
		t.Fatal("accepted a broken workbook")
	}
}

func TestReadTextPicksTheDelimiter(t *testing.T) {
	for name, text := range map[string]string{
		"tabs.txt":  "ip\tname\n10.0.0.5\tweb 1\n",
		"semi.csv":  "ip;name\n10.0.0.5;web 1\n",
		"pipes.tsv": "ip|name\n10.0.0.5|web 1\n",
		"comma.txt": "ip,name\n10.0.0.5,web 1\n",
	} {
		rows, err := Read(name, strings.NewReader(text), 1<<20)
		if err != nil || len(rows) != 2 || len(rows[1]) != 2 || rows[1][1] != "web 1" {
			t.Errorf("%s: %v %q", name, err, rows)
		}
	}
}
