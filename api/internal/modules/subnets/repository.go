package subnets

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"dfipam/api/internal/netcalc"
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
		"s.network_zone_id = $%d::uuid": f.NetworkZoneID, "s.business_unit_id = $%d::uuid": f.BusinessUnitID,
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

// Overlaps includes exception subnets, so a warning never hides a real clash.
func (Repository) Overlaps(ctx context.Context, q querier, routingDomainID, cidr string) ([]Conflict, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name, cidr::text FROM subnets
		WHERE routing_domain_id = $1::uuid AND archived_at IS NULL AND cidr && $2::text::cidr
		ORDER BY masklen(cidr) DESC LIMIT 10`, routingDomainID, cidr)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Conflict, error) {
		var c Conflict
		return c, r.Scan(&c.ID, &c.Name, &c.CIDR)
	})
	if out == nil {
		out = []Conflict{}
	}
	return out, err
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

// ReservedRanges loads reserved ranges for many subnets in one query.
func (Repository) ReservedRanges(ctx context.Context, q querier, ids []string) (map[string][]netcalc.Range, error) {
	rows, err := q.Query(ctx, `SELECT subnet_id::text, host(range_start), host(range_end) FROM subnet_reserved_ranges WHERE subnet_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]netcalc.Range{}
	for rows.Next() {
		var id, first, last string
		if err := rows.Scan(&id, &first, &last); err != nil {
			return nil, err
		}
		a, errA := netip.ParseAddr(first)
		b, errB := netip.ParseAddr(last)
		if errA != nil || errB != nil {
			continue
		}
		out[id] = append(out[id], netcalc.Range{First: a, Last: b})
	}
	return out, rows.Err()
}

// HeldAddresses returns addresses that can't be offered: records in a held state.
func (Repository) HeldAddresses(ctx context.Context, q querier, id string) ([]netip.Addr, error) {
	rows, err := q.Query(ctx, `SELECT host(address) FROM ip_addresses
		WHERE subnet_id = $1::uuid AND allocation_state IN ('allocated','reserved','quarantined','retired') ORDER BY address`, id)
	if err != nil {
		return nil, err
	}
	hosts, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(hosts))
	for _, h := range hosts {
		if a, err := netip.ParseAddr(h); err == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// DHCPRanges returns the subnet's DHCP pools.
func (Repository) DHCPRanges(ctx context.Context, q querier, id string) ([]netcalc.Range, error) {
	rows, err := q.Query(ctx, `SELECT host(range_start), host(range_end) FROM dhcp_scopes WHERE subnet_id = $1::uuid`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []netcalc.Range
	for rows.Next() {
		var first, last string
		if err := rows.Scan(&first, &last); err != nil {
			return nil, err
		}
		a, errA := netip.ParseAddr(first)
		b, errB := netip.ParseAddr(last)
		if errA == nil && errB == nil {
			out = append(out, netcalc.Range{First: a, Last: b})
		}
	}
	return out, rows.Err()
}

// Cell is a recorded address in a loaded range.
type Cell struct {
	ID, Address, AllocationState, ObservationState, Hostname string
}

// Cells loads only the records inside one range of the subnet.
func (Repository) Cells(ctx context.Context, q querier, id, first, last string) ([]Cell, error) {
	rows, err := q.Query(ctx, `SELECT id::text, host(address), allocation_state, observation_state, hostname FROM ip_addresses
		WHERE subnet_id = $1::uuid AND address BETWEEN $2::text::inet AND $3::text::inet`, id, first, last)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Cell, error) {
		var c Cell
		return c, r.Scan(&c.ID, &c.Address, &c.AllocationState, &c.ObservationState, &c.Hostname)
	})
}

// MappingProblem reports a mapping that contradicts the subnet's own context.
func (Repository) MappingProblem(ctx context.Context, q querier, routingDomainID, cidr string, m Mappings) (string, string, error) {
	var field, issue string
	err := q.QueryRow(ctx, `
		SELECT field, issue FROM (
		  SELECT 'networkBlockId' field, 'The network block must be in the same routing domain and contain this subnet.' issue
		  FROM network_blocks b WHERE b.id = $3::uuid AND (b.routing_domain_id <> $1::uuid OR NOT $2::text::cidr <<= b.cidr)
		  UNION ALL
		  SELECT 'vdomId', 'This VDOM belongs to a different routing domain.'
		  FROM vdoms v WHERE v.id = $4::uuid AND v.routing_domain_id IS NOT NULL AND v.routing_domain_id <> $1::uuid
		  UNION ALL
		  SELECT 'vlanId', 'This VLAN belongs to a different site.'
		  FROM vlans l WHERE l.id = $5::uuid AND $6::uuid IS NOT NULL AND l.site_id IS NOT NULL AND l.site_id <> $6::uuid
		  UNION ALL
		  SELECT 'networkZoneId', 'This zone belongs to a different site.'
		  FROM network_zones z WHERE z.id = $7::uuid AND $6::uuid IS NOT NULL AND z.site_id IS NOT NULL AND z.site_id <> $6::uuid
		  UNION ALL
		  SELECT 'overlapExceptionId', 'This overlap exception has expired.'
		  FROM overlap_exceptions e WHERE e.id = $8::uuid AND e.expires_at < now()
		) p LIMIT 1`, routingDomainID, cidr, m.NetworkBlockID, m.VDOMID, m.VLANID, m.SiteID, m.NetworkZoneID, m.OverlapExceptionID).Scan(&field, &issue)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return field, issue, err
}

// SetOverlapException runs only when the exception changes. Listing the column in
// every update would re-run the exception-overlap trigger on unrelated edits.
func (Repository) SetOverlapException(ctx context.Context, q querier, id string, exceptionID *string) error {
	_, err := q.Exec(ctx, `UPDATE subnets SET overlap_exception_id = $2::uuid WHERE id = $1::uuid`, id, exceptionID)
	return err
}

// AddProviderRanges records the addresses AWS or Azure keep in a cloud subnet,
// so utilisation and candidates account for them from the start.
func (Repository) AddProviderRanges(ctx context.Context, q querier, id, routingDomainID string, p netip.Prefix) error {
	var kind string
	if err := q.QueryRow(ctx, `SELECT kind FROM routing_domains WHERE id = $1::uuid`, routingDomainID).Scan(&kind); err != nil {
		return err
	}
	provider, ranges := ProviderReserved(kind, p)
	reason := map[string]string{"aws": "Reserved by AWS", "azure": "Reserved by Azure"}[provider]
	for _, r := range ranges {
		if _, err := q.Exec(ctx, `INSERT INTO subnet_reserved_ranges (subnet_id, range_start, range_end, reason, provider)
			VALUES ($1::uuid, $2::text::inet, $3::text::inet, $4, $5)`, id, r.First.String(), r.Last.String(), reason, provider); err != nil {
			return err
		}
	}
	return nil
}

func (Repository) Archive(ctx context.Context, q querier, actor, id string) error {
	_, err := q.Exec(ctx, `UPDATE subnets SET archived_at = now(), updated_by = $2::uuid, version = version + 1 WHERE id = $1::uuid`, id, actor)
	return err
}

func (Repository) Delete(ctx context.Context, q querier, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM subnets WHERE id = $1::uuid`, id)
	return err
}
