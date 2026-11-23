package inventory

import (
	"net/http"
	"net/netip"
	"strings"

	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// segmentRow is one network from the text, and what the import does with it.
type segmentRow struct {
	Line     int    `json:"line"`
	Name     string `json:"name"`
	Network  string `json:"network"`
	Gateway  string `json:"gateway"`
	VLAN     *int   `json:"vlan"`
	Firewall string `json:"firewall"`
	Down     bool   `json:"down"`
	Add      bool   `json:"add"`
	Reason   string `json:"reason"`
}

type segmentImportResult struct {
	DryRun  bool         `json:"dryRun"`
	Added   int          `json:"added"`
	Skipped int          `json:"skipped"`
	Rows    []segmentRow `json:"rows"`
}

type existingSegment struct {
	name    string
	network netip.Prefix
}

// importSegments adds segments from pasted text: FortiGate "show system
// interface" output, or lines holding a CIDR and a name. ?dryRun=true only
// reports what would happen. Lines in skip are left out on purpose.
func (h *Handler) importSegments(c *gin.Context) {
	var in struct {
		Text string `json:"text"`
		Kind string `json:"kind"`
		Skip []int  `json:"skip"`
	}
	if !httpx.Bind(c, &in) {
		return
	}
	var v validate.V
	if in.Kind == "" {
		in.Kind = "server"
	}
	v.OneOf("kind", in.Kind, []string{"dhcp", "server", "cloud", "vpn"}, "segment type")
	if strings.TrimSpace(in.Text) == "" {
		v.Add("text", "Paste the text, or choose a .txt file.")
	}
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	existing, err := h.existingSegments(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows := planSegments(parseSegmentText(in.Text), existing, in.Skip)
	if len(rows) > maxImportRows {
		httpx.Fail(c, httpx.Validation("text", "Import up to 5,000 networks at a time."))
		return
	}
	dry := isDryRun(c)
	result := segmentImportResult{DryRun: dry, Rows: rows}
	err = importTx(c, h.conn, dry, func(tx pgx.Tx) error {
		n, err := insertSegments(c, tx, rows, in.Kind)
		result.Added = n
		return err
	})
	if err != nil {
		httpx.Fail(c, friendlyIP(err))
		return
	}
	result.Skipped = len(rows) - result.Added
	if !dry {
		h.recheck(c)
	}
	httpx.OK(c, http.StatusOK, result)
}

func (h *Handler) existingSegments(c *gin.Context) ([]existingSegment, error) {
	rows, err := h.conn.Query(c, "SELECT name, network::text FROM segments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []existingSegment
	for rows.Next() {
		var name, network string
		if err := rows.Scan(&name, &network); err != nil {
			return nil, err
		}
		if p, err := netip.ParsePrefix(network); err == nil {
			out = append(out, existingSegment{name, p})
		}
	}
	return out, rows.Err()
}
