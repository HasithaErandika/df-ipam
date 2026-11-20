package links

import (
	"bytes"
	"net/http"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// template downloads an empty import workbook: the columns, and a sheet that
// explains each one with the values it accepts.
func (h *Handler) template(c *gin.Context) {
	s, err := loadSettings(c, h.conn)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	cols := visibleColumns(can(c, auth.LinksCommercialManage))
	types := make([]string, 0, len(s.Types))
	for _, t := range s.Types {
		if t.Active {
			types = append(types, t.Label)
		}
	}
	help := [][]string{
		{"Column", "What to enter"},
		{"Link ID", "Leave empty to add a link. An existing Link ID (DF-LNK-0001) updates that link, after you confirm."},
		{"Link Name, Link Type, Source Location, Business Purpose, Link Role, Criticality", "Required."},
		{"Link Type", strings.Join(types, "; ")},
		{"Service Provider", "A provider's name as listed in Network Links > Service providers."},
		{"Source Location", strings.Join(s.Sites, "; ")},
		{"Environment", "production; dr; uat; other (or empty)"},
		{"Link Role", strings.Join(roles, "; ")},
		{"Criticality", strings.Join(criticalities, "; ")},
		{"Status", strings.Join(statuses, "; ") + " (empty means active)"},
		{"Responsible Officer", "The person's sign-in email."},
		{"Download Mbps, Upload Mbps", "Numbers in Mbps: 1 Gbps is 1000."},
		{"Connection Medium", "fibre; wireless; copper; other"},
		{"WAN IP, Gateway IP", "An IPv4 address, with its prefix or mask if you have it: 10.10.40.253 /24 or 122.255.4.226 255.255.255.248. The address is kept."},
		{"WAN Subnet, Local Subnets", "A segment's network (192.168.10.0/24) or name. Several are separated by ;. An empty WAN Subnet uses the WAN IP's prefix when that network is a segment."},
		{"Public IPs", "One IPv4 address or a network such as 203.0.113.0/29 or 122.255.4.224 255.255.255.248."},
		{"Connected Device", "An approved device's name or MAC, or any text."},
		{"Routing", "static; bgp; ospf; other"},
		{"VPN Type", "ipsec; ssl; other. VPN columns are read for VPN link types only."},
		{"Remote Subnets", "The far side's networks, separated by ;. They may overlap other links' and are never counted as your address space."},
		{"Related Links", "Link IDs with the relationship: DF-LNK-0002:backup; DF-LNK-0003:underlying (or secondary)."},
		{"Dates", "YYYY-MM-DD"},
		{"No Separate Cost", "Yes for a link paid for through another, such as a VPN over an internet line."},
		{"Currency", "LKR, USD or another three-letter code."},
		{"Secrets", "Never put pre-shared keys or passwords in this file. Use Vault Reference to say where they are kept."},
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := fillSheets(f, map[string][][]string{"Network links": {headers(cols)}, "How to fill it in": help},
		[]string{"Network links", "How to fill it in"}); err != nil {
		httpx.Fail(c, err)
		return
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="network-links-import-template.xlsx"`)
	c.Data(http.StatusOK, tabular.Formats["xlsx"], buf.Bytes())
}

// fillSheets writes each sheet in order, the first replacing Sheet1, with a
// bold, frozen header row.
func fillSheets(f *excelize.File, sheets map[string][][]string, order []string) error {
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#E8EEF9"}, Pattern: 1}})
	if err != nil {
		return err
	}
	for i, name := range order {
		if i == 0 {
			err = f.SetSheetName("Sheet1", name)
		} else {
			_, err = f.NewSheet(name)
		}
		if err == nil {
			err = writeSheet(f, name, sheets[name], bold)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func writeSheet(f *excelize.File, name string, rows [][]string, bold int) error {
	for r, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, r+1)
		values := make([]any, len(row))
		for j, v := range row {
			values[j] = v
		}
		if err := f.SetSheetRow(name, cell, &values); err != nil {
			return err
		}
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return nil
	}
	last, _ := excelize.CoordinatesToCellName(len(rows[0]), 1)
	if err := f.SetCellStyle(name, "A1", last, bold); err != nil {
		return err
	}
	return f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}

func cutColon(s string) (string, string, bool) { return strings.Cut(s, ":") }
