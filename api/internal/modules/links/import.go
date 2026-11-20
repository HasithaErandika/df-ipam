package links

import (
	"errors"
	"net/http"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	maxImportBytes = 5 << 20
	maxImportRows  = 2000
)

type rowIssue struct {
	Row   int    `json:"row"`
	Issue string `json:"issue"`
}

type previewRow struct {
	Row    int    `json:"row"`
	Key    string `json:"key"`
	Label  string `json:"label"`
	Action string `json:"action"`
}

// importResult says what an import did, or would do on a dry run.
type importResult struct {
	DryRun   bool         `json:"dryRun"`
	Imported int          `json:"imported"`
	Added    int          `json:"added"`
	Updated  int          `json:"updated"`
	Skipped  int          `json:"skipped"`
	Issues   []rowIssue   `json:"issues"`
	Preview  []previewRow `json:"preview"`
}

var errDryRun = errors.New("dry run")

// importLinks adds or updates links from a spreadsheet. A row with a known
// Link ID, or with a provider and circuit ID already on a link, updates that
// link only when ?update=true confirms it. Columns missing from the file keep
// their values. ?dryRun=true does everything, reports it, and keeps nothing.
func (h *Handler) importLinks(c *gin.Context) {
	records, ok := readSheet(c)
	if !ok {
		return
	}
	s, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	x, err := newResolver(c, h.conn, s)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	run := &importRun{h: h, c: c, s: s, x: x, header: records[0], update: c.Query("update") == "true",
		res: importResult{DryRun: c.Query("dryRun") == "true", Issues: []rowIssue{}, Preview: []previewRow{}}}
	err = db.InTx(c, h.conn, func(tx pgx.Tx) error {
		for i, rec := range records[1:] {
			run.row(tx, i+2, rec)
		}
		if run.res.DryRun {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) {
		httpx.Fail(c, err)
		return
	}
	run.res.Imported = run.res.Added + run.res.Updated
	httpx.OK(c, http.StatusOK, run.res)
}

type importRun struct {
	h      *Handler
	c      *gin.Context
	s      Settings
	x      *resolver
	header []string
	update bool
	res    importResult
}

func (r *importRun) skip(line int, issue string) {
	r.res.Skipped++
	r.res.Issues = append(r.res.Issues, rowIssue{Row: line, Issue: issue})
}

// row imports one line in its own savepoint, so a bad row leaves the others.
func (r *importRun) row(tx pgx.Tx, line int, rec []string) {
	cells := map[string]string{}
	blank := true
	for i, v := range rec {
		if i < len(r.header) {
			cells[r.header[i]] = strings.TrimSpace(v)
			blank = blank && cells[r.header[i]] == ""
		}
	}
	if blank {
		return
	}
	id, action, err := r.target(tx, cells)
	if err != nil {
		r.skip(line, err.Error())
		return
	}
	in, err := r.input(tx, id, cells)
	if err != nil {
		r.skip(line, err.Error())
		return
	}
	sp, err := tx.Begin(r.c)
	if err != nil {
		r.skip(line, err.Error())
		return
	}
	t, _ := r.s.typeOf(in.Type)
	if err = in.validate(t.VPN); err == nil {
		id, err = writeLink(r.c, sp, in, id, r.s, can(r.c, auth.LinksCommercialManage))
	}
	if err != nil {
		_ = sp.Rollback(r.c)
		r.skip(line, describe(err))
		return
	}
	if err := sp.Commit(r.c); err != nil {
		r.skip(line, describe(err))
		return
	}
	r.count(tx, line, id, in.Name, action)
}

func (r *importRun) count(tx pgx.Tx, line int, id, name, action string) {
	// A dry run's new link has no lasting ID: the real import gives the next one.
	code := "New"
	if action == "update" || !r.res.DryRun {
		_ = tx.QueryRow(r.c, `SELECT code FROM network_links WHERE id = $1::uuid`, id).Scan(&code)
	}
	if action == "add" {
		r.res.Added++
	} else {
		r.res.Updated++
	}
	r.res.Preview = append(r.res.Preview, previewRow{Row: line, Key: code, Label: name, Action: action})
}

// describe turns a save failure into one line for the import report.
func describe(err error) string {
	var re rowError
	if errors.As(err, &re) {
		return string(re)
	}
	var he *httpx.Error
	if !errors.As(db.Friendly(err), &he) {
		return "This row couldn't be saved."
	}
	if len(he.Details) == 0 {
		return he.Message
	}
	parts := make([]string, len(he.Details))
	for i, d := range he.Details {
		parts[i] = d.Issue
	}
	return strings.Join(parts, " ")
}
