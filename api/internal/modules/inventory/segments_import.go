package inventory

import (
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"dfipam/api/internal/audit"
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
	v.OneOf("kind", in.Kind, []string{"dhcp", "server", "cloud"}, "segment type")
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

// planSegments decides each row: added, or skipped with a reason people can act
// on. A name already in use gets the firewall's name added, when known.
func planSegments(parsed []textSegment, existing []existingSegment, skip []int) []segmentRow {
	names := map[string]bool{}
	for _, e := range existing {
		names[strings.ToLower(e.name)] = true
	}
	out := make([]segmentRow, 0, len(parsed))
	for _, p := range parsed {
		r := segmentRow{Line: p.Line, Name: p.Name, VLAN: p.VLAN, Firewall: p.Firewall, Down: p.Down, Reason: p.Skip}
		if p.Network.IsValid() {
			r.Network = p.Network.String()
		}
		if p.Gateway.IsValid() {
			r.Gateway = p.Gateway.String()
		}
		if r.Reason == "" {
			r.Reason = clash(p.Network, existing)
		}
		if r.Reason == "" && slices.Contains(skip, p.Line) {
			r.Reason = "Left out"
		}
		if r.Reason == "" {
			r.Name, r.Reason = freeName(r.Name, p.Firewall, names)
		}
		if r.Add = r.Reason == ""; r.Add {
			names[strings.ToLower(r.Name)] = true
			existing = append(existing, existingSegment{r.Name, p.Network})
		}
		out = append(out, r)
	}
	return out
}

func clash(network netip.Prefix, existing []existingSegment) string {
	for _, e := range existing {
		if e.network == network {
			return "Already a segment: " + e.name
		}
		if e.network.Overlaps(network) {
			return "Overlaps " + e.name + " (" + e.network.String() + ")"
		}
	}
	return ""
}

func freeName(name, firewall string, taken map[string]bool) (string, string) {
	if len(name) > 80 {
		name = name[:80]
	}
	if !taken[strings.ToLower(name)] {
		return name, ""
	}
	if firewall != "" {
		alt := name + " (" + firewall + ")"
		if len(alt) <= 80 && !taken[strings.ToLower(alt)] {
			return alt, ""
		}
	}
	return name, "The name " + name + " is already used"
}

// insertSegments adds the planned rows in one statement and audits each.
func insertSegments(c *gin.Context, tx pgx.Tx, rows []segmentRow, kind string) (int, error) {
	var names, vlans, networks, gateways, notes []string
	for _, r := range rows {
		if !r.Add {
			continue
		}
		vlan := ""
		if r.VLAN != nil {
			vlan = strconv.Itoa(*r.VLAN)
		}
		names, vlans, networks, gateways = append(names, r.Name), append(vlans, vlan), append(networks, r.Network), append(gateways, r.Gateway)
		notes = append(notes, describeImported(r))
	}
	if len(names) == 0 {
		return 0, nil
	}
	uid := actor(c)
	inserted, err := tx.Query(c, `INSERT INTO segments (name, kind, vlan, network, gateway, description, created_by, updated_by)
		SELECT n, $6, NULLIF(v, '')::integer, net::cidr, NULLIF(g, '')::inet, d, $7::uuid, $7::uuid
		FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[]) AS x(n, v, net, g, d)
		RETURNING id::text, name, network::text`, names, vlans, networks, gateways, notes, kind, uid)
	if err != nil {
		return 0, err
	}
	var entries []audit.Entry
	for inserted.Next() {
		var id, name, network string
		if err := inserted.Scan(&id, &name, &network); err != nil {
			inserted.Close()
			return 0, err
		}
		entries = append(entries, audit.Entry{ActorID: uid, Action: "import", ObjectType: "segment", ObjectID: id,
			New: map[string]string{"name": name, "network": network, "kind": kind}})
	}
	inserted.Close()
	if err := inserted.Err(); err != nil {
		return 0, err
	}
	return len(entries), audit.WriteMany(c, tx, entries)
}

func describeImported(r segmentRow) string {
	d := "Imported from text"
	if r.Firewall != "" {
		d = "Interface on " + r.Firewall
	}
	if r.Down {
		d += ". The interface was down when imported"
	}
	return d + "."
}
