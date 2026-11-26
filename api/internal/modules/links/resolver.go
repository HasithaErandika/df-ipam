package links

import (
	"context"
	"strconv"
	"strings"

	"dfipam/api/internal/platform/db"
)

// resolver turns the names in an imported file (provider names, emails,
// subnets, device names, link IDs) into ids, noting any it can't find.
type resolver struct {
	ids      map[string]map[string]string
	shared   map[string]map[string]bool // names held by more than one record
	settings Settings
	issues   []string
}

var resolverQueries = map[string]string{
	"providers": `SELECT lower(name) AS k, id::text AS v FROM network_link_providers`,
	"officers":  `SELECT lower(email), id::text FROM users WHERE is_active`,
	"segments": `SELECT lower(network::text), id::text FROM segments UNION ALL SELECT lower(name), id::text FROM segments
		UNION ALL SELECT lower(host(network)) || '/' || masklen(network), id::text FROM segments`,
	"devices": `SELECT lower(mac::text), id::text FROM devices UNION ALL SELECT lower(name), id::text FROM devices WHERE name <> ''`,
	"links":   `SELECT lower(code), id::text FROM network_links`,
	// Provider and circuit ID of each live link, read once so an import doesn't
	// query per row. The key is circuitKey(provider, circuit).
	"circuits": `SELECT lower(p.name) || chr(31) || lower(l.circuit_id), l.id::text FROM network_links l
		JOIN network_link_providers p ON p.id = l.provider_id WHERE l.archived_at IS NULL AND COALESCE(l.circuit_id, '') <> ''`,
}

// circuitKey matches the "circuits" lookup: provider and circuit ID, any case.
func circuitKey(provider, circuit string) string {
	return strings.ToLower(provider) + "\x1f" + strings.ToLower(circuit)
}

var uniqueBy = map[string]string{"circuits": "provider and circuit ID", "providers": "exact name", "officers": "email", "segments": "network", "devices": "MAC address", "links": "link ID"}

var resolverLabels = map[string]string{"circuits": "link", "providers": "provider", "officers": "active user", "segments": "subnet", "devices": "device", "links": "link ID"}

func newResolver(ctx context.Context, conn db.Conn, s Settings) (*resolver, error) {
	x := &resolver{ids: map[string]map[string]string{}, shared: map[string]map[string]bool{}, settings: s}
	for kind, q := range resolverQueries {
		if err := x.load(ctx, conn, kind, q); err != nil {
			return nil, err
		}
	}
	return x, nil
}

// load reads one kind's names. A name two records share (two devices called
// "Firewall") is marked, so the import asks for something unique instead of
// picking one.
func (x *resolver) load(ctx context.Context, conn db.Conn, kind, q string) error {
	rows, err := conn.Query(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	m, shared := map[string]string{}, map[string]bool{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		if old, ok := m[k]; ok && old != v {
			shared[k] = true
		}
		m[k] = v
	}
	x.ids[kind], x.shared[kind] = m, shared
	return rows.Err()
}

// find returns the id for a name, or "" (noting it) when there is none.
func (x *resolver) find(kind, name string) string {
	if name = strings.TrimSpace(name); name == "" {
		return ""
	}
	if x.shared[kind][strings.ToLower(name)] {
		x.issues = append(x.issues, "More than one "+resolverLabels[kind]+" is called "+name+": give its "+uniqueBy[kind]+" instead.")
		return ""
	}
	if id, ok := x.ids[kind][strings.ToLower(name)]; ok {
		return id
	}
	if kind != "devices" { // an unknown device is kept as text
		x.issues = append(x.issues, "Unknown "+resolverLabels[kind]+": "+name+".")
	}
	return ""
}

func (x *resolver) findAll(kind, list string) []string {
	out := []string{}
	for _, name := range split(list) {
		if id := x.find(kind, name); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// linkType accepts a type's key or its name.
func (x *resolver) linkType(v string) string {
	for _, t := range x.settings.Types {
		if strings.EqualFold(t.Key, v) || strings.EqualFold(t.Label, v) {
			return t.Key
		}
	}
	if v != "" {
		x.issues = append(x.issues, "Unknown link type: "+v+".")
	}
	return v
}

// relations reads "DF-LNK-0002:backup; DF-LNK-0003:underlying".
func (x *resolver) relations(v string) []relationInput {
	out := []relationInput{}
	for _, part := range split(v) {
		code, kind, _ := strings.Cut(part, ":")
		if id := x.find("links", code); id != "" {
			out = append(out, relationInput{RelatedID: id, Kind: lower(firstOf(kind, "backup"))})
		}
	}
	return out
}

func lower(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func split(v string) []string {
	out := []string{}
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ';' || r == ',' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func isYes(v string) bool {
	switch lower(v) {
	case "yes", "y", "true", "1":
		return true
	}
	return false
}

// bad is a value no field accepts, so a cell that isn't a number is reported.
var bad = -1.0

func parseNum(v string) *float64 {
	v = strings.ReplaceAll(strings.TrimSpace(v), ",", "")
	if v == "" {
		return nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		b := bad
		return &b
	}
	return &n
}

func parseInt(v string) *int {
	if n := parseNum(v); n != nil {
		i := int(*n)
		return &i
	}
	return nil
}
