package imports

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// nameOf is how a name is compared: case never matters.
func nameOf(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// aliasQueries give each record's names. The first name is always the unique one,
// such as DF/Corporate VRF/10.0.1.0/24. The others let a file use a shorter one.
var aliasQueries = map[string]string{
	"people":            `SELECT id::text, ARRAY[lower(email)] AS names FROM users WHERE is_active`,
	"organisations":     `SELECT id::text, ARRAY[lower(code), lower(name)] AS names FROM organisations`,
	"routing_domains":   `SELECT r.id::text, ARRAY[lower(o.code||'/'||r.name), lower(o.name||'/'||r.name), lower(r.name)] AS names FROM routing_domains r JOIN organisations o ON o.id = r.organisation_id`,
	"business_units":    childOfOrg("business_units"),
	"sites":             childOfOrg("sites"),
	"network_zones":     `SELECT z.id::text, array_remove(ARRAY[lower(o.code||'/'||s.code||'/'||z.name), lower(s.code||'/'||z.name), lower(s.name||'/'||z.name), lower(z.name)], NULL) AS names FROM network_zones z LEFT JOIN sites s ON s.id = z.site_id LEFT JOIN organisations o ON o.id = s.organisation_id`,
	"business_services": `SELECT id::text, ARRAY[lower(name)] AS names FROM business_services`,
	"applications":      `SELECT id::text, ARRAY[lower(name)] AS names FROM applications`,
	"teams":             `SELECT id::text, ARRAY[lower(name)] AS names FROM teams`,
	"devices":           `SELECT id::text, ARRAY[lower(name)] AS names FROM devices`,
	"vdoms":             `SELECT v.id::text, ARRAY[lower(d.name||'/'||v.name), lower(v.name)] AS names FROM vdoms v JOIN devices d ON d.id = v.device_id`,
	"vlans":             `SELECT v.id::text, array_remove(ARRAY[lower(o.code||'/'||s.code||'/'||v.vlan_id), lower(s.code||'/'||v.vlan_id), lower(s.name||'/'||v.vlan_id), v.vlan_id::text], NULL) AS names FROM vlans v LEFT JOIN sites s ON s.id = v.site_id LEFT JOIN organisations o ON o.id = s.organisation_id`,
	"interfaces":        `SELECT i.id::text, ARRAY[lower(d.name||'/'||i.name)] AS names FROM device_interfaces i JOIN devices d ON d.id = i.device_id`,
	"network_blocks":    inDomain("network_blocks", "t.cidr::text", ""),
	"subnets":           inDomain("subnets", "t.cidr::text", "WHERE t.archived_at IS NULL"),
	"ip_addresses":      inDomain("ip_addresses", "host(t.address)", ""),
	"reserved_ranges":   inSubnet("subnet_reserved_ranges"),
	"dhcp_scopes":       inSubnet("dhcp_scopes"),
}

func childOfOrg(table string) string {
	return `SELECT t.id::text, ARRAY[lower(o.code||'/'||t.code), lower(o.code||'/'||t.name), lower(o.name||'/'||t.code), lower(o.name||'/'||t.name), lower(t.code), lower(t.name)] AS names
		FROM ` + table + ` t JOIN organisations o ON o.id = t.organisation_id`
}

func inDomain(table, value, where string) string {
	return `SELECT t.id::text, ARRAY[lower(o.code||'/'||r.name||'/'||` + value + `), lower(o.name||'/'||r.name||'/'||` + value + `), lower(r.name||'/'||` + value + `), ` + value + `] AS names
		FROM ` + table + ` t JOIN routing_domains r ON r.id = t.routing_domain_id JOIN organisations o ON o.id = r.organisation_id ` + where
}

func inSubnet(table string) string {
	return `SELECT t.id::text, ARRAY[lower(o.code||'/'||r.name||'/'||n.cidr::text||'/'||host(t.range_start))] AS names
		FROM ` + table + ` t JOIN subnets n ON n.id = t.subnet_id JOIN routing_domains r ON r.id = n.routing_domain_id JOIN organisations o ON o.id = r.organisation_id`
}

// entry is one record a file can refer to, in the inventory or earlier in the file.
type entry struct {
	id    string
	names []string
}

// index finds records by name. Each load is one query for all the names it is given.
type index struct {
	tx    pgx.Tx
	found map[string]map[string][]*entry
	known map[string]bool
}

func newIndex(tx pgx.Tx) *index {
	return &index{tx: tx, found: map[string]map[string][]*entry{}, known: map[string]bool{}}
}

func (x *index) add(kind string, e *entry) {
	if x.known[kind+"|"+e.id] {
		return
	}
	x.known[kind+"|"+e.id] = true
	if x.found[kind] == nil {
		x.found[kind] = map[string][]*entry{}
	}
	for _, n := range slices.Compact(slices.Sorted(slices.Values(e.names))) {
		x.found[kind][n] = append(x.found[kind][n], e)
	}
}

func (x *index) load(ctx context.Context, kind string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	rows, err := x.tx.Query(ctx, `SELECT id, names FROM (`+aliasQueries[kind]+`) t WHERE names && $1::text[]`, names)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (*entry, error) {
		e := &entry{}
		return e, r.Scan(&e.id, &e.names)
	})
	for _, e := range list {
		x.add(kind, e)
	}
	return err
}

// find returns what a name means. More than one match is for the caller to refuse.
func (x *index) find(kind, name string) []*entry { return x.found[kind][nameOf(name)] }
