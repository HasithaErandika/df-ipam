package links

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

const maxExport = 20000

// visibleColumns are the columns this person may export.
func visibleColumns(money bool) []ioColumn {
	out := make([]ioColumn, 0, len(ioColumns))
	for _, c := range ioColumns {
		if money || !c.Commercial {
			out = append(out, c)
		}
	}
	return out
}

// export downloads the links that match the list's filters: Excel with extra
// sheets for VPN subnets and related links, or CSV / text with one sheet.
func (h *Handler) export(c *gin.Context) {
	format := c.DefaultQuery("format", "xlsx")
	if _, ok := tabular.Formats[format]; !ok {
		httpx.Fail(c, httpx.Validation("format", "Choose Excel, CSV or text."))
		return
	}
	s, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	w, err := linkFilter(c, s)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := ioRows(c, h.conn, w, s)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	cols := visibleColumns(can(c, auth.LinksCommercialView))
	var data []byte
	if format == "xlsx" {
		data, err = workbook(cols, rows)
	} else {
		data, err = flatFile(format, cols, rows)
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	name := "network-links-" + time.Now().Format("2006-01-02") + "." + format
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(http.StatusOK, tabular.Formats[format], data)
}

// ioRows loads the matching links with their local subnets and related links,
// three queries in all.
func ioRows(ctx context.Context, conn db.Conn, w db.Where, s Settings) ([]*ioRow, error) {
	links, err := db.All[linkDTO](ctx, conn, linkColumns+linkFrom+w.SQL()+fmt.Sprintf(" ORDER BY l.code LIMIT %d", maxExport), w.Args...)
	if err != nil {
		return nil, err
	}
	rows, byID, ids := make([]*ioRow, len(links)), map[string]*ioRow{}, make([]string, len(links))
	for i := range links {
		t, _ := s.typeOf(links[i].Type)
		rows[i] = &ioRow{linkDTO: links[i], TypeLabel: firstOf(t.Label, links[i].Type)}
		byID[links[i].ID], ids[i] = rows[i], links[i].ID
	}
	pairs, err := conn.Query(ctx, `SELECT x.link_id::text, s.network::text FROM network_link_segments x JOIN segments s ON s.id = x.segment_id
		WHERE x.link_id = ANY($1::uuid[]) ORDER BY s.network`, ids)
	if err != nil {
		return nil, err
	}
	if err := eachPair(pairs, func(id, v string) { byID[id].LocalSubnets = append(byID[id].LocalSubnets, v) }); err != nil {
		return nil, err
	}
	pairs, err = conn.Query(ctx, `SELECT r.link_id::text, l.code || ':' || r.kind FROM network_link_relations r JOIN network_links l ON l.id = r.related_id
		WHERE r.link_id = ANY($1::uuid[]) ORDER BY l.code`, ids)
	if err != nil {
		return nil, err
	}
	return rows, eachPair(pairs, func(id, v string) { byID[id].Related = append(byID[id].Related, v) })
}

type scanner interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

func eachPair(rows scanner, fn func(id, v string)) error {
	defer rows.Close()
	for rows.Next() {
		var id, v string
		if err := rows.Scan(&id, &v); err != nil {
			return err
		}
		fn(id, v)
	}
	return rows.Err()
}

func headers(cols []ioColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

func flatFile(format string, cols []ioColumn, rows []*ioRow) ([]byte, error) {
	w, err := tabular.NewWriter(format, "Network links", headers(cols))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		line := make([]string, len(cols))
		for i, c := range cols {
			line[i] = tabular.Safe(c.get(r))
		}
		if err := w.Write(line); err != nil {
			return nil, err
		}
	}
	return w.Bytes()
}

// workbook writes the links, then one row per VPN subnet and per related link.
func workbook(cols []ioColumn, rows []*ioRow) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	main := [][]string{headers(cols)}
	subnets := [][]string{{"Link ID", "Side", "Subnet"}}
	related := [][]string{{"Link ID", "Related link", "Relationship"}}
	for _, r := range rows {
		line := make([]string, len(cols))
		for i, c := range cols {
			line[i] = tabular.Safe(c.get(r))
		}
		main = append(main, line)
		for _, s := range r.LocalSubnets {
			subnets = append(subnets, []string{r.Code, "Local", s})
		}
		for _, s := range r.RemoteSubnets {
			subnets = append(subnets, []string{r.Code, "Remote", s})
		}
		for _, x := range r.Related {
			code, kind, _ := cutColon(x)
			related = append(related, []string{r.Code, code, kind})
		}
	}
	if err := fillSheets(f, map[string][][]string{"Network links": main, "VPN subnets": subnets, "Related links": related},
		[]string{"Network links", "VPN subnets", "Related links"}); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err := f.Write(&buf)
	return buf.Bytes(), err
}
