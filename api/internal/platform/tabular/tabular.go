// Package tabular reads and writes the table files people import and export: CSV,
// tab-separated text (.txt) and Excel (.xlsx). Every export and import in DF-Atlas
// goes through here, so the formats behave the same way.
package tabular

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Formats an export can be written in, with their content types.
var Formats = map[string]string{
	"csv":  "text/csv; charset=utf-8",
	"txt":  "text/plain; charset=utf-8",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// Writer collects rows and renders the file at the end.
type Writer interface {
	Write(row []string) error
	Bytes() ([]byte, error)
}

// NewWriter starts a file with a header row. format is csv, txt or xlsx.
func NewWriter(format, sheet string, header []string) (Writer, error) {
	switch format {
	case "csv":
		w := &csvWriter{}
		w.buf.WriteString("\xef\xbb\xbf") // Excel reads UTF-8 CSV correctly with a byte order mark
		w.w = csv.NewWriter(&w.buf)
		return w, w.w.Write(header)
	case "txt":
		w := &textWriter{}
		return w, w.Write(header)
	case "xlsx":
		return newSheet(sheet, header)
	}
	return nil, errors.New("unknown format")
}

// Safe stops a spreadsheet running a value as a formula. Hostnames and names can
// come from the network, so anyone on it could plant one.
func Safe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

type csvWriter struct {
	buf bytes.Buffer
	w   *csv.Writer
}

func (c *csvWriter) Write(row []string) error {
	out := make([]string, len(row))
	for i, v := range row {
		out[i] = Safe(v)
	}
	return c.w.Write(out)
}

func (c *csvWriter) Bytes() ([]byte, error) {
	c.w.Flush()
	return c.buf.Bytes(), c.w.Error()
}

// textWriter writes one line per row, values separated by tabs: it reads in any
// text editor, pastes into a spreadsheet as columns, and imports back as is.
// A tab or line break inside a value becomes a space, so rows stay on one line.
type textWriter struct{ buf bytes.Buffer }

var flatten = strings.NewReplacer("\t", " ", "\r\n", " ", "\n", " ", "\r", " ")

func (t *textWriter) Write(row []string) error {
	for i, v := range row {
		if i > 0 {
			t.buf.WriteByte('\t')
		}
		t.buf.WriteString(Safe(flatten.Replace(v)))
	}
	t.buf.WriteString("\r\n") // Windows line endings, so Notepad shows the lines
	return nil
}

func (t *textWriter) Bytes() ([]byte, error) { return t.buf.Bytes(), nil }

// Read returns every row of an uploaded CSV or .xlsx file (the first sheet).
// The name decides the format; content that doesn't match is refused.
func Read(name string, r io.Reader, limit int64) ([][]string, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("the file is larger than %d MB", limit>>20)
	}
	switch strings.ToLower(name[strings.LastIndex(name, ".")+1:]) {
	case "csv", "txt", "tsv":
		data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
		cr := csv.NewReader(bytes.NewReader(data))
		cr.Comma = delimiter(data)
		cr.FieldsPerRecord = -1
		cr.LazyQuotes = true
		return cr.ReadAll()
	case "xlsx":
		return readXLSX(data)
	}
	return nil, errors.New("upload a .csv, .txt or .xlsx file")
}

// delimiter picks the separator a text file uses from its header line: a tab,
// semicolon or pipe when one is clearly there, otherwise a comma.
func delimiter(data []byte) rune {
	header, _, _ := bytes.Cut(data, []byte("\n"))
	best, count := ',', bytes.Count(header, []byte(","))
	for _, r := range []rune{'\t', ';', '|'} {
		if n := bytes.Count(header, []byte(string(r))); n > count {
			best, count = r, n
		}
	}
	return best
}
