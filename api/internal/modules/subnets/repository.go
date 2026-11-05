package subnets

import (
	"context"
	"fmt"
	"strings"

	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type Repository struct{}

// countState counts records in a state, leaving out addresses already inside a
// reserved range so that nothing is counted twice.
func countState(state string) string {
	return `(SELECT count(*) FROM ip_addresses i WHERE i.subnet_id = s.id AND i.allocation_state = '` + state + `'
		AND NOT EXISTS (SELECT 1 FROM subnet_reserved_ranges r WHERE r.subnet_id = s.id AND i.address BETWEEN r.range_start AND r.range_end))`
}

var columns = `s.id::text, s.routing_domain_id::text, rd.name, s.cidr::text, s.name, host(s.gateway), s.environment,
	s.security_zone, s.description, s.discovery_enabled, s.warn_threshold, s.critical_threshold, s.sensitivity,
	s.version, s.archived_at, s.created_at, s.updated_at, ` +
	countState("allocated") + ", " + countState("reserved") + ", " + countState("quarantined") + `,
	s.network_block_id::text, nb.name, s.site_id::text, st.name, s.network_zone_id::text, nz.name, s.vdom_id::text, vd.name,
	s.vlan_id::text, CASE WHEN vl.id IS NULL THEN NULL ELSE vl.vlan_id::text || CASE WHEN vl.name = '' THEN '' ELSE ' ' || vl.name END END,
	s.business_unit_id::text, bu.name, s.business_service_id::text, bs.name,
	s.technical_owner_id::text, tu.email, s.business_owner_id::text, ou.email, s.overlap_exception_id::text`

const from = ` FROM subnets s
	JOIN routing_domains rd ON rd.id = s.routing_domain_id
	LEFT JOIN network_blocks nb ON nb.id = s.network_block_id
	LEFT JOIN sites st ON st.id = s.site_id
	LEFT JOIN network_zones nz ON nz.id = s.network_zone_id
	LEFT JOIN vdoms vd ON vd.id = s.vdom_id
	LEFT JOIN vlans vl ON vl.id = s.vlan_id
	LEFT JOIN business_units bu ON bu.id = s.business_unit_id
	LEFT JOIN business_services bs ON bs.id = s.business_service_id
	LEFT JOIN users tu ON tu.id = s.technical_owner_id
	LEFT JOIN users ou ON ou.id = s.business_owner_id`

func scan(row pgx.Row) (Subnet, error) {
	var s Subnet
	err := row.Scan(&s.ID, &s.RoutingDomainID, &s.RoutingDomainName, &s.CIDR, &s.Name, &s.Gateway, &s.Environment,
		&s.SecurityZone, &s.Description, &s.DiscoveryEnabled, &s.WarnThreshold, &s.CriticalThreshold, &s.Sensitivity,
		&s.Version, &s.ArchivedAt, &s.CreatedAt, &s.UpdatedAt, &s.Allocated, &s.Reserved, &s.Quarantined,
		&s.NetworkBlockID, &s.NetworkBlockName, &s.SiteID, &s.SiteName, &s.NetworkZoneID, &s.NetworkZoneName,
		&s.VDOMID, &s.VDOMName, &s.VLANID, &s.VLANName, &s.BusinessUnitID, &s.BusinessUnitName,
		&s.BusinessServiceID, &s.BusinessServiceName, &s.TechnicalOwnerID, &s.TechnicalOwnerName,
		&s.BusinessOwnerID, &s.BusinessOwnerName, &s.OverlapExceptionID)
	return s, err
}

// Sort fields are allowlisted, since they can't be passed as query parameters.
var sortColumns = map[string]string{
	"name": "s.name", "cidr": "s.cidr", "environment": "s.environment", "updatedAt": "s.updated_at",
	"utilisation": `(SELECT count(*) FROM ip_addresses i WHERE i.subnet_id = s.id AND i.allocation_state IN ('allocated','reserved','quarantined'))::numeric
		/ power(2::numeric, (CASE family(s.cidr) WHEN 4 THEN 32 ELSE 128 END) - masklen(s.cidr))`,
}

func orderBy(sort string) string {
	direction := "ASC"
	if strings.HasPrefix(sort, "-") {
		direction, sort = "DESC", sort[1:]
	}
	column, ok := sortColumns[sort]
	if !ok {
		return "s.cidr ASC, s.id"
	}
	return column + " " + direction + ", s.id"
}

func (Repository) List(ctx context.Context, q querier, f Filter, p httpx.Page) ([]Subnet, int64, error) {
	var where []string
	var args []any
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if !f.IncludeArchived {
		where = append(where, "s.archived_at IS NULL")
	}
	if f.Query != "" {
		add("(s.name ILIKE '%%' || $%[1]d || '%%' OR s.cidr::text ILIKE '%%' || $%[1]d || '%%')", f.Query)
	}
	if f.RoutingDomainID != "" {
		add("s.routing_domain_id = $%d::uuid", f.RoutingDomainID)
	}
	if f.Environment != "" {
		add("s.environment = $%d", f.Environment)
	}
	for clause, value := range map[string]string{
		"s.site_id = $%d::uuid": f.SiteID, "s.vlan_id = $%d::uuid": f.VLANID, "s.vdom_id = $%d::uuid": f.VDOMID,
		"s.business_service_id = $%d::uuid": f.BusinessServiceID, "s.network_block_id = $%d::uuid": f.NetworkBlockID,
	} {
		if value != "" {
			add(clause, value)
		}
	}
	if f.Family == 4 || f.Family == 6 {
		add("family(s.cidr) = $%d", f.Family)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	if err := q.QueryRow(ctx, "SELECT count(*)"+from+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, p.Size, p.Offset())
	rows, err := q.Query(ctx, fmt.Sprintf("SELECT %s%s%s ORDER BY %s LIMIT $%d OFFSET $%d",
		columns, from, clause, orderBy(f.Sort), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Subnet
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (Repository) Get(ctx context.Context, q querier, id string, lock bool) (Subnet, error) {
	sql := "SELECT " + columns + from + " WHERE s.id = $1::uuid"
	if lock {
		sql += " FOR UPDATE OF s"
	}
	return scan(q.QueryRow(ctx, sql, id))
}

func (Repository) Insert(ctx context.Context, q querier, actor string, r CreateRequest, cidr string, warn, critical int16) (string, error) {
	var id string
	err := q.QueryRow(ctx, `
		INSERT INTO subnets (routing_domain_id, cidr, name, gateway, environment, security_zone, description,
		                     discovery_enabled, warn_threshold, critical_threshold, created_by, updated_by,
		                     network_block_id, site_id, network_zone_id, vdom_id, vlan_id, business_unit_id,
		                     business_service_id, technical_owner_id, business_owner_id, overlap_exception_id)
		VALUES ($1::uuid, $2::text::cidr, $3, $4::text::inet, $5, $6, $7, $8, $9, $10, $11::uuid, $11::uuid,
		        $12::uuid, $13::uuid, $14::uuid, $15::uuid, $16::uuid, $17::uuid, $18::uuid, $19::uuid, $20::uuid, $21::uuid)
		RETURNING id::text`,
		r.RoutingDomainID, cidr, r.Name, r.Gateway, r.Environment, r.SecurityZone, r.Description,
		r.DiscoveryEnabled, warn, critical, actor, r.NetworkBlockID, r.SiteID, r.NetworkZoneID, r.VDOMID, r.VLANID,
		r.BusinessUnitID, r.BusinessServiceID, r.TechnicalOwnerID, r.BusinessOwnerID, r.OverlapExceptionID).Scan(&id)
	return id, err
}

func (Repository) Update(ctx context.Context, q querier, actor string, s Subnet) error {
	_, err := q.Exec(ctx, `
		UPDATE subnets SET name = $2, gateway = $3::text::inet, environment = $4, security_zone = $5, description = $6,
		       discovery_enabled = $7, warn_threshold = $8, critical_threshold = $9, updated_by = $10::uuid,
		       sensitivity = $11, network_block_id = $12::uuid, site_id = $13::uuid, network_zone_id = $14::uuid,
		       vdom_id = $15::uuid, vlan_id = $16::uuid, business_unit_id = $17::uuid, business_service_id = $18::uuid,
		       technical_owner_id = $19::uuid, business_owner_id = $20::uuid, version = version + 1
		WHERE id = $1::uuid`,
		s.ID, s.Name, s.Gateway, s.Environment, s.SecurityZone, s.Description, s.DiscoveryEnabled,
		s.WarnThreshold, s.CriticalThreshold, actor, s.Sensitivity, s.NetworkBlockID, s.SiteID, s.NetworkZoneID,
		s.VDOMID, s.VLANID, s.BusinessUnitID, s.BusinessServiceID, s.TechnicalOwnerID, s.BusinessOwnerID)
	return err
}

func (Repository) Archive(ctx context.Context, q querier, actor, id string) error {
	_, err := q.Exec(ctx, `UPDATE subnets SET archived_at = now(), updated_by = $2::uuid, version = version + 1 WHERE id = $1::uuid`, id, actor)
	return err
}

func (Repository) Delete(ctx context.Context, q querier, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM subnets WHERE id = $1::uuid`, id)
	return err
}
